package variantorigin

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.variantorigin", 1, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("variantorigin: invalid canonical record %d", record)
	}
	rawState, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawState > uint64(top) {
		return Bottom(), fmt.Errorf("variantorigin: invalid canonical state")
	}
	family, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	count, err := reader.Count()
	if err != nil {
		return Bottom(), err
	}
	if count > uint64(maxInt()) || count > uint64(reader.RemainingBytes()/3) {
		return Bottom(), fmt.Errorf("variantorigin: canonical case count exceeds structural input")
	}
	cases := make([]int, int(count))
	for index := range cases {
		value, err := reader.Int()
		if err != nil {
			return Bottom(), err
		}
		if int64(int(value)) != value || index != 0 && int(value) <= cases[index-1] {
			return Bottom(), fmt.Errorf("variantorigin: noncanonical case sequence")
		}
		cases[index] = int(value)
	}
	decodedState := state(rawState)
	switch decodedState {
	case bottom, top:
		if family != 0 || len(cases) != 0 {
			return Bottom(), fmt.Errorf("variantorigin: terminal canonical value carries payload")
		}
		return Value{state: decodedState}, nil
	case concrete:
		if family == 0 || len(cases) == 0 {
			return Bottom(), fmt.Errorf("variantorigin: concrete canonical value has empty payload")
		}
		return Value{state: concrete, family: family, cases: caseset.New(cases)}, nil
	default:
		return Bottom(), fmt.Errorf("variantorigin: invalid canonical state")
	}
}

func maxInt() int { return int(^uint(0) >> 1) }

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
