// Package host answers "how does this machine do it" for Ryoku's runtime
// components: services, packages, power, time zone, keymap and session start.
// The shell, Hub, scripts and compositor autostarts call the ryoku-host binary
// instead of spelling systemctl or dnf themselves, so the host's commands live
// in one place behind one exit-code contract.
//
// Fedora runs one backend, systemd and dnf/rpm. The package queries reuse the
// CLI's own rpm primitives (internal/sys) and the wm seam's repo availability
// check, so there is no second dnf implementation here. ryoku-host never
// escalates itself: a verb that needs root is run through sudo or pkexec by its
// caller, and timedatectl and localectl ask polkit on their own.
package host

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"ryoku-cli/internal/sys"

	wm "ryoku-wm"
)

// The exit-code contract every verb answers with.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitFalse       = 3
	ExitAbsent      = 4
	ExitNotProvided = 5
)

// Runner executes host commands, wired to the caller's stdio, and returns the
// exit status.
type Runner interface {
	Run(name string, args ...string) int
}

// ExecRunner runs commands on the real host.
type ExecRunner struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

func (r ExecRunner) Run(name string, args ...string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r.Stdin, r.Stdout, r.Stderr
	return exitCode(cmd.Run())
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	return ExitFailure
}

// Packages is the read side of the package manager. The defaults are the
// CLI's rpm primitives and the wm seam's cached repo query.
type Packages struct {
	Manager   func() string
	Installed func(string) bool
	Available func(string) bool
	Version   func(string) string
	Owner     func(string) string
}

// Repo is the RyokuCOPR channel read and write, from internal/sys.
type Repo struct {
	Server     func() string
	SetChannel func(string) error
}

type Config struct {
	Runner            Runner
	Stdout, Stderr    io.Writer
	Packages          Packages
	Repo              Repo
	SystemdRuntimeDir string
}

type App struct {
	cfg Config
}

func New(cfg Config) *App {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Runner == nil {
		cfg.Runner = ExecRunner{Stdin: os.Stdin, Stdout: cfg.Stdout, Stderr: cfg.Stderr}
	}
	p := &cfg.Packages
	if p.Manager == nil {
		p.Manager = sys.RPMManager
	}
	if p.Installed == nil {
		p.Installed = sys.PkgInstalled
	}
	if p.Available == nil {
		p.Available = wm.PackageAvailable
	}
	if p.Version == nil {
		p.Version = sys.PkgVersion
	}
	if p.Owner == nil {
		p.Owner = sys.PkgOwner
	}
	if cfg.Repo.Server == nil {
		cfg.Repo.Server = sys.RyokuServer
	}
	if cfg.Repo.SetChannel == nil {
		cfg.Repo.SetChannel = sys.SetPackagedChannel
	}
	if cfg.SystemdRuntimeDir == "" {
		cfg.SystemdRuntimeDir = "/run/systemd/system"
	}
	return &App{cfg: cfg}
}

// Default is the production seam on the current process and host.
func Default() *App { return New(Config{}) }

func (a *App) run(name string, args ...string) int { return a.cfg.Runner.Run(name, args...) }

func (a *App) failf(format string, args ...any) int {
	fmt.Fprintf(a.cfg.Stderr, "ryoku-host: "+format+"\n", args...)
	return ExitFailure
}

// commandExit folds a command's status into the contract: success or failure.
func commandExit(code int) int {
	if code == 0 {
		return ExitOK
	}
	return ExitFailure
}
