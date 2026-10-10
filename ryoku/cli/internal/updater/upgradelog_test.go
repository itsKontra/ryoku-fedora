package updater

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures are dnf5 output captured through a pipe, the way runUpgrade
// reads it: the Ryoku lane's plan (distro-sync --assumeno), a successful
// install, and an upgrade whose rpm transaction failed on an unpack error.
func transcript(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(b), "\n")
}

func renderTranscript(t *testing.T, transcript string, ok bool) string {
	t.Helper()
	var buf bytes.Buffer
	r := newUpgradeRenderer(&buf, "System", false) // animate=false: deterministic
	for _, ln := range strings.Split(transcript, "\n") {
		r.feed(ln)
	}
	r.finish(ok)
	return buf.String()
}

func TestRendererSummarisesPlan(t *testing.T) {
	out := renderTranscript(t, transcript(t, "dnf-ryoku-plan.txt"), true)
	for _, want := range []string{
		"System packages", // header
		"11 packages",     // Upgrading only; Replacing is the old side
		"+822 KiB",        // net size
		"ryoku-desktop",   // ryoku highlight
		"ryoku-shell",
		"11 upgraded",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("curated output missing %q:\n%s", want, out)
		}
	}
	for _, noise := range []string{"Repositories loaded", "replacing", "RyokuCOPR", "Transaction Summary", "inbound"} {
		if strings.Contains(out, noise) {
			t.Errorf("curated output leaked %q:\n%s", noise, out)
		}
	}
	if strings.Contains(out, "ryogami") || strings.Contains(out, "sddm-theme-ryoku") {
		t.Errorf("highlight should list only ryoku* packages:\n%s", out)
	}
}

func TestRendererHidesNoise(t *testing.T) {
	out := renderTranscript(t, transcript(t, "dnf-upgrade-failed.txt"), false)
	for _, noise := range []string{
		"Updating and loading", "Package ", "Upgrading:", "replacing",
		"Already downloaded", "Total", "-----", "Verify package files",
		"Upgrading libgcc", "Removing", "Scriptlet output", "/dev/null",
		"%pretrans", "NOKEY",
	} {
		if strings.Contains(out, noise) {
			t.Errorf("curated output leaked noise %q:\n%s", noise, out)
		}
	}
}

func TestRendererKeepsSignal(t *testing.T) {
	out := renderTranscript(t, transcript(t, "dnf-upgrade-failed.txt"), false)
	for _, want := range []string{
		"42 packages",                 // 3 installed + 39 upgraded
		"+3 MiB",                      // net size
		"unpacking of archive failed", // rpm's own error
		"Unpack error: filesystem",    // dnf's error for it
		"install failed",              // rpm's verdict on the package
		"Transaction failed",          // dnf's verdict
		"skipped OpenPGP checks",      // a warning
		"System upgrade failed",       // failure cap
	} {
		if !strings.Contains(out, want) {
			t.Errorf("curated output missing %q:\n%s", want, out)
		}
	}
}

func TestRendererSuccessCap(t *testing.T) {
	out := renderTranscript(t, transcript(t, "dnf-install.txt"), true)
	if !strings.Contains(out, "✓") || !strings.Contains(out, "2 upgraded") || !strings.Contains(out, "2 packages") {
		t.Errorf("expected success cap with count, got:\n%s", out)
	}
	if strings.Contains(out, "Complete!") {
		t.Errorf("closing words leaked:\n%s", out)
	}
}

// The run phase counts what it put on the box, not its verify, prepare and
// removal steps.
func TestRendererCountsAppliedPackages(t *testing.T) {
	var buf bytes.Buffer
	r := newUpgradeRenderer(&buf, "System", false)
	for _, ln := range strings.Split(transcript(t, "dnf-upgrade-failed.txt"), "\n") {
		r.feed(ln)
	}
	if r.applied != 42 {
		t.Errorf("applied = %d, want 42 (the Installing and Upgrading steps)", r.applied)
	}
}

