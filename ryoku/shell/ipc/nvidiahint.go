package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// nvidiaHintDelay lets the shell, which is the notification server, finish
// coming up before the hint is posted; a notification sent earlier is lost.
const nvidiaHintDelay = 20 * time.Second

// nvidiaDriverHint tells the user once per login that their NVIDIA GPU runs on
// nouveau while the signed driver is one command away. The installer normally
// installs it; a failed install step leaves nouveau without saying so, and
// Vulkan compute on nouveau (NVK) is slower and can return corrupt results.
// The marker lives in the runtime dir, so a shell restart does not repeat the
// hint but the next login does, until the driver is installed.
func nvidiaDriverHint(exited <-chan struct{}) {
	select {
	case <-exited:
		return
	case <-time.After(nvidiaHintDelay):
	}
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		return
	}
	marker := filepath.Join(rt, "ryoku-nvidia-hint")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	out, err := exec.Command("ryoku-nvidia", "status").Output()
	if err != nil || !needsNvidiaDriver(string(out)) {
		return
	}
	_ = os.WriteFile(marker, nil, 0o600)
	_ = exec.Command("notify-send", "-a", "Ryoku", "-i", "video-display",
		"NVIDIA driver not installed",
		"Your NVIDIA GPU is running on the open nouveau driver. To install the NVIDIA driver, open a terminal and run:\nsudo ryoku-nvidia install").Run()
}

// needsNvidiaDriver reads `ryoku-nvidia status`: a GPU the signed open driver
// supports (Turing and newer) with no NVIDIA driver installed. Older GPUs have
// no driver to offer, and a host akmod driver is the user's choice.
func needsNvidiaDriver(status string) bool {
	facts := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			facts[k] = v
		}
	}
	return facts["gpu"] == "turing" && facts["driver"] == "none"
}
