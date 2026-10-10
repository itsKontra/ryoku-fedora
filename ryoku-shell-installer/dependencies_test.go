package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePayload writes the specs and packages.list fedoraPackages reads: an
// umbrella requiring desktop, two first-party specs (ryoku-shell requiring uv),
// one spec per compositor
// requiring its own package, and a packages.list with list as its [desktop]
// section.
func fakePayload(t *testing.T, desktop, list []string) string {
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
	write("installation/fedora/packages.list", "[base]\nglibc\n\n[desktop]\n"+strings.Join(list, "\n")+"\n")
	return dir
}

func TestFedoraPackagesFromSpecsAndList(t *testing.T) {
	dir := fakePayload(t,
		[]string{"ryoku-shell = %{version}-%{release}", "ryoku-desktop-compositor", "quickshell", "kitty >= 0.40"},
		[]string{"# a comment", "ryoku-extras", "nautilus", "kitty", "hyprland", "niri", "rpmfusion-free-release"})
	got, err := fedoraPackages(dir, "hyprland")
	if err != nil {
		t.Fatal(err)
	}
	if want := "uv quickshell kitty hyprland nautilus"; strings.Join(got, " ") != want {
		t.Fatalf("fedoraPackages = %q, want %q", strings.Join(got, " "), want)
	}
	if _, err := fedoraPackages(dir, "sway"); err == nil {
		t.Fatal("an unknown compositor must fail, not install a partial set")
	}
}

// The shipped payload: the chosen provider's packages are in, the other's are
// out, a first-party package's own needs (ryogami's inotify-tools) are in,
// nothing first-party leaks into a dnf transaction, and the boot chain the
// box already owns is never touched.
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
		for _, want := range []string{provider, "quickshell", "qt6-qtwayland", "polkit", "golang", "nautilus", "inotify-tools"} {
			if !strings.Contains(got, " "+want+" ") {
				t.Errorf("%s: missing %s", provider, want)
			}
		}
		for _, bad := range []string{other, "ryoku-desktop", "ryoku-extras", "ryogami", "ryoku-desktop-compositor", "glibc", "kernel", "grub2-efi-x64", "rpmfusion-free-release"} {
			if strings.Contains(got, " "+bad+" ") {
				t.Errorf("%s: must not install %s", provider, bad)
			}
		}
	}
}
