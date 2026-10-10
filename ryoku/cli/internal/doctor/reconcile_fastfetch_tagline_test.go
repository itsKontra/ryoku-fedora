package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const seededTagline = `{ "type": "custom", "format": "\u001b[38;2;143;135;112mRYOKU · 力 · a hand-built Arch desktop\u001b[0m" },`

func TestUpgradeFastfetchTagline(t *testing.T) {
	got, changed := upgradeFastfetchTagline(seededTagline)
	if !changed || !strings.Contains(got, "a hand-built Fedora desktop") || strings.Contains(got, "Arch") {
		t.Fatalf("seeded tagline not upgraded:\n%s", got)
	}
	if again, changed := upgradeFastfetchTagline(got); changed || again != got {
		t.Fatal("upgrading twice must be a no-op")
	}
	saved := `"format": "RYOKU · 力 · a hand-built Arch desktop"`
	if got, changed := upgradeFastfetchTagline(saved); !changed || got != `"format": "RYOKU · 力 · a hand-built Fedora desktop"` {
		t.Fatalf("a Hub-saved config (raw UTF-8) must be upgraded too:\n%s", got)
	}
	if _, changed := upgradeFastfetchTagline(`"format": "my own tagline"`); changed {
		t.Fatal("a user-rewritten tagline must be left alone")
	}
}

func TestReconcileFastfetchTagline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir := filepath.Join(home, ".config", "fastfetch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(path, []byte(seededTagline), 0o644); err != nil {
		t.Fatal(err)
	}

	if r := reconcileFastfetchTagline(true); r.status != recWouldFix {
		t.Fatalf("check mode status = %v, want would-fix", r.status)
	}
	if b, _ := os.ReadFile(path); string(b) != seededTagline {
		t.Fatal("check mode must not write")
	}
	if r := reconcileFastfetchTagline(false); r.status != recFixed {
		t.Fatalf("apply status = %v, want fixed", r.status)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "a hand-built Fedora desktop") {
		t.Fatalf("tagline not rewritten:\n%s", b)
	}
	if r := reconcileFastfetchTagline(true); r.status != recOK {
		t.Fatalf("second pass status = %v, want ok", r.status)
	}
}
