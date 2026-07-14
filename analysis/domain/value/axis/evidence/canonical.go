package evidence

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonical("value.axis.evidence", 1, encodeCanonical)
}

// encodeCanonical writes the complete fixed carrier observed by Equal. Even
// inactive array slots are included because raw Value equality observes them.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.kind)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.origins.count)); err != nil {
		return err
	}
	if err := writer.Bool(v.origins.truncated); err != nil {
		return err
	}
	for _, origin := range v.origins.items {
		if err := writer.Uint(uint64(origin.Kind)); err != nil {
			return err
		}
		if err := writer.Uint(origin.ID); err != nil {
			return err
		}
	}
	return nil
}