func TestRendererSurfacesResolveProblems(t *testing.T) {
	out := renderTranscript(t, `Updating and loading repositories:
Repositories loaded.
Failed to resolve the transaction:
Problem: cannot install the best update candidate for package ryoku-shell-0:0.4046-63.fc44.x86_64
  - nothing provides quickshell >= 0.3 needed by ryoku-shell-0:0.4057-73.fc44.x86_64 from RyokuCOPR
You can try to add to command line:
  --skip-broken to skip uninstallable packages`, false)
	for _, want := range []string{"Failed to resolve", "Problem: cannot install", "nothing provides quickshell"} {
		if !strings.Contains(out, want) {
			t.Errorf("curated output missing %q:\n%s", want, out)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		line string
		want lineClass
	}{
		{"Repositories loaded.", clDrop},
		{"[42/42] Total                           100% |  10.6 MiB/s |  15.6 MiB |  00m01s", clDrop},
		{"--------------------------------------------------------------------------------", clDrop},
		{"Complete!", clDrop},
		{"warning: /etc/foo.conf created as /etc/foo.conf.rpmnew", clWarn},
		{"Warning: skipped OpenPGP checks for 2 packages from repositories: fedora, updates", clWarn},
		{"Transaction failed: Rpm transaction failed.", clError},
		{"- file /usr/share/plymouth/themes/ryoku/logo.png from install of ryoku-desktop-2-2.noarch conflicts with file from package plymouth-1-1.x86_64", clConflict},
		{"[ 4/83] Upgrading libgcc-0:16.2.1-2.fc4 100% | 133.0 MiB/s | 272.3 KiB |  00m00s", clProgress},
		{" Fedora 44 - x86_64                     100% |   7.1 MiB/s |  37.8 MiB |  00m05s", clProgress},
	}
	for _, c := range cases {
		if got := classify(strings.TrimSpace(c.line)); got != c.want {
			t.Errorf("classify(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}

// TestPipedRunIsCurated pins the consumer the Hub's update island and
// `ryoku update > log` represent: stdout is a pipe, so the curated view (never
// the raw package-manager firehose) must reach it.
func TestPipedRunIsCurated(t *testing.T) {
	oldInhibit := sleepInhibitOK
	sleepInhibitOK = func() bool { return false }
	t.Cleanup(func() { sleepInhibitOK = oldInhibit })

	capR, capW, err := os.Pipe()
	if err != nil {
		t.Skipf("no pipe: %v", err)
	}
	keep := os.Stdout
	os.Stdout = capW
	script := `printf '%s\n' 'Updating and loading repositories:' 'Repositories loaded.' 'Transaction Summary:' ' Upgrading:          2 packages' '' '[1/2] ryoku-0:2-1.fc44.x86_64       100% |   1.0 MiB/s |   1.0 MiB |  00m00s' 'Running transaction' '[1/4] Upgrading ryoku-0:2-1.fc44.x86 100% |   1.0 MiB/s |   1.0 MiB |  00m00s' '>>> [RPM] warning: /etc/foo.conf created as /etc/foo.conf.rpmnew' '  - file /usr/share/plymouth/themes/ryoku/bullet.png from install of ryoku-desktop-2-2.noarch conflicts with file from package plymouth-1-1.x86_64'`
	_ = runUpgrade("System", "test", []string{"bash", "-c", script})
	capW.Close()
	os.Stdout = keep
	out, _ := io.ReadAll(capR)
	capR.Close()
	got := string(out)

	for _, noise := range []string{"Repositories loaded", "Upgrading ryoku", "100%"} {
		if strings.Contains(got, noise) {
			t.Errorf("piped output leaked firehose line %q:\n%s", noise, got)
		}
	}
	for _, want := range []string{"System packages", ".rpmnew", "conflicts with file"} {
		if !strings.Contains(got, want) {
			t.Errorf("piped output missing signal %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\r") {
		t.Errorf("piped output carries spinner carriage returns:\n%q", got)
	}
}
