package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fedoraNvidiaFacts(generation string) *facts {
	return &facts{distro: newFedoraLinux(), distroID: "fedora", hasNvidia: true, nvidiaGeneration: generation}
}

func nvidiaDriverItem(t *testing.T, f *facts) (planItem, *plan, bool) {
	t.Helper()
	p := defaultPlan(f)
	for _, item := range buildItems(f, p) {
		if item.label == "NVIDIA driver" {
			return item, p, true
		}
		if item.label == "NVIDIA proprietary drivers" {
			t.Fatal("Fedora shows the Arch NVIDIA item")
		}
	}
	return planItem{}, p, false
}

func TestFedoraNvidiaDriverItemIsOnForASupportedGPU(t *testing.T) {
	item, p, ok := nvidiaDriverItem(t, fedoraNvidiaFacts("supported"))
	if !ok {
		t.Fatal("no NVIDIA driver item on a Fedora machine with an NVIDIA GPU")
	}
	if !p.nvidia || !*item.on || item.locked {
		t.Fatalf("supported GPU: on %v, locked %v", p.nvidia, item.locked)
	}
	if !strings.Contains(item.detail, "nouveau") {
		t.Fatalf("detail does not name the nouveau opt-out: %q", item.detail)
	}
	if strings.Contains(item.detail, "MokManager") {
		t.Fatalf("detail promises an enrollment without Secure Boot: %q", item.detail)
	}
}

func TestFedoraNvidiaDriverItemIsLockedOffBeforeTuring(t *testing.T) {
	item, p, ok := nvidiaDriverItem(t, fedoraNvidiaFacts("unsupported"))
	if !ok || p.nvidia || *item.on || !item.locked {
		t.Fatalf("pre-Turing GPU: shown %v, on %v, locked %v", ok, p.nvidia, item.locked)
	}
	if !strings.Contains(item.detail, "Turing") {
		t.Fatalf("no explanation for the lock: %q", item.detail)
	}
}

func TestFedoraNvidiaDriverItemWithAnUnknownGeneration(t *testing.T) {
	item, p, ok := nvidiaDriverItem(t, fedoraNvidiaFacts("unknown"))
	if !ok || p.nvidia || item.locked {
		t.Fatalf("unknown generation: shown %v, on %v, locked %v", ok, p.nvidia, item.locked)
	}
}

func TestFedoraNvidiaDriverItemIsNeverLockedBySecureBoot(t *testing.T) {
	f := fedoraNvidiaFacts("supported")
	f.secureBoot = true
	f.enrollmentPassword = "enroll"
	item, p, ok := nvidiaDriverItem(t, f)
	if !ok || !p.nvidia || item.locked {
		t.Fatalf("Secure Boot: shown %v, on %v, locked %v", ok, p.nvidia, item.locked)
	}
	for _, want := range []string{"Secure Boot", "MokManager", "enroll"} {
		if !strings.Contains(item.detail, want) {
			t.Fatalf("Secure Boot detail lacks %q: %q", want, item.detail)
		}
	}
}

func TestFedoraWithoutNvidiaHasNoDriverItem(t *testing.T) {
	f := fedoraNvidiaFacts("")
	f.hasNvidia = false
	if _, p, ok := nvidiaDriverItem(t, f); ok || p.nvidia {
		t.Fatal("NVIDIA driver item on a machine without an NVIDIA GPU")
	}
}

func TestNvidiaGenerationComesFromTheDriverScriptsQuery(t *testing.T) {
	for code, want := range map[string]string{"0": "supported", "3": "unsupported", "4": "absent", "5": "unknown", "1": "unknown"} {
		script := "[ \"$1\" = --query ] || exit 9\nexit " + code + "\n"
		if got := queryNvidiaGeneration([]byte(script)); got != want {
			t.Errorf("exit %s read as %q, want %q", code, got, want)
		}
	}
	if got := queryNvidiaGeneration(nil); got != "unknown" {
		t.Errorf("no script read as %q", got)
	}
}

func TestNvidiaQueryScriptLoadsFromThePayload(t *testing.T) {
	payload := t.TempDir()
	path := filepath.Join(payload, "system", "hardware", "drivers", "nvidia.sh")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("exit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := string(loadNvidiaQueryScript(nil, "main", payload)); got != "exit 0\n" {
		t.Fatalf("script = %q", got)
	}
}

func TestFedoraDriversInstallTheDriverWhenTheItemIsOn(t *testing.T) {
	home, logPath := fakeKmodHost(t)
	t.Setenv("ACTION", "queued")
	t.Setenv("ENROLLMENT", "queued")
	_, output := runFedoraDriversWith(t, home, true, true)
	want := "kmod prepare-key\nkmod install-driver\nkmod queue-enrollment\nkmod record-choice driver"
	if got := hostCalls(t, logPath); got != want {
		t.Fatalf("ryoku-host calls = %q, want %q", got, want)
	}
	if strings.Contains(output, "keeps NVIDIA on nouveau") {
		t.Fatalf("still says Fedora keeps nouveau:\n%s", output)
	}
}

func TestFedoraDriversRecordNouveauWhenTheItemIsOff(t *testing.T) {
	home, logPath := fakeKmodHost(t)
	t.Setenv("ACTION", "none")
	t.Setenv("ENROLLMENT", "not_queued")
	runFedoraDriversWith(t, home, true, false)
	want := "kmod prepare-key\nkmod queue-enrollment\nkmod record-choice nouveau"
	if got := hostCalls(t, logPath); got != want {
		t.Fatalf("ryoku-host calls = %q, want %q", got, want)
	}
}

func TestFedoraDriversWarnAndContinueWhenTheDriverFails(t *testing.T) {
	home, logPath := fakeKmodHost(t)
	t.Setenv("ACTION", "queued")
	t.Setenv("ENROLLMENT", "queued")
	t.Setenv("FAIL_VERB", "install-driver")
	e, output := runFedoraDriversWith(t, home, true, true)
	want := "kmod prepare-key\nkmod install-driver\nkmod queue-enrollment\nkmod record-choice driver"
	if got := hostCalls(t, logPath); got != want {
		t.Fatalf("ryoku-host calls = %q, want %q", got, want)
	}
	if !strings.Contains(output, "warning:") || len(enrollmentSummary(e)) == 0 {
		t.Fatalf("a failed driver did not warn, or dropped the enrollment steps:\n%s", output)
	}
}

func TestFedoraDriversRecordNoChoiceWithoutNvidia(t *testing.T) {
	home, logPath := fakeKmodHost(t)
	t.Setenv("ACTION", "none")
	t.Setenv("ENROLLMENT", "not_queued")
	runFedoraDriversWith(t, home, false, false)
	if got := hostCalls(t, logPath); got != "kmod prepare-key\nkmod queue-enrollment" {
		t.Fatalf("ryoku-host calls = %q", got)
	}
}
