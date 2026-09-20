package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectDistro(t *testing.T) {
	for _, c := range []struct {
		id, like, want string
	}{
		{"arch", "", "arch"},
		{"cachyos", "arch", "arch"},
		{"endeavouros", "arch", "arch"},
		{"debian", "", "debian"},
		{"ubuntu", "debian", "debian"},
		{"linuxmint", "ubuntu debian", "debian"},
		{"fedora", "", "fedora"},
		{"nobara", "fedora", "fedora"},
		{"bazzite", "fedora", "fedora"},
		{"void", "", ""},
	} {
		d := detectDistro(c.id, c.like)
		got := ""
		if d != nil {
			got = d.id
		}
		if got != c.want {
			t.Errorf("detectDistro(%q,%q) = %q, want %q", c.id, c.like, got, c.want)
		}
	}
}

func TestLocalAllRenamesAndDrops(t *testing.T) {
	in := []string{"git", "networkmanager", "fd", "matugen", "limine", "kitty"}
	got := debianLinux.localAll(in)
	want := []string{"git", "network-manager", "fd-find", "kitty"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("localAll (debian) = %v, want %v", got, want)
	}
	gotFedora := fedoraLinux.localAll(in)
	wantFedora := []string{"git", "NetworkManager", "fd-find", "kitty"}
	if strings.Join(gotFedora, ",") != strings.Join(wantFedora, ",") {
		t.Fatalf("localAll (fedora) = %v, want %v", gotFedora, wantFedora)
	}
	if archLinux.local("networkmanager") != "networkmanager" {
		t.Error("arch must pass base.packages names through unchanged")
	}
}

// The Arch step list is the contract that must not drift; the Debian and Fedora
// ones swap the pacman-only steps for the source build.
func TestStepsPerDistro(t *testing.T) {
	ids := func(f *facts) []string {
		e := newEngine(f, &plan{}, true, "", "")
		var out []string
		for _, s := range e.steps {
			out = append(out, s.id)
		}
		return out
	}

	arch := strings.Join(ids(&facts{distro: archLinux}), " ")
	wantArch := "legacy sysupgrade tools payload backup repo conflicts packages drivers session configs aur shell doctor verify"
	if arch != wantArch {
		t.Errorf("arch steps = %q, want %q", arch, wantArch)
	}

	deb := strings.Join(ids(&facts{distro: debianLinux}), " ")
	wantDeb := "sysupgrade tools payload backup conflicts packages build session configs shell doctor verify"
	if deb != wantDeb {
		t.Errorf("debian steps = %q, want %q", deb, wantDeb)
	}

	fed := strings.Join(ids(&facts{distro: fedoraLinux}), " ")
	wantFed := "sysupgrade tools payload backup repo conflicts packages package-migration session configs shell doctor verify"
	if fed != wantFed {
		t.Errorf("fedora steps = %q, want %q", fed, wantFed)
	}
}

func TestInstallArgs(t *testing.T) {
	got := strings.Join(archLinux.installArgs([]string{"git"}), " ")
	if got != "pacman -Syu --needed --noconfirm git" {
		t.Errorf("arch installArgs = %q", got)
	}
	got = strings.Join(debianLinux.installArgs([]string{"git"}), " ")
	if got != "apt-get -y install git" {
		t.Errorf("debian installArgs = %q", got)
	}
	got = strings.Join(debianLinux.removeArgs([]string{"dunst"}), " ")
	if got != "apt-get -y remove dunst" {
		t.Errorf("debian removeArgs = %q", got)
	}
	got = strings.Join(fedoraLinux.installArgs([]string{"git"}), " ")
	if got != "dnf -y install --best git" {
		t.Errorf("fedora installArgs = %q", got)
	}
	got = strings.Join(fedoraLinux.removeArgs([]string{"dunst"}), " ")
	if got != "dnf -y remove dunst" {
		t.Errorf("fedora removeArgs = %q", got)
	}
	if got := fedoraLinux.installArgs(nil); len(got) != 0 {
		t.Errorf("fedora installArgs(nil) = %v, want empty/nil", got)
	}
	if got := fedoraLinux.removeArgs([]string{}); len(got) != 0 {
		t.Errorf("fedora removeArgs([]) = %v, want empty/nil", got)
	}
}

