package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKvmfrACLRoundTrip(t *testing.T) {
	stock := "#cgroup_device_acl = [\n#    \"/dev/null\"\n#]\nuser = \"qemu\""
	added, ok := withKvmfrACL(stock)
	if !ok || !strings.Contains(added, `"/dev/kvmfr0"`) {
		t.Fatalf("enable did not add kvmfr0:\n%s", added)
	}
	if again, _ := withKvmfrACL(added); again != added {
		t.Error("enable is not idempotent")
	}
	if got := withoutKvmfrACL(added); got != stock+"\n" {
		t.Errorf("disable left %q, want the stock file back", got)
	}
}

func TestKvmfrACLRespectsAdminList(t *testing.T) {
	own := "cgroup_device_acl = [ \"/dev/null\" ]\n"
	if out, ok := withKvmfrACL(own); ok || out != own {
		t.Errorf("an admin list without kvmfr0 must be left alone and reported; ok=%v", ok)
	}
	withDev := "cgroup_device_acl = [ \"/dev/null\", \"/dev/kvmfr0\" ]\n"
	if out, ok := withKvmfrACL(withDev); !ok || out != withDev {
		t.Errorf("an admin list that already has kvmfr0 is fine as is; ok=%v", ok)
	}
}

func TestApplyKvmfrACLWritesUnderRoot(t *testing.T) {
	root := t.TempDir()
	restarts := 0
	restartLibvirt = func() { restarts++ }
	if ok, err := applyKvmfrACL(root, true); err != nil || !ok {
		t.Fatalf("enable on a missing qemu.conf: ok=%v err=%v", ok, err)
	}
	b, _ := os.ReadFile(filepath.Join(root, qemuConfRel))
	if !strings.Contains(string(b), qemuConfBegin) {
		t.Fatalf("qemu.conf lacks the block:\n%s", b)
	}
	if _, err := applyKvmfrACL(root, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, qemuConfRel)); len(b) != 0 {
		t.Errorf("disable left %q", b)
	}
	if restarts != 2 {
		t.Errorf("libvirt restarted %d times, want once per change", restarts)
	}
}
