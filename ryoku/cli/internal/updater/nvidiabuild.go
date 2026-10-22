package updater

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"ryoku-cli/internal/host"
	i18n "ryoku-i18n"
)

// The system lane on Fedora can install a kernel whose NVIDIA module akmods
// has not built yet; that kernel then boots into a driver fallback. Waiting
// for the build here, instead of leaving it to akmods' background hook and the
// build at boot, lets the user know before rebooting whether the next boot has
// the driver. The kernel itself is never held back.

// nvidiaBuild is what `ryoku-host kmod build-for-kernel` reports: whether the
// kernel has an NVIDIA module now, and the newest kernel that has one.
type nvidiaBuild struct {
	Kernel   string
	Built    bool
	Fallback string
}

var (
	nvidiaAkmodInstalled = func() bool {
		return host.Default().Package([]string{"installed", "akmod-nvidia"}) == host.ExitOK
	}
	nvidiaBuildForKernel = runNvidiaBuildForKernel
)

// awaitNvidiaBuild builds the NVIDIA module for the kernel the system lane
// just installed and returns the line telling the user whether the next boot
// has the driver. It returns "" when there is no new kernel or no akmod-nvidia.
// kernelBefore is the newest kernel before the upgrade.
func awaitNvidiaBuild(kernelBefore string) string {
	kernel := newestKernel(modulesDir)
	if kernel == "" || kernel == kernelBefore || (kernelBefore != "" && compareVersions(kernel, kernelBefore) < 0) {
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
	case result.Fallback != "":
		return i18n.Tf("NVIDIA driver build failed for kernel %s: the next boot uses nouveau, kernel %s still has the driver", kernel, result.Fallback)
	default:
		return i18n.Tf("NVIDIA driver build failed for kernel %s: the next boot uses nouveau, and no installed kernel has the driver", kernel)
	}
}

// runNvidiaBuildForKernel runs the build as root, sleep-inhibited, in the
// foreground. akmods' own output goes to the raw log with the DNF transaction
// it follows; only the key=value report on stdout is read.
func runNvidiaBuildForKernel(kernel string) (nvidiaBuild, error) {
	argv := []string{"ryoku-host", "kmod", "build-for-kernel", "--kernel", kernel}
	if os.Geteuid() != 0 {
		argv = append([]string{"sudo"}, argv...)
	}
	full := inhibited("NVIDIA driver build", argv)
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(full[0], full[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &stdout, io.MultiWriter(os.Stdout, &stderr)
	if err := cmd.Run(); err != nil {
		if reason := lastLine(stderr.String()); reason != "" {
			return nvidiaBuild{}, fmt.Errorf("%s", reason)
		}
		return nvidiaBuild{}, err
	}
	return parseNvidiaBuild(stdout.String())
}

// parseNvidiaBuild reads the build lines that lead the report; the status
// block after them repeats no build key.
func parseNvidiaBuild(out string) (nvidiaBuild, error) {
	values := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if _, seen := values[key]; ok && !seen {
			values[key] = value
		}
	}
	build, ok := values["build"]
	if !ok || (build != "ok" && build != "failed") {
		return nvidiaBuild{}, fmt.Errorf("unexpected build-for-kernel output %q", strings.TrimSpace(out))
	}
	return nvidiaBuild{Kernel: values["kernel"], Built: build == "ok", Fallback: values["newest_module_kernel"]}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
