package main

import "testing"

func TestGroupPlanItems(t *testing.T) {
	on := true
	var items []planItem
	for _, l := range []string{
		"Resume the previous run",
		"NVIDIA proprietary drivers", "Switch login to SDDM", "Enable SDDM login",
		"Ryoku greeter theme", "AZERTY keyboard (French)", "Switch to NetworkManager",
		"Remove rival shells", "Disable conflicting daemons", "Carry over monitor layout",
		"Developer toolchain", "fish as login shell",
	} {
		items = append(items, planItem{label: l, on: &on})
	}
	got := groupPlanItems(items)
	headers := 0
	for _, it := range got {
		if it.on == nil {
			headers++
		}
	}
	if headers != 3 {
		t.Fatalf("want 3 section headers over %d toggles, got %d", len(items), headers)
	}
	if got[0].on == nil {
		t.Fatal("the resume row stays on top, before any header")
	}
	if firstToggle(got) != 0 {
		t.Fatalf("first toggle should be the resume row, got %d", firstToggle(got))
	}

	short := groupPlanItems(items[:6])
	if len(short) != 6 {
		t.Fatalf("short plans stay flat, got %d rows", len(short))
	}
}

func TestAzertyPlanItems(t *testing.T) {
	find := func(items []planItem, label string) *planItem {
		for i := range items {
			if items[i].label == label {
				return &items[i]
			}
		}
		return nil
	}
	p := &plan{}
	items := buildItems(&facts{kbLayout: "us"}, p)
	fr := find(items, "AZERTY keyboard (French)")
	be := find(items, "AZERTY keyboard (Belgian)")
	if fr == nil || be == nil {
		t.Fatal("AZERTY toggles missing on a plain us layout")
	}
	if fr.on != &p.azertyFR || be.on != &p.azertyBE {
		t.Fatal("AZERTY toggles wired to the wrong plan fields")
	}
	if find(buildItems(&facts{kbLayout: "de"}, &plan{}), "AZERTY keyboard (French)") != nil {
		t.Fatal("AZERTY toggles must stay hidden when a layout was salvaged")
	}
	if find(buildItems(&facts{}, &plan{}), "AZERTY keyboard (Belgian)") == nil {
		t.Fatal("AZERTY toggles missing when no layout was salvaged")
	}

	// the two choices are exclusive: the one just switched on wins.
	*fr.on = true
	p.azertyExclusive(fr.on)
	*be.on = true
	p.azertyExclusive(be.on)
	if p.azertyFR || !p.azertyBE {
		t.Fatalf("want fr=false be=true after toggling both, got fr=%v be=%v", p.azertyFR, p.azertyBE)
	}
	*be.on = false
	p.azertyExclusive(be.on)
	if p.azertyFR || p.azertyBE {
		t.Fatal("switching a toggle off must not resurrect the other")
	}
}

func TestCleanTermLine(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "installing gpk\n", "installing gpk"},
		{"curl progress keeps the last repaint", "  0     0    0\r 12 12.2M   12  1.54M\n", " 12 12.2M   12  1.54M"},
		{"trailing crlf", "done\r\n", "done"},
		{"tabs spaced", "a\tb", "a  b"},
		{"control bytes dropped", "ok\x1b[1m!\x07", "ok[1m!"},
		{"only a bare repaint", "\r", ""},
	}
	for _, c := range cases {
		if got := cleanTermLine(c.in); got != c.want {
			t.Errorf("%s: cleanTermLine(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
