package keyring

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutologinConfigured(t *testing.T) {
	root := t.TempDir()
	if autologinConfigured("", root) {
		t.Fatal("empty /etc must report no autologin")
	}
	confd := filepath.Join(root, "sddm.conf.d")
	writeConf(t, filepath.Join(confd, "kde_settings.conf"), "[Autologin]\nSession=hyprland\nUser=\n")
	if autologinConfigured("", root) {
		t.Fatal("an empty User= is not autologin")
	}
	writeConf(t, filepath.Join(confd, "zz-autologin.conf"), "[Autologin]\nUser=nero\n")
	if !autologinConfigured("", root) {
		t.Fatal("a non-empty User= in a drop-in must count as autologin")
	}
}

func TestAutologinIgnoresOtherSections(t *testing.T) {
	root := t.TempDir()
	writeConf(t, filepath.Join(root, "sddm.conf"), "[General]\nUser=nero\n")
	if autologinConfigured("", root) {
		t.Fatal("User= outside [Autologin] must not count")
	}
}

// SDDM reads the system drop-ins, then /etc/sddm.conf.d, each sorted by name,
// then /etc/sddm.conf, and the last value wins.
func TestAutologinLastFileWins(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"later drop-in clears", map[string]string{
			"etc/sddm.conf.d/10-auto.conf":  "[Autologin]\nUser=nero\n",
			"etc/sddm.conf.d/20-clear.conf": "[Autologin]\nUser=\n",
		}, false},
		{"earlier drop-in clear is overridden", map[string]string{
			"etc/sddm.conf.d/10-clear.conf": "[Autologin]\nUser=\n",
			"etc/sddm.conf.d/20-auto.conf":  "[Autologin]\nUser=nero\n",
		}, true},
		{"sddm.conf outranks every drop-in", map[string]string{
			"etc/sddm.conf.d/99-auto.conf": "[Autologin]\nUser=nero\n",
			"etc/sddm.conf":                "[Autologin]\nUser=\n",
		}, false},
		{"etc drop-in outranks the system dir", map[string]string{
			"sys/50-auto.conf":              "[Autologin]\nUser=nero\n",
			"etc/sddm.conf.d/00-clear.conf": "[Autologin]\nUser=\n",
		}, false},
		{"system dir alone counts", map[string]string{
			"sys/50-auto.conf": "[Autologin]\nUser=nero\n",
		}, true},
		{"a file without User= keeps the earlier value", map[string]string{
			"etc/sddm.conf.d/10-auto.conf":  "[Autologin]\nUser=nero\n",
			"etc/sddm.conf.d/20-theme.conf": "[Theme]\nCurrent=ryoku\n[Autologin]\nSession=hyprland.desktop\n",
		}, true},
		{"dotfiles are not read", map[string]string{
			"etc/sddm.conf.d/.auto.conf": "[Autologin]\nUser=nero\n",
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, body := range c.files {
				writeConf(t, filepath.Join(dir, rel), body)
			}
			if got := autologinConfigured(filepath.Join(dir, "sys"), filepath.Join(dir, "etc")); got != c.want {
				t.Fatalf("autologin = %v, want %v", got, c.want)
			}
		})
	}
}

// SDDM matches section and key names exactly and cuts comments at '#'.
func TestAutologinParsesLikeSDDM(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{"[Autologin]\nUser=nero\n", true},
		{"  [Autologin]  \n  User = nero  \n", true},
		{"[Autologin] # login\nUser=nero # me\n", true},
		{"[autologin]\nUser=nero\n", false},
		{"[Autologin]\nuser=nero\n", false},
		{"[Autologin]\n#User=nero\n", false},
		{"[Autologin]\nUser=# nobody\n", false},
		{"[Autologin]\nUser=nero\nUser=\n", false},
	}
	for _, c := range cases {
		root := t.TempDir()
		writeConf(t, filepath.Join(root, "sddm.conf"), c.body)
		if got := autologinConfigured("", root); got != c.want {
			t.Errorf("%q: autologin = %v, want %v", c.body, got, c.want)
		}
	}
}

func TestSDDMSysConfDirStaysOffFixtures(t *testing.T) {
	t.Setenv("RYOKU_SDDM_CONF_ROOT", t.TempDir())
	if d := sddmSysConfDir(); d != "" {
		t.Fatalf("a fixture root must not read the host system dir, got %q", d)
	}
	t.Setenv("RYOKU_SDDM_SYS_CONF_DIR", "/fixture/sys")
	if d := sddmSysConfDir(); d != "/fixture/sys" {
		t.Fatalf("RYOKU_SDDM_SYS_CONF_DIR must win, got %q", d)
	}
}

func TestStatusModeInference(t *testing.T) {
	data := t.TempDir()
	cfg := t.TempDir()
	confRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("RYOKU_SDDM_CONF_ROOT", confRoot)
	os.MkdirAll(filepath.Join(data, "keyrings"), 0o700)

	pam := filepath.Join(t.TempDir(), "sddm")
	t.Setenv("RYOKU_PAM_FILE", pam)

	// no config, keyring lines stripped -> never-ask (the no-prompt fallback).
	os.WriteFile(pam, []byte(bareSDDM), 0o644)
	st := gatherStatus()
	if st.Mode != ModeNeverAsk || st.ModeSource != "inferred" {
		t.Fatalf("bare stack should infer never-ask, got %q/%q", st.Mode, st.ModeSource)
	}
	if st.PamPresent {
		t.Fatal("the stripped stack has no keyring lines")
	}

	// Fedora's stock stack carries the lines -> unlock-on-login.
	os.WriteFile(pam, []byte(fedoraSDDM), 0o644)
	st = gatherStatus()
	if st.Mode != ModeUnlockOnLogin || !st.PamPresent {
		t.Fatalf("the stock stack should infer unlock-on-login, got %q pam=%v", st.Mode, st.PamPresent)
	}

	// explicit config wins over inference.
	writeConfig(ModeNeverAsk)
	st = gatherStatus()
	if st.Mode != ModeNeverAsk || st.ModeSource != "configured" {
		t.Fatalf("config should win, got %q/%q", st.Mode, st.ModeSource)
	}
}

func TestStatusEncryptedCaveatNotes(t *testing.T) {
	data := t.TempDir()
	cfg := t.TempDir()
	confRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("RYOKU_SDDM_CONF_ROOT", confRoot)
	os.MkdirAll(filepath.Join(data, "keyrings"), 0o700)
	pam := filepath.Join(t.TempDir(), "sddm")
	os.WriteFile(pam, []byte(bareSDDM), 0o644)
	t.Setenv("RYOKU_PAM_FILE", pam)

	// encrypted login keyring under unlock-on-login -> the unknown-match caveat.
	body := append(append([]byte{}, encryptedMagic...), 0x00)
	os.WriteFile(filepath.Join(data, "keyrings", "login.keyring"), body, 0o600)
	writeConfig(ModeUnlockOnLogin)
	st := gatherStatus()
	if len(st.Notes) == 0 {
		t.Fatal("an encrypted default keyring under unlock-on-login must carry the caveat note")
	}
}
