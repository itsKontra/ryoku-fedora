package kmod

import (
	"os"
	"path/filepath"
	"strings"
)

// Choice is what the installer recorded for an NVIDIA machine: the NVIDIA
// driver, or nouveau by the user's choice. Doctor offers the driver only to a
// machine with no recorded nouveau choice.
type Choice string

const (
	ChoiceDriver  Choice = "driver"
	ChoiceNouveau Choice = "nouveau"
	ChoiceNone    Choice = ""
)

// ChoicePath sits beside the channel intent in Ryoku's state directory.
const ChoicePath = "/var/lib/ryoku/nvidia-choice"

func (m *Machine) choice() Choice {
	data, err := os.ReadFile(m.path(ChoicePath))
	if err != nil {
		return ChoiceNone
	}
	switch choice := Choice(strings.TrimSpace(string(data))); choice {
	case ChoiceDriver, ChoiceNouveau:
		return choice
	}
	return ChoiceNone
}

// RecordChoice writes the choice in one rename, so a reader never sees half.
func (m *Machine) RecordChoice(choice Choice) (Status, error) {
	file := m.path(ChoicePath)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return Status{}, err
	}
	if err := os.WriteFile(file+".new", []byte(string(choice)+"\n"), 0o644); err != nil {
		return Status{}, err
	}
	if err := os.Chmod(file+".new", 0o644); err != nil {
		return Status{}, err
	}
	if err := os.Rename(file+".new", file); err != nil {
		return Status{}, err
	}
	return m.Status()
}
