package wm

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Reclaim answers the one question a compositor switch raises that the neutral
// store cannot: leaving the outgoing compositor for the incoming one, what does
// removing the outgoing compositor's packages actually free? The package lists
// are the providers' own (caps.Packages), because only a provider knows what it
// is made of, so neither the CLI nor the Hub ever spells a compositor package
// name; this seam is the one place both lists meet.
//
// Two exclusions keep the removal from breaking the box:
//   - a package the incoming compositor also declares is shared, so it stays;
//   - a package another installed package still needs is kept.
//
// The set is dnf's own answer for removing the outgoing-only, installed
// packages: the compositor package, its satellites, and the libraries nothing
// else needs once they go. dnf is the authority because it, not a re-derived
// graph, knows how a provides-satisfied dependency resolves; this seam layers
// the two exclusions on top and pins the orchestration with tests.

// ReclaimSet is what leaving Outgoing for Incoming would remove. Targets are the
// declared compositor packages the removal names; Packages is the full
// transaction, those plus the unused dependencies that go with them.
// Removable is false when nothing is installed to remove, so the switch UI can
// say so honestly rather than hide the keep-or-remove choice.
type ReclaimSet struct {
	Outgoing  string   `json:"outgoing"`
	Incoming  string   `json:"incoming"`
	Targets   []string `json:"targets"`
	Packages  []string `json:"packages"`
	Count     int      `json:"count"`
	Size      int64    `json:"size"` // total installed size, bytes
	Removable bool     `json:"removable"`
}

// planner answers what removing targets would take, with the incoming
// provider's variant package assumed installed: the packages removed, or the
// targets a surviving package still needs (blocked), which the caller drops.
type planner func(incoming string, targets []string) (set, blocked []string, err error)

// Package-manager seams, replaced in tests so the orchestration runs without a
// live dnf or rpm.
var (
	pkgInstalled             = hostPkgInstalled
	removalOnce      planner = hostRemovalOnce
	installedSizes           = hostInstalledSizes
	providerPackages         = capsPackages
	runPlan                  = hostRunPlan
	targetProvides           = hostTargetProvides
	packageRequires          = hostPackageRequires
)

// Reclaim computes the removal set for leaving outgoing to run incoming. Both
// are provider names; their package lists come from caps.
func Reclaim(outgoing, incoming string) (ReclaimSet, error) {
	rs := ReclaimSet{Outgoing: outgoing, Incoming: incoming, Targets: []string{}, Packages: []string{}}
	out, err := providerPackages(outgoing)
	if err != nil {
		return rs, err
	}
	// The incoming provider may not be installed yet, so its binary can be
	// absent; its declared list is still read best-effort to spare a shared
	// satellite from removal.
	in, _ := providerPackages(incoming)
	targets := reclaimTargets(out, in, pkgInstalled)
	rs.Targets = targets
	if len(targets) == 0 {
		return rs, nil // nothing installed to remove: honest empty set
	}
	set, kept, err := removalSet(incoming, targets, removalOnce)
	if err != nil {
		return rs, err
	}
	// A target a surviving package still needs is dropped from the removal and
	// stays installed, so the names the switch removes are only the ones that
	// survived the plan.
	rs.Targets = kept
	if len(set) == 0 {
		return rs, nil
	}
	size, err := installedSizes(set)
	if err != nil {
		return rs, err
	}
	rs.Packages = set
	rs.Count = len(set)
	rs.Size = size
	rs.Removable = true
	return rs, nil
}

