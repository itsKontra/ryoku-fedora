package doctor

import (
	"os"
	"strings"

	"ryoku-cli/internal/kmod"
	"ryoku-cli/internal/sys"
	i18n "ryoku-i18n"
)

// On Fedora, RPM Fusion's packaging owns the NVIDIA setup and Ryoku's NVIDIA
// package fills its gaps. Doctor heals only what a package cannot own: the
// package itself, the kernel arguments and the fallback unit's enablement.

const nvidiaFallbackUnit = "nvidia-fallback.service"

var installDriverCommand = kmod.Advice("install-driver")

// rpmFusionNouveauBlacklist is the exact argument pair RPM Fusion puts on every
// kernel. nvidia-fallback.service fires only on this string, so it is restored
// verbatim, never rewritten.
var rpmFusionNouveauBlacklist = []string{
	"rd.driver.blacklist=nouveau,nova_core",
	"modprobe.blacklist=nouveau,nova_core",
}

var (
	fedoraNvidiaGPU = func() kmod.GPU {
		return kmod.New(kmod.Config{}).GPU()
	}
	fedoraNvidiaChoice = func() kmod.Choice {
		return kmod.New(kmod.Config{}).Choice()
	}
	installNvidiaSupport = func() error {
		return sys.Sudo(ryokuHostBin(), "pkg", "install", kmod.DriverPackage)
	}
	runningKernelCmdline    = "/proc/cmdline"
	kernelCmdlineTemplate   = "/etc/kernel/cmdline"
	restoreNouveauBlacklist = func() error {
		return sys.Sudo("grubby", "--update-kernel=ALL", "--args="+strings.Join(rpmFusionNouveauBlacklist, " "))
	}
	enableNvidiaFallback = func() error {
		return runSystemService("enable", nvidiaFallbackUnit)
	}
)

// fedoraNvidiaChecks replace the Arch and Void NVIDIA reconcilers on Fedora.
func fedoraNvidiaChecks() []reconciler {
	return []reconciler{
		{i18n.T("NVIDIA driver"), reconcileFedoraNvidiaDriver},
		{i18n.T("Ryoku NVIDIA package"), reconcileFedoraNvidiaPackage},
		{i18n.T("nouveau blacklist arguments"), reconcileFedoraNouveauBlacklist},
		{i18n.T("NVIDIA fallback service"), reconcileFedoraNvidiaFallback},
	}
}

// reconcileFedoraNvidiaDriver only ever advises. Installing the driver enables
// third-party repositories and may need someone at the console to approve an
// enrollment, so it never runs unattended inside `ryoku update`.
func reconcileFedoraNvidiaDriver(bool) recResult {
	installed := nvidiaPackageInstalled(kmod.AkmodPackage)
	switch fedoraNvidiaGPU() {
	case kmod.GPUAbsent:
		return okRes(i18n.T("no NVIDIA GPU detected"))
	case kmod.GPUUnknown:
		return okRes(i18n.T("cannot tell whether this GPU takes the NVIDIA driver; the GPU check needs lspci from pciutils"))
	case kmod.GPUUnsupported:
		if installed {
			return warnRes(i18n.T("akmod-nvidia is installed, but this NVIDIA GPU is older than Turing and the NVIDIA driver cannot drive it; doctor leaves the package alone, and the GPU works on nouveau without it")).
				withFix("sudo dnf remove %s", kmod.AkmodPackage)
		}
		return okRes(i18n.T("this NVIDIA GPU is older than Turing, so it stays on nouveau"))
	}
	if installed || sys.Exists(nvidiaModuleDir) {
		return okRes(i18n.T("the NVIDIA driver is installed"))
	}
	if fedoraNvidiaChoice() == kmod.ChoiceNouveau {
		return okRes(i18n.T("this NVIDIA GPU stays on nouveau, as chosen at install"))
	}
	return noteRes(i18n.T("this NVIDIA GPU runs on nouveau; one command installs the NVIDIA driver, built and signed on this machine")).
		withFix("%s", installDriverCommand)
}

// nvidiaDriverOnSupportedGPU is a machine whose NVIDIA setup doctor keeps:
// RPM Fusion's driver installed for a GPU the generation check passed.
func nvidiaDriverOnSupportedGPU() bool {
	return nvidiaPackageInstalled(kmod.AkmodPackage) && fedoraNvidiaGPU() == kmod.GPUSupported
}

