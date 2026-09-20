package sys

import (
	"fmt"
	"os"
	"strings"
)

var RPMRepoFile = "/etc/yum.repos.d/ryoku.repo"
var RPMBaseFile = "/etc/dnf/vars/ryoku_baseurl"

func RPMManager() string {
	if Has("pacman") {
		return ""
	}
	if Has("dnf") {
		return "dnf"
	}
	return ""
}
func rpmChannelServer(channel string) string {
	if channel != ChannelStable && channel != ChannelTesting && !IsReleaseTag(channel) {
		return ""
	}
	base, err := os.ReadFile(RPMBaseFile)
	if err != nil {
		return ""
	}
	root := strings.TrimRight(strings.TrimSpace(string(base)), "/")
	if !strings.HasPrefix(root, "https://") && !strings.HasPrefix(root, "file://") {
		return ""
	}
	lane := "/channels/"
	if IsReleaseTag(channel) {
		lane = "/releases/"
	}
	return root + lane + channel + "/$releasever/$basearch"
}
func rpmChannelOfServer(server string) string {
	parts := strings.Split(strings.TrimRight(server, "/"), "/")
	if len(parts) < 4 {
		return ""
	}
	channel := parts[len(parts)-3]
	if rpmChannelServer(channel) == strings.TrimRight(server, "/") {
		return channel
	}
	return ""
}
func rpmRepoServer() string {
	b, _ := os.ReadFile(RPMRepoFile)
	in := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = line == "[ryoku]"
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if in && ok && strings.TrimSpace(k) == "baseurl" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func setRPMChannel(channel string) error {
	server := rpmChannelServer(channel)
	if server == "" {
		return fmt.Errorf("invalid RPM channel or missing %s", RPMBaseFile)
	}
	b, err := os.ReadFile(RPMRepoFile)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	in, done := false, false
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = line == "[ryoku]"
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if in && ok && strings.TrimSpace(k) == "baseurl" {
			lines[i] = "baseurl=" + server
			done = true
		}
	}
	if !done {
		return fmt.Errorf("no [ryoku] baseurl in %s", RPMRepoFile)
	}
	return WriteRootFile(RPMRepoFile, strings.Join(lines, "\n"), "0644")
}
func rpmChannelURL(channel string) string {
	server := rpmChannelServer(channel)
	release, _ := os.ReadFile("/etc/os-release")
	for _, line := range strings.Split(string(release), "\n") {
		if strings.HasPrefix(line, "VERSION_ID=") {
			server = strings.ReplaceAll(server, "$releasever", strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\""))
		}
	}
	return strings.ReplaceAll(server, "$basearch", "x86_64")
}

func RPMReleaseBase() string {
	base, _ := os.ReadFile(RPMBaseFile)
	return strings.TrimRight(strings.TrimSpace(string(base)), "/")
}
