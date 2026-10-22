package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ryoku-cli/internal/host"
	"ryoku-cli/internal/kmod"
)

// fedoraNvidiaMachine is a Fedora machine as the NVIDIA reconcilers see it.
// Its fields are what the seams answer; the counters record what doctor did.
type fedoraNvidiaMachine struct {
	gpu              kmod.GPU
	choice           kmod.Choice
	installed        map[string]bool
	fallbackEnabled  bool
	failInstall      bool
	supportInstalls  int
	blacklistRestore int
	fallbackEnables  int
	running          string
	template         string
}

const rpmFusionArgs = "rd.driver.blacklist=nouveau,nova_core modprobe.blacklist=nouveau,nova_core"

func stubFedoraNvidia(t *testing.T, m *fedoraNvidiaMachine) {
	t.Helper()
	stubDoctorDNF(t)
	if m.installed == nil {
		m.installed = map[string]bool{}
	}
	dir := t.TempDir()
	running, template := filepath.Join(dir, "cmdline"), filepath.Join(dir, "kernel-cmdline")
	write := func() {
		if err := os.WriteFile(running, []byte(m.running+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(template, []byte(m.template+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write()

	oldGPU, oldChoice, oldInstalled, oldModule := fedoraNvidiaGPU, fedoraNvidiaChoice, nvidiaPackageInstalled, nvidiaModuleDir
	oldSupport, oldRunning, oldTemplate, oldRestore := installNvidiaSupport, runningKernelCmdline, kernelCmdlineTemplate, restoreNouveauBlacklist
	oldService, oldEnable, oldRemove := doctorService, enableNvidiaFallback, nvidiaRemovePackage
	t.Cleanup(func() {
		nvidiaRemovePackage = oldRemove
		fedoraNvidiaGPU, fedoraNvidiaChoice, nvidiaPackageInstalled, nvidiaModuleDir = oldGPU, oldChoice, oldInstalled, oldModule
		installNvidiaSupport, runningKernelCmdline, kernelCmdlineTemplate, restoreNouveauBlacklist = oldSupport, oldRunning, oldTemplate, oldRestore
		doctorService, enableNvidiaFallback = oldService, oldEnable
	})

	fedoraNvidiaGPU = func() kmod.GPU { return m.gpu }
	fedoraNvidiaChoice = func() kmod.Choice { return m.choice }
	nvidiaPackageInstalled = func(name string) bool { return m.installed[name] }
	nvidiaModuleDir = filepath.Join(dir, "no-nvidia-module")
	installNvidiaSupport = func() error {
		m.supportInstalls++
		if m.failInstall {
			return errors.New("dnf said no")
		}
		m.installed[kmod.DriverPackage] = true
		return nil
	}
	runningKernelCmdline, kernelCmdlineTemplate = running, template
	restoreNouveauBlacklist = func() error {
		m.blacklistRestore++
		m.template = strings.TrimSpace(m.template + " " + rpmFusionArgs)
		write()
		return nil
	}
	doctorService = func(args ...string) int {
		if strings.Join(args, " ") == "--system is-enabled nvidia-fallback.service" && m.fallbackEnabled {
			return 0
		}
		return 1
	}
	enableNvidiaFallback = func() error {
		m.fallbackEnables++
		m.fallbackEnabled = true
		return nil
	}
}

func assertFedoraNvidiaAdvice(t *testing.T, result recResult) {
	t.Helper()
	assertNoForeignPackageAdvice(t, result)
	text := strings.ToLower(result.detail + " " + result.remedy)
	for _, manual := range []string{"dracut", "enable rpm fusion", "keeps nouveau active", "left to the user", "to you"} {
		if strings.Contains(text, manual) {
			t.Errorf("Fedora NVIDIA result still gives the manual advice %q: %+v", manual, result)
		}
	}
}

func TestFedoraNvidiaDriverOfferedWithoutARecordedChoice(t *testing.T) {
	for _, checkOnly := range []bool{true, false} {
		stubFedoraNvidia(t, &fedoraNvidiaMachine{gpu: kmod.GPUSupported})
		result := reconcileFedoraNvidiaDriver(checkOnly)
		if result.status != recNote || result.remedy != "sudo ryoku-host kmod install-driver" {
			t.Fatalf("checkOnly=%v: offer = %+v", checkOnly, result)
		}
		assertFedoraNvidiaAdvice(t, result)
	}
}

func TestFedoraNvidiaDriverNotOfferedAfterARecordedNouveauChoice(t *testing.T) {
	for _, checkOnly := range []bool{true, false} {
		stubFedoraNvidia(t, &fedoraNvidiaMachine{gpu: kmod.GPUSupported, choice: kmod.ChoiceNouveau})
		result := reconcileFedoraNvidiaDriver(checkOnly)
		if result.status != recOK || result.remedy != "" || !strings.Contains(result.detail, "nouveau") {
			t.Fatalf("checkOnly=%v: a nouveau choice still got the offer: %+v", checkOnly, result)
		}
	}
}

func TestFedoraNvidiaDriverNotOfferedOnceInstalled(t *testing.T) {
	stubFedoraNvidia(t, &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}})
	if result := reconcileFedoraNvidiaDriver(true); result.status != recOK || result.remedy != "" {
		t.Fatalf("installed driver = %+v", result)
	}
}

func TestFedoraNvidiaDriverNotOfferedWithoutASupportedGPU(t *testing.T) {
	for _, gpu := range []kmod.GPU{kmod.GPUAbsent, kmod.GPUUnsupported, kmod.GPUUnknown} {
		stubFedoraNvidia(t, &fedoraNvidiaMachine{gpu: gpu})
		if result := reconcileFedoraNvidiaDriver(true); result.status != recOK || result.remedy != "" {
			t.Fatalf("%s GPU: %+v", gpu, result)
		}
	}
}

func TestFedoraNvidiaPreTuringDriverIsReportedNeverRemoved(t *testing.T) {
	for _, checkOnly := range []bool{true, false} {
		removed := false
		stubFedoraNvidia(t, &fedoraNvidiaMachine{gpu: kmod.GPUUnsupported, installed: map[string]bool{"akmod-nvidia": true}})
		nvidiaRemovePackage = func(string) error { removed = true; return nil }
		result := reconcileFedoraNvidiaDriver(checkOnly)
		if result.status != recWarn || !strings.Contains(result.detail, "akmod-nvidia") || !strings.Contains(result.detail, "Turing") {
			t.Fatalf("checkOnly=%v: pre-Turing report = %+v", checkOnly, result)
		}
		if removed {
			t.Fatalf("checkOnly=%v: doctor removed a package it did not install", checkOnly)
		}
		assertFedoraNvidiaAdvice(t, result)
	}
}

func TestFedoraNvidiaSupportPackageMissingIsInstalledAgain(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}}
	stubFedoraNvidia(t, machine)

	result := reconcileFedoraNvidiaPackage(true)
	if result.status != recWouldFix || !strings.Contains(result.remedy, kmod.DriverPackage) || machine.supportInstalls != 0 {
		t.Fatalf("check-only = %+v after %d installs", result, machine.supportInstalls)
	}
	assertFedoraNvidiaAdvice(t, result)

	result = reconcileFedoraNvidiaPackage(false)
	if result.status != recFixed || machine.supportInstalls != 1 {
		t.Fatalf("fix = %+v after %d installs", result, machine.supportInstalls)
	}
	assertFedoraNvidiaAdvice(t, result)
	if result = reconcileFedoraNvidiaPackage(false); result.status != recOK || machine.supportInstalls != 1 {
		t.Fatalf("second pass = %+v after %d installs", result, machine.supportInstalls)
	}
}

func TestFedoraNvidiaSupportPackageInstallFailureIsReported(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}, failInstall: true}
	stubFedoraNvidia(t, machine)
	result := reconcileFedoraNvidiaPackage(false)
	if result.status != recFailed || !strings.Contains(result.detail, "dnf said no") || !strings.Contains(result.remedy, kmod.DriverPackage) {
		t.Fatalf("failed install = %+v", result)
	}
}

