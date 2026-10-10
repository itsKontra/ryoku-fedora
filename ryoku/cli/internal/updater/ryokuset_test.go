package updater

import (
	"reflect"
	"testing"
)

// The set decides what `ryoku update` is allowed to touch, so every property
// here is load-bearing: only packages the RyokuCOPR repo serves, and only ones
// the box actually has.
func TestRyokuSet(t *testing.T) {
	for _, c := range []struct {
		name            string
		repo, installed []string
		want            []string
	}{
		{
			name:      "installed and served, sorted",
			repo:      []string{"ryoku-shell", "ryoku-desktop", "ryotunes"},
			installed: []string{"linux", "ryoku-desktop", "ryoku-shell", "firefox"},
			want:      []string{"ryoku-desktop", "ryoku-shell"},
		},
		{
			name:      "a package the repo serves but the box lacks is not installed by an update",
			repo:      []string{"ryoku-desktop", "asusctl"},
			installed: []string{"ryoku-desktop"},
			want:      []string{"ryoku-desktop"},
		},
		{
			name:      "a kernel is never in the set, because the repo never serves one",
			repo:      []string{"ryoku-desktop"},
			installed: []string{"kernel", "kernel-core", "linux-firmware", "ryoku-desktop"},
			want:      []string{"ryoku-desktop"},
		},
		{
			name:      "nothing served: empty, never a bare dnf target",
			repo:      nil,
			installed: []string{"ryoku-desktop"},
			want:      []string{},
		},
		{
			name:      "duplicate repo lines collapse",
			repo:      []string{"ryoku-desktop", "ryoku-desktop"},
			installed: []string{"ryoku-desktop"},
			want:      []string{"ryoku-desktop"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := ryokuSet(c.repo, c.installed)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ryokuSet = %v, want %v", got, c.want)
			}
		})
	}
}
