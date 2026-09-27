package main

// Shaping an imported file before it lands in a user-include. Dotfiles taken
// from a Ryoku box carry Ryoku's own shipped configs, and those end by loading
// the user-include. Copied into that include verbatim, the include loads
// itself: fish re-sources user.fish until the call stack gives out, and kitty
// reports every include as already included.

import (
	"path/filepath"
	"regexp"
	"strings"
)

// fishSourcesUserLayer matches a fish line that sources user.fish, in any of
// the forms `source f`, `. f` or `test -f f && source f`.
var fishSourcesUserLayer = regexp.MustCompile(`(^|[\s;&|(])(source|\.)\s+\S*user\.fish\b`)

var kittyIncludeKeys = map[string]bool{"include": true, "globinclude": true, "envinclude": true, "geninclude": true}

// layerContent returns the part of an imported file the user layer needs.
// Nothing when it matches what Ryoku already ships at that path or is a
// generated file, only the appended tail when it extends the shipped file, the
// whole file otherwise; lines for which drop is true are removed in every case.
func layerContent(src, shipped string, drop func(line string) bool) string {
	if isGeneratedConfig(src) {
		return ""
	}
	if shipped != "" {
		norm := strings.TrimRight(shipped, "\n")
		if strings.TrimRight(src, "\n") == norm {
			return ""
		}
		if strings.HasPrefix(src, norm+"\n") {
			src = strings.TrimLeft(src[len(norm)+1:], "\n")
		}
	}
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if !drop(line) {
			out = append(out, line)
		}
	}
	body := strings.Join(out, "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return ensureTrailingNL(body)
}

// isGeneratedConfig reports a file a Ryoku renderer writes (the theme daemon,
// matugen). Its header says not to edit it; layering an old copy on top would
// pin a stale palette over every later theme change.
func isGeneratedConfig(src string) bool {
	head := strings.SplitN(src, "\n", 6)
	for _, l := range head[:min(len(head), 5)] {
		if strings.Contains(strings.ToLower(l), "do not edit") {
			return true
		}
	}
	return false
}

func fishDropLine(line string) bool {
	t := strings.TrimSpace(line)
	return !strings.HasPrefix(t, "#") && fishSourcesUserLayer.MatchString(t)
}

// kittyDropLine drops includes of the user layer itself and of any file the
// shipped kitty.conf already includes.
func kittyDropLine(shipped string) func(string) bool {
	shippedTargets := map[string]bool{}
	for _, l := range strings.Split(shipped, "\n") {
		if key, target, ok := kittyInclude(l); ok {
			shippedTargets[key+" "+target] = true
		}
	}
	return func(line string) bool {
		key, target, ok := kittyInclude(line)
		if !ok {
			return false
		}
		return filepath.Base(target) == "user.conf" || shippedTargets[key+" "+target]
	}
}

func kittyInclude(line string) (key, target string, ok bool) {
	f := strings.Fields(line)
	if len(f) < 2 || !kittyIncludeKeys[f[0]] {
		return "", "", false
	}
	return f[0], strings.Join(f[1:], " "), true
}