func TestFedoraNvidiaSupportPackageOnlyFollowsTheDriverOnASupportedGPU(t *testing.T) {
	for _, machine := range []*fedoraNvidiaMachine{
		{gpu: kmod.GPUSupported},
		{gpu: kmod.GPUUnsupported, installed: map[string]bool{"akmod-nvidia": true}},
		{gpu: kmod.GPUUnknown, installed: map[string]bool{"akmod-nvidia": true}},
	} {
		stubFedoraNvidia(t, machine)
		if result := reconcileFedoraNvidiaPackage(false); result.status != recOK || machine.supportInstalls != 0 {
			t.Fatalf("%s GPU, installed %v: %+v after %d installs", machine.gpu, machine.installed, result, machine.supportInstalls)
		}
	}
}

const bootArgs = "root=UUID=ab92 ro rootflags=subvol=root rhgb quiet"

func TestFedoraNouveauBlacklistMissingIsRestoredOnEveryKernel(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}, running: "BOOT_IMAGE=/vmlinuz " + bootArgs, template: bootArgs}
	stubFedoraNvidia(t, machine)

	result := reconcileFedoraNouveauBlacklist(true)
	want := `sudo grubby --update-kernel=ALL --args="` + rpmFusionArgs + `"`
	if result.status != recWouldFix || result.remedy != want || machine.blacklistRestore != 0 {
		t.Fatalf("check-only = %+v after %d restores", result, machine.blacklistRestore)
	}
	assertFedoraNvidiaAdvice(t, result)

	result = reconcileFedoraNouveauBlacklist(false)
	if result.status != recFixed || machine.blacklistRestore != 1 || !strings.Contains(result.detail, "boot") {
		t.Fatalf("fix = %+v after %d restores", result, machine.blacklistRestore)
	}

	result = reconcileFedoraNouveauBlacklist(false)
	if result.status != recNote || machine.blacklistRestore != 1 || !strings.Contains(result.detail, "reboot") {
		t.Fatalf("pass before the reboot = %+v after %d restores", result, machine.blacklistRestore)
	}

	machine.running = "BOOT_IMAGE=/vmlinuz " + machine.template
	stubFedoraNvidia(t, machine)
	if result = reconcileFedoraNouveauBlacklist(false); result.status != recOK || machine.blacklistRestore != 1 {
		t.Fatalf("pass after the reboot = %+v after %d restores", result, machine.blacklistRestore)
	}
}

