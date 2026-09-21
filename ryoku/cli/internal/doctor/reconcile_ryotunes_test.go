package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileRyotunesFedora(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	log := filepath.Join(home, "commands")
	installed := filepath.Join(home, "installed")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	t.Setenv("RYOKU_REPO", "")
	t.Setenv("PATH", bin)
	t.Setenv("TEST_COMMANDS", log)
	t.Setenv("TEST_INSTALLED", installed)
	scripts := map[string]string{
		"dnf": "exit 0",
		"rpm": `case "$*" in
  *ryoku-desktop*) exit 0 ;;
  *ryotunes*) test -f "$TEST_INSTALLED" ;;
  *) exit 1 ;;
esac`,
		"sudo": `echo "$*" >> "$TEST_COMMANDS"
test "$*" = "dnf -y install ryotunes" || exit 1
: > "$TEST_INSTALLED"`,
		"systemctl": `echo "$*" >> "$TEST_COMMANDS"
case "$*" in *is-enabled*) exit 1 ;; esac`,
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if result := reconcileRyotunes(true); result.status != recWouldFix {
		t.Fatalf("check: %s %s", result.status.label(), result.detail)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Fatal("check mode installed the package")
	}
	if result := reconcileRyotunes(false); result.status != recFixed {
		t.Fatalf("fix: %s %s", result.status.label(), result.detail)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dnf -y install ryotunes", "--user enable --now ryotunesd.socket"} {
		if !strings.Contains(string(calls), want) {
			t.Fatalf("missing %s: %s", want, calls)
		}
	}
}
