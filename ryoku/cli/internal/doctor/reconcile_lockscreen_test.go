package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockscreenInstallerRunsForLegacyTapeUpgrade(t *testing.T) {
	for _, test := range []struct {
		name          string
		lockerPresent bool
		legacyTape    bool
		want          bool
	}{
		{name: "missing locker", want: true},
		{name: "current install", lockerPresent: true, want: false},
		{name: "legacy Tape on existing install", lockerPresent: true, legacyTape: true, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := needsLockscreenInstaller(test.lockerPresent, test.legacyTape); got != test.want {
				t.Fatalf("needsLockscreenInstaller() = %v, want %v", got, test.want)
			}
		})
	}
}

// greeterScriptStale is the decision that carries greeter fixes (the NVIDIA
// software-cursor renderer) to a box the package never touched. It must fire on
// a missing or drifted script and stay quiet when the installed copy already
// matches or when there is no shipped source to reconcile from (a package box).
func TestGreeterScriptStale(t *testing.T) {
	saved := greeterCompositorBin
	defer func() { greeterCompositorBin = saved }()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("#!/bin/sh\n--renderer=pixman\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	greeterCompositorBin = filepath.Join(dir, "installed")

	if greeterScriptStale("") {
		t.Error("no shipped source (package box) must not report drift")
	}
	if !greeterScriptStale(src) {
		t.Error("a missing installed script must report stale")
	}
	if err := os.WriteFile(greeterCompositorBin, []byte("#!/bin/sh\nold\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !greeterScriptStale(src) {
		t.Error("a drifted installed script must report stale")
	}
	if err := os.WriteFile(greeterCompositorBin, []byte("#!/bin/sh\n--renderer=pixman\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if greeterScriptStale(src) {
		t.Error("an up-to-date installed script must not report stale")
	}
}

func writeGreeterFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// wantGreeterPick maps the lock preference to the greeter the login screen must
// wear: the package's stock dir for the default skin, the picked dir otherwise,
// and nothing when the picked skin is gone or the preference is not a slug.
func TestWantGreeterPick(t *testing.T) {
	themes := t.TempDir()
	writeGreeterFile(t, filepath.Join(themes, "nier-automata", "Main.qml"), "nier")

	for _, test := range []struct {
		pref string
		want greeterPick
	}{
		{pref: "", want: greeterPick{theme: stockGreeterTheme}},
		{pref: defaultLockSkin + "\n", want: greeterPick{theme: stockGreeterTheme}},
		{pref: "nier-automata\n", want: greeterPick{theme: pickedGreeterTheme, src: filepath.Join(themes, "nier-automata")}},
		{pref: "uninstalled-skin", want: greeterPick{}},
		{pref: "../../etc", want: greeterPick{}},
		{pref: "/etc", want: greeterPick{}},
	} {
		if got := wantGreeterPick(test.pref, themes); got != test.want {
			t.Errorf("wantGreeterPick(%q) = %+v, want %+v", test.pref, got, test.want)
		}
	}
}

// A package update used to lay the stock skin back over a picked one. The
// reconciler must notice a box whose config still selects the stock dir while
// the user picked another skin, and one whose picked copy drifted from the
// user's skin; a box already wearing the pick is left alone.
func TestGreeterPickStale(t *testing.T) {
	dir := t.TempDir()
	skin := filepath.Join(dir, "themes", "nier-automata")
	writeGreeterFile(t, filepath.Join(skin, "Main.qml"), "nier")
	picked := filepath.Join(dir, "sddm", pickedGreeterTheme)
	conf := filepath.Join(dir, "99-ryoku.conf")
	pick := greeterPick{theme: pickedGreeterTheme, src: skin}

	if greeterPickStale(pick, conf, picked) {
		t.Error("a box without the Ryoku greeter config has nothing to steer")
	}
	writeGreeterFile(t, conf, "[Theme]\nCurrent="+stockGreeterTheme+"\n")
	if !greeterPickStale(pick, conf, picked) {
		t.Error("a pick the config does not select must report stale")
	}
	writeGreeterFile(t, conf, "[Theme]\nCurrent="+pickedGreeterTheme+"\n")
	if !greeterPickStale(pick, conf, picked) {
		t.Error("a missing picked copy must report stale")
	}
	writeGreeterFile(t, filepath.Join(picked, "Main.qml"), "nier")
	if greeterPickStale(pick, conf, picked) {
		t.Error("a greeter already wearing the pick must not report stale")
	}
	if !greeterPickStale(greeterPick{theme: stockGreeterTheme}, conf, picked) {
		t.Error("returning to the stock skin must repoint the config")
	}
	if greeterPickStale(greeterPick{}, conf, picked) {
		t.Error("an unresolvable pick must not report stale")
	}
}
