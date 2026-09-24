package main

import (
	"os"
	"strings"
	"testing"
)

// The caps list is documented as "kept in step with that package's depends".
// A satellite added to the PKGBUILD but not to this list could never be
// reclaimed on a packaged box; the variant package itself, missing from the
// list, owns every satellite and blocks all their removal. Both drifts fail
// here, against the real PKGBUILD.
func TestReclaimListMatchesVariantDepends(t *testing.T) {
	raw, err := os.ReadFile("../../../release/rpm/ryoku-desktop-niri.spec")
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
	if !have["ryoku-desktop-niri"] {
		t.Error("the variant package must be in its own reclaim list")
	}
	for p := range want {
		if !have[p] {
			t.Errorf("RPM spec requires %s but the reclaim list omits it", p)
		}
	}
}
