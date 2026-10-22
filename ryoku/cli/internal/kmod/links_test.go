package kmod

import (
	"os"
	"path/filepath"
	"testing"
)

func (f *fakeMachine) link(t *testing.T, machinePath, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path(machinePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, f.path(machinePath)); err != nil {
		t.Fatal(err)
	}
}

func TestAkmodsLinksAreNotNeededWithoutAkmods(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withKey(t, 0o600, 0, 0)
	status, err := fake.machine().Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Key.Links != LinksNotNeeded {
		t.Fatalf("links = %q, want not_needed", status.Key.Links)
	}
}

func TestAkmodsLinkStates(t *testing.T) {
	cases := []struct {
		name    string
		key     bool
		private string
		public  string
		want    LinkState
	}{
		{"both point at the DKMS pair", true, KeyPath, CertPath, LinksOK},
		{"neither exists", true, "", "", LinksMissing},
		{"one missing", true, KeyPath, "", LinksMissing},
		{"both dangling after the key was deleted", false, KeyPath, CertPath, LinksDangling},
		{"akmods made its own key", true, "/etc/pki/akmods/private/host-1.priv", "/etc/pki/akmods/certs/host-1.der", LinksForeign},
		{"a foreign link outranks a missing one", true, "/etc/pki/akmods/private/host-1.priv", "", LinksForeign},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMachine(t)
			fake.withAkmodsGroup(t)
			if tc.key {
				fake.withKey(t, 0o640, 0, testAkmodsGID)
			}
			writeFile(t, fake.path("etc/pki/akmods/private/host-1.priv"), "akmods key", 0o640)
			writeFile(t, fake.path("etc/pki/akmods/certs/host-1.der"), "akmods cert", 0o644)
			if tc.private != "" {
				fake.link(t, AkmodsKeyPath, tc.private)
			}
			if tc.public != "" {
				fake.link(t, AkmodsCertPath, tc.public)
			}
			status, err := fake.machine().Status()
			if err != nil {
				t.Fatal(err)
			}
			if status.Key.Links != tc.want {
				t.Fatalf("links = %q, want %q", status.Key.Links, tc.want)
			}
		})
	}
}

func TestAkmodsKeyFileThatIsNotALinkIsForeign(t *testing.T) {
	fake := newFakeMachine(t)
	fake.withAkmodsGroup(t)
	fake.withKey(t, 0o640, 0, testAkmodsGID)
	writeFile(t, fake.path(AkmodsKeyPath), "akmods key", 0o640)
	fake.link(t, AkmodsCertPath, CertPath)
	status, err := fake.machine().Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Key.Links != LinksForeign {
		t.Fatalf("links = %q, want foreign", status.Key.Links)
	}
}

func TestPrepareKeyPointsAkmodsAtTheDKMSPair(t *testing.T) {
	for _, existing := range []string{"missing", "dangling", "foreign"} {
		t.Run(existing, func(t *testing.T) {
			fake := newFakeMachine(t)
			fake.withAkmodsGroup(t)
			fake.withKey(t, 0o640, 0, testAkmodsGID)
			switch existing {
			case "dangling":
				fake.link(t, AkmodsKeyPath, "/var/lib/dkms/old.key")
				fake.link(t, AkmodsCertPath, "/var/lib/dkms/old.pub")
			case "foreign":
				writeFile(t, fake.path("etc/pki/akmods/private/host-1.priv"), "akmods key", 0o640)
				fake.link(t, AkmodsKeyPath, "/etc/pki/akmods/private/host-1.priv")
				writeFile(t, fake.path(AkmodsCertPath), "akmods cert", 0o644)
			}
			result, err := fake.machine().PrepareKey()
			if err != nil {
				t.Fatal(err)
			}
			if result.Action != KeyFixed || result.Status.Key.Links != LinksOK {
				t.Fatalf("action %q, links %q; want fixed, ok", result.Action, result.Status.Key.Links)
			}
			for link, want := range map[string]string{AkmodsKeyPath: KeyPath, AkmodsCertPath: CertPath} {
				got, err := os.Readlink(fake.path(link))
				if err != nil || got != want {
					t.Fatalf("%s -> %q (%v), want %q", link, got, err, want)
				}
			}
			again, err := fake.machine().PrepareKey()
			if err != nil || again.Action != KeyUnchanged {
				t.Fatalf("second prepare: %q, %v", again.Action, err)
			}
		})
	}
}

func TestPrepareKeyLeavesAkmodsAloneWhereItIsNotInstalled(t *testing.T) {
	fake := newFakeMachine(t)
	if _, err := fake.machine().PrepareKey(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(fake.path("etc/pki/akmods")); !os.IsNotExist(err) {
		t.Fatalf("prepare without akmods touched /etc/pki/akmods: %v", err)
	}
}
