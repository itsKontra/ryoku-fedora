package main

import (
	"encoding/json"
	"errors"
	"os/exec"
)

// keyringUnlocksAtLogin asks the ryoku CLI, the one owner of the SDDM keyring
// wiring (sddm/setup delegates to it), whether signing in unlocks the login
// keyring.
func (e *engine) keyringUnlocksAtLogin() (bool, error) {
	bin := e.ryokuBin()
	if bin == "" {
		return false, errors.New("the ryoku CLI is not installed")
	}
	out, err := exec.Command(bin, "keyring", "status", "--json").Output()
	if err != nil {
		return false, err
	}
	return parseKeyringUnlock(out)
}

// parseKeyringUnlock reads `ryoku keyring status --json`: login unlocks the
// keyring when the PAM lines are present and SDDM does not autologin (an
// autologin has no password for PAM to unlock with).
func parseKeyringUnlock(statusJSON []byte) (bool, error) {
	var st struct {
		PamPresent bool `json:"pam_present"`
		Autologin  bool `json:"autologin"`
	}
	if err := json.Unmarshal(statusJSON, &st); err != nil {
		return false, err
	}
	return st.PamPresent && !st.Autologin, nil
}
