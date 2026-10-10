package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ---- reconciler: display backlight -------------------------------------------

// reconcileBacklight flags the common brightness failures:
//   - no backlight interface at all.
//   - backlight present but no brightnessctl to drive it.
//   - hybrid-GPU laptop with only a firmware backlight.
//
// for the last we trust the kernel's own verdict (dGPU reports no native
// backlight) over a sysfs value the panel may ignore. detect-and-warn only;
// the fixes (GPU mux switch, kernel parameters) are too machine-specific to
// apply blindly.
func reconcileBacklight(_ bool) recResult {
	devs := backlightDevices()
	if len(devs) == 0 {
		if !isLaptop() {
			return okRes(i18n.T("no internal backlight (desktop or external display)"))
		}
		return warnRes(i18n.T("no backlight interface found; display brightness cannot be set")).
			withFix(i18n.T("try a kernel parameter such as acpi_backlight=native or acpi_backlight=vendor"))
	}
	if !sys.Has("brightnessctl") {
		return warnRes(i18n.T("backlight present but brightnessctl is missing; brightness keys and idle-dim will not work")).
			withFix("sudo pacman -S brightnessctl")
	}
	if gpus := gpuDriversLoaded(); len(gpus) >= 2 && onlyFirmwareBacklight(devs) {
		detail := fmt.Sprintf(i18n.T("hybrid GPU (%s) with only a firmware backlight (%s); the panel may not dim"),
			strings.Join(gpus, "+"), strings.Join(devs, ","))
		if nvidiaBacklightDead() {
			detail = fmt.Sprintf(i18n.T("hybrid GPU (%s): the kernel reports the dGPU has no working backlight, and the firmware fallback (%s) does not dim the panel"),
				strings.Join(gpus, "+"), strings.Join(devs, ","))
		}
		fix := i18n.T("route the panel to the iGPU: set the BIOS GPU/MUX mode to Hybrid and reboot")
		if name := igpuBacklightName(gpus); name != "" {
			fix += i18n.Tf(", then %s appears", name)
		} else {
			fix += i18n.T(" so the panel's native backlight appears")
		}
		if sys.Has("supergfxctl") {
			fix += i18n.T("; on a supported ASUS laptop `supergfxctl -m Hybrid` switches it without a BIOS trip")
		}
		return noteRes("%s", detail).withFix(fix)
	}
	if len(devs) > 1 {
		// More than one backlight means one of them is a phantom on this
		// hardware, and a brightness control that "only has off and full" is
		// exactly what writing percentages to the wrong one looks like. Name
		// the device every Ryoku writer targets and each candidate's current
		// level, so the next report from such a box pinpoints it in one line.
		pick := backlightPick()
		if pick == "" {
			pick = "-"
		}
		return noteRes(i18n.T("backlight: %s; Ryoku writes to %s (%s)"),
			strings.Join(devs, ", "), pick, backlightLevels(devs))
	}
	return okRes(i18n.T("backlight: %s"), strings.Join(devs, ", "))
}

// backlightSysRoot is the sysfs root the backlight readings use; a var so the
// level formatter is unit-tested against a fixture tree.
var backlightSysRoot = "/sys/class/backlight"

