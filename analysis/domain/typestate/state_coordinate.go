package typestate

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

// StateCoordinate is Typestate's scoped, immutable protocol-state authority.
// Target supplies the compact protocol/state coordinates; Typestate binds
// them to one Link-backed Schema and owns their content identity.
type StateCoordinate struct {
	schema   *universe
	protocol target.Protocol
	state    target.State
	id       keyspace.ContentID
}

func (s StateCoordinate) validFor(schema *universe) bool {
	if schema == nil || s.schema != schema || s.protocol == 0 || s.state == 0 || !s.id.Available() {
		return false
	}
	final, ok := schema.source.Boundary().Target()
	if !ok || final == nil {
		return false
	}
	if _, ok := final.StateFinal(s.protocol, s.state); !ok {
		return false
	}
	return s.id == stateCoordinateID(schema.linkID, s.protocol, s.state)
}

// ContentID returns this Schema-scoped state identity, suitable for replay and
// manifest content without exposing a Link-owned state handle.
func (s StateCoordinate) ContentID() keyspace.ContentID {
	if !s.validFor(s.schema) {
		return keyspace.ContentID{}
	}
	return s.id
}

func (schema Schema) stateCoordinate(protocol target.Protocol, state target.State) (StateCoordinate, bool) {
	if !schema.Valid() {
		return StateCoordinate{}, false
	}
	return stateCoordinateForUniverse(schema.universe, protocol, state)
}

func stateCoordinateForUniverse(universe *universe, protocol target.Protocol, state target.State) (StateCoordinate, bool) {
	if universe == nil || universe.source == nil || !universe.linkID.Available() || protocol == 0 || state == 0 {
		return StateCoordinate{}, false
	}
	contract, ok := universe.source.Boundary().Target()
	if !ok || contract == nil {
		return StateCoordinate{}, false
	}
	if _, ok := contract.StateFinal(protocol, state); !ok {
		return StateCoordinate{}, false
	}
	coordinate := StateCoordinate{schema: universe, protocol: protocol, state: state}
	coordinate.id = stateCoordinateID(universe.linkID, protocol, state)
	return coordinate, coordinate.validFor(universe)
}

// StateAt returns the canonical state authority at key's protocol-local index.
func (schema Schema) StateAt(key Key, index int) (StateCoordinate, bool) {
	range_, ok := schema.keyRange(key)
	if !ok || index < 0 || index >= len(range_.states) {
		return StateCoordinate{}, false
	}
	state := range_.states[index]
	return state, state.validFor(schema.universe)
}

func stateCoordinateID(scope keyspace.ContentID, protocol target.Protocol, state target.State) keyspace.ContentID {
	var payload [32 + 4*8]byte
	copy(payload[:32], scope[:])
	words := payload[32:]
	// Preserve the established content preimage while moving its derivation
	// authority into Typestate; manifests/replay do not observe Go ownership.
	binary.BigEndian.PutUint64(words[0:8], 0x6c696e6b2d707273) // "link-prs"
	binary.BigEndian.PutUint64(words[8:16], 1)
	binary.BigEndian.PutUint64(words[16:24], uint64(protocol))
	binary.BigEndian.PutUint64(words[24:32], uint64(state))
	return sha256.Sum256(payload[:])
}

func protocolCoordinateID(scope keyspace.ContentID, protocol target.Protocol) keyspace.ContentID {
	var payload [32 + 3*8]byte
	copy(payload[:32], scope[:])
	words := payload[32:]
	binary.BigEndian.PutUint64(words[0:8], 0x6c696e6b2d70726f) // "link-pro"
	binary.BigEndian.PutUint64(words[8:16], 1)
	binary.BigEndian.PutUint64(words[16:24], uint64(protocol))
	return sha256.Sum256(payload[:])
}
