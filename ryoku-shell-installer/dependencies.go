package main

import (
	"fmt"
	wm "ryoku-wm"
)

func (e *engine) sourceDependencies(base []string) ([]string, error) {
	provider := wm.SourcePackages(e.p.compositor)
	if len(provider) == 0 {
		return nil, fmt.Errorf("unknown compositor %q", e.p.compositor)
	}
	d := e.d()
	packages := append(d.localAll(base), d.build...)
	packages = append(packages, d.localAll(provider)...)
	seen := map[string]bool{}
	result := []string{}
	for _, pkg := range packages {
		if pkg != "" && !seen[pkg] {
			result = append(result, pkg)
			seen[pkg] = true
		}
	}
	return result, nil
}
