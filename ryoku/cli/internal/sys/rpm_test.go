package sys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPMChannelsPreserveRepositorySecurity(t *testing.T) {
	dir := t.TempDir()
	oldFile, oldBase := RPMRepoFile, RPMBaseFile
	t.Cleanup(func() { RPMRepoFile, RPMBaseFile = oldFile, oldBase })
	RPMRepoFile, RPMBaseFile = filepath.Join(dir, "ryoku.repo"), filepath.Join(dir, "ryoku_baseurl")
	os.WriteFile(RPMBaseFile, []byte("https://fork.example/rpm\n"), 0o644)
	os.WriteFile(RPMRepoFile, []byte("[other]\nbaseurl=https://other.example\n[ryoku]\nbaseurl="+rpmChannelServer("stable")+"\ngpgcheck=1\nrepo_gpgcheck=1\ngpgkey=file:///key\n"), 0o644)
	for _, channel := range []string{"testing", "v1.0.0", "stable"} {
		if err := setRPMChannel(channel); err != nil {
			t.Fatal(err)
		}
		if got := rpmChannelOfServer(rpmRepoServer()); got != channel {
			t.Fatalf("got %q want %q", got, channel)
		}
		b, _ := os.ReadFile(RPMRepoFile)
		if !strings.Contains(string(b), "gpgcheck=1\nrepo_gpgcheck=1") || !strings.Contains(string(b), "baseurl=https://other.example") {
			t.Fatal("lost repo settings")
		}
	}
	if err := setRPMChannel("../untrusted"); err == nil {
		t.Fatal("accepted bad channel")
	}
}
