package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ryoku-cli/internal/host"
	"ryoku-cli/internal/kmod"
)

const (
	runningTestKernel = "6.18.1-200.fc44.x86_64"
	olderTestKernel   = "6.17.9-200.fc44.x86_64"
)

// fallbackMachine is an NVIDIA Fedora machine as the driver fallback
// reconciler sees it through its seams; builds records each build it asks for.
type fallbackMachine struct {
	akmodNvidia bool
	gpu         bool
	modules     map[string]bool
	build       kmod.BuildResult
	detail      string
	buildErr    error
	builds      []string
}

func newFallbackMachine(t *testing.T) *fallbackMachine {
	t.Helper()
	m := &fallbackMachine{akmodNvidia: true, gpu: true, modules: map[string]bool{}}
	oldInstalled, oldKernel, oldModule, oldGPU, oldBuild := nvidiaPackageInstalled, runningKernel, nvidiaModuleBuilt, driverFallbackGPU, buildNvidiaForKernel
	t.Cleanup(func() {
		nvidiaPackageInstalled, runningKernel, nvidiaModuleBuilt, driverFallbackGPU, buildNvidiaForKernel = oldInstalled, oldKernel, oldModule, oldGPU, oldBuild
	})
	nvidiaPackageInstalled = func(name string) bool { return name == "akmod-nvidia" && m.akmodNvidia }
	runningKernel = func() string { return runningTestKernel }
	nvidiaModuleBuilt = func(kernel string) bool { return m.modules[kernel] }
	driverFallbackGPU = func() bool { return m.gpu }
	buildNvidiaForKernel = func(opts kmod.BuildOptions) (kmod.BuildResult, string, error) {
		call := opts.Kernel
		if opts.Rebuild {
			call += " --rebuild"
		}
		m.builds = append(m.builds, call)
		if m.build.Built {
			m.modules[opts.Kernel] = true
		}
		return m.build, m.detail, m.buildErr
	}
	return m
}

func TestDriverFallbackIsHealedByBuildingForTheRunningKernel(t *testing.T) {
	machine := newFallbackMachine(t)
	machine.build = kmod.BuildResult{Built: true, NewestWithModule: runningTestKernel}
	pass := &moduleSigningPass{}

	todo := pass.reconcileDriverFallback(true)
	if todo.status != recWouldFix || len(machine.builds) != 0 {
		t.Fatalf("check-only = %s %q, builds %v; want a todo and no build", todo.status.label(), todo.detail, machine.builds)
	}
	if !strings.Contains(todo.detail, "nouveau") || todo.remedy != "sudo ryoku-host kmod build-for-kernel --kernel "+runningTestKernel {
		t.Fatalf("check-only = %q / %q, want the fallback explained and build-for-kernel as the fix", todo.detail, todo.remedy)
	}

	fixed := pass.reconcileDriverFallback(false)
	if fixed.status != recFixed || strings.Join(machine.builds, ";") != runningTestKernel {
		t.Fatalf("fix = %s %q, builds %v; want fixed by one build for the running kernel", fixed.status.label(), fixed.detail, machine.builds)
	}
	if !strings.Contains(fixed.detail, "reboot") {
		t.Fatalf("fix detail = %q, want it to say a reboot loads the driver", fixed.detail)
	}
	if again := pass.reconcileDriverFallback(false); again.status != recOK || len(machine.builds) != 1 {
		t.Fatalf("second run = %s %q, builds %v; want ok", again.status.label(), again.detail, machine.builds)
	}
}

func TestDriverFallbackThatCannotBeBuiltNamesTheNewestKernelWithTheDriver(t *testing.T) {
	machine := newFallbackMachine(t)
	machine.modules[olderTestKernel] = true
	machine.build = kmod.BuildResult{NewestWithModule: olderTestKernel}
	machine.detail = "Building rpms failed; see /var/cache/akmods/nvidia/580.95-for-6.18.1.failed.log"

	got := (&moduleSigningPass{}).reconcileDriverFallback(false)
	if got.status != recNote {
		t.Fatalf("result = %s %q, want a note", got.status.label(), got.detail)
	}
	for _, want := range []string{runningTestKernel, "580.95-for-6.18.1.failed.log", olderTestKernel, "GRUB"} {
		if !strings.Contains(got.detail+" "+got.remedy, want) {
			t.Fatalf("note = %q / %q, want it to name %q", got.detail, got.remedy, want)
		}
	}
}

