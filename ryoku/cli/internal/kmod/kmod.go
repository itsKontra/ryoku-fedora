// Package kmod owns the out-of-tree kernel modules Ryoku builds on Fedora: the
// machine's one module signing key (DKMS's), its enrollment with MokManager,
// and the NVIDIA driver signed with that key. The installers, doctor and the
// updater all go through it, so they cannot disagree about any of it.
package kmod

import (
	"path/filepath"

	"ryoku-cli/internal/host"
)

// EnrollmentPassword is the fixed one-time password MokManager asks for to
// approve an enrollment. It proves someone is at the console, so it is public
// and the same on every machine. It is defined here and nowhere else.
const EnrollmentPassword = "enroll"

// DKMS's key pair, which every module on the machine is signed with.
const (
	KeyPath  = "/var/lib/dkms/mok.key"
	CertPath = "/var/lib/dkms/mok.pub"
)

const efiGlobalGUID = "8be4df61-93ca-11d2-aa0d-00e098032b8c"

// Config points a Machine at a root filesystem. Commands such as openssl and
// mokutil run through Runner; ownership goes through Owners.
type Config struct {
	Root   string
	Runner host.Runner
	Owners Owners
}

// Machine is one root filesystem whose module signing key and enrollment are
// inspected or changed.
type Machine struct {
	cfg Config
}

func New(cfg Config) *Machine {
	if cfg.Root == "" {
		cfg.Root = "/"
	}
	if cfg.Runner == nil {
		cfg.Runner = host.ExecRunner{}
	}
	if cfg.Owners == nil {
		cfg.Owners = osOwners{}
	}
	return &Machine{cfg: cfg}
}

// path maps an absolute path on the machine to the filesystem it lives in.
func (m *Machine) path(machinePath string) string {
	return filepath.Join(m.cfg.Root, machinePath)
}
