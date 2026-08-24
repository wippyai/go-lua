package inspect

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
)

// writef appends one newline-separated fact line. Every line names the
// accessor it was read from, so a reader can reproduce it from the public
// product surface without consulting this package.
func writef(b *strings.Builder, format string, args ...any) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, format, args...)
}

func decimal(value uint64) string { return strconv.FormatUint(value, 10) }

// compactSpace folds a multi-line rendering onto one line so the inspector's
// output stays one fact per line.
func compactSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// semanticSpelling renders one semantic key as its hex digest and declared
// interpretation version. The pair is the key's whole content.
func semanticSpelling(key identity.SemanticKey) string {
	digest := key.Digest()
	return hex.EncodeToString(digest[:]) + "@" + decimal(key.Version())
}
