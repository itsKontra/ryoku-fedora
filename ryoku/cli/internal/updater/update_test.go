package updater

import (
	"encoding/json"
	"errors"
	"github.com/godbus/dbus/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	wm "ryoku-wm"
	"strings"
	"testing"
)

// An up-to-date packaged box (installed == latest) reports nothing pending but
// lists the recent history the installed commit contains, so the Hub's Updates
// page still has content. packagedStatus is the pure core of buildStatus, so
// this exercises the sha branching and the recent lookup without dnf.
func TestPackagedStatusUpToDatePopulatesRecent(t *testing.T) {
	srv, _ := stubRecent(t, [][2]string{
		{"latest work", "86a91f4aaaa"},
		{"earlier work", "1184abcdddd"},
	})
	t.Setenv("RYOKU_GITHUB_API", srv.URL)
	t.Setenv("RYOKU_REPO_SLUG", "owner/repo")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	ver := "0.12.6.r1184.g86a91f4-1" // installed == latest: shortCommit -> 86a91f4
	r := packagedStatus(ver, ver)

	if r.Behind != 0 {
		t.Errorf("pendingUpdates = %d, want 0 when up to date", r.Behind)
	}
	if r.Available {
		t.Error("available = true, want false when up to date")
	}
	if len(r.Updates) != 0 {
		t.Errorf("updates = %d, want 0 (nothing incoming)", len(r.Updates))
	}
	if len(r.Recent) != 2 {
		t.Fatalf("recent = %d, want 2 (from the stub)", len(r.Recent))
	}
	if r.Recent[0].Name != "latest work" || r.Recent[0].New != "86a91f4" {
		t.Errorf("recent[0] = %+v, want {latest work, 86a91f4}", r.Recent[0])
	}
	// The Hub reads these JSON keys off `ryoku status --json`; pin them.
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if js := string(b); !strings.Contains(js, `"pendingUpdates":0`) || !strings.Contains(js, `"recent":[{`) {
		t.Errorf("status JSON = %s, want pendingUpdates:0 and a non-empty recent[]", js)
	}
}

// Offline / rate-limited: the recent lookup fails, so the up-to-date report
// degrades to an empty (non-nil) recent list with no error or hang, keeping the
// JSON shape stable.
func TestPackagedStatusUpToDateOfflineEmptyRecent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RYOKU_GITHUB_API", srv.URL)
	t.Setenv("RYOKU_REPO_SLUG", "owner/repo")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	ver := "0.12.6.r1184.g86a91f4-1"
	r := packagedStatus(ver, ver)

	if r.Behind != 0 {
		t.Errorf("pendingUpdates = %d, want 0 when up to date", r.Behind)
	}
	if r.Recent == nil {
		t.Error("recent = nil, want a non-nil empty slice so the JSON stays stable")
	}
	if len(r.Recent) != 0 {
		t.Errorf("recent = %d, want 0 on a failed lookup", len(r.Recent))
	}
}

// prowlDecide is the pure core of prowlRefresh: a dev install (on PATH, not
// rpm-owned) self-updates; an rpm-owned copy is left to dnf; an absent binary
// is a no-op. Pinned so the dev-vs-packaged branch cannot regress.
func TestProwlDecide(t *testing.T) {
	cases := []struct {
		name     string
		onPath   bool
		pkgOwned bool
		want     prowlAction
	}{
		{"absent does nothing", false, false, prowlNoop},
		{"packaged is left to dnf", true, true, prowlManaged},
		{"dev install self-updates", true, false, prowlSelfUpdate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := prowlDecide(c.onPath, c.pkgOwned); got != c.want {
				t.Fatalf("prowlDecide(onPath=%v, owned=%v) = %v, want %v", c.onPath, c.pkgOwned, got, c.want)
			}
		})
	}
}

func TestFedoraUpdateLanes(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "dnf"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	got := strings.Join(ryokuInstallArgs([]string{"ryoku-desktop", "ryoku-shell"}), " ")
	if got != "sudo dnf -y --repo=RyokuCOPR distro-sync ryoku-desktop ryoku-shell" {
		t.Fatalf("Ryoku lane: %s", got)
	}
	if got := strings.Join(systemUpgradeArgs(), " "); got != "sudo dnf -y upgrade" {
		t.Fatalf("system lane: %s", got)
	}
	if got := strings.Join(refreshDBArgs(), " "); got != "sudo dnf --repo=RyokuCOPR --refresh makecache" {
		t.Fatalf("refresh: %s", got)
	}
}

