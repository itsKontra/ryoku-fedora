package kmod

import (
	"os"
	"testing"
)

func TestStatusReportsAMissingKey(t *testing.T) {
	fake := newFakeMachine(t)
	status, err := fake.machine().Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Key.Present {
		t.Fatal("a machine without /var/lib/dkms/mok.key reports a key")
	}
	if status.Key.OK() {
		t.Fatal("a missing key cannot be in its expected state")
	}
}

func TestExpectedKeyAccessFollowsTheAkmodsGroup(t *testing.T) {
	cases := []struct {
		name       string
		akmods     bool
		mode       os.FileMode
		uid, gid   int
		ok         bool
		wantMode   os.FileMode
		wantOwner  string
		reportedAs string
	}{
		{"root only without akmods", false, 0o600, 0, 0, true, 0o600, "root:root", "root:root"},
		{"group readable without akmods is too open", false, 0o640, 0, 0, false, 0o600, "root:root", "root:root"},
		{"akmods group present and readable", true, 0o640, 0, testAkmodsGID, true, 0o640, "root:akmods", "root:akmods"},
		{"akmods cannot read a root-only key", true, 0o600, 0, 0, false, 0o640, "root:akmods", "root:root"},
		{"world readable with akmods is too open", true, 0o644, 0, testAkmodsGID, false, 0o640, "root:akmods", "root:akmods"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMachine(t)
			if tc.akmods {
				fake.withAkmodsGroup(t)
			}
			fake.withKey(t, tc.mode, tc.uid, tc.gid)
			status, err := fake.machine().Status()
			if err != nil {
				t.Fatal(err)
			}
			key := status.Key
			if !key.Present {
				t.Fatal("existing key reported missing")
			}
			if key.OK() != tc.ok {
				t.Fatalf("OK() = %v, want %v (mode %04o owner %s)", key.OK(), tc.ok, key.Mode, key.Owner)
			}
			if key.Mode != tc.mode || key.Owner != tc.reportedAs {
				t.Fatalf("reported %04o %s, want %04o %s", key.Mode, key.Owner, tc.mode, tc.reportedAs)
			}
			if key.WantMode != tc.wantMode || key.WantOwner != tc.wantOwner {
				t.Fatalf("expected state %04o %s, want %04o %s", key.WantMode, key.WantOwner, tc.wantMode, tc.wantOwner)
			}
		})
	}
}
