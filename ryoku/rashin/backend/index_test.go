package main

import (
	"context"
	"strings"
	"testing"
)

// desktop.md must carry the GUI map so an agent answers "how do I" questions
// GUI-first: the Ryoku Hub deep link, a compositor-neutral page name like
// Graphics & Power, and the picker and bar-settings commands behind the surfaces.
func TestDesktopBodyGUIMap(t *testing.T) {
	body := desktopBody()
	for _, want := range []string{
		"## GUI map",
		"hub open",
		"Graphics & Power",
		"ryogami wallpaper ui",
		"ryoku-shell bar settings",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("desktop.md GUI map missing %q", want)
		}
	}
}

func TestCountUpdates(t *testing.T) {
	out := "Upgrades\n" +
		"7zip.x86_64                 26.04-1.fc44        updates\n" +
		"ImageMagick.x86_64          1:7.1.2.32-1.fc44   updates\n" +
		"\n" +
		"Obsoleting packages\n" +
		"grub2-tools.x86_64          1:2.12-40.fc44      updates\n" +
		"    grub2-tools-efi.x86_64  1:2.12-39.fc44      @System\n"
	if n := countUpdates(out); n != 2 {
		t.Fatalf("countUpdates = %d, want 2 (headings and the obsoleting section do not count)", n)
	}
	if n := countUpdates("    indented.x86_64 1.0-1 @System\n"); n != 0 {
		t.Fatalf("an indented continuation line counted: %d", n)
	}
	if n := countUpdates(""); n != 0 {
		t.Fatalf("empty output counted %d", n)
	}
}

func TestLastLineTakesNewestVersion(t *testing.T) {
	if v := lastLine("6.16.1-200.fc44\n6.17.2-200.fc44\n"); v != "6.17.2-200.fc44" {
		t.Fatalf("lastLine = %q, want the newest of several installed versions", v)
	}
	if v := lastLine("1.2-3.fc44\n"); v != "1.2-3.fc44" {
		t.Fatalf("lastLine = %q", v)
	}
	if v := lastLine(""); v != "" {
		t.Fatalf("lastLine of nothing = %q", v)
	}
}

func TestRpmVersionOneLinePerCopy(t *testing.T) {
	if _, ok := probe(5, "rpm", "--version"); !ok {
		t.Skip("rpm not installed")
	}
	if v := rpmVersion("kernel-core"); strings.Contains(v, "\n") {
		t.Fatalf("rpmVersion glued several versions together: %q", v)
	}
	if v := rpmVersion("ryoku-no-such-package"); v != "" {
		t.Fatalf("a missing package reported %q", v)
	}
}

func TestProbeExitReportsCode(t *testing.T) {
	if out, code, ran := probeExit(context.Background(), 5, "sh", "-c", "echo hi; exit 100"); !ran || code != 100 || strings.TrimSpace(out) != "hi" {
		t.Fatalf("probeExit = %q, %d, %v; want hi, 100, true", out, code, ran)
	}
	if _, _, ran := probeExit(context.Background(), 5, "ryoku-no-such-command"); ran {
		t.Fatal("a missing command must report ran=false")
	}
	if _, ok := probe(5, "sh", "-c", "exit 1"); ok {
		t.Fatal("probe must fail on a non-zero exit")
	}
}
