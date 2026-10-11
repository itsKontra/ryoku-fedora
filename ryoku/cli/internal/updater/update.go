package updater

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/godbus/dbus/v5"
	"os"
	"os/exec"
	"path/filepath"
	"ryoku-cli/internal/sys"
	i18n "ryoku-i18n"
	wm "ryoku-wm"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

const snapperConfig = "root"

// gitSteps / pkgSteps are the ordered stages the GUI renders as a determinate
// multi-segment bar. The git-channel path (a dev/mirror checkout) and the
// packaged path (dnf) run different stages; stage2 re-begins pkgSteps and
// marks the pre-handoff steps done so the exec handoff keeps one continuous bar.
var (
	gitSteps = []runStep{
		{Key: "snapshot", Label: i18n.T("Taking a snapshot")},
		{Key: "channel", Label: i18n.T("Pulling the latest commits")},
		{Key: "deploy", Label: i18n.T("Deploying the desktop")},
		{Key: "doctor", Label: i18n.T("Healing the system")},
		{Key: "finalize", Label: i18n.T("Finishing up")},
	}
	// The Ryoku lane, the default: only the packages Ryoku publishes move (see
	// ryokuset.go). The base system, its kernel and Flatpak are the user's
	// lane, reported at the end and taken with `sudo dnf upgrade`.
	pkgSteps = []runStep{
		{Key: "snapshot", Label: i18n.T("Taking a snapshot")},
		{Key: "packages", Label: i18n.T("Updating the Ryoku packages")},
		{Key: "apply", Label: i18n.T("Applying the new configuration")},
		{Key: "reload", Label: i18n.T("Reloading the desktop")},
		{Key: "doctor", Label: i18n.T("Healing the system")},
		{Key: "finalize", Label: i18n.T("Finishing up")},
	}
	// `ryoku update --system`: the same run plus the user's lane, for a box
	// that wants one command. The extra steps sit where they used to.
	systemSteps = []runStep{
		{Key: "snapshot", Label: i18n.T("Taking a snapshot")},
		{Key: "packages", Label: i18n.T("Updating the Ryoku packages")},
		{Key: "system", Label: i18n.T("Updating the system packages")},
		{Key: "flatpak", Label: i18n.T("Updating Flatpak apps")},
		{Key: "apply", Label: i18n.T("Applying the new configuration")},
		{Key: "reload", Label: i18n.T("Reloading the desktop")},
		{Key: "doctor", Label: i18n.T("Healing the system")},
		{Key: "finalize", Label: i18n.T("Finishing up")},
	}
)

// Update = the whole safe update, wrapped in a snapper pre/post pair.
// checkout box -> git channel (fast-forward + redeploy). packaged box ->
// the Ryoku packages, then hand off to the binary dnf just installed
// (--stage2) so the deploy and doctor semantics of the new release apply
// during this same update, not one release late. stage2 quiesces the shell,
// materializes, brings the desktop back, and runs `ryoku doctor` (same one
// users run by hand) to heal stateful drift, then the post snapshot.
// snapshots are best-effort: an unconfigured snapper never blocks an update,
// but a failed step still aborts first. Each stage is published to the
// run-state file so the update island and Hub show real, determinate progress.
//
// It updates the Ryoku set only (ryokuset.go). The base system and its kernel
// come from Fedora and stay the user's to take with `sudo dnf upgrade`; the run
// reports what is waiting there.
// `--system` runs that lane too, in one command.
func Update(args []string) error {
	stage2 := len(args) >= 2 && args[0] == "--stage2"
	withSystem := false
	for _, a := range args {
		if a == "-v" || a == "--verbose" {
			verboseLog = true
		}
		if a == "--system" || a == "--full" {
			withSystem = true
		}
	}

	// One update at a time: a second run mid-transaction (a double-click, a timer
	// racing a manual update) can corrupt the dnf transaction or the config swap. Best-effort
	// -- a lock we cannot even create never blocks an update, only a held one does.
	lock, busy := acquireUpdateLock()
	if busy != nil {
		return busy
	}
	if lock != nil {
		defer lock.Close()
	}

	if stage2 {
		return updateStage2(args[1], withSystem)
	}

	// Stop before starting if the disk is too full: a dnf transaction or a
	// copy-on-write snapshot that runs out of space leaves the system half-upgraded.
	if free, ok := enoughFreeSpace(); !ok {
		return fmt.Errorf(i18n.T("only %s free on /; free up space before updating "+
			"(an update that runs out of disk can leave the system half-upgraded)"), free)
	}

	// Cache the sudo credential once, on the terminal, before any step needs it.
	// The pre-snapshot, dnf, flatpak and the post snapshot all escalate, several
	// through pipes or RunOut where a prompt cannot be seen; one prompt up front is
	// what users were doing by hand with `sudo -v && ryoku update`.
	primeSudo()
	stopKeepalive := sudoKeepalive()
	defer stopKeepalive()

	checkout := sys.ResolveRepo() != ""
	switch {
	case checkout:
		progress.begin(gitSteps)
	case withSystem:
		progress.begin(systemSteps)
	default:
		progress.begin(pkgSteps)
	}

	progress.at("snapshot")
	pre := snapperPre(snapshotDesc())
	progress.setSnapshot(pre)

	// checkout: update through the git channel. packaged: dnf + a hand-off
	// to the freshly installed binary (stage2).
	if checkout {
		logPath := startUpdateLog()
		defer stopUpdateLog()
		if err := channelUpdate(); err != nil {
			progress.fail(err)
			return err
		}
		rashinReindex()
		prowlRefresh()
		progress.at("doctor")
		runFreshDoctor()
		progress.at("finalize")
		snapperPost(pre, "ryoku-update")
		progress.logf(i18n.T("Update complete"))
		if logPath != "" {
			fmt.Println("  " + sys.Dim(i18n.T("full log: ")+logPath))
		}
		return finishRun()
	}

	progress.at("packages")
	// the release this box runs before dnf moves it; stage2 (the new
	// binary) reads it from the environment to arm the boot guard.
	if from := sys.ReadRelease().Release; from != "" {
		os.Setenv("RYOKU_UPDATE_FROM", from)
	}
	// Refresh first, then read the set, so it is what the repo serves now.
	if err := sys.Sudo(refreshDBArgs()[1:]...); err != nil {
		progress.logf(i18n.T("Could not refresh the package databases: %v"), err)
	}
	set, err := installedRyokuSet()
	if err != nil {
		e := fmt.Errorf(i18n.T("cannot read the %s repository, so there is nothing safe to update: %w"), sys.RPMRepoName, err)
		progress.fail(e)
		return e
	}
	if len(set) == 0 {
		e := fmt.Errorf(i18n.T("no packages from the %s repository are installed; `ryoku doctor` checks the repo setup"), sys.RPMRepoName)
		progress.fail(e)
		return e
	}
	progress.logf(i18n.T("Updating %d Ryoku package(s); the base system stays as it is"), len(set))
	err = runRyokuUpgrade(set)
	if err != nil {
		// One retry on fresh metadata: a COPR mirror or network hiccup is the
		// usual cause, and the second attempt often lands on a healthy mirror.
		progress.logf(i18n.T("The Ryoku package upgrade failed; refreshing %s and retrying once"), sys.RPMRepoName)
		_ = sys.Sudo(refreshDBArgs()[1:]...)
		err = runRyokuUpgrade(set)
	}
	if err != nil {
		// only advertise `ryoku rollback` when the pre snapshot it needs exists;
		// snapperPre is best-effort and returns "" when it was skipped.
		hint := i18n.T("no pre-update snapshot exists (snapper was unavailable), so `ryoku rollback` cannot revert this; recover with dnf directly")
		if pre != "" {
			hint = i18n.T("see `ryoku rollback` (pre-update snapshot ") + pre + ")"
		}
		e := fmt.Errorf(i18n.T("the Ryoku package upgrade failed; %s: %w"), hint, err)
		progress.fail(e)
		return e
	}

	// The user's lane. Off by default: the kernel and the base come from Fedora
	// on the user's schedule, and this run must not decide that for
	// them. Reported either way, so nobody has to guess whether something is
	// waiting; `--system` takes it here instead.
	pendingSystem := systemLanePending(set)
	if withSystem {
		runSystemLane(pendingSystem)
	} else {
		reportSystemLane(pendingSystem)
	}

	// exec replaces this process with the freshly installed binary; on any
	// failure fall through and finish in-process, exactly as before.
	if sys.Exists("/usr/bin/ryoku") {
		hand := []string{"ryoku", "update", "--stage2", pre}
		if withSystem {
			hand = append(hand, "--system")
		}
		if err := syscall.Exec("/usr/bin/ryoku", hand, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("warning: could not hand off to the updated binary: %v\n"), err)
		}
	}
	return updateStage2(pre, withSystem)
}

