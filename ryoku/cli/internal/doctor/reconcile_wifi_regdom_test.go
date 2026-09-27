package doctor

import (
	"strings"
	"testing"
)

// stubWifi swaps the impure inputs the reconciler reads (radio presence, the
// effective domain, and the timezone country) for fixtures, restoring the real
// ones when the test ends. It keeps every case hermetic: no real /sys, iw, or
// /etc/localtime or tzdata is touched.
func stubWifi(t *testing.T, radio bool, domain, source string, ok bool, country string) {
	t.Helper()
	origRadio, origRegdom, origTZ, origHelper := wifiRadioPresent, wifiRegdom, wifiTimezoneCountry, wifiRegdomHelperPresent
	t.Cleanup(func() {
		wifiRadioPresent = origRadio
		wifiRegdom = origRegdom
		wifiTimezoneCountry = origTZ
		wifiRegdomHelperPresent = origHelper
	})
	wifiRadioPresent = func() bool { return radio }
	wifiRegdom = func() (string, string, bool) { return domain, source, ok }
	wifiTimezoneCountry = func() string { return country }
	wifiRegdomHelperPresent = func() bool { return true }
}

// A desktop with no radio must never see this reconciler at all.
func TestRegdomSkipsBoxWithNoRadio(t *testing.T) {
	stubWifi(t, false, "", "", false, "US")
	if got := reconcileWifiRegdom(true); got.status != recOK {
		t.Errorf("no radio should be ok, got %v (%s)", got.status, got.detail)
	}
}

// A box already on a real country is healthy: report ok and change nothing.
func TestRegdomHealthyDomainLeftAlone(t *testing.T) {
	stubWifi(t, true, "US", "configured", true, "")
	got := reconcileWifiRegdom(true)
	if got.status != recOK {
		t.Errorf("a set domain should be ok, got %v (%s)", got.status, got.detail)
	}
}

// Domain 00 with a country the timezone reveals: check mode proposes setting it
// from that country and points the fix at the exact command.
func TestRegdomWorldDomainWouldSetFromTimezone(t *testing.T) {
	stubWifi(t, true, "00", "unset", true, "DE")
	got := reconcileWifiRegdom(true)
	if got.status != recWouldFix {
		t.Fatalf("domain 00 with a timezone country should be would-fix, got %v (%s)", got.status, got.detail)
	}
	if !strings.Contains(got.remedy, "ryoku-wifi-regdom set DE") {
		t.Errorf("fix should name the timezone country, got %q", got.remedy)
	}
}

func TestRegdomAppliesThroughSudo(t *testing.T) {
	stubWifi(t, true, "00", "unset", true, "FR")
	old := setWifiRegdom
	t.Cleanup(func() { setWifiRegdom = old })
	called := ""
	setWifiRegdom = func(country string) error {
		called = country
		return nil
	}
	reconcileWifiRegdom(false)
	if called != "FR" {
		t.Fatalf("regdom helper country = %q, want FR", called)
	}
}

// Domain 00 with no country to infer: warn honestly and tell the user to pass
// their own country code, since guessing one would be wrong.
func TestRegdomWorldDomainNoTimezoneCountryWarns(t *testing.T) {
	stubWifi(t, true, "00", "unset", true, "")
	got := reconcileWifiRegdom(true)
	if got.status != recWarn {
		t.Fatalf("domain 00 with no timezone country should warn, got %v (%s)", got.status, got.detail)
	}
	if !strings.Contains(got.remedy, "ryoku-wifi-regdom set <CC>") {
		t.Errorf("fix should tell the user to set a country, got %q", got.remedy)
	}
}

func TestRegdomZoneFromLocaltime(t *testing.T) {
	cases := map[string]string{
		"../usr/share/zoneinfo/Europe/Vienna":     "Europe/Vienna",
		"/usr/share/zoneinfo/America/New_York":    "America/New_York",
		"/usr/share/zoneinfo/posix/Europe/Berlin": "Europe/Berlin",
		"/usr/share/zoneinfo/UTC":                 "UTC",
		"/somewhere/else":                         "",
	}
	for target, want := range cases {
		if got := zoneFromLocaltime(target); got != want {
			t.Errorf("zoneFromLocaltime(%q) = %q, want %q", target, got, want)
		}
	}
}

// The timezone names the country even when the language is en_US, the case that
// pinned a Vienna laptop to US rules.
func TestRegdomCountryFromZoneTab(t *testing.T) {
	zoneTab := "# comment\tEurope/Vienna\nAT\t+4813+01620\tEurope/Vienna\nDE\t+5230+01322\tEurope/Berlin\tmost of Germany\n"
	zone1970 := "DE,DK,NO,SE,SJ\t+5230+01322\tEurope/Berlin\tmost of Germany\n"
	cases := []struct{ tab, zone, want string }{
		{zoneTab, "Europe/Vienna", "AT"},
		{zoneTab, "Europe/Berlin", "DE"},
		{zone1970, "Europe/Berlin", "DE"},
		{zoneTab, "UTC", ""},
		{"(open /usr/share/zoneinfo/zone.tab: no such file)", "Europe/Vienna", ""},
	}
	for _, c := range cases {
		if got := countryFromZoneTab(c.tab, c.zone); got != c.want {
			t.Errorf("countryFromZoneTab(%q, %q) = %q, want %q", c.tab, c.zone, got, c.want)
		}
	}
}

func TestRegdomIwRegCountry(t *testing.T) {
	cases := map[string]string{
		"global\ncountry US: DFS-FCC\n": "US",
		"global\ncountry 00: DFS-UNSET": "00",
		"global\n(no country line)\n":   "00",
	}
	for out, want := range cases {
		if got := iwRegCountry(out); got != want {
			t.Errorf("iwRegCountry(%q) = %q, want %q", out, got, want)
		}
	}
}