func TestDNFSymlinkAndEmptyRyokuSet(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "dnf5"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	if err := os.Symlink("dnf5", filepath.Join(bin, "dnf")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if got := strings.Join(ryokuInstallArgs([]string{"ryoku-shell"}), " "); got != "sudo dnf -y --repo=RyokuCOPR distro-sync ryoku-shell" {
		t.Fatal(got)
	}
	if got := strings.Join(ryokuInstallArgs(nil), " "); got != "true" {
		t.Fatalf("empty set could upgrade host: %s", got)
	}
}

// A COPR repo keeps several builds in its repodata and dnf5's --qf adds no
// newline, so an unbounded repoquery glued every version into one string that
// never matched the installed one -- a fully updated box showed "available"
// mush and "behind 1 commit(s)".
func TestLatestAvailableRPMPicksNewestBuild(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		"limit=0\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"--latest-limit=1\" ]; then limit=1; fi\n" +
		"done\n" +
		"if [ \"$limit\" = 1 ]; then\n" +
		"  printf '0.3723-8.fc44\\n'\n" +
		"else\n" +
		"  printf '0.3719-4.fc440.3720-5.fc440.3721-6.fc440.3722-7.fc440.3723-8.fc44'\n" +
		"fi\n"
	if err := os.WriteFile(filepath.Join(bin, "dnf"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if got := latestAvailable("ryoku-desktop", false); got != "0.3723-8.fc44" {
		t.Fatalf("latestAvailable = %q, want the single newest build", got)
	}
}

func TestLogin1GraphicalUserProperties(t *testing.T) {
	properties := func(sessionType, sessionClass, desktop string) map[string]dbus.Variant {
		return map[string]dbus.Variant{
			"Type":    dbus.MakeVariant(sessionType),
			"Class":   dbus.MakeVariant(sessionClass),
			"Desktop": dbus.MakeVariant(desktop),
		}
	}
	for _, tc := range []struct {
		name       string
		properties map[string]dbus.Variant
		want       bool
	}{
		{"wayland Hyprland user", properties("wayland", "user", "Hyprland"), true},
		{"x11 niri early user", properties("x11", "user-early", "niri"), false},
		{"tty user", properties("tty", "user", "Hyprland"), false},
		{"wayland greeter", properties("wayland", "greeter", "Hyprland"), false},
		{"wayland lock screen", properties("wayland", "lock-screen", "Hyprland"), false},
		{"other desktop", properties("wayland", "user", "GNOME"), false},
		{"missing class", map[string]dbus.Variant{
			"Type": dbus.MakeVariant("wayland"), "Desktop": dbus.MakeVariant("Hyprland"),
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := login1GraphicalUserProperties(tc.properties); got != tc.want {
				t.Fatalf("login1GraphicalUserProperties() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasUpdateSleepGuardRequiresOwnedBlock(t *testing.T) {
	const uid = 1000
	for _, tc := range []struct {
		name       string
		inhibitors []login1Inhibitor
		want       bool
	}{
		{"owned sleep block", []login1Inhibitor{{
			What: "sleep:shutdown", Who: "ryoku-session-cutover", Mode: "block", UID: uid,
		}}, true},
		{"wrong user", []login1Inhibitor{{
			What: "sleep", Who: "ryoku-session-cutover", Mode: "block", UID: 1001,
		}}, false},
		{"delay is not a block", []login1Inhibitor{{
			What: "sleep", Who: "ryoku-session-cutover", Mode: "delay", UID: uid,
		}}, false},
		{"lid-only block", []login1Inhibitor{{
			What: "handle-lid-switch", Who: "ryoku-session-cutover", Mode: "block", UID: uid,
		}}, false},
		{"foreign owner", []login1Inhibitor{{
			What: "sleep", Who: "other", Mode: "block", UID: uid,
		}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasUpdateSleepGuard(tc.inhibitors, uid); got != tc.want {
				t.Fatalf("hasUpdateSleepGuard() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPackagePowerCutoverRetriesIncompleteAdoption(t *testing.T) {
	for _, tc := range []struct {
		name            string
		markerPresent   bool
		rootGuardActive bool
		want            bool
	}{
		{"first rollout", false, false, true},
		{"completed hook", true, false, false},
		{"stale marker with emergency guard", true, true, true},
		{"failed unmarked adoption", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsPackagePowerCutover(tc.markerPresent, tc.rootGuardActive); got != tc.want {
				t.Fatalf("needsPackagePowerCutover(%v, %v) = %v, want %v",
					tc.markerPresent, tc.rootGuardActive, got, tc.want)
			}
		})
	}
}

func TestConfigReloadResultAcceptsProviderWithoutReload(t *testing.T) {
	if err := configReloadResult(wm.ErrUnsupported); err != nil {
		t.Fatalf("unsupported reload should defer to provider file watching: %v", err)
	}
	providerErr := errors.New("provider unavailable")
	if err := configReloadResult(providerErr); !errors.Is(err, providerErr) {
		t.Fatalf("live provider error was suppressed: %v", err)
	}
}

// The status check runs as the user, whose dnf cache can lag the root one
// `ryoku update` refreshed. A repo view older than the running package must
// not read as an update: the bar kept offering "Click to update" after one.
func TestPackagedStatusOlderRepoIsNotAnUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RYOKU_GITHUB_API", srv.URL)
	t.Setenv("RYOKU_REPO_SLUG", "owner/repo")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	r := packagedStatus("0.3916-35.fc44", "0.3739-24.fc44")

	if r.Available || r.Behind != 0 || len(r.Updates) != 0 {
		t.Fatalf("available=%v behind=%d updates=%v, want nothing pending", r.Available, r.Behind, r.Updates)
	}
	if r.Latest != "0.3916-35.fc44" {
		t.Errorf("latest = %q, want the running version", r.Latest)
	}
}

func TestLatestAvailableRPMRefreshRefetchesMetadata(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"--refresh\" ]; then printf '0.3916-35.fc44\\n'; exit 0; fi\n" +
		"done\n" +
		"printf '0.3739-24.fc44\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "dnf"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if got := latestAvailable("ryoku-desktop", false); got != "0.3739-24.fc44" {
		t.Fatalf("cached latestAvailable = %q, want the cached answer", got)
	}
	if got := latestAvailable("ryoku-desktop", true); got != "0.3916-35.fc44" {
		t.Fatalf("refreshed latestAvailable = %q, want the refetched answer", got)
	}
}

// A dotted package name keeps its dots; only the arch is cut.
func TestCheckUpdateName(t *testing.T) {
	for in, want := range map[string]string{
		"python3.13-libs.x86_64":   "python3.13-libs",
		"kernel.x86_64":            "kernel",
		"glibc-langpack-en.noarch": "glibc-langpack-en",
		"Obsoleting":               "",
		".x86_64":                  "",
	} {
		if got := checkUpdateName(in); got != want {
			t.Errorf("checkUpdateName(%q) = %q, want %q", in, got, want)
		}
	}
}
