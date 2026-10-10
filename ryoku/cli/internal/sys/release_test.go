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
