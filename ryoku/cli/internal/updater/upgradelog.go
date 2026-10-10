package updater

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ryoku-cli/internal/sys"
	i18n "ryoku-i18n"
)

// The dnf5 firehose (repository loading bars, the package table, one line per
// downloaded and installed package, scriptlet chatter) buries the handful of
// lines a person
// running `ryoku update` actually cares about: what is upgrading, how far along
// it is, and any real warning or error. renderUpgrade runs an upgrade command
// with its output piped and rebuilds it as a curated, styled view -- a phase
// header, a one-line package summary, an in-place spinner, and untouched
// surfacing of warnings and errors. It only runs on a real terminal (the caller
// keeps raw passthrough for pipes, logs, and --verbose), so nothing that scrapes
// dnf output breaks, and the command's exit status is passed straight back.

// verboseLog forces raw package-manager output instead of the curated view; set
// from `ryoku update --verbose` / `-v` for debugging a transaction.
var verboseLog bool

// dnf5 shapes, read from what it prints to a pipe (no live redraws: each bar
// is printed once, finished). testdata/dnf-*.txt are captured transcripts.
var (
	// "[ 4/83] Upgrading libgcc-0:16.2.1-2.fc4 100% | ..." (run phase) or
	// "[ 3/42] audit-libs-0:4.2.1-1.fc44.x86_6 100% | ..." (download).
	stepRe = regexp.MustCompile(`^\[\s*(\d+)/(\d+)\]\s+(.*)$`)
	// the finished bar after a label: " 100% |  24.2 MiB/s |   1.5 MiB |  00m00s"
	barTailRe = regexp.MustCompile(`\s+\d+%\s+\|.*$`)
	// "Package   Arch   Version   Repository   Size" opens the package table.
	tableHeadRe = regexp.MustCompile(`^Package\s+Arch\s+Version\s+Repository\s+Size$`)
	// " Upgrading:         11 packages" under "Transaction Summary:".
	summaryRe = regexp.MustCompile(`^(\w+):\s+(\d+) packages?$`)
	// "After this operation, 822 KiB extra will be used (...)" or "... will be freed (...)".
	netSizeRe  = regexp.MustCompile(`^After this operation, (.+?) (extra will be used|will be freed)`)
	spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
)

// appliedVerbs are the run-phase actions that put a package on the box; the
// "Removing" steps of an upgrade drop the versions it replaced.
var appliedVerbs = map[string]bool{"Installing": true, "Upgrading": true, "Downgrading": true, "Reinstalling": true}

// renderUpgrade runs argv, rendering a curated view of its output. phase is the
// short label for the header ("System", "Flatpak").
func renderUpgrade(phase string, argv []string) error {
	// sudo is primed once up front by the update (primeSudo), so the piped
	// transaction here never blocks on an unseen password prompt.
	pr, pw, err := os.Pipe()
	if err != nil {
		return sys.Run(argv[0], argv[1:]...)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return err
	}
	pw.Close()
	r := newUpgradeRenderer(os.Stdout, phase, sys.StdoutIsTTY())
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		logRaw(sc.Text())
		r.feed(sc.Text())
	}
	pr.Close()
	werr := cmd.Wait()
	r.finish(werr == nil)
	return werr
}

// sleepInhibitOK probes once whether this session may take a sleep/idle
// inhibitor. Taking one is polkit-gated in sessions with no agent (SSH, a
// headless run), where systemd-inhibit exits "Access denied" BEFORE the wrapped
// command runs: wrapping there turned a best-effort guard into a failed
// transaction, with dnf never invoked. Probe once per process; on a denial
// the transaction runs unwrapped instead of failing the update.
var sleepInhibitOK = sync.OnceValue(func() bool {
	if !sys.Has("systemd-inhibit") {
		return false
	}
	return exec.Command("systemd-inhibit", "--what=sleep:idle",
		"--who=ryoku update", "--why=probe", "--mode=block", "true").Run() == nil
})

