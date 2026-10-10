package main

// dependencies.go reads the Fedora packages a source install needs from the
// payload: the hard Requires of the ryoku-desktop RPM and the chosen compositor
// variant. It is the set a packaged install pulls, so a source box cannot drift
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

	wm "ryoku-wm"
)

// bootChain are ryoku-desktop Requires a converted box must not get: it already
// owns its initramfs, bootloader and snapshot stack (an ext4 root, systemd-boot),
// and installing these would bring a second one alongside it. snapper goes in
// only on a btrfs root, where ryoku doctor configures the snapshots it promises.
var bootChain = map[string]bool{
	"dracut": true, "grubby": true, "grub2-tools": true, "btrfs-progs": true, "snapper": true,
}

func specPath(payload, name string) string {
	return filepath.Join(payload, "release/rpm", name+".spec")
}

func (e *engine) sourceDependencies() ([]string, error) {
	pkgs, err := fedoraPackages(e.payload, e.p.compositor)
	if err != nil {
		return nil, err
	}
	pkgs = slices.DeleteFunc(pkgs, func(p string) bool {
		return bootChain[p] && !(p == "snapper" && e.f.btrfsRoot)
	})
	return uniq(append(pkgs, e.d().build...)), nil
}

// fedoraPackages is the distribution package set behind ryoku-desktop and the
// chosen compositor. A source install builds every Ryoku package itself, so it
// follows Requires through each first-party spec and keeps only what a
// distribution repository supplies.
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
		for _, r := range tags["Requires"] {
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

// readSpecTags collects the package name of every Name, Provides and Requires
// line in an RPM spec, dropping version constraints.
func readSpecTags(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tags := map[string][]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		tag, value, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		switch tag {
		case "Name", "Provides", "Requires":
			if fields := strings.Fields(value); len(fields) > 0 {
				tags[tag] = append(tags[tag], fields[0])
			}
		}
	}
	return tags, sc.Err()
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