// --- update safeguards (concurrency, disk, sleep, snapshot noise) -----------

// acquireUpdateLock takes an exclusive, non-blocking flock so only one
// `ryoku update` runs at a time. Returns (nil, err) when another update already
// holds it (the caller aborts); (nil, nil) when the lock file cannot even be
// created (proceed best-effort, like the snapshot); (f, nil) when acquired -- the
// caller keeps f open for the update's lifetime and closes it to release. The fd
// is close-on-exec, so the stage1->stage2 handoff re-acquires cleanly.
func acquireUpdateLock() (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(sys.Xdg("XDG_RUNTIME_DIR", ".cache"), "ryoku-update.lock"),
		os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, nil // cannot create a lock file -> never block the update
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf(i18n.T("another ryoku update is already running; wait for it to finish"))
	}
	return f, nil
}

// enoughFreeSpace reports whether / has room for an update (the download, the new
// packages, and a copy-on-write snapshot). A statfs it cannot read never blocks.
// The returned string is the human-readable free size, for the error message.
func enoughFreeSpace() (string, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return "", true
	}
	free := st.Bavail * uint64(st.Bsize)
	return humanBytes(free), free >= (1 << 30) // 1 GiB floor
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MiB", n>>20)
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}

// primeSudo caches the sudo credential once, up front, on the real terminal, so
// every escalation the rest of the update makes finds it instead of prompting.
// Several of those prompts cannot be seen or answered: the pre-snapshot runs
// through RunOut (no tty), dnf runs through the curated output pipe, and flatpak
// escalates on its own -- an unseen prompt there is exactly why users
// learned to run `sudo -v` by hand first. No tty -> skip (a GUI or timer run has
// no terminal to prompt on); a NOPASSWD box sees nothing. Best-effort.
func primeSudo() {
	if !sys.StdinIsTTY() {
		return
	}
	_ = sys.Run("sudo", "-v")
}

// sudoKeepalive refreshes the cached credential every minute so a long
// transaction (a slow mirror) cannot let it lapse mid-run and re-prompt
// where the prompt is invisible. `-n` never prompts, so once the credential is
// gone this is a silent no-op, and RunOut keeps its "a password is required" off
// the terminal. The returned stop func ends the refresher; the stage1->stage2
// exec replaces the process, so a stop it never reaches leaks nothing.
func sudoKeepalive() func() {
	if !sys.StdinIsTTY() {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				_, _ = sys.RunOut("sudo", "-n", "-v")
			}
		}
	}()
	return func() { close(stop) }
}

// snapshotDesc labels the pre-update snapshot (and its GRUB boot-menu entry)
// with the version being updated from, so a user restoring after a bad update can
// tell the snapshots apart instead of a row of identical "ryoku-update".
func snapshotDesc() string {
	v := ""
	if repo := sys.ResolveRepo(); repo != "" {
		v = gitShort(repo, "HEAD")
	} else {
		v = shortCommit(sys.InstalledVersion())
	}
	if v == "" {
		return "ryoku-update"
	}
	return "ryoku-update (from " + v + ")"
}

// runRyokuUpgrade moves the Ryoku set, sleep-inhibited (a lid-close or idle
// suspend mid-transaction cannot corrupt it). The argv, and why it is not a
// sysupgrade, live in ryokuset.go.
func runRyokuUpgrade(set []string) error {
	return runUpgrade("Ryoku", "Ryoku package upgrade", ryokuInstallArgs(set))
}

// runSystemLane is `ryoku update --system`: the user's lane, opted into. It is
// the full sysupgrade, so it can move the kernel; that is the point of asking
// for it. Failures warn and continue, like the Flatpak step: the
// Ryoku set is already in, and stage2 still has to bring the desktop back.
func runSystemLane(pending []updateItem) {
	progress.at("system")
	progress.logf(i18n.T("Updating %d system package(s) (%s, kernel included)"), len(pending), systemUpgradeCmd())
	if err := runInhibited(i18n.T("System"), "System package upgrade", systemUpgradeArgs()); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("warning: the system upgrade reported errors: %v\n"), err)
	}
	// Flatpak apps are a separate channel from dnf. Skipped when
	// there is nothing to update: an offline box, or one with the client but no
	// remote, must not turn a whole update red.
	if flatpakUpdatable() {
		progress.at("flatpak")
		progress.logf(i18n.T("Updating Flatpak apps"))
		if err := runFlatpakUpgrade(); err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("warning: flatpak update reported errors: %v\n"), err)
		}
	} else {
		progress.skip("flatpak")
	}
}

// reportSystemLane names what the other lane is holding and the one command
// that takes it. Silence here would read as "everything is up to date", which
// is the whole failure this split has to avoid.
func reportSystemLane(pending []updateItem) {
	if len(pending) == 0 {
		progress.logf(i18n.T("The base system is current; nothing waiting outside the Ryoku set"))
		return
	}
	progress.logf(i18n.T("%d system package(s) waiting from your distribution (kernel included): take them with `sudo %s`"), len(pending), systemUpgradeCmd())
}

// systemUpgradeCmd is the human-readable form of what systemUpgradeArgs runs,
// for the update and status messages.
func systemUpgradeCmd() string { return "dnf upgrade" }

// systemUpgradeArgs is the user's lane, run only by `ryoku update --system`:
// the full upgrade, kernel included, exactly what `sudo dnf upgrade` does by
// hand.
func systemUpgradeArgs() []string { return []string{"sudo", "dnf", "-y", "upgrade"} }

// flatpakUpdatable reports whether a flatpak update is worth attempting at all:
// the client is present and at least one remote is configured. A fresh offline
// install has the client and no remote (doctor adds it once there is a network),
// and `flatpak update` there would only print a confusing error into the middle
// of an otherwise clean run.
func flatpakUpdatable() bool {
	if !sys.Has("flatpak") {
		return false
	}
	out, err := sys.RunOut("flatpak", "remotes", "--columns=name")
	return err == nil && strings.TrimSpace(out) != ""
}

// runFlatpakUpgrade updates every installed flatpak app and runtime under the
// same sleep inhibitor as the package steps, since a suspend mid-deploy leaves a
// half-written app tree.
func runFlatpakUpgrade() error {
	return runInhibited("Flatpak", "Flatpak app upgrade",
		[]string{"flatpak", "update", "--noninteractive", "--assumeyes"})
}

// runInhibited runs argv while holding a logind sleep+idle block, so a suspend
// mid-upgrade cannot interrupt a package transaction. Degrades to running argv
// directly when systemd-inhibit is unavailable. On a real terminal it renders a
// curated view of the output (phase is the header label); for pipes, logs, and
// --verbose it streams raw so nothing that scrapes the output breaks.
func runInhibited(phase, why string, argv []string) error {
	full := argv
	if sys.Has("systemd-inhibit") {
		head := []string{"systemd-inhibit", "--what=sleep:idle", "--who=ryoku update", "--why=" + why, "--mode=block"}
		full = append(head, argv...)
	}
	if verboseLog || !sys.StdoutIsTTY() {
		return sys.Run(full[0], full[1:]...)
	}
	return renderUpgrade(phase, full)
}

// finishRun publishes the terminal "done" state, holds it briefly so a watching
// GUI catches the completion, then clears the run so the island folds away.
func finishRun() error {
	progress.finish()
	time.Sleep(1200 * time.Millisecond)
	progress.idle()
	return nil
}

