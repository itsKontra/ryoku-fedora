package main

import (
	"os"

	"ryoku-cli/internal/host"
	"ryoku-cli/internal/kmod"
)

// kmod builds on host's runner, so host cannot dispatch to it; its verbs are
// routed here instead.
func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "kmod" {
		os.Exit(kmod.Execute(args[1:], kmod.Config{}, os.Stdout, os.Stderr))
	}
	os.Exit(host.Default().Execute(args))
}
