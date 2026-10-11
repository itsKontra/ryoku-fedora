package host

// Power asks systemd, which hands the request to logind and its inhibitors.
// Suspend is the exception: the shell daemon holds a sleep block for the whole
// session and owns the fail-closed transaction behind `ryoku-shell suspend`
// (lock first, then release its own block), so a bare systemctl suspend would
// be denied by that block or sleep unlocked. Hibernate has no such transaction
// yet, so it is not offered.
func (a *App) Power(args []string) int {
	if len(args) != 1 {
		return ExitUsage
	}
	switch args[0] {
	case "poweroff", "reboot":
		return commandExit(a.run("systemctl", args[0]))
	case "suspend":
		return commandExit(a.run("ryoku-shell", "suspend"))
	}
	return ExitUsage
}
