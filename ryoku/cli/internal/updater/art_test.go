package updater

import (
	"os"
	"strings"
	"testing"
)

func TestCurrentLineHasArt(t *testing.T) {
	b, err := os.ReadFile("../../../../CODENAME")
	if err != nil {
		t.Skip("CODENAME not beside the checkout")
	}
	name := strings.TrimSpace(string(b))
	if releaseArt(name) == "" {
		t.Errorf("%s has no art (internal/updater/art/%s.txt)", name, strings.ToLower(name))
	}
	if releaseArt("nobody") != "" {
		t.Fatal("an unnamed line must have no art")
	}
}
