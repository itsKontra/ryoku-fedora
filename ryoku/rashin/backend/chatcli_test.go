package main

import "testing"

func TestFollowOutboxKeepsOrderAcrossAnOutage(t *testing.T) {
	var o followOutbox
	o.push([]byte("a"))
	o.push([]byte("b"))

	var sent []string
	up := false
	send := func(m []byte) bool {
		if !up {
			return false
		}
		sent = append(sent, string(m))
		return true
	}
	o.flush(send)
	if len(sent) != 0 || len(o.lines) != 2 {
		t.Fatalf("a down socket must keep every line: sent %v, kept %d", sent, len(o.lines))
	}

	up = true
	o.push([]byte("c"))
	o.flush(send)
	if got := len(sent); got != 3 || sent[0] != "a" || sent[1] != "b" || sent[2] != "c" {
		t.Fatalf("sent %v, want [a b c]", sent)
	}
	if len(o.lines) != 0 {
		t.Fatalf("outbox not drained: %d left", len(o.lines))
	}
}

func TestFollowOutboxCapsAtTheNewest(t *testing.T) {
	var o followOutbox
	for i := 0; i < followOutboxCap+10; i++ {
		o.push([]byte{byte(i)})
	}
	if len(o.lines) != followOutboxCap || o.lines[0][0] != 10 {
		t.Fatalf("kept %d lines starting at %d, want %d starting at 10", len(o.lines), o.lines[0][0], followOutboxCap)
	}
}
