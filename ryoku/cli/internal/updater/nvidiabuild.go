package updater

import (
	"os"

	"ryoku-cli/internal/host"
	"ryoku-cli/internal/kmod"
	i18n "ryoku-i18n"
)

// The system lane on Fedora can install a kernel whose NVIDIA module akmods
// has not built yet; that kernel then boots into a driver fallback. Waiting
// for the build here, instead of leaving it to akmods' background hook and the
// build at boot, lets the user know before rebooting whether the next boot has
// the driver. The kernel itself is never held back.

var (
	nvidiaAkmodInstalled = func() bool {
		return host.Default().Package([]string{"installed", kmod.AkmodPackage}) == host.ExitOK
	}
	nvidiaBuildForKernel = runNvidiaBuildForKernel
)

// awaitNvidiaBuild builds the NVIDIA module for the kernel the system lane
// just installed and returns the line telling the user whether the next boot
// has the driver. It returns "" when there is no new kernel or no akmod-nvidia.
// kernelBefore is the newest kernel before the upgrade.
func awaitNvidiaBuild(kernelBefore string) string {
	kernel := newestKernel(modulesDir)
	if kernel == "" || kernel == kernelBefore || (kernelBefore != "" && host.CompareEVR(kernel, kernelBefore) < 0) {
		return ""
	}
	if !nvidiaAkmodInstalled() {
		return ""
	}
	progress.detailf(i18n.T("Building the NVIDIA driver for kernel %s"), kernel)
	result, err := nvidiaBuildForKernel(kernel)
	switch {
	case err != nil:
		return i18n.Tf("The NVIDIA driver build for kernel %s could not run (%v); akmods builds it at the next boot", kernel, err)
	case result.Built:
		return i18n.Tf("NVIDIA driver ready for the new kernel %s", kernel)
	case result.NewestWithModule != "":
		return i18n.Tf("NVIDIA driver build failed for kernel %s: the next boot uses nouveau, kernel %s still has the driver", kernel, result.NewestWithModule)
	default:
		return i18n.Tf("NVIDIA driver build failed for kernel %s: the next boot uses nouveau, and no installed kernel has the driver", kernel)
	}
}

// runNvidiaBuildForKernel runs the build as root, sleep-inhibited, in the
// foreground. akmods' own output goes to the raw log with the DNF transaction
// it follows; only the key=value report on stdout is read.
func runNvidiaBuildForKernel(kernel string) (kmod.BuildResult, error) {
	root := kmod.AsRoot{
		Progress: os.Stdout,
		Wrap:     func(argv []string) []string { return inhibited("NVIDIA driver build", argv) },
	}
	result, _, err := root.BuildForKernel(kmod.BuildOptions{Kernel: kernel})
	return result, err
}
