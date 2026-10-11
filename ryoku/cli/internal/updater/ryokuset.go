package updater

import (
	"bufio"
	"fmt"
	"os"
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
// every enabled repository before the real transaction. An install that would
// upgrade, downgrade or remove a base package is the user's lane, so it runs
// only under --system (see baseMoves).

// nothingProvidesRe: dnf's "nothing provides <capability> needed by <nevra>".
var nothingProvidesRe = regexp.MustCompile(`nothing provides (.+?) needed by `)

// dryRun runs a dnf --assumeno command in the C locale, so the English text
// these parsers match is what dnf prints, with stdout and stderr together (the
// problems go to stderr). --assumeno exits 1 whenever there is work, so a
// non-zero status is a failure only when the output holds no resolve.
func dryRun(argv []string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	b, err := cmd.CombinedOutput()
	out := string(b)
	if err != nil && !resolved(out) {
		if ls := lines(out); len(ls) > 0 {
			return out, fmt.Errorf("%s: %s", strings.Join(argv, " "), ls[len(ls)-1])
		}
		return out, err
	}
	return out, nil
}

// resolved: dnf got as far as an answer, a transaction it declined or a
// problem it could not solve.
func resolved(out string) bool {
	return strings.Contains(out, "Operation aborted by the user") || nothingProvidesRe.MatchString(out)
}

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
// --refresh because it runs as the user: resolved against a stale user cache,
// the old build requires nothing new and the missing dependency goes unseen.
func ryokuPlanArgs(set []string) []string {
	return append([]string{"dnf", "--assumeno", "--refresh", "--repo=" + sys.RPMRepoName, "distro-sync"}, set...)
}

// depInstallArgs installs the missing requirements from every enabled repo.
func depInstallArgs(caps []string) []string {
	return append([]string{"sudo", "dnf", "-y", "install"}, caps...)
}

// depPlanArgs resolves that install without running it.
func depPlanArgs(caps []string) []string {
	return append([]string{"dnf", "--assumeno", "install"}, caps...)
}

// ryokuMissingDeps: what the set's new builds require that RyokuCOPR alone
// cannot provide.
func ryokuMissingDeps(set []string) ([]string, error) {
	out, err := dryRun(ryokuPlanArgs(set))
	if err != nil {
		return nil, err
	}
	return missingDeps(out), nil
}

// baseMoves: the installed packages a dnf transaction table upgrades,
// downgrades or removes, the base moving under a "new packages only" install
// (a "libfoo >= 2.1" while 2.0 is installed, or a newer glibc a font pulls
// in). Pure over dnf's C-locale output: a section header starts the line, a
// package sits one space in, and its "replacing" line further.
func baseMoves(out string) []string {
	var names []string
	moving := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "Transaction Summary"):
			return names
		case l != "" && !strings.HasPrefix(l, " "):
			moving = strings.HasPrefix(l, "Upgrading") || strings.HasPrefix(l, "Downgrading") ||
				strings.HasPrefix(l, "Removing")
		case moving && strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "  "):
			if f := strings.Fields(l); len(f) > 0 {
				names = append(names, f[0])
			}
		}
	}
	return names
}

// depBaseMoves: the base packages installing caps would move.
func depBaseMoves(caps []string) ([]string, error) {
	out, err := dryRun(depPlanArgs(caps))
	if err != nil {
		return nil, err
	}
	return baseMoves(out), nil
}

// heldBack: the set's packages the box runs at any build other than the newest
// the repo serves, older or newer: distro-sync moves the set down as readily as
// up, so a pulled build left installed is as stuck as a missed one. dnf skips
// an unresolvable move and exits 0, so its status alone cannot say the set
// moved. Pure over two name -> EVR maps.
func heldBack(installed, latest map[string]string) []updateItem {
	var behind []updateItem
	for name, have := range installed {
		if want, ok := latest[name]; ok && versionCompare(have, want) != 0 {
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

// ryokuHeldBack compares the box against the repo after the transaction. It
// refreshes: a stale user cache would agree with the old build and hide the
// very packages this exists to catch.
func ryokuHeldBack(set []string) ([]updateItem, error) {
	latest, err := latestAvailableSet(set, true)
	if err != nil {
		return nil, err
	}
	installed, err := sys.RunOut("rpm", append([]string{"-q", "--qf",
		"%{NAME} %{EPOCHNUM}:%{VERSION}-%{RELEASE}\n"}, set...)...)
	if err != nil && strings.TrimSpace(installed) == "" {
		return nil, err
	}
	return heldBack(evrMap(installed), latest), nil
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
