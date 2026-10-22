package kmod

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// AsRoot runs `ryoku-host kmod` as root for doctor and the updater, which run
// as the desktop user, and reads back the key=value report Execute writes.
type AsRoot struct {
	// Host is the ryoku-host binary; "ryoku-host" when empty. Doctor passes
	// an absolute path so sudo never resolves it through the user's PATH.
	Host string
	// NonInteractive adds sudo's -n, for a read that must never prompt.
	NonInteractive bool
	// Progress also receives ryoku-host's stderr as it runs, for a build that
	// takes minutes.
	Progress io.Writer
	// Wrap, when set, wraps the whole command line, such as in a sleep
	// inhibitor.
	Wrap func(argv []string) []string
}

// Advice is the command a user runs by hand for a kmod verb.
func Advice(args ...string) string {
	return "sudo ryoku-host kmod " + strings.Join(args, " ")
}

// Run returns ryoku-host's report and the last line of its stderr, which on a
// failure is also the error.
func (r AsRoot) Run(args ...string) (Report, string, error) {
	host := r.Host
	if host == "" {
		host = "ryoku-host"
	}
	argv := append([]string{host, "kmod"}, args...)
	if os.Geteuid() != 0 {
		sudo := []string{"sudo"}
		if r.NonInteractive {
			sudo = append(sudo, "-n")
		}
		argv = append(sudo, argv...)
	}
	if r.Wrap != nil {
		argv = r.Wrap(argv)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &stdout, &stderr
	if r.Progress != nil {
		cmd.Stderr = io.MultiWriter(&stderr, r.Progress)
	}
	err := cmd.Run()
	detail := lastLine(stderr.String())
	if err != nil && detail != "" {
		err = errors.New(detail)
	}
	return ParseReport(stdout.String()), detail, err
}

// BuildForKernel runs build-for-kernel as root. detail is akmods' last word,
// which says why a build failed.
func (r AsRoot) BuildForKernel(opts BuildOptions) (result BuildResult, detail string, err error) {
	report, detail, err := r.Run(opts.Args()...)
	if err != nil {
		return BuildResult{}, detail, err
	}
	result, err = report.Build()
	return result, detail, err
}

// Args are build-for-kernel's command line for these options.
func (o BuildOptions) Args() []string {
	args := []string{"build-for-kernel"}
	if o.Kernel != "" {
		args = append(args, "--kernel", o.Kernel)
	}
	if o.Rebuild {
		args = append(args, "--rebuild")
	}
	return args
}

// Report is a `ryoku-host kmod` answer read back. A verb's own lines lead and
// the status block follows; no key appears twice, and the first value wins.
type Report map[string]string

func ParseReport(out string) Report {
	report := Report{}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if _, seen := report[key]; ok && !seen {
			report[key] = value
		}
	}
	return report
}

// Build reads build-for-kernel's lines. Its status block is not read back.
func (r Report) Build() (BuildResult, error) {
	build, ok := r["build"]
	if !ok || (build != "ok" && build != "failed") {
		return BuildResult{}, fmt.Errorf("build-for-kernel printed no build=ok|failed")
	}
	return BuildResult{Kernel: r["kernel"], Built: build == "ok", NewestWithModule: r["newest_module_kernel"]}, nil
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
