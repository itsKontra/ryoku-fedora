package kmod

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/host"
)

// KeyAction says what PrepareKey did to the module signing key.
type KeyAction string

const (
	KeyCreated   KeyAction = "created"
	KeyFixed     KeyAction = "fixed"
	KeyUnchanged KeyAction = "unchanged"
)

type KeyResult struct {
	Action KeyAction
	Status Status
}

// PrepareKey makes sure the machine has its module signing key in the
// expected state. It creates DKMS's pair only when either half is missing, as
// DKMS itself would, and otherwise only corrects the key's mode and owner and,
// where akmods is installed, its links to the pair.
func (m *Machine) PrepareKey() (KeyResult, error) {
	action := KeyUnchanged
	if !m.exists(KeyPath) || !m.exists(CertPath) {
		if err := m.generateKeyPair(); err != nil {
			return KeyResult{}, err
		}
		action = KeyCreated
	}
	key, err := m.keyState()
	if err != nil {
		return KeyResult{}, err
	}
	if !key.OK() {
		// Narrow the mode before the owner changes, so neither the old nor the
		// new group can read the key through a mode meant for the other.
		if err := os.Chmod(m.path(KeyPath), key.Mode&key.WantMode); err != nil {
			return KeyResult{}, fmt.Errorf("set the mode of %s: %w", KeyPath, err)
		}
		if err := m.cfg.Owners.Chown(m.path(KeyPath), key.wantUID, key.wantGID); err != nil {
			return KeyResult{}, fmt.Errorf("set the owner of %s: %w", KeyPath, err)
		}
		if err := os.Chmod(m.path(KeyPath), key.WantMode); err != nil {
			return KeyResult{}, fmt.Errorf("set the mode of %s: %w", KeyPath, err)
		}
		if action == KeyUnchanged {
			action = KeyFixed
		}
	}
	if key.akmods {
		relinked, err := m.pointAkmodsAtKey(key.wantGID)
		if err != nil {
			return KeyResult{}, err
		}
		if relinked && action == KeyUnchanged {
			action = KeyFixed
		}
	}
	status, err := m.Status()
	if err != nil {
		return KeyResult{}, err
	}
	return KeyResult{Action: action, Status: status}, nil
}

// generateKeyPair runs DKMS's own openssl invocation into a private scratch
// directory beside the key, then moves both halves into place, so the private
// key is never readable by others and a failed run leaves no half pair.
func (m *Machine) generateKeyPair() error {
	dir := filepath.Dir(m.path(KeyPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	scratch, err := os.MkdirTemp(dir, ".mok-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	key := filepath.Join(scratch, "mok.key")
	cert := filepath.Join(scratch, "mok.pub")
	var output strings.Builder
	result := m.cfg.Runner.Run(host.Command{
		Name: "openssl",
		Args: []string{
			"req", "-new", "-x509", "-nodes", "-days", "36500",
			"-subj", "/CN=DKMS module signing key",
			"-newkey", "rsa:2048", "-keyout", key,
			"-addext", "extendedKeyUsage=codeSigning",
			"-outform", "DER", "-out", cert,
		},
		Stdout: &output,
		Stderr: &output,
	})
	if result.Code != 0 {
		return fmt.Errorf("openssl could not create the module signing key: %s", strings.TrimSpace(output.String()))
	}
	if err := os.Chmod(cert, 0o644); err != nil {
		return err
	}
	if err := m.cfg.Owners.Chown(cert, 0, 0); err != nil {
		return err
	}
	if err := os.Rename(key, m.path(KeyPath)); err != nil {
		return err
	}
	return os.Rename(cert, m.path(CertPath))
}

func (m *Machine) exists(machinePath string) bool {
	_, err := os.Stat(m.path(machinePath))
	return !errors.Is(err, fs.ErrNotExist)
}
