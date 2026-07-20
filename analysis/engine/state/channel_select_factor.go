package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

// ApplyChannelSelectFactsFactor publishes one complete ordered N3 fact batch
// into the registered ChannelSelect component without reconstructing State.
// The lane registration remains the sole owner of the concrete lattice.
func (d ProductDomain) ApplyChannelSelectFactsFactor(current LaneFactor, facts []channelselectfact.Fact) (LaneFactor, error) {
	runtime, err := d.validateFactor(current)
	if err != nil || runtime.lane.id != LaneChannelSelect {
		return LaneFactor{}, fmt.Errorf("%w: ChannelSelect factor is foreign", ErrInvalidLaneFactor)
	}
	payload, ok := current.payload.(typedLaneFactorPayload[channelselectfact.Lane])
	if !ok {
		return LaneFactor{}, fmt.Errorf("%w: ChannelSelect factor carrier is malformed", ErrInvalidLaneFactor)
	}
	next := payload.value
	for _, fact := range facts {
		if fact.Select == "" {
			return LaneFactor{}, fmt.Errorf("%w: empty ChannelSelect fact", ErrInvalidLaneFactor)
		}
		next = next.Add(fact)
	}
	if channelselectfact.Domain().Same(payload.value, next) {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[channelselectfact.Lane]{value: next}}, nil
}

// ChannelSelectFactsFactorSnapshot observes the registered ChannelSelect
// component without reconstructing a product State. Factor-native semantic
// programs use this as the sole read boundary for the lane's finite must-set.
func (d ProductDomain) ChannelSelectFactsFactorSnapshot(current LaneFactor) (ChannelSelectFactsSnapshot, error) {
	runtime, err := d.validateFactor(current)
	if err != nil || runtime.lane.id != LaneChannelSelect {
		return ChannelSelectFactsSnapshot{}, fmt.Errorf("%w: ChannelSelect factor is foreign", ErrInvalidLaneFactor)
	}
	payload, ok := current.payload.(typedLaneFactorPayload[channelselectfact.Lane])
	if !ok {
		return ChannelSelectFactsSnapshot{}, fmt.Errorf("%w: ChannelSelect factor carrier is malformed", ErrInvalidLaneFactor)
	}
	return channelSelectFactsSnapshot(payload.value), nil
}
