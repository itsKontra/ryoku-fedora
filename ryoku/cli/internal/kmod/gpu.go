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

// GPU is the generation check's answer, in the words its --query mode prints.
type GPU string

const (
	GPUSupported   GPU = "supported"
	GPUUnsupported GPU = "unsupported"
	GPUAbsent      GPU = "absent"
	GPUUnknown     GPU = "unknown"
)

// GPU asks the driver script's --query mode, which installs nothing and needs
// no root. A script that is missing or fails answers unknown.
func (m *Machine) GPU() GPU {
	gpu, _ := m.queryGPU()
	return gpu
}

func (m *Machine) queryGPU() (GPU, int) {
	result := m.cfg.Runner.Run(host.Command{Name: "bash", Args: []string{GPUCheckScript, "--query"}})
	switch result.Code {
	case 0:
		return GPUSupported, result.Code
	case 3:
		return GPUUnsupported, result.Code
	case 4:
		return GPUAbsent, result.Code
	}
	return GPUUnknown, result.Code
}

func (m *Machine) checkGPU() error {
	gpu, code := m.queryGPU()
	switch {
	case gpu == GPUSupported:
		return nil
	case gpu == GPUUnsupported:
		return fmt.Errorf("%w: it is older than Turing, so it stays on nouveau", ErrGPUNotSupported)
	case gpu == GPUAbsent:
		return fmt.Errorf("%w: there is no NVIDIA GPU", ErrGPUNotSupported)
	case code == 5:
		return fmt.Errorf("%w: lspci is missing, so its generation is unknown; install pciutils", ErrGPUNotSupported)
	}
	return fmt.Errorf("%w: %s --query failed with exit status %d", ErrGPUNotSupported, GPUCheckScript, code)
}
