package assertion

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonical("value.axis.assertion", 1, encodeCanonical)
}

// encodeCanonical writes exactly the state and normalized claim bits observed
// by Equal. Non-concrete flag storage and unknown concrete bits are not semantic.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.state)); err != nil {
		return err
	}
	if v.state != concrete {
		return nil
	}
	return writer.Uint(uint64(normalizedFlags(v)))
}
