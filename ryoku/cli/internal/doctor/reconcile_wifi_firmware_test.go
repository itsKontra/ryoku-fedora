package doctor

import (
	"strings"
	"testing"
)

// stubWifiFirmware swaps the reconciler's impure inputs for fixtures: the
// package manager, the Intel card probe, which packages are installed, and the
// install, reload and radio probes. installed names the packages already
// present; the returned slice records what an install asked for.
func stubWifiFirmware(t *testing.T, manager string, card bool, installed ...string) *[]string {
	t.Helper()
	origMgr, origCard, origInst, origInstall, origReload, origRadio :=
		wifiFirmwareManager, intelWifiCardPresent, wifiFirmwareInstalled, installWifiFirmware, reloadIwlwifi, wifiRadioPresent
	t.Cleanup(func() {
		wifiFirmwareManager = origMgr
		intelWifiCardPresent = origCard
		wifiFirmwareInstalled = origInst
		installWifiFirmware = origInstall
		reloadIwlwifi = origReload
		wifiRadioPresent = origRadio
	})
	have := map[string]bool{}
	for _, p := range installed {
		have[p] = true
	}
	var asked []string
	wifiFirmwareManager = func() string { return manager }
	intelWifiCardPresent = func() bool { return card }
	wifiFirmwareInstalled = func(p string) bool { return have[p] }
	installWifiFirmware = func(_ string, pkgs []string) error {
		asked = append(asked, pkgs...)
		for _, p := range pkgs {
			have[p] = true
		}
		return nil
	}
	reloadIwlwifi = func() bool { return true }
	wifiRadioPresent = func() bool { return false }
	return &asked
}

func TestWifiFirmwareSkipsNonRPM(t *testing.T) {
	stubWifiFirmware(t, "", true)
	if got := reconcileWifiFirmware(false); got.status != recOK {
		t.Errorf("a box without dnf should be ok, got %v (%s)", got.status, got.detail)
	}
}

func TestWifiFirmwareSkipsWithoutIntelCard(t *testing.T) {
	asked := stubWifiFirmware(t, "dnf", false)
	if got := reconcileWifiFirmware(false); got.status != recOK {
		t.Errorf("no Intel card should be ok, got %v (%s)", got.status, got.detail)
	}
	if len(*asked) != 0 {
		t.Errorf("nothing should install without an Intel card, asked for %v", *asked)
	}
}

func TestWifiFirmwareInstalledIsOK(t *testing.T) {
	stubWifiFirmware(t, "dnf", true, intelWifiFirmwarePkgs...)
	if got := reconcileWifiFirmware(true); got.status != recOK {
		t.Errorf("installed firmware should be ok, got %v (%s)", got.status, got.detail)
	}
}

func TestWifiFirmwareCheckNamesMissing(t *testing.T) {
	asked := stubWifiFirmware(t, "dnf", true, "iwlwifi-dvm-firmware", "iwlegacy-firmware")
	got := reconcileWifiFirmware(true)
	if got.status != recWouldFix {
		t.Fatalf("missing firmware should be would-fix, got %v (%s)", got.status, got.detail)
	}
	if !strings.Contains(got.remedy, "dnf install iwlwifi-mvm-firmware iwlwifi-mld-firmware") {
		t.Errorf("fix should install exactly the missing packages, got %q", got.remedy)
	}
	if len(*asked) != 0 {
		t.Errorf("check mode must not install, asked for %v", *asked)
	}
}

func TestWifiFirmwareInstallsMissing(t *testing.T) {
	asked := stubWifiFirmware(t, "dnf", true, "iwlwifi-dvm-firmware")
	got := reconcileWifiFirmware(false)
	if got.status != recFixed {
		t.Fatalf("install should report fixed, got %v (%s)", got.status, got.detail)
	}
	want := "iwlwifi-mvm-firmware iwlwifi-mld-firmware iwlegacy-firmware"
	if strings.Join(*asked, " ") != want {
		t.Errorf("installed %v, want %s", *asked, want)
	}
	if !strings.Contains(got.detail, "Wi-Fi is up") {
		t.Errorf("a successful reload should say Wi-Fi is up, got %q", got.detail)
	}
}

func TestWifiFirmwareAsksForRestartWhenReloadFails(t *testing.T) {
	stubWifiFirmware(t, "dnf", true)
	reloadIwlwifi = func() bool { return false }
	got := reconcileWifiFirmware(false)
	if got.status != recFixed || !strings.Contains(got.detail, "restart") {
		t.Errorf("a failed reload should still be fixed and ask for a restart, got %v (%s)", got.status, got.detail)
	}
}
