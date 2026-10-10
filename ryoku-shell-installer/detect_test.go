package main

import "testing"

func TestParseOSRelease(t *testing.T) {
	id, like, name := parseOSRelease("NAME=\"Nobara Linux\"\nPRETTY_NAME=\"Nobara Linux 44\"\nID=nobara\nID_LIKE=\"rhel centos fedora\"\n")
	if id != "nobara" || like != "rhel centos fedora" || name != "Nobara Linux 44" {
		t.Fatalf("got %q %q %q", id, like, name)
	}
}

func TestShellJoin(t *testing.T) {
	if got := shellJoin("dnf", []string{"install", "a b"}); got != "dnf install \"a b\"" {
		t.Fatalf("got %q", got)
	}
}

func TestSecureBootEnforcing(t *testing.T) {
	attr := []byte{6, 0, 0, 0} // efivarfs attribute header
	on, off := append(attr, 1), append(attr, 0)
	if !secureBootEnforcing(on, off) {
		t.Fatal("SecureBoot=1 SetupMode=0 must be enforcing")
	}
	if secureBootEnforcing(on, on) {
		t.Fatal("setup mode means nothing is enforced")
	}
	if secureBootEnforcing(off, off) || secureBootEnforcing(nil, nil) || secureBootEnforcing([]byte{1}, nil) {
		t.Fatal("short or zero variables must not read as enforcing")
	}
}