// runUpgrade runs a package transaction sleep-inhibited when the session may
// take the inhibitor, rendered like renderUpgrade, or streamed raw for
// --verbose.
func runUpgrade(phase, why string, argv []string) error {
	full := argv
	if sleepInhibitOK() {
		full = append([]string{"systemd-inhibit", "--what=sleep:idle",
			"--who=ryoku update", "--why=" + why, "--mode=block"}, argv...)
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return sys.Run(full[0], full[1:]...)
	}
	cmd := exec.Command(full[0], full[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return err
	}
	pw.Close()
	// Curated for every consumer, animated only for a terminal: the Hub's update
	// island and `ryoku update > log` read stdout through a pipe, where the raw
	// dnf firehose (package lists, progress redraws, conflict paths) reads as
	// a broken install. The full firehose still lands in update-log.txt, and
	// --verbose keeps the raw passthrough on stdout.
	rendered := !verboseLog
	var r *upgradeRenderer
	if rendered {
		r = newUpgradeRenderer(os.Stdout, phase, sys.StdoutIsTTY())
	}
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		line := sc.Text()
		logRaw(line)
		if rendered {
			r.feed(line)
		} else {
			fmt.Fprintln(os.Stdout, line)
		}
	}
	pr.Close()
	werr := cmd.Wait()
	if rendered {
		r.finish(werr == nil)
	}
	return werr
}

// rawLog, set for the duration of an update, receives every unstyled line the
// renderers consume, so the full firehose is preserved for review even though
// the terminal shows only the curated view.
var rawLog io.Writer

func logRaw(line string) {
	if rawLog != nil {
		fmt.Fprintln(rawLog, line)
	}
}

var updateLogFile *os.File

// startUpdateLog opens the per-update raw log (overwritten each run) and points
// rawLog at it, so a curated run still leaves the full firehose to read. Returns
// the path, or "" when it cannot be created (logging is best-effort).
func startUpdateLog() string {
	path := filepath.Join(sys.Xdg("XDG_STATE_HOME", ".local/state"), "ryoku", "update-log.txt")
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return ""
	}
	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	updateLogFile = f
	rawLog = f
	return path
}

func stopUpdateLog() {
	rawLog = nil
	if updateLogFile != nil {
		updateLogFile.Close()
		updateLogFile = nil
	}
}

// liveSpinner animates on a timer, so a long silent step (a plugin compile that
// prints nothing for a minute) keeps spinning instead of looking hung. set()
// swaps the label, the ticker redraws, perm() prints a line above the spinner.
// Every write is serialized through the mutex.
type liveSpinner struct {
	mu     sync.Mutex
	w      io.Writer
	label  string
	frame  int
	active bool
	stop   chan struct{}
	done   chan struct{}
}

func newLiveSpinner(w io.Writer) *liveSpinner {
	s := &liveSpinner{w: w, stop: make(chan struct{}), done: make(chan struct{})}
	go s.loop()
	return s
}

func (s *liveSpinner) loop() {
	t := time.NewTicker(120 * time.Millisecond)
	defer t.Stop()
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.mu.Lock()
			s.draw()
			s.mu.Unlock()
		}
	}
}

// draw redraws the current label in place; the caller holds the lock.
func (s *liveSpinner) draw() {
	if s.label == "" {
		return
	}
	f := string(spinFrames[s.frame%len(spinFrames)])
	s.frame++
	label := s.label
	if w := sys.TermWidth() - 1; len(label)+4 > w && w > 4 {
		label = label[:w-4]
	}
	fmt.Fprint(s.w, "\r\033[K  "+sys.Brand(f)+" "+sys.Dim(label))
	s.active = true
}

func (s *liveSpinner) set(label string) {
	s.mu.Lock()
	s.label = label
	s.draw() // show the new step at once, without waiting for the next tick
	s.mu.Unlock()
}

func (s *liveSpinner) perm(line string) {
	s.mu.Lock()
	if s.active {
		fmt.Fprint(s.w, "\r\033[K")
		s.active = false
	}
	fmt.Fprintln(s.w, line)
	s.mu.Unlock()
}

func (s *liveSpinner) close() {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	if s.active {
		fmt.Fprint(s.w, "\r\033[K")
		s.active = false
	}
	s.mu.Unlock()
}

