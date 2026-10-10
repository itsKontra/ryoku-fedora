package keyring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fedoraSDDM is Fedora's stock /etc/pam.d/sddm (sddm 0.21, Fedora 44), copied
// unmodified into testdata. It already carries the optional pam_gnome_keyring
// lines, so a fresh install reads as unlock-on-login.
var fedoraSDDM = readTestdata("fedora-sddm")

func readTestdata(name string) string {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// bareSDDM is fedoraSDDM with its keyring lines stripped, the stack the
// never-ask and ask modes leave and the byte-for-byte baseline the wiring
// must preserve around its inserts.
var bareSDDM = stripKeyring(fedoraSDDM)

func TestFedoraStockStackIsWired(t *testing.T) {
	if !pamHasKeyring(fedoraSDDM, "auth") || !pamHasKeyring(fedoraSDDM, "session") {
		t.Fatal("Fedora's stock stack carries both optional keyring lines")
	}
	if out, missing := applyPAMText(fedoraSDDM, true); out != fedoraSDDM || len(missing) != 0 {
		t.Fatalf("converging the stock stack must be a no-op, missing %v:\n%s", missing, out)
	}
}

// ask (or never-ask) and then unlock-on-login must give back the stock file
// byte-for-byte, so the package-owned stack never counts as modified.
func TestStockStackRoundTrips(t *testing.T) {
	stripped, _ := applyPAMText(fedoraSDDM, false)
	if stripped != bareSDDM || pamPresent(stripped) {
		t.Fatalf("strip must remove exactly the two keyring lines:\n%s", stripped)
	}
	if back, missing := applyPAMText(stripped, true); back != fedoraSDDM || len(missing) != 0 {
		t.Fatalf("unlock-on-login must restore the stock file byte-for-byte, missing %v:\n%s", missing, back)
	}

	p := filepath.Join(t.TempDir(), "sddm")
	if err := os.WriteFile(p, []byte(fedoraSDDM), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RYOKU_PAM_FILE", p)
	for _, mode := range []string{ModeAsk, ModeUnlockOnLogin, ModeNeverAsk, ModeUnlockOnLogin} {
		if err := runApplyPAM([]string{mode}); err != nil {
			t.Fatalf("apply-pam %s: %v", mode, err)
		}
	}
	if got, _ := os.ReadFile(p); string(got) != fedoraSDDM {
		t.Fatalf("the file did not round-trip to the stock stack:\n%s", got)
	}
}

func TestApplyPAMTextInsert(t *testing.T) {
	out, missing := applyPAMText(bareSDDM, true)
	if len(missing) != 0 {
		t.Fatalf("stock stack has both anchors; got missing %v", missing)
	}
	if !pamHasKeyring(out, "auth") || !pamHasKeyring(out, "session") {
		t.Fatalf("both keyring lines must be present after insert:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "auth") && strings.Contains(l, "password-auth") {
			if lines[i+1] != pamAuthLine {
				t.Fatalf("auth keyring line must sit right after the auth include, got %q", lines[i+1])
			}
		}
		if strings.HasPrefix(l, "session") && strings.Contains(l, "password-auth") {
			if lines[i+1] != pamSessionLine {
				t.Fatalf("session keyring line must sit right after the session include, got %q", lines[i+1])
			}
		}
	}
	// the untouched lines survive verbatim.
	if !strings.Contains(out, "-auth        optional      pam_kwallet5.so") ||
		!strings.Contains(out, "session     optional      pam_keyinit.so force revoke") {
		t.Fatalf("insert disturbed unrelated lines:\n%s", out)
	}
}

func TestApplyPAMTextIdempotent(t *testing.T) {
	once, _ := applyPAMText(bareSDDM, true)
	twice, _ := applyPAMText(once, true)
	if once != twice {
		t.Fatalf("second insert changed the file (duplicate lines):\n%s", twice)
	}
	if strings.Count(twice, "pam_gnome_keyring.so") != 2 {
		t.Fatalf("want exactly two keyring lines, got %d", strings.Count(twice, "pam_gnome_keyring.so"))
	}
}

func TestApplyPAMTextStrip(t *testing.T) {
	wired, _ := applyPAMText(bareSDDM, true)
	stripped, _ := applyPAMText(wired, false)
	if pamPresent(stripped) {
		t.Fatalf("strip left keyring lines behind:\n%s", stripped)
	}
	if stripped != bareSDDM {
		t.Fatalf("strip did not restore the stock stack byte-for-byte:\n%q", stripped)
	}
	// strip is idempotent on an already-clean stack.
	again, _ := applyPAMText(stripped, false)
	if again != stripped {
		t.Fatalf("strip is not idempotent")
	}
}

func TestApplyPAMTextMissingAnchor(t *testing.T) {
	noAnchors := "#%PAM-1.0\n\naccount     include     password-auth\n"
	out, missing := applyPAMText(noAnchors, true)
	if out != noAnchors {
		t.Fatalf("with no anchors the content must be untouched:\n%s", out)
	}
	if len(missing) != 2 {
		t.Fatalf("both anchors should be reported missing, got %v", missing)
	}
}

func TestApplyPAMFileWritesOnlyOnChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sddm")
	if err := os.WriteFile(p, []byte(bareSDDM), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyPAMFile(p, ModeUnlockOnLogin); err != nil {
		t.Fatalf("apply: %v", err)
	}
	info, _ := os.Stat(p)
	mtime := info.ModTime()
	// a second converge is a no-op and must not rewrite the file.
	if err := applyPAMFile(p, ModeUnlockOnLogin); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	info2, _ := os.Stat(p)
	if !info2.ModTime().Equal(mtime) {
		t.Fatalf("idempotent apply rewrote the file")
	}
	got, _ := os.ReadFile(p)
	if !pamHasKeyring(string(got), "auth") || !pamHasKeyring(string(got), "session") {
		t.Fatalf("file not wired:\n%s", got)
	}
}

func TestApplyPAMFileMissingAnchorErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sddm")
	os.WriteFile(p, []byte("#%PAM-1.0\naccount include password-auth\n"), 0o644)
	if err := applyPAMFile(p, ModeUnlockOnLogin); err == nil {
		t.Fatal("want an error when the insert anchor is missing")
	}
}

func TestRunApplyPAMHonorsFixtureEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sddm")
	os.WriteFile(p, []byte(bareSDDM), 0o644)
	t.Setenv("RYOKU_PAM_FILE", p)
	if err := runApplyPAM([]string{ModeUnlockOnLogin}); err != nil {
		t.Fatalf("apply-pam: %v", err)
	}
	if err := runApplyPAM([]string{ModeAsk}); err != nil {
		t.Fatalf("apply-pam strip: %v", err)
	}
	got, _ := os.ReadFile(p)
	if pamPresent(string(got)) {
		t.Fatalf("ask should have stripped the lines:\n%s", got)
	}
}

