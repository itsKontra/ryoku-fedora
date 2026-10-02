package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ryoku-cli/internal/ryotunesrelease"
)

func TestRyotunesReleaseUpdaterSkippedOnRPM(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "dnf"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	oldCheck := checkRyotunesRelease
	oldUpgrade := upgradeRyotunesRelease
	t.Cleanup(func() {
		checkRyotunesRelease = oldCheck
		upgradeRyotunesRelease = oldUpgrade
	})

	checkCalled := false
	upgradeCalled := false
	checkRyotunesRelease = func(context.Context) (ryotunesrelease.Status, error) {
		checkCalled = true
		return ryotunesrelease.Status{}, nil
	}
	upgradeRyotunesRelease = func(context.Context) (ryotunesrelease.Status, error) {
		upgradeCalled = true
		return ryotunesrelease.Status{}, nil
	}

	upgradeRyotunes()
	addRyotunesUpdate(&statusReport{})

	if upgradeCalled || checkCalled {
		t.Fatalf("Fedora must not use the Arch Ryotunes release updater: upgrade=%v check=%v", upgradeCalled, checkCalled)
	}
}
