package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"ryoku-cli/internal/sys"
	"strings"
)

// Version prints the running Ryoku version. Plain form feeds fastfetch's OS
// line ("Ryoku v0.1.0-beta.14"); `--branch` feeds its BRANCH line as
// "<channel> · <sha>" (e.g. "main · dcd7b80"). Deliberately fast: it runs on
// every shell launch, so it never touches the network or `pacman -Sl`. A
// checkout reads git, a packaged box parses the local pacman version. Any
// unknown piece degrades gracefully rather than erroring.
func Version(args []string) error {
	branch, pretty := false, false
	for _, a := range args {
		switch a {
		case "--branch":
			branch = true
		case "--pretty":
			pretty = true
		}
	}
	base, sha := versionParts()
	// --pretty: the line's name in front ("Onogoro v0.56.0-beta.19"); on a
	// terminal the line's art above it. fastfetch and scripts read a pipe, so
	// they get the one line.
	if pretty {
		name := ReleaseName()
		if name != "" {
			if sys.StdoutIsTTY() {
				fmt.Print(sys.Brand(releaseArt(name)))
				fmt.Println()
			}
			fmt.Printf("%s ", name)
		}
	}

	if branch {
		ch := ryokuChannel()
		if sha != "" {
			fmt.Printf("%s · %s\n", ch, sha)
		} else {
			fmt.Println(ch)
		}
		return nil
	}

	if base == "" {
		base = "dev"
		fmt.Println(base)
		return nil
	}
	// a packaged box names the release it runs (/etc/ryoku-release, written
	// by the ryoku-desktop package at publish); the bare core version is the
	// fallback for a box installed before releases were named.
	if rel := sys.ReadRelease(); rel.Release != "" && sys.ResolveRepo() == "" {
		fmt.Println(rel.Release)
		return nil
	}
	fmt.Printf("v%s\n", base)
	return nil
}

// ReleaseName is the name of the line this box runs: /etc/ryoku-release on a
// packaged box, the checkout's CODENAME on a dev box, "" when neither says.
func ReleaseName() string {
	if sys.ResolveRepo() == "" {
		return sys.ReadRelease().Name
	}
	b, err := os.ReadFile(filepath.Join(sys.ResolveRepo(), "CODENAME"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// versionParts returns (base semver, short sha) for the running Ryoku. On a
// checkout: the VERSION file + git HEAD. On a packaged install: parsed from the
// pacman version "<core>.r<count>.g<sha>-<rel>" the repo build embeds (the
// r<count> token is skipped). Any field comes back "" when undeterminable.
func versionParts() (base, sha string) {
	if repo := sys.ResolveRepo(); repo != "" {
		if b, err := os.ReadFile(filepath.Join(repo, "VERSION")); err == nil {
			base = strings.TrimSpace(string(b))
		}
		if out, err := sys.RunOut("git", "-C", repo, "rev-parse", "--short=7", "HEAD"); err == nil {
			sha = strings.TrimSpace(out)
		}
		return base, sha
	}

	// packaged: extract git sha and semver core from Arch or RPM version format.
	rawVer := sys.InstalledVersion()
	for _, tok := range strings.FieldsFunc(rawVer, func(r rune) bool {
		return r == '.' || r == '-' || r == '^' || r == '_'
	}) {
		if len(tok) >= 10 && strings.HasPrefix(tok, "git") && isHex(tok[3:]) {
			sha = tok[3:]
		} else if len(tok) >= 8 && strings.HasPrefix(tok, "g") && isHex(tok[1:]) {
			sha = tok[1:]
		}
	}

	ver := strings.SplitN(rawVer, "-", 2)[0]
	ver = strings.SplitN(ver, "^", 2)[0]
	var core []string
	seen := false
	for _, tok := range strings.Split(ver, ".") {
		switch {
		case len(tok) >= 2 && tok[0] == 'r' && isDigits(tok[1:]):
			seen = true
		case len(tok) >= 8 && tok[0] == 'g' && isHex(tok[1:]):
			seen = true
		default:
			if !seen {
				core = append(core, tok)
			}
		}
	}
	return strings.Join(core, "."), sha
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
