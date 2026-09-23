package main

import (
	"fmt"
	"os"
	"path/filepath"
	"ryoku-cli/internal/sys"
	i18n "ryoku-i18n"
)

func recoveryURL() string {
	if sys.RPMManager() != "" {
		return "https://raw.githubusercontent.com/itsKontra/ryoku-fedora/main/bin/ryoku-recovery"
	}
	return "https://raw.githubusercontent.com/ryoku-dev/ryoku-arch/main/bin/ryoku-recovery"
}

// cmdRecovery hands off to bin/ryoku-recovery: prefer the copy in a local
// checkout, otherwise fetch the canonical one. The script does the real work and
// does not lean on this binary, so it still recovers when the build is broken.
func cmdRecovery(args []string) error {
	if repo := sys.ResolveRepo(); repo != "" {
		if script := filepath.Join(repo, "bin", "ryoku-recovery"); sys.Exists(script) {
			return sys.Run("bash", append([]string{script}, args...)...)
		}
	}

	url := recoveryURL()
	if !sys.Has("curl") {
		return fmt.Errorf(i18n.T("no local recovery script and curl is missing; run it by hand:\n  curl -fsSL %s | bash"), url)
	}
	tmp, err := os.CreateTemp("", "ryoku-recovery-*.sh")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := sys.Run("curl", "-fsSL", url, "-o", tmp.Name()); err != nil {
		return fmt.Errorf(i18n.T("fetch recovery script from %s: %w"), url, err)
	}
	return sys.Run("bash", append([]string{tmp.Name()}, args...)...)
}
