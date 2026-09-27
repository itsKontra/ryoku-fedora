package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Voxtype has no Fedora package, so the Hub installs the RPM upstream attaches
// to each GitHub release. Both commands run in a terminal the Dictation page
// opens, so dnf can prompt for sudo.
//
//	ryoku-hub voxtype install   dnf-install the latest release RPM
//	ryoku-hub voxtype remove    dnf-remove it and drop the user service

const (
	voxtypePackage    = "voxtype"
	voxtypeReleaseAPI = "https://api.github.com/repos/peteonrails/voxtype/releases/latest"
)

type ghRelease struct {
	Tag    string    `json:"tag_name"`
	Assets []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func voxtypeInstall() error {
	url, err := voxtypeRPMURL()
	if err != nil {
		return err
	}
	fmt.Println("Installing", url)
	return ttyRun("sudo", "dnf", "install", "-y", url)
}

// voxtypeRemove leaves the config and models for a reinstall, but disables and
// deletes the user unit so no dead service lingers.
func voxtypeRemove() error {
	if err := ttyRun("sudo", "dnf", "remove", "-y", voxtypePackage); err != nil {
		return err
	}
	_ = userctl("disable", "--now", "voxtype.service")
	_ = os.Remove(voxtypeUnitPath())
	_ = os.RemoveAll(filepath.Join(configHome(), "systemd", "user", "voxtype.service.d"))
	_ = userctl("daemon-reload")
	return nil
}

func voxtypeRPMURL() (string, error) {
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(voxtypeReleaseAPI)
	if err != nil {
		return "", fmt.Errorf("looking up the latest Voxtype release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("looking up the latest Voxtype release: %s", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("reading the Voxtype release: %w", err)
	}
	return pickVoxtypeRPM(rel, rpmArch(runtime.GOARCH))
}

func pickVoxtypeRPM(rel ghRelease, arch string) (string, error) {
	suffix := "." + arch + ".rpm"
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, voxtypePackage+"-") && strings.HasSuffix(a.Name, suffix) {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("Voxtype %s has no %s RPM", rel.Tag, arch)
}

func rpmArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return goarch
}
