package wm

import "strconv"

// WlsunsetArgs is the argv that holds wlsunset at one colour temperature all
// year, for the providers whose night light backend it is. With no location
// wlsunset sits in a static state, but which one follows the season: the low
// (-t) while the sun's declination is negative, the high (-T) from about March
// to September, when it reads the missing latitude as midnight sun. A fixed far
// high turned the screen blue every summer, so the high sits one kelvin above
// the low, the smallest gap its high-above-low validation accepts.
func WlsunsetArgs(temp int) []string {
	return []string{"wlsunset", "-t", strconv.Itoa(temp), "-T", strconv.Itoa(temp + 1)}
}
