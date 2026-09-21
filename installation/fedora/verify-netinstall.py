#!/usr/bin/env python3
"""Resolve the ISO's exact package selection against its Kickstart repositories."""

import argparse
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile

from pykickstart.parser import KickstartParser
from pykickstart.version import makeVersion


def verify(kickstart, media_repo, packages_file):
    parser = KickstartParser(makeVersion("F44"))
    parser.readKickstart(str(kickstart))
    data = parser.handler
    spec = importlib.util.spec_from_file_location("build_repo", Path(__file__).with_name("build-repo.py"))
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    packages = builder.read_packages_list(packages_file)
    builder.create_repo(media_repo, packages=packages)
    with tempfile.TemporaryDirectory(prefix="ryoku-netinstall-") as work:
        work = Path(work)
        repos = work / "repos"
        repos.mkdir()
        entries = [("Fedora", data.url)]
        entries.extend((repo.name, repo) for repo in data.repo.repoList)
        config = []
        for name, repo in entries:
            config.extend([f"[{name}]", "enabled=1", "skip_if_unavailable=0"])
            if name == "RyokuMedia":
                config.append(f"baseurl={media_repo.resolve().as_uri()}")
            else:
                for option in ("baseurl", "metalink", "mirrorlist"):
                    value = getattr(repo, option, None)
                    if option == "baseurl" and name == "Fedora":
                        value = getattr(repo, "url", None)
                    if value:
                        config.append(f"{option}={value}")
            config.append("")
        (repos / "installer.repo").write_text("\n".join(config))
        command = [
            "dnf5", f"--installroot={work / 'root'}", "--releasever=44",
            f"--setopt=reposdir={repos}", "--setopt=install_weak_deps=True",
            "--setopt=optional_metadata_types=filelists", "--assumeno",
        ]
        if data.packages.excludedList:
            command.append("--exclude=" + ",".join(data.packages.excludedList))
        # Explicit names make missing mandatory comps entries fatal as well.
        command.extend(["install", "@^" + data.packages.environment, *packages])
        result = subprocess.run(command, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, env={**os.environ, "LC_ALL": "C"})
        print(result.stdout, end="", flush=True)
        if result.returncode != 1 or "Operation aborted" not in result.stdout:
            raise RuntimeError(f"Installer transaction did not resolve (exit {result.returncode})")
        if "No match for" in result.stdout or "conflicting requests" in result.stdout:
            raise RuntimeError("Installer transaction has unavailable packages")
        print("Installer dependency resolution passed; no packages installed.")


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("--ks", type=Path, required=True)
    cli.add_argument("--repo-dir", type=Path, required=True)
    cli.add_argument("--packages", type=Path, default=Path(__file__).with_name("packages.list"))
    args = cli.parse_args()
    verify(args.ks, args.repo_dir, args.packages)
