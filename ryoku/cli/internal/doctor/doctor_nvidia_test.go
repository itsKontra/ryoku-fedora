package doctor

import "testing"

func TestNvidiaDriverActiveForKepler(t *testing.T) {
	if nvidiaDriverActiveFor(true, false, []string{"nvidia"}) {
		t.Fatal("Kepler hardware without a driver package must not count as NVIDIA-driven")
	}
	if !nvidiaDriverActiveFor(true, true, []string{"nvidia"}) {
		t.Fatal("an installed Kepler-compatible NVIDIA package must remain active")
	}
}

func TestNvidiaAutostartMasked(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty file", "", false},
		{"normal autostart without mask", "[Desktop Entry]\nType=Application\nExec=nvidia-settings -l\n", false},
		{"Hidden=true", "[Desktop Entry]\nType=Application\nExec=nvidia-settings -l\nHidden=true\n", true},
		{"X-systemd-skip=true", "[Desktop Entry]\nType=Application\nExec=nvidia-settings -l\nX-systemd-skip=true\n", true},
		{"Hidden=false", "[Desktop Entry]\nType=Application\nExec=nvidia-settings -l\nHidden=false\n", false},
	}
	for _, c := range cases {
		if got := nvidiaAutostartMasked(c.content); got != c.want {
			t.Errorf("%s: nvidiaAutostartMasked(...) = %v, want %v", c.name, got, c.want)
		}
	}
}

// The sleep-units reconciler repairs preset drift (xorg-x11-drv-nvidia-power
// enables the three units), so a healthy box must read ok (idempotency: doctor never
// re-enables what is already enabled), and only the disabled units are
// named for the fix.
func TestPlanNvidiaSleepUnits(t *testing.T) {
	all := map[string]bool{"nvidia-suspend.service": true, "nvidia-hibernate.service": true, "nvidia-resume.service": true}
	enabled := func(states ...string) map[string]bool {
		m := map[string]bool{}
		for _, s := range states {
			m[s] = true
		}
		return m
	}
	cases := []struct {
		name        string
		nvidia      bool
		exists      map[string]bool
		enabled     map[string]bool
		wantMissing []string
		wantVerdict string
	}{
		{"mesa-only box stays silent", false, nil, nil, nil, "no proprietary NVIDIA driver in use"},
		{"nvidia-power absent: nothing to enable", true, map[string]bool{}, nil, nil, "the NVIDIA sleep units are not installed on this machine"},
		{"healthy box: all three enabled", true, all, enabled("nvidia-suspend.service", "nvidia-hibernate.service", "nvidia-resume.service"), nil, "the NVIDIA sleep units are enabled"},
		{"converted box: all three disabled", true, all, nil, []string{"nvidia-suspend.service", "nvidia-hibernate.service", "nvidia-resume.service"}, ""},
		{"partial drift: only resume disabled", true, all, enabled("nvidia-suspend.service", "nvidia-hibernate.service"), []string{"nvidia-resume.service"}, ""},
	}
	for _, c := range cases {
		missing, verdict := planNvidiaSleepUnits(c.nvidia, c.exists, c.enabled)
		if verdict != c.wantVerdict {
			t.Errorf("%s: verdict = %q, want %q", c.name, verdict, c.wantVerdict)
		}
		if len(missing) != len(c.wantMissing) {
			t.Errorf("%s: missing = %v, want %v", c.name, missing, c.wantMissing)
			continue
		}
		for i := range missing {
			if missing[i] != c.wantMissing[i] {
				t.Errorf("%s: missing[%d] = %s, want %s", c.name, i, missing[i], c.wantMissing[i])
			}
		}
	}
}

// pinKeplerHost fixes the RPM fact the Kepler reconciler reads, so the tests
// give the same answer on a Fedora workstation as on a CI runner.
// An RPM box keeps its own NVIDIA driver: the reconciler touches nothing even on
// Kepler hardware with the 580xx package present.
