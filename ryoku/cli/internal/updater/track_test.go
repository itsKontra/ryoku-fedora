package updater

import (
	"errors"
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

// initRyokuClone makes a git work tree at dir whose origin names ryoku-fedora,
// so ResolveRepo's fallback and a recorded pointer both accept it.
func initRyokuClone(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init")
	mustGit(t, dir, "remote", "add", "origin", "https://github.com/ryoku-dev/ryoku-fedora.git")
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

// sourceCheckout makes this box take its updates from a checkout under home: a
// clone, the recorded pointer to it and a RYOKU_CHANNEL=unstable-dev env file.
func sourceCheckout(t *testing.T, home string) string {
	t.Helper()
	clone := filepath.Join(home, "ryoku-fedora")
	initRyokuClone(t, clone)
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
	return clone
}

// stubTrack replaces what Track does outside the filesystem: the rpm query,
// the dnf install and update, and the session env. It returns counters for the
// install and update calls.
func stubTrack(t *testing.T, installed bool, installErr error) (installs, updates *int) {
	t.Helper()
	installs, updates = new(int), new(int)
	oldInstalled, oldInstall, oldUpdate, oldEnv := pkgInstalled, installRyokuDesktop, runChannelUpdate, clearSessionChannelEnv
	pkgInstalled = func(string) bool { return installed }
	installRyokuDesktop = func() error { *installs++; return installErr }
	runChannelUpdate = func() error { *updates++; return nil }
	clearSessionChannelEnv = func() {}
	t.Cleanup(func() {
		pkgInstalled, installRyokuDesktop, runChannelUpdate, clearSessionChannelEnv = oldInstalled, oldInstall, oldUpdate, oldEnv
	})
	return installs, updates
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

	clone := sourceCheckout(t, home)

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

// A source box without ryoku-desktop gets it installed before its checkout is
// retired; when the install fails, the box keeps updating from the checkout
// and no update runs.
func TestTrackFailedInstallKeepsSourceTracking(t *testing.T) {
	home := isolateHome(t)
	packagedRepo(t, sys.COPRServer)
	clone := sourceCheckout(t, home)
	installs, updates := stubTrack(t, false, errors.New("dnf failed"))

	if err := Track(sys.ChannelCOPR); err == nil {
		t.Fatal("Track: want an error when ryoku-desktop does not install")
	}
	if *installs != 1 || *updates != 0 {
		t.Fatalf("installs=%d updates=%d, want one install and no update", *installs, *updates)
	}
	if !sys.SourceTracked() || sys.ResolveRepo() != clone {
		t.Fatalf("SourceTracked=%v ResolveRepo=%q, want the checkout at %q kept", sys.SourceTracked(), sys.ResolveRepo(), clone)
	}
}

func TestTrackInstallsThenRetiresCheckout(t *testing.T) {
	home := isolateHome(t)
	packagedRepo(t, sys.COPRServer)
	sourceCheckout(t, home)
	installs, updates := stubTrack(t, false, nil)

	if err := Track(sys.ChannelCOPR); err != nil {
		t.Fatalf("Track: %v", err)
	}
	if *installs != 1 || *updates != 1 {
		t.Fatalf("installs=%d updates=%d, want one of each", *installs, *updates)
	}
	if sys.SourceTracked() {
		t.Fatal("still source tracked after a successful install")
	}
}

// Without ryoku-desktop and without the repo to install it from, Track refuses
// before it touches anything.
func TestTrackWithoutRepoRefuses(t *testing.T) {
	home := isolateHome(t)
	old := sys.RPMRepoFile
	sys.RPMRepoFile = filepath.Join(t.TempDir(), "missing.repo")
	t.Cleanup(func() { sys.RPMRepoFile = old })
	clone := sourceCheckout(t, home)
	installs, updates := stubTrack(t, false, nil)

	if err := Track(sys.ChannelCOPR); err == nil {
		t.Fatal("Track: want an error with no repo to install from")
	}
	if *installs != 0 || *updates != 0 {
		t.Fatalf("installs=%d updates=%d, want none", *installs, *updates)
	}
	if sys.ResolveRepo() != clone {
		t.Fatalf("ResolveRepo = %q, want the checkout kept", sys.ResolveRepo())
	}
	if _, err := os.Stat(sys.RPMRepoFile); !os.IsNotExist(err) {
		t.Fatalf("repo file written (err=%v)", err)
	}
}
