package updater

import (
	"bufio"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"ryoku-cli/internal/sys"
)

// The Ryoku lane.
//
// `ryoku update` moves the packages Ryoku publishes, and nothing else. The base
// system and its kernel belong to Fedora, and the user takes those with `sudo
// dnf upgrade`, on their own schedule. Two lanes, for reasons that are the
// whole design:
//
//   - The kernel is not ours to move. A Ryoku release must never decide when a
//     box changes kernel, rebuilds its akmods, or rewrites its boot image.
//   - A release must be reversible. A snapshot restore puts the system back;
//     an update that moved the base too would take far more with it.
//   - The lanes fail independently. A box that cannot take a Fedora upgrade
//     today (a mirror out of sync, a held package, a full /boot) must still be
//     able to take a Ryoku fix, and the other way round.
//
// So this file is the only place that decides what `ryoku update` may touch:
// the installed packages the RyokuCOPR repository serves. Everything else is
// reported, never moved. `ryoku update --system` opts back into one command
// that also runs the user's lane, for people who want it.

// ryokuSet: the installed packages the RyokuCOPR repo serves, sorted. Pure over
// its two inputs, so the selection is unit-testable without dnf: repoNames is
// `dnf repoquery --repo RyokuCOPR`, installed is `rpm -qa`.
func ryokuSet(repoNames, installed []string) []string {
	have := make(map[string]bool, len(installed))
	for _, p := range installed {
		if p = strings.TrimSpace(p); p != "" {
			have[p] = true
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(repoNames))
	for _, p := range repoNames {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] || !have[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// installedRyokuSet reads the box: the installed packages the RyokuCOPR repo
// serves. An error means the question could not be answered (no repo, no
// metadata): the caller must stop rather than fall back to a system upgrade,
// which is the other lane. Nothing is held back: the set moves by `dnf
// distro-sync --repo=RyokuCOPR`, which settles every name on our build in one
// transaction, so no distro repo can flip-flop it.
func installedRyokuSet() ([]string, error) {
	repo, err := sys.RunOut("dnf", "repoquery", "--repo", sys.RPMRepoName, "--qf", "%{name}\n")
	if err != nil {
		return nil, err
	}
	installed, err := sys.RunOut("rpm", "-qa", "--qf", "%{NAME}\n")
	if err != nil {
		return nil, err
	}
	return ryokuSet(lines(repo), lines(installed)), nil
}

// lines splits command output into non-empty trimmed lines.
func lines(out string) []string {
	var xs []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			xs = append(xs, l)
		}
	}
	return xs
}

// refreshDBArgs refetches the RyokuCOPR metadata, and nothing else. It runs
// BEFORE the set is read, so the set is what the repo serves now.
func refreshDBArgs() []string {
	return []string{"sudo", "dnf", "--repo=" + sys.RPMRepoName, "--refresh", "makecache"}
}

// ryokuInstallArgs moves exactly the set onto what RyokuCOPR serves.
// distro-sync, never upgrade: a sysupgrade is the user's lane, and
// distro-sync settles each name on our build in either direction.
func ryokuInstallArgs(set []string) []string {
	if len(set) == 0 {
		return []string{"true"}
	}
	return append([]string{"sudo", "dnf", "-y", "--repo=" + sys.RPMRepoName, "distro-sync"}, set...)
}

// A release can add a Requires on a package only Fedora serves (a font, a
// library). The lane is restricted to RyokuCOPR, so dnf sees nothing that
// provides it and skips the whole set with exit 0. ryokuMissingDeps finds those
// requirements with a dry resolve, and depInstallArgs installs just them from
// every enabled repository before the real transaction: new packages only, so
// the base system still does not move.

// nothingProvidesRe: dnf's "nothing provides <capability> needed by <nevra>".
var nothingProvidesRe = regexp.MustCompile(`nothing provides (.+?) needed by `)

// missingDeps: the capabilities a resolve reported nothing provides, in order
// of first mention, deduplicated. Pure over dnf's output.
func missingDeps(out string) []string {
	seen := map[string]bool{}
	var caps []string
	for _, m := range nothingProvidesRe.FindAllStringSubmatch(out, -1) {
		if c := strings.TrimSpace(m[1]); c != "" && !seen[c] {
			seen[c] = true
			caps = append(caps, c)
		}
	}
	return caps
}

// ryokuPlanArgs resolves the lane without running it; --assumeno needs no root.
func ryokuPlanArgs(set []string) []string {
	return append([]string{"dnf", "--assumeno", "--repo=" + sys.RPMRepoName, "distro-sync"}, set...)
}

// depInstallArgs installs the missing requirements from every enabled repo.
func depInstallArgs(caps []string) []string {
	return append([]string{"sudo", "dnf", "-y", "install"}, caps...)
}

// ryokuMissingDeps: what the set's new builds require that RyokuCOPR alone
// cannot provide. --assumeno exits non-zero whenever there is work, so the
// output is read whatever the status.
func ryokuMissingDeps(set []string) []string {
	argv := ryokuPlanArgs(set)
	out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	return missingDeps(string(out))
}

// heldBack: the set's packages the box still runs older than the newest build
// the repo serves. dnf skips an uninstallable upgrade and exits 0, so its
// status alone cannot say the set moved. Pure over two name -> EVR maps.
func heldBack(installed, latest map[string]string) []updateItem {
	var behind []updateItem
	for name, have := range installed {
		if want, ok := latest[name]; ok && versionCompare(have, want) < 0 {
			behind = append(behind, updateItem{Name: name, Old: have, New: want})
		}
	}
	sort.Slice(behind, func(i, j int) bool { return behind[i].Name < behind[j].Name })
	return behind
}

// evrMap reads "name evr" lines, keeping the newest EVR of a repeated name.
func evrMap(out string) map[string]string {
	m := map[string]string{}
	for _, l := range lines(out) {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		if have, ok := m[f[0]]; !ok || versionCompare(have, f[1]) < 0 {
			m[f[0]] = f[1]
		}
	}
	return m
}

// ryokuHeldBack compares the box against the repo after the transaction.
func ryokuHeldBack(set []string) ([]updateItem, error) {
	latest, err := sys.RunOut("dnf", append([]string{"repoquery", "--repo=" + sys.RPMRepoName,
		"--latest-limit=1", "--qf", "%{name} %{evr}\n"}, set...)...)
	if err != nil {
		return nil, err
	}
	installed, err := sys.RunOut("rpm", append([]string{"-q", "--qf",
		"%{NAME} %{EPOCHNUM}:%{VERSION}-%{RELEASE}\n"}, set...)...)
	if err != nil && strings.TrimSpace(installed) == "" {
		return nil, err
	}
	return heldBack(evrMap(installed), evrMap(latest)), nil
}

// systemLanePending: what the user's lane would take, minus the Ryoku set we
// just moved.
func systemLanePending(ryokuTargets []string) []updateItem {
	ours := make(map[string]bool, len(ryokuTargets))
	for _, t := range ryokuTargets {
		ours[t] = true
	}
	var ups []updateItem
	for _, u := range pendingUpdates() {
		if !ours[u.Name] {
			ups = append(ups, u)
		}
	}
	return ups
}
