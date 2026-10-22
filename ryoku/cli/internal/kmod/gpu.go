package kmod

import (
	"errors"
	"fmt"

	"ryoku-cli/internal/host"
)

// GPUCheckScript is the driver script whose GSP check is the one definition of
// an NVIDIA GPU the open modules drive. The Fedora ryoku package ships it.
const GPUCheckScript = "/usr/libexec/ryoku/nvidia.sh"

var ErrGPUNotSupported = errors.New("this GPU does not get the NVIDIA driver")

// checkGPU asks the driver script's --query mode, which installs nothing.
func (m *Machine) checkGPU() error {
	result := m.cfg.Runner.Run(host.Command{Name: "bash", Args: []string{GPUCheckScript, "--query"}})
	switch result.Code {
	case 0:
		return nil
	case 3:
		return fmt.Errorf("%w: it is older than Turing, so it stays on nouveau", ErrGPUNotSupported)
	case 4:
		return fmt.Errorf("%w: there is no NVIDIA GPU", ErrGPUNotSupported)
	case 5:
		return fmt.Errorf("%w: lspci is missing, so its generation is unknown; install pciutils", ErrGPUNotSupported)
	}
	return fmt.Errorf("%w: %s --query failed with exit status %d", ErrGPUNotSupported, GPUCheckScript, result.Code)
}
