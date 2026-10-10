package keyboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const machineID = "0123456789abcdef0123456789abcdef"

// bootFixture points the boot image search at boot (the /boot initramfs dir)
// and esp (an EFI/Linux dir), with layout written to a kernel-install config
// when it is not empty.
func bootFixture(t *testing.T, boot, esp, layout string) {
	t.Helper()
	og, ou, oc := BootImageGlobs, UKIGlobs, InstallConfPaths
	t.Cleanup(func() { BootImageGlobs, UKIGlobs, InstallConfPaths = og, ou, oc })
	BootImageGlobs = []string{filepath.Join(boot, "initramfs-*.img")}
	UKIGlobs = []string{filepath.Join(esp, "*.efi")}
	conf := filepath.Join(t.TempDir(), "install.conf")
	if layout != "" {
		if err := os.WriteFile(conf, []byte("#layout=bls\nlayout="+layout+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	InstallConfPaths = []string{conf}
}

// denyDir makes dir unreadable for the test's lifetime, the way Fedora's ESP
// (vfat, dmask=0077) is for a normal user.
func denyDir(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory; permission denial cannot be simulated")
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// dracut's rescue initramfs is regenerated on its own schedule, so it must
// never be the image freshness is judged against.
func TestBootImageTimeIgnoresInitramfsRescue(t *testing.T) {
	boot := t.TempDir()
	base := time.Now().Add(-2 * time.Hour)
	touch(t, filepath.Join(boot, "initramfs-6.17.1-300.fc44.x86_64.img"), base)
	touch(t, filepath.Join(boot, "initramfs-6.18.0-200.fc44.x86_64.img"), base.Add(30*time.Minute))
	touch(t, filepath.Join(boot, "initramfs-0-rescue-"+machineID+".img"), base.Add(90*time.Minute))
	bootFixture(t, boot, t.TempDir(), "")

	when, which, denied := BootImageTime()
	if filepath.Base(which) != "initramfs-6.18.0-200.fc44.x86_64.img" {
		t.Errorf("picked %q, want the newest non-rescue image", filepath.Base(which))
	}
	if !when.Equal(base.Add(30 * time.Minute)) {
		t.Errorf("time = %v, want the newest non-rescue mtime", when)
	}
	if denied != "" {
		t.Errorf("denied = %q, want none", denied)
	}
}

// kernel-install's uki layout names its rescue image <machine-id>-0-rescue.efi;
// it is skipped the same way.
func TestBootImageTimeIgnoresUKIRescue(t *testing.T) {
	esp := t.TempDir()
	base := time.Now().Add(-2 * time.Hour)
	uki := machineID + "-6.18.0-200.fc44.x86_64.efi"
	touch(t, filepath.Join(esp, uki), base.Add(30*time.Minute))
	touch(t, filepath.Join(esp, machineID+"-0-rescue.efi"), base.Add(90*time.Minute))
	bootFixture(t, t.TempDir(), esp, "uki")

	if _, which, _ := BootImageTime(); filepath.Base(which) != uki {
		t.Errorf("picked %q, want %q", filepath.Base(which), uki)
	}
}

// With layout=uki the initramfs left in /boot is not what boots, so the UKI
// answers even when an initramfs is newer.
func TestBootImageTimeUKILayoutSkipsInitramfs(t *testing.T) {
	boot, esp := t.TempDir(), t.TempDir()
	base := time.Now().Add(-2 * time.Hour)
	uki := machineID + "-6.18.0-200.fc44.x86_64.efi"
	touch(t, filepath.Join(esp, uki), base)
	touch(t, filepath.Join(boot, "initramfs-6.18.0-200.fc44.x86_64.img"), base.Add(time.Hour))
	bootFixture(t, boot, esp, "uki")

	if _, which, _ := BootImageTime(); filepath.Base(which) != uki {
		t.Errorf("picked %q, want the UKI", filepath.Base(which))
	}
}

// No initramfs in /boot and no layout set: the UKI answers.
func TestBootImageTimeFallsBackToUKI(t *testing.T) {
	esp := t.TempDir()
	uki := machineID + "-6.18.0-200.fc44.x86_64.efi"
	touch(t, filepath.Join(esp, uki), time.Now().Add(-time.Hour))
	bootFixture(t, t.TempDir(), esp, "")

	if _, which, _ := BootImageTime(); filepath.Base(which) != uki {
		t.Errorf("picked %q, want the UKI", filepath.Base(which))
	}
}

// A GRUB box boots the initramfs in /boot, so an ESP the user cannot read does
// not make the answer unknown.
func TestBootStaleKnownDespiteUnreadableESP(t *testing.T) {
	boot, esp := t.TempDir(), t.TempDir()
	base := time.Now().Add(-time.Hour)
	touch(t, filepath.Join(boot, "initramfs-6.18.0-200.fc44.x86_64.img"), base.Add(20*time.Minute))
	vc := filepath.Join(t.TempDir(), "vconsole.conf")
	touch(t, vc, base)
	bootFixture(t, boot, esp, "")
	denyDir(t, esp)
	ov := VconsolePath
	VconsolePath = vc
	t.Cleanup(func() { VconsolePath = ov })

	b := BootStale()
	if b.Unknown() || b.Stale || b.Image == "" {
		t.Errorf("got %+v, want a current initramfs", b)
	}
}

// The UKI lives on an ESP this user cannot read: the boot prompt was not
// checked, so it must read as unknown, never as current.
func TestBootStaleUnknownWhenUKIDirUnreadable(t *testing.T) {
	esp := t.TempDir()
	touch(t, filepath.Join(esp, machineID+"-6.18.0-200.fc44.x86_64.efi"), time.Now())
	bootFixture(t, t.TempDir(), esp, "uki")
	denyDir(t, esp)

	b := BootStale()
	if !b.Unknown() || b.Stale {
		t.Errorf("got %+v, want unknown", b)
	}
	if b.Denied != esp {
		t.Errorf("denied = %q, want %q", b.Denied, esp)
	}
}

// The trap this whole package exists for: every file under /etc can agree while
// the passphrase prompt is still wrong, because the image carries its own copy.
func TestBootStaleWhenVconsoleIsNewer(t *testing.T) {
	boot := t.TempDir()
	base := time.Now().Add(-time.Hour)
	img := filepath.Join(boot, "initramfs-6.17.1-300.fc44.x86_64.img")
	vc := filepath.Join(t.TempDir(), "vconsole.conf")
	touch(t, img, base)
	touch(t, vc, base.Add(10*time.Minute))
	bootFixture(t, boot, t.TempDir(), "")
	ov := VconsolePath
	VconsolePath = vc
	t.Cleanup(func() { VconsolePath = ov })

	b := BootStale()
	if !b.Stale {
		t.Error("a boot image older than vconsole.conf must read as stale")
	}
	if b.Image != img {
		t.Errorf("image = %q", b.Image)
	}

	touch(t, img, base.Add(20*time.Minute))
	if BootStale().Stale {
		t.Error("a boot image newer than vconsole.conf must not read as stale")
	}
}
