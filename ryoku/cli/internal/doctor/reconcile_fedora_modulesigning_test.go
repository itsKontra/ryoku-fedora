package doctor

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"ryoku-cli/internal/host"
	"ryoku-cli/internal/kmod"
)

const testAkmodsGID = 973

// signingMachine is a fake Fedora root seen through kmod, with mokutil and
// file ownership simulated, so the reconcilers run kmod's real status, prepare
// and queue operations against it.
type signingMachine struct {
	root       string
	owners     map[string][2]int
	enrollment kmod.Enrollment
	mokutil    []string
	failQueue  bool
}

func newSigningMachine(t *testing.T) *signingMachine {
	t.Helper()
	root := t.TempDir()
	writeSigningFile(t, filepath.Join(root, "etc/passwd"), "root:x:0:0:root:/root:/bin/bash\n", 0o644)
	writeSigningFile(t, filepath.Join(root, "etc/group"), "root:x:0:\nwheel:x:10:\n", 0o644)
	return &signingMachine{root: root, owners: map[string][2]int{}, enrollment: kmod.EnrollmentNotQueued}
}

func (m *signingMachine) path(machinePath string) string { return filepath.Join(m.root, machinePath) }

func (m *signingMachine) withAkmodsGroup(t *testing.T) {
	t.Helper()
	writeSigningFile(t, m.path("etc/group"), "root:x:0:\nwheel:x:10:\nakmods:x:973:\n", 0o644)
}

func (m *signingMachine) withKey(t *testing.T, mode os.FileMode, uid, gid int) {
	t.Helper()
	writeSigningFile(t, m.path(kmod.KeyPath), "private key", 0o600)
	writeSigningFile(t, m.path(kmod.CertPath), "certificate", 0o644)
	if err := os.Chmod(m.path(kmod.KeyPath), mode); err != nil {
		t.Fatal(err)
	}
	m.owners[m.path(kmod.KeyPath)] = [2]int{uid, gid}
	m.owners[m.path(kmod.CertPath)] = [2]int{0, 0}
}

func (m *signingMachine) withSecureBoot(t *testing.T, on bool) {
	t.Helper()
	value := byte(0)
	if on {
		value = 1
	}
	dir := m.path("sys/firmware/efi/efivars")
	writeSigningFile(t, filepath.Join(dir, "SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"), string([]byte{6, 0, 0, 0, value}), 0o644)
	writeSigningFile(t, filepath.Join(dir, "SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c"), string([]byte{6, 0, 0, 0, 0}), 0o644)
}

