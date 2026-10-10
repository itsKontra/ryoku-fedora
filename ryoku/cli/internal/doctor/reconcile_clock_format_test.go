package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func clockFormatOf(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("migrated JSON does not parse: %v", err)
	}
	return cfg
}

func TestMigrateClockFormatPrecedence(t *testing.T) {
	const qsbarCache12 = "1 1 0 0 0 active tanzaku 0 1 1"
	const qsbarCache24 = "1 1 0 0 0 active tanzaku 0 0 1"
	for _, tc := range []struct {
		name    string
		src     legacyClockSources
		want24  bool
		dropped []string
	}{
		{"default is 24-hour", legacyClockSources{shellStore: []byte(`{"general":{"clock_format_24_h":false}}`)}, true, nil},
		{"active kairos 12-hour wins", legacyClockSources{
			shellStore:   []byte(`{"barStyle":"kairos","kairos":{"clock12h":true,"scale":1},"general":{"clock_format_24_h":false}}`),
			widgetsStore: []byte(`{"clock24h":true}`),
		}, false, []string{"kairos"}},
		{"active qsbar reads its cache", legacyClockSources{
			shellStore:   []byte(`{"barStyle":"qsbar","qsbar":{}}`),
			widgetsStore: []byte(`{"clock24h":true}`),
			qsbarCache:   []byte(qsbarCache12),
		}, false, nil},
		{"qsbar store key beats its cache", legacyClockSources{
			shellStore: []byte(`{"barStyle":"qsbar","qsbar":{"clock12h":false}}`),
			qsbarCache: []byte(qsbarCache12),
		}, true, []string{"qsbar"}},
		{"other bar falls back to the desktop clock", legacyClockSources{
			shellStore:   []byte(`{"barStyle":"sumi","kairos":{"clock12h":true}}`),
			widgetsStore: []byte(`{"clock24h":false}`),
			qsbarCache:   []byte(qsbarCache24),
		}, false, []string{"kairos"}},
		{"old cache layout without bluetooth", legacyClockSources{
			shellStore: []byte(`{"barStyle":"qsbar"}`),
			qsbarCache: []byte("1 1 0 0 active tanzaku 0 1 1"),
		}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, want24, err := migrateClockFormat(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if want24 != tc.want24 {
				t.Fatalf("want24 = %v, want %v", want24, tc.want24)
			}
			cfg := clockFormatOf(t, out)
			if cfg["general"].(map[string]any)["clock_format_24_h"] != tc.want24 {
				t.Fatalf("general.clock_format_24_h not written: %v", cfg["general"])
			}
			for _, bar := range tc.dropped {
				if _, ok := cfg[bar].(map[string]any)["clock12h"]; ok {
					t.Errorf("%s.clock12h was not dropped", bar)
				}
			}
		})
	}
}

func TestMigrateClockFormatKeepsOtherKeys(t *testing.T) {
	out, _, err := migrateClockFormat(legacyClockSources{
		shellStore: []byte(`{"general":{"temperature_unit":"Imperial"},"kairos":{"clock12h":true,"scale":1.2},"theme":{"theme":"Default"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := clockFormatOf(t, out)
	if cfg["general"].(map[string]any)["temperature_unit"] != "Imperial" {
		t.Error("general.temperature_unit lost")
	}
	if cfg["kairos"].(map[string]any)["scale"] != 1.2 {
		t.Error("kairos.scale lost")
	}
	if _, ok := cfg["theme"]; !ok {
		t.Error("theme lost")
	}
	if _, _, err := migrateClockFormat(legacyClockSources{shellStore: []byte("{")}); err == nil {
		t.Error("a malformed store must not be rewritten")
	}
}

func TestReconcileClockFormatRunsOnce(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(cfg, "ryoku", "shell.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"barStyle":"kairos","kairos":{"clock12h":true},"general":{"clock_format_24_h":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := reconcileClockFormat(true); r.status != recWouldFix {
		t.Fatalf("check-only status = %v, want would", r.status)
	}
	if r := reconcileClockFormat(false); r.status != recFixed {
		t.Fatalf("fix status = %v (%s), want fixed", r.status, r.detail)
	}
	raw, _ := os.ReadFile(path)
	if got := clockFormatOf(t, raw)["general"].(map[string]any)["clock_format_24_h"]; got != false {
		t.Fatalf("clock_format_24_h = %v, want false (the kairos 12-hour choice)", got)
	}
	// A later choice in Ryoku Settings must survive the next doctor run.
	if err := os.WriteFile(path, []byte(`{"barStyle":"kairos","general":{"clock_format_24_h":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := reconcileClockFormat(false); r.status != recOK {
		t.Fatalf("second run status = %v, want ok", r.status)
	}
	raw, _ = os.ReadFile(path)
	if got := clockFormatOf(t, raw)["general"].(map[string]any)["clock_format_24_h"]; got != true {
		t.Fatal("the marker did not keep the user's later choice")
	}
}
