package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestSeedStageStoreKeysAddsMissing(t *testing.T) {
	out, changed, err := seedStageStoreKeys([]byte(`{"quality":"fine","front":["clock"]}`))
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["quality"] != "fine" {
		t.Fatalf("quality lost: %v", got["quality"])
	}
	if front, _ := got["front"].([]any); len(front) != 1 || front[0] != "clock" {
		t.Fatalf("front lost: %v", got["front"])
	}
	models, _ := got["models"].(map[string]any)
	if models["fine"] != "birefnet-general-lite" || models["draft"] != "u2netp" || models["standard"] != "u2netp" {
		t.Fatalf("models not seeded: %v", got["models"])
	}
	if got["editGridSize"] != float64(32) || got["editGridSnap"] != true {
		t.Fatalf("grid keys not seeded: %v %v", got["editGridSize"], got["editGridSnap"])
	}
}

func TestSeedStageStoreKeysKeepsChosenModel(t *testing.T) {
	out, changed, err := seedStageStoreKeys([]byte(`{"models":{"fine":"birefnet-portrait"},"editGridSize":48,"editGridSnap":false}`))
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var got struct {
		Models map[string]string `json:"models"`
		Size   int               `json:"editGridSize"`
		Snap   bool              `json:"editGridSnap"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Models["fine"] != "birefnet-portrait" || got.Models["draft"] != "u2netp" {
		t.Fatalf("models = %v", got.Models)
	}
	if got.Size != 48 || got.Snap {
		t.Fatalf("grid overwritten: %d %v", got.Size, got.Snap)
	}
}

func TestSeedStageStoreKeysIdempotent(t *testing.T) {
	full := `{"models":{"draft":"silueta","standard":"u2netp","fine":"birefnet-general"},"editGridSize":16,"editGridSnap":true}`
	if _, changed, err := seedStageStoreKeys([]byte(full)); err != nil || changed {
		t.Fatalf("complete store rewritten: changed=%v err=%v", changed, err)
	}
	if _, _, err := seedStageStoreKeys([]byte(`{`)); err == nil {
		t.Fatal("malformed store accepted")
	}
}

func TestReconcileStageStoreWritesInPlace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "ryoku")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "stage.json")
	if err := os.WriteFile(path, []byte(`{"quality":"draft"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := reconcileStageStore(true); res.status == recFixed {
		t.Fatal("check-only run wrote the store")
	}
	before, _ := os.ReadFile(path)
	if string(before) != `{"quality":"draft"}` {
		t.Fatalf("check-only changed the file: %s", before)
	}
	reconcileStageStore(false)
	after, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(after, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["models"]; !ok {
		t.Fatalf("models not written: %s", after)
	}
	again := reconcileStageStore(false)
	if again.status == recFixed {
		t.Fatal("second run rewrote an already-seeded store")
	}
}

// TestStageModelDefaultsMatchShell pins the doctor's seed to the two tables it
// mirrors: the shell's stage Config.defaultModels and the daemon's
// defaultStageModels. They live in modules that share no package, so a
// default changed in one place must fail here rather than seed a model the
// daemon never cuts with.
func TestStageModelDefaultsMatchShell(t *testing.T) {
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	qml := read("shell/quickshell/shell/modules/stage/Singletons/Config.qml")
	daemon := read("shell/ipc/stage.go")
	daemonField := map[string]string{"draft": "Draft", "standard": "Standard", "fine": "Fine"}
	for tier, model := range stageModelDefaults {
		qmlRe := regexp.MustCompile(`defaultModels: \(\{[^}]*\b` + tier + `: "([^"]+)"`)
		if m := qmlRe.FindStringSubmatch(qml); m == nil || m[1] != model {
			t.Errorf("shell Config.defaultModels.%s = %v, doctor seeds %q", tier, m, model)
		}
		goRe := regexp.MustCompile(`(?s)func defaultStageModels\(\).*?\b` + daemonField[tier] + `:\s*"([^"]+)"`)
		if m := goRe.FindStringSubmatch(daemon); m == nil || m[1] != model {
			t.Errorf("daemon defaultStageModels.%s = %v, doctor seeds %q", daemonField[tier], m, model)
		}
	}
}
