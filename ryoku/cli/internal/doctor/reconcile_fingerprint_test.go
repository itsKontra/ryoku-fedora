package doctor

import "testing"

// The plan stays silent where there is no reader, reports the module as present
// when it is, and otherwise notes that no package provides it.
func TestPlanFingerprintModule(t *testing.T) {
	cases := []struct {
		name       string
		reader     bool
		moduleOK   bool
		wantStatus recStatus
	}{
		{"no reader is a clean no-op", false, false, recOK},
		{"reader with the module present is ok", true, true, recOK},
		{"reader with the module missing only notes", true, false, recNote},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := planFingerprintModule(c.reader, c.moduleOK); got.status != c.wantStatus {
				t.Fatalf("status = %v, want %v", got.status, c.wantStatus)
			}
		})
	}
}
