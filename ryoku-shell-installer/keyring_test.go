package main

import "testing"

func TestParseKeyringUnlock(t *testing.T) {
	cases := []struct {
		json string
		want bool
	}{
		{`{"mode":"unlock-on-login","pam_present":true,"autologin":false}`, true},
		{`{"pam_present":false,"autologin":false}`, false},
		{`{"pam_present":true,"autologin":true}`, false},
	}
	for _, c := range cases {
		got, err := parseKeyringUnlock([]byte(c.json))
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v; want %v", c.json, got, err, c.want)
		}
	}
	if _, err := parseKeyringUnlock([]byte("usage: ryoku keyring")); err == nil {
		t.Error("output that is not JSON must be an error, not a silent false")
	}
}
