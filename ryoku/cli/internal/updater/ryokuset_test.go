package updater

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The set decides what `ryoku update` is allowed to touch, so every property
// here is load-bearing: only packages the RyokuCOPR repo serves, and only ones
// the box actually has.
func TestRyokuSet(t *testing.T) {
	for _, c := range []struct {
		name            string
		repo, installed []string
		want            []string
	}{
		{
			name:      "installed and served, sorted",
			repo:      []string{"ryoku-shell", "ryoku-desktop", "ryotunes"},
			installed: []string{"linux", "ryoku-desktop", "ryoku-shell", "firefox"},
			want:      []string{"ryoku-desktop", "ryoku-shell"},
		},
		{
			name:      "a package the repo serves but the box lacks is not installed by an update",
			repo:      []string{"ryoku-desktop", "asusctl"},
			installed: []string{"ryoku-desktop"},
			want:      []string{"ryoku-desktop"},
		},
		{
			name:      "a kernel is never in the set, because the repo never serves one",
			repo:      []string{"ryoku-desktop"},
			installed: []string{"kernel", "kernel-core", "linux-firmware", "ryoku-desktop"},
			want:      []string{"ryoku-desktop"},
		},
		{
			name:      "nothing served: empty, never a bare dnf target",
			repo:      nil,
			installed: []string{"ryoku-desktop"},
			want:      []string{},
		},
		{
			name:      "duplicate repo lines collapse",
			repo:      []string{"ryoku-desktop", "ryoku-desktop"},
			installed: []string{"ryoku-desktop"},
			want:      []string{"ryoku-desktop"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := ryokuSet(c.repo, c.installed)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ryokuSet = %v, want %v", got, c.want)
			}
		})
	}
}

// The dnf transcript of a release that added two Fedora-only Requires: the
// lane is restricted to RyokuCOPR, so nothing provides them.
func TestMissingDeps(t *testing.T) {
	out := `Problem 1: cannot install the best update candidate for package ryoku-desktop-0.4093-109.fc44.x86_64
  - nothing provides google-rubik-fonts needed by ryoku-desktop-0.4096-112.fc44.x86_64 from RyokuCOPR
  - nothing provides julietaula-montserrat-fonts needed by ryoku-desktop-0.4096-112.fc44.x86_64 from RyokuCOPR
Problem 2: package ryoku-desktop-hyprland-0.4096-112.fc44.x86_64 from RyokuCOPR requires ryoku-desktop = 0.4096-112.fc44, but none of the providers can be installed
  - nothing provides google-rubik-fonts needed by ryoku-desktop-0.4096-112.fc44.x86_64 from RyokuCOPR
  - nothing provides libfoo >= 2.1 needed by ryogami-0.4096-112.fc44.x86_64 from RyokuCOPR`
	want := []string{"google-rubik-fonts", "julietaula-montserrat-fonts", "libfoo >= 2.1"}
	if got := missingDeps(out); !reflect.DeepEqual(got, want) {
		t.Errorf("missingDeps = %v, want %v", got, want)
	}
	if got := missingDeps("Nothing to do."); got != nil {
		t.Errorf("missingDeps(clean) = %v, want nil", got)
	}
}

func TestDependencyArgs(t *testing.T) {
	if got := ryokuPlanArgs([]string{"ryoku-desktop"}); !reflect.DeepEqual(got,
		[]string{"dnf", "--assumeno", "--refresh", "--repo=RyokuCOPR", "distro-sync", "ryoku-desktop"}) {
		t.Errorf("plan: %v", got)
	}
	if got := depInstallArgs([]string{"google-rubik-fonts"}); !reflect.DeepEqual(got,
		[]string{"sudo", "dnf", "-y", "install", "google-rubik-fonts"}) {
		t.Errorf("deps: %v", got)
	}
}

// dnf exits 0 when it skips an uninstallable upgrade, so this is what tells a
// stuck update from a finished one.
func TestHeldBack(t *testing.T) {
	installed := evrMap("ryoku-desktop 0:0.4093-109.fc44\nryogami 0:0.4096-112.fc44\nryoku-shell 0:0.4097-113.fc44\n")
	latest := evrMap("ryoku-desktop 0.4095-111.fc44\nryoku-desktop 0.4096-112.fc44\nryogami 0.4096-112.fc44\nryoku-shell 0.4096-112.fc44\n")
	want := []updateItem{
		{Name: "ryoku-desktop", Old: "0:0.4093-109.fc44", New: "0.4096-112.fc44"},
		{Name: "ryoku-shell", Old: "0:0.4097-113.fc44", New: "0.4096-112.fc44"},
	}
	if got := heldBack(installed, latest); !reflect.DeepEqual(got, want) {
		t.Errorf("heldBack = %v, want %v", got, want)
	}
}

// The dry resolve of a dependency install, in dnf5's C-locale table: the new
// font is fine, the library it pulls along upgrades the base.
func TestBaseMoves(t *testing.T) {
	out := `Updating and loading repositories:
Repositories loaded.
Package                Arch   Version        Repository      Size
Installing:
 google-rubik-fonts    noarch 0:2.200-3.fc44 fedora     1.2 MiB
Installing dependencies:
 fontpackages-filesystem noarch 0:1.44-1.fc44 fedora   0 B
Upgrading:
 libfoo                x86_64 0:2.1-1.fc44   updates   300 KiB
   replacing libfoo    x86_64 0:2.0-1.fc44   fedora    290 KiB
Removing dependencies:
 libfoo-compat         x86_64 0:2.0-1.fc44   fedora     12 KiB

Transaction Summary:
 Installing:         2 packages
 Upgrading:          1 package
 Removing:           1 package
Operation aborted by the user.`
	if got, want := baseMoves(out), []string{"libfoo", "libfoo-compat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("baseMoves = %v, want %v", got, want)
	}
	if got := baseMoves("Package Arch\nInstalling:\n google-rubik-fonts noarch 0:2.200-3.fc44 fedora 1 MiB\n"); got != nil {
		t.Errorf("baseMoves(new only) = %v, want nil", got)
	}
}

// dnf's text is matched in English, so the dry resolve must run in the C
// locale; and a non-zero exit is a failure only when dnf resolved nothing.
func TestDryRun(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  fail) echo 'Failed to download metadata for repo RyokuCOPR' >&2; exit 1 ;;\n" +
		"esac\n" +
		"if [ \"$LC_ALL\" = C ]; then echo ' - nothing provides libfoo needed by ryogami-1-1.x86_64' >&2\n" +
		"else echo ' - nichts stellt libfoo bereit' >&2; fi\n" +
		"echo 'Operation aborted by the user.'\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "dnf"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	out, err := dryRun([]string{"dnf", "plan"})
	if err != nil {
		t.Fatalf("a declined transaction is a resolve, not an error: %v", err)
	}
	if got := missingDeps(out); !reflect.DeepEqual(got, []string{"libfoo"}) {
		t.Errorf("missingDeps under a German locale = %v, want [libfoo]", got)
	}
	if _, err := dryRun([]string{"dnf", "fail"}); err == nil || !strings.Contains(err.Error(), "Failed to download metadata") {
		t.Errorf("a resolve that never ran: err = %v, want dnf's last line", err)
	}
}