// backlightPick is the device every Ryoku brightness writer targets (the media
// keys, the OSD daemon and the bar slider all call the same helper). Empty when
// the helper is absent or cannot decide.
var backlightPick = func() string {
	out, err := sys.RunOut("ryoku-hw-backlight")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// backlightLevels renders "name cur/max" per device from sysfs, so a phantom
// (stuck at max while the panel dims, or stuck at 0) is visible in the report.
func backlightLevels(devs []string) string {
	parts := make([]string, 0, len(devs))
	for _, d := range devs {
		read := func(attr string) string {
			b, err := os.ReadFile(filepath.Join(backlightSysRoot, d, attr))
			if err != nil {
				return "?"
			}
			return strings.TrimSpace(string(b))
		}
		parts = append(parts, fmt.Sprintf("%s %s/%s", d, read("brightness"), read("max_brightness")))
	}
	return strings.Join(parts, ", ")
}

// nvidiaBacklightDead: the kernel's own tell that the dGPU has no usable
// backlight and fell back to the often-broken ACPI/EC interface.
func nvidiaBacklightDead() bool {
	n := strings.TrimSpace(captureOut("sh", "-c",
		"journalctl -k -b --no-pager 2>/dev/null | grep -ic 'no NVIDIA native backlight'"))
	return n != "" && n != "0"
}

func backlightDevices() []string {
	entries, err := os.ReadDir("/sys/class/backlight")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func onlyFirmwareBacklight(devs []string) bool {
	for _, d := range devs {
		if strings.TrimSpace(readFileSafe("/sys/class/backlight/"+d+"/type")) != "firmware" {
			return false
		}
	}
	return len(devs) > 0
}

// gpuDriversLoaded: loaded GPU kernel drivers, so a hybrid-GPU box is
// recognizable.
func gpuDriversLoaded() []string {
	var out []string
	for _, m := range []string{"amdgpu", "nvidia", "i915", "nouveau", "xe"} {
		if sys.Exists("/sys/module/" + m) {
			out = append(out, m)
		}
	}
	return out
}

// igpuBacklightName is the native panel backlight the integrated GPU exposes
// once the panel is routed to it: intel_backlight for an Intel iGPU (i915/xe),
// amdgpu_bl0 for an AMD one. Intel wins when both are present, since an Intel
// iGPU drives the eDP even beside an AMD discrete card. "" when neither driver
// is loaded, so the hint drops the device name rather than naming the wrong one.
func igpuBacklightName(gpus []string) string {
	has := func(m string) bool {
		for _, g := range gpus {
			if g == m {
				return true
			}
		}
		return false
	}
	switch {
	case has("i915") || has("xe"):
		return "intel_backlight"
	case has("amdgpu"):
		return "amdgpu_bl0"
	default:
		return ""
	}
}

// isLaptop: machine has a battery, i.e. an internal panel whose backlight
// we'd expect to control.
func isLaptop() bool {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "BAT") {
			return true
		}
	}
	return false
}

// ---- NVIDIA driver detection ----------------------------------------------

var nvidiaPCIOutput = func() string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func keplerGpuPresent() bool {
	pci := strings.ToLower(nvidiaPCIOutput())
	return strings.Contains(pci, "nvidia") && strings.Contains(pci, "gk")
}

// nvidiaDriverActive: does this box use the proprietary/open nvidia driver?
// a loaded module is the clearest tell, but nouveau can win the boot race,
// so the module may NOT be loaded -- fall back to "an nvidia kernel-module
// package is installed". the userspace driver alone is excluded: with no
// module to load, the sleep units have no VRAM to save.
func nvidiaDriverActive() bool {
	return nvidiaDriverActiveFor(keplerGpuPresent(), nvidiaDriverPackagePresent(), gpuDriversLoaded())
}

func nvidiaDriverActiveFor(kepler, packagePresent bool, loaded []string) bool {
	if kepler && !packagePresent {
		return false
	}
	for _, m := range loaded {
		if m == "nvidia" {
			return true
		}
	}
	return packagePresent
}

// nvidiaDriverPackagePresent: a kernel-module package is installed, Ryoku's
// signed modules or RPM Fusion's akmod.
func nvidiaDriverPackagePresent() bool {
	return anyPkgInstalled("ryoku-nvidia", "akmod-nvidia")
}

// ---- reconciler: NVIDIA Wayland autostart ------------------------------------

const (
	nvidiaSysAutostartDesktop = "/etc/xdg/autostart/nvidia-settings-user.desktop"
	nvidiaAutostartMask       = "[Desktop Entry]\nType=Application\nName=nvidia-settings\nExec=nvidia-settings -l\nHidden=true\nX-systemd-skip=true\n"
)

// nvidiaAutostartMasked checks if an autostart desktop entry has been disabled
// via Hidden=true or X-systemd-skip=true.
func nvidiaAutostartMasked(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "Hidden=true") || strings.EqualFold(trimmed, "X-systemd-skip=true") {
			return true
		}
	}
	return false
}

