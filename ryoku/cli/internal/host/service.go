package host

import "strings"

var serviceVerbs = map[string]bool{
	"start": true, "stop": true, "restart": true, "try-restart": true,
	"reload": true, "is-active": true, "is-enabled": true, "enable": true,
	"disable": true, "kill": true, "reset-failed": true, "daemon-reload": true,
}

// unitName gives a bare name the .service suffix; a typed unit is kept.
func unitName(name string) string {
	if strings.Contains(name, ".") {
		return name
	}
	return name + ".service"
}

// Service runs a systemctl verb. The scope is the user manager unless
// --system is given. systemctl's own statuses map onto the contract: an
// unknown unit is absent, an inactive or disabled one is false.
func (a *App) Service(args []string) int {
	user := true
	for len(args) > 0 && (args[0] == "--user" || args[0] == "--system") {
		user = args[0] == "--user"
		args = args[1:]
	}
	if len(args) == 0 {
		return ExitUsage
	}
	verb, names := args[0], args[1:]
	if verb == "env" {
		return a.ServiceEnv(names)
	}
	if !serviceVerbs[verb] {
		return ExitUsage
	}
	now := false
	if len(names) > 0 && names[0] == "--now" {
		now, names = true, names[1:]
	}
	if verb == "daemon-reload" {
		if now || len(names) != 0 {
			return ExitUsage
		}
	} else if len(names) == 0 {
		return ExitUsage
	}

	argv := make([]string, 0, len(names)+4)
	if user {
		argv = append(argv, "--user")
	}
	argv = append(argv, verb)
	if now {
		argv = append(argv, "--now")
	}
	if verb == "is-active" || verb == "is-enabled" {
		argv = append(argv, "--quiet")
	}
	for _, name := range names {
		argv = append(argv, unitName(name))
	}
	code := a.run("systemctl", argv...)
	switch {
	case code == 0:
		return ExitOK
	case code == 4 || code == 5:
		return ExitAbsent
	case verb == "is-active" && code == 3, verb == "is-enabled" && code == 1:
		return ExitFalse
	}
	return ExitFailure
}

// ServiceEnv pushes session variables into the user manager and D-Bus
// activation environment: --all, NAME (its current value) or NAME=VALUE.
func (a *App) ServiceEnv(args []string) int {
	if len(args) == 0 {
		return ExitUsage
	}
	if !(len(args) == 1 && args[0] == "--all") {
		for _, arg := range args {
			if !validEnvName(strings.SplitN(arg, "=", 2)[0]) {
				return ExitUsage
			}
		}
	}
	return commandExit(a.run("dbus-update-activation-environment", append([]string{"--systemd"}, args...)...))
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
