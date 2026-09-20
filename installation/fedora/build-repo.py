#!/usr/bin/env python3
"""Fedora 44 offline RPM repository builder and dependency closure resolver."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

PINNED_KEYS = {
    "RPM-GPG-KEY-fedora-44-primary": "36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6",
    "RPM-GPG-KEY-rpmfusion-free-fedora-2020": "E9A491A3DE247814E7E067EAE06F8ECDD651FF2E",
    "RPM-GPG-KEY-rpmfusion-nonfree-fedora-2020": "79BDB88F9BBF73910FD4095B6A2AF96194843C65",
    "RPM-GPG-KEY-ryoku-copr": "7575D19C9D8ECDC212702114ED1C392039749395",
}

PINNED_RPMFUSION_RELEASES = [
    "rpmfusion-free-release-44",
    "rpmfusion-nonfree-release-44",
]

EXCLUDED_FREE_FFMPEG = [
    "ffmpeg-free",
    "libavcodec-free",
    "libavdevice-free",
    "libavfilter-free",
    "libavformat-free",
    "libavutil-free",
    "libswresample-free",
    "libswscale-free",
]

REQUIRED_RPMFUSION_FFMPEG = [
    "ffmpeg",
    "ffmpeg-libs",
    "libavdevice",
]


def extract_key_fingerprint(key_path, gpg_bin="gpg"):
    """Extract the primary key fingerprint from an armored GPG key file."""
    path = Path(key_path)
    if not path.is_file():
        raise FileNotFoundError(f"Key file not found: {key_path}")

    with tempfile.TemporaryDirectory() as temp_home:
        cmd = [gpg_bin, "--homedir", temp_home, "--batch", "--with-colons", "--show-keys", str(path)]
        output = subprocess.check_output(cmd, text=True, stderr=subprocess.DEVNULL)

    fingerprints = []
    primary = False
    for line in output.splitlines():
        parts = line.split(":")
        if parts[0] == "pub":
            primary = True
        elif parts[0] == "fpr" and primary:
            fingerprints.append(parts[9].strip().upper())
            primary = False

    if not fingerprints:
        raise ValueError(f"No fingerprint found in key file: {key_path}")
    return fingerprints[0]


def verify_key(key_path, expected_fingerprint, gpg_bin="gpg"):
    """Verify that a key file matches the expected fingerprint."""
    clean_expected = expected_fingerprint.replace(" ", "").strip().upper()
    actual = extract_key_fingerprint(key_path, gpg_bin=gpg_bin)
    if actual != clean_expected:
        raise ValueError(f"Key {key_path} fingerprint mismatch: got {actual}, expected {clean_expected}")
    return actual


def verify_all_pinned_keys(keys_dir, gpg_bin="gpg"):
    """Verify all pinned GPG keys located in keys_dir."""
    directory = Path(keys_dir)
    results = {}
    for filename, expected_fp in PINNED_KEYS.items():
        key_file = directory / filename
        if not key_file.is_file():
            # Check if key is available in system path
            sys_path = Path("/etc/pki/rpm-gpg") / filename
            if sys_path.is_file():
                key_file = sys_path
            else:
                raise FileNotFoundError(f"Required pinned key {filename} not found in {keys_dir} or /etc/pki/rpm-gpg")
        verified = verify_key(key_file, expected_fp, gpg_bin=gpg_bin)
        results[filename] = verified
    return results


def read_packages_list(list_path, selected_sections=None):
    """Read package closure definitions from a structured packages.list file."""
    path = Path(list_path)
    if not path.is_file():
        raise FileNotFoundError(f"Package list file not found: {list_path}")

    packages_by_section = {}
    current_section = "default"
    packages_by_section[current_section] = []

    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            stripped = line.split("#")[0].strip()
            if not stripped:
                continue
            if stripped.startswith("[") and stripped.endswith("]"):
                current_section = stripped[1:-1].strip()
                if current_section not in packages_by_section:
                    packages_by_section[current_section] = []
                continue
            packages_by_section[current_section].append(stripped)

    if selected_sections is not None:
        result = []
        for sec in selected_sections:
            result.extend(packages_by_section.get(sec, []))
    else:
        result = []
        for sec_pkgs in packages_by_section.values():
            result.extend(sec_pkgs)

    deduped = []
    seen = set()
    for pkg in result:
        if pkg not in seen:
            seen.add(pkg)
            deduped.append(pkg)
    return deduped


def build_dnf_resolve_args(
    packages,
    installroot,
    dnf_bin="dnf",
    repofrompath=None,
    disablerepo=None,
    enablerepo=None,
    exclude=None,
    install_weak_deps=False,
    assumeno=True,
    downloadonly=False,
    destdir=None,
):
    """Build DNF command line arguments for resolving transactions in an isolated installroot."""
    args = [dnf_bin]
    if installroot:
        args.append(f"--installroot={installroot}")
    if not install_weak_deps:
        args.append("--setopt=install_weak_deps=False")
    if disablerepo:
        args.append(f"--disablerepo={disablerepo}")
    if enablerepo:
        for r in enablerepo:
            args.append(f"--enablerepo={r}")
    if repofrompath:
        for r in repofrompath:
            args.append(f"--repofrompath={r}")
    if exclude:
        args.append(f"--exclude={','.join(exclude)}")
    if assumeno:
        args.append("--assumeno")
    if downloadonly:
        args.append("--downloadonly")
    if destdir:
        args.append(f"--destdir={destdir}")
    args.append("install")
    args.extend(packages)
    return args


def validate_ffmpeg_transaction(transaction_output):
    """Ensure RPM Fusion FFmpeg replaced ffmpeg-free with no forbidden free libraries."""
    lines = transaction_output.splitlines()
    installed_packages = []
    for line in lines:
        parts = line.strip().split()
        if parts:
            installed_packages.append(parts[0])

    forbidden = [p for p in installed_packages if any(f == p or p.startswith(f + ".") for f in EXCLUDED_FREE_FFMPEG)]
    if forbidden:
        raise RuntimeError(f"Transaction contains forbidden ffmpeg-free packages: {forbidden}")

    has_ffmpeg = any("ffmpeg" in p and "ffmpeg-free" not in p for p in lines)
    if not has_ffmpeg:
        raise RuntimeError("Transaction did not include full RPM Fusion ffmpeg package")
    return True


def query_rpm_nevra(rpm_path):
    """Query RPM metadata using the rpm CLI if available."""
    try:
        cmd = ["rpm", "-qp", "--qf", "%{NAME}|%{EPOCH}|%{VERSION}|%{RELEASE}|%{ARCH}", str(rpm_path)]
        output = subprocess.check_output(cmd, text=True, stderr=subprocess.DEVNULL).strip()
        parts = output.split("|")
        if len(parts) == 5:
            return {
                "name": parts[0],
                "epoch": None if parts[1] == "(none)" else parts[1],
                "version": parts[2],
                "release": parts[3],
                "arch": parts[4],
            }
    except Exception:
        pass
    name = rpm_path.name
    if name.endswith(".rpm"):
        name = name[:-4]
    return {"filename": rpm_path.name}


def generate_manifest(repo_dir, output_path=None):
    """Generate SHA256 manifest and package inventory for all RPMs in a repository directory."""
    path = Path(repo_dir)
    if not path.is_dir():
        raise NotADirectoryError(f"Repository directory not found: {repo_dir}")

    rpms = sorted(path.glob("*.rpm"))
    manifest = {
        "total_packages": len(rpms),
        "packages": {},
        "sha256": {},
    }

    for rpm_path in rpms:
        h = hashlib.sha256()
        with open(rpm_path, "rb") as f:
            while chunk := f.read(65536):
                h.update(chunk)
        digest = h.hexdigest()
        manifest["sha256"][rpm_path.name] = digest

        nevra = query_rpm_nevra(rpm_path)
        manifest["packages"][rpm_path.name] = {
            "sha256": digest,
            "size": rpm_path.stat().st_size,
            "nevra": nevra,
        }

    if output_path:
        out = Path(output_path)
        out.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

    return manifest


def create_repo(repo_dir, createrepo_bin="createrepo_c"):
    """Execute createrepo_c to build repodata with SHA256 checksums."""
    path = Path(repo_dir)
    if not path.is_dir():
        raise NotADirectoryError(f"Directory not found: {repo_dir}")
    cmd = [createrepo_bin, "--checksum=sha256", str(path)]
    subprocess.check_call(cmd)


def verify_offline_closure(repo_dir, packages, installroot=None, dnf_bin="dnf"):
    """Verify that packages can be satisfied entirely from the local repo with networking disabled."""
    repo_path = Path(repo_dir).resolve()
    repomd = repo_path / "repodata" / "repomd.xml"
    if not repomd.is_file():
        raise FileNotFoundError(f"Missing repodata/repomd.xml in {repo_dir}")

    cleanup_root = False
    if not installroot:
        temp_dir = tempfile.mkdtemp(prefix="ryoku-offline-verify-")
        installroot = temp_dir
        cleanup_root = True

    try:
        cmd = [
            dnf_bin,
            f"--installroot={installroot}",
            "--disablerepo=*",
            f"--repofrompath=offline,file://{repo_path}",
            "--setopt=install_weak_deps=False",
            "--setopt=localpkg_gpgcheck=0",
            "--assumeno",
            "install",
        ] + packages

        result = subprocess.run(cmd, capture_output=True, text=True)
        combined = result.stdout + "\n" + result.stderr

        # Check for dependency resolution failures
        failure_indicators = [
            "Failed to resolve",
            "Auflösen der Transaktion fehlgeschlagen",
            "Error: Problems in request",
            "No match for argument",
            "conflicting requests",
            "widersprüchliche Anforderungen",
        ]
        for indicator in failure_indicators:
            if indicator in combined:
                raise RuntimeError(f"Offline dependency closure failed with '{indicator}':\n{combined}")

        success_indicators = [
            "Operation aborted",
            "Operation aborted by the user",
            "Operation aborted by user",
            "Vorgang vom Benutzer abgebrochen",
            "Nothing to do",
            "Nichts zu tun",
        ]
        has_success = any(ind in combined for ind in success_indicators) or result.returncode == 0
        if not has_success:
            raise RuntimeError(f"Offline verification failed (code {result.returncode}):\n{combined}")

        return True
    finally:
        if cleanup_root and Path(installroot).exists():
            shutil.rmtree(installroot, ignore_errors=True)


def parse_args():
    script_dir = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description="Fedora 44 offline RPM repository builder and verifier")
    parser.add_argument("--packages", default=str(script_dir / "packages.list"), help="Path to packages.list")
    parser.add_argument("--keys-dir", default=str(script_dir / "keys"), help="Path to directory with pinned GPG keys")
    parser.add_argument("--dest", default="/tmp/ryoku-offline-repo", help="Target repository directory")
    parser.add_argument("--manifest", default=None, help="Path to output manifest.json")
    parser.add_argument("--verify-keys", action="store_true", help="Verify pinned GPG keys")
    parser.add_argument("--verify-closure", action="store_true", help="Verify offline repository closure")
    parser.add_argument("--create-repo", action="store_true", help="Run createrepo_c on destination directory")
    return parser.parse_args()


def main():
    args = parse_args()
    if args.verify_keys:
        print("Verifying pinned GPG keys...")
        verified = verify_all_pinned_keys(args.keys_dir)
        for key, fp in verified.items():
            print(f"  OK: {key} ({fp})")

    packages = read_packages_list(args.packages)
    print(f"Loaded {len(packages)} packages from {args.packages}")

    if args.create_repo:
        print(f"Creating repository metadata in {args.dest}...")
        create_repo(args.dest)

    if args.manifest:
        print(f"Generating manifest at {args.manifest}...")
        generate_manifest(args.dest, args.manifest)

    if args.verify_closure:
        print(f"Verifying offline closure against {args.dest}...")
        verify_offline_closure(args.dest, packages)
        print("Offline repository closure verified successfully.")


if __name__ == "__main__":
    main()
