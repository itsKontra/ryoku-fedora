package kmod

import (
	"bytes"
	"strings"
	"testing"
)

func runCLI(fake *fakeMachine, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, Config{Runner: fake.runner, Owners: fake.owners}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// The installers and the ISO parse these lines; changing a key or value is a
// contract change.
func TestStatusOutputContract(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withAkmodsGroup(t)
	fake.withKey(t, 0o600, 0, 0)
	fake.withSecureBoot(t, 1, 0)
	fake.withMokutilQueue(EnrollmentNotQueued, "")
	code, stdout, stderr := runCLI(fake, "status", "--root", fake.root)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := strings.Join([]string{
		"key=present",
		"key_mode=0600",
		"key_owner=root:root",
		"key_expected_mode=0640",
		"key_expected_owner=root:akmods",
		"key_ok=no",
		"secure_boot=enforcing",
		"enrollment=not_queued",
		"password=enroll",
		"akmods_links=missing",
		"nvidia_choice=",
	}, "\n") + "\n"
	if stdout != want {
		t.Fatalf("status output:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestStatusOutputWithoutAKey(t *testing.T) {
	fake := newFakeMachine(t)
	code, stdout, _ := runCLI(fake, "status", "--root", fake.root)
	want := strings.Join([]string{
		"key=absent",
		"key_mode=",
		"key_owner=",
		"key_expected_mode=0600",
		"key_expected_owner=root:root",
		"key_ok=no",
		"secure_boot=unsupported",
		"enrollment=unknown",
		"password=enroll",
		"akmods_links=not_needed",
		"nvidia_choice=",
	}, "\n") + "\n"
	if code != 0 || stdout != want {
		t.Fatalf("exit %d, output:\n%s\nwant:\n%s", code, stdout, want)
	}
}

func TestPrepareKeyAndQueueEnrollmentReportTheirActionThenStatus(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withSecureBoot(t, 1, 0)
	fake.withMokutilQueue(EnrollmentNotQueued, "")
	code, stdout, stderr := runCLI(fake, "prepare-key", "--root", fake.root)
	if code != 0 || !strings.HasPrefix(stdout, "action=created\nkey=present\n") || !strings.Contains(stdout, "\nkey_ok=yes\n") {
		t.Fatalf("prepare-key exit %d:\n%s%s", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI(fake, "queue-enrollment", "--root", fake.root)
	if code != 0 || !strings.HasPrefix(stdout, "action=queued\nkey=present\n") || !strings.Contains(stdout, "\nenrollment=queued\npassword=enroll\n") {
		t.Fatalf("queue-enrollment exit %d:\n%s%s", code, stdout, stderr)
	}
	code, stdout, _ = runCLI(fake, "queue-enrollment", "--root", fake.root)
	if code != 0 || !strings.HasPrefix(stdout, "action=none\n") {
		t.Fatalf("second queue-enrollment exit %d:\n%s", code, stdout)
	}
}

func TestCLIFailuresAndUsage(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withSecureBoot(t, 1, 0)
	code, stdout, stderr := runCLI(fake, "queue-enrollment", "--root", fake.root)
	if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "ryoku-host: ") {
		t.Fatalf("queue without a key: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, args := range [][]string{nil, {"bogus"}, {"status", "--root"}, {"status", "extra"}} {
		if code, _, _ := runCLI(fake, args...); code != 2 {
			t.Fatalf("%q exit %d, want usage 2", args, code)
		}
	}
}

func TestRecordChoiceIsReportedByStatus(t *testing.T) {
	fake := newFakeMachine(t)
	for _, choice := range []string{"driver", "nouveau"} {
		code, stdout, stderr := runCLI(fake, "record-choice", choice, "--root", fake.root)
		if code != 0 || !strings.HasPrefix(stdout, "nvidia_choice="+choice+"\nkey=") || !strings.HasSuffix(stdout, "\nnvidia_choice="+choice+"\n") {
			t.Fatalf("record-choice %s exit %d:\n%s%s", choice, code, stdout, stderr)
		}
		if got := readFile(t, fake.path("var/lib/ryoku/nvidia-choice")); got != choice+"\n" {
			t.Fatalf("nvidia-choice = %q", got)
		}
	}
	for _, args := range [][]string{{"record-choice"}, {"record-choice", "nvidia"}, {"record-choice", "driver", "--root"}} {
		if code, _, _ := runCLI(fake, args...); code != 2 {
			t.Fatalf("%q exit %d, want usage 2", args, code)
		}
	}
}
