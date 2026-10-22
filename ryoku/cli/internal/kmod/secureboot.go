package kmod

import "os"

type SecureBoot string

const (
	SecureBootEnforcing   SecureBoot = "enforcing"
	SecureBootOff         SecureBoot = "off"
	SecureBootUnsupported SecureBoot = "unsupported"
)

// secureBoot applies the shell installer's rule: SecureBoot set is not
// enough, because in SetupMode the firmware accepts any key and enforces
// nothing. Without the variable the machine booted without UEFI.
func (m *Machine) secureBoot() SecureBoot {
	dir := m.path("/sys/firmware/efi/efivars")
	secureBoot, err := os.ReadFile(dir + "/SecureBoot-" + efiGlobalGUID)
	if err != nil {
		return SecureBootUnsupported
	}
	setupMode, _ := os.ReadFile(dir + "/SetupMode-" + efiGlobalGUID)
	if efivarOn(secureBoot) && !efivarOn(setupMode) {
		return SecureBootEnforcing
	}
	return SecureBootOff
}

// efivarOn reads an efivarfs boolean: a 4-byte attribute header, then the
// value byte.
func efivarOn(b []byte) bool {
	return len(b) >= 5 && b[4] == 1
}