// renderQuiet runs argv showing a single in-place spinner of its current line
// rather than its full output, surfacing only warnings and errors. It suits a
// noisy-but-uninteresting step like deploy.sh (dozens of "building/installed"
// lines) whose phase header the caller already narrated. Exit status is passed
// through; a pipe failure degrades to raw passthrough.
func renderQuiet(argv []string) error {
	// sudo is primed once up front by the update (primeSudo); nothing to do here.
	pr, pw, err := os.Pipe()
	if err != nil {
		return sys.Run(argv[0], argv[1:]...)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return err
	}
	pw.Close()
	sp := newLiveSpinner(os.Stdout)
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		logRaw(line)
		switch {
		case quietError(line):
			sp.perm("  " + sys.Red("✗ ") + line)
		case quietWarn(line):
			sp.perm("  " + sys.Amber("! ") + line)
		default:
			sp.set(line)
		}
	}
	pr.Close()
	werr := cmd.Wait()
	sp.close()
	return werr
}

func quietError(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "error") || strings.Contains(l, "failed") ||
		strings.Contains(l, "cannot ") || strings.HasPrefix(l, "fatal")
}

func quietWarn(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "warning") || strings.Contains(l, "not restarted") ||
		strings.Contains(l, "skipped") || strings.Contains(l, ".rpmnew")
}

