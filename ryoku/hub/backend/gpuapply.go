package main

// gpuapply.go = the one-time, reversible "enable passthrough" + its undo.
// installs the stack, writes a small set of idempotent /etc files (kvmfr
// autoload + perms, the libvirt hook, a polkit rule), adds the user to
// libvirt/kvm, enables libvirtd, and -- only on an Intel host with IOMMU off --
// adds the kernel cmdline token. everything `enable` writes, `disable` removes.
// runs under pkexec (the lock.go pattern); a --dry-run prints the exact plan
// without touching anything, which is what the Hub shows before the user OKs.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func runGpuApply(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("gpu apply needs enable|disable [--dry-run]")
	}
	action := args[0]
	if action != "enable" && action != "disable" {
		return fmt.Errorf("gpu apply: action must be enable or disable")
	}
	dryRun := false
	for _, a := range args[1:] {
		if a == "--dry-run" {
			dryRun = true
		}
	}
	// hook = an internal entrypoint libvirt calls. same subcommand tree, but it
	// has to route before the privilege dance.
	if dryRun {
		return applyPlan(action, invokingUser(), selfExe(), true)
	}
	if os.Geteuid() != 0 {
		return escalateApply(args)
	}
	return applyPlan(action, invokingUser(), selfExe(), false)
}

// escalateApply re-runs the gpu-apply subcommand as root via pkexec.
func escalateApply(args []string) error {
	return escalateSelf(append([]string{"gpu", "apply"}, args...)...)
}

