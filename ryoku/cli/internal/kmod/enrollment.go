package kmod

import (
	"strings"

	"ryoku-cli/internal/host"
)

// Enrollment is where the module signing key stands with MokManager.
type Enrollment string

const (
	EnrollmentEnrolled  Enrollment = "enrolled"
	EnrollmentQueued    Enrollment = "queued"
	EnrollmentNotQueued Enrollment = "not_queued"
	EnrollmentUnknown   Enrollment = "unknown"
)

// enrollment asks mokutil 0.7.2's --test-key, which checks db, MokListRT and
// the pending MokNew request in one call: exit 0 means none of them holds the
// key, exit 1 prints which one does.
func (m *Machine) enrollment(key KeyState) Enrollment {
	if !key.Present || !m.exists(CertPath) || !m.cfg.Runner.LookPath("mokutil") {
		return EnrollmentUnknown
	}
	result := m.cfg.Runner.Run(host.Command{Name: "mokutil", Args: []string{"--test-key", m.path(CertPath)}})
	switch {
	case result.Code == 0 && strings.Contains(result.Output, " is not enrolled"):
		return EnrollmentNotQueued
	case result.Code == 1 && (strings.Contains(result.Output, " is already enrolled") || strings.Contains(result.Output, " is already in db")):
		return EnrollmentEnrolled
	case result.Code == 1 && strings.Contains(result.Output, " is already in the enrollment request"):
		return EnrollmentQueued
	default:
		return EnrollmentUnknown
	}
}
