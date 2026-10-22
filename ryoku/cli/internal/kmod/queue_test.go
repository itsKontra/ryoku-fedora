package kmod

import (
	"testing"

	"ryoku-cli/internal/host"
)

// withMokutilQueue makes the fake mokutil remember an import, so a later
// --test-key reports the request the way firmware variables would.
func (f *fakeMachine) withMokutilQueue(initial Enrollment, failing string) {
	state := initial
	cert := f.path("var/lib/dkms/mok.pub")
	f.runner.answer = func(command host.Command) host.Result {
		if command.Name != "mokutil" || len(command.Args) == 0 {
			return host.Result{}
		}
		if command.Args[0] == failing {
			return host.Result{Code: 1}
		}
		switch command.Args[0] {
		case "--import":
			state = EnrollmentQueued
		case "--test-key":
			switch state {
			case EnrollmentEnrolled:
				return host.Result{Code: 1, Output: cert + " is already enrolled\n"}
			case EnrollmentQueued:
				return host.Result{Code: 1, Output: cert + " is already in the enrollment request\n"}
			case EnrollmentNotQueued:
				return host.Result{Output: cert + " is not enrolled\n"}
			}
			return host.Result{Code: 255}
		}
		return host.Result{}
	}
}

func (f *fakeMachine) mokutilChanges() []string {
	var changes []string
	for i, command := range f.runner.commands {
		if command == "mokutil --test-key "+f.path("var/lib/dkms/mok.pub") {
			continue
		}
		changes = append(changes, command+" <"+f.runner.stdin[i])
	}
	return changes
}

func TestQueueEnrollmentImportsTheKeyWithThePasswordAndNoTimeout(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withKey(t, 0o600, 0, 0)
	fake.withSecureBoot(t, 1, 0)
	fake.withMokutilQueue(EnrollmentNotQueued, "")
	result, err := fake.machine().QueueEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Queued || result.Status.Enrollment != EnrollmentQueued {
		t.Fatalf("queued = %v, enrollment = %q", result.Queued, result.Status.Enrollment)
	}
	want := []string{
		"mokutil --timeout -1 <",
		"mokutil --import " + fake.path("var/lib/dkms/mok.pub") + " <enroll\nenroll\n",
	}
	got := fake.mokutilChanges()
	if len(got) != len(want) {
		t.Fatalf("mokutil changes = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mokutil changes = %q, want %q", got, want)
		}
	}
}

func TestQueueEnrollmentLeavesTheFirmwareAloneWhenNothingIsDue(t *testing.T) {
	cases := []struct {
		name       string
		secureBoot byte
		setupMode  byte
		state      Enrollment
	}{
		{"secure boot off", 0, 0, EnrollmentNotQueued},
		{"setup mode", 1, 1, EnrollmentNotQueued},
		{"already enrolled", 1, 0, EnrollmentEnrolled},
		{"already queued", 1, 0, EnrollmentQueued},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMachine(t)
			fake.withKey(t, 0o600, 0, 0)
			fake.withSecureBoot(t, tc.secureBoot, tc.setupMode)
			fake.withMokutilQueue(tc.state, "")
			result, err := fake.machine().QueueEnrollment()
			if err != nil {
				t.Fatal(err)
			}
			if result.Queued {
				t.Fatal("reported a new enrollment")
			}
			if changes := fake.mokutilChanges(); len(changes) != 0 {
				t.Fatalf("mokutil changed firmware state: %q", changes)
			}
			if result.Status.Enrollment != tc.state {
				t.Fatalf("enrollment = %q, want %q", result.Status.Enrollment, tc.state)
			}
		})
	}
}

func TestQueueEnrollmentWithoutEFIDoesNothing(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withKey(t, 0o600, 0, 0)
	fake.withMokutilQueue(EnrollmentUnknown, "")
	result, err := fake.machine().QueueEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	if result.Queued || len(fake.mokutilChanges()) != 0 {
		t.Fatalf("a BIOS machine queued an enrollment: %q", fake.mokutilChanges())
	}
}

func TestQueueEnrollmentFailsWhenItCannotQueue(t *testing.T) {
	cases := []struct {
		name    string
		key     bool
		state   Enrollment
		failing string
		changes int
	}{
		{"no key to enroll", false, EnrollmentNotQueued, "", 0},
		{"mokutil cannot tell", true, EnrollmentUnknown, "", 0},
		{"timeout refused", true, EnrollmentNotQueued, "--timeout", 1},
		{"import refused", true, EnrollmentNotQueued, "--import", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMachine(t)
			if tc.key {
				fake.withKey(t, 0o600, 0, 0)
			}
			fake.withSecureBoot(t, 1, 0)
			fake.withMokutilQueue(tc.state, tc.failing)
			result, err := fake.machine().QueueEnrollment()
			if err == nil {
				t.Fatalf("no error, result %+v", result)
			}
			if result.Queued {
				t.Fatal("a failed queue reported success")
			}
			if changes := fake.mokutilChanges(); len(changes) != tc.changes {
				t.Fatalf("mokutil changes = %q, want %d", changes, tc.changes)
			}
		})
	}
}
