package doctor

import (
	"os"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ---- reconciler: fastfetch tagline -------------------------------------------

// The readout's tagline called Ryoku "a hand-built Arch desktop" until the
// Fedora move. config.jsonc is seeded once and then owned by the box, so the
// shipped wording never reaches an existing install on its own; this rewrites
// that one phrase and nothing else. The phrase is plain ASCII, so the seed's
// \u escapes and a Hub-saved config spell it the same way.
const (
	fastfetchTaglineOld = "a hand-built Arch desktop"
	fastfetchTaglineNew = "a hand-built Fedora desktop"
)

func upgradeFastfetchTagline(raw string) (string, bool) {
	if !strings.Contains(raw, fastfetchTaglineOld) {
		return raw, false
	}
	return strings.Replace(raw, fastfetchTaglineOld, fastfetchTaglineNew, 1), true
}

func reconcileFastfetchTagline(checkOnly bool) recResult {
	path := filepath.Join(sys.ConfigHome(), "fastfetch", "config.jsonc")
	raw := readFileSafe(path)
	if raw == "" {
		return okRes(i18n.T("no Ryoku fastfetch config"))
	}
	migrated, changed := upgradeFastfetchTagline(raw)
	if !changed {
		return okRes(i18n.T("fastfetch's tagline is current"))
	}
	if checkOnly {
		return wouldRes(i18n.T("fastfetch's tagline still says Arch")).
			withFix(i18n.T("ryoku doctor rewrites that one phrase"))
	}
	tmp := path + ".ryoku-tmp"
	if err := os.WriteFile(tmp, []byte(migrated), 0o644); err != nil {
		return failRes(i18n.T("could not write %s: %v"), tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return failRes(i18n.T("could not replace %s: %v"), path, err)
	}
	return fixedRes(i18n.T("fastfetch's tagline now says Fedora"))
}
