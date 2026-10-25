package main

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// The shipped baseline is the first compose layer and the one file the
// package owns outright. Three mistakes hurt here: a section whose plugin is
// not in core.plugins (a bind that silently never fires, which is how the
// development rig shipped for weeks), a plugin missing that the store's rows
// or the act verbs dial (caps that lie), and leftovers from that rig riding
// along into users' sessions. This reads the repo's payload, the exact bytes
// the package installs, and pins all three.
func TestShippedBaselineIsHonest(t *testing.T) {
	raw, err := os.ReadFile("../../wayfire/wayfire.ini")
	if err != nil {
		t.Fatalf("read the shipped baseline: %v", err)
	}
	doc := parseIni(raw)
	if len(doc.sections) == 0 {
		t.Fatal("the shipped baseline parses to nothing")
	}

	loaded := map[string]bool{}
	plugins, _ := doc.get("core", "plugins")
	for _, p := range strings.Fields(plugins) {
		loaded[p] = true
	}
	if len(loaded) == 0 {
		t.Fatal("core.plugins lists no plugins")
	}

	for _, sec := range doc.sections {
		if sec.name == "core" || strings.HasPrefix(sec.name, "output:") {
			continue
		}
		if !loaded[sec.name] {
			t.Errorf("section [%s] has no plugin in core.plugins", sec.name)
		}
	}

	// What the rest of the desktop dials: the store's sections and gestures,
	// the caps, and the act verbs each need their plugin already loaded.
	for _, want := range []string{
		"animate", "autostart", "command", "decoration", "depthdeck",
		"fast-switcher", "gtk-shell", "idle", "ipc", "ipc-rules", "session-lock",
		"shortcuts-inhibit", "simple-tile", "vswipe", "window-rules", "wm-actions",
		"wsets", "xdg-activation",
	} {
		if !loaded[want] {
			t.Errorf("core.plugins is missing %q", want)
		}
	}

	pluginOrder := strings.Fields(plugins)
	if len(pluginOrder) == 0 || pluginOrder[len(pluginOrder)-1] != "autostart" {
		t.Errorf("core.plugins = %q; autostart must initialize after every capability provider", plugins)
	}

	if vwidth, _ := doc.get("core", "vwidth"); vwidth != "5" {
		t.Errorf("core.vwidth = %q, want 5 for the ten-workspace catalogue", vwidth)
	}
	if vheight, _ := doc.get("core", "vheight"); vheight != "2" {
		t.Errorf("core.vheight = %q, want 2 for the ten-workspace catalogue", vheight)
	}
	if xwayland, _ := doc.get("core", "xwayland"); xwayland != "true" {
		t.Errorf("core.xwayland = %q, want true for the variant's X11 apps", xwayland)
	}
	if closeTop, _ := doc.get("core", "close_top_view"); closeTop != "none" {
		t.Errorf("core.close_top_view = %q; the catalogue owns close confirmation and its chords", closeTop)
	}
	if exit, _ := doc.get("core", "exit"); exit != "none" {
		t.Errorf("core.exit = %q; session exit must use Ryoku's guarded logout path", exit)
	}
	if saver, _ := doc.get("idle", "screensaver_timeout"); saver != "0" {
		t.Errorf("idle.screensaver_timeout = %q; ryoku-idle owns the timeout", saver)
	}
	if dpms, _ := doc.get("idle", "dpms_timeout"); dpms != "-1" {
		t.Errorf("idle.dpms_timeout = %q; ryoku-idle owns output power", dpms)
	}
	if tiled, _ := doc.get("simple-tile", "tile_by_default"); tiled != "all" {
		t.Errorf("simple-tile.tile_by_default = %q, want all", tiled)
	}
	for _, option := range []struct {
		section string
		key     string
	}{
		{"simple-tile", "key_toggle"},
		{"simple-tile", "key_focus_above"},
		{"simple-tile", "key_focus_below"},
		{"simple-tile", "key_focus_left"},
		{"simple-tile", "key_focus_right"},
		{"simple-tile", "key_toggle_maximized"},
		{"vswitch", "binding_left"},
		{"vswitch", "binding_right"},
		{"vswitch", "binding_up"},
		{"vswitch", "binding_down"},
	} {
		if got, _ := doc.get(option.section, option.key); got != "none" {
			t.Errorf("%s.%s = %q; the shared catalogue owns that keyboard chord", option.section, option.key, got)
		}
	}
	for _, option := range []struct {
		section string
		key     string
		want    string
	}{
		{"move", "activate", "<super> BTN_LEFT"},
		{"simple-tile", "button_move", "<super> BTN_LEFT"},
		{"resize", "activate", "<super> BTN_RIGHT"},
		{"simple-tile", "button_resize", "<super> BTN_RIGHT"},
	} {
		if got, _ := doc.get(option.section, option.key); got != option.want {
			t.Errorf("%s.%s = %q, want the intentional tiled/floating pointer pair %q", option.section, option.key, got, option.want)
		}
	}
	if mode, _ := doc.get("place", "mode"); mode != "center" {
		t.Errorf("place.mode = %q; untiled Ryoku windows must open centred", mode)
	}
	for _, option := range []struct {
		section string
		key     string
		want    string
	}{
		{"fast-switcher", "activate", "<alt> KEY_TAB"},
		{"fast-switcher", "activate_backward", "<alt> <shift> KEY_TAB"},
		{"grid", "slot_bl", "<ctrl> <super> KEY_KP1"},
		{"grid", "slot_b", "<ctrl> <super> KEY_KP2"},
		{"grid", "slot_br", "<ctrl> <super> KEY_KP3"},
		{"grid", "slot_l", "<ctrl> <super> KEY_KP4"},
		{"grid", "slot_c", "<ctrl> <super> KEY_KP5"},
		{"grid", "slot_r", "<ctrl> <super> KEY_KP6"},
		{"grid", "slot_tl", "<ctrl> <super> KEY_KP7"},
		{"grid", "slot_t", "<ctrl> <super> KEY_KP8"},
		{"grid", "slot_tr", "<ctrl> <super> KEY_KP9"},
		{"grid", "restore", "<ctrl> <super> KEY_KP0"},
		{"vswitch", "with_win_left", "<ctrl> <super> <shift> KEY_LEFT"},
		{"vswitch", "with_win_down", "<ctrl> <super> <shift> KEY_DOWN"},
		{"vswitch", "with_win_up", "<ctrl> <super> <shift> KEY_UP"},
		{"vswitch", "with_win_right", "<ctrl> <super> <shift> KEY_RIGHT"},
		{"zoom", "modifier", "<super>"},
	} {
		if got, _ := doc.get(option.section, option.key); got != option.want {
			t.Errorf("%s.%s = %q, want audited native bind %q", option.section, option.key, got, option.want)
		}
	}

	// The rig's leftovers stay out: each of these sections shipped while its
	// plugin was off, so their binds did nothing, and the depth deck's knobs
	// belong to the store rather than to two copies of the same defaults.
	for _, dead := range []string{
		"alpha", "cube", "depthdeck", "expo", "fisheye",
		"invert", "oswitch", "switcher", "wayfire-shell", "wrot",
	} {
		for _, sec := range doc.sections {
			if sec.name == dead {
				t.Errorf("section [%s] belongs to a plugin this baseline does not run", dead)
			}
		}
	}

	// The catalogue owns its binds now: apply writes them beside this layer,
	// so a command row here would be a second copy waiting to drift.
	if term, _ := doc.get("command", "command_app_terminal"); term != "" {
		t.Errorf("command_app_terminal = %q; the baseline must not carry a second copy of the catalogue", term)
	}
	if cmd := defaultBinds()["app.terminal"].cmd; cmd != "ryoku-app terminal" {
		t.Errorf("app.terminal = %q, want the ryoku app verb", cmd)
	}

	// The login bootstrap: nothing else brings the session up, so a wayfire
	// login without this row is a bare compositor with no shell, no portals
	// restart and no depthdeck, and every other row here would be config for
	// a desktop that never starts.
	session, _ := doc.get("autostart", "0_session")
	for _, want := range []string{
		"ryoku-host session start",
		"xdg-desktop-portal-wlr.service",
	} {
		if !strings.Contains(session, want) {
			t.Errorf("autostart.0_session is missing %q", want)
		}
	}
	if wf, _ := doc.get("autostart", "autostart_wf_shell"); wf != "false" {
		t.Errorf("autostart_wf_shell = %q; the session's shell is Quickshell, not wf-panel", wf)
	}
}

