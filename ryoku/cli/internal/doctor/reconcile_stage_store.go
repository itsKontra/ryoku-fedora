package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// The Stage Editor added three keys to stage.json: `models` (the cut-out model
// each quality tier uses), and `editGridSize`/`editGridSnap` (the editor's
// placement grid). The shell and the daemon read sane defaults when a key is
// missing, but a box upgraded from before the editor would otherwise carry a
// store that names no model until the user first touched the Depth page, and
// the Hub and `ryostage` read the file, not the shell's defaults. This seeds the
// missing keys (and any missing tier inside `models`) in place, keeping every
// other key as its own bytes. A store that already has them is left alone.
func reconcileStageStore(checkOnly bool) recResult {
	path := filepath.Join(sys.ConfigHome(), "ryoku", "stage.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return okRes(i18n.T("no stage.json yet (written on the first Stage edit)"))
	}
	migrated, changed, err := seedStageStoreKeys(raw)
	if err != nil {
		return warnRes(i18n.T("stage.json does not parse (%v); Stage falls back to defaults"), err).
			withFix(i18n.T("delete %s to re-seed it"), path)
	}
	if !changed {
		return okRes(i18n.T("stage.json carries the Stage Editor's keys"))
	}
	if checkOnly {
		return wouldRes(i18n.T("stage.json predates the Stage Editor's model and grid keys")).
			withFix(i18n.T("ryoku doctor seeds them in place"))
	}
	tmp := path + ".ryoku-tmp"
	if err := os.WriteFile(tmp, migrated, 0o644); err != nil {
		return failRes(i18n.T("could not write %s: %v"), tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return failRes(i18n.T("could not replace %s: %v"), path, err)
	}
	return fixedRes(i18n.T("seeded the Stage Editor's model and grid keys in stage.json"))
}

// stageModelDefaults mirrors the shell's stage Config.defaultModels and the
// daemon's defaultStageModels: the model each quality tier cuts with out of
// the box. The two Go modules share no package, so the copies are pinned by
// TestStageModelDefaultsMatchShell.
var stageModelDefaults = map[string]string{
	"draft":    "u2netp",
	"standard": "u2netp",
	"fine":     "birefnet-general-lite",
}

// seedStageStoreKeys adds the Stage Editor keys a stage.json is missing and
// reports whether it changed anything. A `models` value that is not an object
// is left alone (a hand edit the shell already ignores), as is every other key.
func seedStageStoreKeys(raw []byte) ([]byte, bool, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, false, err
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}
	changed := false
	if mRaw, ok := top["models"]; ok {
		var models map[string]json.RawMessage
		if json.Unmarshal(mRaw, &models) == nil && models != nil {
			added := false
			for tier, model := range stageModelDefaults {
				if _, has := models[tier]; !has {
					b, _ := json.Marshal(model)
					models[tier] = b
					added = true
				}
			}
			if added {
				b, err := json.Marshal(models)
				if err != nil {
					return nil, false, err
				}
				top["models"] = b
				changed = true
			}
		}
	} else {
		b, err := json.Marshal(stageModelDefaults)
		if err != nil {
			return nil, false, err
		}
		top["models"] = b
		changed = true
	}
	if _, ok := top["editGridSize"]; !ok {
		top["editGridSize"] = json.RawMessage("32")
		changed = true
	}
	if _, ok := top["editGridSnap"]; !ok {
		top["editGridSnap"] = json.RawMessage("true")
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}
