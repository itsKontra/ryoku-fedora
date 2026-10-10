package main

// dependencies.go reads the Fedora package sets a source install needs from the
// payload: the hard Requires of the ryoku-desktop RPM and the chosen compositor
// variant, and the desktop sections of the ISO's packages.list. They are the
// lists the RPM and ISO installs read, so a source box cannot drift from either.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	wm "ryoku-wm"
)

// desktopSections are the packages.list sections a converted box takes. [base],
// [kernel] and [bootloader] compose the ISO's own OS; an existing box already
// owns its base system and boot chain, and reinstalling them would rewrite it.
var desktopSections = []string{"drivers", "utilities", "services", "multimedia", "desktop"}

func specPath(payload, name string) string {
	return filepath.Join(payload, "release/rpm", name+".spec")
}

func packagesListPath(payload string) string {
	return filepath.Join(payload, "installation/fedora/packages.list")
}

func (e *engine) sourceDependencies() ([]string, error) {
	pkgs, err := fedoraPackages(e.payload, e.p.compositor)
	if err != nil {
		return nil, err
	}
	return uniq(append(pkgs, e.d().build...)), nil
}

// fedoraPackages is the distribution package set behind ryoku-desktop and the
// chosen compositor. A source install builds every Ryoku package itself, so it
// follows Requires through each first-party spec and keeps only what a
// distribution repository supplies. From packages.list it leaves out the other
// compositors' packages and the RPM Fusion release packages the ISO enables but
// a converted box keeps its own say over.
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

	skip := map[string]bool{}
	for _, other := range wm.Providers() {
		if other != compositor {
			for _, n := range specs["ryoku-desktop-"+other]["Requires"] {
				skip[n] = true
			}
		}
	}
	sections, err := readPackagesList(packagesListPath(payload))
	if err != nil {
		return nil, err
	}
	for _, s := range desktopSections {
		for _, p := range sections[s] {
			if !own[p] && !skip[p] && !strings.HasSuffix(p, "-release") {
				pkgs = append(pkgs, p)
			}
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

// readPackagesList parses installation/fedora/packages.list into its [section]
// sets: one package per line, '#' comments and blank lines dropped.
func readPackagesList(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sections := map[string][]string{}
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = strings.Trim(line, "[]")
		case section != "":
			sections[section] = append(sections[section], line)
		}
	}
	return sections, sc.Err()
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
