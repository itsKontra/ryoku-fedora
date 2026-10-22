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
                     and point akmods' key paths at it
  queue-enrollment   queue the key's enrollment when Secure Boot is enforcing
  record-choice driver|nouveau
                     record the NVIDIA choice the installer made
  install-driver [--kernel VERSION] [--gpu-supported]
                     enable RPM Fusion and install the NVIDIA driver, built and signed
                     for VERSION (default: the boot default kernel); --gpu-supported
                     says the caller already ran the GPU generation check
  build-for-kernel [--kernel VERSION] [--rebuild]
                     run akmods for VERSION, retrying a build that failed before;
                     --rebuild also rebuilds a module that is already there

Exit codes: 0 success, 1 failure, 2 usage. build-for-kernel exits 0 once
akmods ran and reports build=ok|failed.
`

// invocation is one parsed `ryoku-host kmod` command line.
type invocation struct {
	verb         string
	choice       Choice
	kernel       string
	rebuild      bool
	gpuSupported bool
}

// Execute runs `ryoku-host kmod <verb>`. Its key=value output is read by the
// installers and the ISO, so keys and values are stable and never translated.
func Execute(args []string, cfg Config, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Fprint(stdout, Usage)
		return host.ExitOK
	}
	inv, ok := parse(args, &cfg)
	if !ok {
		fmt.Fprint(stderr, Usage)
		return host.ExitUsage
	}
	if cfg.Log == nil {
		cfg.Log = stderr
	}
	machine := New(cfg)
	var (
		status Status
		err    error
	)
	switch inv.verb {
	case "status":
		status, err = machine.Status()
	case "prepare-key":
		var result KeyResult
		if result, err = machine.PrepareKey(); err == nil {
			fmt.Fprintf(stdout, "action=%s\n", result.Action)
			status = result.Status
		}
	case "queue-enrollment":
		var result EnrollmentResult
		if result, err = machine.QueueEnrollment(); err == nil {
			action := "none"
			if result.Queued {
				action = "queued"
			}
			fmt.Fprintf(stdout, "action=%s\n", action)
			status = result.Status
		}
	case "record-choice":
		if status, err = machine.RecordChoice(inv.choice); err == nil {
			fmt.Fprintf(stdout, "nvidia_choice=%s\n", inv.choice)
		}
	case "install-driver":
		var result DriverResult
		if result, err = machine.InstallDriver(DriverOptions{Kernel: inv.kernel, GPUSupported: inv.gpuSupported}); err == nil {
			fmt.Fprintf(stdout, "action=installed\nkernel=%s\n", result.Kernel)
			status = result.Status
		}
	case "build-for-kernel":
		var result BuildResult
		if result, err = machine.BuildForKernel(BuildOptions{Kernel: inv.kernel, Rebuild: inv.rebuild}); err == nil {
			build := "failed"
			if result.Built {
				build = "ok"
			}
			fmt.Fprintf(stdout, "build=%s\nkernel=%s\nnewest_module_kernel=%s\n", build, result.Kernel, result.NewestWithModule)
			status = result.Status
		}
	}
	if err != nil {
		return fail(stderr, err)
	}
	writeStatus(stdout, status)
	return host.ExitOK
}

// parse reads the verb, its own arguments and the shared --root.
func parse(args []string, cfg *Config) (invocation, bool) {
	if len(args) == 0 {
		return invocation{}, false
	}
	inv := invocation{verb: args[0]}
	var positional []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--root" && i+1 < len(args):
			i++
			cfg.Root = args[i]
		case arg == "--kernel" && i+1 < len(args) && args[i+1] != "" && (inv.verb == "install-driver" || inv.verb == "build-for-kernel"):
			i++
			inv.kernel = args[i]
		case arg == "--rebuild" && inv.verb == "build-for-kernel":
			inv.rebuild = true
		case arg == "--gpu-supported" && inv.verb == "install-driver":
			inv.gpuSupported = true
		case len(arg) > 0 && arg[0] != '-':
			positional = append(positional, arg)
		default:
			return inv, false
		}
	}
	switch inv.verb {
	case "status", "prepare-key", "queue-enrollment", "install-driver", "build-for-kernel":
		return inv, len(positional) == 0
	case "record-choice":
		if len(positional) != 1 {
			return inv, false
		}
		inv.choice = Choice(positional[0])
		return inv, inv.choice == ChoiceDriver || inv.choice == ChoiceNouveau
	}
	return inv, false
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
	fmt.Fprintf(w, "akmods_links=%s\n", key.Links)
	fmt.Fprintf(w, "nvidia_choice=%s\n", status.Choice)
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "ryoku-host: %v\n", err)
	return host.ExitFailure
}