// updateStage2 finishes an update after the package transactions: deploy the
// new configs with the shell quiesced, bring the desktop back, heal drift. It
// runs in the freshly installed binary (a new process after the exec handoff),
// so it re-begins the packaged step list and marks the pre-handoff steps done
// to keep one continuous progress bar.
func updateStage2(pre string, withSystem bool) error {
	// A fresh process after the exec handoff: re-cache the credential (a no-op
	// inside the timeout) so materialize, the post snapshot and doctor never
	// prompt where it cannot be seen.
	primeSudo()
	stopKeepalive := sudoKeepalive()
	defer stopKeepalive()
	// Re-begin the SAME step list stage1 published, or the island would lose
	// the steps that already ran when a --system run hands over.
	steps := pkgSteps
	if withSystem {
		steps = systemSteps
	}
	progress.begin(steps)
	progress.setSnapshot(pre)
	progress.markDone("snapshot", "packages")
	if withSystem {
		progress.markDone("system")
		if flatpakUpdatable() {
			progress.markDone("flatpak")
		} else {
			progress.skip("flatpak")
		}
	}

	// The packages are in. From here the boot guard watches: if the next two
	// boots never bring the desktop up, it puts the Ryoku set back on the
	// release this box ran before. Armed only for a real move (the release
	// changed) so a no-op update never leaves a marker behind.
	armBootGuard(pre)

	progress.at("apply")
	progress.logf(i18n.T("Applying the new configuration"))
	if err := ensurePackagePowerCutover(); err != nil {
		progress.fail(err)
		return err
	}
	graphicalPresent, shellExpected, err := graphicalSessionState()
	if err != nil {
		progress.fail(err)
		return err
	}
	wallpaperWasActive := shellExpected &&
		exec.Command("systemctl", "--user", "is-active", "--quiet", "ryogami.service").Run() == nil
	powerCutoverLock, err := acquirePowerCutoverLock()
	if err != nil {
		progress.fail(err)
		return err
	}
	defer func() {
		if powerCutoverLock != nil {
			_ = powerCutoverLock.Close()
		}
	}()
	cutoverGuard, err := acquireUpdateSleepGuard()
	if err != nil {
		progress.fail(err)
		return err
	}
	// Every later failure used to return with the durable block still live:
	// the leaked guard denies every suspend with logind's "Operation denied
	// due to active block inhibitor" until logout (#282, #285). The explicit
	// release below stays authoritative; this is the silent safety net, and
	// Release is a no-op once the unit is stopped.
	defer func() {
		if rerr := cutoverGuard.Release(); rerr != nil {
			fmt.Fprintf(os.Stderr, i18n.Tf("warning: could not release the update sleep guard: %v\n", rerr))
		}
	}()
	defer cutoverGuard.Disconnect()
	// Activate logind's sessionless fallback while every old owner and the
	// durable block are still live. Only after that succeeds may the old
	// direct-suspend hooks and clamshell process be quiesced.
	if err := reloadLogindLidPolicy(); err != nil {
		progress.fail(err)
		return err
	}
	if err := preparePowerPolicyUpdate(); err != nil {
		progress.fail(err)
		return err
	}
	generationGuardHeld := false
	defer func() {
		// A failed update may never restart the shell whose ExecStartPost would
		// release the guard; left held, every lock and suspend blocks on it.
		if generationGuardHeld {
			_ = runPowerHelper("ryoku-power-cutover", "generation-guard-stop")
		}
	}()
	if graphicalPresent {
		// Stop admission before quiescing the old daemon. New launchers queue at
		// this boundary; already-loaded wrappers can still use the old daemon to
		// complete their ordered unlock.
		generationGuardHeld = true
		if err := runPowerHelper("ryoku-power-cutover", "generation-guard-start"); err != nil {
			progress.fail(err)
			return err
		}
	}
	// stop the shell first: a live quickshell would hot-reload the half-copied
	// tree mid-swap, re-instantiating the new QML against whatever plugin .so
	// the old process still has mapped. pause Hyprland's Lua auto-reload for
	// the same reason (= emergency overlay popping up with no keybinds).
	if err := stopShell(); err != nil {
		progress.fail(err)
		return err
	}
	if graphicalPresent {
		// The first rollout can encounter a pre-guard wrapper. With its launcher
		// now quiesced, drain that final legacy client before replacing files.
		if err := runPowerHelper("ryoku-power-cutover", "wait-lock-clients"); err != nil {
			progress.fail(err)
			return err
		}
	}
	pauseConfigAutoreload()
	if err := Materialize(); err != nil {
		if shellExpected {
			_ = reloadConfig()
			_ = startShell()
			if ready, readyErr := waitForShellReady(true); readyErr == nil && ready {
				if activateErr := activatePowerPolicy(); activateErr != nil {
					fmt.Fprintf(os.Stderr, i18n.Tf("warning: could not restore the lid policy: %v\n", activateErr))
				}
			} else if readyErr != nil {
				fmt.Fprintf(os.Stderr, i18n.Tf("warning: could not restore the shell sleep guard: %v\n", readyErr))
			}
			restartWallpaper(false)
		}
		progress.fail(err)
		return err
	}
	if err := regenerateConfig(); err != nil {
		progress.logf(i18n.Tf("could not re-author the compositor settings: %v", err))
	}
	if graphicalPresent {
		if err := runPowerHelper("ryoku-power-cutover", "session-bind"); err != nil {
			if shellExpected {
				_ = reloadConfig()
				_ = startShell()
				restartWallpaper(wallpaperWasActive)
			}
			progress.fail(err)
			return err
		}
	}

	if shellExpected {
		progress.at("reload")
		progress.logf(i18n.T("Reloading the desktop"))
		// One clean reload picks up the new config and restores auto-reload. The
		// lid owner comes back as soon as the new shell answers sleep-ready;
		// long-running reindex, doctor, and snapshot work starts only afterward.
		if err := reloadConfig(); err != nil {
			restartWallpaper(wallpaperWasActive)
			progress.fail(err)
			return fmt.Errorf("reload compositor power bindings: %w", err)
		}
		if err := startShell(); err != nil {
			restartWallpaper(wallpaperWasActive)
			progress.fail(err)
			return err
		}
		ready, err := waitForShellReady(true)
		if err != nil {
			restartWallpaper(wallpaperWasActive)
			progress.fail(err)
			return err
		}
		if ready {
			if err := activatePowerPolicy(); err != nil {
				restartWallpaper(wallpaperWasActive)
				progress.fail(err)
				return err
			}
		}
		restartWallpaper(wallpaperWasActive)
	}
	// Either the foreground shell owns the normal sleep block, or every
	// inactive graphical session is qylock-secured and observed while logind
	// owns the sessionless lid fallback. Release only after proving that state.
	if shellExpected {
		if err := runPowerHelper("ryoku-power-cutover", "session-check"); err != nil {
			progress.fail(err)
			return err
		}
	}
	if graphicalPresent {
		if err := runPowerHelper("ryoku-power-cutover", "generation-guard-stop"); err != nil {
			progress.fail(err)
			return err
		}
		generationGuardHeld = false
	}
	if err := cutoverGuard.Release(); err != nil {
		progress.fail(err)
		return err
	}
	_ = powerCutoverLock.Close()
	powerCutoverLock = nil
	rashinReindex()
	prowlRefresh()

	progress.at("doctor")
	runFreshDoctor()

	progress.at("finalize")
	snapperPost(pre, "ryoku-update")
	progress.logf(i18n.T("Update complete"))
	return finishRun()
}

func acquirePowerCutoverLock() (*os.File, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	lock, err := os.OpenFile(filepath.Join(dir, "ryoku-power-cutover.lock"),
		os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open power cutover lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("acquire power cutover lock: %w", err)
	}
	return lock, nil
}

const updateSleepGuardUnit = "ryoku-power-cutover-guard.service"
const packagePowerCutoverMarker = "/var/lib/ryoku/power-cutover-hook-active"

func needsPackagePowerCutover(markerPresent, rootGuardActive bool) bool {
	return !markerPresent || rootGuardActive
}

func ensurePackagePowerCutover() error {
	markerPresent := false
	if _, err := os.Stat(packagePowerCutoverMarker); err == nil {
		markerPresent = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect package power cutover state: %w", err)
	}
	rootGuardActive := exec.Command(
		"sudo", "-n", "systemctl", "is-active", "--quiet", updateSleepGuardUnit,
	).Run() == nil
	if !needsPackagePowerCutover(markerPresent, rootGuardActive) {
		return nil
	}
	const helper = "/usr/bin/ryoku-power-cutover"
	if _, err := os.Stat(helper); err != nil {
		if os.IsNotExist(err) && !rootGuardActive {
			return nil
		}
		return fmt.Errorf("inspect package power cutover helper: %w", err)
	}
	if err := sys.Sudo(helper, "package"); err != nil {
		return fmt.Errorf("adopt packaged power policy in live sessions: %w", err)
	}
	return nil
}

