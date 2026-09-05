package main

// engine.go runs the install: ordered steps, each an idempotent shell-out
// sequence streamed line by line to the UI and the log file. The recipe is
// installation/backend/lib/deploy.sh translated from chroot to live system,
// plus the migration work (backup, rival shells, DM/network switch) that an
// existing machine needs and a blank ISO target never did.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// line-anchored so a commented-out "#[ryoku]" stanza does not count.
var ryokuStanzaRe = regexp.MustCompile(`(?m)^\[ryoku\]`)

const repoURL = "https://github.com/neur0map/ryoku-arch.git"

const pacmanStanza = `
[ryoku]
SigLevel = Required
Server = https://repo.ryoku.dev/stable/$arch
`

// ryokuPkgs mirrors the ISO's deploy.sh: the keyring plus the ryoku-desktop
// umbrella. The umbrella version-pins and pulls every monorepo component and
// the desktop's runtime tools (recorder, night light, dictation, OCR/QR, LED,
// external-monitor brightness, ...) as hard depends, so installing just these
// two -- exactly what the ISO installs -- is the single source of truth for the
// Ryoku desktop set. Everything else (session, base OS, fonts) comes from
// base.packages via readBasePackages: the same manifest the ISO pacstraps.
var ryokuPkgs = []string{"ryoku-keyring", "ryoku-desktop"}

// ryokuOverwriteGlob names the ryoku-desktop-owned paths that a prior partial
// install, a dev deploy, or the ISO installer can leave unowned on disk: the
// privileged helpers, their polkit rules, and the Plymouth splash theme. The
// desktop transaction --overwrites them so a resume or a conversion never aborts
// on "exists in filesystem". Mirrors the ISO installer and `ryoku update`
// (updater.ryokuOverwriteGlob); keep the three in sync.
const ryokuOverwriteGlob = "/usr/bin/ryoku-*," +
	"/usr/share/polkit-1/rules.d/*ryoku*.rules," +
	"/usr/share/plymouth/themes/ryoku/*"

// bootChainSkip: base.packages entries a shell-converted box must NOT get -- it
// already owns its bootloader, initramfs, encryption and snapshot stack, and
// (re)installing these would rewrite its boot. Everything else in base.packages
// installs, so a shell box matches the ISO's set with no second hand-kept list
// to drift out of sync.
var bootChainSkip = map[string]bool{
	"base": true, "base-devel": true, "linux": true, "linux-firmware": true,
	"mkinitcpio": true, "sudo": true, "btrfs-progs": true, "cryptsetup": true,
	"dosfstools": true, "efibootmgr": true, "limine": true, "plymouth": true,
	"snapper": true, "snap-pac": true,
	// the limine hook packages pull limine + mkinitcpio back as depends and ship
	// /etc/pacman.d/hooks entries that collide with a host keeping its own boot
	// stack (Garuda's garuda-hooks owns 90-mkinitcpio-install.hook, #58).
	"limine-mkinitcpio-hook": true, "limine-snapper-sync": true,
}

// the standard Ryoku extras, all best-effort here. awww (the wallpaper daemon,
// from the [ryoku] repo) and matugen (the palette generator, from the official
// repo) are hard ryoku-desktop depends the packages step already pulled; no AUR build is needed.
var aurPkgs = []string{"bibata-cursor-theme-bin", "localsend-bin", "voxtype-bin"}

// system/packages/dev.packages; ryoku recovery builds from source and needs go.
var devPkgs = []string{"go", "nodejs", "npm", "python", "python-pip", "python-pipx", "mise"}

var sparsePaths = []string{
	"ryoku/lockscreen", "ryoku/assets", "ryoku/apps",
	"system/hardware/drivers", "system/hardware/input",
	"system/packages", "release/packages/ryoku-keyring",
}

type plan struct {
	nvidia    bool // proprietary NVIDIA driver setup
	switchDM  bool // disable current DM, enable SDDM
	switchNet bool // disable other network stacks, enable NetworkManager
	rivals    bool // remove rival shell packages
	softOff   bool // disable conflicting user daemons
	aur       bool // AUR extras
	fish      bool // fish as login shell
	devtools  bool // dev.packages toolchains (go/rust/node/python; recovery needs go)
	omarchy   bool // retire the [omarchy] repo and mirror pin
	monPins   bool // pin the salvaged monitor layout in monitors_user.lua
	greeter   bool // point SDDM at the Ryoku greeter theme
	resume    bool // skip steps a previous interrupted run already finished
	azertyFR  bool // force the French AZERTY layout (fr) on desktop, console, greeter
	azertyBE  bool // force the Belgian AZERTY layout (be) on desktop, console, greeter
}

func defaultPlan(f *facts) *plan {
	return &plan{
		// secure boot rejects unsigned dkms modules and the nvidia script also
		// blacklists nouveau: proceeding would boot into a black screen. only
		// an sbctl-managed box gets to keep the default.
		nvidia:    f.hasNvidia && !f.nouveauLive && !(f.secureBoot && !f.sbctlSigned),
		switchDM:  true,
		switchNet: true,
		rivals:    true,
		softOff:   true,
		aur:       true,
		fish:      !strings.HasSuffix(f.userShell, "/fish"),
		omarchy:   f.omarchyRepo || f.omarchyMirror || len(f.omarchyGuards) > 0,
		monPins:   len(f.monOutputs) > 0,
		// when KDE's sddm-kcm owns sddm.conf.d the user chose that greeter
		// look; keep it unless they opt in.
		greeter: !f.kdeSddmConf,
		resume:  f.prevRun != nil,
		// the AZERTY overrides are opt-in only; a salvaged layout already
		// covers anyone who had one configured.
	}
}

// azertyExclusive keeps the two AZERTY toggles mutually exclusive: switching
// one on switches the other off. just is the toggle that was just flipped.
func (p *plan) azertyExclusive(just *bool) {
	if !*just {
		return
	}
	switch just {
	case &p.azertyFR:
		p.azertyBE = false
	case &p.azertyBE:
		p.azertyFR = false
	}
}

type evStep struct {
	idx   int
	title string
}
type evLine struct {
	line string
	// a live progress repaint (\r-terminated): the UI replaces the previous
	// transient line instead of appending, and the log file skips it
	transient bool
}
type evDone struct {
	err error
	idx int
}

type estep struct {
	id    string
	title string
	fn    func(*engine) error
}

type engine struct {
	f   *facts
	p   *plan
	dry bool
	ref string

	events  chan any
	logf    *os.File
	logPath string
	logMu   sync.Mutex

	payloadOverride string
	payload         string
	backupDir       string
	restorePath     string
	prevBackups     int
	pendingRestore  []string // undo lines queued before restore.sh exists
	state           *runState

	steps []estep
}

func newEngine(f *facts, p *plan, dry bool, ref, payloadOverride string) *engine {
	e := &engine{f: f, p: p, dry: dry, ref: ref, payloadOverride: payloadOverride}
	if payloadOverride != "" {
		if abs, err := filepath.Abs(payloadOverride); err == nil {
			e.payload = abs
			e.payloadOverride = abs
		} else {
			e.payload = payloadOverride
		}
	} else {
		cache := os.Getenv("XDG_CACHE_HOME")
		if cache == "" && f != nil && f.homeDir != "" {
			cache = filepath.Join(f.homeDir, ".cache")
		} else if cache == "" {
			cache = filepath.Join(os.Getenv("HOME"), ".cache")
		}
		e.payload = filepath.Join(cache, "ryoku-shell-install/repo")
	}
	e.openLog()
	// resuming continues the previous run's backup dir so restore.sh stays
	// one script; declining starts a fresh state (the file is rewritten at
	// the first completed step).
	if p.resume && f.prevRun != nil {
		e.state = f.prevRun
		if f.prevRun.BackupDir != "" {
			if fi, err := os.Stat(f.prevRun.BackupDir); err == nil && fi.IsDir() {
				e.backupDir = f.prevRun.BackupDir
			}
		}
	}
	// repo trust comes before conflict removal on purpose: nothing gets
	// uninstalled until the [ryoku] db has actually been fetched. legacy
	// sources go first so the full upgrade already runs on clean mirrors.
	pacmanOnly := map[string]bool{"legacy": true, "repo": true, "aur": true, "drivers": true}
	all := []estep{
		{"legacy", "Retiring the previous distro's package sources", stepLegacy},
		{"sysupgrade", "Updating the system", stepSysupgrade},
		{"tools", "Installing installer tools (git, build tools)", stepTools},
		{"payload", "Fetching the Ryoku payload", stepPayload},
		{"backup", "Backing up your configs", stepBackup},
		{"repo", "Trusting the [ryoku] package repository", stepRepo},
		{"conflicts", "Clearing conflicting shells and daemons", stepConflicts},
		{"packages", "Installing the Ryoku desktop", stepPackages},
		{"drivers", "Setting up GPU drivers", stepDrivers},
		{"build", "Building the Ryoku desktop from source", stepBuild},
		{"session", "Wiring the login session (SDDM, network)", stepSession},
		{"configs", "Laying down your Ryoku configs", stepConfigs},
		{"aur", "Building the AUR extras", stepAUR},
		{"shell", "Switching your login shell to fish", stepFish},
		{"doctor", "Converging the system (ryoku doctor)", stepDoctor},
		{"verify", "Verifying the install", stepVerify},
	}
	src := f.distro != nil && f.distro.fromSource
	for _, s := range all {
		if src && pacmanOnly[s.id] {
			if s.id == "repo" && f.distro != nil && f.distro.id == "fedora" {
				// Fedora runs stepRepo to configure COPR repositories (quickshell, matugen, awww, cursors)
			} else {
				continue
			}
		}
		if !src && s.id == "build" {
			continue
		}
		e.steps = append(e.steps, s)
	}
	return e
}

