package kmod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareKeyCreatesAMissingPairWithDKMSParameters(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withAkmodsGroup(t)
	result, err := fake.machine().PrepareKey()
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != KeyCreated {
		t.Fatalf("action = %q, want created", result.Action)
	}
	if !strings.HasPrefix(fake.runner.commands[0], "openssl ") {
		t.Fatalf("commands = %q, want an openssl run first", fake.runner.commands)
	}
	command := fake.runner.commands[0]
	for _, want := range []string{
		"openssl req -new -x509 -nodes -days 36500",
		"-subj /CN=DKMS module signing key",
		"-newkey rsa:2048",
		"-addext extendedKeyUsage=codeSigning",
		"-outform DER",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("openssl run %q lacks %q", command, want)
		}
	}
	if got := readFile(t, fake.path("var/lib/dkms/mok.key")); got != "new private key" {
		t.Fatalf("mok.key = %q", got)
	}
	if got := readFile(t, fake.path("var/lib/dkms/mok.pub")); got != "new certificate" {
		t.Fatalf("mok.pub = %q", got)
	}
	key := result.Status.Key
	if !key.OK() || key.Mode != 0o640 || key.Owner != "root:akmods" {
		t.Fatalf("new key is %04o %s, want 0640 root:akmods", key.Mode, key.Owner)
	}
	info, err := os.Stat(fake.path("var/lib/dkms/mok.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mok.pub mode = %04o, want 0644", info.Mode().Perm())
	}
	if leftovers, _ := filepath.Glob(fake.path("var/lib/dkms/.*")); len(leftovers) != 0 {
		t.Fatalf("key generation left %q behind", leftovers)
	}
}

func TestPrepareKeyRegeneratesAHalfPairLikeDKMS(t *testing.T) {
	fake := newFakeMachine(t)
	writeFile(t, fake.path("var/lib/dkms/mok.key"), "orphaned private key", 0o600)
	result, err := fake.machine().PrepareKey()
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != KeyCreated || readFile(t, fake.path("var/lib/dkms/mok.pub")) != "new certificate" {
		t.Fatalf("a key without its certificate was not regenerated: %q", result.Action)
	}
}

func TestPrepareKeyLeavesAnExistingKeyAndFixesItsAccess(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withAkmodsGroup(t)
	fake.withKey(t, 0o600, 0, 0)
	result, err := fake.machine().PrepareKey()
	if err != nil {
		t.Fatal(err)
	}
	if fake.runner.ran("openssl") {
		t.Fatal("an existing key pair was regenerated")
	}
	if got := readFile(t, fake.path("var/lib/dkms/mok.key")); got != "existing private key" {
		t.Fatalf("existing key changed to %q", got)
	}
	if result.Action != KeyFixed {
		t.Fatalf("action = %q, want fixed", result.Action)
	}
	if key := result.Status.Key; !key.OK() || key.Owner != "root:akmods" || key.Mode != 0o640 {
		t.Fatalf("key is %04o %s after prepare", key.Mode, key.Owner)
	}
}

func TestPrepareKeyTightensATooOpenKey(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withKey(t, 0o644, 0, testAkmodsGID)
	result, err := fake.machine().PrepareKey()
	if err != nil {
		t.Fatal(err)
	}
	if key := result.Status.Key; !key.OK() || key.Mode != 0o600 || key.Owner != "root:root" {
		t.Fatalf("key is %04o %s, want 0600 root:root", key.Mode, key.Owner)
	}
}

func TestPrepareKeyTwiceChangesNothing(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withAkmodsGroup(t)
	if _, err := fake.machine().PrepareKey(); err != nil {
		t.Fatal(err)
	}
	fake.runner.commands = nil
	before := readFile(t, fake.path("var/lib/dkms/mok.key"))
	result, err := fake.machine().PrepareKey()
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != KeyUnchanged {
		t.Fatalf("second prepare action = %q, want unchanged", result.Action)
	}
	if fake.runner.ran("openssl") {
		t.Fatalf("second prepare ran %q", fake.runner.commands)
	}
	if readFile(t, fake.path("var/lib/dkms/mok.key")) != before {
		t.Fatal("second prepare replaced the key")
	}
}
