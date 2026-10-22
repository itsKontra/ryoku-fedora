package kmod

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// akmodsGroup is the group akmods builds and signs as. Its presence on the
// machine decides whether the key must be readable by it.
const akmodsGroup = "akmods"

// accounts is the machine's /etc/passwd and /etc/group, read from its root
// rather than through NSS, so a target root resolves its own ids.
type accounts struct {
	users  map[int]string
	groups map[int]string
	gids   map[string]int
}

func (m *Machine) accounts() accounts {
	acc := accounts{users: map[int]string{}, groups: map[int]string{}, gids: map[string]int{}}
	readIDs(m.path("/etc/passwd"), 2, func(name string, id int) { acc.users[id] = name })
	readIDs(m.path("/etc/group"), 2, func(name string, id int) {
		acc.groups[id] = name
		acc.gids[name] = id
	})
	return acc
}

func readIDs(path string, field int, add func(string, int)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) <= field || fields[0] == "" {
			continue
		}
		if id, err := strconv.Atoi(fields[field]); err == nil {
			add(fields[0], id)
		}
	}
}

func (a accounts) owner(uid, gid int) string {
	return nameOr(a.users, uid) + ":" + nameOr(a.groups, gid)
}

func nameOr(names map[int]string, id int) string {
	if name, ok := names[id]; ok {
		return name
	}
	return strconv.Itoa(id)
}
