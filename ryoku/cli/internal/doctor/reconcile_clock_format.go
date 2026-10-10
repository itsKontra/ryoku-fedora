package doctor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

const clockFormatMigration = "clock-format-global"

func clockFormatMarker() string {
	return filepath.Join(sys.StateDir(), "migrations", clockFormatMigration)
}

// legacyClockSources is where each surface kept its own hour cycle before
// general.clock_format_24_h became the one Ryoku-wide preference.
type legacyClockSources struct {
	shellStore   []byte // ~/.config/ryoku/shell.json
	widgetsStore []byte // ~/.config/ryoku/widgets.json (desktop clock)
	qsbarCache   []byte // ~/.cache/quickshell_widgets_v2 (QS Bar flags)
}

// reconcileClockFormat folds the per-surface 12/24-hour switches into
// general.clock_format_24_h once. The shell store already carries that key from
// its schema default, but nothing in Ryoku ever let a user set it, so its value
// says nothing about what the user saw; the active bar's own switch does. The
// marker keeps a later choice in Ryoku Settings authoritative.
func reconcileClockFormat(checkOnly bool) recResult {
	marker := clockFormatMarker()
	if sys.Exists(marker) {
		return okRes(i18n.T("clock format already uses the Ryoku-wide preference"))
	}
	ryokuDir := filepath.Join(sys.ConfigHome(), "ryoku")
	path := filepath.Join(ryokuDir, "shell.json")
	shell, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !checkOnly {
			if err := markMigration(marker); err != nil {
				return failRes(i18n.T("could not record the clock format migration: %v"), err)
			}
		}
		return okRes(i18n.T("no shell.json yet; clocks use the 24-hour default"))
	}
	if err != nil {
		return warnRes(i18n.T("could not read %s for the clock format migration: %v"), path, err).
			withFix(i18n.T("fix the file permissions, then run `ryoku doctor`"))
	}
	src := legacyClockSources{shellStore: shell}
	src.widgetsStore, _ = os.ReadFile(filepath.Join(ryokuDir, "widgets.json"))
	src.qsbarCache, _ = os.ReadFile(filepath.Join(sys.Home(), ".cache", "quickshell_widgets_v2"))

	migrated, want24, err := migrateClockFormat(src)
	if err != nil {
		return warnRes(i18n.T("shell.json cannot be safely migrated (%v); it was left untouched"), err).
			withFix(i18n.T("delete %s to re-seed it"), path)
	}
	format := "24-hour"
	if !want24 {
		format = "12-hour"
	}
	if checkOnly {
		return wouldRes(i18n.T("the clocks still keep separate 12/24-hour switches")).
			withFix(i18n.T("ryoku doctor sets the Ryoku-wide preference to %s"), format)
	}
	tmp := path + ".ryoku-tmp"
	if err := os.WriteFile(tmp, migrated, 0o644); err != nil {
		return failRes(i18n.T("could not write %s: %v"), tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return failRes(i18n.T("could not replace %s: %v"), path, err)
	}
	if err := markMigration(marker); err != nil {
		return failRes(i18n.T("the clock format was set, but its migration marker could not be written: %v"), err).
			withFix(i18n.T("run `ryoku doctor` again"))
	}
	return fixedRes(i18n.T("set the Ryoku-wide clock format to %s from the existing clock settings"), format)
}

// migrateClockFormat writes the derived hour cycle to general.clock_format_24_h
// and drops the bars' retired clock12h keys, keeping every other key as its own
// raw bytes. Precedence: the active bar's switch, then the desktop clock's,
// then any other bar's, then the 24-hour default every Ryoku clock shipped with.
func migrateClockFormat(src legacyClockSources) (out []byte, want24 bool, err error) {
	var top map[string]json.RawMessage
	if err = json.Unmarshal(src.shellStore, &top); err != nil {
		return nil, false, err
	}
	if top == nil {
		return nil, false, errors.New(i18n.T("top level must be a JSON object"))
	}
	bars := map[string]map[string]json.RawMessage{}
	for _, name := range []string{"qsbar", "kairos"} {
		raw, ok := top[name]
		if !ok {
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) == nil && obj != nil {
			bars[name] = obj
		}
	}
	twelve := func(bar string) (bool, bool) {
		if v, ok := jsonBool(bars[bar]["clock12h"]); ok {
			return v, true
		}
		if bar == "qsbar" {
			return qsbarCacheTwelve(src.qsbarCache)
		}
		return false, false
	}

	var active string
	_ = json.Unmarshal(top["barStyle"], &active)
	want24 = true
	if v, ok := twelve(active); ok && (active == "qsbar" || active == "kairos") {
		want24 = !v
	} else if v, ok := widgetsClock24(src.widgetsStore); ok {
		want24 = v
	} else if v, ok := twelve("qsbar"); ok {
		want24 = !v
	} else if v, ok := twelve("kairos"); ok {
		want24 = !v
	}

	general := map[string]json.RawMessage{}
	if raw, ok := top["general"]; ok {
		if err = json.Unmarshal(raw, &general); err != nil {
			return nil, false, err
		}
		if general == nil {
			general = map[string]json.RawMessage{}
		}
	}
	general["clock_format_24_h"], _ = json.Marshal(want24)
	if top["general"], err = json.Marshal(general); err != nil {
		return nil, false, err
	}
	for name, obj := range bars {
		if _, ok := obj["clock12h"]; !ok {
			continue
		}
		delete(obj, "clock12h")
		if top[name], err = json.Marshal(obj); err != nil {
			return nil, false, err
		}
	}
	out, err = json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), want24, nil
}

func jsonBool(raw json.RawMessage) (value, ok bool) {
	if raw == nil {
		return false, false
	}
	return value, json.Unmarshal(raw, &value) == nil
}

func widgetsClock24(raw []byte) (bool, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return false, false
	}
	return jsonBool(obj["clock24h"])
}

// qsbarCacheTwelve reads the QS Bar's clock12h flag from its space-separated
// widget cache, mirroring the bar's own loader: older caches lack the
// bluetooth field, which shifts the workspace mode, and clock12h sits three
// fields after the workspace mode.
func qsbarCacheTwelve(raw []byte) (bool, bool) {
	parts := strings.Fields(string(raw))
	if len(parts) < 5 {
		return false, false
	}
	ws := 5
	switch parts[4] {
	case "5", "active", "10":
		ws = 4
	}
	if len(parts) <= ws+3 {
		return false, false
	}
	return parts[ws+3] == "1", true
}
