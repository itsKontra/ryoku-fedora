package kmod

import (
	"errors"
	"strings"
	"testing"
)

func TestInstallDriverRefusesAGPUOlderThanTuring(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
	}{{"pre-Turing", 3}, {"no NVIDIA GPU", 4}, {"no lspci", 5}} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeFedora(t)
			fake.gpuCode = tc.code
			_, err := fake.machine().InstallDriver(DriverOptions{})
			if !errors.Is(err, ErrGPUNotSupported) {
				t.Fatalf("err = %v, want ErrGPUNotSupported", err)
			}
			if got := fake.runner.commands[len(fake.runner.commands)-1]; got != "bash "+GPUCheckScript+" --query" {
				t.Fatalf("ran %q after the GPU check", fake.runner.commands)
			}
		})
	}
}

func TestInstallDriverTrustsACallerThatAlreadyCheckedTheGPU(t *testing.T) {
	fake := newFakeFedora(t)
	fake.gpuCode = 5
	if _, err := fake.machine().InstallDriver(DriverOptions{GPUSupported: true}); err != nil {
		t.Fatal(err)
	}
	if fake.runner.ran("bash ") {
		t.Fatalf("the GPU check ran inside a root whose caller already checked: %q", fake.runner.commands)
	}
}

func TestInstallDriverOrder(t *testing.T) {
	fake := newFakeFedora(t)
	result, err := fake.machine().InstallDriver(DriverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kernel != testKernel {
		t.Fatalf("kernel = %q, want the boot default %q", result.Kernel, testKernel)
	}
	steps := []string{
		"bash " + GPUCheckScript + " --query",
		"rpm --import",
		"dnf5 install -y ",
		"dnf5 install -y akmods kernel-devel-" + testKernel,
		"openssl req",
		"dnf5 install -y ryoku-nvidia-support",
		"akmods --force --kernels " + testKernel + " --akmod nvidia",
		"modinfo -k " + testKernel + " -F sig_key nvidia",
	}
	last := -1
	for _, step := range steps {
		at := fake.index(step)
		if at <= last {
			t.Fatalf("%q ran at %d, after step %d; commands:\n%s", step, at, last, strings.Join(fake.runner.commands, "\n"))
		}
		last = at
	}
	if fake.keyAtDriver != "0640 root:akmods links=ok" {
		t.Fatalf("when akmod-nvidia's package installed the key was %q, want 0640 root:akmods links=ok", fake.keyAtDriver)
	}
	for _, command := range fake.runner.commands {
		if strings.Contains(command, "tainted") || strings.Contains(command, "versionlock") {
			t.Fatalf("install driver ran %q", command)
		}
	}
}

func TestInstallDriverEnablesRPMFusionFromItsReleasePackages(t *testing.T) {
	fake := newFakeFedora(t)
	if _, err := fake.machine().InstallDriver(DriverOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{
		"https://mirrors.rpmfusion.org/free/fedora/rpmfusion-free-release-44.noarch.rpm",
		"https://mirrors.rpmfusion.org/nonfree/fedora/rpmfusion-nonfree-release-44.noarch.rpm",
	} {
		if !fake.runner.ran("curl ") || !strings.Contains(strings.Join(fake.runner.commands, "\n"), url) {
			t.Fatalf("no download of %s:\n%s", url, strings.Join(fake.runner.commands, "\n"))
		}
	}
	install := fake.runner.commands[fake.index("dnf5 install -y ")]
	if !strings.Contains(install, "rpmfusion-free-release-44.noarch.rpm") || !strings.Contains(install, "rpmfusion-nonfree-release-44.noarch.rpm") {
		t.Fatalf("release packages transaction = %q", install)
	}
}

func TestInstallDriverRejectsARPMFusionKeyThatIsNotPinned(t *testing.T) {
	fake := newFakeFedora(t)
	fake.fingerprints["nonfree"] = "0123456789ABCDEF0123456789ABCDEF01234567"
	_, err := fake.machine().InstallDriver(DriverOptions{})
	if err == nil || !strings.Contains(err.Error(), "0123456789ABCDEF0123456789ABCDEF01234567") {
		t.Fatalf("err = %v, want a fingerprint mismatch", err)
	}
	if fake.runner.ran("dnf5 ") || fake.runner.ran("rpm --import") {
		t.Fatalf("a rejected key still led to:\n%s", strings.Join(fake.runner.commands, "\n"))
	}
}

func TestInstallDriverRejectsAReleasePackageTheKeyDidNotSign(t *testing.T) {
	fake := newFakeFedora(t)
	fake.checksigCode = 1
	if _, err := fake.machine().InstallDriver(DriverOptions{}); err == nil {
		t.Fatal("a release package with a bad signature was accepted")
	}
	if fake.runner.ran("dnf5 ") {
		t.Fatalf("installed after a failed signature check:\n%s", strings.Join(fake.runner.commands, "\n"))
	}
}

func TestInstallDriverSkipsRPMFusionWhenItIsAlreadyEnabled(t *testing.T) {
	fake := newFakeFedora(t)
	fake.fusionInstalled = true
	if _, err := fake.machine().InstallDriver(DriverOptions{}); err != nil {
		t.Fatal(err)
	}
	if fake.runner.ran("curl ") {
		t.Fatalf("downloaded RPM Fusion again:\n%s", strings.Join(fake.runner.commands, "\n"))
	}
}

func TestInstallDriverChecksTheModuleIsSignedWithTheMachineKey(t *testing.T) {
	cases := []struct {
		name   string
		built  bool
		sigKey string
		ok     bool
	}{
		{"signed with the machine key", true, "5B:12:C0:FF:EE", true},
		{"leading zero bytes do not matter", true, "00:5B:12:C0:FF:EE", true},
		{"signed with another key", true, "AA:BB:CC", false},
		{"unsigned", true, "", false},
		{"never built", false, "5B:12:C0:FF:EE", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeFedora(t)
			fake.buildFor[testKernel] = tc.built
			fake.sigKey = tc.sigKey
			_, err := fake.machine().InstallDriver(DriverOptions{})
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
		})
	}
}

func TestInstallDriverRebuildsAModuleSignedWithAnotherKeyOnce(t *testing.T) {
	fake := newFakeFedora(t)
	fake.sigKey = "AA:BB:CC"
	_, _ = fake.machine().InstallDriver(DriverOptions{})
	if !fake.runner.ran("akmods --force --rebuild --kernels " + testKernel) {
		t.Fatalf("no rebuild of a module signed with another key:\n%s", strings.Join(fake.runner.commands, "\n"))
	}
}

func TestInstallDriverBuildsForTheKernelItIsGiven(t *testing.T) {
	other := "6.16.9-200.fc44.x86_64"
	fake := newFakeFedora(t, testKernel, other)
	fake.buildFor[other] = true
	result, err := fake.machine().InstallDriver(DriverOptions{Kernel: other})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kernel != other || !fake.runner.ran("akmods --force --kernels "+other) || !fake.runner.ran("dnf5 install -y akmods kernel-devel-"+other) {
		t.Fatalf("kernel %q, commands:\n%s", result.Kernel, strings.Join(fake.runner.commands, "\n"))
	}
	if _, err := fake.machine().InstallDriver(DriverOptions{Kernel: "6.1.0-1.fc44.x86_64"}); err == nil {
		t.Fatal("a kernel that is not installed was accepted")
	}
}
