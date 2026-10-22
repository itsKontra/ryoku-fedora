package kmod

import (
	"errors"
	"io/fs"
	"os"
)

// KeyState is the module signing key's private half as found on the machine,
// next to the mode and owner it should have. akmods signs as its own user, so
// wherever the akmods group exists the key is 0640 root:akmods; elsewhere it
// is 0600 root:root.
type KeyState struct {
	Present   bool
	Mode      fs.FileMode
	Owner     string
	WantMode  fs.FileMode
	WantOwner string

	uid, gid         int
	wantUID, wantGID int
}

func (k KeyState) OK() bool {
	return k.Present && k.Mode == k.WantMode && k.uid == k.wantUID && k.gid == k.wantGID
}

func (m *Machine) keyState() (KeyState, error) {
	acc := m.accounts()
	key := KeyState{WantMode: 0o600}
	if gid, ok := acc.gids[akmodsGroup]; ok {
		key.WantMode, key.wantGID = 0o640, gid
	}
	key.WantOwner = acc.owner(key.wantUID, key.wantGID)

	info, err := os.Stat(m.path(KeyPath))
	if errors.Is(err, fs.ErrNotExist) {
		return key, nil
	}
	if err != nil {
		return key, err
	}
	uid, gid, err := m.cfg.Owners.Owner(m.path(KeyPath))
	if err != nil {
		return key, err
	}
	key.Present = true
	key.Mode = info.Mode().Perm()
	key.uid, key.gid = uid, gid
	key.Owner = acc.owner(uid, gid)
	return key, nil
}
