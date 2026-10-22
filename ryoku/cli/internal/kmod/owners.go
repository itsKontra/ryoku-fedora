package kmod

import (
	"fmt"
	"os"
	"syscall"
)

// Owners reads and sets the numeric owner of a file.
type Owners interface {
	Owner(path string) (uid, gid int, err error)
	Chown(path string, uid, gid int) error
}

type osOwners struct{}

func (osOwners) Owner(path string) (int, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("no owner information for %s", path)
	}
	return int(stat.Uid), int(stat.Gid), nil
}

func (osOwners) Chown(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}
