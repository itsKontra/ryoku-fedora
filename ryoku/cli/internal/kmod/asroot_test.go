package kmod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The report Execute writes is the one AsRoot reads back.
func TestBuildReportReadsWhatBuildForKernelPrints(t *testing.T) {
	fake := newFakeFedora(t, oldKernel, testKernel)
	fake.modules[oldKernel] = true
	fake.buildFor[testKernel] = false
	code, stdout, stderr := runCLI(fake.fakeMachine, append(BuildOptions{Kernel: testKernel}.Args(), "--root", fake.root)...)
	if code != 0 {
		t.Fatalf("build-for-kernel exit %d:\n%s", code, stderr)
	}
	got, err := ParseReport(stdout).Build()
	if err != nil {
		t.Fatal(err)
	}
	want := BuildResult{Kernel: testKernel, Built: false, NewestWithModule: oldKernel}
	if got != want {
		t.Fatalf("build = %+v, want %+v", got, want)
	}
}

func TestReportWithoutABuildLineIsAnError(t *testing.T) {
	if _, err := ParseReport("key=present\nkey_ok=yes\n").Build(); err == nil {
		t.Fatal("a status report read as a build")
	}
}

func TestBuildOptionsArgs(t *testing.T) {
	got := strings.Join(BuildOptions{Kernel: testKernel, Rebuild: true}.Args(), " ")
	if got != "build-for-kernel --kernel "+testKernel+" --rebuild" {
		t.Fatalf("args = %q", got)
	}
	if got := Advice(BuildOptions{}.Args()...); got != "sudo ryoku-host kmod build-for-kernel" {
		t.Fatalf("advice = %q", got)
	}
}

// fakeSudo puts a sudo on PATH that logs its argv and runs body.
func fakeSudo(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "sudo.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\n" + body
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return log
}

func TestAsRootRunsRyokuHostThroughSudoAndReadsItsReport(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root runs ryoku-host without sudo")
	}
	log := fakeSudo(t, "printf 'build=failed\\nkernel=k\\nnewest_module_kernel=old\\n'\nprintf 'akmods: building\\nBuilding rpms failed\\n' >&2\n")
	var progress strings.Builder
	build, detail, err := AsRoot{Host: "/usr/bin/ryoku-host", NonInteractive: true, Progress: &progress}.BuildForKernel(BuildOptions{Kernel: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if build != (BuildResult{Kernel: "k", NewestWithModule: "old"}) || detail != "Building rpms failed" {
		t.Fatalf("build = %+v, detail %q", build, detail)
	}
	if !strings.Contains(progress.String(), "akmods: building") {
		t.Fatalf("progress = %q", progress.String())
	}
	argv, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(argv)); got != "-n /usr/bin/ryoku-host kmod build-for-kernel --kernel k" {
		t.Fatalf("sudo argv = %q", got)
	}
}

func TestAsRootFailureCarriesRyokuHostsReason(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root runs ryoku-host without sudo")
	}
	fakeSudo(t, "echo 'dnf output' >&2\necho 'ryoku-host: akmods is not installed' >&2\nexit 1\n")
	_, _, err := AsRoot{}.Run("status")
	if err == nil || err.Error() != "ryoku-host: akmods is not installed" {
		t.Fatalf("err = %v, want ryoku-host's last line", err)
	}
}
