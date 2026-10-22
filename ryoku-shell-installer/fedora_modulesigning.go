package main

import (
	"strings"

	i18n "ryoku-i18n"
)

// stepFedoraDrivers gives the machine its one module signing key, installs
// the NVIDIA driver when the plan says so, and, when Secure Boot is enforcing,
// queues the key's enrollment, so xpadneo and the NVIDIA driver load after a
// single approval in MokManager. ryoku-host owns every step and the
// enrollment password; this only reads its answer.
func stepFedoraDrivers(e *engine) {
	host := e.ryokuTool("ryoku-host")
	if host == "" {
		host = "ryoku-host"
	}
	keyReady := e.sudo(host, "kmod", "prepare-key") == nil
	if !keyReady {
		e.say(i18n.T("warning: could not prepare the module signing key; modules such as xpadneo may not load under Secure Boot (continuing)"))
	}
	if installFedoraNvidia(e, host) {
		keyReady = true
	}
	if keyReady {
		queueFedoraEnrollment(e, host)
	}
	recordFedoraNvidiaChoice(e, host)
}

func queueFedoraEnrollment(e *engine, host string) {
	out, err := e.sudoOutput(host, "kmod", "queue-enrollment")
	if err != nil {
		e.say(i18n.T("warning: could not queue the module signing key's enrollment; modules such as xpadneo may not load under Secure Boot (continuing)"))
		return
	}
	status := parseKeyValues(out)
	if status["enrollment"] == "queued" {
		e.enrollmentPassword = status["password"]
	}
	for _, line := range enrollmentSummary(e) {
		e.say(line)
	}
}

// enrollmentSummary is what the user must do at the next boot, empty when no
// enrollment is waiting.
func enrollmentSummary(e *engine) []string {
	if e == nil || e.enrollmentPassword == "" {
		return nil
	}
	return []string{
		i18n.T("Secure Boot: at the next reboot a blue MokManager screen asks you to approve this machine's module signing key. It waits until you do."),
		i18n.Tf("Choose 'Enroll MOK', then 'Continue', then 'Yes', and type the password %s. The machine then restarts.", e.enrollmentPassword),
	}
}

func parseKeyValues(out string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			values[key] = value
		}
	}
	return values
}
