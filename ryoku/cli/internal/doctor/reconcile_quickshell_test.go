package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const symbolError = `quickshell: symbol lookup error: quickshell: undefined symbol: ` +
	`_ZN23QUntypedPropertyBindingC1EP23QPropertyBindingPrivate, version Qt_6_PRIVATE_API`

// stubPath puts fake binaries on PATH so the reconciler can be driven without a
// real quickshell.
func stubPath(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestLoaderFailureRecognisesTheQtPrivateApiBreak(t *testing.T) {
	if got := loaderFailure(symbolError); got == "" {
		t.Fatal("the reported black-screen error must be recognised")
	}
	for _, out := range []string{
		"quickshell: error while loading shared libraries: libQt6Quick.so.6: cannot open shared object file",
		"/home/u/.local/lib/qt6/qml/Ryoku/Blobs/libblobs.so: undefined symbol: _ZN9QQmlDebug",
	} {
		if loaderFailure(out) == "" {
			t.Fatalf("not recognised as a loader failure: %s", out)
		}
	}
	for _, out := range []string{"Quickshell 0.3.0", "", "some other crash"} {
		if got := loaderFailure(out); got != "" {
			t.Fatalf("loaderFailure(%q) = %q, want none", out, got)
		}
	}
}

func TestQuickshellBrokenAsksForAnUpgrade(t *testing.T) {
	stubPath(t, map[string]string{
		"qs": "echo '" + symbolError + "' >&2; exit 127",
	})
	res := reconcileQuickshell(true)
	if res.status != recWarn {
		t.Fatalf("status = %v (%s)", res.status, res.detail)
	}
	if !strings.Contains(res.remedy, "dnf upgrade quickshell") {
		t.Fatalf("a broken renderer has to come with the command that fixes it: %q", res.remedy)
	}
}

func TestQuickshellRunningIsQuiet(t *testing.T) {
	stubPath(t, map[string]string{
		"qs": "echo 'Quickshell 0.3.0'; exit 0",
	})
	if res := reconcileQuickshell(true); res.status != recOK {
		t.Fatalf("status = %v (%s)", res.status, res.detail)
	}
}

func TestQuickshellMissingIsReported(t *testing.T) {
	stubPath(t, map[string]string{})
	res := reconcileQuickshell(true)
	if res.status != recWarn || !strings.Contains(res.detail, "not installed") {
		t.Fatalf("status = %v (%s)", res.status, res.detail)
	}
}

// A locally built module is ours to move, which is what un-blacks the screen for
// someone developing a plugin.
func TestQuickshellStaleLocalModuleIsMovedAside(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mod := filepath.Join(home, ".local", "lib", "qt6", "qml", "Ryoku", "Blobs", "libblobs.so")
	if err := os.MkdirAll(filepath.Dir(mod), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mod, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubPath(t, map[string]string{
		"qs": "echo 'quickshell: symbol lookup error: " + mod + ": undefined symbol: _ZN9QQmlDebug, version Qt_6_PRIVATE_API' >&2; exit 127",
	})

	if res := reconcileQuickshell(true); res.status != recWouldFix {
		t.Fatalf("check-only status = %v (%s)", res.status, res.detail)
	}
	if _, err := os.Stat(mod); err != nil {
		t.Fatal("check-only must not move anything")
	}
	res := reconcileQuickshell(false)
	if res.status != recFixed {
		t.Fatalf("fix status = %v (%s)", res.status, res.detail)
	}
	if _, err := os.Stat(mod); !os.IsNotExist(err) {
		t.Fatal("the stale module should have been moved aside")
	}
	if _, err := os.Stat(mod + ".stale"); err != nil {
		t.Fatalf("the module should be kept beside its place, not deleted: %v", err)
	}
}

func TestStaleQmlModuleIgnoresSystemModules(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := staleQmlModule("/usr/lib/qt6/qml/QtQuick/libqtquick.so: undefined symbol: x"); got != "" {
		t.Fatalf("a packaged module is not ours to move: %s", got)
	}
}
