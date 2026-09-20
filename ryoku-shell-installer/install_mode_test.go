package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageMigrationPreservesUnknownAndEditedArtifacts(t *testing.T) {
	home := t.TempDir()
	e := &engine{f: &facts{distro: fedoraLinux, homeDir: home}}
	path := filepath.Join(home, ".local/bin/ryoku")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user script"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.checkPackageMigration(); err == nil {
		t.Fatal("untracked local CLI accepted")
	}
	if err := recordArtifacts(home, map[string]artifact{}); err != nil {
		t.Fatal(err)
	}
	if err := e.checkPackageMigration(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("later user edit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.checkPackageMigration(); err == nil {
		t.Fatal("edited source artifact accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "later user edit" {
		t.Fatal("preflight changed a user artifact")
	}
}

func TestSourceResumeDoesNotSkipPackageInstallation(t *testing.T) {
	f := &facts{distro: fedoraLinux, homeDir: t.TempDir(), prevRun: &runState{
		Mode: "source", Provider: "test", Completed: []string{"packages"},
	}}
	e := newEngine(f, &plan{resume: true, compositor: "test"}, true, "main", "")
	if e.state != nil {
		t.Fatal("source resume state reused in package mode")
	}
}

func TestFedoraInstallMode(t *testing.T) {
	for _, c := range []struct {
		mode, payload, ref string
		source             bool
	}{
		{"auto", "", "main", false},
		{"auto", "/checkout", "main", true},
		{"auto", "", "feature", true},
		{"packages", "/checkout", "main", false},
		{"source", "", "main", true},
	} {
		if got := sourceMode(fedoraLinux, c.mode, c.payload, c.ref); got != c.source {
			t.Errorf("sourceMode(%+v) = %v", c, got)
		}
	}
}

func TestCustomRepositoryRetainsSourceBuild(t *testing.T) {
	previous := repoURL
	t.Cleanup(func() { repoURL = previous })
	repoURL = "https://github.com/example/ryoku-fedora.git"
	if !sourceMode(fedoraLinux, "auto", "", "main") {
		t.Fatal("a custom publisher must not silently install the default publisher's RPMs")
	}
}

func TestFedoraRepositoryRequiresURLAndKeys(t *testing.T) {
	t.Setenv("RYOKU_RPM_BASE_URL", "")
	if _, _, _, err := fedoraRepositoryConfig(); err == nil {
		t.Fatal("missing URL accepted")
	}
	t.Setenv("RYOKU_RPM_BASE_URL", "https://packages.example.invalid/fedora")
	t.Setenv("RYOKU_COPR_FINGERPRINT", strings.Repeat("A", 40))
	t.Setenv("RYOKU_RPM_SIGNING_KEY", strings.Repeat("B", 40))
	if _, _, _, err := fedoraRepositoryConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RYOKU_RPM_BASE_URL", "https://packages.example.invalid/\n[other]")
	if _, _, _, err := fedoraRepositoryConfig(); err == nil {
		t.Fatal("injected repository accepted")
	}
}

func TestFedoraSourceStepsStillBuild(t *testing.T) {
	e := newEngine(&facts{distro: fedoraLinux, homeDir: t.TempDir()}, &plan{}, true, "main", "/checkout")
	found := false
	for _, step := range e.steps {
		if step.id == "build" {
			found = true
		}
		if step.id == "package-migration" {
			t.Fatal("source mode must not retire local artifacts")
		}
	}
	if !found {
		t.Fatal("checkout installation no longer builds local sources")
	}
}
