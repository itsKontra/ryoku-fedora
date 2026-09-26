package updater

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mountinfo as a Fedora install with a separate /boot and the kickstart's
// root, home and snapshots subvolumes shows it.
const fedoraMountInfo = `43 1 0:35 /root / rw,relatime shared:1 - btrfs /dev/mapper/luks-1 rw,seclabel,compress=zstd:1,subvol=/root
61 43 0:35 /home /home rw,relatime shared:2 - btrfs /dev/mapper/luks-1 rw,seclabel,subvol=/home
62 43 0:35 /snapshots /.snapshots rw,relatime shared:3 - btrfs /dev/mapper/luks-1 rw,seclabel,subvol=/snapshots
73 43 259:2 / /boot rw,relatime shared:167 - ext4 /dev/vda2 rw,seclabel
`

func TestParseMountInfoReadsSubvolumes(t *testing.T) {
	m := parseMountInfo(fedoraMountInfo)
	if got := m["/"]; got.root != "/root" || got.fstype != "btrfs" || got.source != "/dev/mapper/luks-1" {
		t.Fatalf("root mount = %+v", got)
	}
	if got := m["/.snapshots"].root; got != "/snapshots" {
		t.Fatalf("snapshots subvolume = %q", got)
	}
	if got := m["/boot"]; got.fstype != "ext4" || got.source != "/dev/vda2" {
		t.Fatalf("boot mount = %+v", got)
	}
}

func TestEntryOptionsSwapTheSubvolumeOnly(t *testing.T) {
	opts, flags := baseSnapshotOptions("BOOT_IMAGE=(hd0,gpt2)/vmlinuz-6.17 root=UUID=abc ro rootflags=subvol=root,compress=zstd:1 rd.luks.uuid=luks-1 rhgb quiet systemd.unit=multi-user.target rd.ryoku.preview=x")
	if opts != "root=UUID=abc rd.luks.uuid=luks-1 rhgb quiet" {
		t.Fatalf("base options = %q", opts)
	}
	if !reflect.DeepEqual(flags, []string{"compress=zstd:1"}) {
		t.Fatalf("rootflags kept = %q", flags)
	}
	l := menuLayout{options: opts, rootflags: flags}
	got := l.entryOptions("snapshots/42/snapshot", "rd.ryoku.preview=snapshots/42/snapshot")
	want := "root=UUID=abc rd.luks.uuid=luks-1 rhgb quiet ro rootflags=subvol=snapshots/42/snapshot,compress=zstd:1 rd.ryoku.preview=snapshots/42/snapshot"
	if got != want {
		t.Fatalf("entry options\n got %q\nwant %q", got, want)
	}
}

func TestCompareVersionsOrdersFedoraKernels(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"6.17.1-300.fc44.x86_64", "6.9.12-200.fc44.x86_64", 1},
		{"6.17.1-300.fc44.x86_64", "6.17.10-300.fc44.x86_64", -1},
		{"6.17.1-300.fc44.x86_64", "6.17.1-300.fc44.x86_64", 0},
		{"6.17.1-301.fc44.x86_64", "6.17.1-300.fc44.x86_64", 1},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign of %d", c.a, c.b, got, c.want)
		}
	}
}

