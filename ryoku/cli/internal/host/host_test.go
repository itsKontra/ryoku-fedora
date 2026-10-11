package host

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	commands []string
	codes    map[string]int // argv prefix -> exit code
}

func (f *fakeRunner) code(argv string) int {
	for prefix, code := range f.codes {
		if strings.HasPrefix(argv, prefix) {
			return code
		}
	}
	return 0
}

func (f *fakeRunner) Run(name string, args ...string) int {
	argv := strings.Join(append([]string{name}, args...), " ")
	f.commands = append(f.commands, argv)
	return f.code(argv)
}

func testApp(runner *fakeRunner, installed map[string]bool) (*App, *bytes.Buffer) {
	out := &bytes.Buffer{}
	app := New(Config{
		Runner: runner,
		Stdout: out,
		Stderr: &bytes.Buffer{},
		Packages: Packages{
			Manager:   func() string { return "dnf" },
			Installed: func(n string) bool { return installed[n] },
			Available: func(n string) bool { return n == "foo" },
			Version:   func(n string) string { return map[string]string{"foo": "1.0-1.fc44"}[n] },
			Owner:     func(p string) string { return map[string]string{"/usr/bin/foo": "foo"}[p] },
		},
		Repo: Repo{
			Server: func() string {
				return "https://download.copr.fedorainfracloud.org/results/itskontra/ryoku/fedora-$releasever-$basearch"
			},
			SetChannel: func(c string) error { return errors.New("no " + c) },
		},
	})
	return app, out
}

