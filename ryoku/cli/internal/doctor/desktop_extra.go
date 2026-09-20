package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"ryoku-cli/internal/sys"
	"strings"
	"time"
)

func installDesktopExtra(name string) error {
	helper, _ := exec.LookPath("ryoku-install-extra")
	if repo := sys.ResolveRepo(); repo != "" {
		helper = filepath.Join(repo, "ryoku/shell/scripts/ryoku-install-extra")
	}
	if helper == "" {
		return fmt.Errorf("ryoku-install-extra is missing; update the shell package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "python3", helper, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", output, err)
	}
	return nil
}

func validFont(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic [4]byte
	_, err = io.ReadFull(f, magic[:])
	if err != nil || (string(magic[:]) != "\x00\x01\x00\x00" && string(magic[:]) != "OTTO" && string(magic[:]) != "ttcf") {
		return false
	}
	output, err := exec.Command("fc-scan", "--format", "%{family}", path).Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}
