package variantorigin

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonical("value.axis.variantorigin", 1, encodeCanonical)
}

// encodeCanonical writes the complete raw identity observed by Equal. The case
// set owns canonical sorted storage, so indexed traversal records its exact
// signed members without exposing slice backing.
//
// This helper is intentionally package-private until the axis registry owns
// codec completeness and publication authority.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.state)); err != nil {
		return err
	}
	if err := writer.Uint(v.family); err != nil {
		return err
	}
	if err := writer.Count(uint64(v.cases.Len())); err != nil {
		return err
	}
	for i := 0; i < v.cases.Len(); i++ {
		if err := writer.Int(int64(v.cases.At(i))); err != nil {
			return err
		}
	}
	return nil
}
