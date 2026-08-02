package evidence

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.evidence", 2, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("evidence: invalid canonical record %d", record)
	}
	rawKind, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawKind > uint64(top) {
		return Bottom(), fmt.Errorf("evidence: invalid canonical kind")
	}
	return Value{kind: kind(rawKind)}, nil
}

// encodeCanonical writes exactly the semantic lattice state observed by Equal.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(v.kind))
}
