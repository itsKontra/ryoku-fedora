package kmod

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"ryoku-cli/internal/host"
)

// DriverPackage is Ryoku's NVIDIA package. It requires akmod-nvidia, so
// installing it is what installs the driver.
const DriverPackage = "ryoku-nvidia-support"

type DriverOptions struct {
	// Kernel to build for; it defaults to the boot default kernel.
	Kernel string
	// GPUSupported is set by a caller that ran the GPU generation check
	// itself, such as the ISO, whose target root may lack lspci.
	GPUSupported bool
}

type DriverResult struct {
	Kernel string
	Status Status
}

// InstallDriver installs RPM Fusion's NVIDIA driver, built and signed on this
// machine for one kernel. The order matters: akmod-nvidia starts a build the
// moment it installs, so akmods (and with it the akmods group) and the key it
// can read must be in place before it. Otherwise that build is unsigned or
// fails, and akmods would not retry it on its own.
func (m *Machine) InstallDriver(opts DriverOptions) (DriverResult, error) {
	if !opts.GPUSupported {
		if err := m.checkGPU(); err != nil {
			return DriverResult{}, err
		}
	}
	kernel, err := m.targetKernel(opts.Kernel)
	if err != nil {
		return DriverResult{}, err
	}
	release, err := m.fedoraRelease()
	if err != nil {
		return DriverResult{}, err
	}
	if err := m.enableRPMFusion(release); err != nil {
		return DriverResult{}, err
	}
	if err := m.dnf("install", "-y", "akmods", "kernel-devel-"+kernel); err != nil {
		return DriverResult{}, err
	}
	if _, err := m.PrepareKey(); err != nil {
		return DriverResult{}, err
	}
	if err := m.dnf("install", "-y", DriverPackage); err != nil {
		return DriverResult{}, err
	}
	build, err := m.BuildForKernel(BuildOptions{Kernel: kernel})
	if err != nil {
		return DriverResult{}, err
	}
	signed := m.moduleSignedByKey(kernel)
	if signed != nil && build.Built {
		// A module from before the key was in place, or from an earlier
		// akmod-nvidia, counts as current to akmods; only a rebuild re-signs it.
		if _, err := m.BuildForKernel(BuildOptions{Kernel: kernel, Rebuild: true}); err != nil {
			return DriverResult{}, err
		}
		signed = m.moduleSignedByKey(kernel)
	}
	if signed != nil {
		return DriverResult{}, fmt.Errorf("%w; akmods' log is under /var/cache/akmods/nvidia", signed)
	}
	status, err := m.Status()
	if err != nil {
		return DriverResult{}, err
	}
	return DriverResult{Kernel: kernel, Status: status}, nil
}

// fedoraRelease is the machine's Fedora VERSION_ID, which names RPM Fusion's
// release packages.
func (m *Machine) fedoraRelease() (string, error) {
	file, err := os.Open(m.path("/etc/os-release"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if key, value, ok := strings.Cut(scanner.Text(), "="); ok {
			values[key] = strings.Trim(value, `"'`)
		}
	}
	if values["ID"] != "fedora" || values["VERSION_ID"] == "" {
		return "", errors.New("the NVIDIA driver install is for Fedora only")
	}
	return values["VERSION_ID"], nil
}

func (m *Machine) dnf(args ...string) error {
	name := "dnf"
	if m.cfg.Runner.LookPath("dnf5") {
		name = "dnf5"
	}
	result := m.cfg.Runner.Run(host.Command{Name: name, Args: args, Stdout: m.cfg.Log, Stderr: m.cfg.Log})
	if result.Code != 0 {
		return fmt.Errorf("%s %s failed with exit status %d", name, strings.Join(args, " "), result.Code)
	}
	return nil
}

// quiet runs a short command and folds its output into the error.
func (m *Machine) quiet(name string, args ...string) error {
	var output strings.Builder
	result := m.cfg.Runner.Run(host.Command{Name: name, Args: args, Stdout: &output, Stderr: &output})
	if result.Code != 0 {
		detail := strings.TrimSpace(output.String())
		if detail == "" {
			detail = fmt.Sprintf("exit status %d", result.Code)
		}
		return fmt.Errorf("%s: %s", name, detail)
	}
	return nil
}
