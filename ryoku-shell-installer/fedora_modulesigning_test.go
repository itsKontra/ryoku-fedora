package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKmodHost puts a ryoku-host on the system bin path that logs its argv
// and answers queue-enrollment with the enrollment state in $ENROLLMENT, plus
// a sudo that drops -n and runs the command as is.
func fakeKmodHost(t *testing.T) (home, logPath string) {
	t.Helper()
	root := t.TempDir()
	oldSystemBin := ryokuSystemBin
	ryokuSystemBin = filepath.Join(root, "usr-bin")
	t.Cleanup(func() { ryokuSystemBin = oldSystemBin })
	home = filepath.Join(root, "home")
	fakeBin := filepath.Join(root, "fake-bin")
	for _, dir := range []string{ryokuSystemBin, home, fakeBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	logPath = filepath.Join(root, "host.log")
	host := `#!/bin/sh
printf '%s\n' "$*" >> "$HOST_LOG"
[ "$2" != "$FAIL_VERB" ] || { echo "ryoku-host: refused" >&2; exit 1; }
case "$2" in
  prepare-key) printf 'action=created\nkey=present\n' ;;
  queue-enrollment)
    printf 'action=%s\nkey=present\nkey_ok=yes\nsecure_boot=enforcing\nenrollment=%s\npassword=enroll\n' "$ACTION" "$ENROLLMENT"
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(ryokuSystemBin, "ryoku-host"), []byte(host), 0o755); err != nil {
		t.Fatal(err)
	}
	sudo := "#!/bin/sh\n[ \"$1\" != -n ] || shift\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "sudo"), []byte(sudo), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOST_LOG", logPath)
	t.Setenv("FAIL_VERB", "")
	return home, logPath
}

func runFedoraDrivers(t *testing.T, home string) (*engine, string) {
	t.Helper()
	events := make(chan any, 256)
	e := &engine{f: &facts{distro: newFedoraLinux(), homeDir: home}, p: &plan{}, events: events}
	if err := stepDrivers(e); err != nil {
		t.Fatal(err)
	}
	close(events)
	var lines []string
	for event := range events {
		if line, ok := event.(evLine); ok {
			lines = append(lines, line.line)
		}
	}
	return e, strings.Join(lines, "\n")
}

func hostCalls(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestFedoraDriversPrepareTheKeyThenQueueTheEnrollment(t *testing.T) {
	home, logPath := fakeKmodHost(t)
	t.Setenv("ACTION", "queued")
	t.Setenv("ENROLLMENT", "queued")
	e, output := runFedoraDrivers(t, home)
	if got, want := hostCalls(t, logPath), "kmod prepare-key\nkmod queue-enrollment"; got != want {
		t.Fatalf("ryoku-host calls = %q, want %q", got, want)
	}
	summary := strings.Join(enrollmentSummary(e), "\n")
	for _, want := range []string{"MokManager", "'Enroll MOK'", "'Continue'", "'Yes'", "password enroll."} {
		if !strings.Contains(summary, want) {
			t.Fatalf("enrollment summary lacks %q:\n%s", want, summary)
		}
	}
	if !strings.Contains(output, summary) {
		t.Fatalf("the install log does not carry the enrollment steps:\n%s", output)
	}
}

func TestFedoraDriversReadThePasswordFromRyokuHost(t *testing.T) {
	home, _ := fakeKmodHost(t)
	t.Setenv("ACTION", "queued")
	t.Setenv("ENROLLMENT", "queued")
	host := filepath.Join(ryokuSystemBin, "ryoku-host")
	script, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, []byte(strings.ReplaceAll(string(script), "password=enroll", "password=from-host")), 0o755); err != nil {
		t.Fatal(err)
	}
	e, _ := runFedoraDrivers(t, home)
	if summary := strings.Join(enrollmentSummary(e), "\n"); !strings.Contains(summary, "from-host") {
		t.Fatalf("summary does not use ryoku-host's password:\n%s", summary)
	}
}

func TestFedoraDriversPrintNoEnrollmentStepsWhenNoneIsPending(t *testing.T) {
	for _, enrollment := range []string{"enrolled", "not_queued", "unknown"} {
		t.Run(enrollment, func(t *testing.T) {
			home, _ := fakeKmodHost(t)
			t.Setenv("ACTION", "none")
			t.Setenv("ENROLLMENT", enrollment)
			e, output := runFedoraDrivers(t, home)
			if summary := enrollmentSummary(e); len(summary) != 0 {
				t.Fatalf("summary for %s: %q", enrollment, summary)
			}
			if strings.Contains(output, "MokManager") {
				t.Fatalf("enrollment steps printed for %s:\n%s", enrollment, output)
			}
		})
	}
}

func TestFedoraDriversWarnAndContinueWhenTheKeyFails(t *testing.T) {
	home, logPath := fakeKmodHost(t)
	t.Setenv("FAIL_VERB", "prepare-key")
	e, output := runFedoraDrivers(t, home)
	if got := hostCalls(t, logPath); got != "kmod prepare-key" {
		t.Fatalf("ryoku-host calls after a failed key = %q", got)
	}
	if !strings.Contains(output, "warning:") || len(enrollmentSummary(e)) != 0 {
		t.Fatalf("a failed key did not warn, or still printed steps:\n%s", output)
	}

	home, logPath = fakeKmodHost(t)
	t.Setenv("FAIL_VERB", "queue-enrollment")
	e, output = runFedoraDrivers(t, home)
	if got := hostCalls(t, logPath); got != "kmod prepare-key\nkmod queue-enrollment" {
		t.Fatalf("ryoku-host calls = %q", got)
	}
	if !strings.Contains(output, "warning:") || len(enrollmentSummary(e)) != 0 {
		t.Fatalf("a failed queue did not warn, or still printed steps:\n%s", output)
	}
}