func reconcileNvidiaAutostart(checkOnly bool) recResult {
	if !sys.Exists(nvidiaSysAutostartDesktop) {
		return okRes("no conflicting X11 nvidia-settings autostart entry")
	}
	userFile := filepath.Join(sys.ConfigHome(), "autostart", "nvidia-settings-user.desktop")
	if sys.Exists(userFile) && nvidiaAutostartMasked(readFileSafe(userFile)) {
		return okRes("nvidia-settings autostart masked for Wayland")
	}
	if checkOnly {
		return wouldRes("nvidia-settings autostart is active but fails under Wayland (NV-CONTROL is X11-only)").
			withFix("ryoku doctor  (masks it in ~/.config/autostart)")
	}
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		return failRes("creating autostart dir: %v", err)
	}
	if err := os.WriteFile(userFile, []byte(nvidiaAutostartMask), 0o644); err != nil {
		return failRes("writing %s: %v", userFile, err)
	}
	_ = sys.Run("systemctl", "--user", "daemon-reload")
	_ = sys.Run("systemctl", "--user", "reset-failed", "app-nvidia\\x2dsettings\\x2duser@autostart.service")
	return fixedRes("masked nvidia-settings autostart in ~/.config/autostart (fails under Wayland)")
}

// ---- reconciler: NVIDIA sleep units ------------------------------------------

// nvidiaSleepUnits are the systemd sleep hooks xorg-x11-drv-nvidia-power ships
// and presets enabled. They save and restore the GPU's VRAM across
// suspend/hibernate; with the driver's default NVreg_UseKernelSuspendNotifiers=1
// they exit early (the kernel PM chain does the work), but boxes that turn the
// notifier off depend on them. A box that carries them disabled has drifted
// from the package's preset, repaired here.
var nvidiaSleepUnits = []string{
	"nvidia-suspend.service",
	"nvidia-hibernate.service",
	"nvidia-resume.service",
}

const nvidiaUnitDir = "/usr/lib/systemd/system"

// planNvidiaSleepUnits decides from observed state: which of the units exist
// and which are enabled. Pure, so the healthy-box silence and the
// partial-enable repair are testable without a live systemd.
func planNvidiaSleepUnits(nvidiaActive bool, exists map[string]bool, enabled map[string]bool) (missing []string, verdict string) {
	if !nvidiaActive {
		return nil, "no proprietary NVIDIA driver in use"
	}
	for _, u := range nvidiaSleepUnits {
		if !exists[u] {
			// xorg-x11-drv-nvidia-power not installed: nothing to enable.
			return nil, "the NVIDIA sleep units are not installed on this machine"
		}
	}
	for _, u := range nvidiaSleepUnits {
		if !enabled[u] {
			missing = append(missing, u)
		}
	}
	if len(missing) == 0 {
		return nil, "the NVIDIA sleep units are enabled"
	}
	return missing, ""
}

func reconcileNvidiaSleepUnits(checkOnly bool) recResult {
	exists := map[string]bool{}
	enabled := map[string]bool{}
	for _, u := range nvidiaSleepUnits {
		exists[u] = sys.Exists(nvidiaUnitDir + "/" + u)
		enabled[u] = sys.UnitEnabled(u)
	}
	missing, verdict := planNvidiaSleepUnits(nvidiaDriverActive(), exists, enabled)
	if verdict != "" {
		return okRes(i18n.T("%s"), verdict)
	}
	if checkOnly {
		return wouldRes(i18n.T("the NVIDIA sleep units (%s) are disabled, so VRAM is not saved across suspend the way the driver package presets them; a box with the kernel suspend notifier off wakes to a corrupted session"), strings.Join(missing, ", ")).
			withFix(i18n.T("ryoku doctor  (enables them with sudo systemctl)"))
	}
	args := append([]string{"systemctl", "enable"}, missing...)
	if err := sys.Sudo(args...); err != nil {
		return failRes(i18n.T("could not enable %s: %v"), strings.Join(missing, ", "), err).
			withFix(i18n.T("sudo systemctl enable %s"), strings.Join(missing, " "))
	}
	return fixedRes(i18n.T("enabled the NVIDIA sleep units so VRAM is preserved across suspend, matching the driver package's preset"))
}