// desktopPacmanArgs must --overwrite the ryoku-desktop-owned paths a prior partial
// install, a dev deploy, or the ISO installer can leave unowned (the bin helpers,
// their polkit rules, and the Plymouth splash theme) so a resume or conversion
// never aborts on "exists in filesystem". Dropping any path silently reintroduces
// that outage, so pin coverage here. fromSource distros build from the payload and
// must never carry --overwrite.
func TestDesktopPacmanArgsAdoptsRyokuPaths(t *testing.T) {
	args := desktopPacmanArgs(archLinux, []string{"ryoku-desktop"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--overwrite") {
		t.Fatalf("arch desktop install missing --overwrite: %v", args)
	}
	var glob string
	for i, a := range args {
		if a == "--overwrite" && i+1 < len(args) {
			glob = args[i+1]
		}
	}
	for _, p := range []string{
		"/usr/bin/ryoku-dns",
		"/usr/share/polkit-1/rules.d/50-ryoku-dns.rules",
		"/usr/share/plymouth/themes/ryoku/bullet.png",
	} {
		covered := false
		for _, g := range strings.Split(glob, ",") {
			if ok, _ := filepath.Match(g, p); ok {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("--overwrite %q does not cover seeded path %q", glob, p)
		}
	}
	if strings.Contains(strings.Join(desktopPacmanArgs(debianLinux, []string{"foo"}), " "), "--overwrite") {
		t.Error("fromSource distro must not carry --overwrite")
	}
}

func TestImmutableFedoraRejected(t *testing.T) {
	for _, id := range []string{"silverblue", "kinoite", "coreos", "sway-atomic", "bazzite", "bluefin"} {
		if !immutableFedora("ID=fedora\nVARIANT_ID=\"" + id + "\"\n") {
			t.Errorf("accepted %s", id)
		}
	}
	if immutableFedora("ID=fedora\nVARIANT_ID=workstation\n") {
		t.Fatal("rejected mutable Fedora")
	}
}

func TestFedoraSourceDependenciesIncludeChosenProvider(t *testing.T) {
	for _, provider := range []string{"hyprland", "niri"} {
		e := &engine{f: &facts{distro: fedoraLinux}, p: &plan{compositor: provider}}
		packages, err := e.sourceDependencies([]string{"polkit", "qt6-wayland"})
		if err != nil {
			t.Fatal(err)
		}
		got := " " + strings.Join(packages, " ") + " "
		if !strings.Contains(got, " "+provider+" ") || !strings.Contains(got, " qt6-qtwayland ") || !strings.Contains(got, " polkit ") {
			t.Fatalf("missing dependencies: %s", got)
		}
	}
}

func TestFedoraToolsFollowDNFSymlink(t *testing.T) {
	for _, implementation := range []string{"dnf4", "dnf5"} {
		t.Run(implementation, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, implementation), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(implementation, filepath.Join(bin, "dnf")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			e := &engine{f: &facts{distro: fedoraLinux}, dry: true, events: make(chan any, 16)}
			if err := stepTools(e); err != nil {
				t.Fatal(err)
			}
			close(e.events)
			var output string
			for event := range e.events {
				if line, ok := event.(evLine); ok {
					output += line.line + "\n"
				}
			}
			plugin := "dnf-plugins-core"
			if implementation == "dnf5" {
				plugin = "dnf5-plugins"
			}
			if !strings.Contains(output, "dnf -y install --best") || !strings.Contains(output, plugin) {
				t.Fatalf("wrong command or COPR plugin: %s", output)
			}
		})
	}
}
