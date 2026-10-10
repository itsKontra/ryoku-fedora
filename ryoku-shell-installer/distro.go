package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// distro is the only place the installer knows a package manager. Every step
// asks it for argv. Ryoku ships for Fedora only, so fedoraLinux is the one
// table; anything else is refused before the engine starts.
type distro struct {
	id   string
	name string

	// build are the extra packages a source install needs to compile the Go
	// programs, the Ryoku.Blobs QML plugin, and the Hyprland plugins.
	build []string

	installCmd []string
	removeCmd  []string
	updateCmd  []string
	refreshCmd []string
	queryCmd   []string
}

var fedoraLinux = &distro{
	id:         "fedora",
	name:       "Fedora",
	installCmd: []string{"dnf", "-y", "install", "--best"},
	removeCmd:  []string{"dnf", "-y", "remove"},
	updateCmd:  []string{"dnf", "-y", "upgrade"},
	refreshCmd: []string{"dnf", "makecache"},
	queryCmd:   []string{"rpm", "-q", "--quiet"},
	build: []string{
		"gcc", "gcc-c++", "cmake", "ninja-build", "pkgconf-pkg-config", "golang",
		"qt6-qtbase-devel", "qt6-qtdeclarative-devel", "qt6-qtmultimedia-devel",
		"qt6-qtshadertools-devel", "qt6-qtsvg-devel", "qt6-qt5compat-devel", "qt6-qtwayland-devel",
		"wayland-devel", "wayland-protocols-devel", "ffmpeg-free-devel", "libsecret", "gnome-keyring-pam",
	},
}

// activeDistro is set once by detectFacts; installed() reads it from the
// detection paths that have no engine to hand.
var activeDistro = fedoraLinux

func detectDistro(id, like string) *distro {
	if id == "fedora" || strings.Contains(like, "fedora") {
		return fedoraLinux
	}
	return nil
}

func (d *distro) installArgs(pkgs []string) []string {
	if len(pkgs) == 0 {
		return nil
	}
	return append(append([]string{}, d.installCmd...), pkgs...)
}

func (d *distro) removeArgs(pkgs []string) []string {
	if len(pkgs) == 0 {
		return nil
	}
	return append(append([]string{}, d.removeCmd...), pkgs...)
}

// removeKeepingDepsArgs takes the named packages out without autoremoving
// what they pulled in: niri under a rival shell meta must survive as a
// fallback session.
func (d *distro) removeKeepingDepsArgs(pkgs []string) []string {
	if len(pkgs) == 0 {
		return nil
	}
	return d.removeArgs(append([]string{"--no-autoremove"}, pkgs...))
}

func (d *distro) installedPkg(pkg string) bool {
	args := append(append([]string{}, d.queryCmd[1:]...), pkg)
	return exec.Command(d.queryCmd[0], args...).Run() == nil
}

// installed queries the detected distro.
func installed(pkg string) bool { return activeDistro.installedPkg(pkg) }

// d is the engine's detected distro.
func (e *engine) d() *distro {
	if e.f != nil && e.f.distro != nil {
		return e.f.distro
	}
	return activeDistro
}

// ryokuBin finds the ryoku CLI: /usr/bin from a package, ~/.local/bin from a
// source build. Empty when it is not installed yet.
func (e *engine) ryokuBin() string {
	cands := []string{"/usr/bin/ryoku"}
	if e.f != nil && e.f.homeDir != "" {
		if e.fromSource() {
			cands = append([]string{filepath.Join(e.f.homeDir, ".local", "bin", "ryoku")}, cands...)
		}
	}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// detectHostDistro resolves the distro from /etc/os-release and latches it, so
// the preflight gate and the later detection pass agree.
func detectHostDistro() *distro {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil
	}
	id, like, _ := parseOSRelease(string(b))
	d := detectDistro(id, like)
	if d == nil || immutableFedora(string(b)) || pathExists("/run/ostree-booted") {
		return nil
	}
	activeDistro = d
	return d
}

func immutableFedora(release string) bool {
	for _, line := range strings.Split(release, "\n") {
		key, value, _ := strings.Cut(line, "=")
		value = strings.Trim(value, "\"'")
		if key == "VARIANT_ID" || key == "ID" {
			switch value {
			case "silverblue", "kinoite", "sericea", "onyx", "sway-atomic", "budgie-atomic", "coreos", "bazzite", "bluefin", "aurora":
				return true
			}
		}
	}
	return false
}
