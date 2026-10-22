package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ryoku-cli/internal/kmod"
)

func (m *signingMachine) link(t *testing.T, machinePath, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(m.path(machinePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, m.path(machinePath)); err != nil {
		t.Fatal(err)
	}
}

func (m *signingMachine) linkTarget(t *testing.T, machinePath string) string {
	t.Helper()
	target, err := os.Readlink(m.path(machinePath))
	if err != nil {
		t.Fatalf("%s: %v", machinePath, err)
	}
	return target
}

func TestAkmodsKeyLinksAreRepointedToTheModuleSigningKey(t *testing.T) {
	cases := map[string]func(*testing.T, *signingMachine){
		"missing": func(*testing.T, *signingMachine) {},
		"dangling": func(t *testing.T, m *signingMachine) {
			m.link(t, kmod.AkmodsKeyPath, "/var/lib/dkms/old.key")
			m.link(t, kmod.AkmodsCertPath, "/var/lib/dkms/old.pub")
		},
		"pointing at a key akmods generated": func(t *testing.T, m *signingMachine) {
			writeSigningFile(t, m.path("etc/pki/akmods/private/fedora-1.priv"), "akmods key", 0o640)
			writeSigningFile(t, m.path("etc/pki/akmods/certs/fedora-1.der"), "akmods cert", 0o644)
			m.link(t, kmod.AkmodsKeyPath, "/etc/pki/akmods/private/fedora-1.priv")
			m.link(t, kmod.AkmodsCertPath, "/etc/pki/akmods/certs/fedora-1.der")
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			machine := newSigningMachine(t)
			machine.withAkmodsGroup(t)
			machine.withKey(t, 0o640, 0, testAkmodsGID)
			arrange(t, machine)
			calls := machine.use(t)

			todo := reconcileAkmodsKeyLinks(true)
			if todo.status != recWouldFix || calls.prepared != 0 || todo.remedy != "sudo ryoku-host kmod prepare-key" {
				t.Fatalf("check-only = %s %q / %q, prepared %d times; want a todo fixed by prepare-key", todo.status.label(), todo.detail, todo.remedy, calls.prepared)
			}
			fixed := reconcileAkmodsKeyLinks(false)
			if fixed.status != recFixed || calls.prepared != 1 {
				t.Fatalf("fix = %s %q, prepared %d times; want fixed once", fixed.status.label(), fixed.detail, calls.prepared)
			}
			if got := machine.linkTarget(t, kmod.AkmodsKeyPath); got != kmod.KeyPath {
				t.Fatalf("%s -> %s, want %s", kmod.AkmodsKeyPath, got, kmod.KeyPath)
			}
			if got := machine.linkTarget(t, kmod.AkmodsCertPath); got != kmod.CertPath {
				t.Fatalf("%s -> %s, want %s", kmod.AkmodsCertPath, got, kmod.CertPath)
			}
			if again := reconcileAkmodsKeyLinks(false); again.status != recOK || calls.prepared != 1 {
				t.Fatalf("second run = %s %q, prepared %d times; want ok", again.status.label(), again.detail, calls.prepared)
			}
		})
	}
}

func TestAkmodsKeyLinksAreNotNeededWithoutAkmods(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o600, 0, 0)
	calls := machine.use(t)

	if got := reconcileAkmodsKeyLinks(false); got.status != recOK || calls.prepared != 0 {
		t.Fatalf("result = %s %q, prepared %d times; want ok untouched", got.status.label(), got.detail, calls.prepared)
	}
	if _, err := os.Lstat(machine.path("etc/pki/akmods")); !os.IsNotExist(err) {
		t.Fatalf("doctor created /etc/pki/akmods: %v", err)
	}
}

// The desktop user cannot list akmods' 0750 key directories, so doctor asks
// root with sudo -n, which never prompts.
func TestAkmodsKeyLinksTheUserCannotReadAreReadAsRoot(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withAkmodsGroup(t)
	machine.withKey(t, 0o640, 0, testAkmodsGID)
	calls := machine.use(t)
	oldAsRoot := moduleSigningLinksAsRoot
	t.Cleanup(func() { moduleSigningLinksAsRoot = oldAsRoot })
	moduleSigningStatus = func() (kmod.Status, error) {
		status, err := machine.machine().Status()
		status.Key.Links = kmod.LinksUnreadable
		return status, err
	}

	moduleSigningLinksAsRoot = func() (kmod.LinkState, error) { return kmod.LinksForeign, nil }
	if got := reconcileAkmodsKeyLinks(false); got.status != recFixed || calls.prepared != 1 {
		t.Fatalf("foreign as root = %s %q, prepared %d times; want fixed", got.status.label(), got.detail, calls.prepared)
	}

	moduleSigningLinksAsRoot = func() (kmod.LinkState, error) { return kmod.LinksOK, nil }
	if got := reconcileAkmodsKeyLinks(false); got.status != recOK || calls.prepared != 1 {
		t.Fatalf("ok as root = %s %q, prepared %d times; want ok", got.status.label(), got.detail, calls.prepared)
	}

	moduleSigningLinksAsRoot = func() (kmod.LinkState, error) { return "", errors.New("sudo: a password is required") }
	got := reconcileAkmodsKeyLinks(false)
	if got.status != recNote || calls.prepared != 1 || got.remedy != "sudo ryoku-host kmod status" {
		t.Fatalf("no cached sudo = %s %q / %q, prepared %d times; want a note to read them as root", got.status.label(), got.detail, got.remedy, calls.prepared)
	}
	if !strings.Contains(got.detail, "root") {
		t.Fatalf("detail = %q, want it to say the links are root's to read", got.detail)
	}
}
