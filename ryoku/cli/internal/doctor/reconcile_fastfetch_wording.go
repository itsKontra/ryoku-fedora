package doctor

import (
	"path/filepath"
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ---- reconciler: fastfetch config wording ------------------------------------

// config.jsonc is seeded once and then owned by the box (the Hub, the store and
// the user edit it in place), so a shipped wording change never reaches an
// existing box on its own. Each migration rewrites one retired phrase and
// nothing else; a phrase the user already changed is left alone.
type fastfetchMigration struct {
	old, new string
	why      string
}

var fastfetchMigrations = []fastfetchMigration{
	// the OS line moved to `ryoku version --pretty` with named releases. The
	// seed writes `2>`; a config the Hub saved carries Go's JSON escape
	// `2>` for the same byte, so both spellings are the old line.
	{`ryoku version 2>/dev/null`, `ryoku version --pretty 2>/dev/null`, "the OS line predates named releases"},
	{`ryoku version 2>/dev/null`, `ryoku version --pretty 2>/dev/null`, "the OS line predates named releases"},
	// the tagline is plain ASCII, so the seed and a Hub-saved config spell it alike.
	{"a hand-built Arch desktop", "a hand-built Fedora desktop", "the tagline still says Arch"},
}

// upgradeFastfetchWording applies every pending migration and returns why each
// one was needed, deduplicated in table order.
func upgradeFastfetchWording(raw string) (string, []string) {
	var whys []string
	for _, m := range fastfetchMigrations {
		if !strings.Contains(raw, m.old) {
			continue
		}
		raw = strings.Replace(raw, m.old, m.new, 1)
		if len(whys) == 0 || whys[len(whys)-1] != m.why {
			whys = append(whys, m.why)
		}
	}
	return raw, whys
}

func reconcileFastfetchWording(checkOnly bool) recResult {
	path := filepath.Join(sys.ConfigHome(), "fastfetch", "config.jsonc")
	raw := readFileSafe(path)
	if raw == "" {
		return okRes(i18n.T("no Ryoku fastfetch config"))
	}
	migrated, whys := upgradeFastfetchWording(raw)
	if len(whys) == 0 {
		return okRes(i18n.T("fastfetch's config wording is current"))
	}
	if checkOnly {
		return wouldRes(i18n.T("fastfetch's config carries retired wording: %s"), strings.Join(whys, "; ")).
			withFix(i18n.T("ryoku doctor rewrites those phrases"))
	}
	if err := sys.WriteFileAtomic(path, migrated, 0o644); err != nil {
		return failRes(i18n.T("could not rewrite %s: %v"), path, err)
	}
	return fixedRes(i18n.T("updated fastfetch's config wording: %s"), strings.Join(whys, "; "))
}
