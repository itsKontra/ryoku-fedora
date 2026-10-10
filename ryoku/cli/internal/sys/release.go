package sys

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReleaseFile is the package-owned marker ryoku-desktop ships naming the
// release a box runs; a var for tests.
var ReleaseFile = "/etc/ryoku-release"

// PackagedChannel is the channel a packaged box follows: the channel its
// RyokuCOPR baseurl names, "" when there is no repo file or it points at a
// mirror Ryoku does not publish.
func PackagedChannel() string { return ChannelOfServer(RyokuServer()) }

// WriteRootFile replaces path whole with contents at the given mode. A target
// whose directory we can write (a user-owned file, a test fixture under a temp
// dir) is written in place; only a root-owned path like /etc falls back to
// sudo install. The in-place path keeps writers hermetic under a temp
// RPMRepoFile and never shells out to sudo in a unit test.
func WriteRootFile(path, contents, mode string) error {
	if writableDir(filepath.Dir(path)) {
		return writeFileAtomic(path, contents, fileMode(mode))
	}
	tmp, err := os.CreateTemp("", "ryoku-root-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(contents); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return Run("sudo", "install", "-D", "-m", mode, "-o", "root", "-g", "root", tmp.Name(), path)
}

// writableDir reports whether the current process can create a file in dir (a
// user-owned config dir or a test temp dir); a root-owned dir like /etc returns
// false so the caller escalates.
func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".ryoku-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	return true
}

// writeFileAtomic writes contents to path via a temp file in the same directory
// and an atomic rename, so a reader never sees a half-written file.
func writeFileAtomic(path, contents string, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ryoku-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(contents); err != nil {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// fileMode parses an octal mode string ("0644") for the in-place writer,
// defaulting to 0644 for anything unparseable.
func fileMode(mode string) os.FileMode {
	if m, err := strconv.ParseUint(mode, 8, 32); err == nil {
		return os.FileMode(m)
	}
	return 0o644
}

// Release is /etc/ryoku-release: the named state a packaged box runs, written
// by the ryoku-desktop package from what prepare-srpms.sh was told at build.
type Release struct {
	Release string // a release tag, git describe past one (v0.73.0-5-gabc1234), or local-* for a hand build
	Name    string // the line's name (CODENAME at the build)
	Channel string // copr, local
	Version string // the RPM version
	Commit  string
	Date    string
}

// ReadRelease parses ReleaseFile; every field is "" when the file is absent
// (a box installed before releases were named; the next update carries it).
func ReadRelease() Release {
	var r Release
	b, err := os.ReadFile(ReleaseFile)
	if err != nil {
		return r
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch strings.TrimSpace(k) {
		case "RELEASE":
			r.Release = v
		case "NAME":
			r.Name = v
		case "CHANNEL":
			r.Channel = v
		case "VERSION":
			r.Version = v
		case "COMMIT":
			r.Commit = v
		case "DATE":
			r.Date = v
		}
	}
	return r
}
