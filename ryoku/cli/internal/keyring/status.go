package keyring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	i18n "ryoku-i18n"
)

// KeyringFile is the state of one on-disk keyring, as the Hub renders it.
type KeyringFile struct {
	Name   string `json:"name"`
	Role   string `json:"role"` // "default" or "login"
	Exists bool   `json:"exists"`
	Format string `json:"format"` // encrypted, plaintext, or absent
}

// Status is the full read-only picture: the configured mode and where it came
// from, the system wiring (PAM, autologin, daemon), and the keyring files that
// matter. The Hub decides from this whether a mode switch is blocked.
type Status struct {
	Mode        string        `json:"mode"`
	ModeSource  string        `json:"mode_source"` // "configured" or "inferred"
	PamPresent  bool          `json:"pam_present"`
	Autologin   bool          `json:"autologin"`
	DaemonAlive bool          `json:"daemon_alive"`
	Keyrings    []KeyringFile `json:"keyrings"`
	Notes       []string      `json:"notes"`
}

// autologinConfigured reports whether SDDM will autologin a user: the effective
// [Autologin] User= is non-empty. It is the one autologin detector; the
// installers read it from `ryoku keyring status --json` (the "autologin" field).
//
// It follows SDDM's ConfigReader (0.21): every file in the system directory
// (sysDir), then every file in <root>/sddm.conf.d, each directory sorted by
// name, then <root>/sddm.conf last, and the last value read wins. So a later
// file with an empty User= turns autologin off. Dotfiles are skipped (QDir::Files
// without QDir::Hidden). SDDM sorts LocaleAware; byte order matches it for the
// usual digit- or lowercase-prefixed drop-in names.
func autologinConfigured(sysDir, root string) bool {
	var files []string
	for _, dir := range []string{sysDir, filepath.Join(root, "sddm.conf.d")} {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	files = append(files, filepath.Join(root, "sddm.conf"))
	user := ""
	for _, f := range files {
		if v, ok := autologinUser(f); ok {
			user = v
		}
	}
	return user != ""
}

// autologinUser returns the last [Autologin] User= value a file sets, and
// whether it sets one. It parses the way SDDM's ConfigBase::loadInternal does:
// each line is trimmed and cut at the first '#'; a line holding '=' is an
// assignment (key and value trimmed); otherwise "[Name]" opens a section. Section
// and key names match exactly, case-sensitively, as SDDM looks them up, so
// "[autologin]" or "user=" is ignored by SDDM and here alike.
func autologinUser(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	section, user, set := "General", "", false
	for _, l := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(l)
		if i := strings.IndexByte(t, '#'); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		if k, v, ok := strings.Cut(t, "="); ok {
			if section == "Autologin" && strings.TrimSpace(k) == "User" {
				user, set = strings.TrimSpace(v), true
			}
			continue
		}
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = t[1 : len(t)-1]
		}
	}
	return user, set
}

func sddmConfRoot() string {
	if r := os.Getenv("RYOKU_SDDM_CONF_ROOT"); r != "" {
		return r
	}
	return "/etc"
}

// sddmSysConfDir is SDDM's system drop-in directory. A test that points
// RYOKU_SDDM_CONF_ROOT at a fixture sees only that fixture unless it also sets
// RYOKU_SDDM_SYS_CONF_DIR, so the host's own SDDM config never leaks in.
func sddmSysConfDir() string {
	if d, ok := os.LookupEnv("RYOKU_SDDM_SYS_CONF_DIR"); ok {
		return d
	}
	if os.Getenv("RYOKU_SDDM_CONF_ROOT") != "" {
		return ""
	}
	return "/usr/lib/sddm/sddm.conf.d"
}

func gatherStatus() Status {
	pamContent, _ := os.ReadFile(pamFilePath())
	pam := pamPresent(string(pamContent))

	mode, configured := readConfig()
	source := "configured"
	if !configured {
		source = "inferred"
		// Fedora's stock SDDM stack wires pam_gnome_keyring, so an unconfigured
		// stock box infers unlock-on-login. Only a stack without the lines
		// (stripped by never-ask or ask, or edited) falls back to never-ask (a
		// blank, passwordless keyring) rather than ask, which prompts on first
		// use. `ryoku keyring init` records the inferred mode at first login.
		if pam {
			mode = ModeUnlockOnLogin
		} else {
			mode = ModeNeverAsk
		}
	}

	defName := defaultKeyringName()
	defFmt := probeFormat(keyringFile(defName))
	kfs := []KeyringFile{
		{Name: defName, Role: "default", Exists: defFmt != fmtAbsent, Format: defFmt},
	}
	// login.keyring is the PAM-managed store; list it too unless it is already
	// the default (no need to name the same file twice).
	if defName != "login" {
		lf := probeFormat(keyringFile("login"))
		kfs = append(kfs, KeyringFile{Name: "login", Role: "login", Exists: lf != fmtAbsent, Format: lf})
	}

	st := Status{
		Mode:        mode,
		ModeSource:  source,
		PamPresent:  pam,
		Autologin:   autologinConfigured(sddmSysConfDir(), sddmConfRoot()),
		DaemonAlive: daemonAlive(),
		Keyrings:    kfs,
	}
	st.Notes = statusNotes(st)
	return st
}

// statusNotes spells out the caveats a bare state cannot: whether an encrypted
// keyring will actually unlock, and the autologin conflict.
func statusNotes(st Status) []string {
	var notes []string
	if st.Mode == ModeUnlockOnLogin {
		for _, k := range st.Keyrings {
			if k.Role == "default" && k.Format == fmtEncrypted {
				notes = append(notes, fmt.Sprintf(i18n.T("the %q keyring is password-protected; it will unlock silently at next login only if its password is your login password"), k.Name))
			}
		}
		if st.Autologin {
			notes = append(notes, i18n.T("autologin is configured; unlock-on-login has no login password to reuse, so it cannot unlock silently -- use never-ask under autologin"))
		}
	}
	if st.Mode == ModeNeverAsk {
		for _, k := range st.Keyrings {
			if k.Role == "default" && k.Format == fmtEncrypted {
				notes = append(notes, fmt.Sprintf(i18n.T("the %q keyring is still password-protected; never-ask needs it blank -- convert it (with your current password) or start fresh"), k.Name))
			}
		}
	}
	return notes
}

func runStatus(args []string) error {
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		} else {
			return fmt.Errorf(i18n.T("usage: ryoku keyring status [--json]"))
		}
	}
	st := gatherStatus()
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}
	printStatus(st)
	return nil
}

func printStatus(st Status) {
	fmt.Printf(i18n.T("mode:      %s (%s)\n"), st.Mode, st.ModeSource)
	fmt.Printf(i18n.T("pam lines: %s\n"), yesno(st.PamPresent))
	fmt.Printf(i18n.T("autologin: %s\n"), yesno(st.Autologin))
	fmt.Printf(i18n.T("daemon:    %s\n"), map[bool]string{true: i18n.T("running"), false: i18n.T("not running")}[st.DaemonAlive])
	for _, k := range st.Keyrings {
		state := k.Format
		if !k.Exists {
			state = "absent"
		}
		fmt.Printf(i18n.T("keyring %-8s (%s): %s\n"), k.Name, k.Role, state)
	}
	for _, n := range st.Notes {
		fmt.Printf(i18n.T("note: %s\n"), n)
	}
}

func yesno(b bool) string {
	if b {
		return i18n.T("yes")
	}
	return i18n.T("no")
}
