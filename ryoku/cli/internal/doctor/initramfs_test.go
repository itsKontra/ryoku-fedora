package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitramfsUsesDracutAndPropagatesFailure(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	t.Setenv("PATH", bin)
	t.Setenv("TEST_INITRAMFS_LOG", log)
	os.WriteFile(filepath.Join(bin, "sudo"), []byte("#!/bin/sh\nexec \"$@\"\n"), 0o755)
	path := filepath.Join(bin, "dracut")
	os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$TEST_INITRAMFS_LOG\"\n"), 0o755)
	if err := rebuildInitramfs(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	if strings.TrimSpace(string(data)) != "--regenerate-all --force" {
		t.Fatalf("args: %s", data)
	}
	os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	if err := rebuildInitramfs(); err == nil {
		t.Fatal("ignored dracut failure")
	}
	os.Remove(path)
	if err := rebuildInitramfs(); err == nil {
		t.Fatal("reported success without a builder")
	}
}