func writeSnapshot(t *testing.T, dir, num, kind, desc string, kvers ...string) {
	t.Helper()
	base := filepath.Join(dir, num)
	for _, k := range kvers {
		mod := filepath.Join(base, "snapshot/usr/lib/modules", k)
		if err := os.MkdirAll(mod, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mod, "vmlinuz"), []byte("kernel"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	info := `<?xml version="1.0"?>
<snapshot>
  <type>` + kind + `</type>
  <num>` + num + `</num>
  <date>2026-09-20 10:11:12</date>
  <description>` + desc + `</description>
  <cleanup>number</cleanup>
</snapshot>
`
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "info.xml"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadBootSnapshotsNewestFirstWithTheirKernel(t *testing.T) {
	dir := t.TempDir()
	writeSnapshot(t, dir, "9", "pre", "ryoku update", "6.16.9-200.fc44.x86_64")
	writeSnapshot(t, dir, "10", "post", "ryoku update", "6.16.9-200.fc44.x86_64", "6.17.1-300.fc44.x86_64")
	writeSnapshot(t, dir, "11", "single", "no kernel")
	if err := os.MkdirAll(filepath.Join(dir, "12"), 0o755); err != nil {
		t.Fatal(err)
	}
	snaps := readBootSnapshots(dir)
	var got []string
	for _, s := range snaps {
		got = append(got, s.Num+"="+s.kver)
	}
	want := []string{"11=", "10=6.17.1-300.fc44.x86_64", "9=6.16.9-200.fc44.x86_64"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshots = %q, want %q", got, want)
	}
	if snaps[1].Kind != "post" || snaps[1].Description != "ryoku update" || snaps[1].Date != "2026-09-20 10:11:12" {
		t.Fatalf("info.xml not read: %+v", snaps[1])
	}
}

func TestRenderSnapshotMenu(t *testing.T) {
	l := menuLayout{bootUUID: "b00t", options: "root=UUID=abc rhgb quiet", rootSubvol: "root", snapSubvol: "snapshots"}
	snaps := []bootSnapshot{{Num: "42", Kind: "pre", Date: "2026-09-20 10:11:12", Description: "it's ryoku update", kver: "6.17.1-300.fc44.x86_64"}}
	cfg := renderSnapshotMenu(snaps, l)
	for _, want := range []string{
		"submenu 'Ryoku snapshots' --id ryoku-snapshots {\n",
		"  menuentry 'Snapshot 42, 2026-09-20 10:11 UTC, pre its ryoku update: look (read-only)' --id ryoku-snapshot-42-preview {\n",
		"    search --no-floppy --fs-uuid --set=root b00t\n",
		"    linux /ryoku/snapshots/6.17.1-300.fc44.x86_64/vmlinuz root=UUID=abc rhgb quiet ro rootflags=subvol=snapshots/42/snapshot rd.ryoku.preview=snapshots/42/snapshot\n",
		"    initrd /ryoku/snapshots/6.17.1-300.fc44.x86_64/initramfs.img\n",
		" --id ryoku-snapshot-42-restore {\n",
		"    linux /ryoku/snapshots/6.17.1-300.fc44.x86_64/vmlinuz root=UUID=abc rhgb quiet ro rootflags=subvol=root rd.ryoku.restore=snapshots/42/snapshot\n",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("menu lacks %q\n%s", want, cfg)
		}
	}
	if got := strings.Count(cfg, "{"); got != strings.Count(cfg, "}") {
		t.Fatalf("unbalanced braces in\n%s", cfg)
	}
	if empty := renderSnapshotMenu(nil, l); strings.Contains(empty, "submenu") {
		t.Fatalf("an empty menu still opens a submenu:\n%s", empty)
	}
}

func TestSnapshotEntryIDIsTheGrubRebootPath(t *testing.T) {
	if got := snapshotEntryID("42", "restore"); got != "ryoku-snapshots>ryoku-snapshot-42-restore" {
		t.Fatalf("entry id = %q", got)
	}
	cfg := renderSnapshotMenu([]bootSnapshot{{Num: "42", kver: "k"}}, menuLayout{})
	for _, action := range []string{"preview", "restore"} {
		id := strings.TrimPrefix(snapshotEntryID("42", action), snapshotMenuID+">")
		if !strings.Contains(cfg, " --id "+id+" ") {
			t.Errorf("menu has no entry %s", id)
		}
	}
}

func TestBLSKernelsSkipsRescueAndTwins(t *testing.T) {
	dir := t.TempDir()
	mid := "0123456789abcdef"
	for _, name := range []string{
		mid + "-6.17.1-300.fc44.x86_64.conf",
		mid + "-6.17.1-300.fc44.x86_64~ryokuconsole.conf",
		mid + "-6.17.1-300.fc44.x86_64~debug.conf",
		mid + "-0-rescue-" + mid + ".conf",
		mid + "-6.16.9-200.fc44.x86_64.conf",
		"otherid-6.1.0.conf",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := blsKernels(dir, mid)
	want := []string{"6.16.9-200.fc44.x86_64", "6.17.1-300.fc44.x86_64"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kernels = %q, want %q", got, want)
	}
}
