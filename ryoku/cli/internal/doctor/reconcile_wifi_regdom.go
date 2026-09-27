package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"ryoku-cli/internal/sys"
	"strings"

	i18n "ryoku-i18n"
)

// ---- reconciler: wireless regulatory domain ----------------------------------
//
// The kernel gates which Wi-Fi channels a radio may use on the regulatory
// domain. With no country configured the kernel stays on the worldwide default
// "00", which disables or marks no-IR most 5 GHz channels, so the machine only
// ever sees (and can only join) 2.4 GHz networks. wireless-regdb + iw ship the
// database and the tool, but nothing sets a country on its own; the
// ryoku-wifi-regdom helper does, and this wires that heal into `ryoku doctor`
// so an update (or a hand-run doctor) restores 5 GHz on a box that never had a
// domain set.
//
// Fires ONLY on a box that has a radio yet sits on domain 00. A desktop with no
// Wi-Fi, or a laptop already on a real country, is left alone, so a routine
// doctor pass never touches a working radio.

// wifiRadioPresent reports whether the box has any wireless interface, from the
// per-device `wireless/` directory the kernel exposes in sysfs for a wiphy. A
// var so a test can run the reconciler on a machine with no radio without a
// real /sys.
var wifiRadioPresent = func() bool {
	m, _ := filepath.Glob("/sys/class/net/*/wireless")
	return len(m) > 0
}

// wifiRegdom returns the effective regulatory domain (a two-letter code or the
// worldwide "00") and where it came from. It asks the ryoku-wifi-regdom helper
// first, whose `get` prints "<CC> <source>" with source configured|driver|unset;
// when the helper is absent it parses the first `country XX` line of
// `iw reg get`. ok is false only when neither the helper nor iw can answer, the
// signal that this box has no way to read a domain. A var so a test drives the
// decision without the helper or iw on PATH.
var wifiRegdom = func() (domain, source string, ok bool) {
	if _, err := exec.LookPath("ryoku-wifi-regdom"); err == nil {
		if out, err := exec.Command("ryoku-wifi-regdom", "get").Output(); err == nil {
			switch f := strings.Fields(string(out)); {
			case len(f) >= 2:
				return f[0], f[1], true
			case len(f) == 1:
				return f[0], "unset", true
			}
		}
	}
	if _, err := exec.LookPath("iw"); err != nil {
		return "", "", false
	}
	out, err := exec.Command("iw", "reg", "get").Output()
	if err != nil {
		return "00", "unset", true
	}
	return iwRegCountry(string(out)), "driver", true
}

var setWifiRegdom = func(country string) error {
	return sys.Sudo("ryoku-wifi-regdom", "set", country)
}

// wifiRegdomHelperPresent reports whether the ryoku-wifi-regdom helper is on
// PATH. A var so a test drives the apply path without the helper installed.
var wifiRegdomHelperPresent = func() bool {
	_, err := exec.LookPath("ryoku-wifi-regdom")
	return err == nil
}

// iwRegCountry pulls the two-letter domain from the first `country XX:` line of
// `iw reg get` output, "00" when the block reports the worldwide default. pure,
// so the parse is unit-testable without iw.
func iwRegCountry(out string) string {
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "country ")
		if !ok {
			continue
		}
		cc := rest
		if i := strings.IndexAny(cc, " :"); i >= 0 {
			cc = cc[:i]
		}
		if cc == "" {
			return "00"
		}
		return cc
	}
	return "00"
}

// wifiTimezoneCountry derives a candidate two-letter country from the system
// timezone, "" when none can be read. The timezone, not the locale, because the
// installer asks for it by location, while LANG only names a language: en_US is
// the default for English speakers everywhere, and would pin a European laptop
// to US rules. A var so a test can supply a country without /etc or tzdata.
var wifiTimezoneCountry = func() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	zone := zoneFromLocaltime(target)
	if zone == "" {
		return ""
	}
	// zone.tab maps each zone to the one country it was created for; zone1970.tab
	// is the fallback for a tzdata that stops shipping it, and lists that country
	// first.
	for _, tab := range []string{"/usr/share/zoneinfo/zone.tab", "/usr/share/zoneinfo/zone1970.tab"} {
		if cc := countryFromZoneTab(readFileSafe(tab), zone); cc != "" {
			return cc
		}
	}
	return ""
}

// zoneFromLocaltime turns the /etc/localtime symlink target into a zone name
// (../usr/share/zoneinfo/Europe/Vienna -> Europe/Vienna), "" when the target is
// not under a zoneinfo directory. pure, so it is unit-testable without /etc.
func zoneFromLocaltime(target string) string {
	_, zone, ok := strings.Cut(target, "zoneinfo/")
	if !ok {
		return ""
	}
	zone = strings.TrimPrefix(strings.TrimPrefix(zone, "posix/"), "right/")
	return zone
}

// countryFromZoneTab looks zone up in a zone.tab or zone1970.tab body and
// returns its first country code, "" when the zone is not listed (UTC and the
// Etc/ zones name no country). pure, so the lookup is unit-testable without
// tzdata.
func countryFromZoneTab(tab, zone string) string {
	for _, line := range strings.Split(tab, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 || f[2] != zone {
			continue
		}
		cc, _, _ := strings.Cut(f[0], ",")
		if len(cc) != 2 {
			return ""
		}
		return cc
	}
	return ""
}

func reconcileWifiRegdom(checkOnly bool) recResult {
	if !wifiRadioPresent() {
		return okRes(i18n.T("this machine has no wireless device"))
	}
	domain, source, ok := wifiRegdom()
	if !ok {
		return okRes(i18n.T("iw is not installed, so the wireless regulatory domain cannot be read"))
	}
	if domain != "00" {
		return okRes(i18n.T("the wireless regulatory domain is set to %s (%s)"), domain, source)
	}
	// domain 00 is the kernel's worldwide fallback: it keeps most 5 GHz channels
	// disabled, so the radio only ever sees 2.4 GHz networks until a country is set.
	country := wifiTimezoneCountry()
	if country == "" {
		return warnRes(i18n.T("the wireless regulatory domain is unset (00), so the kernel keeps 5 GHz channels disabled, and no country could be inferred from the system timezone to set one")).
			withFix("ryoku-wifi-regdom set <CC>")
	}
	if checkOnly {
		return wouldRes(i18n.T("the wireless regulatory domain is unset (00), so the kernel keeps 5 GHz channels disabled; the system timezone points at %s"), country).
			withFix("ryoku-wifi-regdom set " + country)
	}
	if !wifiRegdomHelperPresent() {
		return warnRes(i18n.T("the wireless regulatory domain is unset (00) and ryoku-wifi-regdom is not installed to set it from the timezone country %s"), country).
			withFix("ryoku-wifi-regdom set " + country)
	}
	if err := setWifiRegdom(country); err != nil {
		return warnRes(i18n.T("could not set the wireless regulatory domain to %s: %v"), country, err).
			withFix("sudo ryoku-wifi-regdom set " + country)
	}
	if again, _, ok := wifiRegdom(); ok && again != "00" {
		return fixedRes(i18n.T("set the wireless regulatory domain to %s from the system timezone, so the kernel enables 5 GHz channels again"), again)
	}
	return warnRes(i18n.T("tried to set the wireless regulatory domain to %s from the system timezone but it is still unset (00)"), country).
		withFix("ryoku-wifi-regdom set " + country)
}
