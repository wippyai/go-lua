package wire

import (
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/domain/typestate"
)

func encodeLifecycleProtocol(protocol typestate.Protocol, missing string) (string, error) {
	if protocol == "" {
		return "", errors.New(missing)
	}
	return protocol.String(), nil
}

func encodeRequiredLifecycleState(state typestate.State, missing string) (string, error) {
	if state == "" {
		return "", errors.New(missing)
	}
	return state.String(), nil
}

func encodeLifecycleObligation(obligation typestate.Obligation) []string {
	return EncodeTypestateStates(obligation.FinalStateList())
}

func decodeLifecycleProtocol(raw string, missing string) (typestate.Protocol, error) {
	protocol, ok := typestate.ProtocolFromString(raw)
	if !ok {
		return "", errors.New(missing)
	}
	return protocol, nil
}

func decodeRequiredLifecycleState(raw string, missing string) (typestate.State, error) {
	state, ok := typestate.StateFromString(raw)
	if !ok {
		return "", errors.New(missing)
	}
	return state, nil
}

func decodeLifecycleObligation(raw []string) (typestate.Obligation, error) {
	states, err := DecodeTypestateStates(raw, "final state")
	if err != nil {
		return typestate.Obligation{}, err
	}
	obligation, ok := typestate.NewObligation(states...)
	if !ok {
		return typestate.Obligation{}, errors.New("signature/wire: lifecycle obligation contains an empty final state")
	}
	return obligation, nil
}

// EncodeTypestateStates writes a state set as its sorted spellings, so the same
// set is one byte sequence whatever order it was built in.
func EncodeTypestateStates(states []typestate.State) []string {
	if len(states) == 0 {
		return nil
	}
	out := make([]string, 0, len(states))
	for _, state := range states {
		out = append(out, state.String())
	}
	sort.Strings(out)
	return out
}

// DecodeTypestateStates reads a state set back, naming the role in the refusal
// so a malformed member reports where it was carried.
func DecodeTypestateStates(raw []string, role string) ([]typestate.State, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]typestate.State, 0, len(raw))
	for _, name := range raw {
		state, ok := typestate.StateFromString(name)
		if !ok {
			return nil, fmt.Errorf("empty %s", role)
		}
		out = append(out, state)
	}
	return out, nil
}
