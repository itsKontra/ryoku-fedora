package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The passthrough domain hands QEMU /dev/kvmfr0 through the qemu namespace, a
// device libvirt's cgroup filter does not know about, so the VM cannot start
// until the device is in cgroup_device_acl. libvirt reads that list only from
// qemu.conf, so enable appends a marked block and disable strips it again.

const (
	qemuConfRel    = "etc/libvirt/qemu.conf"
	qemuConfBegin  = "# BEGIN ryoku kvmfr (managed by ryoku-hub gpu apply)"
	qemuConfEnd    = "# END ryoku kvmfr"
	kvmfrDevice    = "/dev/kvmfr0"
	qemuConfPreset = `cgroup_device_acl = [
    "/dev/null", "/dev/full", "/dev/zero",
    "/dev/random", "/dev/urandom",
    "/dev/ptmx", "/dev/kvm", "/dev/userfaultfd",
    "` + kvmfrDevice + `"
]`
)

var activeDeviceACL = regexp.MustCompile(`(?m)^\s*cgroup_device_acl\s*=`)

// withKvmfrACL returns qemu.conf with the kvmfr block added. ok is false when
// the admin already set their own cgroup_device_acl without kvmfr0: a second
// assignment would override theirs, so that list is theirs to extend.
func withKvmfrACL(conf string) (out string, ok bool) {
	if strings.Contains(conf, qemuConfBegin) {
		return conf, true
	}
	if activeDeviceACL.MatchString(conf) {
		return conf, strings.Contains(conf, `"`+kvmfrDevice+`"`)
	}
	if conf != "" && !strings.HasSuffix(conf, "\n") {
		conf += "\n"
	}
	return conf + qemuConfBegin + "\n" + qemuConfPreset + "\n" + qemuConfEnd + "\n", true
}

func withoutKvmfrACL(conf string) string {
	start := strings.Index(conf, qemuConfBegin)
	if start < 0 {
		return conf
	}
	end := strings.Index(conf[start:], qemuConfEnd)
	if end < 0 {
		return conf
	}
	end += start + len(qemuConfEnd)
	if end < len(conf) && conf[end] == '\n' {
		end++
	}
	return conf[:start] + conf[end:]
}

// applyKvmfrACL edits qemu.conf under root and reports whether the ACL now
// admits kvmfr0. The libvirt daemons read qemu.conf only at start.
func applyKvmfrACL(root string, enable bool) (bool, error) {
	p := filepath.Join(root, qemuConfRel)
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	conf := string(b)
	next, ok := conf, true
	if enable {
		next, ok = withKvmfrACL(conf)
	} else {
		next = withoutKvmfrACL(conf)
	}
	if next == conf {
		return ok, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(p, []byte(next), 0o600); err != nil {
		return false, err
	}
	restartLibvirt()
	return ok, nil
}

var restartLibvirt = func() { run("systemctl", "try-restart", "virtqemud.service", "libvirtd.service") }