func TestSessionStartRunsTheAutostartChain(t *testing.T) {
	runner := &fakeRunner{}
	app, _ := testApp(runner, nil)
	if code := app.Execute([]string{"session", "start", "xdg-desktop-portal-gtk.service"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	want := []string{
		"dbus-update-activation-environment --systemd --all",
		"systemctl --user daemon-reload",
		"ryoku-reload-cover begin boot",
		"ryoku-power-cutover session-start-logged",
		"systemctl --user try-restart xdg-desktop-portal.service xdg-desktop-portal-gtk.service",
	}
	if strings.Join(runner.commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands:\n%s", strings.Join(runner.commands, "\n"))
	}
}

func TestSessionStartContinuesPastEnvAndCoverButStopsAtCutover(t *testing.T) {
	runner := &fakeRunner{codes: map[string]int{"dbus-update": 1, "systemctl --user daemon-reload": 1, "ryoku-reload-cover": 1}}
	app, _ := testApp(runner, nil)
	if code := app.Execute([]string{"session", "start"}); code != ExitOK {
		t.Fatalf("a failed env push, reload or cover failed the session: exit = %d", code)
	}
	if len(runner.commands) != 5 {
		t.Fatalf("session stopped early: %v", runner.commands)
	}
	runner = &fakeRunner{codes: map[string]int{"ryoku-power-cutover": 1}}
	app, _ = testApp(runner, nil)
	if code := app.Execute([]string{"session", "start"}); code != ExitFailure {
		t.Fatalf("exit = %d", code)
	}
	if len(runner.commands) != 4 {
		t.Fatalf("portal restarted after a failed cutover: %v", runner.commands)
	}
}

func TestServiceArgvAndExitMapping(t *testing.T) {
	runner := &fakeRunner{}
	app, _ := testApp(runner, nil)
	app.Execute([]string{"svc", "is-active", "voxtype"})
	app.Execute([]string{"svc", "--system", "enable", "--now", "libvirtd.socket"})
	want := "systemctl --user is-active --quiet voxtype.service\nsystemctl enable --now libvirtd.socket"
	if got := strings.Join(runner.commands, "\n"); got != want {
		t.Fatalf("argv:\n%s", got)
	}
	for _, tc := range []struct {
		verb string
		code int
		want int
	}{
		{"is-active", 3, ExitFalse},
		{"is-enabled", 1, ExitFalse},
		{"stop", 5, ExitAbsent},
		{"stop", 1, ExitFailure},
	} {
		app, _ := testApp(&fakeRunner{codes: map[string]int{"systemctl": tc.code}}, nil)
		if got := app.Execute([]string{"svc", tc.verb, "x"}); got != tc.want {
			t.Errorf("%s exit %d -> %d, want %d", tc.verb, tc.code, got, tc.want)
		}
	}
	if got := app.Execute([]string{"svc", "env", "BAD-NAME"}); got != ExitUsage {
		t.Errorf("bad env name exit = %d", got)
	}
}

func TestPackageVerbs(t *testing.T) {
	runner := &fakeRunner{}
	app, out := testApp(runner, map[string]bool{"foo": true})
	checks := []struct {
		args []string
		want int
	}{
		{[]string{"pkg", "installed", "foo"}, ExitOK},
		{[]string{"pkg", "installed", "foo", "bar"}, ExitFalse},
		{[]string{"pkg", "available", "foo"}, ExitOK},
		{[]string{"pkg", "available", "bar"}, ExitFalse},
		{[]string{"pkg", "version", "foo"}, ExitOK},
		{[]string{"pkg", "owner", "/usr/bin/foo"}, ExitOK},
		{[]string{"pkg", "owner", "/nope"}, ExitFalse},
		{[]string{"pkg", "install"}, ExitUsage},
		{[]string{"pkg", "install", "foo"}, ExitOK},
		{[]string{"pkg", "install", "--upgrade", "foo"}, ExitOK},
		{[]string{"pkg", "install-file", "/tmp/a.rpm"}, ExitOK},
		{[]string{"pkg", "remove", "bar"}, ExitOK},
		{[]string{"pkg", "remove", "foo", "bar"}, ExitOK},
	}
	for _, c := range checks {
		if got := app.Execute(c.args); got != c.want {
			t.Errorf("%v exit = %d, want %d", c.args, got, c.want)
		}
	}
	if out.String() != "1.0-1.fc44\nfoo\n" {
		t.Errorf("stdout = %q", out.String())
	}
	want := []string{
		"dnf -y install -- foo",
		"dnf -y upgrade -- foo",
		"dnf install -y /tmp/a.rpm",
		"dnf -y --setopt=clean_requirements_on_remove=False remove -- foo",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("transactions:\n%s", got)
	}
}

func TestPackageWithoutManagerFails(t *testing.T) {
	app, _ := testApp(&fakeRunner{}, nil)
	app.cfg.Packages.Manager = func() string { return "" }
	if code := app.Execute([]string{"pkg", "installed", "foo"}); code != ExitFailure {
		t.Fatalf("exit = %d", code)
	}
	if code := app.Execute([]string{"pkgmgr"}); code != ExitAbsent {
		t.Fatalf("pkgmgr exit = %d", code)
	}
}

func TestRepoChannel(t *testing.T) {
	app, out := testApp(&fakeRunner{}, nil)
	if code := app.Execute([]string{"repo", "channel"}); code != ExitOK || out.String() != "copr\n" {
		t.Fatalf("exit %d out %q", code, out.String())
	}
	app.cfg.Repo.Server = func() string { return "" }
	if code := app.Execute([]string{"repo", "channel"}); code != ExitAbsent {
		t.Fatalf("no repo exit = %d", code)
	}
	app.cfg.Repo.Server = func() string { return "https://mirror.example/ryoku" }
	if code := app.Execute([]string{"repo", "channel"}); code != ExitFalse {
		t.Fatalf("foreign mirror exit = %d", code)
	}
	if code := app.Execute([]string{"repo", "set-channel", "stable"}); code != ExitFailure {
		t.Fatalf("set-channel exit = %d", code)
	}
}

func TestPowerTimeKeymap(t *testing.T) {
	runner := &fakeRunner{}
	app, _ := testApp(runner, nil)
	for _, args := range [][]string{
		{"power", "reboot"},
		{"time", "set-zone", "Europe/Vienna"},
		{"time", "zone"},
		{"keymap", "set", "de", "nodeadkeys"},
	} {
		if code := app.Execute(args); code != ExitOK {
			t.Fatalf("%v exit = %d", args, code)
		}
	}
	want := []string{
		"systemctl reboot",
		"timedatectl set-timezone Europe/Vienna",
		"timedatectl show -p Timezone --value",
		"localectl set-x11-keymap de  nodeadkeys ",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands:\n%s", got)
	}
	for _, bad := range [][]string{{"power", "halt"}, {"keymap", "set", "de;rm"}, {"time"}} {
		if code := app.Execute(bad); code != ExitUsage {
			t.Errorf("%v exit = %d", bad, code)
		}
	}
}

func TestInitNeedsSystemd(t *testing.T) {
	dir := t.TempDir()
	app, out := testApp(&fakeRunner{}, nil)
	app.cfg.SystemdRuntimeDir = filepath.Join(dir, "missing")
	if code := app.Execute([]string{"init"}); code != ExitAbsent {
		t.Fatalf("exit = %d", code)
	}
	if err := os.Mkdir(app.cfg.SystemdRuntimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := app.Execute([]string{"init"}); code != ExitOK || out.String() != "systemd\n" {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}
