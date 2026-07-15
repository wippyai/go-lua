package escape

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.escape", 1, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("escape: invalid canonical record %d", record)
	}
	raw, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if raw > uint64(escaped) {
		return Bottom(), fmt.Errorf("escape: invalid canonical value")
	}
	return Value(raw), nil
}

// encodeCanonical writes the exact Equal identity of v. It is intentionally
// package-private until the axis registry owns codec completeness.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(v))
}
