package updater

import "testing"

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.3916-35.fc44", "0.3916-35.fc44", 0},
		{"0.3739-24.fc44", "0.3916-35.fc44", -1},
		{"0.3916-35.fc44", "0.3739-24.fc44", 1},
		{"0.3916-9.fc44", "0.3916-10.fc44", -1},
		{"1.0-2", "1.0.1-1", -1},
		{"1.010", "1.9", 1},
		{"1.0a", "1.0", 1},
		{"1.0", "1.0a", -1},
		{"2.0", "2.a", 1},
		{"1.0~rc1", "1.0", -1},
		{"1.0^git1", "1.0", 1},
		{"1.0^git1", "1.0.1", -1},
		{"1:0.1-1", "9.9-1", 1},
		{"0.12.6.r1184.g86a91f4-1", "0.12.6.r1190.g1a2b3c4-1", -1},
	}
	for _, c := range cases {
		if got := versionCompare(c.a, c.b); got != c.want {
			t.Errorf("versionCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