// reclaimTargets is the pure target selection: the outgoing provider's declared
// packages that are installed, minus any the incoming provider also declares (a
// shared satellite the switch must keep). Order follows the outgoing
// declaration so a caller sees a stable, most-significant-first list.
func reclaimTargets(outgoing, incoming []string, installed func(string) bool) []string {
	shared := make(map[string]bool, len(incoming))
	for _, p := range incoming {
		shared[p] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, p := range outgoing {
		if p == "" || seen[p] || shared[p] || !installed(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// removalSet drives the planner to a stable removal transaction. A target a
// surviving package still requires comes back blocked; it is then dropped (kept
// installed) and the plan asked again, so the final set removes everything safe
// and keeps everything needed. It returns the packages removed and the named
// targets that survived the drops.
func removalSet(incoming string, targets []string, plan planner) (removed, kept []string, err error) {
	cur := append([]string(nil), targets...)
	for len(cur) > 0 {
		set, blocked, err := plan(incoming, cur)
		if err != nil {
			return nil, nil, err
		}
		if len(blocked) == 0 {
			sort.Strings(set)
			return set, cur, nil
		}
		cur = drop(cur, blocked)
	}
	return nil, []string{}, nil
}

// VerifyRemoval refuses a removal whose real transaction would differ from the
// reviewed set. Run immediately before the destructive removal, it asks dnf
// again what removing the surviving targets would take and compares that to the
// reviewed set: if dnf would pull in a package the review never showed, or a
// target has since become one a surviving package needs, the switch's cleanup is
// refused rather than run, so a removal can never quietly cascade past what the
// user saw.
func VerifyRemoval(rs ReclaimSet) error {
	if !rs.Removable {
		return nil
	}
	set, blocked, err := removalOnce(rs.Incoming, rs.Targets)
	if err != nil {
		return fmt.Errorf("refusing to remove %s: the package manager could not plan the removal: %w", rs.Outgoing, err)
	}
	if len(blocked) > 0 {
		return fmt.Errorf("refusing to remove %s: %s is now required by another package", rs.Outgoing, strings.Join(blocked, " "))
	}
	if extra := difference(set, rs.Packages); len(extra) > 0 {
		return fmt.Errorf("refusing to remove %s: the package manager would also remove %s, outside the reviewed set", rs.Outgoing, strings.Join(extra, " "))
	}
	if missing := difference(rs.Packages, set); len(missing) > 0 {
		return fmt.Errorf("refusing to remove %s: the package manager would keep %s from the reviewed set", rs.Outgoing, strings.Join(missing, " "))
	}
	return nil
}

// drop returns cur without any member of rm.
func drop(cur, rm []string) []string {
	gone := make(map[string]bool, len(rm))
	for _, p := range rm {
		gone[p] = true
	}
	var out []string
	for _, p := range cur {
		if !gone[p] {
			out = append(out, p)
		}
	}
	return out
}

// difference returns the members of a that are not in b, sorted.
func difference(a, b []string) []string {
	have := make(map[string]bool, len(b))
	for _, x := range b {
		have[x] = true
	}
	var out []string
	for _, x := range a {
		if !have[x] {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// capsPackages reads a provider's declared package list from its caps.
func capsPackages(name string) ([]string, error) {
	caps, err := OpenNamed(name).Caps()
	if err != nil {
		return nil, err
	}
	return caps.Packages, nil
}

// hostRemovalOnce asks dnf for the transaction that removing targets would run
// and reads every package it removes, unused dependencies included. A package
// outside the targets that dnf would remove because it needs one of them (a
// plugin, a -devel package) is a survivor the user never chose to lose, so the
// targets it requires come back blocked rather than cascading.
func hostRemovalOnce(incoming string, targets []string) (set, blocked []string, err error) {
	if len(targets) == 0 {
		return nil, nil, nil
	}
	out, err := runPlan(incoming, targets)
	if err != nil {
		return nil, nil, err
	}
	plan := parseRemovalPlan(out)
	dependents := plan.dependents(targets)
	if len(dependents) == 0 {
		return plan.removed, nil, nil
	}
	provides, err := targetProvides(targets)
	if err != nil {
		return nil, nil, err
	}
	requires, err := packageRequires(dependents)
	if err != nil {
		return nil, nil, err
	}
	blocked = blockedTargets(targets, provides, requires)
	if len(blocked) == 0 {
		return nil, nil, fmt.Errorf("removing %s would also remove %s", strings.Join(targets, " "), strings.Join(dependents, " "))
	}
	return nil, blocked, nil
}

// hostRunPlan runs dnf's own planner with --assumeno, which resolves the full
// transaction, prints it, and aborts before touching anything. dnf exits 1 on
// that abort, so a plan is recognised by its summary rather than the status.
//
// A switch installs the incoming variant before it removes anything, and that
// variant provides the compositor virtual the ryoku-desktop umbrella requires.
// Planning the removal alone would read the umbrella, and the whole desktop with
// it, as dependents. So when the variant is not installed yet, the plan is one
// `dnf do` transaction that installs it and removes the targets, the state the
// real removal will run in. --allowerasing lets that transaction list dependents
// instead of refusing; nothing is erased because --assumeno aborts it. A variant
// the cache cannot supply (a checkout box) falls back to the removal alone,
// where the umbrella then blocks the variant and the plan keeps it.
func hostRunPlan(incoming string, targets []string) (string, error) {
	if v := VariantPackage(incoming); incoming != "" && !pkgInstalled(v) {
		args := append([]string{"-C", "--assumeno", "do", "--allowerasing", "--action=install", v, "--action=remove"}, targets...)
		if out, err := dnfPlan(args); err == nil {
			return out, nil
		}
	}
	return dnfPlan(append([]string{"-C", "--assumeno", "remove"}, targets...))
}

func dnfPlan(args []string) (string, error) {
	cmd := dnfCommand(args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	text := string(out)
	if err == nil || strings.Contains(text, "Transaction Summary:") {
		return text, nil
	}
	return "", fmt.Errorf("dnf %s: %w: %s", args[2], err, strings.TrimSpace(stderr.String()))
}

// removalPlan is a dnf transaction table read back by section.
type removalPlan struct {
	removed   []string // every package the transaction removes
	dependent []string // removed because it needs a removed package
}

// dependents is every removed package the user did not target and dnf did not
// call an unused dependency: something that needs a target and would go with it.
func (p removalPlan) dependents(targets []string) []string {
	return difference(p.dependent, targets)
}

// parseRemovalPlan reads the transaction table dnf5 prints in the C locale. A
// section header starts in column 0 and ends in a colon; a package row is
// indented one space with the name first; a "replacing" line under an upgrade
// is indented further and names an old version, not a removal. Rows under the
// "Removing" headers are removed: "Removing:" holds the named targets (and any
// package a conflict would erase), "Removing dependent packages:" the packages
// that need them, and "Removing unused dependencies:" the libraries nothing
// needs any more. The summary block ends the table.
func parseRemovalPlan(text string) removalPlan {
	var plan removalPlan
	section := ""
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			if strings.HasPrefix(line, "Transaction Summary") {
				break
			}
			section = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		if len(line) < 2 || line[1] == ' ' || !strings.HasPrefix(section, "Removing") {
			continue
		}
		name := strings.Fields(line)[0]
		if seen[name] {
			continue
		}
		seen[name] = true
		plan.removed = append(plan.removed, name)
		if section != "Removing unused dependencies" {
			plan.dependent = append(plan.dependent, name)
		}
	}
	return plan
}

// blockedTargets names the targets a dependent requires: a requirement matched
// by a capability or file the target provides.
func blockedTargets(targets []string, provides map[string][]string, requires []string) []string {
	need := make(map[string]bool, len(requires))
	for _, r := range requires {
		need[r] = true
	}
	var blocked []string
	for _, t := range targets {
		if need[t] {
			blocked = append(blocked, t)
			continue
		}
		for _, c := range provides[t] {
			if need[c] {
				blocked = append(blocked, t)
				break
			}
		}
	}
	return blocked
}

// hostTargetProvides maps each target to the capabilities and files it
// provides, read from one rpm query.
func hostTargetProvides(targets []string) (map[string][]string, error) {
	args := append([]string{"-q", "--qf", "[%{=NAME}\t%{PROVIDENAME}\n][%{=NAME}\t%{FILENAMES}\n]"}, targets...)
	out, err := exec.Command("rpm", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("rpm -q provides: %w", err)
	}
	provides := map[string][]string{}
	for _, line := range strings.Split(string(out), "\n") {
		name, c, ok := strings.Cut(line, "\t")
		if ok && c != "" {
			provides[name] = append(provides[name], c)
		}
	}
	return provides, nil
}

// hostPackageRequires lists the capabilities the named packages require, read
// from one rpm query.
func hostPackageRequires(names []string) ([]string, error) {
	args := append([]string{"-q", "--qf", "[%{REQUIRENAME}\n]"}, names...)
	out, err := exec.Command("rpm", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("rpm -q requires: %w", err)
	}
	return strings.Fields(string(out)), nil
}

// hostInstalledSizes sums the installed size in bytes of the named packages,
// read from one rpm query.
func hostInstalledSizes(names []string) (int64, error) {
	if len(names) == 0 {
		return 0, nil
	}
	args := append([]string{"-q", "--qf", "%{SIZE}\n"}, names...)
	out, err := exec.Command("rpm", args...).Output()
	if err != nil {
		return 0, fmt.Errorf("rpm -q: %w", err)
	}
	return sumSizes(string(out))
}

// sumSizes totals rpm's %{SIZE} output, one byte count per line.
func sumSizes(text string) (int64, error) {
	var total int64
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, convErr := strconv.ParseInt(line, 10, 64)
		if convErr != nil {
			return 0, fmt.Errorf("rpm size %q: %w", line, convErr)
		}
		total += n
	}
	return total, nil
}
