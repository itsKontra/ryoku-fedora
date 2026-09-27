package main

import "testing"

func TestNeedsNvidiaDriver(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"gpu=turing\ndriver=none\nsecureboot=off\nkey=nocert\n", true},
		{"gpu=turing\ndriver=ryoku\nsecureboot=on\nkey=enrolled\n", false},
		{"gpu=turing\ndriver=host\n", false},
		{"gpu=legacy\ndriver=none\n", false},
		{"gpu=none\ndriver=none\n", false},
		{"", false},
	}
	for _, c := range cases {
		if got := needsNvidiaDriver(c.status); got != c.want {
			t.Errorf("needsNvidiaDriver(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}
