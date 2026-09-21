package sys

import (
	"fmt"
	"os"
	"strings"
)

const RPMRepoName = "RyokuCOPR"

var RPMRepoFile = "/etc/yum.repos.d/" + RPMRepoName + ".repo"

const COPRServer = "https://download.copr.fedorainfracloud.org/results/itskontra/ryoku/fedora-$releasever-$basearch"
const ChannelCOPR = "copr"

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
	if channel == ChannelCOPR {
		return COPRServer
	}
	return ""
}
func rpmChannelOfServer(server string) string {
	if strings.TrimRight(server, "/") == COPRServer {
		return ChannelCOPR
	}
	return ""
}
func rpmRepoServer() string {
	b, _ := os.ReadFile(RPMRepoFile)
	in := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = line == "["+RPMRepoName+"]"
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
		return fmt.Errorf("Fedora uses the copr channel; stable, testing and frozen release tags are unavailable")
	}
	repoFile := RPMRepoFile
	b, err := os.ReadFile(repoFile)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	in, done := false, false
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = line == "["+RPMRepoName+"]"
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if in && ok && strings.TrimSpace(k) == "baseurl" {
			lines[i] = "baseurl=" + server
			done = true
		}
	}
	if !done {
		return fmt.Errorf("no [RyokuCOPR] baseurl in %s", repoFile)
	}
	return WriteRootFile(repoFile, strings.Join(lines, "\n"), "0644")
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
