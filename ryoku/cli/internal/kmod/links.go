package kmod

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// akmods signs with the pair at these paths. Pointing them at DKMS's pair makes
// akmods sign with the machine's one module signing key, and akmods-keygen,
// which only runs when either path is missing or empty, never makes a second.
const (
	AkmodsKeyPath  = "/etc/pki/akmods/private/private_key.priv"
	AkmodsCertPath = "/etc/pki/akmods/certs/public_key.der"
)

// LinkState is where akmods' key paths lead, worst of the two.
type LinkState string

const (
	LinksNotNeeded LinkState = "not_needed"
	LinksOK        LinkState = "ok"
	LinksMissing   LinkState = "missing"
	LinksDangling  LinkState = "dangling"
	LinksForeign   LinkState = "foreign"
)

var linkRank = map[LinkState]int{LinksOK: 0, LinksMissing: 1, LinksDangling: 2, LinksForeign: 3}

var akmodsLinks = [][2]string{{AkmodsKeyPath, KeyPath}, {AkmodsCertPath, CertPath}}

func (m *Machine) linkState(akmods bool) LinkState {
	if !akmods {
		return LinksNotNeeded
	}
	worst := LinksOK
	for _, link := range akmodsLinks {
		if state := m.oneLink(link[0], link[1]); linkRank[state] > linkRank[worst] {
			worst = state
		}
	}
	return worst
}

func (m *Machine) oneLink(linkPath, want string) LinkState {
	info, err := os.Lstat(m.path(linkPath))
	if err != nil {
		return LinksMissing
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return LinksForeign
	}
	target, err := os.Readlink(m.path(linkPath))
	if err != nil {
		return LinksForeign
	}
	if !path.IsAbs(target) {
		target = path.Join(path.Dir(linkPath), target)
	}
	target = path.Clean(target)
	if _, err := os.Stat(m.path(target)); err != nil {
		return LinksDangling
	}
	if target != want {
		return LinksForeign
	}
	return LinksOK
}

// pointAkmodsAtKey replaces each akmods key path that does not lead to DKMS's
// pair with a link to it. The swap is a rename, so akmods never sees a gap.
func (m *Machine) pointAkmodsAtKey(akmodsGID int) (bool, error) {
	changed := false
	for _, link := range akmodsLinks {
		if m.oneLink(link[0], link[1]) == LinksOK {
			continue
		}
		dir := filepath.Dir(m.path(link[0]))
		if _, err := os.Stat(dir); err != nil {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return changed, err
			}
			if err := m.cfg.Owners.Chown(dir, 0, akmodsGID); err != nil {
				return changed, err
			}
		}
		staged := m.path(link[0]) + ".ryoku-new"
		_ = os.Remove(staged)
		if err := os.Symlink(link[1], staged); err != nil {
			return changed, fmt.Errorf("link %s to %s: %w", link[0], link[1], err)
		}
		if err := os.Rename(staged, m.path(link[0])); err != nil {
			_ = os.Remove(staged)
			return changed, fmt.Errorf("link %s to %s: %w", link[0], link[1], err)
		}
		changed = true
	}
	return changed, nil
}
