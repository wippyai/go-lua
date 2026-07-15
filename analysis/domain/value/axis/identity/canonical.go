package identity

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.identity", 1, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("identity: invalid canonical record %d", record)
	}
	rawState, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawState > uint64(top) {
		return Bottom(), fmt.Errorf("identity: invalid canonical state")
	}
	decodedState := state(rawState)
	if decodedState == singleton {
		kind, err := reader.String()
		if err != nil {
			return Bottom(), err
		}
		site, err := reader.String()
		if err != nil {
			return Bottom(), err
		}
		index, err := reader.Uint()
		if err != nil {
			return Bottom(), err
		}
		id := ID{Kind: kind, Site: site, Index: index}
		if id == (ID{}) {
			return Bottom(), fmt.Errorf("identity: canonical singleton has zero identity")
		}
		return Value{state: decodedState, id: id}, nil
	}
	// Bottom and Top have no payload in the constructor-reachable carrier.
	return Value{state: decodedState}, nil
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
