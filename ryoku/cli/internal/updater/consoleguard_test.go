package updater

import (
	"strings"
	"testing"
)

// journal renders messages as the JSON lines journalctl -o json prints.
func journal(msgs ...string) []byte {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(`{"__CURSOR":"s=1","MESSAGE":"` + strings.ReplaceAll(m, `\`, `\\`) + `"}` + "\n")
	}
	return []byte(b.String())
}

const (
	started  = "Started sddm.service - Simple Desktop Display Manager."
	stopping = "Stopping sddm.service - Simple Desktop Display Manager..."
	gOpen    = "pam_unix(sddm-greeter:session): session opened for user sddm(uid=985) by (uid=0)"
	gClose   = "pam_unix(sddm-greeter:session): session closed for user sddm"
	login    = "pam_unix(sddm:session): session opened for user matthias(uid=1000) by matthias(uid=0)"
)

func TestGreeterDied(t *testing.T) {
	cases := []struct {
		name string
		msgs []string
		want bool
	}{
		// the shapes below were read from a Fedora 44 VM's journal
		{"login then shutdown", []string{started, gOpen, login, stopping}, false},
		{"shutdown at the login screen", []string{started, gOpen, stopping}, false},
		{"greeter exits and sddm leaves a black screen", []string{started, gOpen, stopping, started, gOpen, gClose}, true},
		{"greeter closes as sddm stops", []string{started, gOpen, stopping, gClose}, false},
		{"a new greeter replaced the dead one", []string{started, gOpen, gClose, gOpen}, false},
		{"a login followed the closed greeter", []string{started, gOpen, gClose, login}, false},
		{"no sddm in that boot", nil, false},
	}
	for _, c := range cases {
		if got := greeterDied(journal(c.msgs...)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGreeterDiedSkipsNoise(t *testing.T) {
	j := append(journal(started, gOpen, gClose), []byte("not json\n{}\n")...)
	if !greeterDied(j) {
		t.Fatal("malformed lines must not reset the verdict")
	}
}
