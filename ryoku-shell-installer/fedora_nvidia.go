package main

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	i18n "ryoku-i18n"
)

// nvidiaQueryScript is the payload's NVIDIA driver script, read before the
// plan is built: its --query mode is the one definition of a GPU the NVIDIA
// driver supports, and the plan needs the answer before the payload exists.
var nvidiaQueryScript []byte

const nvidiaScriptPath = "system/hardware/drivers/nvidia.sh"

// loadNvidiaQueryScript reads the driver script from the payload checkout, or
// from the ref on GitHub as the edition's package table is. Without it the
// GPU's generation is unknown and the plan leaves the driver off.
func loadNvidiaQueryScript(client httpDoer, ref, payload string) []byte {
	if payload != "" {
		data, err := os.ReadFile(filepath.Join(payload, nvidiaScriptPath))
		if err != nil {
			return nil
		}
		return data
	}
	resp, err := httpStatusOK(client, http.MethodGet, "https://raw.githubusercontent.com/ryoku-dev/ryoku/"+ref+"/"+nvidiaScriptPath)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	return data
}

// gpuGeneration is the driver script's --query answer, in the words it prints.
type gpuGeneration string

const (
	gpuSupported   gpuGeneration = "supported"
	gpuUnsupported gpuGeneration = "unsupported" // older than Turing
	gpuAbsent      gpuGeneration = "absent"
	gpuUnknown     gpuGeneration = "unknown"
)

// queryNvidiaGeneration runs the script's --query, which installs nothing,
// and reads its exit code. The codes are nvidia.sh's --query contract, which
// ryoku-cli's kmod package reads too (a separate Go module): 0 supported,
// 3 unsupported, 4 absent, anything else unknown.
func queryNvidiaGeneration(script []byte) gpuGeneration {
	if len(script) == 0 {
		return gpuUnknown
	}
	dir, err := os.MkdirTemp("", "ryoku-nvidia-query-")
	if err != nil {
		return gpuUnknown
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "nvidia.sh")
	if err := os.WriteFile(path, script, 0o600); err != nil {
		return gpuUnknown
	}
	err = exec.Command("bash", path, "--query").Run()
	if err == nil {
		return gpuSupported
	}
	if exit, ok := err.(*exec.ExitError); ok {
		switch exit.ExitCode() {
		case 3:
			return gpuUnsupported
		case 4:
			return gpuAbsent
		}
	}
	return gpuUnknown
}

// fedoraNvidia is a Fedora machine with an NVIDIA GPU, the one that gets the
// Fedora NVIDIA driver item.
func (f *facts) fedoraNvidia() bool {
	return f.hasNvidia && f.distro != nil && f.distro.id == "fedora"
}

// detectFedoraNvidia fills in what the Fedora NVIDIA driver item needs: the
// GPU's generation and, when ryoku-host is already installed, the enrollment
// password it owns.
func (f *facts) detectFedoraNvidia() {
	if !f.fedoraNvidia() {
		return
	}
	f.nvidiaGeneration = queryNvidiaGeneration(nvidiaQueryScript)
	if !f.secureBoot {
		return
	}
	host := filepath.Join(ryokuSystemBin, "ryoku-host")
	if out, err := exec.Command(host, "kmod", "status").Output(); err == nil {
		f.enrollmentPassword = parseKeyValues(string(out))["password"]
	}
}

// fedoraNvidiaItem offers RPM Fusion's driver, built and signed on this
// machine. Secure Boot never locks it: the machine's own key is enrolled.
func fedoraNvidiaItem(f *facts, p *plan) planItem {
	item := planItem{label: "NVIDIA driver", on: &p.nvidia}
	switch f.nvidiaGeneration {
	case gpuSupported:
		item.detail = i18n.T("installs RPM Fusion's NVIDIA driver, built and signed on this machine; toggle off to stay on nouveau")
	case gpuUnsupported:
		item.detail = i18n.T("held off: this GPU is older than Turing, which the NVIDIA driver Ryoku installs needs; it stays on nouveau")
		item.locked = true
		return item
	default:
		item.detail = i18n.T("held off: the GPU's generation could not be checked (is lspci installed?); it stays on nouveau, and ryoku doctor offers the driver once it can check")
		item.locked = true
		return item
	}
	if f.secureBoot {
		if f.enrollmentPassword != "" {
			item.detail += "; " + i18n.Tf("Secure Boot is on: at the next reboot MokManager asks you to approve this machine's module signing key with the password %s", f.enrollmentPassword)
		} else {
			item.detail += "; " + i18n.T("Secure Boot is on: at the next reboot MokManager asks you to approve this machine's module signing key with a password the installer shows when it finishes")
		}
	}
	return item
}

// installFedoraNvidia runs between preparing the key and queueing its
// enrollment, so the enrollment covers the key the driver was signed with.
// A failed driver warns: the machine boots on nouveau and doctor offers it.
func installFedoraNvidia(e *engine, host string) bool {
	if !e.f.hasNvidia {
		return false
	}
	if !e.p.nvidia {
		switch e.f.nvidiaGeneration {
		case gpuSupported:
			e.say(i18n.T("keeping nouveau for the NVIDIA GPU (the NVIDIA driver item is off)"))
		case gpuUnsupported:
			e.say(i18n.T("this NVIDIA GPU is older than Turing; it stays on nouveau"))
		default:
			e.say(i18n.T("the NVIDIA GPU's generation could not be checked; it stays on nouveau, and ryoku doctor can install the driver later"))
		}
		return false
	}
	e.say(i18n.T("installing the NVIDIA driver from RPM Fusion; building it for this machine takes several minutes"))
	if err := e.sudo(host, "kmod", "install-driver"); err != nil {
		e.say(i18n.T("warning: the NVIDIA driver did not install; the machine boots on nouveau and `ryoku doctor` offers the driver again (continuing)"))
		return false
	}
	return true
}

// recordFedoraNvidiaChoice tells doctor whether this NVIDIA machine chose the
// driver or nouveau, so it never pushes the driver on a user who opted out.
// Only a supported GPU's item is the user's to turn off; a locked item is no
// choice, so nothing is recorded and doctor stays free to offer the driver.
func recordFedoraNvidiaChoice(e *engine, host string) {
	var choice string
	switch {
	case !e.f.hasNvidia:
		return
	case e.p.nvidia:
		choice = "driver"
	case e.f.nvidiaGeneration == gpuSupported:
		choice = "nouveau"
	default:
		return
	}
	if err := e.sudo(host, "kmod", "record-choice", choice); err != nil {
		e.say(i18n.T("warning: could not record the NVIDIA choice for ryoku doctor (continuing)"))
	}
}
