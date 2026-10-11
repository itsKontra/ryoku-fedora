package host

import "fmt"

const Usage = `Usage: ryoku-host <verb> [arguments]
       ryoku-host --help

Host detection:
  init
  pkgmgr
  repo channel
  repo set-channel <channel>

Services:
  svc [--user|--system] <start|stop|restart|try-restart|reload|is-active|is-enabled|enable|disable|kill|reset-failed> [--now] <name>...
  svc [--user|--system] daemon-reload
  svc env [--all | NAME | NAME=VALUE ...]

Session:
  session start [extra-unit...]
  session ensure <--system|--user>
  session check

Packages (Fedora package names):
  pkg installed <name>...
  pkg available <name>...
  pkg install [--upgrade] <name>...
  pkg install-file <path>...
  pkg remove <name>...
  pkg version <name>
  pkg owner <path>

System settings:
  power <poweroff|reboot|suspend|hibernate>
  time zone
  time set-zone <Area/City>
  keymap set <layout> [variant] [options]

Exit codes: 0 success/true, 1 failure, 2 usage, 3 false, 4 absent, 5 not provided.
`

// Execute runs one ryoku-host invocation and returns its exit code.
func (a *App) Execute(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.cfg.Stderr, Usage)
		return ExitUsage
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "--help", "-h", "help":
		fmt.Fprint(a.cfg.Stdout, Usage)
		return ExitOK
	case "init":
		return a.InitVerb(rest)
	case "pkgmgr":
		return a.PackageManagerVerb(rest)
	case "repo":
		return a.Repo(rest)
	case "svc":
		return a.Service(rest)
	case "session":
		return a.Session(rest)
	case "pkg":
		return a.Package(rest)
	case "power":
		return a.Power(rest)
	case "time":
		return a.Time(rest)
	case "keymap":
		return a.Keymap(rest)
	}
	fmt.Fprint(a.cfg.Stderr, Usage)
	return ExitUsage
}
