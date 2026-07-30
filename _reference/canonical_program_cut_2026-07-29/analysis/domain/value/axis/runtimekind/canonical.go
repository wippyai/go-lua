package runtimekind

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.runtimekind", 1, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("runtimekind: invalid canonical record %d", record)
	}
	raw, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if raw > uint64(allKnownMask) {
		return Bottom(), fmt.Errorf("runtimekind: invalid canonical mask")
	}
	return Value{mask: uint16(raw)}, nil
}

// encodeCanonical writes the known runtime-kind mask observed by Equal. Bits
// outside the declared tag vocabulary are deliberately not semantic.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(v.mask & allKnownMask))
}
