package passphrase

import (
	"regexp"
	"testing"
)

func TestNewFormat(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+-[a-z]+$`)
	for i := 0; i < 50; i++ {
		p := New()
		if !re.MatchString(p) {
			t.Fatalf("bad format: %q", p)
		}
	}
}

func TestNewVariety(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[New()] = true
	}
	if len(seen) < 50 {
		t.Errorf("low variety: only %d unique in 200 draws", len(seen))
	}
}
