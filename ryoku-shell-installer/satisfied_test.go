package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The qt6ct-kde shape: an installed provider must drop the shipped name from
// the install set instead of the run dying on the conflict prompt.
func TestDropSatisfiedKeepsOnlyUnmet(t *testing.T) {
	e := &engine{}
	unmetOut := "qt6ct\nryoku-desktop\n"
	got := filterByUnmet([]string{"git", "qt6ct", "ryoku-desktop"}, unmetOut)
	want := []string{"qt6ct", "ryoku-desktop"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("filter = %v, want %v", got, want)
	}
	_ = e
}

func TestDropSatisfiedEmptyUnmetDropsAll(t *testing.T) {
	if got := filterByUnmet([]string{"git", "curl"}, ""); len(got) != 0 {
		t.Errorf("everything satisfied should drop all, got %v", got)
	}
}

func TestDropSatisfiedFedora(t *testing.T) {
	if exec.Command("rpm", "-q", "--quiet", "git").Run() != nil {
		t.Skip("git is not installed via rpm; skipping Fedora dropSatisfied test")
	}
	f := &facts{distro: fedoraLinux}
	e := &engine{f: f, dry: false}
	pkgs := []string{"git", "nonexistent-pkg-test-xyz"}
	res := e.dropSatisfied(pkgs)
	for _, p := range res {
		if p == "git" {
			t.Errorf("git is installed and should have been dropped on Fedora, got %v", res)
		}
	}
	foundNonexistent := false
	for _, p := range res {
		if p == "nonexistent-pkg-test-xyz" {
			foundNonexistent = true
		}
	}
	if !foundNonexistent {
		t.Errorf("uninstalled package should be kept, got %v", res)
	}
}

func TestStepPackagesAllSatisfiedNoop(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "system", "packages")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "base.packages"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	customDistro := &distro{id: "fedora", name: "Fedora", fromSource: true, build: nil, installCmd: []string{"fail-if-called"}}
	f := &facts{distro: customDistro}
	e := &engine{f: f, p: &plan{}, payload: dir, dry: false}
	if err := stepPackages(e); err != nil {
		t.Errorf("stepPackages when all satisfied should return nil without running installCmd, got %v", err)
	}
}

