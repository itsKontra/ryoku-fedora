package doctor

import (
	"errors"
	"testing"
)

func TestReconcileNvidiaSigned(t *testing.T) {
	oldStatus, oldEnroll, oldHost := nvidiaSignedStatus, enrollNvidiaKey, nvidiaSignedHost
	t.Cleanup(func() { nvidiaSignedStatus, enrollNvidiaKey, nvidiaSignedHost = oldStatus, oldEnroll, oldHost })
	nvidiaSignedHost = func() bool { return true }

	status := func(driver, secureboot, key string) string {
		return "gpu=turing\ndriver=" + driver + "\nsecureboot=" + secureboot + "\nkey=" + key + "\n"
	}
	cases := []struct {
		name      string
		status    string
		checkOnly bool
		enrollErr error
		want      recStatus
		enrolls   bool
	}{
		{"no supported GPU", "gpu=legacy\ndriver=none\nsecureboot=on\nkey=nocert\n", false, nil, recOK, false},
		{"host akmod driver is kept", status("host", "on", "nocert"), false, nil, recOK, false},
		{"GPU on nouveau gets the install hint", status("none", "on", "nocert"), false, nil, recNote, false},
		{"Secure Boot off needs no key", status("ryoku", "off", "missing"), false, nil, recOK, false},
		{"enrolled key is ok", status("ryoku", "on", "enrolled"), false, nil, recOK, false},
		{"pending key asks for the reboot", status("ryoku", "on", "pending"), false, nil, recWarn, false},
		{"missing key under --check", status("ryoku", "on", "missing"), true, nil, recWouldFix, false},
		{"missing key is re-queued", status("ryoku", "on", "missing"), false, nil, recFixed, true},
		{"failed enrollment reports", status("ryoku", "on", "missing"), false, errors.New("denied"), recFailed, true},
	}
	for _, c := range cases {
		enrolled := false
		nvidiaSignedStatus = func() string { return c.status }
		enrollNvidiaKey = func() error { enrolled = true; return c.enrollErr }
		got := reconcileNvidiaSigned(c.checkOnly)
		if got.status != c.want {
			t.Errorf("%s: status %v, want %v (%s)", c.name, got.status, c.want, got.detail)
		}
		if enrolled != c.enrolls {
			t.Errorf("%s: enrolled=%v, want %v", c.name, enrolled, c.enrolls)
		}
	}
}
