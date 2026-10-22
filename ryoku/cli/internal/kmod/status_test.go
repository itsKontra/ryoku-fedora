package kmod

import (
	"testing"
)

func TestSecureBootIsEnforcingOnlyOutsideSetupMode(t *testing.T) {
	cases := []struct {
		name                  string
		efi                   bool
		secureBoot, setupMode byte
		want                  SecureBoot
	}{
		{"enforcing", true, 1, 0, SecureBootEnforcing},
		{"setup mode enforces nothing", true, 1, 1, SecureBootOff},
		{"switched off", true, 0, 0, SecureBootOff},
		{"BIOS boot", false, 0, 0, SecureBootUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMachine(t)
			if tc.efi {
				fake.withSecureBoot(t, tc.secureBoot, tc.setupMode)
			}
			status, err := fake.machine().Status()
			if err != nil {
				t.Fatal(err)
			}
			if status.SecureBoot != tc.want {
				t.Fatalf("secure boot = %q, want %q", status.SecureBoot, tc.want)
			}
		})
	}
}

func TestEnrollmentStateComesFromMokutilTestKey(t *testing.T) {
	cert := "/var/lib/dkms/mok.pub"
	cases := []struct {
		name   string
		code   int
		output string
		want   Enrollment
	}{
		{"not enrolled", 0, cert + " is not enrolled\n", EnrollmentNotQueued},
		{"enrolled", 1, cert + " is already enrolled\n", EnrollmentEnrolled},
		{"trusted by db", 1, cert + " is already in db\n", EnrollmentEnrolled},
		{"queued", 1, cert + " is already in the enrollment request\n", EnrollmentQueued},
		{"blocked", 1, cert + " is blocked in dbx\n", EnrollmentUnknown},
		{"no EFI variables", 1, "EFI variables are not supported on this system\n", EnrollmentUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMachine(t)
			fake.withKey(t, 0o600, 0, 0)
			fake.withMokutil(tc.code, tc.output)
			status, err := fake.machine().Status()
			if err != nil {
				t.Fatal(err)
			}
			if status.Enrollment != tc.want {
				t.Fatalf("enrollment = %q, want %q", status.Enrollment, tc.want)
			}
			if want := "mokutil --test-key " + fake.path("var/lib/dkms/mok.pub"); fake.runner.commands[0] != want {
				t.Fatalf("ran %q, want %q", fake.runner.commands, want)
			}
		})
	}
}

func TestEnrollmentIsUnknownWithoutAKeyOrMokutil(t *testing.T) {
	fake := newFakeMachine(t)
	status, err := fake.machine().Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enrollment != EnrollmentUnknown || len(fake.runner.commands) != 0 {
		t.Fatalf("no key: enrollment %q after %q", status.Enrollment, fake.runner.commands)
	}

	fake = newFakeMachine(t)
	fake.withKey(t, 0o600, 0, 0)
	fake.runner.paths = map[string]bool{}
	status, err = fake.machine().Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enrollment != EnrollmentUnknown || len(fake.runner.commands) != 0 {
		t.Fatalf("no mokutil: enrollment %q after %q", status.Enrollment, fake.runner.commands)
	}
}

func TestStatusCarriesTheEnrollmentPassword(t *testing.T) {
	status, err := newFakeMachine(t).machine().Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Password != "enroll" {
		t.Fatalf("password = %q, want the documented enroll", status.Password)
	}
}
