package doctor

import (
	"os"
	"strings"

	"ryoku-cli/internal/kmod"
	i18n "ryoku-i18n"
)

// nvidiaBuild is what `ryoku-host kmod build-for-kernel` reports: whether the
// kernel has a module now, the newest installed kernel that has one, and
// akmods' last word on the build.
type nvidiaBuild struct {
	built            bool
	newestWithModule string
	detail           string
}

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
	driverFallbackGPU    = nvidiaDRMPresent
	buildNvidiaForKernel = func(kernel string, rebuild bool) (nvidiaBuild, error) {
		output, stderr, err := kmodAsRoot(kmodRoot{Progress: os.Stderr}, buildNvidiaArgs(kernel, rebuild)...)
		if err != nil {
			return nvidiaBuild{}, err
		}
		return nvidiaBuild{
			built:            kmodValue(output, "build") == "ok",
			newestWithModule: kmodValue(output, "newest_module_kernel"),
			detail:           lastLine(stderr),
		}, nil
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
	if !doctorPackageInstalled("akmod-nvidia") {
		return okRes(i18n.T("akmod-nvidia is not installed, so there is no NVIDIA module to build"))
	}
	kernel := runningKernel()
	if kernel == "" {
		return warnRes(i18n.T("could not read the running kernel's version"))
	}
	hasModule := nvidiaModuleBuilt(kernel)
	rebuild := p.keyChanged
	if !rebuild {
		switch {
		case hasModule:
			return okRes(i18n.T("kernel %s has its NVIDIA module"), kernel)
		case !driverFallbackGPU():
			return okRes(i18n.T("there is no NVIDIA GPU, so kernel %s needs no NVIDIA module"), kernel)
		}
	}
	command := "sudo ryoku-host kmod " + strings.Join(buildNvidiaArgs(kernel, rebuild), " ")
	if checkOnly {
		if rebuild {
			return wouldRes(i18n.T("the NVIDIA module for kernel %s is rebuilt once the module signing key is fixed"), kernel).withFix("%s", command)
		}
		return wouldRes(i18n.T("kernel %s has no NVIDIA module, so this boot fell back to nouveau"), kernel).withFix("%s", command)
	}
	build, err := buildNvidiaForKernel(kernel, rebuild)
	if err != nil {
		return failRes(i18n.T("could not build the NVIDIA module for kernel %s: %v"), kernel, err).withFix("%s", command)
	}
	if build.built {
		return fixedRes(i18n.T("built the NVIDIA module for kernel %s; reboot to load it"), kernel)
	}
	failure := i18n.Tf("akmods could not build the NVIDIA module for kernel %s", kernel)
	if build.detail != "" {
		failure += ": " + build.detail
	}
	if build.newestWithModule == "" {
		return noteRes("%s; %s", failure, i18n.T("no installed kernel has the NVIDIA driver, so the desktop stays on nouveau")).withFix("%s", command)
	}
	return noteRes("%s", failure).
		withFix(i18n.T("pick kernel %s, the newest with the NVIDIA driver, from the GRUB menu at the next boot"), build.newestWithModule)
}

func buildNvidiaArgs(kernel string, rebuild bool) []string {
	args := []string{"build-for-kernel", "--kernel", kernel}
	if rebuild {
		args = append(args, "--rebuild")
	}
	return args
}

func lastLine(text string) string {
	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}