type login1Inhibitor struct {
	What string
	Who  string
	Why  string
	Mode string
	UID  uint32
	PID  uint32
}

type updateSleepGuard struct {
	conn *dbus.Conn
}

func hasUpdateSleepGuard(inhibitors []login1Inhibitor, uid uint32) bool {
	for _, inhibitor := range inhibitors {
		if inhibitor.UID != uid || inhibitor.Who != "ryoku-session-cutover" ||
			inhibitor.Mode != "block" {
			continue
		}
		for _, what := range strings.Split(inhibitor.What, ":") {
			if what == "sleep" {
				return true
			}
		}
	}
	return false
}

func acquireUpdateSleepGuard() (*updateSleepGuard, error) {
	for _, command := range []string{"systemctl", "systemd-run"} {
		if _, err := exec.LookPath(command); err != nil {
			return nil, fmt.Errorf("%s is required for the update sleep guard", command)
		}
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect login1 for update sleep guard: %w", err)
	}
	guard := &updateSleepGuard{conn: conn}
	active := exec.Command("systemctl", "--user", "is-active", "--quiet", updateSleepGuardUnit).Run() == nil
	if !active {
		_ = exec.Command("systemctl", "--user", "reset-failed", updateSleepGuardUnit).Run()
		output, runErr := exec.Command(
			"systemd-run", "--user", "--quiet", "--collect", "--unit="+updateSleepGuardUnit,
			"--property=Type=exec", "--property=TimeoutStopSec=5s",
			"/usr/bin/systemd-inhibit", "--what=sleep", "--mode=block",
			"--who=ryoku-session-cutover",
			"--why=keep the desktop awake while suspend owners are replaced",
			"/usr/bin/sleep", "infinity",
		).CombinedOutput()
		if runErr != nil &&
			exec.Command("systemctl", "--user", "is-active", "--quiet", updateSleepGuardUnit).Run() != nil {
			guard.Disconnect()
			return nil, fmt.Errorf("start durable update sleep guard: %w: %s", runErr, strings.TrimSpace(string(output)))
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var inhibitors []login1Inhibitor
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = conn.Object(
			"org.freedesktop.login1",
			dbus.ObjectPath("/org/freedesktop/login1"),
		).CallWithContext(
			ctx,
			"org.freedesktop.login1.Manager.ListInhibitors",
			0,
		).Store(&inhibitors)
		cancel()
		if err == nil && hasUpdateSleepGuard(inhibitors, uint32(os.Getuid())) {
			return guard, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = exec.Command("systemctl", "--user", "stop", updateSleepGuardUnit).Run()
	guard.Disconnect()
	if err != nil {
		return nil, fmt.Errorf("verify durable update sleep guard: %w", err)
	}
	return nil, fmt.Errorf("durable update sleep guard did not acquire a login1 block")
}

func (g *updateSleepGuard) Release() error {
	if g == nil {
		return nil
	}
	if exec.Command("systemctl", "--user", "is-active", "--quiet", updateSleepGuardUnit).Run() == nil {
		if output, err := exec.Command("systemctl", "--user", "stop", updateSleepGuardUnit).CombinedOutput(); err != nil {
			return fmt.Errorf("release durable update sleep guard: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func (g *updateSleepGuard) Disconnect() {
	if g != nil && g.conn != nil {
		_ = g.conn.Close()
		g.conn = nil
	}
}

// preparePowerPolicyUpdate removes pre-upgrade suspend owners before the shell
// is quiesced. hypridle.conf is generated state outside materialize, while a
// running shell-script daemon keeps executing the old file after replacement.
func preparePowerPolicyUpdate() error {
	_ = exec.Command("systemctl", "--user", "stop",
		"ryoku-clamshell.service", "ryoku-idle.service").Run()
	if err := runPowerHelper("ryoku-clamshell", "stop"); err != nil {
		return fmt.Errorf("stop previous lid policy: %w", err)
	}
	if err := runPowerHelper("ryoku-idle", "stop"); err != nil {
		return fmt.Errorf("stop previous idle policy: %w", err)
	}
	return nil
}

func hasStatusLine(status, want string) bool {
	for _, line := range strings.Split(status, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// activatePowerPolicy starts both policy owners as session services only after
// the new shell can serve its secure suspend transaction.
func activatePowerPolicy() error {
	if output, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("reload user services: %w: %s", err, strings.TrimSpace(string(output)))
	}
	_ = exec.Command("systemctl", "--user", "reset-failed",
		"ryoku-idle.service", "ryoku-clamshell.service").Run()
	if output, err := exec.Command("systemctl", "--user", "restart", "ryoku-idle.service").CombinedOutput(); err != nil {
		return fmt.Errorf("start idle policy service: %w: %s", err, strings.TrimSpace(string(output)))
	}
	idleReady := false
	for range 40 {
		output, _ := exec.Command(pkgBin("ryoku-idle"), "status").Output()
		status := string(output)
		if hasStatusLine(status, "idle=inactive") ||
			(hasStatusLine(status, "idle=active") &&
				hasStatusLine(status, "running=yes") &&
				exec.Command("systemctl", "--user", "is-active", "--quiet", "ryoku-idle.service").Run() == nil) {
			idleReady = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !idleReady {
		return fmt.Errorf("idle policy service did not acquire its hypridle owner")
	}

	if output, err := exec.Command("systemctl", "--user", "restart", "ryoku-clamshell.service").CombinedOutput(); err != nil {
		return fmt.Errorf("start lid policy service: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if exec.Command(pkgBin("ryoku-clamshell"), "is-laptop").Run() != nil {
		return nil
	}
	for range 40 {
		output, _ := exec.Command(pkgBin("ryoku-clamshell"), "status").Output()
		status := string(output)
		if hasStatusLine(status, "owner-monitor=ready") &&
			hasStatusLine(status, "inhibitor=held") &&
			exec.Command("systemctl", "--user", "is-active", "--quiet", "ryoku-clamshell.service").Run() == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("lid policy service did not acquire its login1 inhibitor")
}

func runPowerHelper(name, arg string) error {
	if !sys.Has(name) {
		return fmt.Errorf("%s is not installed", name)
	}
	if err := exec.Command(pkgBin(name), arg).Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, arg, err)
	}
	return nil
}

func reloadLogindLidPolicy() error {
	if err := exec.Command("sudo", "systemctl", "reload", "systemd-logind").Run(); err != nil {
		return fmt.Errorf("reload logind lid policy: %w", err)
	}
	return nil
}

// rashinReindex refreshes the agent-OS vault after an update so agents see
// the new system immediately. Best effort: rashin is optional and a failed
// index never blocks an update.
func rashinReindex() {
	if !sys.Has("ryoku-rashin") {
		return
	}
	fmt.Println(i18n.T("==> Reindexing the Rashin vault"))
	if err := sys.Run(pkgBin("ryoku-rashin"), "index"); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("warning: rashin reindex failed: %v\n"), err)
	}
	// Re-wire every agent after the index: the shipped ryoku skill and Prowl's
	// skills may have moved or grown with this update, and wire is idempotent.
	if err := sys.Run(pkgBin("ryoku-rashin"), "wire"); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("warning: rashin wire failed: %v\n"), err)
	}
}

// prowlRefresh keeps a dev box's prowl current after an update. A packaged box
// already got it through its package transaction, so this runs `<bin> update`
// only when the binary is on PATH but not owned by a package (a dev or manual
// install). The CLI was renamed prowl-agent -> prowl; prefer the new name and
// fall back to the old one upstream still ships. Best effort, one line either way.
func prowlRefresh() {
	path, err := exec.LookPath("prowl")
	if err != nil {
		if path, err = exec.LookPath("prowl-agent"); err != nil {
			return
		}
	}
	switch prowlDecide(true, prowlPackageOwned(path)) {
	case prowlManaged:
		fmt.Println(i18n.T("==> prowl is managed by the system packages"))
	case prowlSelfUpdate:
		fmt.Println(i18n.T("==> Updating prowl"))
		if err := sys.Run(path, "update"); err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("warning: prowl update failed: %v\n"), err)
		}
	}
}

// prowlPackageOwned reports whether path belongs to an installed package;
// returns non-zero for a file no package owns (a dev install).
func prowlPackageOwned(path string) bool {
	return exec.Command("rpm", "-qf", path).Run() == nil
}

// prowlAction is what an update should do about prowl.
type prowlAction int

const (
	prowlNoop       prowlAction = iota // not installed; nothing to do
	prowlManaged                       // package-owned; the system upgrade covered it
	prowlSelfUpdate                    // dev install; run `<bin> update`
)

// prowlDecide is the pure update decision, split out so it is unit-testable
// without a live PATH or rpm.
func prowlDecide(onPath, packageOwned bool) prowlAction {
	if !onPath {
		return prowlNoop
	}
	if packageOwned {
		return prowlManaged
	}
	return prowlSelfUpdate
}

// runFreshDoctor runs `ryoku doctor` after the new binary lands, so the
// reconcilers shipped in this release run inside the same update. same
// command users run by hand; calling it here keeps doctor one thing instead
// of a copy baked into update. best-effort: a finding never fails update.
func runFreshDoctor() {
	fmt.Println(i18n.T("==> Running doctor"))
	// pkgBin, not PATH: on a box with ~/.local/bin residue the bare name is the
	// STALE CLI, whose doctor predates the reconcilers this release ships --
	// including the residue scan that would clear that very shadow.
	_ = sys.Run(pkgBin("ryoku"), "doctor")
}

// Rollback lists the system snapshots, and `ryoku rollback <id>` guides a
// whole-system restore of one from the boot menu. COPR keeps a limited package
// history and no frozen releases, so there is no rollback to a release tag.
//
// The restore is a boot-menu restore, not a live one: Fedora pins the root
// subvolume on the kernel cmdline and in fstab (rootflags=subvol=root), and
// `snapper rollback` cannot serve that layout, since it works by flipping the
// btrfs default subvolume, which a pinned subvol= simply ignores. So the
// command teaches the GRUB snapshot menu instead of running a snapper command
// that cannot restore the system.
func Rollback(args []string) error {
	for _, arg := range args {
		if arg == "--to" || strings.HasPrefix(arg, "--to=") {
			return fmt.Errorf(i18n.T("COPR does not archive frozen Ryoku releases; tag rollback is unavailable. DNF can downgrade only to versions still retained by COPR"))
		}
	}
	if len(args) > 0 {
		return restoreGuide(args[0])
	}
	fmt.Println(i18n.T("COPR keeps a limited package history; tag-based Ryoku rollback is unavailable."))
	fmt.Println(i18n.T("Available system snapshots:"))
	return Snapshots()
}

// restoreGuide is `ryoku rollback <id>`: the boot-menu restore, step by step,
// naming the snapshot when snapper can describe it.
func restoreGuide(id string) error {
	label := id
	if rows, err := snapshotRows(); err == nil {
		for _, r := range rows {
			if r.number == id {
				label = fmt.Sprintf("%s  (%s, %s %s)", id, shortSnapDate(r.date), r.kind, orDash(r.description))
				break
			}
		}
	}
	fmt.Printf(i18n.T("Restoring snapshot %s\n\n"), label)
	return grubRestoreGuide(id)
}

// grubRestoreGuide is the Fedora restore: the "Ryoku snapshots" GRUB submenu
// that `ryoku boot-menu sync` writes. With a terminal it offers to pick the
// restore entry for the next boot only (grub2-reboot), so nothing changes on
// disk until the machine restarts, and the boot after that is normal again.
func grubRestoreGuide(id string) error {
	if n, err := strconv.Atoi(id); err != nil || n <= 0 {
		return fmt.Errorf(i18n.T("%q is not a snapshot number (see `ryoku snapshots`)"), id)
	}
	if !snapshotsInBootMenu() {
		fmt.Println(i18n.T("The snapshot boot menu is not set up on this machine (ryoku-desktop ships it)."))
		return nil
	}
	restore := snapshotEntryID(id, "restore")
	if sys.Sudo("grep", "-q", "--", "--id "+strings.TrimPrefix(restore, snapshotMenuID+">")+" ", snapshotMenuCfg()) != nil {
		fmt.Printf(i18n.T("Snapshot %s is not in the boot menu. `ryoku snapshots` lists the ones that are;\n"), id)
		fmt.Println(i18n.T("a snapshot whose kernel was removed before its copy was made cannot be booted."))
		return nil
	}
	fmt.Println(i18n.T("The GRUB menu lists it under \"Ryoku snapshots\":"))
	fmt.Println(i18n.T("  look (read-only)  boots it with changes kept in RAM; nothing on disk changes"))
	fmt.Println(i18n.T("  restore           makes it the system again and boots it; the current system"))
	fmt.Println(i18n.T("                    is kept on the disk for two weeks as root.broken-<date>"))
	fmt.Println(i18n.T("Your home folder is not part of the snapshot and stays as it is."))
	if !sys.StdinIsTTY() {
		return nil
	}
	fmt.Printf(i18n.T("\nRestore snapshot %s on the next boot? [y/N] "), id)
	var resp string
	_, _ = fmt.Scanln(&resp)
	if r := strings.ToLower(strings.TrimSpace(resp)); r != "y" && r != "yes" {
		return nil
	}
	if err := sys.Sudo("grub2-reboot", restore); err != nil {
		return fmt.Errorf(i18n.T("setting the next boot failed: %v"), err)
	}
	fmt.Println(i18n.T("Done. Reboot to restore it; the boot after that uses the normal menu again."))
	return nil
}

// Snapshots prints the snapshot table (the SNAPSHOTS block of `ryoku rollback`).
func Snapshots() error {
	if !sys.Has("snapper") {
		return fmt.Errorf(i18n.T("snapper is not installed"))
	}
	if !sys.Exists("/etc/snapper/configs/root") {
		fmt.Println(i18n.T("  not configured on this machine; `ryoku doctor` enables them"))
		return nil
	}
	rows, err := snapshotRows()
	if err != nil {
		// fall back to snapper's own table rather than showing nothing.
		return sys.Sudo("snapper", "-c", snapperConfig, "list")
	}
	printSnapshotTable(rows)
	return nil
}

// snapshotRows lists the root snapshots, parsed, without ever prompting. It runs
// snapper unprivileged first -- which succeeds once the config grants the user
// read access (ALLOW_USERS + SYNC_ACL, snapper's own mechanism, which `ryoku
// doctor` sets up) -- then a cached-credential `sudo -n`, which never prompts,
// so it needs no tty and cannot trip pam_faillock. Only a real CSV listing
// counts as success: snapper prints "No permissions." to stderr and still exits
// 0 on a denied read, so the exit code alone would read an empty stdout as an
// empty store. The error is returned when neither attempt yields a listing, so a
// caller can tell a failed read from a genuinely empty one instead of both
// looking like zero.
func snapshotRows() ([]snapshotRow, error) {
	args := []string{"-c", snapperConfig, "--csvout", "list",
		"--columns", "number,type,date,description,cleanup"}
	if out, err := sys.RunOut("snapper", args...); err == nil && isSnapshotCSV(out) {
		return parseSnapshotRows(out), nil
	}
	if out, err := sys.RunOut("sudo", append([]string{"-n", "snapper"}, args...)...); err == nil && isSnapshotCSV(out) {
		return parseSnapshotRows(out), nil
	}
	return nil, fmt.Errorf(i18n.T("snapper root snapshots are not readable (grant access with `ryoku doctor` or prime sudo)"))
}

// isSnapshotCSV reports whether out is a real `snapper --csvout list` listing:
// its first non-empty line is the column header. A denied read ("No
// permissions." on stderr, empty stdout, exit 0) fails this, so it is never
// mistaken for an empty store.
func isSnapshotCSV(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return strings.HasPrefix(s, "number,")
		}
	}
	return false
}

// snapshotRow is one parsed line of `snapper --csvout list`.
type snapshotRow struct {
	number      string
	kind        string // pre | post | single
	date        string
	description string
	cleanup     string
}

// parseSnapshotRows parses the CSV list, dropping the header and base snapshot 0.
func parseSnapshotRows(out string) []snapshotRow {
	var rows []snapshotRow
	r := csv.NewReader(strings.NewReader(out))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return rows
	}
	for _, rec := range records {
		if len(rec) < 5 {
			continue
		}
		num := strings.TrimSpace(rec[0])
		if num == "" || num == "number" || num == "0" {
			continue
		}
		rows = append(rows, snapshotRow{
			number:      num,
			kind:        strings.TrimSpace(rec[1]),
			date:        strings.TrimSpace(rec[2]),
			description: strings.TrimSpace(rec[3]),
			cleanup:     strings.TrimSpace(rec[4]),
		})
	}
	return rows
}

