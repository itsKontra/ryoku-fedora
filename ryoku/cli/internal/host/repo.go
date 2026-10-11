package host

import (
	"fmt"

	"ryoku-cli/internal/sys"
)

// Repo reads or moves the Ryoku package channel. `channel` exits absent when
// no RyokuCOPR repo is configured (a checkout box) and false for a baseurl
// Ryoku does not publish.
func (a *App) Repo(args []string) int {
	switch {
	case len(args) == 1 && args[0] == "channel":
		server := a.cfg.Repo.Server()
		if server == "" {
			return ExitAbsent
		}
		channel := sys.ChannelOfServer(server)
		if channel == "" {
			return ExitFalse
		}
		fmt.Fprintln(a.cfg.Stdout, channel)
		return ExitOK
	case len(args) == 2 && args[0] == "set-channel":
		if err := a.cfg.Repo.SetChannel(args[1]); err != nil {
			return a.failf("%v", err)
		}
		return ExitOK
	}
	return ExitUsage
}
