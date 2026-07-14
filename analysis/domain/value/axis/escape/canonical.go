package escape

import "github.com/wippyai/go-lua/analysis/internal/canonical"

const canonicalValueRecord uint64 = 1

// encodeCanonical writes the exact Equal identity of v. It is intentionally
// package-private until the axis registry owns codec completeness.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(v))
}