// printSnapshotTable shows the snapshots (newest last, the way snapper counts)
// plus a count, free space, and whether the boot menu lists them.
func printSnapshotTable(rows []snapshotRow) {
	if len(rows) == 0 {
		fmt.Println(i18n.T("  none yet; `ryoku update` takes one before and after each update"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	for _, s := range rows {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", s.number, shortSnapDate(s.date), s.kind, orDash(s.description))
	}
	w.Flush()
	fmt.Println()
	summary := fmt.Sprintf(i18n.T("  %d snapshots"), len(rows))
	if free := rootFree(); free != "" {
		summary += ", " + free + i18n.T(" free on /")
	}
	switch {
	case snapshotsInBootMenu():
		summary += i18n.T(", listed in the boot menu")
	case sys.Exists(snapshotMenuTool):
		summary += i18n.T(", NOT in the boot menu (journalctl -u ryoku-snapshot-menu says why)")
	default:
		summary += i18n.T(", NOT in the boot menu (ryoku-desktop ships it; run ryoku update)")
	}
	fmt.Println(summary)
	fmt.Println(i18n.T("  ryoku rollback <#>          shows how to boot into a snapshot and restore it"))
}

// shortSnapDate trims snapper's "2026-09-03 22:10:06" to the minute.
func shortSnapDate(d string) string {
	if len(d) >= 16 {
		return d[:16]
	}
	return d
}

// snapshotsInBootMenu reports whether snapshots reach the GRUB snapshot
// submenu.
func snapshotsInBootMenu() bool {
	return sys.Exists(snapshotMenuTool) && snapshotsListed()
}

// rootFree is the human-readable free space on /, or "" when statfs fails.
func rootFree() string {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return ""
	}
	return humanBytes(st.Bavail * uint64(st.Bsize))
}

func Status(args []string) error {
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		}
	}
	r := buildStatus()

	if jsonOut {
		b, _ := json.Marshal(r)
		fmt.Println(string(b))
		return nil
	}

	fmt.Printf(i18n.T("config base:   %s\n"), sys.BaseConfigDir())
	fmt.Printf(i18n.T("channel:       %s\n"), orDash(r.Channel))
	if r.Release != "" {
		fmt.Printf(i18n.T("release:       %s%s\n"), withSpace(r.ReleaseName), r.Release)
	}
	fmt.Printf(i18n.T("installed:     %s\n"), orDash(r.Installed))
	if r.Available {
		fmt.Printf(i18n.T("available:     %s\n"), orDash(r.Latest))
		fmt.Printf(i18n.T("behind:        %d commit(s)\n"), r.Behind)
	} else {
		fmt.Println(i18n.T("behind:        up to date"))
	}
	// the other lane, named as such: `ryoku update` never moves these, so a
	// box that reads "up to date" above can still owe its distribution a
	// kernel.
	if r.SystemPending > 0 {
		fmt.Printf(i18n.T("system:        %d package(s) waiting (sudo %s)\n"), r.SystemPending, systemUpgradeCmd())
	} else {
		fmt.Println(i18n.T("system:        up to date"))
	}
	// Three distinct states, never conflated: no root config at all (doctor
	// restores it), a config we could not read (a bare "0" here used to look
	// like a real empty store -- the opposite meaning), and the real count.
	switch {
	case !sys.Exists("/etc/snapper/configs/root"):
		fmt.Println(i18n.T("snapshots:     not configured (run ryoku doctor)"))
	case !r.SnapshotsKnown:
		fmt.Println(i18n.T("snapshots:     unavailable (grant read access: run ryoku doctor)"))
	default:
		fmt.Printf(i18n.T("snapshots:     %d\n"), r.Snapshots)
	}
	return nil
}

