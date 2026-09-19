package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type artifact struct {
	Data []byte      `json:"data,omitempty"`
	Link string      `json:"link,omitempty"`
	Mode fs.FileMode `json:"mode"`
}
type ownedArtifact struct {
	Installed string    `json:"installed"`
	Original  *artifact `json:"original,omitempty"`
}

var artifactRoots = []string{".local/bin", ".local/lib/qt6/qml", ".config/systemd/user"}

func artifactAllowed(rel string) bool {
	if filepath.IsAbs(rel) || filepath.Clean(rel) != rel {
		return false
	}
	for _, root := range artifactRoots {
		if strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func readArtifact(path string) (artifact, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return artifact{}, err
	}
	a := artifact{Mode: fi.Mode()}
	if fi.Mode()&os.ModeSymlink != 0 {
		a.Link, err = os.Readlink(path)
	} else if fi.Mode().IsRegular() {
		a.Data, err = os.ReadFile(path)
	} else {
		return a, fmt.Errorf("unsupported artifact %s", path)
	}
	return a, err
}
func (a artifact) digest() string {
	b, _ := json.Marshal(a)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
func snapshotArtifacts(home string) (map[string]artifact, error) {
	out := map[string]artifact{}
	for _, root := range artifactRoots {
		err := filepath.WalkDir(filepath.Join(home, root), func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			a, err := readArtifact(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(home, path)
			if err != nil {
				return err
			}
			out[rel] = a
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
func ownershipPath(home string) string {
	return filepath.Join(home, ".local/state/ryoku/source-artifacts.json")
}
func readOwnership(home string) (map[string]ownedArtifact, error) {
	entries := map[string]ownedArtifact{}
	b, err := os.ReadFile(ownershipPath(home))
	if os.IsNotExist(err) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	return entries, json.Unmarshal(b, &entries)
}
func writeOwnership(home string, entries map[string]ownedArtifact) error {
	path := ownershipPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".artifacts-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func recordArtifacts(home string, before map[string]artifact) error {
	entries, err := readOwnership(home)
	if err != nil {
		return err
	}
	after, err := snapshotArtifacts(home)
	if err != nil {
		return err
	}
	for path, a := range after {
		old, existed := before[path]
		if existed && old.digest() == a.digest() {
			continue
		}
		entry, owned := entries[path]
		if !owned || (existed && entry.Installed != old.digest()) {
			entry = ownedArtifact{}
			if existed {
				copy := old
				entry.Original = &copy
			}
		}
		entry.Installed = a.digest()
		entries[path] = entry
	}
	return writeOwnership(home, entries)
}
func (e *engine) runOwnedStep(step estep) error {
	if e.dry || !e.d().fromSource {
		return step.fn(e)
	}
	// A corrupt receipt must fail before anything can overwrite the originals.
	if _, err := readOwnership(e.f.homeDir); err != nil {
		return err
	}
	before, err := snapshotArtifacts(e.f.homeDir)
	if err != nil {
		return err
	}
	stepErr := step.fn(e)
	return errors.Join(stepErr, recordArtifacts(e.f.homeDir, before))
}

func uninstallArtifacts(home string, dry bool, run func(string, ...string) error) error {
	entries, err := readOwnership(home)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("no source ownership receipt; preserving untracked files")
		return nil
	}
	for rel, entry := range entries {
		if !artifactAllowed(rel) {
			return fmt.Errorf("invalid ownership path %q", rel)
		}
		path := filepath.Join(home, rel)
		// Do not follow an ancestor replaced with a symlink since installation.
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil || parent != filepath.Dir(path) {
			return fmt.Errorf("ownership path changed: %s", path)
		}
		a, err := readArtifact(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if a.digest() != entry.Installed {
			fmt.Println("preserving modified artifact: " + path)
			continue
		}
		if dry {
			fmt.Println("DRYRUN: remove or restore recorded artifact " + path)
			continue
		}
		if strings.HasPrefix(rel, ".config/systemd/user/") && !strings.Contains(strings.TrimPrefix(rel, ".config/systemd/user/"), "/") && (strings.HasSuffix(rel, ".service") || strings.HasSuffix(rel, ".timer")) {
			if err := run("systemctl", "--user", "disable", "--now", filepath.Base(rel)); err != nil {
				return err
			}
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if old := entry.Original; old != nil {
			if old.Link != "" {
				err = os.Symlink(old.Link, path)
			} else {
				err = os.WriteFile(path, old.Data, old.Mode.Perm())
			}
			if err != nil {
				return err
			}
		}
		delete(entries, rel)
	}
	if dry {
		return nil
	}
	if err := writeOwnership(home, entries); err != nil {
		return err
	}
	return run("systemctl", "--user", "daemon-reload")
}
