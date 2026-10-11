package host

// Power asks systemd, which hands the request to logind and its inhibitors.
func (a *App) Power(args []string) int {
	if len(args) != 1 {
		return ExitUsage
	}
	switch args[0] {
	case "poweroff", "reboot", "suspend", "hibernate":
		return commandExit(a.run("systemctl", args[0]))
	}
	return ExitUsage
}