func (e *engine) openLog() {
	// dry runs must not touch the filesystem beyond a throwaway log.
	dir := os.TempDir()
	if !e.dry {
		dir = filepath.Join(e.f.homeDir, ".local/state/ryoku")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			dir = os.TempDir()
		}
	}
	e.logPath = filepath.Join(dir, "shell-install.log")
	e.logf, _ = os.OpenFile(e.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	e.log(fmt.Sprintf("---- ryoku-shell-install run %s (dry=%v ref=%s) ----", time.Now().Format(time.RFC3339), e.dry, e.ref))
}

func (e *engine) log(s string) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	if e.logf != nil {
		fmt.Fprintln(e.logf, s)
	}
}

func (e *engine) say(s string) {
	e.log(s)
	if e.events != nil {
		e.events <- evLine{line: s}
	}
}

func (e *engine) sayf(format string, a ...any) { e.say(fmt.Sprintf(format, a...)) }

func (e *engine) sayTransient(s string) {
	if e.events != nil {
		e.events <- evLine{line: s, transient: true}
	}
}

// runFrom executes steps starting at idx (retry re-enters at the failed one).
func (e *engine) runFrom(idx int) chan any {
	e.events = make(chan any, 256)
	go func() {
		for i := idx; i < len(e.steps); i++ {
			s := e.steps[i]
			e.events <- evStep{idx: i, title: s.title}
			e.log("==== step " + s.id + " ====")
			if e.p.resume && e.state != nil && e.state.has(s.id) {
				e.say("finished in the previous run, resuming past it")
				continue
			}
			if err := s.fn(e); err != nil {
				e.sayf("step %s failed: %v", s.id, err)
				e.events <- evDone{err: err, idx: i}
				return
			}
			e.markStepDone(s.id)
		}
		e.clearState()
		e.events <- evDone{idx: len(e.steps)}
	}()
	return e.events
}

