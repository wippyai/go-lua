package typestate

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/target"
)

// Acquisition is Typestate's opaque proof of one resource-origin acquisition
// declaration.  It is derived from an existing acquisition ResourceOrigin and
// the matching sealed Contract row; it is not a Link-owned protocol row.
type Acquisition struct {
	schema   *universe
	resource ResourceOrigin
	key      Key
	state    StateCoordinate
	content  keyspace.ContentID
}

// Key returns the one already-admitted resource range acquired by this
// declaration.
func (a Acquisition) Key() Key { return a.key }

// ContentID is the stable identity of the origin selected by this acquisition.
func (a Acquisition) ContentID() keyspace.ContentID { return a.content }

// Transition is Typestate's opaque proof of one Contract transition outcome
// applied to a formal ResourceOrigin at one existing Application × operation
// coordinate.  Its fields deliberately retain no caller-constructible
// protocol, state, or outcome relation.
type Transition struct {
	schema   *universe
	resource ResourceOrigin
	key      Key
	from     StateCoordinate
	to       StateCoordinate
	final    bool
	content  keyspace.ContentID
}

// Key returns the exact resource range selected by this transition.
func (t Transition) Key() Key { return t.key }

// ContentID is the stable identity of this exact declared outcome.
func (t Transition) ContentID() keyspace.ContentID { return t.content }

// AcquisitionForSource derives the one acquisition declaration named by an
// existing Typestate resource source. The source itself carries the
// canonical protocol and declaration-row coordinate; Contract supplies its
// initial state.
func (schema Schema) AcquisitionForSource(origin ResourceSource, role materialization.Role) (Acquisition, bool) {
	if !schema.Valid() || !role.Valid() || !origin.validFor(schema.universe.source) || origin.kind != resourceSourceAcquisition {
		return Acquisition{}, false
	}
	source := schema.universe.source
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil || uint64(origin.row) > uint64(^uint(0)>>1) {
		return Acquisition{}, false
	}
	_, _, _, state, ok := contract.ProtocolAcquisitionAt(origin.protocol, int(origin.row))
	if !ok {
		return Acquisition{}, false
	}
	initial, ok := schema.stateCoordinate(origin.protocol, state)
	if !ok {
		return Acquisition{}, false
	}
	resource, ok := materializeResource(origin, role)
	if !ok {
		return Acquisition{}, false
	}
	key, ok := schema.Admit(resource)
	if !ok {
		return Acquisition{}, false
	}
	content := origin.contentID()
	if !content.Available() {
		return Acquisition{}, false
	}
	return Acquisition{schema: schema.universe, resource: resource, key: key, state: initial, content: content}, true
}

// TransitionForSource derives one exact Contract transition outcome for an
// input-backed Typestate resource source. transition and outcome are zero-based canonical
// Contract row coordinates, never Link relation handles.  The application
// owned by origin must expose the declaration's exact operation coordinate.
func (schema Schema) TransitionForSource(origin ResourceSource, transition, outcome int, role materialization.Role) (Transition, bool) {
	if !schema.Valid() || !role.Valid() || transition < 0 || outcome < 0 || !origin.validFor(schema.universe.source) || origin.kind != resourceSourceInput {
		return Transition{}, false
	}
	source := schema.universe.source
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return Transition{}, false
	}
	operation, kind, ordinal, fromState, ok := contract.TransitionAt(origin.protocol, transition)
	if !ok || origin.input.Kind != kind || origin.input.Ordinal != ordinal || !source.Boundary().ApplicationOperationAvailable(contract, origin.application, operation) {
		return Transition{}, false
	}
	outcomeOrdinal, toState, ok := contract.TransitionOutcomeAt(origin.protocol, transition, outcome)
	if !ok {
		return Transition{}, false
	}
	from, ok := schema.stateCoordinate(origin.protocol, fromState)
	if !ok {
		return Transition{}, false
	}
	to, ok := schema.stateCoordinate(origin.protocol, toState)
	if !ok {
		return Transition{}, false
	}
	final, ok := contract.StateFinal(origin.protocol, toState)
	if !ok {
		return Transition{}, false
	}
	resource, ok := materializeResource(origin, role)
	if !ok {
		return Transition{}, false
	}
	key, ok := schema.Admit(resource)
	if !ok {
		return Transition{}, false
	}
	content, ok := transitionContent(source, origin.protocol, origin, from, to, outcomeOrdinal, role)
	if !ok {
		return Transition{}, false
	}
	return Transition{schema: schema.universe, resource: resource, key: key, from: from, to: to, final: final, content: content}, true
}

func (schema Schema) validAcquisition(acquisition Acquisition) bool {
	return schema.Valid() && acquisition.schema == schema.universe && acquisition.content.Available() &&
		acquisition.resource.validFor(schema.universe.source) && acquisition.key == (Key{Resource: acquisition.resource}) &&
		func() bool { key, ok := schema.Admit(acquisition.resource); return ok && key == acquisition.key }()
}

// ValidAcquisition reports whether acquisition is bound to this exact sealed
// schema. Opaque declarations cannot be fabricated outside Typestate.
func (schema Schema) ValidAcquisition(acquisition Acquisition) bool {
	return schema.validAcquisition(acquisition)
}

func (schema Schema) validTransition(transition Transition) bool {
	return schema.Valid() && transition.schema == schema.universe && transition.content.Available() &&
		transition.resource.validFor(schema.universe.source) && transition.key == (Key{Resource: transition.resource}) &&
		func() bool { key, ok := schema.Admit(transition.resource); return ok && key == transition.key }()
}

// ValidTransition reports whether transition is bound to this exact sealed
// schema and remains an admitted resource rewrite declaration.
func (schema Schema) ValidTransition(transition Transition) bool {
	return schema.validTransition(transition)
}

func transitionContent(source *proglink.Link, protocol target.Protocol, origin ResourceSource, from, to StateCoordinate, ordinal uint32, role materialization.Role) (keyspace.ContentID, bool) {
	if source == nil || !source.ContentID().Available() || !role.Valid() {
		return keyspace.ContentID{}, false
	}
	protocolID := protocolCoordinateID(source.ContentID(), protocol)
	originID := origin.contentID()
	fromID, toID := from.ContentID(), to.ContentID()
	if !originID.Available() || !fromID.Available() || !toID.Available() {
		return keyspace.ContentID{}, false
	}
	linkID := source.ContentID()
	var payload [len("wippy.analysis.typestate.transition") + 32*5 + 8 + 8]byte
	offset := copy(payload[:], "wippy.analysis.typestate.transition")
	offset += copy(payload[offset:], linkID[:])
	offset += copy(payload[offset:], protocolID[:])
	offset += copy(payload[offset:], originID[:])
	offset += copy(payload[offset:], fromID[:])
	offset += copy(payload[offset:], toID[:])
	binary.BigEndian.PutUint64(payload[offset:offset+8], uint64(ordinal))
	binary.BigEndian.PutUint64(payload[offset+8:offset+16], uint64(role))
	return keyspace.ContentID(sha256.Sum256(payload[:])), true
}
