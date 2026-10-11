package wm

import (
	"fmt"
	"strings"
	"testing"
)

// wlsunset reads a missing location as polar night half the year and as
// midnight sun the other half, holding -t or -T respectively, so both must
// land on the user's temperature: one kelvin apart, the smallest gap its
// high-above-low validation accepts.
func TestWlsunsetArgsHoldOneTemperatureAllYear(t *testing.T) {
	for _, temp := range []int{1000, 4000, 25000} {
		got := strings.Join(WlsunsetArgs(temp), " ")
		want := fmt.Sprintf("wlsunset -t %d -T %d", temp, temp+1)
		if got != want {
			t.Errorf("WlsunsetArgs(%d) = %q, want %q", temp, got, want)
		}
	}
}
