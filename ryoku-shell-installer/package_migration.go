package main

import (
	"fmt"
	"os"
	"path/filepath"

	i18n "ryoku-i18n"
)

func (e *engine) checkPackageMigration() error {
	entries, err := readOwnership(e.f.homeDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		for _, rel := range []string{".local/bin/ryoku", ".local/bin/ryoku-shell", ".local/lib/qt6/qml/Ryoku/Ui/qmldir"} {
			if pathExists(filepath.Join(e.f.homeDir, rel)) {
				return fmt.Errorf(i18n.T("local desktop artifact %s has no ownership receipt; preserve or move it before switching to packages"), rel)
			}
		}
	}
	for _, marker := range []string{"repo", "deployed"} {
		if pathExists(filepath.Join(e.f.homeDir, ".local/state/ryoku", marker)) && len(entries) == 0 {
			return fmt.Errorf("%s", i18n.T("source deployment has no ownership receipt; retain --install-mode=source until its local binaries and service overrides are migrated"))
		}
	}
	for rel, entry := range entries {
		if !artifactAllowed(rel) {
			return fmt.Errorf(i18n.T("invalid source ownership path %q"), rel)
		}
		path := filepath.Join(e.f.homeDir, rel)
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || parent != filepath.Dir(path) {
			return fmt.Errorf(i18n.T("source ownership path changed: %s"), path)
		}
		a, err := readArtifact(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if a.digest() != entry.Installed {
			return fmt.Errorf(i18n.T("source artifact %s was edited; preserve or move it before switching to packages"), path)
		}
	}
	return nil
}

func stepPackageMigration(e *engine) error {
	if e.d().id != "fedora" || e.fromSource() {
		return nil
	}
	if err := e.checkPackageMigration(); err != nil {
		return err
	}
	if err := uninstallArtifacts(e.f.homeDir, e.dry, func(name string, args ...string) error {
		return e.cmd("", nil, name, args...)
	}); err != nil {
		return err
	}
	if !e.dry {
		for _, marker := range []string{"repo", "deployed"} {
			err := os.Remove(filepath.Join(e.f.homeDir, ".local/state/ryoku", marker))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