// scanLinesCR splits on either newline or carriage return, so each redraw of a
// progress bar arrives as its own token instead of one buffered mega-line.
func scanLinesCR(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// dnfSection is the part of a dnf5 run a line falls in. dnf prints the plan
// (package table, summary, sizes) before it downloads, then "Running
// transaction" before rpm applies it.
type dnfSection int

const (
	secLoad    dnfSection = iota // repository loading
	secTable                     // the package table
	secSummary                   // "Transaction Summary:" and the size lines
	secFetch                     // package downloads
	secRun                       // the rpm transaction
)

type upgradeRenderer struct {
	w         io.Writer
	phase     string
	animate   bool
	frame     int
	status    bool // an in-place status line is currently on screen
	section   dnfSection
	heading   string // the table heading rows fall under ("Upgrading", "Removing")
	pending   bool   // package summary parsed, not yet printed (waiting for sizes)
	scriptlet bool   // inside a ">>> Scriptlet output:" block
	detail    bool   // after an error header: its "- ..." lines are the reasons
	count     int
	size      string
	ryoku     []string
	applied   int
}

func newUpgradeRenderer(w io.Writer, phase string, animate bool) *upgradeRenderer {
	r := &upgradeRenderer{w: w, phase: phase, animate: animate}
	r.perm(sys.Brand("▸ ") + sys.Bold(phase+i18n.T(" packages")))
	return r
}

func (r *upgradeRenderer) feed(raw string) {
	line := strings.TrimRight(raw, " \t")
	trimmed := strings.TrimSpace(line)

	switch {
	case tableHeadRe.MatchString(trimmed):
		r.section = secTable
		return
	case trimmed == "Transaction Summary:":
		r.section = secSummary
		r.pending = true
		return
	case trimmed == "Running transaction":
		r.section = secRun
		r.flush()
		r.spin(i18n.T("working…"))
		return
	}
	switch r.section {
	case secTable:
		r.collectRow(line)
		return
	case secSummary:
		if r.summaryLine(trimmed) {
			return
		}
	}
	if strings.HasPrefix(trimmed, ">>> ") {
		r.feedRPM(strings.TrimPrefix(trimmed, ">>> "))
		return
	}
	r.scriptlet = false

	c := classify(trimmed)
	if c == clDrop {
		return
	}
	r.flush() // print the deferred package summary before the first real activity
	switch c {
	case clProgress:
		r.detail = false
		r.progress(trimmed)
		return
	case clDetail:
		if r.detail {
			r.perm("  " + sys.Red("  · ") + strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
			return
		}
		r.progress(trimmed)
		return
	case clWarn:
		r.perm("  " + sys.Amber("! ") + sys.Dim(stripPrefix(stripPrefix(trimmed, "Warning:"), "warning:")))
	case clError:
		r.perm("  " + sys.Red("✗ ") + stripPrefix(stripPrefix(trimmed, "Error:"), "error:"))
		r.detail = true
		return
	case clConflict:
		r.perm("  " + sys.Red("  · ") + strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
		return
	}
	r.detail = false
}

// summaryLine consumes the transaction summary and the size lines after it.
// "Replacing" counts the old side of each upgrade, so it is not added.
func (r *upgradeRenderer) summaryLine(trimmed string) bool {
	if m := summaryRe.FindStringSubmatch(trimmed); m != nil {
		if m[1] != "Replacing" {
			n := 0
			fmt.Sscanf(m[2], "%d", &n)
			r.count += n
		}
		return true
	}
	if m := netSizeRe.FindStringSubmatch(trimmed); m != nil {
		if strings.HasPrefix(m[2], "extra") {
			r.size = "+" + m[1]
		} else {
			r.size = "-" + m[1]
		}
		return true
	}
	return trimmed == "" || strings.HasPrefix(trimmed, "Total size of inbound packages")
}

// feedRPM handles dnf's ">>> " lines: scriptlet start/finish and output, and
// messages rpm itself raised ("[RPM] ..."). Scriptlet output is chatter for
// the raw log; rpm's config-file notices, unpack errors and failed installs are
// what a person must see.
func (r *upgradeRenderer) feedRPM(msg string) {
	switch {
	case msg == "Scriptlet output:":
		r.scriptlet = true
		return
	case strings.HasPrefix(msg, "Running ") || strings.HasPrefix(msg, "Finished "):
		r.scriptlet = false
		r.flush()
		r.spin(barTailRe.ReplaceAllString(msg, ""))
		return
	case strings.Contains(msg, ".rpmnew"), strings.Contains(msg, ".rpmsave"):
		r.flush()
		r.perm("  " + sys.Amber("! ") + sys.Dim(stripPrefix(stripPrefix(msg, "[RPM]"), "warning:")))
		return
	case r.scriptlet:
		return
	}
	l := strings.ToLower(msg)
	switch {
	// a failed scriptlet is reported, but rpm keeps the package and goes on.
	case strings.Contains(l, "scriptlet") && (strings.Contains(l, "error") || strings.Contains(l, "failed")):
		r.flush()
		r.perm("  " + sys.Amber("! ") + sys.Dim(stripPrefix(msg, "[RPM]")))
	case strings.HasPrefix(msg, "Unpack error"), strings.Contains(l, "failed"), strings.Contains(l, "error:"):
		r.flush()
		r.perm("  " + sys.Red("✗ ") + stripPrefix(msg, "[RPM]"))
	}
}

// progress spins the label for a step line: what dnf is applying in the run
// phase, a download count before it, or the line itself.
func (r *upgradeRenderer) progress(trimmed string) {
	m := stepRe.FindStringSubmatch(trimmed)
	if m == nil {
		r.spin(barTailRe.ReplaceAllString(trimmed, ""))
		return
	}
	if r.section == secSummary {
		r.section = secFetch
	}
	if r.section != secRun {
		r.spin(fmt.Sprintf(i18n.T("downloading (%s/%s)"), m[1], m[2]))
		return
	}
	label := barTailRe.ReplaceAllString(m[3], "")
	if verb, _, _ := strings.Cut(label, " "); appliedVerbs[verb] {
		r.applied++
	}
	r.spin(fmt.Sprintf("%s (%s/%s)", label, m[1], m[2]))
}

func (r *upgradeRenderer) finish(ok bool) {
	r.clear()
	r.flush()
	if ok {
		tail := ""
		if r.applied > 0 {
			tail = fmt.Sprintf(i18n.T(" · %d upgraded"), r.applied)
		} else if r.count > 0 {
			tail = fmt.Sprintf(i18n.T(" · %d upgraded"), r.count)
		}
		r.perm(sys.Green("✓ ") + r.phase + tail)
		return
	}
	r.perm(sys.Red("✗ ") + r.phase + i18n.T(" upgrade failed"))
}

// --- rendering primitives ---------------------------------------------------

func (r *upgradeRenderer) perm(line string) {
	if r.status {
		fmt.Fprint(r.w, "\r\033[K")
		r.status = false
	}
	fmt.Fprintln(r.w, line)
}

func (r *upgradeRenderer) spin(label string) {
	if !r.animate {
		return
	}
	frame := string(spinFrames[r.frame%len(spinFrames)])
	r.frame++
	line := "  " + sys.Brand(frame) + " " + label
	if w := sys.TermWidth() - 1; len(label)+4 > w && w > 4 {
		line = "  " + sys.Brand(frame) + " " + label[:w-4]
	}
	fmt.Fprint(r.w, "\r\033[K"+line)
	r.status = true
}

func (r *upgradeRenderer) clear() {
	if r.status {
		fmt.Fprint(r.w, "\r\033[K")
		r.status = false
	}
}

// collectRow reads one line of the package table: a heading ("Upgrading:"),
// a package row (" name  arch  version  repo  size") or the old side of an
// upgrade ("   replacing name ..."). Ryoku packages that land on the box are
// kept for the summary's highlight.
func (r *upgradeRenderer) collectRow(line string) {
	if line == "" {
		return
	}
	if !strings.HasPrefix(line, " ") {
		r.heading = strings.TrimSuffix(strings.Fields(line)[0], ":")
		return
	}
	if strings.HasPrefix(line, "  ") || !appliedVerbs[r.heading] {
		return
	}
	if name := strings.Fields(line)[0]; strings.HasPrefix(name, "ryoku") {
		r.ryoku = append(r.ryoku, name)
	}
}

// flush prints the deferred one-line package summary once, after the sizes have
// been parsed and before the first activity. A no-op until a summary was seen,
// and only once.
func (r *upgradeRenderer) flush() {
	if !r.pending {
		return
	}
	r.pending = false
	if r.count == 0 {
		return
	}
	line := fmt.Sprintf(i18n.T("  %s package%s"), sys.Bold(fmt.Sprint(r.count)), plural(r.count))
	if r.size != "" {
		line += sys.Dim(" · " + r.size)
	}
	r.perm(line)
	if len(r.ryoku) > 0 {
		r.perm("  " + sys.Dim(i18n.T("includes ")) + sys.Brand(strings.Join(r.ryoku, ", ")))
	}
}

// --- classification ---------------------------------------------------------

type lineClass int

const (
	clDrop lineClass = iota
	clProgress
	clWarn
	clError
	clConflict
	clDetail
)

func classify(trimmed string) lineClass {
	switch {
	case trimmed == "", isNoise(trimmed):
		return clDrop
	// rpm's file conflicts, listed under "Transaction failed": "- file X from
	// install of A conflicts with file from package B", or between two installs.
	case strings.Contains(trimmed, "conflicts with file from package"),
		strings.Contains(trimmed, "conflicts between attempted installs of"):
		return clConflict
	case strings.HasPrefix(trimmed, "Transaction failed"),
		strings.HasPrefix(trimmed, "Failed to resolve the transaction"),
		strings.HasPrefix(trimmed, "Problem"),
		strings.HasPrefix(trimmed, "Error:"),
		strings.HasPrefix(trimmed, "error:"),
		strings.HasPrefix(trimmed, "Cannot download"):
		return clError
	// .rpmnew/.rpmsave notices are the one warning class a user must act on.
	case strings.Contains(trimmed, ".rpmnew"), strings.Contains(trimmed, ".rpmsave"):
		return clWarn
	case strings.HasPrefix(trimmed, "Warning:"), strings.HasPrefix(trimmed, "warning:"):
		return clWarn
	case strings.HasPrefix(trimmed, "- "):
		return clDetail
	}
	// Steps, repository bars, and anything unrecognised fold into the spinner
	// rather than break the curated view or leak the firehose.
	return clProgress
}

// isNoise is dnf's framing: the loading banner, the download total and its
// rule, and the closing words.
func isNoise(s string) bool {
	switch s {
	case "Updating and loading repositories:", "Repositories loaded.", "Nothing to do.",
		"Complete!", "Dependencies resolved.":
		return true
	}
	if strings.Trim(s, "-") == "" {
		return true
	}
	if m := stepRe.FindStringSubmatch(s); m != nil && strings.HasPrefix(m[3], "Total ") {
		return true
	}
	return false
}

func stripPrefix(s, prefix string) string {
	return strings.TrimSpace(strings.TrimPrefix(s, prefix))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