func (m *signingMachine) keyMode(t *testing.T) os.FileMode {
	t.Helper()
	info, err := os.Stat(m.path(kmod.KeyPath))
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func (m *signingMachine) machine() *kmod.Machine {
	return kmod.New(kmod.Config{Root: m.root, Runner: signingRunner{m}, Owners: signingOwners{m}})
}

// use points doctor's seams at this machine. The root operations run kmod
// directly, as `sudo ryoku-host kmod ...` would on a real machine.
func (m *signingMachine) use(t *testing.T) *signingCalls {
	t.Helper()
	oldStatus, oldPrepare, oldQueue := moduleSigningStatus, prepareModuleSigningKey, queueModuleSigningEnrollment
	t.Cleanup(func() {
		moduleSigningStatus, prepareModuleSigningKey, queueModuleSigningEnrollment = oldStatus, oldPrepare, oldQueue
	})
	calls := &signingCalls{}
	moduleSigningStatus = func() (kmod.Status, error) { return m.machine().Status() }
	prepareModuleSigningKey = func() error {
		calls.prepared++
		_, err := m.machine().PrepareKey()
		return err
	}
	queueModuleSigningEnrollment = func() error {
		calls.queued++
		_, err := m.machine().QueueEnrollment()
		return err
	}
	return calls
}

type signingCalls struct {
	prepared, queued int
}

type signingRunner struct{ m *signingMachine }

func (r signingRunner) Run(command host.Command) host.Result {
	if command.Name != "mokutil" || len(command.Args) == 0 {
		return host.Result{Code: 127}
	}
	if command.Stdin != nil {
		_, _ = io.ReadAll(command.Stdin)
	}
	r.m.mokutil = append(r.m.mokutil, strings.Join(command.Args, " "))
	cert := r.m.path(kmod.CertPath)
	switch command.Args[0] {
	case "--import":
		if r.m.failQueue {
			return host.Result{Code: 1}
		}
		r.m.enrollment = kmod.EnrollmentQueued
	case "--test-key":
		switch r.m.enrollment {
		case kmod.EnrollmentEnrolled:
			return host.Result{Code: 1, Output: cert + " is already enrolled\n"}
		case kmod.EnrollmentQueued:
			return host.Result{Code: 1, Output: cert + " is already in the enrollment request\n"}
		case kmod.EnrollmentNotQueued:
			return host.Result{Output: cert + " is not enrolled\n"}
		}
		return host.Result{Code: 255}
	}
	return host.Result{}
}

func (signingRunner) LookPath(name string) bool             { return name == "mokutil" }
func (signingRunner) SignalGroup(int, syscall.Signal) error { return nil }
func (signingRunner) Alive(int) bool                        { return false }

// signingOwners keeps ownership in memory: tests do not run as root.
type signingOwners struct{ m *signingMachine }

func (o signingOwners) Owner(path string) (int, int, error) {
	if _, err := os.Lstat(path); err != nil {
		return 0, 0, err
	}
	ids, ok := o.m.owners[path]
	if !ok {
		return 1000, 1000, nil
	}
	return ids[0], ids[1], nil
}

func (o signingOwners) Chown(path string, uid, gid int) error {
	o.m.owners[path] = [2]int{uid, gid}
	return nil
}

func writeSigningFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestModuleSigningKeyTooOpenIsTightened(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withAkmodsGroup(t)
	machine.withKey(t, 0o644, 0, testAkmodsGID)
	calls := machine.use(t)

	todo := reconcileModuleSigningKey(true)
	if todo.status != recWouldFix || calls.prepared != 0 || machine.keyMode(t) != 0o644 {
		t.Fatalf("check-only = %s %q, prepared %d times, mode %04o; want todo with nothing changed", todo.status.label(), todo.detail, calls.prepared, machine.keyMode(t))
	}
	if !strings.Contains(todo.detail, "more open") || todo.remedy != "sudo ryoku-host kmod prepare-key" {
		t.Fatalf("check-only = %q / %q, want a too-open finding fixed by prepare-key", todo.detail, todo.remedy)
	}

	fixed := reconcileModuleSigningKey(false)
	if fixed.status != recFixed || calls.prepared != 1 {
		t.Fatalf("fix = %s %q, prepared %d times; want fixed once", fixed.status.label(), fixed.detail, calls.prepared)
	}
	if mode := machine.keyMode(t); mode != 0o640 {
		t.Fatalf("key mode after fix = %04o, want 0640", mode)
	}
	if again := reconcileModuleSigningKey(false); again.status != recOK || calls.prepared != 1 {
		t.Fatalf("second run = %s %q, prepared %d times; want ok without another prepare", again.status.label(), again.detail, calls.prepared)
	}
}

func TestModuleSigningKeyAkmodsCannotReadIsLoosened(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withAkmodsGroup(t)
	machine.withKey(t, 0o600, 0, 0)
	calls := machine.use(t)

	todo := reconcileModuleSigningKey(true)
	if todo.status != recWouldFix || calls.prepared != 0 || !strings.Contains(todo.detail, "akmods cannot read") {
		t.Fatalf("check-only = %s %q, prepared %d times; want an akmods-cannot-read todo", todo.status.label(), todo.detail, calls.prepared)
	}
	if fixed := reconcileModuleSigningKey(false); fixed.status != recFixed {
		t.Fatalf("fix = %s %q, want fixed", fixed.status.label(), fixed.detail)
	}
	if mode, owner := machine.keyMode(t), machine.owners[machine.path(kmod.KeyPath)]; mode != 0o640 || owner != [2]int{0, testAkmodsGID} {
		t.Fatalf("key after fix = %04o %v, want 0640 root:akmods", mode, owner)
	}
}

func TestModuleSigningKeyWithoutAkmodsIsRootOnly(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o640, 0, 10)
	machine.use(t)

	todo := reconcileModuleSigningKey(true)
	if todo.status != recWouldFix || !strings.Contains(todo.detail, "more open than 0600 root:root") {
		t.Fatalf("check-only = %s %q, want a too-open todo against 0600 root:root", todo.status.label(), todo.detail)
	}
	if fixed := reconcileModuleSigningKey(false); fixed.status != recFixed {
		t.Fatalf("fix = %s %q, want fixed", fixed.status.label(), fixed.detail)
	}
	if mode, owner := machine.keyMode(t), machine.owners[machine.path(kmod.KeyPath)]; mode != 0o600 || owner != [2]int{0, 0} {
		t.Fatalf("key after fix = %04o %v, want 0600 root:root", mode, owner)
	}
}

func TestModuleSigningKeyFixThatFailsIsReported(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o644, 0, 0)
	machine.use(t)
	prepareModuleSigningKey = func() error { return errors.New("sudo: a password is required") }

	got := reconcileModuleSigningKey(false)
	if got.status != recFailed || !strings.Contains(got.detail, "a password is required") || got.remedy != "sudo ryoku-host kmod prepare-key" {
		t.Fatalf("result = %s %q / %q, want a failure naming the error and the command", got.status.label(), got.detail, got.remedy)
	}
	if mode := machine.keyMode(t); mode != 0o644 {
		t.Fatalf("key mode = %04o, want it untouched", mode)
	}
}

func TestModuleSigningKeyAbsentIsLeftAlone(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withAkmodsGroup(t)
	calls := machine.use(t)

	if got := reconcileModuleSigningKey(false); got.status != recOK || calls.prepared != 0 {
		t.Fatalf("result = %s %q, prepared %d times; want ok without creating a key", got.status.label(), got.detail, calls.prepared)
	}
	if _, err := os.Stat(machine.path(kmod.KeyPath)); !os.IsNotExist(err) {
		t.Fatalf("doctor created a module signing key: %v", err)
	}
}

