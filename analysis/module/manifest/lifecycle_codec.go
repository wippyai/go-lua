package manifest

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/typestate"
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

func encodeOptionalLifecycleState(state typestate.State) string {
	return state.String()
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

func decodeOptionalLifecycleState(raw string) typestate.State {
	return typestate.OptionalStateFromString(raw)
}
