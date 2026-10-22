package kmod

import (
	"fmt"
	"io"

	"ryoku-cli/internal/host"
)

const Usage = `Usage: ryoku-host kmod <verb> [--root DIR]

Kernel modules on Fedora. Every verb prints key=value lines, the status last.
  status             the module signing key, Secure Boot, the enrollment, the password
  prepare-key        create the module signing key if missing, then fix its mode and owner
  queue-enrollment   queue the key's enrollment when Secure Boot is enforcing

Exit codes: 0 success, 1 failure, 2 usage.
`

// Execute runs `ryoku-host kmod <verb>`. Its key=value output is read by the
// installers and the ISO, so keys and values are stable and never translated.
func Execute(args []string, cfg Config, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, Usage)
		return host.ExitUsage
	}
	verb := args[0]
	if verb == "--help" || verb == "-h" || verb == "help" {
		fmt.Fprint(stdout, Usage)
		return host.ExitOK
	}
	switch {
	case len(args) == 3 && args[1] == "--root":
		cfg.Root = args[2]
	case len(args) != 1:
		fmt.Fprint(stderr, Usage)
		return host.ExitUsage
	}
	machine := New(cfg)
	switch verb {
	case "status":
		status, err := machine.Status()
		if err != nil {
			return fail(stderr, err)
		}
		writeStatus(stdout, status)
	case "prepare-key":
		result, err := machine.PrepareKey()
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "action=%s\n", result.Action)
		writeStatus(stdout, result.Status)
	case "queue-enrollment":
		result, err := machine.QueueEnrollment()
		if err != nil {
			return fail(stderr, err)
		}
		action := "none"
		if result.Queued {
			action = "queued"
		}
		fmt.Fprintf(stdout, "action=%s\n", action)
		writeStatus(stdout, result.Status)
	default:
		fmt.Fprint(stderr, Usage)
		return host.ExitUsage
	}
	return host.ExitOK
}

func writeStatus(w io.Writer, status Status) {
	key := status.Key
	present, mode, ok := "absent", "", "no"
	if key.Present {
		present, mode = "present", fmt.Sprintf("%04o", key.Mode)
	}
	if key.OK() {
		ok = "yes"
	}
	fmt.Fprintf(w, "key=%s\n", present)
	fmt.Fprintf(w, "key_mode=%s\n", mode)
	fmt.Fprintf(w, "key_owner=%s\n", key.Owner)
	fmt.Fprintf(w, "key_expected_mode=%04o\n", key.WantMode)
	fmt.Fprintf(w, "key_expected_owner=%s\n", key.WantOwner)
	fmt.Fprintf(w, "key_ok=%s\n", ok)
	fmt.Fprintf(w, "secure_boot=%s\n", status.SecureBoot)
	fmt.Fprintf(w, "enrollment=%s\n", status.Enrollment)
	fmt.Fprintf(w, "password=%s\n", status.Password)
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "ryoku-host: %v\n", err)
	return host.ExitFailure
}
