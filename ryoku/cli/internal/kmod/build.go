package kmod

import (
	"errors"

	"ryoku-cli/internal/host"
)

type BuildOptions struct {
	// Kernel defaults to the boot default kernel.
	Kernel string
	// Rebuild also rebuilds a module that already exists, for one signed
	// before the module signing key changed.
	Rebuild bool
}

// BuildResult says whether Kernel has an NVIDIA module after the build, and
// which installed kernel is the newest with one: the way back from a driver
// fallback when the build failed.
type BuildResult struct {
	Kernel           string
	Built            bool
	NewestWithModule string
	Status           Status
}

// BuildForKernel runs akmods for one kernel in the foreground. --force retries
// a build an earlier failure marked to skip. akmods waits on its own lock, so
// a build akmod-nvidia started in the background finishes first. akmods exits
// 0 even when the build fails, so the module itself is the outcome.
func (m *Machine) BuildForKernel(opts BuildOptions) (BuildResult, error) {
	kernel, err := m.targetKernel(opts.Kernel)
	if err != nil {
		return BuildResult{}, err
	}
	if !m.cfg.Runner.LookPath("akmods") {
		return BuildResult{}, errors.New("akmods is not installed")
	}
	if gid, ok := m.accounts().gids[akmodsGroup]; ok {
		if err := m.ensureAkmodsRunDir(gid); err != nil {
			return BuildResult{}, err
		}
	}
	args := []string{"--force"}
	if opts.Rebuild {
		args = append(args, "--rebuild")
	}
	args = append(args, "--kernels", kernel, "--akmod", "nvidia")
	m.cfg.Runner.Run(host.Command{Name: "akmods", Args: args, Stdout: m.cfg.Log, Stderr: m.cfg.Log})
	status, err := m.Status()
	if err != nil {
		return BuildResult{}, err
	}
	return BuildResult{
		Kernel:           kernel,
		Built:            m.HasDriverModule(kernel),
		NewestWithModule: m.newestKernelWithModule(),
		Status:           status,
	}, nil
}
