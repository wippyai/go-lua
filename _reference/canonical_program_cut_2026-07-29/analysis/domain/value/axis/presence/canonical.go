package presence

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.presence", 1, encodeCanonical, decodeCanonical)
}

func encodeCanonical(writer *canonical.Writer, value Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(value))
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("presence: invalid canonical record %d", record)
	}
	raw, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if raw > uint64(maybe) {
		return Bottom(), fmt.Errorf("presence: invalid canonical value")
	}
	return Value(raw), nil
}
