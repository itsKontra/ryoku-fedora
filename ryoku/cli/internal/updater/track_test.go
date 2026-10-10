package updater

import (
	"os"
	"path/filepath"
	"testing"

	"ryoku-cli/internal/sys"
)

// packagedRepo points sys.RPMRepoFile at a fixture whose RyokuCOPR baseurl is
// server, so the channel writers and readers stay hermetic under a temp HOME
// and never touch /etc.
func packagedRepo(t *testing.T, server string) {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "RyokuCOPR.repo")
	body := "[RyokuCOPR]\nname=Ryoku\nbaseurl=" + server + "\ngpgcheck=1\n"
	if err := os.WriteFile(repo, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := sys.RPMRepoFile
	sys.RPMRepoFile = repo
	t.Cleanup(func() { sys.RPMRepoFile = old })
}

// initRyokuArchClone makes a git work tree at dir whose origin names ryoku-arch,
// so ResolveRepo's fallback and a recorded pointer both accept it.
func initRyokuArchClone(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init")
	mustGit(t, dir, "remote", "add", "origin", "https://github.com/ryoku-dev/ryoku-arch.git")
}

// isolateHome points HOME and the XDG roots at fresh temp dirs and clears
// RYOKU_REPO, so a track test never reads or writes this box's real state.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RYOKU_REPO", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("RYOKU_CHANNEL", "")
	return home
}

// track copr rewrites a mirror baseurl back to the COPR and reports copr; a
// channel Fedora has no repository for is refused and the file is left alone.
// A packaged box has nothing to migrate.
func TestSwitchToPackageChannelRewritesServer(t *testing.T) {
	isolateHome(t)
	packagedRepo(t, "https://mirror.example/ryoku/fedora-$releasever-$basearch")

	if _, err := switchToPackageChannel("stable"); err == nil {
		t.Fatal("switch to stable: want an error, Fedora has only the COPR channel")
	}
	migrated, err := switchToPackageChannel(sys.ChannelCOPR)
	if err != nil {
		t.Fatalf("switch to copr: %v", err)
	}
	if migrated {
		t.Fatal("a packaged box has nothing to migrate")
	}
	if got := sys.PackagedChannel(); got != sys.ChannelCOPR {
		t.Fatalf("channel = %q, want copr", got)
	}
	if got := ryokuChannel(); got != sys.ChannelCOPR {
		t.Fatalf("ryokuChannel = %q, want copr", got)
	}
	if srv := sys.RyokuServer(); srv != sys.COPRServer {
		t.Fatalf("server = %q, want %q", srv, sys.COPRServer)
	}
}

// track copr on a box whose updates come from a checkout (a recorded pointer plus
// a RYOKU_CHANNEL=unstable-dev env file) migrates it onto packages: both are
// removed so the update path resolves to packages, not the checkout, while the
// clone directory itself is left on disk.
func TestSwitchToPackageChannelMigratesOffCheckout(t *testing.T) {
	home := isolateHome(t)
	packagedRepo(t, sys.COPRServer) // a checkout box with the COPR repo configured

	clone := filepath.Join(home, "ryoku-arch")
	initRyokuArchClone(t, clone)
	if err := os.MkdirAll(sys.StateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sys.StateDir(), "repo"), []byte(clone+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(sys.ConfigHome(), "environment.d", "ryoku.conf")
	if err := os.MkdirAll(filepath.Dir(envFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("RYOKU_CHANNEL=unstable-dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !sys.SourceTracked() || sys.ResolveRepo() != clone {
		t.Fatalf("precondition: SourceTracked=%v ResolveRepo=%q, want a checkout at %q", sys.SourceTracked(), sys.ResolveRepo(), clone)
	}

	migrated, err := switchToPackageChannel(sys.ChannelCOPR)
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if !migrated {
		t.Fatal("expected a migration off the checkout")
	}
	if _, err := os.Stat(filepath.Join(sys.StateDir(), "repo")); !os.IsNotExist(err) {
		t.Fatalf("repo pointer still present (err=%v)", err)
	}
	if sys.TrackedChannel() != "" {
		t.Fatalf("tracked channel still %q", sys.TrackedChannel())
	}
	if got := sys.ResolveRepo(); got != "" {
		t.Fatalf("ResolveRepo = %q, want packages (empty)", got)
	}
	if _, err := os.Stat(clone); err != nil {
		t.Fatalf("clone directory removed: %v", err)
	}
	if got := sys.PackagedChannel(); got != sys.ChannelCOPR {
		t.Fatalf("channel = %q, want copr", got)
	}
}
