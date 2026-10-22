package doctor

import (
	"errors"

	"ryoku-cli/internal/kmod"
	i18n "ryoku-i18n"
)

var readModuleSigningStatusCommand = kmod.Advice("status")

// akmods' key directories are 0750 root:akmods, so the desktop user cannot see
// the links; root reads them with sudo -n, which answers only from a cached
// credential and never prompts.
var moduleSigningLinksAsRoot = func() (kmod.LinkState, error) {
	report, _, err := kmod.AsRoot{Host: ryokuHostBin(), NonInteractive: true}.Run("status")
	if err != nil {
		return "", err
	}
	links := kmod.LinkState(report["akmods_links"])
	if links == "" {
		return "", errors.New("ryoku-host kmod status printed no akmods_links")
	}
	return links, nil
}

// reconcileAkmodsKeyLinks keeps akmods signing with the module signing key.
// Left alone, akmods-keygen fills a missing or dangling path with a key of its
// own, which nobody enrolled, so Secure Boot rejects every module it signs.
func reconcileAkmodsKeyLinks(checkOnly bool) recResult {
	status, err := moduleSigningStatus()
	if err != nil {
		return warnRes(i18n.T("could not read the module signing key %s: %v"), kmod.KeyPath, err)
	}
	links := status.Key.Links
	if links == kmod.LinksUnreadable {
		if links, err = moduleSigningLinksAsRoot(); err != nil {
			return noteRes(i18n.T("only root can read akmods' key paths, and sudo has no cached credential to check them")).
				withFix("%s", readModuleSigningStatusCommand)
		}
	}
	switch links {
	case kmod.LinksNotNeeded:
		return okRes(i18n.T("akmods is not installed, so it has no key paths to link"))
	case kmod.LinksOK:
		return okRes(i18n.T("akmods' key paths link to the module signing key %s"), kmod.KeyPath)
	}
	if checkOnly {
		return wouldRes("%s", akmodsKeyLinksDrift(links)).withFix("%s", prepareModuleSigningKeyCommand)
	}
	if err := prepareModuleSigningKey(); err != nil {
		return failRes(i18n.T("could not point akmods' key paths at the module signing key: %v"), err).
			withFix("%s", prepareModuleSigningKeyCommand)
	}
	return fixedRes(i18n.T("pointed akmods' key paths at the module signing key %s"), kmod.KeyPath)
}

func akmodsKeyLinksDrift(links kmod.LinkState) string {
	switch links {
	case kmod.LinksMissing:
		return i18n.Tf("akmods' key paths %s and %s do not both exist, so akmods would sign with a new key nobody enrolled", kmod.AkmodsKeyPath, kmod.AkmodsCertPath)
	case kmod.LinksDangling:
		return i18n.Tf("akmods' key paths %s and %s lead to a key that no longer exists, so akmods would sign with a new key nobody enrolled", kmod.AkmodsKeyPath, kmod.AkmodsCertPath)
	}
	return i18n.Tf("akmods signs with a key it generated itself instead of the module signing key %s, so Secure Boot rejects what it builds", kmod.KeyPath)
}
