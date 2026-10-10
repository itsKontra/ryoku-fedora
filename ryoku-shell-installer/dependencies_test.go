package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakePayload writes the specs fedoraPackages reads: an umbrella requiring
// desktop, two first-party specs (ryoku-shell requiring uv), and one spec per
// compositor requiring its own package.
func fakePayload(t *testing.T, desktop []string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	requires := func(pkgs []string) string {
		var b strings.Builder
		for _, p := range pkgs {
			b.WriteString("Requires:       " + p + "\n")
		}
		return b.String()
	}
	write("release/rpm/ryoku-desktop.spec", "Name:           ryoku-desktop\n"+requires(desktop))
	write("release/rpm/ryoku-shell.spec", "Name:           ryoku-shell\nRequires:       uv\n")
	write("release/rpm/ryoku-extras.spec", "Name:           ryoku-extras\n")
	for _, provider := range compositors() {
		write("release/rpm/ryoku-desktop-"+provider+".spec",
			"Name:           ryoku-desktop-"+provider+"\n"+
				"Requires:       ryoku-desktop = %{version}-%{release}\n"+
				"Requires:       "+provider+"\n"+
				"Provides:       ryoku-desktop-compositor\n")
	}
	return dir
}

func TestFedoraPackagesFromSpecs(t *testing.T) {
	dir := fakePayload(t,
		[]string{"ryoku-shell = %{version}-%{release}", "ryoku-desktop-compositor", "quickshell", "kitty >= 0.40"})
	got, err := fedoraPackages(dir, "hyprland")
	if err != nil {
		t.Fatal(err)
	}
	if want := "uv quickshell kitty hyprland"; strings.Join(got, " ") != want {
		t.Fatalf("fedoraPackages = %q, want %q", strings.Join(got, " "), want)
	}
	if _, err := fedoraPackages(dir, "sway"); err == nil {
		t.Fatal("an unknown compositor must fail, not install a partial set")
	}
}

// The shipped payload: the chosen provider's packages are in, the other's are
// out, a first-party package's own needs (ryogami's inotify-tools) are in,
// nothing first-party leaks into a dnf transaction, the ISO's own applications
// stay out, and the boot chain the box already owns is never touched.
func TestFedoraSourceDependenciesFromRepo(t *testing.T) {
	payload, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for provider, other := range map[string]string{"hyprland": "niri", "niri": "hyprland"} {
		e := &engine{f: &facts{distro: fedoraLinux}, p: &plan{compositor: provider}, payload: payload}
		pkgs, err := e.sourceDependencies()
		if err != nil {
			t.Fatal(err)
		}
		got := " " + strings.Join(pkgs, " ") + " "
		for _, want := range []string{provider, "quickshell", "qt6-qtwayland", "polkit", "golang", "inotify-tools"} {
			if !strings.Contains(got, " "+want+" ") {
				t.Errorf("%s: missing %s", provider, want)
			}
		}
		for _, bad := range []string{other, "ryoku-desktop", "ryoku-extras", "ryogami", "ryoku-desktop-compositor", "glibc", "kernel", "grub2-efi-x64", "rpmfusion-free-release",
			"moby-engine", "firewalld", "chromium", "linux-firmware",
			"dracut", "grubby", "grub2-tools", "btrfs-progs", "snapper"} {
			if strings.Contains(got, " "+bad+" ") {
				t.Errorf("%s: must not install %s", provider, bad)
			}
		}
	}
}

// snapper is the one boot-chain package a converted box takes, and only on a
// btrfs root, where ryoku doctor configures snapshots.
func TestSourceDependenciesSnapperOnBtrfs(t *testing.T) {
	dir := fakePayload(t, []string{"dracut", "grubby", "snapper", "quickshell"})
	for _, btrfs := range []bool{false, true} {
		e := &engine{f: &facts{distro: fedoraLinux, btrfsRoot: btrfs}, p: &plan{compositor: compositors()[0]}, payload: dir}
		pkgs, err := e.sourceDependencies()
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(pkgs, "dracut") || slices.Contains(pkgs, "grubby") {
			t.Errorf("btrfs=%v: boot chain leaked into %v", btrfs, pkgs)
		}
		if slices.Contains(pkgs, "snapper") != btrfs {
			t.Errorf("btrfs=%v: snapper presence wrong in %v", btrfs, pkgs)
		}
		if !slices.Contains(pkgs, "quickshell") {
			t.Errorf("btrfs=%v: missing quickshell", btrfs)
		}
	}
}

func TestReadSpecTags(t *testing.T) {
	write := func(body string) string {
		path := filepath.Join(t.TempDir(), "x.spec")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tags, err := readSpecTags(write("Name:           ryoku-x\n" +
		"Requires:       a, b >= 1.2 c\n" +
		"Requires:       %{name}-common = %{version}-%{release}\n" +
		"Requires(post): d\n" +
		"BuildRequires:  (e or f)\n" +
		"%if 0%{?fedora}\n%global x 1\n%endif\n" +
		"%description\nnot a Requires: tag\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(tags["Requires"], " "), "a b c ryoku-x-common d"; got != want {
		t.Fatalf("Requires = %q, want %q", got, want)
	}
	for name, body := range map[string]string{
		"rich dependency":     "Requires: (pipewire-pulseaudio or pulseaudio)\n",
		"unknown macro":       "Requires: %{python3_pkgversion}-foo\n",
		"conditional require": "%if 0%{?fedora} >= 45\nRequires: foo\n%endif\n",
		"sub-package":         "Name: x\n%package common\nRequires: foo\n",
	} {
		if _, err := readSpecTags(write(body)); err == nil {
			t.Errorf("%s: must fail, not install a guessed set", name)
		}
	}
}
