package updater

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// bootMarkers points bootOKDir at a temp dir holding the given markers, each
// name mapped to the boot id it records.
func bootMarkers(t *testing.T, markers map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, id := range markers {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := bootOKDir
	bootOKDir = dir
	t.Cleanup(func() { bootOKDir = old })
}

// Only a boot a Ryoku session started in counts, once per boot however many
// users logged in, and never the boot the update ran in or the one the guard
// runs in. Counted markers are removed so a later run cannot count them again.
func TestTakeSessionBootsCountsSessionsOnce(t *testing.T) {
	for _, c := range []struct {
		name    string
		markers map[string]string
		want    int
		left    []string
	}{
		{"no session since the update", map[string]string{}, 0, nil},
		{"session in the update's boot", map[string]string{"session-1000": "armed"}, 0, []string{"session-1000"}},
		{"session in this boot", map[string]string{"session-1000": "now"}, 0, []string{"session-1000"}},
		{"session in the previous boot", map[string]string{"session-1000": "prev"}, 1, nil},
		{"two users in the previous boot", map[string]string{"session-1000": "prev", "session-1001": "prev"}, 1, nil},
		{"two boots the guard missed", map[string]string{"session-1000": "prev", "session-1001": "older"}, 2, nil},
		{"ok markers are not sessions", map[string]string{"ok-1000": "prev"}, 0, []string{"ok-1000"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bootMarkers(t, c.markers)
			if got := takeSessionBoots("armed", "now"); got != c.want {
				t.Errorf("takeSessionBoots = %d, want %d", got, c.want)
			}
			if got := takeSessionBoots("armed", "now"); got != 0 {
				t.Errorf("a second run counted %d boots again", got)
			}
			var left []string
			entries, _ := os.ReadDir(bootOKDir)
			for _, e := range entries {
				left = append(left, e.Name())
			}
			if !slices.Equal(left, c.left) {
				t.Errorf("markers left = %v, want %v", left, c.left)
			}
		})
	}
}

// An update run on top of one that never proved itself keeps the earlier
// update's fallback; once the earlier one is proven, the new one starts fresh.
func TestRearmKeepsTheUnprovenFallback(t *testing.T) {
	dir := t.TempDir()
	old := pendingFile
	pendingFile = filepath.Join(dir, "update-pending.json")
	t.Cleanup(func() { pendingFile = old })
	prev := `{"from":"v1","to":"v2","snapshot":"7","armedBoot":"a","boots":2}`
	if err := os.WriteFile(pendingFile, []byte(prev), 0o644); err != nil {
		t.Fatal(err)
	}
	next := pendingUpdate{From: "v2", To: "v3", Snapshot: "9", ArmedBoot: "b"}

	bootMarkers(t, map[string]string{"session-1000": "x"})
	got := rearm(next)
	if got.From != "v1" || got.To != "v3" || got.Snapshot != "7" || got.Boots != 2 || got.ArmedBoot != "b" {
		t.Errorf("unproven: rearm = %+v", got)
	}

	bootMarkers(t, map[string]string{"ok-1000": "x"})
	if got := rearm(next); got != next {
		t.Errorf("proven: rearm = %+v, want %+v", got, next)
	}
}

func TestProvenAfterIgnoresTheUpdateBoot(t *testing.T) {
	bootMarkers(t, map[string]string{"ok-1000": "armed", "session-1001": "later"})
	if provenAfter("armed") {
		t.Fatal("an ok marker from the update's own boot proves nothing")
	}
	bootMarkers(t, map[string]string{"ok-1000": "later"})
	if !provenAfter("armed") {
		t.Fatal("an ok marker from a later boot proves the update")
	}
}
