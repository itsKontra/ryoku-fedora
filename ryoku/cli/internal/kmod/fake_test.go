package kmod

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"ryoku-cli/internal/host"
)

const testAkmodsGID = 973

// fakeRunner stands in for openssl and mokutil. openssl writes a key pair at
// the paths it is given, so the fake root sees what a real run leaves behind.
type fakeRunner struct {
	commands []string
	stdin    []string
	paths    map[string]bool
	answer   func(host.Command) host.Result
}

func (f *fakeRunner) Run(command host.Command) host.Result {
	f.commands = append(f.commands, argv(command))
	input := ""
	if command.Stdin != nil {
		data, _ := io.ReadAll(command.Stdin)
		input = string(data)
	}
	f.stdin = append(f.stdin, input)
	if command.Name == "openssl" {
		fakeOpenSSL(command.Args)
	}
	if f.answer != nil {
		return f.answer(command)
	}
	return host.Result{}
}

func (f *fakeRunner) LookPath(name string) bool {
	if f.paths == nil {
		return true
	}
	return f.paths[name]
}
func (f *fakeRunner) SignalGroup(int, syscall.Signal) error { return nil }
func (f *fakeRunner) Alive(int) bool                        { return false }

func (f *fakeRunner) ran(prefix string) bool {
	for _, command := range f.commands {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func argv(command host.Command) string {
	return strings.TrimSpace(command.Name + " " + strings.Join(command.Args, " "))
}

func fakeOpenSSL(args []string) {
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "-keyout":
			_ = os.WriteFile(args[i+1], []byte("new private key"), 0o600)
		case "-out":
			_ = os.WriteFile(args[i+1], []byte("new certificate"), 0o644)
		}
	}
}

// fakeOwners records ownership by path; tests do not run as root, so a real
// chown to root:akmods would fail.
type fakeOwners struct {
	owner map[string][2]int
}

func (f *fakeOwners) Owner(path string) (int, int, error) {
	if _, err := os.Lstat(path); err != nil {
		return 0, 0, err
	}
	ids, ok := f.owner[path]
	if !ok {
		return 1000, 1000, nil
	}
	return ids[0], ids[1], nil
}

func (f *fakeOwners) Chown(path string, uid, gid int) error {
	f.owner[path] = [2]int{uid, gid}
	return nil
}

type fakeMachine struct {
	root   string
	runner *fakeRunner
	owners *fakeOwners
}

func newFakeMachine(t *testing.T) *fakeMachine {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc", "passwd"), "root:x:0:0:root:/root:/bin/bash\n", 0o644)
	writeFile(t, filepath.Join(root, "etc", "group"), "root:x:0:\nwheel:x:10:\n", 0o644)
	return &fakeMachine{root: root, runner: &fakeRunner{}, owners: &fakeOwners{owner: map[string][2]int{}}}
}

func (f *fakeMachine) machine() *Machine {
	return New(Config{Root: f.root, Runner: f.runner, Owners: f.owners})
}

func (f *fakeMachine) path(rel string) string { return filepath.Join(f.root, rel) }

func (f *fakeMachine) withAkmodsGroup(t *testing.T) {
	t.Helper()
	writeFile(t, f.path("etc/group"), "root:x:0:\nwheel:x:10:\nakmods:x:973:\n", 0o644)
}

func (f *fakeMachine) withKey(t *testing.T, mode os.FileMode, uid, gid int) {
	t.Helper()
	writeFile(t, f.path("var/lib/dkms/mok.key"), "existing private key", 0o600)
	writeFile(t, f.path("var/lib/dkms/mok.pub"), "existing certificate", 0o644)
	if err := os.Chmod(f.path("var/lib/dkms/mok.key"), mode); err != nil {
		t.Fatal(err)
	}
	f.owners.owner[f.path("var/lib/dkms/mok.key")] = [2]int{uid, gid}
	f.owners.owner[f.path("var/lib/dkms/mok.pub")] = [2]int{0, 0}
}

// withSecureBoot writes the two efivars the way efivarfs presents them: a
// 4-byte attribute header, then the value byte.
func (f *fakeMachine) withSecureBoot(t *testing.T, secureBoot, setupMode byte) {
	t.Helper()
	dir := f.path("sys/firmware/efi/efivars")
	writeFile(t, filepath.Join(dir, "SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"), string([]byte{6, 0, 0, 0, secureBoot}), 0o644)
	writeFile(t, filepath.Join(dir, "SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c"), string([]byte{6, 0, 0, 0, setupMode}), 0o644)
}

// withMokutil answers `mokutil --test-key` with one of mokutil 0.7.2's
// verdicts; every other mokutil call succeeds.
func (f *fakeMachine) withMokutil(testKeyCode int, testKeyOutput string) {
	f.runner.answer = func(command host.Command) host.Result {
		if command.Name == "mokutil" && len(command.Args) > 0 && command.Args[0] == "--test-key" {
			return host.Result{Code: testKeyCode, Output: testKeyOutput}
		}
		return host.Result{}
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