// statusReport = what the Hub and the update island read from
// `ryoku status --json`. installed + available versions, how far behind,
// per-item list. from the git update channel on a Ryoku checkout (the live
// mirror), else from the RyokuCOPR repo.
type statusReport struct {
	Installed string       `json:"installedVersion"`
	Latest    string       `json:"latestVersion"`
	Available bool         `json:"available"`
	Behind    int          `json:"pendingUpdates"`
	Updates   []updateItem `json:"updates"`
	Recent    []updateItem `json:"recent"`
	Channel   string       `json:"channel"`
	Snapshots int          `json:"snapshots"`
	// SnapshotsKnown is false when the count could not be read (no snapper access,
	// cold sudo) rather than genuinely zero, so a consumer never reads a failed
	// query as "no safety net". A bare `snapshots: 0` used to conflate the two.
	SnapshotsKnown bool `json:"snapshotsKnown"`
	// Packages is the OTHER lane: what Fedora has waiting, kernel included.
	// `sudo dnf upgrade` takes those; `ryoku update`
	// deliberately does not, so Available stays about the Ryoku lane alone and
	// the update button never offers a run that would move none of them.
	Packages      []updateItem `json:"packages"`
	SystemPending int          `json:"systemUpdates"`
	// packaged boxes: the release this box runs (/etc/ryoku-release).
	Release     string `json:"release,omitempty"`
	ReleaseName string `json:"releaseName,omitempty"`
}

// withSpace is a release name as a prefix: "Onogoro " or "" when unnamed.
func withSpace(name string) string {
	if name == "" {
		return ""
	}
	return name + " "
}

// buildStatus is the full Updates report: the Ryoku channel (baseStatus) plus,
// listed separately, what the distribution has waiting. baseStatus prefers the
// git update channel (a checkout tracking main); a packaged install reads the
// running and available commits from the [ryoku] repo's package versions and
// lists what is incoming via the public GitHub compare API, so the Hub's list
// is the same commit subjects a dev box shows, not bare package names.
func buildStatus() statusReport {
	r := baseStatus()
	// The user's lane, check-only. It never sets Available: `ryoku update`
	// would not move any of it, and a button that promises otherwise is how a
	// box ends up looking updated while its kernel never moves.
	r.Packages = pendingUpdates()
	r.SystemPending = len(r.Packages)
	return r
}

// baseStatus builds the Ryoku-channel report: the git update channel on a
// checkout, else the [ryoku] repo package versions on a packaged install.
func baseStatus() statusReport {
	if r, ok := channelStatus(); ok {
		// a checkout has no release, but it runs a named line (CODENAME)
		r.ReleaseName = ReleaseName()
		return r
	}
	installed := sys.InstalledVersion()
	latest := latestAvailable("ryoku-desktop", false)
	// the check runs as the user, whose dnf metadata cache is not the one
	// `ryoku update` (root) just refreshed; a repo older than the running
	// package means that cache is stale, so ask the repo again.
	if latest != "" && installed != "" && versionCompare(latest, installed) < 0 {
		latest = latestAvailable("ryoku-desktop", true)
	}
	for _, u := range pendingUpdates() {
		if u.Name == "ryoku-desktop" {
			latest = u.New
		}
	}
	return packagedStatus(installed, latest)
}

// packagedStatus builds the report for a packaged install from the running and
// available package versions. The GitHub lookups are best-effort and stubbable
// (RYOKU_GITHUB_API), so the sha/compare/recent branching is unit-testable
// without a package manager.
func packagedStatus(installed, latest string) statusReport {
	// a repo view older than the running package is never an update.
	if latest != "" && installed != "" && versionCompare(latest, installed) < 0 {
		latest = installed
	}
	installedSha := shortCommit(installed)
	latestSha := shortCommit(latest)

	snaps, snapsKnown := snapshotCount()
	r := statusReport{
		Installed:      installedSha,
		Latest:         latestSha,
		Updates:        []updateItem{}, // non-nil, so a current box marshals [] like the git path
		Recent:         []updateItem{}, // non-nil, so the JSON stays stable when nothing is fetched
		Channel:        ryokuChannel(),
		Snapshots:      snaps,
		SnapshotsKnown: snapsKnown,
		Release:        sys.ReadRelease().Release,
		ReleaseName:    ReleaseName(),
	}
	// up to date: nothing incoming, but list the recent history the installed
	// version contains (best-effort, newest-first) so the Hub's Updates page
	// still shows meaningful content instead of a blank section.
	if installedSha != "" && installedSha == latestSha {
		if rec := recentCommits(installedSha); len(rec) > 0 {
			r.Recent = rec
		}
		return r
	}
	// the [ryoku] repo isn't synced yet: nothing to compare.
	if installedSha == "" || latestSha == "" {
		return r
	}
	r.Available = true
	if ups, behind := incomingCommits(installedSha, latestSha); len(ups) > 0 {
		r.Updates = ups
		r.Behind = behind
	} else {
		// compare unreachable (offline / rate-limited): still surface the
		// pending Ryoku bump so the section isn't empty and available holds.
		r.Updates = []updateItem{{Name: "ryoku-desktop", Old: installed, New: latest}}
		r.Behind = 1
	}
	return r
}

