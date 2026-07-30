package assertion

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.assertion", 1, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("assertion: invalid canonical record %d", record)
	}
	rawState, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawState > uint64(top) {
		return Bottom(), fmt.Errorf("assertion: invalid canonical state")
	}
	decodedState := state(rawState)
	if decodedState == concrete {
		rawFlags, err := reader.Uint()
		if err != nil {
			return Bottom(), err
		}
		if rawFlags == 0 || rawFlags > uint64(knownFlags) || Flag(rawFlags)&^knownFlags != 0 {
			return Bottom(), fmt.Errorf("assertion: invalid canonical flags")
		}
		return Value{state: decodedState, flags: Flag(rawFlags)}, nil
	}
	// Bottom and Top have no payload in the constructor-reachable carrier.
	return Value{state: decodedState}, nil
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
