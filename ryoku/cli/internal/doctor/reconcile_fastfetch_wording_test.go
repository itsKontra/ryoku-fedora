package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const seededFastfetch = `{ "type": "custom", "format": "\u001b[38;2;143;135;112mRYOKU · 力 · a hand-built Arch desktop\u001b[0m" },
{ "type": "command", "key": "OS",     "text": "echo \"Ryoku $(ryoku version 2>/dev/null || echo dev)\"" },
{ "type": "command", "key": "BRANCH", "text": "ryoku version --branch 2>/dev/null || echo main" },`

func TestUpgradeFastfetchWording(t *testing.T) {
	got, whys := upgradeFastfetchWording(seededFastfetch)
	if len(whys) != 2 {
		t.Fatalf("whys = %q, want the OS line and the tagline", whys)
	}
	if !strings.Contains(got, `ryoku version --pretty 2>/dev/null || echo dev`) {
		t.Fatalf("OS line not moved to --pretty:\n%s", got)
	}
	if !strings.Contains(got, `ryoku version --branch 2>/dev/null || echo main`) {
		t.Fatalf("BRANCH line must be untouched:\n%s", got)
	}
	if !strings.Contains(got, "a hand-built Fedora desktop") || strings.Contains(got, "Arch") {
		t.Fatalf("tagline not upgraded:\n%s", got)
	}
	if again, whys := upgradeFastfetchWording(got); len(whys) != 0 || again != got {
		t.Fatal("upgrading twice must be a no-op")
	}

	saved := `"text": "echo \"Ryoku $(ryoku version 2>/dev/null || echo dev)\"", "format": "RYOKU · 力 · a hand-built Arch desktop"`
	got, whys = upgradeFastfetchWording(saved)
	if len(whys) != 2 || !strings.Contains(got, `ryoku version --pretty 2>/dev/null`) || !strings.Contains(got, "a hand-built Fedora desktop") {
		t.Fatalf("a Hub-saved config (JSON-escaped >, raw UTF-8) must be upgraded too:\n%s", got)
	}

	if _, whys := upgradeFastfetchWording(`{ "key": "OS", "text": "echo mine" }, "format": "my own tagline"`); len(whys) != 0 {
		t.Fatal("user-rewritten lines must be left alone")
	}
}

func TestReconcileFastfetchWording(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir := filepath.Join(home, ".config", "fastfetch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(path, []byte(seededFastfetch), 0o644); err != nil {
		t.Fatal(err)
	}

	if r := reconcileFastfetchWording(true); r.status != recWouldFix {
		t.Fatalf("check mode status = %v, want would-fix", r.status)
	}
	if b, _ := os.ReadFile(path); string(b) != seededFastfetch {
		t.Fatal("check mode must not write")
	}
	if r := reconcileFastfetchWording(false); r.status != recFixed {
		t.Fatalf("apply status = %v, want fixed", r.status)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "a hand-built Fedora desktop") {
		t.Fatalf("config not rewritten:\n%s", b)
	}
	if r := reconcileFastfetchWording(true); r.status != recOK {
		t.Fatalf("second pass status = %v, want ok", r.status)
	}
}
