package main

import (
	"os"
	"strings"
	"testing"
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
