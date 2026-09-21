package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	i18n "ryoku-i18n"
)

func stepFedoraRepo(e *engine) error {
	if !e.fromSource() {
		if err := e.checkPackageMigration(); err != nil {
			return err
		}
	}
	if err := e.sudo("bash", filepath.Join(e.payload, "release/rpm/enable-dependencies.sh")); err != nil {
		return err
	}
	if !e.fromSource() {
		copr, err := fedoraRepositoryConfig()
		if err != nil {
			return err
		}
		if err := e.sudo("python3", filepath.Join(e.payload, "release/rpm/configure-repo.py"), copr); err != nil {
			return err
		}
		if err := e.sudo("dnf", "--refresh", "--repo=RyokuCOPR", "makecache"); err != nil {
			return fmt.Errorf(i18n.T("refresh the signed Ryoku repository: %w"), err)
		}
		if !e.dry {
			for _, pkg := range []string{"ryoku-desktop", "ryoku-desktop-" + e.p.compositor} {
				output, err := exec.Command("dnf", "-q", "--repo=RyokuCOPR", "repoquery", "--available", pkg).Output()
				if err != nil || len(strings.TrimSpace(string(output))) == 0 {
					return fmt.Errorf(i18n.T("required package %s is unavailable in [RyokuCOPR]; check the Fedora release, architecture and published channel"), pkg)
				}
			}
		}
		if err := e.sudo("dnf", "-y", "--downloadonly", "install", "ryoku-desktop", "ryoku-desktop-"+e.p.compositor); err != nil {
			return fmt.Errorf(i18n.T("desktop dependency preflight failed before conflict removal: %w"), err)
		}
	}
	return nil
}
