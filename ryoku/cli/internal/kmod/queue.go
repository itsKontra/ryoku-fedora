package kmod

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ryoku-cli/internal/host"
)

type EnrollmentResult struct {
	Queued bool
	Status Status
}

// QueueEnrollment asks MokManager to trust the module signing key from the
// next boot on, but only when Secure Boot is enforcing and the key is neither
// enrolled nor already queued. The timeout goes first: a request without it
// would show MokManager for ten seconds and then boot past it.
func (m *Machine) QueueEnrollment() (EnrollmentResult, error) {
	status, err := m.Status()
	if err != nil {
		return EnrollmentResult{}, err
	}
	if status.SecureBoot != SecureBootEnforcing {
		return EnrollmentResult{Status: status}, nil
	}
	switch {
	case !status.Key.Present:
		return EnrollmentResult{Status: status}, errors.New("there is no module signing key yet; run ryoku-host kmod prepare-key first")
	case status.Enrollment == EnrollmentEnrolled || status.Enrollment == EnrollmentQueued:
		return EnrollmentResult{Status: status}, nil
	case status.Enrollment != EnrollmentNotQueued:
		return EnrollmentResult{Status: status}, errors.New("mokutil cannot tell whether the module signing key is enrolled")
	}
	if err := m.mokutil(nil, "--timeout", "-1"); err != nil {
		return EnrollmentResult{Status: status}, fmt.Errorf("set the MokManager timeout: %w", err)
	}
	password := strings.NewReader(EnrollmentPassword + "\n" + EnrollmentPassword + "\n")
	if err := m.mokutil(password, "--import", m.path(CertPath)); err != nil {
		return EnrollmentResult{Status: status}, fmt.Errorf("queue the enrollment: %w", err)
	}
	status, err = m.Status()
	if err != nil {
		return EnrollmentResult{}, err
	}
	return EnrollmentResult{Queued: true, Status: status}, nil
}

func (m *Machine) mokutil(stdin io.Reader, args ...string) error {
	var output strings.Builder
	result := m.cfg.Runner.Run(host.Command{Name: "mokutil", Args: args, Stdin: stdin, Stdout: &output, Stderr: &output})
	if result.Code != 0 {
		detail := strings.TrimSpace(output.String())
		if detail == "" {
			detail = fmt.Sprintf("exit status %d", result.Code)
		}
		return fmt.Errorf("mokutil %s: %s", args[0], detail)
	}
	return nil
}
