package updater

import (
	"strconv"
	"strings"
)

// versionCompare orders two package versions ([epoch:]version[-release]) the
// way rpm does: -1 when a is older, 0 when equal, 1 when newer. It lives in Go
// rather than shelling out so the status check stays cheap and testable; the
// same rules order the pacman versions a packaged Arch box reports.
func versionCompare(a, b string) int {
	ea, va, ra := splitEVR(a)
	eb, vb, rb := splitEVR(b)
	if ea != eb {
		if ea < eb {
			return -1
		}
		return 1
	}
	if c := rpmvercmp(va, vb); c != 0 {
		return c
	}
	return rpmvercmp(ra, rb)
}

func splitEVR(s string) (epoch int, version, release string) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		epoch, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	if i := strings.LastIndexByte(s, '-'); i >= 0 {
		return epoch, s[:i], s[i+1:]
	}
	return epoch, s, ""
}

// rpmvercmp is rpm's segment comparison: runs of digits compare numerically,
// runs of letters lexically, a digit run outranks a letter run, '~' sorts
// before everything (pre-releases) and '^' after the base (snapshots).
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}
	for a != "" || b != "" {
		a = strings.TrimLeftFunc(a, isVerSeparator)
		b = strings.TrimLeftFunc(b, isVerSeparator)

		if strings.HasPrefix(a, "~") || strings.HasPrefix(b, "~") {
			if !strings.HasPrefix(a, "~") {
				return 1
			}
			if !strings.HasPrefix(b, "~") {
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}
		if strings.HasPrefix(a, "^") || strings.HasPrefix(b, "^") {
			if a == "" {
				return -1
			}
			if b == "" {
				return 1
			}
			if !strings.HasPrefix(a, "^") {
				return 1
			}
			if !strings.HasPrefix(b, "^") {
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}
		if a == "" || b == "" {
			break
		}

		numeric := isDigit(rune(a[0]))
		take := isAlpha
		if numeric {
			take = isDigit
		}
		segA, segB := leading(a, take), leading(b, take)
		a, b = a[len(segA):], b[len(segB):]
		if segB == "" {
			if numeric {
				return 1
			}
			return -1
		}
		if numeric {
			segA = strings.TrimLeft(segA, "0")
			segB = strings.TrimLeft(segB, "0")
			if len(segA) != len(segB) {
				if len(segA) < len(segB) {
					return -1
				}
				return 1
			}
		}
		if c := strings.Compare(segA, segB); c != 0 {
			return c
		}
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

func leading(s string, take func(rune) bool) string {
	for i, r := range s {
		if !take(r) {
			return s[:i]
		}
	}
	return s
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isAlpha(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

func isVerSeparator(r rune) bool { return !isDigit(r) && !isAlpha(r) && r != '~' && r != '^' }
