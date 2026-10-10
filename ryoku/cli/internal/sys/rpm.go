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

// RPMManager is "dnf" on a box that has it, "" otherwise.
func RPMManager() string {
	if Has("dnf") {
		return "dnf"
	}
	return ""
}

// ChannelServer is the RyokuCOPR baseurl for a channel, or "" for a name that
// is not one. Fedora has the one rolling COPR channel.
func ChannelServer(channel string) string {
	if channel == ChannelCOPR {
		return COPRServer
	}
	return ""
}

// ChannelOfServer maps a baseurl back to its channel name, "" for a mirror Ryoku
// does not publish, which is left alone everywhere.
func ChannelOfServer(server string) string {
	if strings.TrimRight(server, "/") == COPRServer {
		return ChannelCOPR
	}
	return ""
}

// RyokuServer returns the baseurl of the RyokuCOPR repo, or "" when the repo
// file or its baseurl is absent.
func RyokuServer() string {
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

// SetPackagedChannel rewrites the RyokuCOPR baseurl to channel. It needs a repo
// file to rewrite; the installer writes it. Written through sudo install, so
// the file is replaced whole and its gpg settings survive.
func SetPackagedChannel(channel string) error {
	server := ChannelServer(channel)
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
