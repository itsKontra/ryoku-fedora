package main

import (
	"os/exec"
	"testing"
)

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
	dir := fakePayload(t, nil, nil)
	f := &facts{distro: &distro{id: "fedora", name: "Fedora", installCmd: []string{"fail-if-called"}, queryCmd: []string{"true"}}}
	e := &engine{f: f, p: &plan{compositor: "hyprland"}, payload: dir, payloadOverride: dir, dry: false}
	if err := stepPackages(e); err != nil {
		t.Errorf("stepPackages when all satisfied should return nil without running installCmd, got %v", err)
	}
}