// escalateSelf re-runs this binary as root via pkexec, preserving the invoking
// user's id (PKEXEC_UID) so the privileged half acts on the right user -- set
// group membership, own the udev node (the lock.go greeter pattern).
func escalateSelf(args ...string) error {
	exe := selfExe()
	uid := strconv.Itoa(os.Getuid())
	full := append([]string{"env", "PKEXEC_UID=" + uid, exe}, args...)
	cmd := exec.Command("pkexec", full...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func ttyRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// missingPkgs returns the packages not yet installed, order preserved.
func missingPkgs(pkgs []string, installed func(string) bool) []string {
	var m []string
	for _, p := range pkgs {
		if !installed(p) {
			m = append(m, p)
		}
	}
	return m
}

func selfExe() string {
	if e, err := os.Executable(); err == nil {
		return e
	}
	return "ryoku-hub"
}

// invokingUser = the human behind the action. PKEXEC_UID when escalated, else
// the current user's name.
func invokingUser() string {
	if u := os.Getenv("PKEXEC_UID"); u != "" {
		if name := userNameByID(u); name != "" {
			return name
		}
	}
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	return os.Getenv("USER")
}

func userNameByID(uid string) string {
	out, err := exec.Command("id", "-nu", uid).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type managedFile struct {
	rel        string
	content    string
	mode       os.FileMode
	needsKvmfr bool // only after the kvmfr module is installed
}

func managedFiles(user, exe string) []managedFile {
	gpuBin := exe // ryoku-hub. the hook also needs ryoku-gpu, resolved below.
	ryokuGpu := "ryoku-gpu"
	if p, err := exec.LookPath("ryoku-gpu"); err == nil {
		ryokuGpu = p
	}
	hook := "#!/bin/bash\n" +
		"# Managed by Ryoku (ryoku-hub gpu apply). libvirt calls this for every domain;\n" +
		"# we forward the Ryoku VM's prepare/release to ryoku-hub, which binds the dGPU to\n" +
		"# vfio-pci on start and hands it back on stop.\n" +
		"export RYOKU_GPU_BIN=" + shellQuote(ryokuGpu) + "\n" +
		"guest=\"$1\"; op=\"$2\"\n" +
		"case \"$op\" in\n" +
		"  prepare) " + shellQuote(gpuBin) + " gpu hook prepare \"$guest\" ;;\n" +
		"  release|stopped) " + shellQuote(gpuBin) + " gpu hook release \"$guest\" ;;\n" +
		"esac\n" +
		"exit 0\n"
	// QEMU runs confined as svirt_t, which SELinux denies on a plain device_t
	// node; svirt_image_t at s0 is the label libvirt gives shared disks, which
	// every VM may open and map. udev ignores SECLABEL where SELinux is off.
	udev := fmt.Sprintf("SUBSYSTEM==\"kvmfr\", OWNER=\"%s\", GROUP=\"kvm\", MODE=\"0660\", SECLABEL{selinux}=\"system_u:object_r:svirt_image_t:s0\"\n", user)
	return []managedFile{
		{"etc/modules-load.d/ryoku-kvmfr.conf", "kvmfr\n", 0o644, true},
		{"etc/modprobe.d/ryoku-kvmfr.conf", fmt.Sprintf("options kvmfr static_size_mb=%d\n", kvmfrStaticMB), 0o644, true},
		{"etc/udev/rules.d/99-ryoku-kvmfr.rules", udev, 0o644, true},
		{"etc/polkit-1/rules.d/50-ryoku-libvirt.rules", polkitRule, 0o644, false},
		{"etc/libvirt/hooks/qemu", hook, 0o755, false},
	}
}

// kvmfrModuleAvailable: is the kvmfr kernel module actually installed? a
// partial enable (no Looking Glass yet) must not write a modules-load entry
// that would fail at every boot.
func kvmfrModuleAvailable() bool {
	return exec.Command("modinfo", "kvmfr").Run() == nil
}

const polkitRule = `// Managed by Ryoku. Let the libvirt group manage libvirt without a password so the
// Ryoku VM launches straight from the app launcher.
polkit.addRule(function(action, subject) {
  if (action.id == "org.libvirt.unix.manage" && subject.isInGroup("libvirt")) {
    return polkit.Result.YES;
  }
});
`

// kvmfrStaticMB = Looking Glass shared-memory size in MiB, written to the kvmfr
// module's static_size_mb. 128 MiB covers SDR panels up to 2160p and most
// ultrawides; bumping it just locks down that RAM. Only the passthrough stack
// uses it (a passthrough VM is launched outside Ryoku, e.g. via libvirt).
const kvmfrStaticMB = 128

// the passthrough stack. core packages are official, install as one
// transaction; the Looking Glass pieces are not in Fedora and install
// best-effort from COPR, so their absence never blocks the core set.
func corePkgs() []string {
	if hasCommand("dnf") && !hasCommand("pacman") {
		return []string{"qemu-kvm", "libvirt-daemon-kvm", "edk2-ovmf", "swtpm", "dnsmasq"}
	}
	return []string{"qemu-desktop", "libvirt", "edk2-ovmf", "swtpm", "dnsmasq"}
}

var extraPassthroughPkgs = []string{"looking-glass-client", "akmod-kvmfr"}

// passthroughCOPRs carry extraPassthroughPkgs: the kvmfr akmod and the client.
var passthroughCOPRs = []string{"hikariknight/looking-glass-kvmfr", "pgaskin/looking-glass-client"}

// installPassthroughExtras enables the COPRs and installs the missing Looking
// Glass pieces. akmod-kvmfr builds the module at boot for each new kernel;
// akmods --force builds it for the running kernel now, so enable can write the
// kvmfr config without a reboot.
func installPassthroughExtras(missing []string) error {
	if err := dnfRun("install", "-y", "dnf5-plugins"); err != nil {
		return err
	}
	for _, c := range passthroughCOPRs {
		if err := dnfRun("copr", "enable", "-y", c); err != nil {
			return err
		}
	}
	if err := dnfRun(append([]string{"install", "-y"}, missing...)...); err != nil {
		return err
	}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		run("akmods", "--force", "--kernels", strings.TrimSpace(string(out)))
	}
	return nil
}

func dnfRun(args ...string) error {
	cmd := exec.Command("dnf", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func applyPlan(action, user, exe string, dryRun bool) error {
	files := managedFiles(user, exe)
	root := etcRoot()
	say := func(s string) { fmt.Println(planPrefix(dryRun) + s) }

	if action == "enable" {
		pkgs := corePkgs()
		say("install packages: " + strings.Join(pkgs, " "))
		if !dryRun {
			snapshot("ryoku gpu passthrough enable")
			if err := pacmanInstall(pkgs); err != nil {
				return fmt.Errorf("installing the passthrough stack failed: %w (update the system with `ryoku update`, then retry)", err)
			}
		}
		switch missing := missingPkgs(extraPassthroughPkgs, pkgInstalled); {
		case len(missing) == 0:
			say("Looking Glass + kvmfr: installed")
		case dryRun:
			say("enable COPRs: " + strings.Join(passthroughCOPRs, " "))
			say("install from COPR: " + strings.Join(missing, " "))
		default:
			say("install from COPR: " + strings.Join(missing, " "))
			if err := installPassthroughExtras(missing); err != nil {
				say("could not install " + strings.Join(missing, " ") + ": " + err.Error() + "; passthrough stays off until they are")
			}
		}
		kvmfrOK := dryRun || kvmfrModuleAvailable()
		for _, f := range files {
			if f.needsKvmfr && !kvmfrOK {
				say("skip /" + f.rel + " (kvmfr module not installed; re-run enable after adding Looking Glass)")
				continue
			}
			say("write /" + f.rel)
			if !dryRun {
				if err := writeManaged(root, f); err != nil {
					return err
				}
			}
		}
		if kvmfrOK {
			say("allow " + kvmfrDevice + " in /" + qemuConfRel + " cgroup_device_acl")
			if !dryRun {
				if ok, err := applyKvmfrACL(root, true); err != nil {
					say("could not edit /" + qemuConfRel + ": " + err.Error())
				} else if !ok {
					say("/" + qemuConfRel + " sets its own cgroup_device_acl; add \"" + kvmfrDevice + "\" to it, or the VM cannot open Looking Glass")
				}
			}
		}
		say("add " + user + " to groups: libvirt, kvm")
		say("enable libvirtd.socket and the default network")
		if !dryRun {
			run("gpasswd", "-a", user, "libvirt")
			run("gpasswd", "-a", user, "kvm")
			run("systemctl", "enable", "--now", "libvirtd.socket")
			run("udevadm", "control", "--reload-rules")
			run("udevadm", "trigger", "--subsystem-match=kvmfr")
			run("virsh", "net-autostart", "default")
		}
		if kvmfrOK {
			say("done. Log out and back in for group membership to take effect.")
		} else {
			say("core stack installed, but Looking Glass / kvmfr are missing, so passthrough stays off. Install them, then run enable again.")
		}
		return nil
	}

	// disable: remove exactly what enable wrote.
	for _, f := range files {
		say("remove /" + f.rel)
		if !dryRun {
			_ = os.Remove(filepath.Join(root, f.rel))
		}
	}
	say("remove the kvmfr block from /" + qemuConfRel)
	if !dryRun {
		if _, err := applyKvmfrACL(root, false); err != nil {
			say("could not edit /" + qemuConfRel + ": " + err.Error())
		}
	}
	say("remove " + user + " from groups: libvirt, kvm")
	if !dryRun {
		run("gpasswd", "-d", user, "libvirt")
		run("gpasswd", "-d", user, "kvm")
		run("udevadm", "control", "--reload-rules")
	}
	say("done. The discrete GPU returns to the host on the next boot.")
	return nil
}

func planPrefix(dryRun bool) string {
	if dryRun {
		return "[plan] "
	}
	return "[apply] "
}

func writeManaged(root string, f managedFile) error {
	p := filepath.Join(root, f.rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if b, err := os.ReadFile(p); err == nil && string(b) == f.content {
		return nil // idempotent: already correct
	}
	return os.WriteFile(p, []byte(f.content), f.mode)
}

func etcRoot() string {
	if r := os.Getenv("RYOKU_ETC_ROOT"); r != "" {
		return r
	}
	return "/"
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pacmanInstall(pkgs []string) error {
	var cmd *exec.Cmd
	if hasCommand("pacman") {
		cmd = exec.Command("pacman", append([]string{"-S", "--needed", "--noconfirm"}, pkgs...)...)
	} else if hasCommand("dnf") {
		cmd = exec.Command("dnf", append([]string{"install", "-y"}, pkgs...)...)
	} else if hasCommand("apt-get") {
		cmd = exec.Command("apt-get", append([]string{"install", "-y"}, pkgs...)...)
	} else {
		return fmt.Errorf("no supported package manager found")
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// pkgInstalled: is this package locally installed? stays quiet (output discarded).
func pkgInstalled(p string) bool {
	if hasCommand("pacman") {
		return exec.Command("pacman", "-Q", p).Run() == nil
	}
	if hasCommand("rpm") {
		return exec.Command("rpm", "-q", "--quiet", p).Run() == nil
	}
	if hasCommand("dpkg-query") {
		return exec.Command("dpkg-query", "-W", "-f=${Status}", p).Run() == nil
	}
	return false
}

func snapshot(desc string) {
	if _, err := exec.LookPath("snapper"); err != nil {
		return
	}
	// -c number tags the snapshot for snapper's number cleanup so it counts
	// against NUMBER_LIMIT and prunes like update snapshots; without it these piled
	// up unbounded. Enforce the cap right after creating.
	run("snapper", "-c", "root", "create", "-c", "number", "--description", desc)
	run("snapper", "-c", "root", "cleanup", "number")
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	_ = cmd.Run() // best-effort; one missing tool never aborts the rest
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
