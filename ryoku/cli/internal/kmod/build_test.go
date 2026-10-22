package kmod

import (
	"strings"
	"testing"
)

const (
	newKernel = "6.18.1-200.fc44.x86_64"
	oldKernel = "6.9.12-200.fc44.x86_64"
)

func TestBuildForKernelReportsTheNewestKernelWithAModule(t *testing.T) {
	cases := []struct {
		name       string
		builds     bool
		modules    []string
		wantBuilt  bool
		wantNewest string
	}{
		{"built", true, []string{oldKernel}, true, newKernel},
		{"failed, an older kernel still has the driver", false, []string{oldKernel, testKernel}, false, testKernel},
		{"failed, no kernel has the driver", false, nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeFedora(t, oldKernel, testKernel, newKernel)
			fake.withAkmodsGroup(t)
			fake.buildFor[newKernel] = tc.builds
			for _, kernel := range tc.modules {
				fake.modules[kernel] = true
			}
			result, err := fake.machine().BuildForKernel(BuildOptions{Kernel: newKernel})
			if err != nil {
				t.Fatal(err)
			}
			if result.Kernel != newKernel || result.Built != tc.wantBuilt || result.NewestWithModule != tc.wantNewest {
				t.Fatalf("got kernel %q built %v newest %q; want %q %v %q",
					result.Kernel, result.Built, result.NewestWithModule, newKernel, tc.wantBuilt, tc.wantNewest)
			}
			if !fake.runner.ran("akmods --force --kernels " + newKernel + " --akmod nvidia") {
				t.Fatalf("commands:\n%s", strings.Join(fake.runner.commands, "\n"))
			}
		})
	}
}

func TestBuildForKernelDefaultsToTheBootDefaultAndCanRebuild(t *testing.T) {
	fake := newFakeFedora(t, oldKernel, testKernel)
	if _, err := fake.machine().BuildForKernel(BuildOptions{Rebuild: true}); err != nil {
		t.Fatal(err)
	}
	if !fake.runner.ran("akmods --force --rebuild --kernels " + testKernel + " --akmod nvidia") {
		t.Fatalf("commands:\n%s", strings.Join(fake.runner.commands, "\n"))
	}
}

func TestBuildForKernelCreatesAkmodsRunDirectory(t *testing.T) {
	fake := newFakeFedora(t)
	fake.withAkmodsGroup(t)
	if _, err := fake.machine().BuildForKernel(BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if ids := fake.owners.owner[fake.path("run/akmods")]; ids != [2]int{0, testAkmodsGID} {
		t.Fatalf("/run/akmods owner = %v, want root:akmods", ids)
	}
}

func TestBuildForKernelNeedsAkmods(t *testing.T) {
	fake := newFakeFedora(t)
	fake.runner.paths = map[string]bool{}
	if _, err := fake.machine().BuildForKernel(BuildOptions{}); err == nil {
		t.Fatal("built without akmods")
	}
}

func TestBuildAndInstallOutputContract(t *testing.T) {
	fake := newFakeFedora(t, oldKernel, testKernel)
	fake.modules[oldKernel] = true
	fake.buildFor[testKernel] = false
	code, stdout, stderr := runCLI(fake.fakeMachine, "build-for-kernel", "--kernel", testKernel, "--root", fake.root)
	want := "build=failed\nkernel=" + testKernel + "\nnewest_module_kernel=" + oldKernel + "\nkey="
	if code != 0 || !strings.HasPrefix(stdout, want) || !strings.HasSuffix(stdout, "\nnvidia_choice=\n") {
		t.Fatalf("build-for-kernel exit %d:\n%s%s", code, stdout, stderr)
	}

	fake.buildFor[testKernel] = true
	code, stdout, stderr = runCLI(fake.fakeMachine, "install-driver", "--root", fake.root)
	if code != 0 || !strings.HasPrefix(stdout, "action=installed\nkernel="+testKernel+"\nkey=present\n") || !strings.Contains(stdout, "\nakmods_links=ok\n") {
		t.Fatalf("install-driver exit %d:\n%s%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "dnf") || strings.Contains(stdout, "akmods --") {
		t.Fatalf("progress leaked into the key=value answer:\n%s", stdout)
	}

	fake.gpuCode = 3
	code, stdout, stderr = runCLI(fake.fakeMachine, "install-driver", "--kernel", testKernel, "--root", fake.root)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "older than Turing") {
		t.Fatalf("pre-Turing install-driver exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, _, _ = runCLI(fake.fakeMachine, "install-driver", "--gpu-supported", "--root", fake.root)
	if code != 0 {
		t.Fatalf("install-driver --gpu-supported exit %d", code)
	}
	for _, args := range [][]string{{"install-driver", "--rebuild"}, {"build-for-kernel", "--gpu-supported"}, {"install-driver", "--kernel"}, {"status", "--kernel", testKernel}} {
		if code, _, _ := runCLI(fake.fakeMachine, args...); code != 2 {
			t.Fatalf("%q exit %d, want usage 2", args, code)
		}
	}
}
