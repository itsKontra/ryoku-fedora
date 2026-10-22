package kmod

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ryoku-cli/internal/host"
)

const (
	testKernel = "6.17.4-200.fc44.x86_64"
	testSerial = "5B12C0FFEE"
)

// fakeFedora answers the commands install driver and build for kernel run, as
// a Fedora 44 machine with an NVIDIA GPU would.
type fakeFedora struct {
	*fakeMachine
	gpuCode         int
	fusionInstalled bool
	fingerprints    map[string]string
	checksigCode    int
	buildFor        map[string]bool
	sigKey          string
	defaultKernel   string
	modules         map[string]bool
	keyAtDriver     string
}

func newFakeFedora(t *testing.T, kernels ...string) *fakeFedora {
	t.Helper()
	f := &fakeFedora{
		fakeMachine:   newFakeMachine(t),
		fingerprints:  map[string]string{"free": rpmFusionFingerprints["free"], "nonfree": rpmFusionFingerprints["nonfree"]},
		buildFor:      map[string]bool{},
		sigKey:        "5B:12:C0:FF:EE",
		defaultKernel: testKernel,
		modules:       map[string]bool{},
	}
	writeFile(t, f.path("etc/os-release"), "NAME=\"Fedora Linux\"\nID=fedora\nVERSION_ID=44\n", 0o644)
	if len(kernels) == 0 {
		kernels = []string{testKernel}
	}
	for _, kernel := range kernels {
		writeFile(t, f.path("usr/lib/modules/"+kernel+"/vmlinuz"), "kernel", 0o644)
	}
	f.buildFor[testKernel] = true
	f.runner.answer = f.answer
	return f
}

func (f *fakeFedora) answer(command host.Command) host.Result {
	args := command.Args
	line := argv(command)
	switch command.Name {
	case "bash":
		return host.Result{Code: f.gpuCode}
	case "grubby":
		return host.Result{Output: "/boot/vmlinuz-" + f.defaultKernel + "\n"}
	case "rpm":
		switch {
		case args[0] == "-q":
			if f.fusionInstalled {
				return host.Result{}
			}
			return host.Result{Code: 1}
		case args[0] == "-K":
			return host.Result{Code: f.checksigCode}
		}
	case "curl":
		_ = os.WriteFile(flagValue(args, "--output"), []byte(rpmFusionRepoOf(args[len(args)-1])), 0o644)
	case "rpm2cpio":
		data, _ := os.ReadFile(args[0])
		_, _ = io.WriteString(command.Stdout, string(data))
	case "cpio":
		repo := f.runner.stdin[len(f.runner.stdin)-1]
		dir := filepath.Join(flagValue(args, "-D"), "etc", "pki", "rpm-gpg")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "RPM-GPG-KEY-rpmfusion-"+repo+"-fedora-2020"), []byte(repo), 0o644)
		_ = os.Symlink("RPM-GPG-KEY-rpmfusion-"+repo+"-fedora-2020", filepath.Join(dir, "RPM-GPG-KEY-rpmfusion-"+repo+"-fedora-44"))
	case "gpg":
		data, _ := os.ReadFile(args[len(args)-1])
		return host.Result{Output: "pub:-:4096:1:E06F8ECDD651FF2E:1571666881:::-:::scSC::::::23::0:\n" +
			"fpr:::::::::" + f.fingerprints[string(data)] + ":\n" +
			"uid:-::::1571666881::044A::RPM Fusion " + string(data) + " repository for Fedora (2020)::::::::::0:\n"}
	case "dnf5":
		if strings.Contains(line, " akmods") {
			_ = os.WriteFile(f.path("etc/group"), []byte("root:x:0:\nwheel:x:10:\nakmods:x:973:\n"), 0o644)
		}
		if strings.Contains(line, " ryoku-nvidia-support") {
			status, _ := f.machine().Status()
			f.keyAtDriver = fmt.Sprintf("%04o %s links=%s", status.Key.Mode, status.Key.Owner, status.Key.Links)
		}
	case "akmods":
		kernel := flagValue(args, "--kernels")
		if f.buildFor[kernel] {
			f.modules[kernel] = true
		}
	case "modinfo":
		kernel, field := flagValue(args, "-k"), flagValue(args, "-F")
		if !f.modules[kernel] {
			return host.Result{Code: 1, Output: "modinfo: ERROR: Module nvidia not found.\n"}
		}
		switch field {
		case "filename":
			return host.Result{Output: "/usr/lib/modules/" + kernel + "/extra/nvidia/nvidia.ko.xz\n"}
		case "sig_key":
			return host.Result{Output: f.sigKey + "\n"}
		}
	case "openssl":
		if args[0] == "x509" {
			return host.Result{Output: "serial=" + testSerial + "\n"}
		}
	}
	return host.Result{}
}

func flagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func rpmFusionRepoOf(url string) string {
	if strings.Contains(url, "/nonfree/") {
		return "nonfree"
	}
	return "free"
}

// index is where the first command starting with prefix ran, or -1.
func (f *fakeFedora) index(prefix string) int {
	for i, command := range f.runner.commands {
		if strings.HasPrefix(command, prefix) {
			return i
		}
	}
	return -1
}
