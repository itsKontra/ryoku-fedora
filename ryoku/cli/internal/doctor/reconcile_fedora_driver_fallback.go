package doctor

import (
	"os"

	"ryoku-cli/internal/kmod"
	i18n "ryoku-i18n"
)

// Variables so tests stand in for the machine. modinfo needs no root, so the
// module check runs as the desktop user; the build goes through ryoku-host as
// root and shows akmods' progress, since it takes minutes.
var (
	runningKernel = func() string {
		return readKernelName("/proc/sys/kernel/osrelease")
	}
	nvidiaModuleBuilt = func(kernel string) bool {
		return kmod.New(kmod.Config{}).HasDriverModule(kernel)
	}
	driverFallbackGPU = nvidiaDRMPresent
	// buildNvidiaForKernel's detail is akmods' last word on the build.
	buildNvidiaForKernel = func(opts kmod.BuildOptions) (build kmod.BuildResult, detail string, err error) {
		return kmod.AsRoot{Host: ryokuHostBin(), Progress: os.Stderr}.BuildForKernel(opts)
	}
)

// moduleSigningPass carries what the key reconcilers changed to the driver
// rebuild that runs after them in the same pass.
type moduleSigningPass struct {
	keyChanged bool
}

// track notes a key fix, or in check-only mode a fix that would happen.
func (p *moduleSigningPass) track(run func(bool) recResult) func(bool) recResult {
	return func(checkOnly bool) recResult {
		result := run(checkOnly)
		if result.status == recFixed || result.status == recWouldFix {
			p.keyChanged = true
		}
		return result
	}
}

// reconcileDriverFallback builds the NVIDIA module for the running kernel when
// it has none, and rebuilds it after a key fix: a build on a key akmods could
// not read failed and left a marker that stops akmods retrying at boot, which
// build-for-kernel's --force clears. It never pins a kernel or changes the
// boot default. A module that exists but is rejected at load is the
// enrollment reconciler's.
func (p *moduleSigningPass) reconcileDriverFallback(checkOnly bool) recResult {
	if !nvidiaPackageInstalled(kmod.AkmodPackage) {
		return okRes(i18n.T("akmod-nvidia is not installed, so there is no NVIDIA module to build"))
	}
	kernel := runningKernel()
	if kernel == "" {
		return warnRes(i18n.T("could not read the running kernel's version"))
	}
	hasModule := nvidiaModuleBuilt(kernel)
	opts := kmod.BuildOptions{Kernel: kernel, Rebuild: p.keyChanged}
	if !opts.Rebuild {
		switch {
		case hasModule:
			return okRes(i18n.T("kernel %s has its NVIDIA module"), kernel)
		case !driverFallbackGPU():
			return okRes(i18n.T("there is no NVIDIA GPU, so kernel %s needs no NVIDIA module"), kernel)
		}
	}
	command := kmod.Advice(opts.Args()...)
	if checkOnly {
		if opts.Rebuild {
			return wouldRes(i18n.T("the NVIDIA module for kernel %s is rebuilt once the module signing key is fixed"), kernel).withFix("%s", command)
		}
		return wouldRes(i18n.T("kernel %s has no NVIDIA module, so this boot fell back to nouveau"), kernel).withFix("%s", command)
	}
	build, detail, err := buildNvidiaForKernel(opts)
	if err != nil {
		return failRes(i18n.T("could not build the NVIDIA module for kernel %s: %v"), kernel, err).withFix("%s", command)
	}
	if build.Built {
		return fixedRes(i18n.T("built the NVIDIA module for kernel %s; reboot to load it"), kernel)
	}
	failure := i18n.Tf("akmods could not build the NVIDIA module for kernel %s", kernel)
	if detail != "" {
		failure += ": " + detail
	}
	if build.NewestWithModule == "" {
		return noteRes("%s; %s", failure, i18n.T("no installed kernel has the NVIDIA driver, so the desktop stays on nouveau")).withFix("%s", command)
	}
	return noteRes("%s", failure).
		withFix(i18n.T("pick kernel %s, the newest with the NVIDIA driver, from the GRUB menu at the next boot"), build.NewestWithModule)
}
