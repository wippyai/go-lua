package runtimekind

import "github.com/wippyai/go-lua/analysis/internal/canonical"

const canonicalValueRecord uint64 = 1

// encodeCanonical writes the known runtime-kind mask observed by Equal. Bits
// outside the declared tag vocabulary are deliberately not semantic.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(v.mask & allKnownMask))
}
