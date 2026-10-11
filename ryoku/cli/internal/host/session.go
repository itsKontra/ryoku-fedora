package host

// Session runs the compositor's login handoff. `start` is the chain both
// autostarts ran inline:
//
//	dbus-update-activation-environment --systemd --all;
//	systemctl --user daemon-reload;
//	ryoku-power-cutover session-start-logged &&
//	  systemctl --user try-restart xdg-desktop-portal.service <extra...>
//
// The environment push and the reload deliberately continue after a failure;
// only the cutover gates the portal restart. --all, not a name list: a systemd
// or D-Bus launched app sees only what is pushed here. Portals restart last, so
// a stale frontend from a previous session cannot proxy ScreenCast requests to
// a dead backend. The extras are the compositor's own portal backends.
//
// `ensure` and `check` repair a session supervisor; systemd's user manager
// needs none, so they succeed and keep the verbs callable on every host.
func (a *App) Session(args []string) int {
	if len(args) == 0 {
		return ExitUsage
	}
	switch args[0] {
	case "start":
		_ = a.run("dbus-update-activation-environment", "--systemd", "--all")
		_ = a.run("systemctl", "--user", "daemon-reload")
		if a.run("ryoku-power-cutover", "session-start-logged") != 0 {
			return ExitFailure
		}
		units := append([]string{"--user", "try-restart", "xdg-desktop-portal.service"}, args[1:]...)
		return commandExit(a.run("systemctl", units...))
	case "ensure":
		if len(args) != 2 || args[1] != "--system" && args[1] != "--user" {
			return ExitUsage
		}
		return ExitOK
	case "check":
		if len(args) != 1 {
			return ExitUsage
		}
		return ExitOK
	}
	return ExitUsage
}
