package todo

import (
	"crypto/rand"
	"strings"
)

const (
	idAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	idLen      = 7
)

// newID returns a random idLen-character ID drawn uniformly from idAlphabet,
// redrawing while taken reports true.
func newID(taken func(string) bool) string {
	for {
		var raw [idLen]byte
		_, _ = rand.Read(raw[:]) // never fails: since Go 1.24 it crashes the program instead
		var b strings.Builder
		for _, r := range raw {
			b.WriteByte(idAlphabet[r&31])
		}
		if id := b.String(); !taken(id) {
			return id
		}
	}
}

// validID reports whether id is exactly idLen characters of idAlphabet.
func validID(id string) bool {
	if len(id) != idLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if strings.IndexByte(idAlphabet, id[i]) < 0 {
			return false
		}
	}
	return true
}
