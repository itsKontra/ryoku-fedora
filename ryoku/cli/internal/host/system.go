package host

import "regexp"

var keymapPattern = regexp.MustCompile(`^[A-Za-z0-9_+.-]+$`)

// Time reads and sets the system time zone through timedated, which asks
// polkit itself, so the caller does not escalate.
func (a *App) Time(args []string) int {
	switch {
	case len(args) == 1 && args[0] == "zone":
		return commandExit(a.run("timedatectl", "show", "-p", "Timezone", "--value"))
	case len(args) == 2 && args[0] == "set-zone":
		return commandExit(a.run("timedatectl", "set-timezone", args[1]))
	}
	return ExitUsage
}

// Keymap writes the greeter and console layout through localed: localectl
// converts the X11 layout to the nearest console keymap, so one call covers
// both, and it escalates through polkit on its own.
func (a *App) Keymap(args []string) int {
	if len(args) < 2 || len(args) > 4 || args[0] != "set" || !keymapPattern.MatchString(args[1]) {
		return ExitUsage
	}
	variant, options := "", ""
	if len(args) > 2 {
		variant = args[2]
	}
	if len(args) > 3 {
		options = args[3]
	}
	return commandExit(a.run("localectl", "set-x11-keymap", args[1], "", variant, options))
}