func TestDriverFallbackWithNoKernelLeftWithTheDriverSaysSo(t *testing.T) {
	machine := newFallbackMachine(t)
	machine.detail = "Building rpms failed"

	got := (&moduleSigningPass{}).reconcileDriverFallback(false)
	if got.status != recNote || !strings.Contains(got.detail, "no installed kernel") {
		t.Fatalf("result = %s %q, want a note that no kernel has the driver", got.status.label(), got.detail)
	}
}

func TestDriverFallbackBuildThatCannotRunIsReported(t *testing.T) {
	machine := newFallbackMachine(t)
	machine.buildErr = errors.New("sudo: a password is required")

	got := (&moduleSigningPass{}).reconcileDriverFallback(false)
	if got.status != recFailed || !strings.Contains(got.detail, "a password is required") ||
		got.remedy != "sudo ryoku-host kmod build-for-kernel --kernel "+runningTestKernel {
		t.Fatalf("result = %s %q / %q, want a failure naming the error and the command", got.status.label(), got.detail, got.remedy)
	}
}

func TestNoDriverFallbackWhereThereIsNothingToBuild(t *testing.T) {
	cases := map[string]func(*fallbackMachine){
		"the running kernel has its module": func(m *fallbackMachine) { m.modules[runningTestKernel] = true },
		"akmod-nvidia is not installed":     func(m *fallbackMachine) { m.akmodNvidia = false },
		"there is no NVIDIA GPU":            func(m *fallbackMachine) { m.gpu = false },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			machine := newFallbackMachine(t)
			arrange(machine)
			for _, checkOnly := range []bool{true, false} {
				if got := (&moduleSigningPass{}).reconcileDriverFallback(checkOnly); got.status != recOK || len(machine.builds) != 0 {
					t.Fatalf("checkOnly=%v: %s %q, builds %v; want ok without a build", checkOnly, got.status.label(), got.detail, machine.builds)
				}
			}
		})
	}
}

// A build on an unreadable key fails and leaves a marker that keeps akmods
// from retrying at boot, so a key fix is followed by a forced rebuild even
// when the running kernel already has a module.
func TestKeyFixInThePassRebuildsTheDriver(t *testing.T) {
	signing := newSigningMachine(t)
	signing.withAkmodsGroup(t)
	signing.withKey(t, 0o600, 0, 0)
	signing.link(t, "/etc/pki/akmods/private/private_key.priv", "/var/lib/dkms/mok.key")
	signing.link(t, "/etc/pki/akmods/certs/public_key.der", "/var/lib/dkms/mok.pub")
	calls := signing.use(t)
	machine := newFallbackMachine(t)
	machine.modules[runningTestKernel] = true
	machine.build = kmod.BuildResult{Built: true, NewestWithModule: runningTestKernel}

	check := runModuleSigningPass(true)
	if got := check["NVIDIA driver fallback"]; got.status != recWouldFix || len(machine.builds) != 0 ||
		got.remedy != "sudo ryoku-host kmod build-for-kernel --kernel "+runningTestKernel+" --rebuild" {
		t.Fatalf("check-only driver row = %s %q / %q, builds %v; want a rebuild todo", got.status.label(), got.detail, got.remedy, machine.builds)
	}

	fix := runModuleSigningPass(false)
	if key := fix["module signing key permissions"]; key.status != recFixed || calls.prepared != 1 {
		t.Fatalf("key row = %s %q, prepared %d times; want fixed", key.status.label(), key.detail, calls.prepared)
	}
	if got := fix["NVIDIA driver fallback"]; got.status != recFixed || strings.Join(machine.builds, ";") != runningTestKernel+" --rebuild" {
		t.Fatalf("driver row = %s %q, builds %v; want one forced rebuild for the running kernel", got.status.label(), got.detail, machine.builds)
	}

	again := runModuleSigningPass(false)
	if got := again["NVIDIA driver fallback"]; got.status != recOK || len(machine.builds) != 1 {
		t.Fatalf("next pass driver row = %s %q, builds %v; want ok without another build", got.status.label(), got.detail, machine.builds)
	}
}

