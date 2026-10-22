package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ryoku-cli/internal/kmod"
)

const (
	oldNvidiaKernel = "6.17.1-200.fc44.x86_64"
	newNvidiaKernel = "6.17.4-200.fc44.x86_64"
)

// stubNvidiaMachine points the updater at a scratch modules tree holding the
// given kernels and replaces akmod-nvidia's presence and the build with
// stubs. It returns the kernels a build was asked for.
func stubNvidiaMachine(t *testing.T, akmod bool, result kmod.BuildResult, kernels ...string) *[]string {
	t.Helper()
	oldModules, oldInstalled, oldBuild := modulesDir, nvidiaAkmodInstalled, nvidiaBuildForKernel
	t.Cleanup(func() {
		modulesDir, nvidiaAkmodInstalled, nvidiaBuildForKernel = oldModules, oldInstalled, oldBuild
	})
	modulesDir = t.TempDir()
	for _, kernel := range kernels {
		installKernel(t, kernel)
	}
	nvidiaAkmodInstalled = func() bool { return akmod }
	var builds []string
	nvidiaBuildForKernel = func(kernel string) (kmod.BuildResult, error) {
		builds = append(builds, kernel)
		return result, nil
	}
	return &builds
}

func installKernel(t *testing.T, kernel string) {
	t.Helper()
	dir := filepath.Join(modulesDir, kernel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vmlinuz"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNvidiaBuildSkippedWithoutANewKernel(t *testing.T) {
	builds := stubNvidiaMachine(t, true, kmod.BuildResult{Built: true}, oldNvidiaKernel)

	if note := awaitNvidiaBuild(oldNvidiaKernel); note != "" {
		t.Fatalf("note = %q, want none", note)
	}
	if len(*builds) != 0 {
		t.Fatalf("built for %v without a new kernel", *builds)
	}
}

func TestNvidiaBuildRunsForTheNewKernelAndReportsTheDriverReady(t *testing.T) {
	builds := stubNvidiaMachine(t, true, kmod.BuildResult{Kernel: newNvidiaKernel, Built: true, NewestWithModule: newNvidiaKernel}, oldNvidiaKernel, newNvidiaKernel)

	note := awaitNvidiaBuild(oldNvidiaKernel)
	if len(*builds) != 1 || (*builds)[0] != newNvidiaKernel {
		t.Fatalf("builds = %v, want one for %s", *builds, newNvidiaKernel)
	}
	if !strings.Contains(note, "driver ready for the new kernel") || !strings.Contains(note, newNvidiaKernel) {
		t.Fatalf("note = %q, want the driver ready for %s", note, newNvidiaKernel)
	}
}

func TestNvidiaBuildFailureNamesTheFallbackKernel(t *testing.T) {
	builds := stubNvidiaMachine(t, true, kmod.BuildResult{Kernel: newNvidiaKernel, Built: false, NewestWithModule: oldNvidiaKernel}, oldNvidiaKernel, newNvidiaKernel)

	note := awaitNvidiaBuild(oldNvidiaKernel)
	if len(*builds) != 1 {
		t.Fatalf("builds = %v, want one", *builds)
	}
	for _, want := range []string{"build failed", "the next boot uses nouveau", "kernel " + oldNvidiaKernel + " still has the driver"} {
		if !strings.Contains(note, want) {
			t.Fatalf("note = %q, want it to say %q", note, want)
		}
	}
}

func TestNvidiaBuildSkippedWithoutAkmodNvidia(t *testing.T) {
	builds := stubNvidiaMachine(t, false, kmod.BuildResult{Built: true}, oldNvidiaKernel, newNvidiaKernel)

	if note := awaitNvidiaBuild(oldNvidiaKernel); note != "" {
		t.Fatalf("note = %q, want none", note)
	}
	if len(*builds) != 0 {
		t.Fatalf("built for %v on a machine without akmod-nvidia", *builds)
	}
}

func TestNvidiaBuildThatCannotRunLeavesItToAkmodsAtBoot(t *testing.T) {
	stubNvidiaMachine(t, true, kmod.BuildResult{}, oldNvidiaKernel, newNvidiaKernel)
	nvidiaBuildForKernel = func(string) (kmod.BuildResult, error) {
		return kmod.BuildResult{}, errors.New("sudo: a password is required")
	}

	note := awaitNvidiaBuild(oldNvidiaKernel)
	if !strings.Contains(note, "could not run") || !strings.Contains(note, "a password is required") {
		t.Fatalf("note = %q, want the reason the build did not run", note)
	}
}

// systemLaneInstallsKernel runs the system lane for the given package manager
// with a sudo that installs newNvidiaKernel as the upgrade would. It returns
// the kernels a build was asked for and whether akmod-nvidia was looked up.
func systemLaneInstallsKernel(t *testing.T, manager string) (builds []string, askedForAkmod bool) {
	t.Helper()
	isolateHome(t)
	recorded := stubNvidiaMachine(t, true, kmod.BuildResult{Kernel: newNvidiaKernel, Built: true}, oldNvidiaKernel)
	nvidiaAkmodInstalled = func() bool { askedForAkmod = true; return true }
	t.Setenv("RYOKU_HOST_PKGMGR", manager)
	bin := t.TempDir()
	kernel := filepath.Join(modulesDir, newNvidiaKernel)
	if err := os.MkdirAll(kernel, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExec(t, filepath.Join(bin, "sudo"), "#!/bin/sh\n: > "+kernel+"/vmlinuz\n")
	writeExec(t, filepath.Join(bin, "systemd-inhibit"), "#!/bin/sh\nwhile [ \"${1#--}\" != \"$1\" ]; do shift; done\nexec \"$@\"\n")
	writeExec(t, filepath.Join(bin, "dnf5"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin)

	runSystemLane(nil)
	return *recorded, askedForAkmod
}

func TestFedoraSystemLaneWaitsForTheNvidiaBuild(t *testing.T) {
	builds, _ := systemLaneInstallsKernel(t, "dnf")
	if len(builds) != 1 || builds[0] != newNvidiaKernel {
		t.Fatalf("builds = %v, want one for %s", builds, newNvidiaKernel)
	}
}

func TestSystemLaneOffFedoraNeverBuildsNvidia(t *testing.T) {
	for _, manager := range []string{"pacman", "xbps"} {
		t.Run(manager, func(t *testing.T) {
			builds, asked := systemLaneInstallsKernel(t, manager)
			if len(builds) != 0 || asked {
				t.Fatalf("builds = %v, akmod-nvidia looked up = %v; want neither off Fedora", builds, asked)
			}
		})
	}
}

// fakeRootTools puts sudo, systemd-inhibit and ryoku-host on an otherwise
// empty PATH. sudo and the inhibitor pass straight through; ryoku-host logs
// its argv to the returned file and runs hostBody.
func fakeRootTools(t *testing.T, hostBody string) string {
	t.Helper()
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	writeExec(t, filepath.Join(bin, "sudo"), "#!/bin/sh\nexec \"$@\"\n")
	writeExec(t, filepath.Join(bin, "systemd-inhibit"), "#!/bin/sh\nwhile [ \"${1#--}\" != \"$1\" ]; do shift; done\nexec \"$@\"\n")
	writeExec(t, filepath.Join(bin, "ryoku-host"), "#!/bin/sh\necho \"$*\" >> "+calls+"\n"+hostBody)
	t.Setenv("PATH", bin)
	return calls
}

func TestNvidiaBuildForKernelReadsTheRyokuHostReport(t *testing.T) {
	calls := fakeRootTools(t, "echo 'akmods: building' >&2\nprintf 'build=failed\\nkernel="+newNvidiaKernel+"\\nnewest_module_kernel="+oldNvidiaKernel+"\\nkey=/etc/pki/akmods/private/private_key.priv\\n'\n")

	result, err := runNvidiaBuildForKernel(newNvidiaKernel)
	if err != nil {
		t.Fatal(err)
	}
	want := kmod.BuildResult{Kernel: newNvidiaKernel, Built: false, NewestWithModule: oldNvidiaKernel}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	logged, _ := os.ReadFile(calls)
	if got := strings.TrimSpace(string(logged)); got != "kmod build-for-kernel --kernel "+newNvidiaKernel {
		t.Fatalf("ryoku-host argv = %q", got)
	}
}

func TestNvidiaBuildForKernelCarriesRyokuHostsReason(t *testing.T) {
	fakeRootTools(t, "echo 'ryoku-host: akmods is not installed' >&2\nexit 1\n")

	_, err := runNvidiaBuildForKernel(newNvidiaKernel)
	if err == nil || !strings.Contains(err.Error(), "akmods is not installed") {
		t.Fatalf("err = %v, want ryoku-host's reason", err)
	}
}

func TestNvidiaBuildFailureWithNoKernelLeftWithTheDriver(t *testing.T) {
	stubNvidiaMachine(t, true, kmod.BuildResult{Kernel: newNvidiaKernel}, oldNvidiaKernel, newNvidiaKernel)

	note := awaitNvidiaBuild(oldNvidiaKernel)
	if !strings.Contains(note, "build failed") || !strings.Contains(note, "no installed kernel has the driver") {
		t.Fatalf("note = %q, want a failure with no kernel to fall back to", note)
	}
}
