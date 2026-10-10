package main

import (
	"strings"
	"testing"
)

// dnf progress repaints end in \r, not \n; they must surface live as
// transient events (throttled), while only newline-terminated lines are final.
func TestCmdReaderSplitsCarriageReturns(t *testing.T) {
	e := &engine{events: make(chan any, 64)}
	err := e.cmd("", nil, "sh", "-c", `printf 'dl 1%%\rdl 2%%\rdl done\n'; printf 'crlf line\r\n'`)
	if err != nil {
		t.Fatalf("cmd: %v", err)
	}
	close(e.events)
	var finals, transients []string
	for msg := range e.events {
		if ln, ok := msg.(evLine); ok {
			s := strings.TrimSpace(ln.line)
			if strings.HasPrefix(s, "$") {
				continue // the echoed command line
			}
			if ln.transient {
				transients = append(transients, s)
			} else {
				finals = append(finals, s)
			}
		}
	}
	if len(transients) != 1 || transients[0] != "dl 1%" {
		t.Errorf("transients = %v, want the first repaint only (throttle eats the second)", transients)
	}
	if len(finals) != 2 || finals[0] != "dl done" || finals[1] != "crlf line" {
		t.Errorf("finals = %v, want [dl done, crlf line] (\\r\\n is one line ending)", finals)
	}
}

// a shell-converted box keeps its own bootloader + initramfs, so the limine
// hook packages -- which pull limine + mkinitcpio back as depends and ship
// pacman hooks that collide with a host's own (Garuda's garuda-hooks, #58) --
// must be filtered out of the install set alongside limine/mkinitcpio/snapper.