func TestAkmodsLinkFixInThePassRebuildsTheDriver(t *testing.T) {
	signing := newSigningMachine(t)
	signing.withAkmodsGroup(t)
	signing.withKey(t, 0o640, 0, testAkmodsGID)
	signing.use(t)
	machine := newFallbackMachine(t)
	machine.modules[runningTestKernel] = true
	machine.build = kmod.BuildResult{Built: true, NewestWithModule: runningTestKernel}

	fix := runModuleSigningPass(false)
	if links := fix["akmods links to the module signing key"]; links.status != recFixed {
		t.Fatalf("links row = %s %q, want fixed", links.status.label(), links.detail)
	}
	if got := fix["NVIDIA driver fallback"]; got.status != recFixed || strings.Join(machine.builds, ";") != runningTestKernel+" --rebuild" {
		t.Fatalf("driver row = %s %q, builds %v; want one forced rebuild", got.status.label(), got.detail, machine.builds)
	}
}

func runModuleSigningPass(checkOnly bool) map[string]recResult {
	results := map[string]recResult{}
	for _, check := range moduleSigningChecks() {
		results[check.name] = check.run(checkOnly)
	}
	return results
}

func TestModuleSigningPassFixesTheKeyBeforeTheDriver(t *testing.T) {
	oldManager := doctorPackageManager
	t.Cleanup(func() { doctorPackageManager = oldManager })
	doctorPackageManager = func() (host.PackageManager, error) { return host.DNF, nil }

	position := map[string]int{}
	for i, r := range reconcilers() {
		position[r.name] = i + 1
	}
	order := []string{
		"module signing key permissions",
		"akmods links to the module signing key",
		"module signing key enrollment",
		"NVIDIA driver fallback",
	}
	for i, name := range order {
		if position[name] == 0 {
			t.Fatalf("Fedora pass has no %q", name)
		}
		if i > 0 && position[order[i-1]] > position[name] {
			t.Fatalf("%q runs before %q; want the key reconcilers first", name, order[i-1])
		}
	}
}

// The build goes through ryoku-host as root. Doctor never pins a kernel or
// changes the boot default: both stay the user's choice from the GRUB menu.
func TestDriverBuildRunsOnlyBuildForKernel(t *testing.T) {
	log := filepath.Join(t.TempDir(), "sudo.log")
	t.Setenv("SUDO_TEST_LOG", log)
	t.Setenv("RYOKU_HOST_BIN", "/usr/bin/ryoku-host")
	stubPath(t, map[string]string{
		"sudo": `printf '%s\n' "$*" >>"$SUDO_TEST_LOG"
printf 'build=failed\nkernel=` + runningTestKernel + `\nnewest_module_kernel=` + olderTestKernel + `\n'
printf 'Building and installing nvidia-kmod\nBuilding rpms failed; see the failed log\n' >&2
`,
	})

	build, detail, err := buildNvidiaForKernel(kmod.BuildOptions{Kernel: runningTestKernel, Rebuild: true})
	if err != nil {
		t.Fatal(err)
	}
	if build.Built || build.NewestWithModule != olderTestKernel || detail != "Building rpms failed; see the failed log" {
		t.Fatalf("build = %+v, detail %q, want failed, the older kernel and akmods' last line", build, detail)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "/usr/bin/ryoku-host kmod build-for-kernel --kernel " + runningTestKernel + " --rebuild\n"
	if string(body) != want {
		t.Fatalf("sudo ran %q, want only %q", body, want)
	}
	for _, forbidden := range []string{"grub2-set-default", "grubby", "versionlock", "--exclude"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("doctor ran %q", forbidden)
		}
	}
}
