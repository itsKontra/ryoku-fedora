package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceOwnershipRestoresOnlyChangedArtifacts(t *testing.T) {
	home := t.TempDir()
	put := func(rel, data string) {
		t.Helper()
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	put(".local/bin/ryo-example", "unrelated")
	put(".local/bin/ryoku", "original")
	put(".config/systemd/user/ryoku-custom.service", "custom")
	before, err := snapshotArtifacts(home)
	if err != nil {
		t.Fatal(err)
	}
	put(".local/bin/ryoku", "installed")
	put(".local/bin/ryogami", "installed")
	put(".config/systemd/user/ryoku-shell.service", "installed")
	if err := recordArtifacts(home, before); err != nil {
		t.Fatal(err)
	}
	var stopped bool
	run := func(name string, args ...string) error {
		if len(args) > 1 && args[1] == "disable" {
			stopped = true
		}
		return nil
	}
	if err := uninstallArtifacts(home, false, run); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{".local/bin/ryo-example": "unrelated", ".local/bin/ryoku": "original", ".config/systemd/user/ryoku-custom.service": "custom"} {
		b, err := os.ReadFile(filepath.Join(home, rel))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %s, %v", rel, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".local/bin/ryogami")); !os.IsNotExist(err) {
		t.Fatal("owned binary survived")
	}
	if !stopped {
		t.Fatal("installed service was not stopped")
	}
}
func TestSourceOwnershipPreservesLaterEdits(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local/bin")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "ryoku")
	os.WriteFile(path, []byte("installed"), 0o755)
	if err := recordArtifacts(home, map[string]artifact{}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("user replacement"), 0o755)
	if err := uninstallArtifacts(home, false, func(string, ...string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "user replacement" {
		t.Fatal("removed user's replacement")
	}
}
