package main

import (
	"errors"
	"net/url"
	"os"
	"strings"

	i18n "ryoku-i18n"
)

var installMode = "auto"

func sourceMode(d *distro, mode, payload, ref string) bool {
	if d.id != "fedora" {
		return d.fromSource
	}
	if mode != "auto" {
		return mode == "source"
	}
	return payload != "" || (ref != "" && ref != "main") || strings.TrimSuffix(repoURL, ".git") != strings.TrimSuffix(defaultRepoURL, ".git")
}

func (e *engine) fromSource() bool {
	return sourceMode(e.d(), installMode, e.payloadOverride, e.ref)
}

func fedoraRepositoryConfig() (string, string, string, error) {
	base := strings.TrimRight(os.Getenv("RYOKU_RPM_BASE_URL"), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(base, " \t\r\n") {
		return "", "", "", errors.New(i18n.T("set RYOKU_RPM_BASE_URL to the HTTPS Fedora release root, or use --install-mode=source for a checkout build"))
	}
	copr, metadata := os.Getenv("RYOKU_COPR_FINGERPRINT"), os.Getenv("RYOKU_RPM_SIGNING_KEY")
	for _, fingerprint := range []string{copr, metadata} {
		if len(fingerprint) != 40 && len(fingerprint) != 64 {
			return "", "", "", errors.New(i18n.T("set RYOKU_COPR_FINGERPRINT and RYOKU_RPM_SIGNING_KEY to the full trusted public-key fingerprints"))
		}
		if strings.Trim(fingerprint, "0123456789abcdefABCDEF") != "" {
			return "", "", "", errors.New(i18n.T("repository fingerprints must be hexadecimal"))
		}
	}
	return base, copr, metadata, nil
}
