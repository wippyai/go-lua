package typewitness

import (
	"context"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const canonicalValueRecord uint64 = 1

// ErrNonportableRecursiveIdentity reports a witness whose exact equality
// depends on process-local recursive IDs. No structural approximation is
// emitted: recursive authority remains pending until declaration-owned stable
// identities exist.
var ErrNonportableRecursiveIdentity = errors.New("typewitness: recursive identity is not portable")

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonical("value.axis.typewitness", 1, encodeCanonical)
}

// encodeCanonical is complete for Bottom, Top, and concrete nonrecursive
// witnesses. Recursive witnesses fail before writing an axis record because
// typ.EncodeCanonical intentionally omits the stricter process-local identity
// set observed by typewitness.Equal.
func encodeCanonical(writer *canonical.Writer, value Value) error {
	state, err := canonicalState(value)
	if err != nil {
		return err
	}
	if state == concrete && typ.ContainsRecursive(value.t) {
		return ErrNonportableRecursiveIdentity
	}

	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(state)); err != nil {
		return err
	}
	if state != concrete {
		return nil
	}
	structural, err := typ.EncodeCanonical(context.Background(), value.t)
	if err != nil {
		return fmt.Errorf("typewitness: encode structural type: %w", err)
	}
	return writer.Bytes(structural)
}

func canonicalState(value Value) (state, error) {
	switch {
	case value.IsBottom():
		return bottom, nil
	case value.IsTop():
		return top, nil
	case value.t == nil:
		return 0, fmt.Errorf("typewitness: malformed non-concrete state")
	default:
		canonical := Of(value.t)
		if canonical.IsTop() || canonical.IsBottom() ||
			value.recursive != canonical.recursive || !Equal(value, canonical) {
			return 0, fmt.Errorf("typewitness: malformed or unsupported concrete value")
		}
		return concrete, nil
	}
}