func TestModuleSigningEnrollmentNotQueuedIsQueued(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o600, 0, 0)
	machine.withSecureBoot(t, true)
	calls := machine.use(t)

	todo := reconcileModuleSigningEnrollment(true)
	if todo.status != recWouldFix || calls.queued != 0 || todo.remedy != "sudo ryoku-host kmod queue-enrollment" {
		t.Fatalf("check-only = %s %q / %q, queued %d times; want a todo fixed by queue-enrollment", todo.status.label(), todo.detail, todo.remedy, calls.queued)
	}
	if machine.enrollment != kmod.EnrollmentNotQueued {
		t.Fatalf("check-only changed the enrollment to %q", machine.enrollment)
	}

	fixed := reconcileModuleSigningEnrollment(false)
	if fixed.status != recFixed || calls.queued != 1 || machine.enrollment != kmod.EnrollmentQueued {
		t.Fatalf("fix = %s %q, queued %d times, enrollment %q; want it queued once", fixed.status.label(), fixed.detail, calls.queued, machine.enrollment)
	}
	if !strings.Contains(fixed.detail, "reboot") || !strings.Contains(fixed.detail, "password enroll") {
		t.Fatalf("fix detail = %q, want reboot and approve with password enroll", fixed.detail)
	}
}

func TestModuleSigningEnrollmentQueuedIsANote(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o600, 0, 0)
	machine.withSecureBoot(t, true)
	machine.enrollment = kmod.EnrollmentQueued
	calls := machine.use(t)

	for _, checkOnly := range []bool{true, false} {
		got := reconcileModuleSigningEnrollment(checkOnly)
		if got.status != recNote || calls.queued != 0 || !strings.Contains(got.detail, "password enroll") {
			t.Fatalf("checkOnly=%v: %s %q, queued %d times; want a note naming password enroll", checkOnly, got.status.label(), got.detail, calls.queued)
		}
	}
}

func TestModuleSigningEnrollmentEnrolledIsOK(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o600, 0, 0)
	machine.withSecureBoot(t, true)
	machine.enrollment = kmod.EnrollmentEnrolled
	calls := machine.use(t)

	for _, checkOnly := range []bool{true, false} {
		if got := reconcileModuleSigningEnrollment(checkOnly); got.status != recOK || calls.queued != 0 {
			t.Fatalf("checkOnly=%v: %s %q, queued %d times; want ok", checkOnly, got.status.label(), got.detail, calls.queued)
		}
	}
}

func TestModuleSigningEnrollmentWaitsForSecureBoot(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o600, 0, 0)
	machine.withSecureBoot(t, false)
	calls := machine.use(t)

	for _, checkOnly := range []bool{true, false} {
		got := reconcileModuleSigningEnrollment(checkOnly)
		if got.status != recOK || calls.queued != 0 || machine.enrollment != kmod.EnrollmentNotQueued {
			t.Fatalf("checkOnly=%v: %s %q, queued %d times; want ok and nothing queued", checkOnly, got.status.label(), got.detail, calls.queued)
		}
		if !strings.Contains(got.detail, "once Secure Boot is turned on") {
			t.Fatalf("detail = %q, want it to say the enrollment waits for Secure Boot", got.detail)
		}
	}
}

func TestModuleSigningEnrollmentQueueThatFailsIsReported(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withKey(t, 0o600, 0, 0)
	machine.withSecureBoot(t, true)
	machine.failQueue = true
	machine.use(t)

	got := reconcileModuleSigningEnrollment(false)
	if got.status != recFailed || got.remedy != "sudo ryoku-host kmod queue-enrollment" {
		t.Fatalf("result = %s %q / %q, want a failure with the queue command", got.status.label(), got.detail, got.remedy)
	}
}

func TestModuleSigningEnrollmentWithoutAKeyIsLeftAlone(t *testing.T) {
	machine := newSigningMachine(t)
	machine.withSecureBoot(t, true)
	calls := machine.use(t)

	if got := reconcileModuleSigningEnrollment(false); got.status != recOK || calls.queued != 0 {
		t.Fatalf("result = %s %q, queued %d times; want ok with nothing queued", got.status.label(), got.detail, calls.queued)
	}
}

func TestModuleSigningReconcilersRunOnlyOnFedoraKeyFirst(t *testing.T) {
	oldManager := doctorPackageManager
	t.Cleanup(func() { doctorPackageManager = oldManager })

	for _, manager := range []host.PackageManager{host.DNF, host.Pacman, host.XBPS} {
		doctorPackageManager = func() (host.PackageManager, error) { return manager, nil }
		position := map[string]int{}
		for i, r := range reconcilers() {
			position[r.name] = i + 1
		}
		key, enrollment := position["module signing key permissions"], position["module signing key enrollment"]
		if manager != host.DNF {
			if key != 0 || enrollment != 0 {
				t.Fatalf("%s registers the module signing reconcilers", manager)
			}
			continue
		}
		if key == 0 || enrollment == 0 || key > enrollment {
			t.Fatalf("Fedora pass positions: key permissions %d, enrollment %d; want both, key first", key, enrollment)
		}
	}
}
