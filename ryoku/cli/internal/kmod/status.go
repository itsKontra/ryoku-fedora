package kmod

// Status is everything an installer, doctor or the updater needs to decide
// what to do next about the module signing key.
type Status struct {
	Key        KeyState
	SecureBoot SecureBoot
	Enrollment Enrollment
	Password   string
	Choice     Choice
}

func (m *Machine) Status() (Status, error) {
	key, err := m.keyState()
	if err != nil {
		return Status{}, err
	}
	return Status{
		Key:        key,
		SecureBoot: m.secureBoot(),
		Enrollment: m.enrollment(key),
		Password:   EnrollmentPassword,
		Choice:     m.choice(),
	}, nil
}