// Every loaded v0.11 plugin default that overlaps Ryoku is overridden in the
// payload above. Compose the catalogue over that payload and reject any chord
// with two owners, except pairs that divide ownership by view type: the pointer
// move/resize bindings and the tiled/floating maximize bindings.
func TestShippedBindingsHaveOneOwnerPerChord(t *testing.T) {
	raw, err := os.ReadFile("../../wayfire/wayfire.ini")
	if err != nil {
		t.Fatalf("read the shipped baseline: %v", err)
	}
	doc := parseIni(raw)
	applyBinds(&doc, defaultStore())

	owners := map[string][]string{}
	for _, sec := range doc.sections {
		for _, option := range sec.keys {
			if !strings.Contains(option.value, "KEY_") && !strings.Contains(option.value, "BTN_") {
				continue
			}
			for _, rawChord := range strings.Split(option.value, "|") {
				chord := normalizedWayfireChord(rawChord)
				if chord != "" {
					owners[chord] = append(owners[chord], sec.name+"."+option.name)
				}
			}
		}
	}

	allowed := map[string]string{
		normalizedWayfireChord("<super> BTN_LEFT"):  "move.activate,simple-tile.button_move",
		normalizedWayfireChord("<super> BTN_RIGHT"): "resize.activate,simple-tile.button_resize",
		// simple-tile toggles tiled views while the maximize act fits floating ones.
		normalizedWayfireChord("<super> KEY_D"): "command.binding_column_maximize,simple-tile.key_toggle_maximized",
	}
	for chord, got := range owners {
		if len(got) < 2 {
			continue
		}
		sort.Strings(got)
		if strings.Join(got, ",") != allowed[chord] {
			t.Errorf("Wayfire chord %q has multiple owners: %s", chord, strings.Join(got, ", "))
		}
	}
}

func normalizedWayfireChord(chord string) string {
	fields := strings.Fields(strings.TrimSpace(chord))
	if len(fields) == 0 {
		return ""
	}
	sort.Strings(fields)
	return strings.Join(fields, " ")
}
