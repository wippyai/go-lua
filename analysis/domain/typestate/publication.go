package typestate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// The built-in protocol declarations are the canonical lifecycle vocabulary
// used by engine publications. Protocol and state names remain extensible for
// manifests, but these declarations prevent the built-in channel/resource
// lanes from carrying a second, implicit state machine.
const (
	ProtocolChannel     Protocol = "channel"
	ProtocolConnection  Protocol = "connection"
	ProtocolTransaction Protocol = "transaction"

	StateOpen      State = "open"
	StateClosed    State = "closed"
	StateActive    State = "active"
	StateCommitted State = "committed"
)

var builtinDefinitions = map[Protocol]Definition{
	ProtocolChannel: {
		Protocol:    ProtocolChannel,
		States:      []State{StateOpen, StateClosed},
		FinalStates: []State{StateClosed},
		Transitions: []TransitionDecl{{From: StateOpen, To: StateClosed}},
	},
	ProtocolConnection: {
		Protocol:    ProtocolConnection,
		States:      []State{StateOpen, StateClosed},
		FinalStates: []State{StateClosed},
		Transitions: []TransitionDecl{{From: StateOpen, To: StateClosed}},
	},
	ProtocolTransaction: {
		Protocol:    ProtocolTransaction,
		States:      []State{StateActive, StateCommitted},
		FinalStates: []State{StateCommitted},
		Transitions: []TransitionDecl{{From: StateActive, To: StateCommitted}},
	},
}

// BuiltinDefinition returns an independent copy of a built-in protocol
// declaration. Unknown protocols fail closed rather than borrowing the state
// vocabulary of another protocol.
func BuiltinDefinition(protocol Protocol) (Definition, bool) {
	definition, ok := builtinDefinitions[protocol]
	if !ok {
		return Definition{}, false
	}
	return definition.Clone(), true
}

// Publication is one exact resource observation transported through a
// fixpoint fact. Resource owns identity and protocol; Slot owns current state,
// obligation, and locality. Keeping all four fields together makes escape a
// locality transition instead of a sentinel state.
type Publication struct {
	Resource Resource
	Slot     Slot
}

// AcquirePublication constructs the initial exact observation for a built-in
// protocol. A resource can only enter a state declared by its own protocol,
// and every obligation target must be a declared final state.
func AcquirePublication(resource Resource, current State, obligation Obligation) (Publication, bool) {
	definition, ok := BuiltinDefinition(resource.Protocol)
	if !ok || resource.ID == "" || !definition.HasState(current) || !validObligation(definition, obligation) {
		return Publication{}, false
	}
	locality := LocalityOpen
	if obligation.SatisfiedBy(current) || definition.IsFinal(current) {
		locality = LocalityClosed
	}
	return Publication{
		Resource: resource,
		Slot:     Slot{Current: current, Obligation: obligation, Locality: locality},
	}, true
}

// Transition returns the next exact observation when the resource remains
// locally controlled and its protocol declares the current -> target edge.
func (p Publication) Transition(target State) (Publication, bool) {
	definition, ok := BuiltinDefinition(p.Resource.Protocol)
	if !ok || !p.valid(definition) || p.Slot.Locality != LocalityOpen ||
		!definition.AllowsTransition(p.Slot.Current, target) {
		return Publication{}, false
	}
	next := p
	next.Slot.Current = target
	if next.Slot.Obligation.SatisfiedBy(target) || definition.IsFinal(target) {
		next.Slot.Locality = LocalityClosed
	}
	return next, true
}

// Escape transfers lifecycle authority away from the local analysis. The
// current protocol state and obligation remain as history, while Locality is
// the sole authority for whether later local operations may consume them.
func (p Publication) Escape() (Publication, bool) {
	definition, ok := BuiltinDefinition(p.Resource.Protocol)
	if !ok || !p.valid(definition) ||
		p.Slot.Locality == LocalityEscaped {
		return Publication{}, false
	}
	next := p
	next.Slot.Locality = LocalityEscaped
	return next, true
}

// Requires reports an exact locally controlled state requirement. Escaped and
// unknown observations cannot prove a precondition merely because they retain
// the last state seen before authority was lost.
func (p Publication) Requires(required State) bool {
	return required != "" &&
		(p.Slot.Locality == LocalityOpen || p.Slot.Locality == LocalityClosed) &&
		p.Slot.Current == required
}

