package wm

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The removal set is the one computation a switch can break a machine with, so
// it is pinned at two levels: the orchestration (shared satellites stay, a
// target a survivor needs stays, the rest goes with its unused libraries) over a
// modelled planner, and the dnf planner itself over transcripts captured from
// dnf5 on a Fedora 44 Hyprland box.

// reclaimEnv swaps the planner seam for a hand-built model and restores it.
type reclaimEnv struct {
	packages map[string][]string // provider -> declared packages
	// private[t] is what removing t drops besides t (its unused libraries),
	// and needs[t] is the surviving package that keeps t installed, if any.
	private map[string][]string
	needs   map[string]string
	size    map[string]int64
}

func (e reclaimEnv) install(t *testing.T) {
	t.Helper()
	oldPkg, oldInst, oldOnce, oldSize := providerPackages, pkgInstalled, removalOnce, installedSizes
	t.Cleanup(func() {
		providerPackages, pkgInstalled, removalOnce, installedSizes = oldPkg, oldInst, oldOnce, oldSize
	})
	installed := map[string]bool{}
	for p := range e.private {
		installed[p] = true
	}
	providerPackages = func(name string) ([]string, error) { return e.packages[name], nil }
	pkgInstalled = func(name string) bool { return installed[name] }
	installedSizes = func(names []string) (int64, error) {
		var total int64
		for _, n := range names {
			total += e.size[n]
		}
		return total, nil
	}
	removalOnce = func(_ string, targets []string) (set, blocked []string, err error) {
		for _, t := range targets {
			if n := e.needs[t]; n != "" && !contains(targets, n) {
				blocked = append(blocked, t)
			}
		}
		if len(blocked) > 0 {
			return nil, blocked, nil
		}
		seen := map[string]bool{}
		for _, t := range targets {
			for _, p := range append([]string{t}, e.private[t]...) {
				if !seen[p] {
					seen[p] = true
					set = append(set, p)
				}
			}
		}
		return set, nil, nil
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestReclaimKeepsSatelliteSharedWithIncoming(t *testing.T) {
	reclaimEnv{
		packages: map[string][]string{
			"out": {"out-core", "shared-portal"},
			"in":  {"in-core", "shared-portal"},
		},
		private: map[string][]string{
			"out-core":      {"out-lib"},
			"shared-portal": nil,
		},
		size: map[string]int64{"out-core": 100, "out-lib": 50},
	}.install(t)

	rs, err := Reclaim("out", "in")
	if err != nil {
		t.Fatal(err)
	}
	if contains(rs.Packages, "shared-portal") {
		t.Errorf("shared-portal must stay: %v", rs.Packages)
	}
	if !contains(rs.Packages, "out-core") || !contains(rs.Packages, "out-lib") {
		t.Errorf("out-core and its unused library must go: %v", rs.Packages)
	}
	if rs.Size != 150 {
		t.Errorf("size = %d, want 150", rs.Size)
	}
}

func TestReclaimKeepsPackageASurvivorNeeds(t *testing.T) {
	reclaimEnv{
		packages: map[string][]string{
			"out": {"out-core", "out-portal"},
			"in":  {"in-core"},
		},
		private: map[string][]string{
			"out-core":   nil,
			"out-portal": nil,
		},
		needs: map[string]string{"out-portal": "some-app"},
		size:  map[string]int64{"out-core": 200, "out-portal": 80},
	}.install(t)

	rs, err := Reclaim("out", "in")
	if err != nil {
		t.Fatal(err)
	}
	if contains(rs.Packages, "out-portal") || contains(rs.Targets, "out-portal") {
		t.Errorf("out-portal is needed by a survivor and must stay: %+v", rs)
	}
	if !contains(rs.Packages, "out-core") {
		t.Errorf("out-core must still be removed: %v", rs.Packages)
	}
	if rs.Size != 200 {
		t.Errorf("size = %d, want 200 (out-portal excluded)", rs.Size)
	}
}

func TestReclaimNothingInstalledIsHonestlyEmpty(t *testing.T) {
	reclaimEnv{
		packages: map[string][]string{
			"out": {"out-core", "out-portal"},
			"in":  {"in-core"},
		},
		private: map[string][]string{},
		size:    map[string]int64{},
	}.install(t)

	rs, err := Reclaim("out", "in")
	if err != nil {
		t.Fatal(err)
	}
	if rs.Removable || len(rs.Packages) != 0 || rs.Count != 0 || rs.Size != 0 {
		t.Errorf("nothing is installed, so nothing is removable: %+v", rs)
	}
}

// removalSet must keep dropping targets a survivor needs until the plan is
// clean, so a chain of blocked targets still yields the safe remainder.
func TestRemovalSetDropsEachBlockedTarget(t *testing.T) {
	blocked := map[string]bool{"a": true, "b": true}
	plan := func(_ string, targets []string) (set, block []string, err error) {
		for _, t := range targets {
			if blocked[t] {
				block = append(block, t)
			}
		}
		if len(block) > 0 {
			return nil, block, nil
		}
		return append([]string{}, targets...), nil, nil
	}
	removed, kept, err := removalSet("in", []string{"a", "b", "c", "d"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, []string{"c", "d"}) || !reflect.DeepEqual(kept, []string{"c", "d"}) {
		t.Errorf("removed = %v, kept = %v, want [c d] both", removed, kept)
	}
}

// VerifyRemoval is the guard run right before the destructive removal: it must
// refuse when dnf's live plan no longer matches the reviewed set.
func TestVerifyRemovalRefusesOnDivergence(t *testing.T) {
	rs := ReclaimSet{Outgoing: "out", Incoming: "in", Targets: []string{"out-core"}, Packages: []string{"out-core", "out-lib"}, Removable: true}

	cases := []struct {
		name string
		set  []string
		blk  []string
		ok   bool
	}{
		{"matches", []string{"out-core", "out-lib"}, nil, true},
		{"pulls in an extra package", []string{"out-core", "out-lib", "shared-lib"}, nil, false},
		{"would keep one from the set", []string{"out-core"}, nil, false},
		{"a target is now needed", nil, []string{"out-core"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := removalOnce
			t.Cleanup(func() { removalOnce = old })
			removalOnce = func(incoming string, _ []string) (set, blocked []string, err error) {
				if incoming != "in" {
					t.Errorf("verify planned for %q, want the reviewed incoming", incoming)
				}
				return c.set, c.blk, nil
			}
			err := VerifyRemoval(rs)
			if c.ok && err != nil {
				t.Errorf("expected pass, got %v", err)
			}
			if !c.ok && err == nil {
				t.Error("expected refusal, got nil")
			}
		})
	}
}

func TestVerifyRemovalSkipsEmptySet(t *testing.T) {
	if err := VerifyRemoval(ReclaimSet{Removable: false}); err != nil {
		t.Errorf("nothing to remove must not refuse: %v", err)
	}
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var hyprTargets = []string{"ryoku-desktop-hyprland", "hyprland", "hyprpolkitagent", "xdg-desktop-portal-hyprland", "hyprpicker"}

// The switch plan installs the incoming variant and removes the outgoing one in
// one transaction. Only the Removing sections count: the incoming install, the
// upgrades it drags in and their "replacing" lines are not removals.
func TestParseRemovalPlanSwitchTranscript(t *testing.T) {
	plan := parseRemovalPlan(readTestdata(t, "dnf5-switch-plan.txt"))
	for _, want := range append([]string{"aquamarine", "hyprcursor", "hyprgraphics", "hyprwire", "uwsm"}, hyprTargets...) {
		if !contains(plan.removed, want) {
			t.Errorf("%s missing from the removed set", want)
		}
	}
	for _, not := range []string{"ryoku-desktop-niri", "niri", "ryoku-desktop", "ryogami", "gtk-layer-shell", "replacing", "Removing:"} {
		if contains(plan.removed, not) {
			t.Errorf("%s is not a removal", not)
		}
	}
	if len(plan.removed) != 30 {
		t.Errorf("removed %d packages, want the 30 dnf's summary counts", len(plan.removed))
	}
	if deps := plan.dependents(hyprTargets); len(deps) != 0 {
		t.Errorf("dependents = %v, want none once the incoming variant provides the virtual", deps)
	}
}

func TestParseRemovalPlanDependentsTranscript(t *testing.T) {
	plan := parseRemovalPlan(readTestdata(t, "dnf5-remove-dependents.txt"))
	want := []string{"ryoku-desktop", "ryoku-desktop-hyprland", "ryotunes"}
	if got := plan.dependents([]string{"hyprpicker"}); !reflect.DeepEqual(got, want) {
		t.Errorf("dependents = %v, want %v", got, want)
	}
	if !contains(plan.removed, "hyprpicker") || !contains(plan.removed, "webkit2gtk4.1") {
		t.Errorf("targets and unused dependencies belong to the removed set: %v", plan.removed)
	}
}

func TestParseRemovalPlanNothingToDo(t *testing.T) {
	plan := parseRemovalPlan(readTestdata(t, "dnf5-remove-nothing.txt"))
	if len(plan.removed) != 0 || len(plan.dependent) != 0 {
		t.Errorf("nothing to do parsed as %+v", plan)
	}
}

// plannerEnv swaps the dnf and rpm seams under hostRemovalOnce.
func plannerEnv(t *testing.T, plan func(targets []string) string, provides map[string][]string, requires map[string][]string) {
	t.Helper()
	oldRun, oldProv, oldReq := runPlan, targetProvides, packageRequires
	t.Cleanup(func() { runPlan, targetProvides, packageRequires = oldRun, oldProv, oldReq })
	runPlan = func(_ string, targets []string) (string, error) { return plan(targets), nil }
	targetProvides = func([]string) (map[string][]string, error) { return provides, nil }
	packageRequires = func(names []string) ([]string, error) {
		var out []string
		for _, n := range names {
			out = append(out, requires[n]...)
		}
		return out, nil
	}
}

// The libraries only the outgoing compositor used go with it, which is what
// makes the reclaimed size worth reporting.
func TestHostRemovalOnceIncludesUnusedLibraries(t *testing.T) {
	transcript := readTestdata(t, "dnf5-switch-plan.txt")
	plannerEnv(t, func([]string) string { return transcript }, nil, nil)
	set, blocked, err := hostRemovalOnce("niri", hyprTargets)
	if err != nil || blocked != nil {
		t.Fatalf("set err = %v, blocked = %v", err, blocked)
	}
	for _, lib := range []string{"aquamarine", "hyprcursor", "hyprgraphics", "hyprland-qt-support"} {
		if !contains(set, lib) {
			t.Errorf("unused library %s missing from %v", lib, set)
		}
	}
}

// A package the user did not target that needs one (a plugin, a -devel
// package) is a survivor: the target it requires is blocked, never cascaded.
func TestHostRemovalOnceDependentBlocksTarget(t *testing.T) {
	transcript := func(targets []string) string {
		var b strings.Builder
		b.WriteString("Package Arch Version Repository Size\nRemoving:\n")
		for _, p := range targets {
			b.WriteString(" " + p + " x86_64 0:1-1.fc44 repo 1.0 MiB\n")
		}
		if contains(targets, "hyprland") {
			b.WriteString("Removing dependent packages:\n hyprland-devel x86_64 0:1-1.fc44 repo 1.0 MiB\n")
		}
		b.WriteString("Removing unused dependencies:\n aquamarine x86_64 0:1-1.fc44 repo 1.0 MiB\n\nTransaction Summary:\n Removing: 3 packages\n")
		return b.String()
	}
	plannerEnv(t, transcript,
		map[string][]string{"hyprland": {"hyprland", "hyprland(x86-64)", "/usr/bin/Hyprland"}},
		map[string][]string{"hyprland-devel": {"hyprland(x86-64)", "pkgconfig"}})

	_, blocked, err := hostRemovalOnce("niri", []string{"hyprland", "hyprpicker"})
	if err != nil || !reflect.DeepEqual(blocked, []string{"hyprland"}) {
		t.Fatalf("blocked = %v, err = %v; want hyprland blocked", blocked, err)
	}
	removed, kept, err := removalSet("niri", []string{"hyprland", "hyprpicker"}, hostRemovalOnce)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kept, []string{"hyprpicker"}) || contains(removed, "hyprland-devel") || contains(removed, "hyprland") {
		t.Errorf("removed = %v, kept = %v; hyprland and its dependent must stay", removed, kept)
	}
}

// A dependent that maps to no target cannot be resolved by dropping one, so the
// plan refuses rather than reporting a set dnf would exceed.
func TestHostRemovalOnceUnmappedDependentRefuses(t *testing.T) {
	transcript := readTestdata(t, "dnf5-remove-dependents.txt")
	plannerEnv(t, func([]string) string { return transcript }, map[string][]string{}, map[string][]string{})
	if _, _, err := hostRemovalOnce("niri", []string{"hyprpicker"}); err == nil {
		t.Error("an unmapped dependent must refuse the plan")
	}
}

// The planner reads English section headers, so dnf must run in the C locale
// whatever the user's session sets.
func TestDnfCommandForcesCLocale(t *testing.T) {
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LANG", "de_DE.UTF-8")
	cmd := dnfCommand("remove", "--assumeno", "x")
	last := ""
	for _, kv := range cmd.Env {
		if v, ok := strings.CutPrefix(kv, "LC_ALL="); ok {
			last = v
		}
	}
	if last != "C" {
		t.Errorf("effective LC_ALL = %q, want C", last)
	}
}

// rpm prints "%{SIZE}" one per line; a non-numeric line must fail rather than
// undercount the reclaimed space.
func TestSumSizes(t *testing.T) {
	total, err := sumSizes("1024\n\n2048\n")
	if err != nil || total != 3072 {
		t.Errorf("sumSizes = %d, %v; want 3072", total, err)
	}
	if _, err := sumSizes("package foo is not installed\n"); err == nil {
		t.Error("a non-numeric line must fail")
	}
}
