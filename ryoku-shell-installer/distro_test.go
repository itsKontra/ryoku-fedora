package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectDistro(t *testing.T) {
	for _, c := range []struct {
		id, like, want string
	}{
		{"fedora", "", "fedora"},
		{"nobara", "rhel centos fedora", "fedora"},
		{"arch", "", ""},
		{"ubuntu", "debian", ""},
		{"void", "", ""},
	} {
		d := detectDistro(c.id, c.like)
		got := ""
		if d != nil {
			got = d.id
		}
		if got != c.want {
			t.Errorf("detectDistro(%q,%q) = %q, want %q", c.id, c.like, got, c.want)
		}
	}
}

// The step lists are the contract that must not drift: a packaged install
// retires a previous source deployment, a source install builds instead.
func TestSteps(t *testing.T) {
	ids := func(payload string) string {
		e := newEngine(&facts{distro: fedoraLinux, homeDir: t.TempDir()}, &plan{}, true, "main", payload)
		var out []string
		for _, s := range e.steps {
			out = append(out, s.id)
		}
		return strings.Join(out, " ")
	}
	if got, want := ids(""), "sysupgrade tools payload backup repo conflicts packages drivers package-migration session configs shell doctor verify"; got != want {
		t.Errorf("package steps = %q, want %q", got, want)
	}
	if got, want := ids("/checkout"), "sysupgrade tools payload backup repo conflicts packages drivers build session configs shell doctor verify"; got != want {
		t.Errorf("source steps = %q, want %q", got, want)
	}
}

func TestInstallArgs(t *testing.T) {
	if got := strings.Join(fedoraLinux.installArgs([]string{"git"}), " "); got != "dnf -y install --best git" {
		t.Errorf("installArgs = %q", got)
	}
	if got := strings.Join(fedoraLinux.removeArgs([]string{"dunst"}), " "); got != "dnf -y remove dunst" {
		t.Errorf("removeArgs = %q", got)
	}
	if got := strings.Join(fedoraLinux.removeKeepingDepsArgs([]string{"dms"}), " "); got != "dnf -y remove --no-autoremove dms" {
		t.Errorf("removeKeepingDepsArgs = %q", got)
	}
	if got := fedoraLinux.removeKeepingDepsArgs(nil); len(got) != 0 {
		t.Errorf("removeKeepingDepsArgs(nil) = %v, want empty/nil", got)
	}
	if got := fedoraLinux.installArgs(nil); len(got) != 0 {
		t.Errorf("installArgs(nil) = %v, want empty/nil", got)
	}
	if got := fedoraLinux.removeArgs([]string{}); len(got) != 0 {
		t.Errorf("removeArgs([]) = %v, want empty/nil", got)
	}
}

func TestImmutableFedoraRejected(t *testing.T) {
	for _, id := range []string{"silverblue", "kinoite", "coreos", "sway-atomic", "bazzite", "bluefin"} {
		if !immutableFedora("ID=fedora\nVARIANT_ID=\"" + id + "\"\n") {
			t.Errorf("accepted %s", id)
		}
	}
	if immutableFedora("ID=fedora\nVARIANT_ID=workstation\n") {
		t.Fatal("rejected mutable Fedora")
	}
}

func TestFedoraToolsFollowDNFSymlink(t *testing.T) {
	for _, implementation := range []string{"dnf4", "dnf5"} {
		t.Run(implementation, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, implementation), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(implementation, filepath.Join(bin, "dnf")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			e := &engine{f: &facts{distro: fedoraLinux}, dry: true, events: make(chan any, 16)}
			if err := stepTools(e); err != nil {
				t.Fatal(err)
			}
			close(e.events)
			var output string
			for event := range e.events {
				if line, ok := event.(evLine); ok {
					output += line.line + "\n"
				}
			}
			plugin := "dnf-plugins-core"
			if implementation == "dnf5" {
				plugin = "dnf5-plugins"
			}
			if !strings.Contains(output, "dnf -y install --best") || !strings.Contains(output, plugin) {
				t.Fatalf("wrong command or COPR plugin: %s", output)
			}
		})
	}
}
