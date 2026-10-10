package main

// dependencies.go reads the Fedora packages a source install needs from the
// payload: the Requires and Recommends of the ryoku-desktop RPM and the chosen
// compositor variant. It is the set a packaged install pulls, so a source box cannot drift
// from one. The ISO's packages.list is not read: its extra sections (browsers,
// containers, virtualisation, a firewall) are the ISO's choices for a fresh
// machine, not desktop dependencies, and they collide with what an existing box
// already runs (moby-engine against docker-ce).

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	wm "ryoku-wm"
)

// bootChain are ryoku-desktop Recommends a converted box must not get: it
// already owns its initramfs, bootloader and snapshot stack (an ext4 root,
// systemd-boot), and installing these would bring a second one alongside it.
// snapper goes in only on a btrfs root, where ryoku doctor configures the
// snapshots it promises.
var bootChain = map[string]bool{
	"dracut": true, "grubby": true, "grub2-tools": true, "btrfs-progs": true, "snapper": true,
}

func (e *engine) skipsBootChain(p string) bool {
	return bootChain[p] && !(p == "snapper" && e.f.btrfsRoot)
}

// bootChainExcludes keeps the boot chain out of a packaged dnf install, the way
// sourceDependencies keeps it out of a source one. Only what is not installed
// is excluded: excluding an installed package would hide it from the solver
// and break whatever already depends on it.
func (e *engine) bootChainExcludes() []string {
	provided, err := installedCapabilities()
	if e.dry || err != nil {
		return nil
	}
	var skip []string
	for p := range bootChain {
		if e.skipsBootChain(p) && !provided(p) {
			skip = append(skip, p)
		}
	}
	if len(skip) == 0 {
		return nil
	}
	slices.Sort(skip)
	return []string{"--exclude=" + strings.Join(skip, ",")}
}

func specPath(payload, name string) string {
	return filepath.Join(payload, "release/rpm", name+".spec")
}

func (e *engine) sourceDependencies() ([]string, error) {
	pkgs, err := fedoraPackages(e.payload, e.p.compositor)
	if err != nil {
		return nil, err
	}
	pkgs = slices.DeleteFunc(pkgs, e.skipsBootChain)
	return uniq(append(pkgs, e.d().build...)), nil
}

// fedoraPackages is the distribution package set behind ryoku-desktop and the
// chosen compositor. A source install builds every Ryoku package itself, so it
// follows Requires and Recommends through each first-party spec, as dnf does
// with weak dependencies on, and keeps only what a distribution repository
// supplies.
func fedoraPackages(payload, compositor string) ([]string, error) {
	if !slices.Contains(wm.Providers(), compositor) {
		return nil, fmt.Errorf("unknown compositor %q", compositor)
	}
	paths, err := filepath.Glob(filepath.Join(payload, "release/rpm/*.spec"))
	if err != nil {
		return nil, err
	}
	specs := map[string]map[string][]string{}
	own := map[string]bool{}
	for _, path := range paths {
		tags, err := readSpecTags(path)
		if err != nil {
			return nil, err
		}
		for _, n := range tags["Name"] {
			specs[n] = tags
			own[n] = true
		}
		for _, n := range tags["Provides"] {
			own[n] = true
		}
	}

	var pkgs []string
	walked := map[string]bool{}
	var walk func(name string) error
	walk = func(name string) error {
		if walked[name] {
			return nil
		}
		walked[name] = true
		tags, ok := specs[name]
		if !ok {
			return fmt.Errorf("release/rpm has no %s.spec", name)
		}
		for _, r := range append(tags["Requires"], tags["Recommends"]...) {
			switch {
			case specs[r] != nil:
				if err := walk(r); err != nil {
					return err
				}
			case !own[r]:
				pkgs = append(pkgs, r)
			}
		}
		return nil
	}
	for _, name := range []string{"ryoku-desktop", "ryoku-desktop-" + compositor} {
		if err := walk(name); err != nil {
			return nil, err
		}
	}
	return uniq(pkgs), nil
}

// readSpecTags collects the package names of every Name, Provides, Requires and
// Recommends line in an RPM spec, dropping version constraints. It reads plain dependency
// lists only and refuses what it cannot evaluate (rich dependencies, macros
// other than %{name}, conditionals, sub-packages) rather than hand dnf a
// mangled name or silently install the wrong set.
func readSpecTags(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tags := map[string][]string{}
	name := ""
	depth := 0
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", path, n, fmt.Sprintf(format, a...))
		}
		switch directive := strings.TrimSpace(line); {
		case strings.HasPrefix(directive, "%if"):
			depth++
			continue
		case strings.HasPrefix(directive, "%endif"):
			depth--
			continue
		case strings.HasPrefix(directive, "%package"):
			return nil, fail("sub-packages are not supported")
		}
		tag, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		tag, _, _ = strings.Cut(tag, "(") // Requires(post) is still a Requires
		switch tag {
		case "Name", "Provides", "Requires", "Recommends":
		default:
			continue
		}
		if depth > 0 {
			return nil, fail("%s inside a conditional cannot be evaluated", tag)
		}
		names, err := depNames(value, name)
		if err != nil {
			return nil, fail("%v", err)
		}
		if tag == "Name" && len(names) > 0 {
			name = names[0]
		}
		tags[tag] = append(tags[tag], names...)
	}
	return tags, sc.Err()
}

// depNames splits a dependency list ("a, b >= 1.2 c") into its package names.
func depNames(value, name string) ([]string, error) {
	if strings.HasPrefix(strings.TrimSpace(value), "(") {
		return nil, fmt.Errorf("rich dependency %q is not supported", strings.TrimSpace(value))
	}
	value = strings.ReplaceAll(value, "%{name}", name)
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	var names []string
	for i := 0; i < len(fields); i++ {
		switch f := fields[i]; {
		case slices.Contains([]string{"=", "<", ">", "<=", ">="}, f):
			i++ // the version it constrains
		case strings.ContainsAny(f, "%(){}<>="):
			return nil, fmt.Errorf("cannot evaluate %q", f)
		default:
			names = append(names, f)
		}
	}
	return names, nil
}

func uniq(pkgs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pkgs {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