// reconcileFedoraNvidiaPackage brings back what RPM Fusion leaves out: video
// decode, the initramfs without nouveau and the fallback unit's preset.
func reconcileFedoraNvidiaPackage(checkOnly bool) recResult {
	if !nvidiaDriverOnSupportedGPU() {
		return okRes(i18n.T("no NVIDIA driver on a supported GPU, so Ryoku's NVIDIA package is not needed"))
	}
	if nvidiaPackageInstalled(kmod.DriverPackage) {
		return okRes(i18n.T("Ryoku's NVIDIA package %s is installed"), kmod.DriverPackage)
	}
	advice := "sudo dnf install " + kmod.DriverPackage
	if checkOnly {
		return wouldRes(i18n.T("the NVIDIA driver is installed without Ryoku's NVIDIA package %s, so hardware video decode is missing and the initramfs carries nouveau"), kmod.DriverPackage).
			withFix("%s", advice)
	}
	if err := installNvidiaSupport(); err != nil {
		return failRes(i18n.T("could not install Ryoku's NVIDIA package %s: %v"), kmod.DriverPackage, err).
			withFix("%s", advice)
	}
	if !nvidiaPackageInstalled(kmod.DriverPackage) {
		return failRes(i18n.T("Ryoku's NVIDIA package %s is still not installed"), kmod.DriverPackage).
			withFix("%s", advice)
	}
	return fixedRes(i18n.T("installed Ryoku's NVIDIA package %s again"), kmod.DriverPackage)
}

// reconcileFedoraNouveauBlacklist reads the running kernel's command line and
// /etc/kernel/cmdline, the one every new kernel inherits; the boot entries
// themselves are readable only by root.
func reconcileFedoraNouveauBlacklist(checkOnly bool) recResult {
	if !nvidiaDriverOnSupportedGPU() {
		return okRes(i18n.T("no NVIDIA driver on a supported GPU, so nouveau stays unblacklisted"))
	}
	running, _ := readKernelCmdline(runningKernelCmdline)
	template, haveTemplate := readKernelCmdline(kernelCmdlineTemplate)
	remedy := `sudo grubby --update-kernel=ALL --args="` + strings.Join(rpmFusionNouveauBlacklist, " ") + `"`
	switch {
	case hasNouveauBlacklist(running) && (!haveTemplate || hasNouveauBlacklist(template)):
		return okRes(i18n.T("the kernel command line blacklists nouveau for the NVIDIA driver"))
	case haveTemplate && hasNouveauBlacklist(template):
		return noteRes(i18n.T("this boot started without the nouveau blacklist the kernels carry; reboot so nouveau stays off the NVIDIA GPU")).
			withFix("%s", remedy)
	}
	if checkOnly {
		return wouldRes(i18n.T("the kernel command line lacks RPM Fusion's nouveau blacklist, so nouveau can take the GPU before the NVIDIA driver and the nouveau fallback never starts")).
			withFix("%s", remedy)
	}
	if err := restoreNouveauBlacklist(); err != nil {
		return failRes(i18n.T("could not restore the nouveau blacklist on the kernel command line: %v"), err).
			withFix("%s", remedy)
	}
	return fixedRes(i18n.T("restored RPM Fusion's nouveau blacklist on every installed kernel; it takes effect at the next boot"))
}

// reconcileFedoraNvidiaFallback keeps the unit that loads nouveau when a kernel
// boots before its NVIDIA module exists. No Fedora or RPM Fusion preset enables
// it; Ryoku's NVIDIA package does, and doctor keeps it so.
func reconcileFedoraNvidiaFallback(checkOnly bool) recResult {
	if !nvidiaPackageInstalled(kmod.AkmodPackage) {
		return okRes(i18n.T("no NVIDIA driver installed, so the nouveau fallback is not needed"))
	}
	if systemServiceEnabled(nvidiaFallbackUnit) {
		return okRes(i18n.T("%s is enabled"), nvidiaFallbackUnit)
	}
	remedy := systemServiceEnableAdvice(nvidiaFallbackUnit, false)
	if checkOnly {
		return wouldRes(i18n.T("%s is disabled, so a kernel booted before its NVIDIA module exists comes up with no display driver instead of nouveau"), nvidiaFallbackUnit).
			withFix("%s", remedy)
	}
	if err := enableNvidiaFallback(); err != nil {
		return failRes(i18n.T("could not enable %s: %v"), nvidiaFallbackUnit, err).withFix("%s", remedy)
	}
	return fixedRes(i18n.T("enabled %s, so a kernel without its NVIDIA module falls back to nouveau"), nvidiaFallbackUnit)
}

func hasNouveauBlacklist(cmdline string) bool {
	args := strings.Fields(cmdline)
	for _, want := range rpmFusionNouveauBlacklist {
		found := false
		for _, arg := range args {
			found = found || arg == want
		}
		if !found {
			return false
		}
	}
	return true
}

func readKernelCmdline(path string) (string, bool) {
	data, err := os.ReadFile(path)
	return string(data), err == nil
}