// cleanTermLine reduces raw child output to what a terminal would leave
// visible: text after the last \r, tabs spaced, control bytes dropped. A bare
// \r (curl's progress meter) rendered inside the TUI panel jumps the cursor
// to column 0 and tears the frame.
func cleanTermLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("  ")
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func shellJoin(name string, args []string) string {
	parts := []string{name}
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"'$") {
			a = fmt.Sprintf("%q", a)
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// cmd runs a command streaming merged stdout/stderr into the UI and the log.
func (e *engine) cmd(dir string, env []string, name string, args ...string) error {
	line := shellJoin(name, args)
	if e.dry {
		e.say("DRYRUN: " + line)
		return nil
	}
	e.say("$ " + line)
	c := exec.Command(name, args...)
	c.Dir = dir
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	pr, pw := io.Pipe()
	c.Stdout, c.Stderr = pw, pw
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// byte loop, not a Scanner: a Scanner stops at its buffer cap and the
		// write side of the pipe then blocks forever. \r ends a token too, so
		// pacman's download progress surfaces live instead of buffering until
		// each file's final newline; a panel that ticks during long downloads
		// is the difference between "working" and "waiting for my password".
		rd := bufio.NewReader(pr)
		var buf []byte
		var lastProgress time.Time
		for {
			b, err := rd.ReadByte()
			if err != nil {
				if ln := cleanTermLine(string(buf)); ln != "" {
					e.say("  " + ln)
				}
				return
			}
			switch b {
			case '\n':
				if ln := cleanTermLine(string(buf)); ln != "" {
					e.say("  " + ln)
				}
				buf = buf[:0]
			case '\r':
				if nxt, perr := rd.Peek(1); perr == nil && nxt[0] == '\n' {
					continue // \r\n: the \n case finishes the line
				}
				// a progress repaint: replaced in place in the UI, rate-capped,
				// never written to the log file
				if ln := cleanTermLine(string(buf)); ln != "" && time.Since(lastProgress) > 80*time.Millisecond {
					lastProgress = time.Now()
					e.sayTransient("  " + ln)
				}
				buf = buf[:0]
			default:
				buf = append(buf, b)
			}
		}
	}()
	err := c.Run()
	pw.Close()
	wg.Wait()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// sudoArgv: what `sudo -n` gets. Omarchy's guard hook aborts a pacman -Syu it did
// not start, and both updateCmd and installCmd are -Syu, so until stepLegacy
// retires the hook every package step needs the escape the guard documents.
func sudoArgv(args []string, guarded bool) []string {
	if !guarded || len(args) == 0 || args[0] != "pacman" {
		return args
	}
	return append([]string{"env", "OMARCHY_ALLOW_DIRECT_PACMAN=1"}, args...)
}

func (e *engine) sudo(args ...string) error {
	if len(args) == 0 {
		return nil
	}
	guarded := e.f != nil && len(e.f.omarchyGuards) > 0
	return e.cmd("", nil, "sudo", append([]string{"-n"}, sudoArgv(args, guarded)...)...)
}

// sudoSh runs a fixed shell snippet as root; only static strings go in here.
func (e *engine) sudoSh(script string) error {
	return e.cmd("", nil, "sudo", "-n", "sh", "-c", script)
}

// sudoWrite replaces a root-owned file with the given content via tee.
func (e *engine) sudoWrite(path, content string) error {
	if e.dry {
		e.say("DRYRUN: write " + path)
		return nil
	}
	e.say("$ sudo tee " + path)
	c := exec.Command("sudo", "-n", "tee", path)
	c.Stdin = strings.NewReader(content)
	c.Stdout, c.Stderr = io.Discard, io.Discard
	if err := c.Run(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// stripPacmanSection drops one [section] and its body from pacman.conf text.
func stripPacmanSection(conf, section string) string {
	var out []string
	skip := false
	for _, ln := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			skip = t == "["+section+"]"
		}
		if !skip {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// ---- steps ----

// an ex-Omarchy machine still trusts the [omarchy] repo and routes core/extra
// through Omarchy's own mirror; a Ryoku box must depend on neither. originals
// are kept as *.pre-ryoku and the undo lands in restore.sh.
func stepLegacy(e *engine) error {
	if !e.p.omarchy || (!e.f.omarchyRepo && !e.f.omarchyMirror && len(e.f.omarchyGuards) == 0) {
		e.say("no previous distro package sources to retire")
		return nil
	}
	if e.f.omarchyRepo {
		conf, err := os.ReadFile("/etc/pacman.conf")
		if err != nil {
			return err
		}
		stripped := stripPacmanSection(string(conf), "omarchy")
		if stripped != string(conf) {
			if err := e.sudo("cp", "/etc/pacman.conf", "/etc/pacman.conf.pre-ryoku"); err != nil {
				return err
			}
			if err := e.sudoWrite("/etc/pacman.conf", stripped); err != nil {
				return err
			}
			e.say("dropped the [omarchy] repository (original at /etc/pacman.conf.pre-ryoku)")
			e.pendingRestore = append(e.pendingRestore,
				"sudo cp /etc/pacman.conf.pre-ryoku /etc/pacman.conf")
		}
	}
	if e.f.omarchyMirror {
		if err := e.sudo("cp", "/etc/pacman.d/mirrorlist", "/etc/pacman.d/mirrorlist.pre-ryoku"); err != nil {
			return err
		}
		ml := "# restored by ryoku-shell-install: the previous Omarchy install pinned its\n" +
			"# own package mirror here. original at mirrorlist.pre-ryoku; rank your own\n" +
			"# mirrors with reflector if you want more than the worldwide CDN.\n" +
			"Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch\n"
		if err := e.sudoWrite("/etc/pacman.d/mirrorlist", ml); err != nil {
			return err
		}
		e.say("restored a standard Arch mirrorlist (original at /etc/pacman.d/mirrorlist.pre-ryoku)")
		e.pendingRestore = append(e.pendingRestore,
			"sudo cp /etc/pacman.d/mirrorlist.pre-ryoku /etc/pacman.d/mirrorlist")
	}
	for _, hook := range e.f.omarchyGuards {
		aside := hook + ".pre-ryoku"
		if err := e.sudo("mv", hook, aside); err != nil {
			return err
		}
		e.say("retired Omarchy's pacman guard hook, which aborts every -Syu (moved to " + aside + ")")
		e.pendingRestore = append(e.pendingRestore, "sudo mv "+aside+" "+hook)
	}
	if !e.dry && pacmanHas("omarchy-keyring") {
		if err := e.sudo("pacman", "-R", "--noconfirm", "omarchy-keyring"); err != nil {
			e.say("warning: could not remove omarchy-keyring (continuing)")
		}
	}
	return nil
}

func stepSysupgrade(e *engine) error {
	d := e.d()
	if d.id != "arch" {
		if err := e.sudo(d.refreshCmd...); err != nil {
			return err
		}
	}
	return e.sudo(d.updateCmd...)
}

func stepTools(e *engine) error {
	d := e.d()
	pkgs := []string{"git", d.local("base-devel")}
	if d.id == "fedora" {
		pkgs = append(pkgs, "dnf-plugins-core")
	}
	return e.sudo(d.installArgs(pkgs)...)
}

func stepPayload(e *engine) error {
	if e.payloadOverride != "" {
		e.payload = e.payloadOverride
		if _, err := os.Stat(filepath.Join(e.payload, "ryoku/lockscreen/install-qylock")); err != nil && !e.dry {
			return fmt.Errorf("payload override %s does not look like a ryoku-arch checkout", e.payload)
		}
		e.say("using payload checkout " + e.payload)
		return nil
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(e.f.homeDir, ".cache")
	}
	e.payload = filepath.Join(cache, "ryoku-shell-install/repo")

	if _, err := os.Stat(filepath.Join(e.payload, ".git")); err == nil {
		if err := e.cmd(e.payload, nil, "git", "fetch", "--depth=1", "origin", e.ref); err != nil {
			return err
		}
		if err := e.cmd(e.payload, nil, "git", "checkout", "-f", "FETCH_HEAD"); err != nil {
			return err
		}
	} else {
		if !e.dry {
			if err := os.MkdirAll(filepath.Dir(e.payload), 0o755); err != nil {
				return err
			}
		}
		if err := e.cmd("", nil, "git", "clone", "--depth=1", "--filter=blob:none", "--sparse",
			"--branch", e.ref, repoURL, e.payload); err != nil {
			return err
		}
	}
	paths := sparsePaths
	if e.d().fromSource {
		paths = []string{
			"ryoku", "system", "release/packages/ryoku-keyring",
		}
	}
	if err := e.cmd(e.payload, nil, "git", append([]string{"sparse-checkout", "set"}, paths...)...); err != nil {
		return err
	}
	// a cache from an older installer can come out of the update missing paths
	// the current engine needs; a broken cache is worth less than a fresh clone.
	if _, err := os.Stat(filepath.Join(e.payload, "system/packages/base.packages")); err != nil && !e.dry {
		e.say("payload cache is incomplete; recloning it fresh")
		if err := os.RemoveAll(e.payload); err != nil {
			return err
		}
		if err := e.cmd("", nil, "git", "clone", "--depth=1", "--filter=blob:none", "--sparse",
			"--branch", e.ref, repoURL, e.payload); err != nil {
			return err
		}
		return e.cmd(e.payload, nil, "git", append([]string{"sparse-checkout", "set"}, paths...)...)
	}
	return nil
}

// paths under $HOME that materialize or the seeds will touch. hypr, quickshell
// and the lockscreen dirs are moved aside wholesale: a stale hyprland.conf or a
// foreign quickshell tree next to the Ryoku one is exactly the breakage a
// migration must avoid. the rest are copied, then clobbered in place.
// xdg-desktop-portal moves aside too: a user-level portals.conf outranks the
// packaged hyprland one and breaks screenshare under the new session.
var backupMove = []string{
	".config/hypr", ".config/quickshell", ".config/xdg-desktop-portal",
	".local/share/quickshell-lockscreen", ".local/share/qylock",
}
var backupCopy = []string{
	".config/niri", ".config/sway", ".config/kitty", ".config/fish", ".config/nvim",
	".config/fastfetch", ".config/yazi", ".config/matugen", ".config/qt6ct",
	".config/starship.toml", ".config/mimeapps.list",
	".config/systemd/user", // raw symlink tree, restore.sh puts wants wiring back as-was
}

func stepBackup(e *engine) error {
	root := filepath.Join(e.f.homeDir, ".local/state/ryoku/shell-install")
	if prev, _ := filepath.Glob(filepath.Join(root, "backup-*")); len(prev) > 0 {
		e.prevBackups = len(prev)
		e.say(fmt.Sprintf("note: %d earlier backup(s) exist under %s; the oldest holds your pre-Ryoku configs", len(prev), root))
	}
	if e.dry {
		e.say("DRYRUN: back up " + strings.Join(append(append([]string{}, backupMove...), backupCopy...), ", "))
		return nil
	}
	// a retry in the same run reuses the dir: items already saved are skipped
	// instead of being re-captured into a second, half-empty backup.
	if e.backupDir == "" {
		e.backupDir = filepath.Join(root, "backup-"+time.Now().Format("20060102-150405"))
	}
	e.restorePath = filepath.Join(e.backupDir, "restore.sh")
	if err := os.MkdirAll(e.backupDir, 0o755); err != nil {
		return err
	}
	// restore.sh exists before the first item moves, and grows a line per
	// saved item, so a kill at any point leaves a script that undoes exactly
	// what happened so far.
	if _, err := os.Stat(e.restorePath); err != nil {
		hdr := "#!/usr/bin/env bash\n" +
			"# restore configs saved by ryoku-shell-install. undo lines are appended as\n" +
			"# the installer changes things; run the whole script to roll back.\n" +
			"set -euo pipefail\nDIR=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n"
		if err := os.WriteFile(e.restorePath, []byte(hdr), 0o755); err != nil {
			return err
		}
	}
	// undo lines queued by steps that ran before this file existed (legacy
	// package-source retirement runs first).
	for _, ln := range e.pendingRestore {
		e.recordRestore(ln)
	}
	e.pendingRestore = nil
	saveOne := func(rel string, move bool) error {
		src := filepath.Join(e.f.homeDir, rel)
		dst := filepath.Join(e.backupDir, rel)
		if _, err := os.Lstat(dst); err == nil {
			return nil // already saved by a previous attempt of this run
		}
		if _, err := os.Lstat(src); err != nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if move {
			if err := os.Rename(src, dst); err != nil {
				// cross-device home layouts (bind mounts, separate subvolume
				// mounts): fall back to copy + delete.
				if err := copyTree(src, dst); err != nil {
					return err
				}
				if err := os.RemoveAll(src); err != nil {
					return err
				}
			}
			e.say("moved aside " + rel)
		} else {
			if err := copyTree(src, dst); err != nil {
				return err
			}
			e.say("backed up " + rel)
		}
		e.recordRestore(fmt.Sprintf("rm -rf %q && mkdir -p %q && cp -a %q %q",
			"$HOME/"+rel, filepath.Dir("$HOME/"+rel), "$DIR/"+rel, "$HOME/"+rel))
		return nil
	}
	for _, rel := range backupMove {
		if err := saveOne(rel, true); err != nil {
			return err
		}
	}
	// moving the hypr/quickshell trees is what disarms a rice's exec-once
	// autostart chain; name it so the log explains the disappearing bar.
	for _, r := range e.f.riceFound {
		e.say("rice " + r + ": its autostart lives in the trees moved above; extra configs (waybar, swaync, ...) stay untouched in place")
	}
	for _, rel := range backupCopy {
		if err := saveOne(rel, false); err != nil {
			return err
		}
	}
	e.say("backup at " + e.backupDir + " (restore.sh inside)")
	return nil
}

// recordRestore appends an undo line to restore.sh.
func (e *engine) recordRestore(line string) {
	if e.dry || e.restorePath == "" {
		return
	}
	fh, err := os.OpenFile(e.restorePath, os.O_APPEND|os.O_WRONLY, 0o755)
	if err != nil {
		return
	}
	defer fh.Close()
	fmt.Fprintln(fh, line)
}

func stepConflicts(e *engine) error {
	// units first, while their unit files still exist. disable only, never
	// stop: running daemons die with the old session.
	if e.p.softOff {
		for _, u := range e.f.softUnits {
			if err := e.cmd("", nil, "systemctl", "--user", "disable", u); err != nil {
				e.say("warning: could not disable " + u)
				continue
			}
			// || true: add-wants units have no [Install] and refuse a bare enable
			e.recordRestore("systemctl --user enable " + u + " || true")
		}
	}
	if e.p.rivals && len(e.f.rivalPkgs) > 0 {
		// plain -R, no -ns cascade: -Rns on a meta like cachyos-niri-noctalia
		// would drag niri itself out, and the plan promises niri survives as a
		// fallback session. leftover deps become orphans doctor can report.
		e.say("removing rival shell packages: " + strings.Join(e.f.rivalPkgs, " "))
		if err := e.sudo(e.d().removeArgs(e.f.rivalPkgs)...); err != nil {
			e.say("bulk removal failed, retrying one by one")
			for _, p := range e.f.rivalPkgs {
				if err := e.sudo(e.d().removeArgs([]string{p})...); err != nil {
					e.say("warning: could not remove " + p + " (continuing)")
				}
			}
		}
	}
	if len(e.f.blockerPkgs) > 0 {
		// pacman --noconfirm answers conflict prompts with No and aborts, so
		// packages that conflict with the desktop set (pulseaudio vs
		// pipewire-pulse, quickshell-git vs quickshell) must go first.
		e.say("removing packages that block the desktop install: " + strings.Join(e.f.blockerPkgs, " "))
		if err := e.sudo(e.d().removeArgs(e.f.blockerPkgs)...); err != nil {
			for _, p := range e.f.blockerPkgs {
				if err := e.sudo(e.d().removeArgs([]string{p})...); err != nil {
					e.say("warning: could not remove " + p + "; the package step may abort on a conflict")
				}
			}
		}
	}
	return nil
}

func stepRepo(e *engine) error {
	if e.d().id == "fedora" {
		coprs := []struct {
			name string
			desc string
		}{
			{"sdegler/hyprland", "Hyprland compositor stack"},
			{"errornointernet/quickshell", "Quickshell desktop engine"},
			{"scottames/awww", "awww wallpaper daemon"},
			{"atim/starship", "Starship prompt"},
			{"atim/lazygit", "lazygit"},
			{"lihaohong/yazi", "yazi"},
			{"tofik/nwg-shell", "nwg-shell tools"},
			{"erikreider/SwayNotificationCenter", "SwayNotificationCenter"},
			{"alternateved/eza", "eza"},
			{"opuk/bottom", "bottom"},
			{"atim/lazydocker", "lazydocker"},
			{"wezfurlong/wezterm-nightly", "wezterm-nightly"},
			{"scottames/ghostty", "ghostty"},
			{"errornointernet/packages", "wallust palette generator"},
		}
		for _, c := range coprs {
			if e.dry {
				e.say("DRYRUN: dnf copr enable -y " + c.name)
				continue
			}
			e.say("enabling " + c.desc + " COPR (" + c.name + ")")
			if err := e.sudo("dnf", "-y", "copr", "enable", c.name); err != nil {
				e.say("warning: could not enable " + c.name + " COPR (continuing): " + err.Error())
			} else {
				e.say("enabled " + c.desc + " COPR (" + c.name + ")")
			}
		}
		return nil
	}

	// on a box that already has ryoku-keyring, the keyring files under
	// /usr/share/pacman/keyrings are package-owned: seeding and deleting them
	// again would strip files out of the installed package. the trustdb is
	// already populated, so the whole dance is unnecessary.
	if !e.dry && pacmanHas("ryoku-keyring") {
		e.say("ryoku-keyring already installed, key trust in place")
	} else {
		kdir := filepath.Join(e.payload, "release/packages/ryoku-keyring")
		kd := "/usr/share/pacman/keyrings"
		for _, f := range []string{"ryoku.gpg", "ryoku-trusted", "ryoku-revoked"} {
			if err := e.sudo("install", "-Dm644", filepath.Join(kdir, f), filepath.Join(kd, f)); err != nil {
				return err
			}
		}
		if err := e.sudo("pacman-key", "--populate", "ryoku"); err != nil {
			return err
		}
		// drop the seeds so the ryoku-keyring package installs without a file
		// conflict; the trustdb keeps the key (same dance as deploy.sh).
		if err := e.sudo("rm", "-f", kd+"/ryoku.gpg", kd+"/ryoku-trusted", kd+"/ryoku-revoked"); err != nil {
			return err
		}
	}

	conf, err := os.ReadFile("/etc/pacman.conf")
	if err != nil {
		return err
	}
	if !ryokuStanzaRe.Match(conf) {
		// whole-file swap, not `>>`: a crash mid-append could leave a truncated
		// stanza that pacman rejects while a resume's regex check still sees
		// `[ryoku]` and skips the repair. mv on the same fs commits atomically.
		// chmod pins 0644: the new inode's mode would otherwise follow the
		// invoking user's umask (sudo propagates it), and a umask-077 box would
		// flip pacman.conf to 0600, breaking every non-root pacman reader,
		// including this installer's own resume read above.
		if err := e.sudoSh(`{ cat /etc/pacman.conf && printf '%s' '` + pacmanStanza + `'; } > /etc/pacman.conf.ryoku-new && ` +
			`chmod 644 /etc/pacman.conf.ryoku-new && ` +
			`mv -f /etc/pacman.conf.ryoku-new /etc/pacman.conf`); err != nil {
			return err
		}
		e.say("added the [ryoku] repository to /etc/pacman.conf")
	} else {
		e.say("[ryoku] repository already present in /etc/pacman.conf")
	}
	// refresh right after the -Syu step, so this cannot strand a partial
	// upgrade; it only pulls the fresh [ryoku] db.
	return e.sudo("pacman", "-Sy")
}

// readBasePackages parses system/packages/base.packages from the payload (the
// same manifest the ISO pacstraps): one package per line, '#' comments and
// blank lines dropped, boot-chain entries skipped. This is the single source of
// truth for the machine package set, shared verbatim with the ISO.
func (e *engine) readBasePackages() ([]string, error) {
	path := filepath.Join(e.payload, "system/packages/base.packages")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var pkgs []string
	for _, ln := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(ln, '#'); i >= 0 {
			ln = ln[:i]
		}
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if bootChainSkip[ln] {
			// btrfs root: allow snapper (and snap-pac on Arch) through so
			// ryoku doctor can configure snapshots and rollback as promised.
			if e.f != nil && e.f.btrfsRoot && (ln == "snapper" || ln == "snap-pac") {
				// keep it
			} else {
				continue
			}
		}
		pkgs = append(pkgs, ln)
	}
	return pkgs, nil
}

func (e *engine) asusAura() bool {
	detector := filepath.Join(e.payload, "system/hardware/input/ryoku-hw-asus-aura")
	return exec.Command(detector).Run() == nil
}

func stepPackages(e *engine) error {
	d := e.d()
	base, err := e.readBasePackages()
	if err != nil {
		if !e.dry {
			return err
		}
		e.say("DRYRUN: payload not cloned; would read system/packages/base.packages")
	}
	var pkgs []string
	if d.fromSource {
		// no [ryoku] repository here: the desktop is built from the payload, so
		// install its dependencies plus the toolchain that builds it.
		pkgs = append(d.localAll(base), d.build...)
	} else {
		pkgs = append(append([]string{}, ryokuPkgs...), base...)
		if e.asusAura() {
			if d.installedPkg("tlp") {
				e.say("ASUS Aura lighting skipped because TLP is installed")
			} else {
				pkgs = append(pkgs, "asusctl")
			}
		}
	}
	if e.f.ucodePkg != "" {
		pkgs = append(pkgs, d.local(e.f.ucodePkg))
	}
	if e.p.devtools {
		pkgs = append(pkgs, d.localAll(devPkgs)...)
	}
	pkgs = e.dropSatisfied(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	if d.id == "arch" {
		// a .part resumed against a mirror whose bytes moved on trips pacman's
		// size cap on every retry; dropping resume state just costs a re-download.
		if err := e.sudoSh(`rm -f /var/cache/pacman/pkg/*.part`); err != nil {
			e.say("warning: could not clear partial downloads (continuing)")
		}
	}
	// -Syu, not -S: a resumed run holds the db its first attempt synced, and
	// a publish in between replaces or prunes the files that db points at.
	return e.sudo(desktopPacmanArgs(d, pkgs)...)
}

// desktopPacmanArgs builds the package transaction for stepPackages. On Arch it
// --overwrites the ryoku-desktop-owned paths a prior partial install, a dev
// deploy, or the ISO installer can leave unowned (ryokuOverwriteGlob), so a
// resume or a conversion adopts them instead of aborting the whole transaction on
// "exists in filesystem". fromSource distros build from the payload and never take
// this path, so they keep the plain install command.
func desktopPacmanArgs(d *distro, pkgs []string) []string {
	args := append([]string{}, d.installCmd...)
	if d.id == "arch" {
		args = append(args, "--overwrite", ryokuOverwriteGlob)
	}
	return append(args, pkgs...)
}

// dropSatisfied keeps only what no installed provider satisfies (pacman -T
// prints the unmet names), so a provides-conflict like qt6ct-kde cannot
// abort the install under --noconfirm.
func (e *engine) dropSatisfied(pkgs []string) []string {
	if e.dry || len(pkgs) == 0 {
		return pkgs
	}
	if e.d().id == "fedora" {
		var keep []string
		for _, p := range pkgs {
			if p == "" {
				continue
			}
			if e.d().installedPkg(p) {
				continue
			}
			if p == "power-profiles-daemon" && (e.d().installedPkg("tuned-ppd") || rpmProvides("ppd-service")) {
				continue
			}
			if (p == "ffmpeg" || p == "ffmpeg-free") && (e.d().installedPkg("ffmpeg-free") || pathExists("/usr/bin/ffmpeg")) {
				continue
			}
			if rpmProvides(p) {
				continue
			}
			keep = append(keep, p)
		}
		if len(keep) < len(pkgs) {
			e.say(fmt.Sprintf("%d of %d packages already satisfied on Fedora", len(pkgs)-len(keep), len(pkgs)))
		}
		return keep
	}
	if e.d().id != "arch" {
		return pkgs
	}
	out, err := exec.Command("pacman", append([]string{"-T"}, pkgs...)...).Output()
	// exit 127 aside, pacman -T exits 1 when any target is unmet; the list on
	// stdout is the answer either way, so only a total failure falls back.
	if len(out) == 0 && err != nil {
		return pkgs
	}
	keep := filterByUnmet(pkgs, string(out))
	if len(keep) < len(pkgs) {
		e.say(fmt.Sprintf("%d of %d packages already satisfied by installed providers", len(pkgs)-len(keep), len(pkgs)))
	}
	return keep
}

func rpmProvides(cap string) bool {
	return exec.Command("rpm", "-q", "--quiet", "--whatprovides", cap).Run() == nil
}

// stepBuild is the fromSource replacement for installing ryoku-desktop: the
// payload's deploy.sh already builds the Go programs, the QML modules and the
// Ryoku.Blobs plugin, then materializes the config. It skips the Hyprland
// compositor plugins when makepkg is absent, which is the case off Arch.
func stepBuild(e *engine) error {
	dir := filepath.Join(e.payload, "ryoku", "shell")
	script := filepath.Join(dir, "deploy.sh")
	if e.dry {
		e.say("DRYRUN: would run " + script)
		return nil
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("payload is missing ryoku/shell/deploy.sh (%s)", script)
	}
	e.say("building the desktop from the payload (this takes a few minutes)")
	return e.cmd(dir, nil, "bash", script)
}

// filterByUnmet keeps only the names pacman -T reported as unmet.
func filterByUnmet(pkgs []string, unmetOut string) []string {
	unmet := map[string]bool{}
	for _, ln := range strings.Split(strings.TrimSpace(unmetOut), "\n") {
		if ln != "" {
			unmet[ln] = true
		}
	}
	var keep []string
	for _, p := range pkgs {
		if unmet[p] {
			keep = append(keep, p)
		}
	}
	return keep
}

func stepDrivers(e *engine) error {
	drv := filepath.Join(e.payload, "system/hardware/drivers")
	scripts := []string{"amd.sh", "intel.sh", "vulkan.sh"}
	if e.p.nvidia {
		scripts = append(scripts, "nvidia.sh")
	} else if e.f.hasNvidia {
		if e.f.secureBoot && !e.f.sbctlSigned {
			e.say("skipping the NVIDIA driver setup (Secure Boot is on and would reject the unsigned modules)")
		} else {
			e.say("skipping the NVIDIA driver setup (kept nouveau; re-run with the toggle on to switch)")
		}
	}
	// the vendor scripts each do a bare `pacman -S`, so -- exactly like
	// stepPackages -- they must transact against a current db. this step is
	// re-entered on a resume with the sysupgrade/packages steps already
	// skipped, and a repo publish can land between those steps and this one,
	// pruning the files a stale db still points at (pacman's "failed
	// retrieving file" abort that reads as a driver that would not install).
	if e.d().id == "arch" {
		if err := e.sudoSh(`rm -f /var/cache/pacman/pkg/*.part`); err != nil {
			e.say("warning: could not clear partial downloads (continuing)")
		}
		if err := e.sudo("pacman", "-Syu", "--noconfirm"); err != nil {
			e.say("warning: could not refresh the package db before the driver install; a stale mirror may still fail a download (continuing)")
		}
	} else if len(e.d().updateCmd) > 0 {
		if err := e.sudo(e.d().updateCmd...); err != nil {
			e.say("warning: could not refresh the package db before the driver install; a stale mirror may still fail a download (continuing)")
		}
	}
	// a single vendor script failing must NOT sink the whole desktop install,
	// matching installation/backend/lib/drivers.sh: the box still boots on the
	// iGPU or software renderer, and stepDoctor plus first boot heal the driver.
	for _, s := range scripts {
		if err := e.cmd("", nil, "bash", filepath.Join(drv, s)); err != nil {
			e.sayf("warning: %s did not finish; leaving the GPU driver for `ryoku doctor` after first boot (continuing)", s)
		}
	}
	if e.p.nvidia && e.f.hasNvidia {
		// the scripts leave the initramfs to the caller. probe for whichever
		// generator the box uses; a missed rebuild is a warning, not an abort,
		// the next kernel update rebuilds anyway.
		var err error
		switch {
		case has("limine-mkinitcpio"):
			err = e.sudo("limine-mkinitcpio")
		case has("mkinitcpio"):
			err = e.sudo("mkinitcpio", "-P")
		case has("dracut-rebuild"):
			err = e.sudo("dracut-rebuild")
		case has("dracut"):
			err = e.sudo("dracut", "--regenerate-all", "--force")
		default:
			e.say("warning: no known initramfs generator found, skipping the rebuild")
		}
		if err != nil {
			e.say("warning: initramfs rebuild failed; run it by hand before rebooting (see log)")
		}
	}
	return nil
}

func stepSession(e *engine) error {
	if e.p.switchDM {
		if dm := e.f.otherDM(); dm != "" {
			// disable, never mask or uninstall: reversible, and the running
			// greeter session is untouched until reboot.
			if err := e.sudo("systemctl", "disable", dm); err != nil {
				return err
			}
			e.recordRestore("sudo systemctl disable sddm.service && sudo systemctl enable " + dm)
		} else if e.f.currentDM == "" {
			e.recordRestore("sudo systemctl disable sddm.service && sudo systemctl set-default multi-user.target")
		}
		if err := e.cmd("", nil, "bash", filepath.Join(e.payload, "ryoku/lockscreen/sddm/setup")); err != nil {
			return err
		}
	} else {
		e.say("keeping your current display manager; select the Hyprland session at login")
	}

	// qylock bundle lives at the same system path the ISO uses, then its own
	// installer wires greeter theme + user lockscreen.
	if err := e.sudo("mkdir", "-p", "/usr/share/ryoku"); err != nil {
		return err
	}
	if err := e.sudoSh(`rm -rf /usr/share/ryoku/qylock`); err != nil {
		return err
	}
	if err := e.sudo("cp", "-r", filepath.Join(e.payload, "ryoku/lockscreen/qylock"), "/usr/share/ryoku/qylock"); err != nil {
		return err
	}
	if err := e.cmd("", []string{"RYOKU_QYLOCK_BUNDLE=/usr/share/ryoku/qylock"},
		"bash", filepath.Join(e.payload, "ryoku/lockscreen/install-qylock")); err != nil {
		return err
	}

	// greeter policy: install-qylock wrote 99-ryoku.conf and sddm/setup wrote
	// 10-ryoku-wayland.conf (DisplayServer=wayland). SDDM reads conf.d lexically
	// and later files win per key, so KDE's kde_settings.conf outranks the theme.
	// The greeter toggle decides who ends up on top; opting out drops both Ryoku
	// files so the user's greeter theme AND display server stand.
	switch {
	case !e.p.greeter:
		if err := e.sudo("rm", "-f", "/etc/sddm.conf.d/99-ryoku.conf", "/etc/sddm.conf.d/zz-ryoku.conf", "/etc/sddm.conf.d/10-ryoku-wayland.conf"); err != nil {
			return err
		}
		e.say("kept your current SDDM greeter (Ryoku's theme and Wayland greeter are installed, not selected)")
	case e.f.kdeSddmConf:
		zz := "# written by ryoku-shell-install: sorts after kde_settings.conf so the\n" +
			"# ryoku greeter theme wins. delete this file to get the KDE greeter back.\n" +
			"[Theme]\nCurrent=ryoku\n"
		if err := e.sudoWrite("/etc/sddm.conf.d/zz-ryoku.conf", zz); err != nil {
			return err
		}
		e.recordRestore("sudo rm -f /etc/sddm.conf.d/zz-ryoku.conf /etc/sddm.conf.d/99-ryoku.conf /etc/sddm.conf.d/10-ryoku-wayland.conf")
		e.say("Ryoku greeter theme selected past KDE's kde_settings.conf drop-in")
	default:
		e.recordRestore("sudo rm -f /etc/sddm.conf.d/99-ryoku.conf /etc/sddm.conf.d/10-ryoku-wayland.conf")
	}

	// sddm/setup strips pam_gnome_keyring on purpose: a fresh Ryoku box uses a
	// passwordless default keyring. an ex-GNOME user instead has a password
	// protected login keyring from GDM, and without these lines every login
	// prompts for it (stock Arch /etc/pam.d/sddm only carries kwallet).
	if e.p.switchDM && hasDesktop(e.f.desktops, "GNOME") {
		if err := e.sudoSh(`f=/etc/pam.d/sddm
if [ -f "$f" ] && ! grep -q pam_gnome_keyring "$f"; then
  if grep -q "password-auth" "$f"; then
    sed -i '/auth.*password-auth/a -auth       optional    pam_gnome_keyring.so' "$f"
    sed -i '/session.*password-auth/a -session    optional    pam_gnome_keyring.so    auto_start' "$f"
  else
    printf '%s\n' 'auth        optional    pam_gnome_keyring.so' 'session     optional    pam_gnome_keyring.so    auto_start' >> "$f"
  fi
fi`); err != nil {
			return err
		}
		e.recordRestore(`sudo sed -i '/pam_gnome_keyring\.so/d' /etc/pam.d/sddm`)
		e.say("kept gnome-keyring auto-unlock working under SDDM (your GNOME login keyring)")
	}
	if len(e.f.desktops) > 0 {
		e.sayf("%s stays installed; pick it from the session menu at the login screen anytime",
			strings.Join(e.f.desktops, ", "))
	}

	if e.p.switchNet {
		for _, n := range e.f.otherNet {
			if err := e.sudo("systemctl", "disable", n); err != nil {
				e.say("warning: could not disable " + n)
				continue
			}
			e.recordRestore("sudo systemctl enable " + n)
		}
		if !e.f.nmEnabled {
			if err := e.sudo("systemctl", "enable", "NetworkManager.service"); err != nil {
				return err
			}
			e.recordRestore("sudo systemctl disable NetworkManager.service")
		}
		// iwd backend pin, Ryoku network policy. takes effect at the next NM
		// restart (reboot), so the live wifi connection is never dropped.
		// On Fedora (and systems without iwd), retain wpa_supplicant so Wi-Fi does not break.
		if e.d().id != "fedora" && (e.d().id == "arch" || installed("iwd")) {
			if err := e.sudoSh(`install -Dm644 /dev/stdin /etc/NetworkManager/conf.d/wifi-backend.conf <<'EOF'
[device]
wifi.backend=iwd
EOF`); err != nil {
				return err
			}
			e.recordRestore("sudo rm -f /etc/NetworkManager/conf.d/wifi-backend.conf")
		}
	} else {
		e.say("keeping your current network stack")
	}
	return nil
}

func stepConfigs(e *engine) error {
	if !e.d().fromSource {
		ryoku := e.ryokuBin()
		if !e.dry && ryoku == "" {
			return fmt.Errorf("the ryoku CLI is missing; the package step did not finish")
		}
		if ryoku == "" {
			ryoku = "ryoku"
		}
		if err := e.cmd("", nil, ryoku, "materialize"); err != nil {
			return err
		}
	}

	// On Wayland, nvidia-settings -l fails (NV-CONTROL is X11-only). If the
	// distro ships nvidia-settings-user.desktop in /etc/xdg/autostart (e.g. Fedora),
	// mask it in ~/.config/autostart so systemd-xdg-autostart-generator skips it.
	if _, err := os.Stat("/etc/xdg/autostart/nvidia-settings-user.desktop"); err == nil {
		userAutostartDir := filepath.Join(e.f.homeDir, ".config/autostart")
		userAutostartFile := filepath.Join(userAutostartDir, "nvidia-settings-user.desktop")
		if e.dry {
			e.say("DRYRUN: would mask nvidia-settings autostart in " + userAutostartFile)
		} else if _, err := os.Lstat(userAutostartFile); err != nil {
			_ = os.MkdirAll(userAutostartDir, 0o755)
			content := "[Desktop Entry]\nType=Application\nName=nvidia-settings\nExec=nvidia-settings -l\nHidden=true\nX-systemd-skip=true\n"
			if err := os.WriteFile(userAutostartFile, []byte(content), 0o644); err == nil {
				e.say("masked nvidia-settings X11 autostart for Wayland -> " + userAutostartFile)
			}
		}
	}

	// On Fedora, gnome-keyring auto-unlock is not wired and secret-service
	// prompts deadlock Chromium's network service. Ensure chromium-flags.conf
	// uses --password-store=basic.
	if e.d().id == "fedora" {
		flagsFile := filepath.Join(e.f.homeDir, ".config/chromium-flags.conf")
		if data, err := os.ReadFile(flagsFile); err == nil {
			newData := strings.ReplaceAll(string(data), "--password-store=gnome-libsecret", "--password-store=basic")
			if newData != string(data) {
				if e.dry {
					e.say("DRYRUN: would set --password-store=basic in " + flagsFile)
				} else if err := os.WriteFile(flagsFile, []byte(newData), 0o644); err == nil {
					e.say("configured Chromium basic password store for Fedora -> " + flagsFile)
				}
			}
		}
	}

	// salvaged monitor pins go in before the stub pass, real pins beat a
	// comment stub. only the hyprland dialect supports desc: names.
	if e.p.monPins && len(e.f.monOutputs) > 0 {
		pins, skipped := renderPins(e.f.monOutputs, e.f.monSource == "hyprland", e.f.monSource)
		for _, name := range skipped {
			e.sayf("note: %s output %q is matched by description; pin it by connector in monitors_user.lua", e.f.monSource, name)
		}
		if pins != "" {
			mu := filepath.Join(e.f.homeDir, ".config/hypr/monitors_user.lua")
			if e.dry {
				e.sayf("DRYRUN: write %s monitor pins to ~/.config/hypr/monitors_user.lua", e.f.monSource)
			} else if _, err := os.Lstat(mu); err != nil {
				if err := os.WriteFile(mu, []byte(pins), 0o644); err != nil {
					return err
				}
				e.sayf("carried the %s monitor layout into hypr/monitors_user.lua", e.f.monSource)
			}
		}
	}

	// an explicit AZERTY choice in the plan beats any salvaged layout.
	azerty := e.p.azertyFR || e.p.azertyBE
	if azerty {
		layout := "fr"
		if e.p.azertyBE {
			layout = "be"
		}
		e.f.kbLayout, e.f.kbVariant, e.f.kbOptions, e.f.kbSource = layout, "", "", "plan"
	}

	// keyboard.lua is user-owned: seeded once, never touched by updates.
	lua := func(s string) string { return strings.NewReplacer(`"`, ``, `\`, ``).Replace(s) }
	if e.f.kbLayout != "" && (e.f.kbLayout != "us" || e.f.kbVariant != "" || e.f.kbOptions != "") {
		src := e.f.kbSource
		if src == "" {
			src = "localectl"
		}
		kb := filepath.Join(e.f.homeDir, ".config/hypr/keyboard.lua")
		// a salvaged layout never clobbers an existing file (a repair run
		// keeps hand edits); an explicit AZERTY choice always writes.
		if _, err := os.Lstat(kb); err == nil && !azerty {
			e.say("hypr/keyboard.lua already exists, keeping it")
		} else {
			e.sayf("seeding keyboard layout %q variant %q options %q (from %s) into hypr/keyboard.lua",
				e.f.kbLayout, e.f.kbVariant, e.f.kbOptions, src)
			if !e.dry {
				content := "-- keyboard layout, carried over by the installer. edits here stick.\n" +
					"hl.config({\n    input = {\n        kb_layout = \"" + lua(e.f.kbLayout) + "\",\n" +
					"        kb_variant = \"" + lua(e.f.kbVariant) + "\",\n" +
					"        kb_options = \"" + lua(e.f.kbOptions) + "\",\n    },\n})\n"
				if err := os.WriteFile(kb, []byte(content), 0o644); err != nil {
					return err
				}
			}
		}
	}

	// console + login-screen parity for an explicit AZERTY choice: the vt
	// keymap and SDDM's X11 greeter follow the desktop layout.
	if azerty {
		layout, keymap := "fr", "fr"
		if e.p.azertyBE {
			layout, keymap = "be", "be-latin1"
		}
		e.say("setting the console keymap to " + keymap + " in /etc/vconsole.conf")
		if err := e.sudoSh(`install -Dm644 /dev/stdin /etc/vconsole.conf <<'EOF'
KEYMAP=` + keymap + `
EOF`); err != nil {
			return err
		}
		e.say("pointing the SDDM login screen at the " + layout + " layout via xorg.conf.d")
		if err := e.sudoSh(`install -Dm644 /dev/stdin /etc/X11/xorg.conf.d/00-keyboard.conf <<'EOF'
Section "InputClass"
        Identifier "system-keyboard"
        MatchIsKeyboard "on"
        Option "XkbLayout" "` + layout + `"
EndSection
EOF`); err != nil {
			return err
		}
	}

	// the published loader still flags missing optional drop-ins in the
	// config-error overlay, so stub them until the searchpath fix ships.
	stubs := []struct{ rel, content string }{
		{".config/hypr/monitors_user.lua", "-- hand-pinned displays, see monitors_user.lua.example. pins here win.\n"},
		{".config/hypr/user.lua", "-- your hyprland overrides. loaded last, never touched by updates.\n"},
		{".config/hypr/theme.lua", "-- owned by ryoku settings: applying a theme replaces this file.\n"},
		{".config/hypr/settings.lua", "-- owned by ryoku settings, regenerated by the hub.\n"},
		{".config/hypr/modules/private.lua", "-- optional private module, yours to fill in.\n"},
		{".config/hypr/ghosttype.lua", "-- owned by ghosttype when installed.\n"},
	}
	for _, s := range stubs {
		if e.dry {
			e.say("DRYRUN: stub ~/" + s.rel + " if absent")
			continue
		}
		p := filepath.Join(e.f.homeDir, s.rel)
		if _, err := os.Lstat(p); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(s.content), 0o644); err != nil {
			return err
		}
		e.say("stubbed ~/" + s.rel)
	}

	seeds := []struct {
		src, dst string
		dir      bool
		ifAbsent bool
	}{
		{"ryoku/assets/brand", ".local/share/ryoku/assets/brand", true, false},
		{"ryoku/assets/wallpapers", "Pictures/Wallpapers", true, true},
		{"ryoku/apps/npm/npmrc", ".npmrc", false, true},
		{"ryoku/apps/nvim/ryoku-nvim.desktop", ".local/share/applications/ryoku-nvim.desktop", false, false},
		// no mimeapps.list seed: the default-app map belongs to the vendor layer
		// (/usr/share/applications/mimeapps.list, laid by ryoku-desktop or by
		// deploy.sh), never to ~/.config/mimeapps.list, which is where the user's
		// own "Set as default" picks live.
	}
	for _, s := range seeds {
		src := filepath.Join(e.payload, s.src)
		dst := filepath.Join(e.f.homeDir, s.dst)
		if e.dry {
			e.say("DRYRUN: seed " + s.src + " -> ~/" + s.dst)
			continue
		}
		if err := seedPath(src, dst, s.dir, s.ifAbsent); err != nil {
			return fmt.Errorf("seed %s: %w", s.dst, err)
		}
		e.say("seeded ~/" + s.dst)
	}
	e.installDesktopExtras()
	return e.cmd("", nil, "systemctl", "--user", "daemon-reload")
}

// installDesktopExtras deploys prebuilt binary and asset releases for tools
// that are not provided by the host package manager (matugen, awww, gpk, Bibata cursors, Space Grotesk).
// This is the default zero-compile installation method on Fedora and fromSource systems,
// guaranteeing that no heavy Rust or C++ compiler toolchains are required on user machines.
func (e *engine) installDesktopExtras() {
	if e.dry || e.f == nil || e.f.homeDir == "" {
		return
	}
	binDir := filepath.Join(e.f.homeDir, ".local", "bin")
	_ = os.MkdirAll(binDir, 0o755)

	// matugen: official precompiled binary release (zero-compile default)
	if !has("matugen") {
		if e.d().id == "fedora" {
			_ = e.sudo("dnf", "-y", "install", "matugen")
		}
	}
	if !has("matugen") && runtime.GOARCH == "amd64" {
		dst := filepath.Join(binDir, "matugen")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			e.say("installing prebuilt matugen binary (zero-compile)")
			cmd := fmt.Sprintf(`curl -fsSL --connect-timeout 10 -m 30 https://github.com/InioX/matugen/releases/download/v4.2.0/matugen-4.2.0-x86_64.tar.gz | tar -xz -C %q matugen && chmod +x %q`, binDir, dst)
			if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
				e.say("warning: could not install prebuilt matugen: " + strings.TrimSpace(string(out)))
			}
		}
	}

	// awww: wallpaper daemon and client
	if !has("awww") && e.d().id == "fedora" {
		e.say("installing awww via DNF")
		_ = e.sudo("dnf", "-y", "install", "awww")
	}

	// gpk: GlazePKG prebuilt binary release
	if !has("gpk") {
		dst := filepath.Join(binDir, "gpk")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			arch := "amd64"
			if runtime.GOARCH == "arm64" {
				arch = "arm64"
			}
			e.say("installing prebuilt gpk binary")
			cmd := fmt.Sprintf(`curl -fsSL --connect-timeout 10 -m 30 https://github.com/neur0map/glazepkg/releases/latest/download/gpk-linux-%s -o %q 2>/dev/null && chmod +x %q || true`, arch, dst, dst)
			_ = exec.Command("sh", "-c", cmd).Run()
		}
	}

	// prowl-agent: official prebuilt binary release for rashin code intelligence
	if !has("prowl-agent") {
		dst := filepath.Join(binDir, "prowl-agent")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			arch := "amd64"
			if runtime.GOARCH == "arm64" {
				arch = "arm64"
			}
			e.say("installing prebuilt prowl-agent binary")
			cmd := fmt.Sprintf(`curl -fsSL --connect-timeout 10 -m 30 https://github.com/neur0map/prowl-agent/releases/latest/download/prowl-agent-linux-%s -o %q 2>/dev/null && chmod +x %q || true`, arch, dst, dst)
			_ = exec.Command("sh", "-c", cmd).Run()
		}
	}

	// spicetify-cli: official prebuilt release
	if !has("spicetify") {
		dst := filepath.Join(binDir, "spicetify")
		spicetifyDir := filepath.Join(e.f.homeDir, ".spicetify")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			arch := "amd64"
			if runtime.GOARCH == "arm64" {
				arch = "arm64"
			}
			e.say("installing prebuilt spicetify-cli")
			cmd := fmt.Sprintf(`mkdir -p %q && curl -fsSL --connect-timeout 10 -m 30 https://github.com/spicetify/cli/releases/download/v2.44.0/spicetify-2.44.0-linux-%s.tar.gz | tar -xz -C %q 2>/dev/null && chmod +x %q && ln -sf %q %q || true`,
				spicetifyDir, arch, spicetifyDir, filepath.Join(spicetifyDir, "spicetify"), filepath.Join(spicetifyDir, "spicetify"), dst)
			_ = exec.Command("sh", "-c", cmd).Run()
		}
	}

	// bibata cursor theme: official prebuilt cursor assets
	iconsDir := filepath.Join(e.f.homeDir, ".local", "share", "icons")
	if !pathExists("/usr/share/icons/Bibata-Modern-Ice") && !pathExists(filepath.Join(iconsDir, "Bibata-Modern-Ice")) {
		_ = os.MkdirAll(iconsDir, 0o755)
		e.say("installing Bibata cursor theme")
		cmd := fmt.Sprintf(`curl -fsSL --connect-timeout 10 -m 30 https://github.com/ful1e5/Bibata_Cursor/releases/download/v2.0.7/Bibata.tar.xz | tar -xJ -C %q 2>/dev/null || true`, iconsDir)
		_ = exec.Command("sh", "-c", cmd).Run()
	}

	// Space Grotesk brand font: official prebuilt OTF release
	fontsDir := filepath.Join(e.f.homeDir, ".local", "share", "fonts")
	if !pathExists("/usr/share/fonts/OTF/SpaceGrotesk-Regular.otf") && !pathExists(filepath.Join(fontsDir, "SpaceGrotesk")) {
		_ = os.MkdirAll(filepath.Join(fontsDir, "SpaceGrotesk"), 0o755)
		e.say("installing Space Grotesk brand font")
		cmd := fmt.Sprintf(`tmp=$(mktemp -d) && curl -fsSL --connect-timeout 10 -m 30 https://github.com/floriankarsten/space-grotesk/releases/download/2.0.0/SpaceGrotesk-2.0.0.zip -o "$tmp/sg.zip" && unzip -qo "$tmp/sg.zip" -d "$tmp" && cp -f "$tmp"/SpaceGrotesk-*/otf/*.otf %q/ 2>/dev/null; rm -rf "$tmp" || true`, filepath.Join(fontsDir, "SpaceGrotesk"))
		_ = exec.Command("sh", "-c", cmd).Run()
	}

	// Material Symbols icon font: official variable TTF release
	fontDst := filepath.Join(fontsDir, "MaterialSymbolsRounded.ttf")
	if !pathExists(fontDst) && !pathExists("/usr/share/fonts/TTF/MaterialSymbolsRounded.ttf") && !pathExists("/usr/share/fonts/material-symbols/MaterialSymbolsRounded.ttf") {
		_ = os.MkdirAll(fontsDir, 0o755)
		e.say("installing Material Symbols icon font")
		cmd := fmt.Sprintf(`curl -fsSL --connect-timeout 10 -m 30 "https://raw.githubusercontent.com/google/material-design-icons/master/variablefont/MaterialSymbolsRounded%%5BFILL%%2CGRAD%%2Copsz%%2Cwght%%5D.ttf" -o %q 2>/dev/null && fc-cache -f %q || true`, fontDst, fontsDir)
		_ = exec.Command("sh", "-c", cmd).Run()
	}
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func stepAUR(e *engine) error {
	if !e.p.aur {
		e.say("AUR extras skipped by choice; wallpaper needs awww (ryoku doctor will nag)")
		return nil
	}
	helper := e.f.aurHelper
	if helper == "" {
		e.say("no AUR helper found, bootstrapping yay-bin")
		tmp, err := os.MkdirTemp("", "ryoku-yay-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if err := e.cmd(tmp, nil, "git", "clone", "https://aur.archlinux.org/yay-bin.git"); err != nil {
			return err
		}
		// build and install separately: makepkg -i runs a plain interactive
		// sudo, which on a lapsed credential prompts on /dev/tty over the TUI
		// and hangs there; the engine's sudo -n fails loudly instead.
		if err := e.cmd(filepath.Join(tmp, "yay-bin"), nil, "makepkg", "--noconfirm"); err != nil {
			return err
		}
		built, _ := filepath.Glob(filepath.Join(tmp, "yay-bin", "*.pkg.tar.zst"))
		if len(built) == 0 {
			return fmt.Errorf("yay-bin build produced no package")
		}
		if err := e.sudo(append([]string{"pacman", "-U", "--noconfirm"}, built...)...); err != nil {
			return err
		}
		helper = "yay"
	}
	var failed []string
	for _, p := range aurPkgs {
		// --sudoflags=-n, same reason: the helper's own sudo must fail into
		// the log, never prompt over the TUI. yay and paru both take it.
		if err := e.cmd("", nil, helper, "-S", "--needed", "--noconfirm", "--sudoflags=-n", p); err != nil {
			failed = append(failed, p)
			e.say("warning: AUR build failed for " + p + " (continuing)")
		}
	}
	if len(failed) > 0 {
		e.say("AUR packages that did not install: " + strings.Join(failed, " "))
		e.say("re-run later with: " + helper + " -S " + strings.Join(failed, " "))
	}
	return nil
}

func stepFish(e *engine) error {
	if !e.p.fish {
		e.say("keeping your current login shell")
		return nil
	}
	if err := e.sudo("usermod", "-s", "/usr/bin/fish", e.f.username); err != nil {
		return err
	}
	e.recordRestore("sudo usermod -s " + e.f.userShell + " " + e.f.username)
	return nil
}

func stepDoctor(e *engine) error {
	// doctor converges snapper (btrfs only), NVIDIA modeset, greeter perms,
	// session components. findings are advice, not failure.
	ryoku := e.ryokuBin()
	if ryoku == "" {
		ryoku = "ryoku"
	}
	if err := e.cmd("", nil, ryoku, "doctor"); err != nil {
		e.say("note: ryoku doctor reported findings (see above); the install itself is done")
	}
	return nil
}

func stepVerify(e *engine) error {
	if e.dry {
		e.say("DRYRUN: verify [ryoku] repo, packages, session files")
		return nil
	}
	var bad []string
	check := func(ok bool, what string) {
		if ok {
			e.say(gCheck + " " + what)
		} else {
			bad = append(bad, what)
			e.say(gBad + " " + what)
		}
	}
	if e.d().fromSource {
		check(e.ryokuBin() != "", "ryoku CLI built and installed")
		_, err := os.Stat(filepath.Join(e.f.homeDir, ".local/bin/ryoku-shell"))
		check(err == nil, "ryoku-shell daemon built")
	} else {
		conf, _ := os.ReadFile("/etc/pacman.conf")
		check(strings.Contains(string(conf), "[ryoku]"), "[ryoku] repository in /etc/pacman.conf")
		check(pacmanHas("ryoku-keyring"), "ryoku-keyring package installed")
		check(pacmanHas("ryoku-desktop"), "ryoku-desktop package installed")
		check(has("ryoku"), "ryoku CLI on PATH")
		st, err := os.Stat("/usr/share/ryoku/config")
		check(err == nil && st.IsDir(), "base config tree at /usr/share/ryoku/config")
	}
	var err error
	_, err = os.Stat(filepath.Join(e.f.homeDir, ".config/hypr/hyprland.lua"))
	check(err == nil, "hyprland.lua materialized in ~/.config/hypr")
	_, err = os.Stat("/usr/share/wayland-sessions/hyprland.desktop")
	check(err == nil, "Hyprland wayland session registered")
	if e.p.switchDM {
		check(unitEnabled("system", "sddm.service"), "sddm.service enabled")
	}
	if e.p.switchDM && e.p.greeter {
		if theme := effectiveSDDMTheme(); theme != "" && theme != "ryoku" {
			e.say(gWarn + " an SDDM drop-in still selects greeter theme " + theme + "; check /etc/sddm.conf.d")
		}
	}
	if e.f.hasNvidia && e.f.secureBoot && !e.p.nvidia {
		e.say(gWarn + " Secure Boot is on, so the proprietary NVIDIA driver was skipped: unsigned")
		e.say("  DKMS modules are rejected at boot. To switch later, disable Secure Boot in")
		e.say("  firmware or sign the kernel and modules (sbctl), then re-run this installer.")
	}
	// matugen palette generator: verified on all distros (packaged on Arch,
	// installed via zero-compile prebuilt release on Fedora and source builds).
	if has("matugen") {
		check(true, "matugen palette generator (colors follow the wallpaper)")
	} else if e.d().local("matugen") == "" && e.d().id == "debian" {
		e.say(gWarn + " matugen is not packaged here: wallpaper palettes stay at their defaults")
	} else {
		check(false, "matugen palette generator (colors follow the wallpaper)")
	}
	if !has("awww") {
		if e.d().id == "arch" {
			e.say(gWarn + " awww missing (AUR): static wallpapers will not set until it installs (ryoku doctor retries it)")
		} else {
			e.say(gWarn + " awww missing: static wallpapers will not set until it installs")
		}
	} else {
		check(true, "awww wallpaper daemon installed")
	}
	if e.p.devtools {
		check(has("go"), "go toolchain on PATH (ryoku recovery rebuilds from source)")
	} else {
		hint := "sudo pacman -S go"
		if e.d().id == "fedora" {
			hint = "sudo dnf install golang"
		} else if e.d().id == "debian" {
			hint = "sudo apt-get install golang"
		}
		e.say(gWarn + " developer toolchain skipped: ryoku recovery needs go; install with: " + hint)
	}
	if e.p.omarchy {
		conf2, _ := os.ReadFile("/etc/pacman.conf")
		if omarchyStanzaRe.Match(conf2) {
			e.say(gWarn + " the [omarchy] repository is still in /etc/pacman.conf")
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d check(s) failed: %s", len(bad), strings.Join(bad, "; "))
	}
	e.say("all checks passed")
	return nil
}

// ---- fs helpers ----

func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		tgt, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(tgt, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		ents, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, ent := range ents {
			if err := copyTree(filepath.Join(src, ent.Name()), filepath.Join(dst, ent.Name())); err != nil {
				return err
			}
		}
		return nil
	default:
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	}
}

// seedPath copies src to dst. dirs merge without overwriting when ifAbsent
// (user wallpapers win); files honor ifAbsent per file.
func seedPath(src, dst string, dir, ifAbsent bool) error {
	if !dir {
		if ifAbsent {
			if _, err := os.Lstat(dst); err == nil {
				return nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyTree(src, dst)
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, ent := range ents {
		s, d := filepath.Join(src, ent.Name()), filepath.Join(dst, ent.Name())
		if ifAbsent {
			if _, err := os.Lstat(d); err == nil {
				continue
			}
		} else if !ent.IsDir() {
			os.Remove(d)
		}
		if err := copyTree(s, d); err != nil {
			return err
		}
	}
	return nil
}