func TestFedoraNouveauBlacklistIsRestoredWhenOnlyTheRunningKernelLacksItAndNoTemplateExists(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}, running: bootArgs}
	stubFedoraNvidia(t, machine)
	kernelCmdlineTemplate = filepath.Join(t.TempDir(), "absent")
	restoreNouveauBlacklist = func() error { machine.blacklistRestore++; return nil }
	if result := reconcileFedoraNouveauBlacklist(false); result.status != recFixed || machine.blacklistRestore != 1 {
		t.Fatalf("fix without /etc/kernel/cmdline = %+v after %d restores", result, machine.blacklistRestore)
	}
}

func TestFedoraNouveauBlacklistIsLeftAloneWithoutTheDriverOnASupportedGPU(t *testing.T) {
	for _, machine := range []*fedoraNvidiaMachine{
		{gpu: kmod.GPUSupported, running: bootArgs, template: bootArgs},
		{gpu: kmod.GPUUnsupported, installed: map[string]bool{"akmod-nvidia": true}, running: bootArgs, template: bootArgs},
	} {
		stubFedoraNvidia(t, machine)
		if result := reconcileFedoraNouveauBlacklist(false); result.status != recOK || machine.blacklistRestore != 0 {
			t.Fatalf("%s GPU, installed %v: %+v after %d restores", machine.gpu, machine.installed, result, machine.blacklistRestore)
		}
	}
}

