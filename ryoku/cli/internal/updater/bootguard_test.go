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

// Only a boot a Ryoku session started in counts, once, and never the boot the
// update ran in or the one the guard runs in.
func TestSessionBootsCountsSessionsOnce(t *testing.T) {
	p := pendingUpdate{ArmedBoot: "armed", Counted: []string{"counted"}}
	for _, c := range []struct {
		name    string
		markers map[string]string
		want    []string
	}{
		{"no session since the update", map[string]string{}, nil},
		{"session in the update's boot", map[string]string{"session-1000": "armed"}, nil},
		{"session already counted", map[string]string{"session-1000": "counted"}, nil},
		{"session in this boot", map[string]string{"session-1000": "now"}, nil},
		{"session in the previous boot", map[string]string{"session-1000": "prev"}, []string{"prev"}},
		{"two users in the previous boot", map[string]string{"session-1000": "prev", "session-1001": "prev"}, []string{"prev"}},
		{"ok markers are not sessions", map[string]string{"ok-1000": "prev"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			bootMarkers(t, c.markers)
			if got := sessionBoots(p, "now"); !slices.Equal(got, c.want) {
				t.Errorf("sessionBoots = %v, want %v", got, c.want)
			}
		})
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
