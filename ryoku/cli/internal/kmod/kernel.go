package kmod

import (
	"fmt"
	"os"
	"strings"

	"ryoku-cli/internal/host"
)

// targetKernel is the kernel given, or else the boot default, which is the
// kernel akmods' own install-time build targets. Either must be installed.
func (m *Machine) targetKernel(kernel string) (string, error) {
	if kernel == "" {
		result := m.cfg.Runner.Run(host.Command{Name: "grubby", Args: []string{"--default-kernel"}})
		if path := strings.TrimSpace(result.Output); result.Code == 0 && strings.HasPrefix(path, "/boot/vmlinuz-") {
			kernel = strings.TrimPrefix(path, "/boot/vmlinuz-")
		} else if kernels := m.installedKernels(); len(kernels) > 0 {
			kernel = kernels[0]
		}
	}
	if kernel == "" || strings.ContainsRune(kernel, '/') || !m.exists("/usr/lib/modules/"+kernel+"/vmlinuz") {
		return "", fmt.Errorf("kernel %q is not installed", kernel)
	}
	return kernel, nil
}

func (m *Machine) installedKernels() []string {
	return host.InstalledKernels(m.path("/usr/lib/modules"))
}

// HasDriverModule says whether kernel has an NVIDIA module installed. modinfo
// needs no root, so doctor asks as the desktop user.
func (m *Machine) HasDriverModule(kernel string) bool {
	result := m.cfg.Runner.Run(host.Command{Name: "modinfo", Args: []string{"-k", kernel, "-F", "filename", "nvidia"}})
	return result.Code == 0 && strings.TrimSpace(result.Output) != ""
}

// newestKernelWithModule is the kernel to pick from the boot menu when the
// default one has no NVIDIA module, or "" when no kernel has one.
func (m *Machine) newestKernelWithModule() string {
	for _, kernel := range m.installedKernels() {
		if m.HasDriverModule(kernel) {
			return kernel
		}
	}
	return ""
}

// moduleSignedByKey compares the module's signer serial with the module
// signing key's certificate, so a module signed by any other key, even one
// also called "DKMS module signing key", fails.
func (m *Machine) moduleSignedByKey(kernel string) error {
	if !m.HasDriverModule(kernel) {
		return fmt.Errorf("there is no NVIDIA module for kernel %s", kernel)
	}
	sigKey := m.cfg.Runner.Run(host.Command{Name: "modinfo", Args: []string{"-k", kernel, "-F", "sig_key", "nvidia"}})
	signer := serialHex(sigKey.Output)
	if sigKey.Code != 0 || signer == "" {
		return fmt.Errorf("the NVIDIA module for kernel %s is not signed", kernel)
	}
	serial := m.cfg.Runner.Run(host.Command{Name: "openssl", Args: []string{"x509", "-inform", "DER", "-in", m.path(CertPath), "-noout", "-serial"}})
	want := serialHex(strings.TrimPrefix(strings.TrimSpace(serial.Output), "serial="))
	if serial.Code != 0 || want == "" {
		return fmt.Errorf("cannot read the serial of %s", CertPath)
	}
	if signer != want {
		return fmt.Errorf("the NVIDIA module for kernel %s is signed by key %s, not by the module signing key %s", kernel, signer, want)
	}
	return nil
}

// serialHex normalises modinfo's "00:5B:12" and openssl's "5B12" alike.
func serialHex(value string) string {
	hex := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), ":", ""))
	return strings.TrimLeft(hex, "0")
}

func (m *Machine) ensureAkmodsRunDir(akmodsGID int) error {
	dir := m.path("/run/akmods")
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o770); err != nil {
		return err
	}
	return m.cfg.Owners.Chown(dir, 0, akmodsGID)
}