func TestFedoraNouveauBlacklistNeedsRPMFusionsExactArguments(t *testing.T) {
	machine := &fedoraNvidiaMachine{
		gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true},
		running:  bootArgs + " rd.driver.blacklist=nouveau modprobe.blacklist=nouveau",
		template: bootArgs + " rd.driver.blacklist=nouveau modprobe.blacklist=nouveau",
	}
	stubFedoraNvidia(t, machine)
	if result := reconcileFedoraNouveauBlacklist(true); result.status != recWouldFix {
		t.Fatalf("a blacklist nvidia-fallback.service does not match passed: %+v", result)
	}
}

func TestFedoraNvidiaFallbackServiceDisabledIsEnabledAgain(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}}
	stubFedoraNvidia(t, machine)

	result := reconcileFedoraNvidiaFallback(true)
	if result.status != recWouldFix || result.remedy != "sudo systemctl enable nvidia-fallback.service" || machine.fallbackEnables != 0 {
		t.Fatalf("check-only = %+v after %d enables", result, machine.fallbackEnables)
	}
	assertFedoraNvidiaAdvice(t, result)

	if result = reconcileFedoraNvidiaFallback(false); result.status != recFixed || machine.fallbackEnables != 1 {
		t.Fatalf("fix = %+v after %d enables", result, machine.fallbackEnables)
	}
	if result = reconcileFedoraNvidiaFallback(false); result.status != recOK || machine.fallbackEnables != 1 {
		t.Fatalf("second pass = %+v after %d enables", result, machine.fallbackEnables)
	}
}

func TestFedoraNvidiaFallbackServiceEnableFailureIsReported(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported, installed: map[string]bool{"akmod-nvidia": true}}
	stubFedoraNvidia(t, machine)
	enableNvidiaFallback = func() error { return errors.New("unit is masked") }
	if result := reconcileFedoraNvidiaFallback(false); result.status != recFailed || !strings.Contains(result.detail, "masked") {
		t.Fatalf("failed enable = %+v", result)
	}
}

func TestFedoraNvidiaFallbackServiceIsLeftAloneWithoutTheDriver(t *testing.T) {
	machine := &fedoraNvidiaMachine{gpu: kmod.GPUSupported}
	stubFedoraNvidia(t, machine)
	if result := reconcileFedoraNvidiaFallback(false); result.status != recOK || machine.fallbackEnables != 0 {
		t.Fatalf("no driver = %+v after %d enables", result, machine.fallbackEnables)
	}
}

func TestNvidiaReconcilersFollowThePackageManager(t *testing.T) {
	oldManager := doctorPackageManager
	t.Cleanup(func() { doctorPackageManager = oldManager })
	fedora := []string{"NVIDIA driver", "Ryoku NVIDIA package", "nouveau blacklist arguments", "NVIDIA fallback service", "NVIDIA sleep units"}
	others := []string{"Kepler NVIDIA recovery", "NVIDIA boot reliability", "NVIDIA update guard hook", "NVIDIA sleep units"}
	for _, tc := range []struct {
		manager host.PackageManager
		want    []string
	}{
		{host.DNF, fedora},
		{host.Pacman, others},
		{host.XBPS, others},
	} {
		doctorPackageManager = func() (host.PackageManager, error) { return tc.manager, nil }
		var got []string
		for _, r := range reconcilers() {
			if slices.Contains(fedora, r.name) || slices.Contains(others, r.name) {
				got = append(got, r.name)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("manager %q NVIDIA reconcilers = %q, want %q", tc.manager, got, tc.want)
		}
	}
}
