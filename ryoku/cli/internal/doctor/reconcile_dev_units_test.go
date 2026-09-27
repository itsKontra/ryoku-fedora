package doctor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeUnitT(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeUnitHost points the scan at temp dirs: a packaged unit dir owned by the
// named packages, and a home whose ~/.config/systemd/user holds the units.
func fakeUnitHost(t *testing.T, owners map[string]string) (home, userDir, pkgDir string) {
	home, pkgDir = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	userDir = filepath.Join(home, ".config", "systemd", "user")
	oldDir, oldOwner := packagedUnitDir, packagedUnitOwner
	t.Cleanup(func() { packagedUnitDir, packagedUnitOwner = oldDir, oldOwner })
	packagedUnitDir = pkgDir
	packagedUnitOwner = func(path string) string { return owners[filepath.Base(path)] }
	for name := range owners {
		writeUnitT(t, filepath.Join(pkgDir, name), "ExecStart=/usr/bin/x\n")
	}
	return home, userDir, pkgDir
}

func TestCheckoutUnitResidueFindsOnlyShadowingCheckoutUnits(t *testing.T) {
	home, userDir, _ := fakeUnitHost(t, map[string]string{
		"ryoku-rashin.service":   "ryoku-rashin",
		"ryoku-ai-usage.service": "ryoku-desktop",
		"foreign.service":        "someone-else",
	})
	bin := filepath.Join(home, ".local", "bin")
	writeUnitT(t, filepath.Join(userDir, "ryoku-rashin.service"), "[Service]\nExecStart="+bin+"/ryoku-rashin serve\n")
	// already materialized from the package: only a comment names ~/.local/bin
	writeUnitT(t, filepath.Join(userDir, "ryoku-ai-usage.service"), "# the dev deploy rewrites these to "+bin+"\nExecStart=-/usr/bin/claude-usage\n")
	writeUnitT(t, filepath.Join(userDir, "foreign.service"), "ExecStart="+bin+"/foreign\n")
	// the user's own unit, no packaged twin
	writeUnitT(t, filepath.Join(userDir, "mine.service"), "ExecStart="+bin+"/mine\n")

	got := checkoutUnitResidue()
	want := []string{filepath.Join(userDir, "ryoku-rashin.service")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("residue = %v, want %v", got, want)
	}
}

func TestRetireCheckoutUnitsReenablesAndRestarts(t *testing.T) {
	_, userDir, _ := fakeUnitHost(t, nil)
	unit := filepath.Join(userDir, "ryoku-rashin.service")
	writeUnitT(t, unit, "ExecStart=/home/u/.local/bin/ryoku-rashin\n")

	var calls []string
	old := systemctlUser
	t.Cleanup(func() { systemctlUser = old })
	systemctlUser = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}

	if kept := retireCheckoutUnits([]string{unit}); len(kept) != 0 {
		t.Fatalf("kept = %v", kept)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatal("the checkout unit must be removed")
	}
	want := []string{
		"is-enabled --quiet ryoku-rashin.service",
		"is-active --quiet ryoku-rashin.service",
		"daemon-reload",
		"reenable ryoku-rashin.service",
		"try-restart ryoku-rashin.service",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("systemctl calls = %v, want %v", calls, want)
	}
}
