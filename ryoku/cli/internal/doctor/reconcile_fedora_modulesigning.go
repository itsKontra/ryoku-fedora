package doctor

import (
	"strings"

	"ryoku-cli/internal/kmod"
	i18n "ryoku-i18n"
)

var (
	prepareModuleSigningKeyCommand      = kmod.Advice("prepare-key")
	queueModuleSigningEnrollmentCommand = kmod.Advice("queue-enrollment")
)

// The key is root-only, so doctor reads its state as the desktop user and asks
// for sudo only to change it, through the same ryoku-host verbs the installers
// use. Variables so tests point them at a fake root.
var (
	moduleSigningStatus = func() (kmod.Status, error) {
		return kmod.New(kmod.Config{}).Status()
	}
	prepareModuleSigningKey = func() error {
		return runKmodAsRoot("prepare-key")
	}
	queueModuleSigningEnrollment = func() error {
		return runKmodAsRoot("queue-enrollment")
	}
)

// moduleSigningChecks run in this order: the key must be readable by akmods,
// and akmods must sign with it, before anything is rebuilt with it.
func moduleSigningChecks() []reconciler {
	pass := &moduleSigningPass{}
	return []reconciler{
		{i18n.T("module signing key permissions"), pass.track(reconcileModuleSigningKey)},
		{i18n.T("akmods links to the module signing key"), pass.track(reconcileAkmodsKeyLinks)},
		{i18n.T("module signing key enrollment"), reconcileModuleSigningEnrollment},
		{i18n.T("NVIDIA driver fallback"), pass.reconcileDriverFallback},
	}
}

func runKmodAsRoot(verb string) error {
	_, _, err := kmod.AsRoot{Host: ryokuHostBin()}.Run(verb)
	return err
}

func reconcileModuleSigningKey(checkOnly bool) recResult {
	status, err := moduleSigningStatus()
	if err != nil {
		return warnRes(i18n.T("could not read the module signing key %s: %v"), kmod.KeyPath, err)
	}
	key := status.Key
	if !key.Present {
		return okRes(i18n.T("no module signing key yet; the first module DKMS or akmods builds creates it"))
	}
	if key.OK() {
		return okRes(i18n.T("the module signing key %s is %04o %s"), kmod.KeyPath, key.Mode, key.Owner)
	}
	if checkOnly {
		return wouldRes("%s", moduleSigningKeyDrift(key)).withFix("%s", prepareModuleSigningKeyCommand)
	}
	if err := prepareModuleSigningKey(); err != nil {
		return failRes(i18n.T("could not fix the module signing key %s: %v"), kmod.KeyPath, err).
			withFix("%s", prepareModuleSigningKeyCommand)
	}
	status, err = moduleSigningStatus()
	if err != nil || !status.Key.OK() {
		return failRes(i18n.T("the module signing key %s is still not %04o %s"), kmod.KeyPath, key.WantMode, key.WantOwner).
			withFix("%s", prepareModuleSigningKeyCommand)
	}
	return fixedRes(i18n.T("set the module signing key %s to %04o %s"), kmod.KeyPath, key.WantMode, key.WantOwner)
}

// moduleSigningKeyDrift says which way the key is wrong: open to more than
// root and the akmods group, or closed to akmods, which then cannot sign.
func moduleSigningKeyDrift(key kmod.KeyState) string {
	user, _, _ := strings.Cut(key.Owner, ":")
	wantUser, _, _ := strings.Cut(key.WantOwner, ":")
	switch {
	case key.Mode&^key.WantMode != 0 || user != wantUser:
		return i18n.Tf("the module signing key %s is %04o %s, more open than %04o %s", kmod.KeyPath, key.Mode, key.Owner, key.WantMode, key.WantOwner)
	case key.WantMode&0o040 != 0:
		return i18n.Tf("akmods cannot read the module signing key %s to sign modules: it is %04o %s, not %04o %s", kmod.KeyPath, key.Mode, key.Owner, key.WantMode, key.WantOwner)
	}
	return i18n.Tf("the module signing key %s is %04o %s, not %04o %s", kmod.KeyPath, key.Mode, key.Owner, key.WantMode, key.WantOwner)
}

// reconcileModuleSigningEnrollment also covers a module that is present but
// rejected at load: with Secure Boot enforcing, an unenrolled key is the cause.
func reconcileModuleSigningEnrollment(checkOnly bool) recResult {
	status, err := moduleSigningStatus()
	if err != nil {
		return warnRes(i18n.T("could not read the module signing key %s: %v"), kmod.KeyPath, err)
	}
	switch {
	case !status.Key.Present:
		return okRes(i18n.T("no module signing key yet, so there is nothing to enroll"))
	case status.Enrollment == kmod.EnrollmentEnrolled:
		return okRes(i18n.T("the module signing key is enrolled"))
	case status.SecureBoot == kmod.SecureBootUnsupported:
		return okRes(i18n.T("this machine boots without UEFI Secure Boot, so the module signing key needs no enrollment"))
	case status.SecureBoot != kmod.SecureBootEnforcing:
		return okRes(i18n.T("Secure Boot is off; doctor queues the module signing key's enrollment once Secure Boot is turned on"))
	case status.Enrollment == kmod.EnrollmentQueued:
		return noteRes("%s", moduleSigningApproval(i18n.T("the module signing key's enrollment is queued"), status.Password))
	case status.Enrollment == kmod.EnrollmentUnknown:
		return warnRes(i18n.T("mokutil cannot tell whether the module signing key is enrolled")).
			withFix("mokutil --test-key %s", kmod.CertPath)
	}
	if checkOnly {
		return wouldRes(i18n.T("Secure Boot rejects every module signed with the module signing key until it is enrolled")).
			withFix("%s", queueModuleSigningEnrollmentCommand)
	}
	if err := queueModuleSigningEnrollment(); err != nil {
		return failRes(i18n.T("could not queue the module signing key's enrollment: %v"), err).
			withFix("%s", queueModuleSigningEnrollmentCommand)
	}
	status, err = moduleSigningStatus()
	if err != nil || status.Enrollment != kmod.EnrollmentQueued {
		return failRes(i18n.T("the module signing key's enrollment is still not queued")).
			withFix("%s", queueModuleSigningEnrollmentCommand)
	}
	return fixedRes("%s", moduleSigningApproval(i18n.T("queued the module signing key's enrollment"), status.Password))
}

// moduleSigningApproval tells the user what MokManager asks at the next boot.
func moduleSigningApproval(done, password string) string {
	return done + "; " + i18n.Tf("reboot, choose Enroll MOK in MokManager, then Continue and Yes, and type the password %s", password)
}
