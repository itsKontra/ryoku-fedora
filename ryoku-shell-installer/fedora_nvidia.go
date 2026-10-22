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

// queryNvidiaGeneration runs the script's --query, which installs nothing,
// and reads its exit code: supported, unsupported (older than Turing), absent,
// or unknown.
func queryNvidiaGeneration(script []byte) string {
	if len(script) == 0 {
		return "unknown"
	}
	dir, err := os.MkdirTemp("", "ryoku-nvidia-query-")
	if err != nil {
		return "unknown"
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "nvidia.sh")
	if err := os.WriteFile(path, script, 0o600); err != nil {
		return "unknown"
	}
	err = exec.Command("bash", path, "--query").Run()
	if err == nil {
		return "supported"
	}
	if exit, ok := err.(*exec.ExitError); ok {
		switch exit.ExitCode() {
		case 3:
			return "unsupported"
		case 4:
			return "absent"
		}
	}
	return "unknown"
}

// detectFedoraNvidia fills in what the Fedora NVIDIA driver item needs: the
// GPU's generation and, when ryoku-host is already installed, the enrollment
// password it owns.
func (f *facts) detectFedoraNvidia() {
	if !f.hasNvidia || f.distro == nil || f.distro.id != "fedora" {
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
	case "supported":
		item.detail = i18n.T("installs RPM Fusion's NVIDIA driver, built and signed on this machine; toggle off to stay on nouveau")
	case "unsupported":
		item.detail = i18n.T("held off: this GPU is older than Turing, which the NVIDIA driver Ryoku installs needs; it stays on nouveau")
		item.locked = true
		return item
	default:
		item.detail = i18n.T("off: the GPU's generation could not be checked (is lspci installed?); toggle on if it is Turing or newer, or run ryoku doctor later")
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
		if e.f.nvidiaGeneration == "unsupported" {
			e.say(i18n.T("this NVIDIA GPU is older than Turing; it stays on nouveau"))
		} else {
			e.say(i18n.T("keeping nouveau for the NVIDIA GPU (the NVIDIA driver item is off)"))
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
func recordFedoraNvidiaChoice(e *engine, host string) {
	if !e.f.hasNvidia {
		return
	}
	choice := "nouveau"
	if e.p.nvidia {
		choice = "driver"
	}
	if err := e.sudo(host, "kmod", "record-choice", choice); err != nil {
		e.say(i18n.T("warning: could not record the NVIDIA choice for ryoku doctor (continuing)"))
	}
}
