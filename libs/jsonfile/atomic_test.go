package jsonfile

import (
	"crypto/rand"
	"testing"
)

// TestTempSuffixMatchesRandText guards the sweep's idea of a temp file name
// against a change in rand.Text, which createTemp uses for the suffix.
func TestTempSuffixMatchesRandText(t *testing.T) {
	for range 100 {
		if s := rand.Text(); !isTempSuffix(s) {
			t.Fatalf("isTempSuffix(%q) = false for rand.Text's output", s)
		}
	}
}
