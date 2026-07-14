package identity

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonical("value.axis.identity", 1, encodeCanonical)
}

// encodeCanonical writes exactly the state observed by Equal. Only singleton
// state gives the stored ID semantic meaning; all other states ignore it.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.state)); err != nil {
		return err
	}
	if v.state != singleton {
		return nil
	}
	if err := writer.String(v.id.Kind); err != nil {
		return err
	}
	if err := writer.String(v.id.Site); err != nil {
		return err
	}
	return writer.Uint(v.id.Index)
}
