package presence

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonical("value.axis.presence", 1, encodeCanonical)
}

func encodeCanonical(writer *canonical.Writer, value Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	return writer.Uint(uint64(value))
}
