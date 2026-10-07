package main

import (
	"os"
	"strings"
	"testing"

	wm "ryoku-wm"
)

// The caps list is the variant package plus that package's RPM Requires, minus
// ryoku-desktop. A satellite the spec requires but the list omits could never
// be reclaimed; an Arch name the spec does not require would make a Fedora
// switch ask dnf for a package it does not ship. Both drifts fail here.
func TestReclaimListMatchesVariantDepends(t *testing.T) {
	raw, err := os.ReadFile("../../../release/rpm/ryoku-desktop-hyprland.spec")
	if err != nil {
		t.Skip("no RPM spec beside the test")
	}
	depends := strings.Split(string(raw), "\n")
	want := map[string]bool{}
	for _, line := range depends {
		if !strings.HasPrefix(line, "Requires:") {
			continue
		}
		pkg := strings.Fields(strings.TrimPrefix(line, "Requires:"))[0]
		if pkg == "" || pkg == "ryoku-desktop" {
			continue // the umbrella is shared with the incoming compositor
		}
		want[pkg] = true
	}
	have := map[string]bool{}
	for _, p := range compositorPackages {
		have[p] = true
	}
	if !have["ryoku-desktop-hyprland"] {
		t.Error("the variant package must be in its own reclaim list")
	}
	for p := range want {
		if !have[p] {
			t.Errorf("RPM spec requires %s but the reclaim list omits it", p)
		}
	}
	for p := range have {
		if p == "ryoku-desktop-hyprland" {
			continue
		}
		if !want[p] {
			t.Errorf("reclaim list names %s, which the RPM spec does not require", p)
		}
	}
}

// A capability the manifest claims but wm.All() omits never reaches a QML
// consumer: the caps frame carries one boolean per All() entry, so the shell
// reads the gate as false and the affordance silently disappears on Hyprland.
func TestManifestIsDeliverable(t *testing.T) {
	all := map[wm.Capability]bool{}
	for _, c := range wm.All() {
		all[c] = true
	}
	for _, c := range capsManifest {
		if !all[c] {
			t.Errorf("manifest claims %q but wm.All() omits it, so it never rides the caps frame", c)
		}
	}
}

// The launcher's frost backdrop is gated on this capability, so Hyprland must
// claim it or every Hyprland user loses the blurred launcher.
func TestClaimsPersistentScreenCapture(t *testing.T) {
	c := wm.Caps{Supports: capsManifest}
	if !c.Has(wm.CapPersistentScreenCapture) {
		t.Error("Hyprland hosts long-lived capturing layer surfaces; the manifest must say so")
	}
}

// The Stage Editor's canvas holds the keyboard exclusively wherever this is
// claimed. Hyprland then hands it every click as well, and the editor's toolbar
// and catalogue stop answering, so the manifest must keep denying it.
func TestDeniesKeyboardGrabSharesPointer(t *testing.T) {
	c := wm.Caps{Supports: capsManifest}
	if c.Has(wm.CapKeyboardGrabSharesPointer) {
		t.Error("Hyprland gives an exclusive keyboard grab the pointer too; the manifest must not claim it shares")
	}
}
