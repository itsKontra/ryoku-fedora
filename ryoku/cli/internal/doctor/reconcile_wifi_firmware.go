package doctor

import (
	"path/filepath"
	"strings"
	"time"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ---- reconciler: Intel Wi-Fi firmware -----------------------------------------
//
// Fedora splits linux-firmware per vendor, and linux-firmware's weak deps pull
// in every Wi-Fi vendor's firmware except Intel's: the iwlwifi packages only
// arrive through the comps hardware-support group, which a Ryoku install does
// not take. Without them iwlwifi logs "no suitable firmware found!", no
// wireless interface ever appears, and the Hub's Wi-Fi list stays empty.
//
// Fires ONLY on a Fedora box with an Intel wireless controller on the PCI bus,
// and installs just the firmware packages that are missing.

var intelWifiFirmwarePkgs = []string{
	"iwlwifi-dvm-firmware",
	"iwlwifi-mvm-firmware",
	"iwlwifi-mld-firmware",
	"iwlegacy-firmware",
}

// intelWifiCardPresent reports whether a PCI network controller (class 0x0280)
// from Intel (vendor 0x8086) is present. A var so a test runs without /sys.
var intelWifiCardPresent = func() bool {
	devs, _ := filepath.Glob("/sys/bus/pci/devices/*")
	for _, d := range devs {
		class := strings.TrimSpace(readFileSafe(filepath.Join(d, "class")))
		vendor := strings.TrimSpace(readFileSafe(filepath.Join(d, "vendor")))
		if strings.HasPrefix(class, "0x0280") && vendor == "0x8086" {
			return true
		}
	}
	return false
}

var wifiFirmwareManager = sys.RPMManager

var wifiFirmwareInstalled = sys.PkgInstalled

var installWifiFirmware = func(manager string, pkgs []string) error {
	return sys.Sudo(append([]string{manager, "-y", "install"}, pkgs...)...)
}

// reloadIwlwifi re-probes the driver so it picks up the new firmware in this
// boot. Only called while no wireless interface exists, so it never drops a
// working link.
var reloadIwlwifi = func() bool {
	_ = sys.Sudo("modprobe", "-r", "iwlwifi")
	if err := sys.Sudo("modprobe", "iwlwifi"); err != nil {
		return false
	}
	for i := 0; i < 20; i++ {
		if wifiRadioPresent() {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func reconcileWifiFirmware(checkOnly bool) recResult {
	manager := wifiFirmwareManager()
	if manager == "" {
		return okRes(i18n.T("Intel Wi-Fi firmware comes with linux-firmware on this system"))
	}
	if !intelWifiCardPresent() {
		return okRes(i18n.T("this machine has no Intel wireless card"))
	}
	var missing []string
	for _, p := range intelWifiFirmwarePkgs {
		if !wifiFirmwareInstalled(p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return okRes(i18n.T("Intel Wi-Fi firmware is installed"))
	}
	fix := "sudo " + manager + " install " + strings.Join(missing, " ")
	if checkOnly {
		return wouldRes(i18n.T("the Intel wireless card has no firmware (%s missing), so Wi-Fi cannot come up"), strings.Join(missing, ", ")).
			withFix(fix)
	}
	if err := installWifiFirmware(manager, missing); err != nil {
		return failRes(i18n.T("could not install the Intel Wi-Fi firmware: %v"), err).withFix(fix)
	}
	if wifiRadioPresent() || reloadIwlwifi() {
		return fixedRes(i18n.T("installed the Intel Wi-Fi firmware (%s); Wi-Fi is up"), strings.Join(missing, ", "))
	}
	return fixedRes(i18n.T("installed the Intel Wi-Fi firmware (%s); restart to bring Wi-Fi up"), strings.Join(missing, ", "))
}