// LocallyControlled reports whether the current state may still authorize a
// local operation. Escaped and unknown resources retain no such authority.
func (p Publication) LocallyControlled() bool {
	return p.Slot.Locality == LocalityOpen || p.Slot.Locality == LocalityClosed
}

func (p Publication) valid(definition Definition) bool {
	if p.Resource.ID == "" ||
		p.Resource.Protocol != definition.Protocol ||
		!definition.HasState(p.Slot.Current) ||
		!validObligation(definition, p.Slot.Obligation) {
		return false
	}
	switch p.Slot.Locality {
	case LocalityOpen:
		return !definition.IsFinal(p.Slot.Current)
	case LocalityClosed:
		return definition.IsFinal(p.Slot.Current)
	case LocalityEscaped, LocalityUnknown:
		return true
	default:
		return false
	}
}

func validObligation(definition Definition, obligation Obligation) bool {
	if obligation.Final != "" && !definition.IsFinal(obligation.Final) {
		return false
	}
	finals := obligation.Finals.States()
	if obligation.Finals != "" && finals == nil {
		return false
	}
	for _, final := range finals {
		if !definition.IsFinal(final) {
			return false
		}
	}
	return true
}

const publicationWireVersion = 1

type publicationWire struct {
	Version     uint8    `json:"version"`
	Resource    []byte   `json:"resource"`
	Protocol    string   `json:"protocol"`
	State       string   `json:"state"`
	Locality    Locality `json:"locality"`
	Final       string   `json:"final,omitempty"`
	FinalStates []string `json:"final_states,omitempty"`
}

// EncodePublication serializes a validated built-in publication. Callers
// cannot use this codec to smuggle an undeclared state or sentinel locality
// into a lifecycle fact.
func EncodePublication(publication Publication) ([]byte, bool) {
	definition, ok := BuiltinDefinition(publication.Resource.Protocol)
	if !ok || !publication.valid(definition) {
		return nil, false
	}
	finals := publication.Slot.Obligation.Finals.States()
	wire := publicationWire{
		Version:     publicationWireVersion,
		Resource:    []byte(publication.Resource.ID),
		Protocol:    publication.Resource.Protocol.String(),
		State:       publication.Slot.Current.String(),
		Locality:    publication.Slot.Locality,
		Final:       publication.Slot.Obligation.Final.String(),
		FinalStates: make([]string, len(finals)),
	}
	for index, final := range finals {
		wire.FinalStates[index] = final.String()
	}
	encoded, err := json.Marshal(wire)
	return encoded, err == nil
}

// DecodePublication strictly decodes and validates one built-in publication.
// Unknown fields, versions, protocols, states, localities, obligations, and
// trailing documents all fail closed.
func DecodePublication(encoded []byte) (Publication, bool) {
	var wire publicationWire
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Publication{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Publication{}, false
	}
	if wire.Version != publicationWireVersion || len(wire.Resource) == 0 {
		return Publication{}, false
	}
	protocol, ok := ProtocolFromString(wire.Protocol)
	if !ok {
		return Publication{}, false
	}
	state, ok := StateFromString(wire.State)
	if !ok {
		return Publication{}, false
	}
	final, ok := optionalConcreteState(wire.Final)
	if !ok {
		return Publication{}, false
	}
	finals := make([]State, len(wire.FinalStates))
	for index, name := range wire.FinalStates {
		finals[index], ok = StateFromString(name)
		if !ok {
			return Publication{}, false
		}
	}
	publication := Publication{
		Resource: Resource{ID: ResourceID(string(wire.Resource)), Protocol: protocol},
		Slot: Slot{
			Current:  state,
			Locality: wire.Locality,
			Obligation: Obligation{
				Final:  final,
				Finals: NewFinalStates(finals...),
			},
		},
	}
	definition, ok := BuiltinDefinition(protocol)
	return publication, ok && publication.valid(definition)
}

func optionalConcreteState(name string) (State, bool) {
	if name == "" {
		return "", true
	}
	state, ok := StateFromString(name)
	return state, ok
}

// ValidateBuiltinDefinitions is retained as a cheap package invariant for
// callers and tests that need to prove the declared registry is internally
// sound without observing its mutable representation.
func ValidateBuiltinDefinitions() error {
	for protocol, definition := range builtinDefinitions {
		if definition.Protocol != protocol {
			return fmt.Errorf("typestate built-in protocol %q has mismatched declaration %q", protocol, definition.Protocol)
		}
		if err := definition.Validate(); err != nil {
			return err
		}
	}
	return nil
}