// Stacks Fedora does not ship but users carry (an include instead of the
// substack, stray whitespace and case, a commented-out anchor) still get the
// stock lines right after the real password-auth anchor, round-trip back on a
// strip, and converge only once.
func TestApplyPAMTextStackVariants(t *testing.T) {
	for _, name := range []string{"sddm-include", "sddm-whitespace"} {
		in, want := readTestdata(name), readTestdata(name+".wired")
		out, missing := applyPAMText(in, true)
		if len(missing) != 0 || out != want {
			t.Fatalf("%s: missing %v, got:\n%s", name, missing, out)
		}
		if again, _ := applyPAMText(out, true); again != out {
			t.Fatalf("%s: second converge changed the stack:\n%s", name, again)
		}
		if stripped, _ := applyPAMText(out, false); stripped != in {
			t.Fatalf("%s: strip did not give the input back:\n%q", name, stripped)
		}
	}
}

// A stack with no password-auth line at all must fail loudly and leave the file
// alone, never append the lines somewhere PAM may not reach them.
func TestApplyPAMNoPasswordAuthFailsLoudly(t *testing.T) {
	in := readTestdata("sddm-no-password-auth")
	p := filepath.Join(t.TempDir(), "sddm")
	if err := os.WriteFile(p, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RYOKU_PAM_FILE", p)
	err := runApplyPAM([]string{ModeUnlockOnLogin})
	if err == nil || !strings.Contains(err.Error(), "auth password-auth / session password-auth") {
		t.Fatalf("want an error naming both missing anchors, got %v", err)
	}
	if got, _ := os.ReadFile(p); string(got) != in {
		t.Fatalf("a failed converge must not touch the file:\n%s", got)
	}
}

// The installers (sddm/setup) read "autologin" from `ryoku keyring status
// --json` and run apply-pam never-ask under it, unlock-on-login otherwise. The
// stock stack must stay byte-identical without autologin, and lose exactly its
// keyring lines with it.
func TestInstallerKeyringFlow(t *testing.T) {
	conf := t.TempDir()
	t.Setenv("RYOKU_SDDM_CONF_ROOT", conf)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := filepath.Join(t.TempDir(), "sddm")
	if err := os.WriteFile(p, []byte(fedoraSDDM), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RYOKU_PAM_FILE", p)
	install := func() {
		t.Helper()
		mode := ModeUnlockOnLogin
		if gatherStatus().Autologin {
			mode = ModeNeverAsk
		}
		if err := runApplyPAM([]string{mode}); err != nil {
			t.Fatalf("apply-pam %s: %v", mode, err)
		}
	}

	install()
	if got, _ := os.ReadFile(p); string(got) != fedoraSDDM {
		t.Fatalf("without autologin the stock stack must stay byte-for-byte:\n%s", got)
	}
	writeConf(t, filepath.Join(conf, "sddm.conf.d", "50-autologin.conf"), "[Autologin]\nUser=nero\n")
	install()
	if got, _ := os.ReadFile(p); string(got) != bareSDDM {
		t.Fatalf("autologin must strip exactly the keyring lines:\n%s", got)
	}
}
