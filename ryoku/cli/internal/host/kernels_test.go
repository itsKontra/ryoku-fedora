package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledKernelsAreNewestFirstInRPMOrder(t *testing.T) {
	dir := t.TempDir()
	for _, kernel := range []string{"6.9.12-200.fc44.x86_64", "6.17.1-300.fc44.x86_64", "6.17.10-300.fc44.x86_64", "6.17.1-301.fc44.x86_64"} {
		if err := os.MkdirAll(filepath.Join(dir, kernel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, kernel, "vmlinuz"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "6.99.0-100.fc44.x86_64"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(InstalledKernels(dir), " ")
	want := "6.17.10-300.fc44.x86_64 6.17.1-301.fc44.x86_64 6.17.1-300.fc44.x86_64 6.9.12-200.fc44.x86_64"
	if got != want {
		t.Fatalf("kernels = %q, want %q (a directory without vmlinuz is no kernel)", got, want)
	}
	if got := InstalledKernels(filepath.Join(dir, "missing")); len(got) != 0 {
		t.Fatalf("missing modules dir listed %v", got)
	}
}
