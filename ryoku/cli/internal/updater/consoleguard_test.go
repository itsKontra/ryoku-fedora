package updater

import "testing"

func TestGreeterStartsCountsOneBoot(t *testing.T) {
	journal := []byte(`{"__CURSOR":"s=1","_BOOT_ID":"0123456789abcdef0123456789abcdef"}
{"__CURSOR":"s=2","_BOOT_ID":"0123456789abcdef0123456789abcdef"}
not json
{"__CURSOR":"s=3"}
{"__CURSOR":"s=4","_BOOT_ID":"0123456789abcdef0123456789abcdef"}
`)
	boot, starts := greeterStarts(journal)
	if boot != "01234567-89ab-cdef-0123-456789abcdef" || starts != 3 {
		t.Fatalf("got %q, %d", boot, starts)
	}
}

func TestGreeterStartsEmptyJournal(t *testing.T) {
	if boot, starts := greeterStarts(nil); boot != "" || starts != 0 {
		t.Fatalf("got %q, %d", boot, starts)
	}
}

func TestDashedBootIDMatchesProcForm(t *testing.T) {
	const proc = "01234567-89ab-cdef-0123-456789abcdef"
	for _, in := range []string{"0123456789ABCDEF0123456789ABCDEF", proc} {
		if got := dashedBootID(in); got != proc {
			t.Fatalf("%q -> %q", in, got)
		}
	}
}
