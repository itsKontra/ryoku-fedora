package main

import "testing"

func TestPickVoxtypeRPM(t *testing.T) {
	rel := ghRelease{Tag: "v1.1.0"}
	for _, n := range []string{
		"SHA256SUMS.txt",
		"voxtype-1.1.0-1.x86_64.rpm.asc",
		"voxtype-1.1.0-linux-x86_64-avx2",
		"voxtype-1.1.0-1.x86_64.rpm",
	} {
		rel.Assets = append(rel.Assets, ghAsset{n, "https://example/" + n})
	}

	got, err := pickVoxtypeRPM(rel, "x86_64")
	if err != nil || got != "https://example/voxtype-1.1.0-1.x86_64.rpm" {
		t.Fatalf("x86_64: got %q, %v", got, err)
	}
	if _, err := pickVoxtypeRPM(rel, "aarch64"); err == nil {
		t.Fatal("aarch64: want an error when the release has no matching RPM")
	}
}

func TestRPMArch(t *testing.T) {
	for goarch, want := range map[string]string{"amd64": "x86_64", "arm64": "aarch64", "riscv64": "riscv64"} {
		if got := rpmArch(goarch); got != want {
			t.Errorf("rpmArch(%q) = %q, want %q", goarch, got, want)
		}
	}
}
