package main

import (
	"errors"
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
	return payload != "" || (ref != "" && ref != "main-fedora") || strings.TrimSuffix(repoURL, ".git") != strings.TrimSuffix(defaultRepoURL, ".git")
}

func (e *engine) fromSource() bool {
	return sourceMode(e.d(), installMode, e.payloadOverride, e.ref)
}

func fedoraRepositoryConfig() (string, error) {
	fingerprint := os.Getenv("RYOKU_COPR_FINGERPRINT")
	if (len(fingerprint) != 40 && len(fingerprint) != 64) || strings.Trim(fingerprint, "0123456789abcdefABCDEF") != "" {
		return "", errors.New(i18n.T("set RYOKU_COPR_FINGERPRINT to the full trusted signing-key fingerprint of itskontra/ryoku"))
	}
	return fingerprint, nil
}
