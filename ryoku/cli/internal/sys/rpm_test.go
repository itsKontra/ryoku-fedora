package sys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCOPRChannelPreservesRepositorySecurity(t *testing.T) {
	oldFile := RPMRepoFile
	t.Cleanup(func() { RPMRepoFile = oldFile })
	RPMRepoFile = filepath.Join(t.TempDir(), "ryoku.repo")
	original := "[other]\nbaseurl=https://other.example\n[RyokuCOPR]\nbaseurl=" + COPRServer + "/\ngpgcheck=1\nrepo_gpgcheck=0\ngpgkey=file:///key\n"
	if err := os.WriteFile(RPMRepoFile, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := rpmChannelOfServer(rpmRepoServer()); got != ChannelCOPR {
		t.Fatalf("channel = %q", got)
	}
	for _, channel := range []string{"stable", "testing", "v1.0.0", "../untrusted"} {
		if rpmChannelServer(channel) != "" {
			t.Fatalf("accepted unsupported channel %q", channel)
		}
		if err := setRPMChannel(channel); err == nil {
			t.Fatal("changed unsupported channel")
		}
	}
	data, _ := os.ReadFile(RPMRepoFile)
	if string(data) != original || !strings.Contains(string(data), "gpgcheck=1") {
		t.Fatal("changed repository security")
	}
	if rpmChannelOfServer("https://other.example/fedora-44-x86_64") != "" {
		t.Fatal("recognized foreign repository")
	}
}
