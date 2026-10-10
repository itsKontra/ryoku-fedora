package main

import (
	"os/exec"
	"slices"
	"strings"
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

// Every package's answer comes from one rpm listing, and the alternatives a box
// may already carry stand in for the package they replace.
func TestDropSatisfiedAlternatives(t *testing.T) {
	stubCapabilities(t, "kitty", "tuned-ppd", "ffmpeg-free")
	e := &engine{f: &facts{distro: fedoraLinux}}
	got := e.dropSatisfied([]string{"kitty", "power-profiles-daemon", "ffmpeg", "", "quickshell"})
	if strings.Join(got, " ") != "quickshell" {
		t.Fatalf("dropSatisfied = %v, want [quickshell]", got)
	}
}

func stubCapabilities(t *testing.T, caps ...string) {
	t.Helper()
	orig := installedCapabilities
	t.Cleanup(func() { installedCapabilities = orig })
	installedCapabilities = func() (func(string) bool, error) {
		return func(c string) bool { return caps == nil || slices.Contains(caps, c) }, nil
	}
}

func TestStepPackagesAllSatisfiedNoop(t *testing.T) {
	stubCapabilities(t)
	dir := fakePayload(t, nil)
	f := &facts{distro: &distro{id: "fedora", name: "Fedora", installCmd: []string{"fail-if-called"}, queryCmd: []string{"true"}}}
	e := &engine{f: f, p: &plan{compositor: "hyprland"}, payload: dir, payloadOverride: dir, dry: false}
	if err := stepPackages(e); err != nil {
		t.Errorf("stepPackages when all satisfied should return nil without running installCmd, got %v", err)
	}
}