// shortCommit pulls the abbreviated commit hash out of a packaged version
// shaped <core>.r<count>.g<sha>(-pkgrel) (what the repo build embeds), so
// Hub and CLI can show the exact commit a packaged box runs. no gNNNN token
// (a hand-pinned 0.1.0-3, say) -> input comes back unchanged.
func shortCommit(ver string) string {
	for _, tok := range strings.FieldsFunc(ver, func(r rune) bool { return r == '.' || r == '-' || r == '^' || r == '_' }) {
		if len(tok) >= 10 && strings.HasPrefix(tok, "git") && isHex(tok[3:]) {
			return tok[3:]
		}
		if len(tok) >= 8 && tok[0] == 'g' && isHex(tok[1:]) {
			return tok[1:]
		}
	}
	return ver
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return s != ""
}

// latestAvailable: version of pkg in the RyokuCOPR repo, or "" when the repo
// isn't configured. refresh makes dnf refetch the repo metadata instead of
// trusting its cache.
func latestAvailable(pkg string, refresh bool) string {
	// --latest-limit=1: a COPR repo keeps several builds in its repodata and
	// repoquery lists every one; dnf5's --qf adds no newline, so the versions
	// came back glued into one string that never matches the installed
	// version, and a fully updated box reported itself behind.
	args := []string{"repoquery", "--repo=" + sys.RPMRepoName,
		"--latest-limit=1", "--qf", "%{VERSION}-%{RELEASE}", pkg}
	if refresh {
		args = append(args, "--refresh")
	}
	out, err := sys.RunOut("dnf", args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// updateItem = one row in the update list. dnf -> a package (name,
// old -> new). git channel -> a commit (subject in Name, short hash in New).
type updateItem struct {
	Name string `json:"name"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// pendingUpdates: packages with a newer version available, via dnf
// check-update. Empty when the system is current.
func pendingUpdates() []updateItem {
	ups := []updateItem{}
	// cap the check: check-update syncs metadata over the network and the
	// update island polls this, so it MUST never hang status. generous so a
	// slow sync still finishes.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "dnf", "check-update").Output()
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		if name := checkUpdateName(f[0]); name != "" {
			ups = append(ups, updateItem{Name: name, New: f[1]})
		}
	}
	return ups
}

// checkUpdateName strips the arch from a `dnf check-update` name.arch column.
// The arch never holds a dot but a name can (python3.13-libs), so the cut is
// at the last one; a column with no dot is not a package row.
func checkUpdateName(nameArch string) string {
	i := strings.LastIndexByte(nameArch, '.')
	if i <= 0 {
		return ""
	}
	return nameArch[:i]
}

// snapshotCount returns how many snapshots the root store holds and whether it
// could be read at all. A failed read -- no snapper access and no cached sudo --
// returns (0, false), distinct from a genuinely empty store (0, true), so
// `ryoku status` can say "unavailable" instead of a bare "0" that means the
// opposite. snapshotRows never prompts, so this is safe from the GUI's
// terminal-less poll (a prompt with no tty trips pam_faillock and can lock the
// account out of sudo -- found the loud way).
func snapshotCount() (int, bool) {
	if !sys.Has("snapper") {
		return 0, false
	}
	rows, err := snapshotRows()
	if err != nil {
		return 0, false
	}
	return len(rows), true
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Deploy = the DEV loop: build the Go binaries + plugin and materialize
// from a repo checkout. production installs never see this; they pull
// everything from the RyokuCOPR repo.
func Deploy(_ []string) error {
	repo := os.Getenv("RYOKU_REPO")
	if repo == "" {
		return fmt.Errorf(i18n.T("set RYOKU_REPO to a Ryoku checkout for `ryoku deploy`"))
	}
	script := filepath.Join(repo, "ryoku", "shell", "deploy.sh")
	if !sys.Exists(script) {
		return fmt.Errorf(i18n.T("not a Ryoku checkout (missing %s)"), script)
	}
	return sys.Run(script)
}

// --- snapper pre/post (best-effort) ----------------------------------------

// updateSnapshotMarker holds the pid of an update that owns the open snapper
// pair, so the dnf hook (system/snapshots/ryoku-dnf-snapshot) does not nest a
// second pair inside it. The pid survives the stage2 exec, and a dead pid
// frees the hook again.
const updateSnapshotMarker = "/run/ryoku/update-snapshot"

func snapperPre(desc string) string {
	if !sys.Has("snapper") {
		fmt.Fprintln(os.Stderr, i18n.T("note: snapper not installed; skipping pre-update snapshot"))
		return ""
	}
	// no root config -> the create below fails with an opaque
	// "config 'root' does not exist". point the user at the fix.
	if !sys.Exists("/etc/snapper/configs/root") {
		fmt.Fprintln(os.Stderr, i18n.T("note: snapshot skipped, snapper root config missing; run 'ryoku doctor' to enable snapshots"))
		return ""
	}
	out, err := sys.RunOut("sudo", "snapper", "-c", snapperConfig, "create",
		"-t", "pre", "-c", "number", "-p", "-d", desc)
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("note: pre-update snapshot skipped: %v\n"), err)
		return ""
	}
	_ = sys.WriteRootFile(updateSnapshotMarker, strconv.Itoa(os.Getpid())+"\n", "0644")
	return strings.TrimSpace(out)
}

func snapperPost(pre, desc string) {
	if pre == "" {
		return
	}
	_ = sys.Sudo("snapper", "-c", snapperConfig, "create",
		"-t", "post", "--pre-number", pre, "-c", "number", "-d", desc)
	_ = sys.Sudo("rm", "-f", updateSnapshotMarker)
	// Prune here, on the path every update takes: snapper-cleanup.timer can be
	// off (doctor turns it back on), and without this the pile grows unbounded.
	// best-effort.
	_ = sys.Sudo("snapper", "-c", snapperConfig, "cleanup", "number")
}

// pauseConfigAutoreload stops the compositor reloading its config mid-swap, so a
// half-written tree is never observed.
func pauseConfigAutoreload() {
	c := wm.Open()
	if c.Detection().Live {
		_ = c.Act(wm.ActionConfigAutoreload, "off")
	}
}

// reloadConfig applies materialized config in one clean pass where the provider
// exposes an imperative reload. Niri watches its KDL and deliberately has no
// reload action, so unsupported means the file watcher owns this step. Any
// other live provider failure is fatal while the update sleep guard is held:
// starting the lid inhibitor without its compositor event producer would turn
// every close into a no-op.
func configReloadResult(err error) error {
	if errors.Is(err, wm.ErrUnsupported) {
		return nil
	}
	return err
}

func reloadConfig() error {
	c := wm.Open()
	if !c.Detection().Live {
		return nil
	}
	return configReloadResult(c.Act(wm.ActionConfigReload))
}

// regenerateConfig re-authors the live provider's generated config from the
// store. Those files are a pure function of the store and the provider that
// wrote them, so a provider update that emits a block differently (niri's
// border only draws with an explicit on flag) has to rewrite them, or the old
// output stays in force until a Hub edit happens to apply again. A box with
// no store yet is left to the first Hub save.
func regenerateConfig() error {
	store := filepath.Join(sys.ConfigHome(), "ryoku", "desktop.json")
	if !sys.Exists(store) {
		return nil
	}
	_, err := wm.Open().Apply(store)
	return err
}

// pkgBin resolves a Ryoku binary an update drives. The packaged /usr/bin copy
// is preferred over a bare PATH lookup: a past `ryoku recovery` or dev deploy
// leaves builds in ~/.local/bin that outrank /usr/bin, and driving those runs
// stale code inside the very update meant to supersede it -- a stale daemon
// restarted over the new QML replays an old supervisor against a one-shot
// switcher (the beta-17 switcher-reopen loop), and a stale doctor predates the
// reconcilers this release ships. A box without the package (a pure checkout)
// falls back to PATH, where the just-deployed build is the right one.
func pkgBin(name string) string {
	if p := "/usr/bin/" + name; sys.Exists(p) {
		return p
	}
	return name
}

// stopShell quiesces the desktop for a config swap: ask the daemon to quit,
// wait for it to go, then drop orphaned surfaces still holding a config's
// single-instance lock (one survivor kills the fresh daemon's components).
// The component list mirrors shell/ipc/daemon.go; "plugins" and "wallpaper"
// are retired resident components, still reaped on boxes whose live daemon
// predates their removal.
func stopShell() error {
	if !sys.Has("ryoku-shell") {
		return nil
	}
	// Under systemd the unit would respawn the daemon two seconds after the
	// quit below and the update would race its own quiesce. Stopping the unit
	// is a no-op where it does not exist yet.
	_ = exec.Command("systemctl", "--user", "stop", "ryoku-shell").Run()
	shell := pkgBin("ryoku-shell")
	_ = exec.Command(shell, "quit").Run()
	stopped := false
	for range 50 {
		if exec.Command(shell, "ping").Run() != nil {
			stopped = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !stopped {
		return fmt.Errorf("pre-upgrade ryoku-shell did not stop")
	}
	// the pattern is anchored: quickshell is a general-purpose tool, and a bare
	// "qs -c wallpaper" would also match a user's own longer config name
	// ("qs -c wallpaperclock"). the daemon always spawns the config name as the
	// final argv element.
	for _, c := range []string{"pill", "launcher", "visualizer", "widgets", "overview", "plugins", "wallpaper"} {
		_ = exec.Command("pkill", "-f", "qs -c "+c+"($| )").Run()
	}
	// The video players outlive the daemon (spawned detached): kill the
	// current one so the restarted daemon relaunches it on the new binary, and
	// the legacy backends older releases shipped (mpvpaper, phonto) -- the new
	// daemon no longer knows their names, and an orphan left on the background
	// layer stacks above Ryogami's surface and swallows every static set after
	// the update.
	for _, p := range []string{"ryoku-livewall", "mpvpaper", "phonto"} {
		_ = exec.Command("pkill", "-x", p).Run()
	}
	// The Hub (Ryoku Settings) is a separate, session-resident quickshell
	// instance under its own /tmp/ryoku-hub.lock, not one of the daemon's
	// components, so the quit above never touches it. Left running it keeps the
	// old QML mapped, and a settings page this update just shipped only appears
	// after a relogin. Reap it so the next open loads the new pages; the lock is
	// advisory and frees with the process.
	_ = exec.Command("pkill", "-f", "hub/quickshell").Run()
	_ = exec.Command("pkill", "-f", "qs -c hub($| )").Run()
	time.Sleep(200 * time.Millisecond)
	return nil
}

// startShell brings the shell daemon back up, under systemd where the unit
// exists so it stays supervised, else detached on the current binary. The
// daemon-reload is what lets a unit materialize just laid down be found; a
// stale-cached user manager would otherwise report it unknown at start.
func startShell() error {
	if !sys.Has("ryoku-shell") {
		return nil
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if exec.Command("systemctl", "--user", "restart", "ryoku-shell").Run() == nil {
		return nil
	}
	if exec.Command("systemctl", "--user", "cat", "ryoku-shell.service").Run() == nil {
		return fmt.Errorf("ryoku-shell.service failed its guarded restart")
	}
	activator := pkgBin("ryoku-qylock-activate")
	if err := exec.Command(activator).Run(); err != nil {
		return fmt.Errorf("activate qylock before fallback shell start: %w", err)
	}
	cmd := exec.Command("setsid", pkgBin("ryoku-shell"), "daemon")
	logp := filepath.Join(sys.Xdg("XDG_STATE_HOME", ".local/state"), "ryoku-shell.log")
	_ = os.MkdirAll(filepath.Dir(logp), 0o755)
	if f, err := os.OpenFile(logp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close()
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start fallback ryoku-shell: %w", err)
	}
	return nil
}

type updaterLogin1Session struct {
	ID   string
	UID  uint32
	User string
	Seat string
	Path dbus.ObjectPath
}

func login1GraphicalUserProperties(properties map[string]dbus.Variant) bool {
	sessionType, typeOK := properties["Type"].Value().(string)
	sessionClass, classOK := properties["Class"].Value().(string)
	desktop, desktopOK := properties["Desktop"].Value().(string)
	desktop = strings.ToLower(desktop)
	supported := false
	for _, provider := range wm.Providers() {
		if desktop == strings.ToLower(provider) {
			supported = true
			break
		}
	}
	return typeOK && classOK && desktopOK && supported &&
		sessionType == "wayland" &&
		(sessionClass == "user" || sessionClass == "user-early")
}

func graphicalSessionState() (present bool, active bool, err error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return false, false, fmt.Errorf("connect login1 to inspect graphical sessions: %w", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	manager := conn.Object("org.freedesktop.login1", dbus.ObjectPath("/org/freedesktop/login1"))
	var sessions []updaterLogin1Session
	if err := manager.CallWithContext(
		ctx,
		"org.freedesktop.login1.Manager.ListSessions",
		0,
	).Store(&sessions); err != nil {
		return false, false, fmt.Errorf("list login1 sessions: %w", err)
	}
	var inspectErr error
	for _, session := range sessions {
		if session.UID != uint32(os.Getuid()) {
			continue
		}
		var properties map[string]dbus.Variant
		err := conn.Object("org.freedesktop.login1", session.Path).CallWithContext(
			ctx,
			"org.freedesktop.DBus.Properties.GetAll",
			0,
			"org.freedesktop.login1.Session",
		).Store(&properties)
		if err != nil {
			inspectErr = err
			continue
		}
		if !login1GraphicalUserProperties(properties) {
			continue
		}
		present = true
		isActive, ok := properties["Active"].Value().(bool)
		if !ok {
			inspectErr = fmt.Errorf("login1 session %s has no boolean Active property", session.ID)
			continue
		}
		if isActive {
			return true, true, nil
		}
	}
	if inspectErr != nil {
		return false, false, fmt.Errorf("inspect login1 sessions: %w", inspectErr)
	}
	return present, false, nil
}

func waitForShellReady(expected bool) (bool, error) {
	if !sys.Has("ryoku-shell") {
		if expected {
			return false, fmt.Errorf("ryoku-shell is not installed")
		}
		return false, nil
	}
	shell := pkgBin("ryoku-shell")
	if exec.Command(shell, "sleep-ready").Run() == nil {
		return true, nil
	}
	if !expected {
		return false, nil
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if exec.Command(shell, "sleep-ready").Run() == nil {
			return true, nil
		}
	}
	return false, fmt.Errorf("new ryoku-shell sleep guard did not become ready after restart")
}

// restartWallpaper brings a previously running wallpaper daemon onto the
// binary the update just installed. Ryogami is supervised beside the shell and
// owns the socket used by the wallpaper, picker and palette. A lifecycle rebind
// stops the whole session target, so try-restart alone would leave it down; in
// that case forceStart preserves the active state recorded before the rebind.
// Outside a graphical session a down daemon is not forced up.
func restartWallpaper(forceStart bool) {
	if !sys.Has("ryogami") {
		return
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	verb := "try-restart"
	if forceStart {
		_ = exec.Command("systemctl", "--user", "reset-failed", "ryogami.service").Run()
		verb = "restart"
	}
	if exec.Command("systemctl", "--user", verb, "ryogami.service").Run() == nil {
		return
	}
	// No unit yet on a box mid-cutover: drop the old process so the shell's
	// respawn picks up the installed binary.
	_ = exec.Command("pkill", "-x", "ryogami").Run()
}

func materializeStatePath() string {
	return filepath.Join(sys.Xdg("XDG_STATE_HOME", ".local/state"), "ryoku", "materialized")
}
