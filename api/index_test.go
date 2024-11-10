package api

import "testing"

func TestSanitizer(t *testing.T) {
	for _, svg := range icons {
		if !VALIDATOR.MatchString(svg) {
			t.Errorf("This SVG mismatches: %s", svg)
		}
	}
}
