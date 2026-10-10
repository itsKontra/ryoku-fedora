package doctor

import "testing"

// The plan is the whole decision: it must stay silent where there is no reader
// (so a readerless desktop never builds an AUR package), report the module as
// present when it is, tell an RPM box there is no package (in check-only too, so
// it never promises an install it cannot do), ask before installing under
// checkOnly, and only signal an install when a reader is present, the module is
// missing, a package source exists, and it is not a check-only run.
func TestPlanFingerprintModule(t *testing.T) {
	cases := []struct {
		name        string
		reader      bool
		moduleOK    bool
		rpm         bool
		checkOnly   bool
		wantStatus  recStatus
		wantInstall bool
	}{
		{"no reader is a clean no-op", false, false, false, false, recOK, false},
		{"no reader even in check-only", false, false, false, true, recOK, false},
		{"reader with the module present is ok", true, true, false, false, recOK, false},
		{"rpm box with the module present is ok", true, true, true, false, recOK, false},
		{"reader, module missing, check-only reports would-fix", true, false, false, true, recWouldFix, false},
		{"reader, module missing, apply signals install", true, false, false, false, recOK, true},
		{"rpm box, module missing, check-only only notes", true, false, true, true, recNote, false},
		{"rpm box, module missing, apply only notes", true, false, true, false, recNote, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, install := planFingerprintModule(c.reader, c.moduleOK, c.rpm, c.checkOnly)
			if install != c.wantInstall {
				t.Fatalf("install = %v, want %v", install, c.wantInstall)
			}
			// When the plan defers to the caller for the install, the result is
			// the zero value and its status is not meaningful; only assert status
			// on the branches the plan resolves itself.
			if !install && res.status != c.wantStatus {
				t.Fatalf("status = %v, want %v", res.status, c.wantStatus)
			}
		})
	}
}
