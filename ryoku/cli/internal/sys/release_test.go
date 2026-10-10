package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadReleaseParsesTheMarker(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ryoku-release")
	os.WriteFile(f, []byte("RELEASE=v0.55.7-beta.19\nNAME=Onogoro\nCHANNEL=stable\nVERSION=0.55.7.r3280.g097f522\nCOMMIT=097f522b5\nDATE=2026-09-03T20:00:00Z\n"), 0o644)
	old := ReleaseFile
	ReleaseFile = f
	defer func() { ReleaseFile = old }()
	r := ReadRelease()
	if r.Release != "v0.55.7-beta.19" || r.Name != "Onogoro" || r.Channel != "stable" || r.Version != "0.55.7.r3280.g097f522" || r.Commit != "097f522b5" {
		t.Fatalf("release = %+v", r)
	}
	ReleaseFile = filepath.Join(dir, "missing")
	if r := ReadRelease(); r.Release != "" {
		t.Fatalf("missing file must read empty, got %+v", r)
	}
}

// A rolling COPR build is not a tagged release, so the boot guard skips it.
func TestIsReleaseTag(t *testing.T) {
	for s, want := range map[string]bool{
		"v0.73.0":          true,
		"v1.0.0-rc.1":      true,
		"v0.73.0-5-gabc12": false,
		"0.73.0":           false,
		"":                 false,
	} {
		if got := IsReleaseTag(s); got != want {
			t.Errorf("IsReleaseTag(%q) = %v, want %v", s, got, want)
		}
	}
}
