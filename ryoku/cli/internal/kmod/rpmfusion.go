package kmod

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/host"
)

// rpmFusionFingerprints pin the keys RPM Fusion signs its Fedora packages
// with ("RPM Fusion free/nonfree repository for Fedora (2020)", used for every
// release since Fedora 33). A release package carrying any other key is
// refused; when RPM Fusion rotates its keys, these change with it.
var rpmFusionFingerprints = map[string]string{
	"free":    "E9A491A3DE247814E7E067EAE06F8ECDD651FF2E",
	"nonfree": "79BDB88F9BBF73910FD4095B6A2AF96194843C65",
}

// rpmFusionRepos are the only two repositories Ryoku enables. The tainted
// repositories are never enabled, and no package is version-locked.
var rpmFusionRepos = []string{"free", "nonfree"}

// enableRPMFusion installs RPM Fusion's free and nonfree release packages for
// this Fedora release. Each package carries its repository's key: the key must
// match the pin before it is imported, and the package must then verify
// against it before dnf installs it.
func (m *Machine) enableRPMFusion(release string) error {
	names := []string{"-q"}
	for _, repo := range rpmFusionRepos {
		names = append(names, "rpmfusion-"+repo+"-release")
	}
	if m.cfg.Runner.Run(host.Command{Name: "rpm", Args: names}).Code == 0 {
		return nil
	}
	work, err := os.MkdirTemp("", "ryoku-rpmfusion-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	packages := make([]string, 0, len(rpmFusionRepos))
	keys := make([]string, 0, len(rpmFusionRepos))
	for _, repo := range rpmFusionRepos {
		pkg, key, err := m.fetchRPMFusionRelease(work, repo, release)
		if err != nil {
			return err
		}
		packages, keys = append(packages, pkg), append(keys, key)
	}
	for _, key := range keys {
		if err := m.quiet("rpm", "--import", key); err != nil {
			return fmt.Errorf("import RPM Fusion's key: %w", err)
		}
	}
	for _, pkg := range packages {
		if err := m.quiet("rpm", "-K", pkg); err != nil {
			return fmt.Errorf("%s is not signed by RPM Fusion's pinned key: %w", filepath.Base(pkg), err)
		}
	}
	return m.dnf(append([]string{"install", "-y"}, packages...)...)
}

// fetchRPMFusionRelease downloads one release package, unpacks its key, and
// checks the key against the pin.
func (m *Machine) fetchRPMFusionRelease(work, repo, release string) (pkg, key string, err error) {
	name := fmt.Sprintf("rpmfusion-%s-release-%s.noarch.rpm", repo, release)
	pkg = filepath.Join(work, name)
	url := fmt.Sprintf("https://mirrors.rpmfusion.org/%s/fedora/%s", repo, name)
	if err := m.quiet("curl", "--fail", "--silent", "--show-error", "--location", "--retry", "3",
		"--proto", "=https", "--proto-redir", "=https", "--output", pkg, url); err != nil {
		return "", "", fmt.Errorf("download %s: %w", url, err)
	}
	tree := filepath.Join(work, repo)
	if err := os.Mkdir(tree, 0o700); err != nil {
		return "", "", err
	}
	if err := m.unpack(pkg, tree); err != nil {
		return "", "", fmt.Errorf("unpack %s: %w", name, err)
	}
	keyDir := filepath.Join(tree, "etc", "pki", "rpm-gpg")
	key, err = keyInside(keyDir, fmt.Sprintf("RPM-GPG-KEY-rpmfusion-%s-fedora-%s", repo, release))
	if err != nil {
		return "", "", fmt.Errorf("%s carries no key for Fedora %s: %w", name, release, err)
	}
	fingerprint, err := m.primaryFingerprint(work, key)
	if err != nil {
		return "", "", err
	}
	if fingerprint != rpmFusionFingerprints[repo] {
		return "", "", fmt.Errorf("RPM Fusion %s key fingerprint is %s; expected %s", repo, fingerprint, rpmFusionFingerprints[repo])
	}
	return pkg, key, nil
}

// unpack extracts a package's files without a shell: rpm2cpio's archive goes
// through a file into cpio.
func (m *Machine) unpack(pkg, tree string) error {
	archive, err := os.Create(tree + ".cpio")
	if err != nil {
		return err
	}
	defer archive.Close()
	if result := m.cfg.Runner.Run(host.Command{Name: "rpm2cpio", Args: []string{pkg}, Stdout: archive, Stderr: m.cfg.Log}); result.Code != 0 {
		return fmt.Errorf("rpm2cpio exited %d", result.Code)
	}
	if _, err := archive.Seek(0, 0); err != nil {
		return err
	}
	result := m.cfg.Runner.Run(host.Command{
		Name:  "cpio",
		Args:  []string{"-idm", "--quiet", "--no-absolute-filenames", "-D", tree},
		Stdin: archive, Stdout: m.cfg.Log, Stderr: m.cfg.Log,
	})
	if result.Code != 0 {
		return fmt.Errorf("cpio exited %d", result.Code)
	}
	return nil
}

// keyInside resolves name in dir, following a link only to a sibling, so a
// package cannot point the check at a file outside what it carries.
func keyInside(dir, name string) (string, error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if target != filepath.Base(target) {
			return "", fmt.Errorf("%s links outside its directory", name)
		}
		path = filepath.Join(dir, target)
		if info, err = os.Lstat(path); err != nil {
			return "", err
		}
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file", name)
	}
	return path, nil
}

// primaryFingerprint reads the one primary key in an armored key file.
func (m *Machine) primaryFingerprint(work, key string) (string, error) {
	home := filepath.Join(work, "gnupg")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	result := m.cfg.Runner.Run(host.Command{Name: "gpg", Args: []string{
		"--batch", "--homedir", home, "--with-colons", "--show-keys", "--fingerprint", key,
	}})
	if result.Code != 0 {
		return "", fmt.Errorf("gpg cannot read %s", filepath.Base(key))
	}
	var fingerprints []string
	primary := false
	scanner := bufio.NewScanner(strings.NewReader(result.Output))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		switch {
		case fields[0] == "pub":
			primary = true
		case fields[0] == "fpr" && primary && len(fields) > 9:
			fingerprints = append(fingerprints, strings.ToUpper(fields[9]))
			primary = false
		case fields[0] == "sub":
			primary = false
		}
	}
	if len(fingerprints) != 1 {
		return "", fmt.Errorf("%s holds %d primary keys; expected exactly one", filepath.Base(key), len(fingerprints))
	}
	return fingerprints[0], nil
}
