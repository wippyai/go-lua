// Package obligation is the typestate rule's judgment: the sealed reading of
// the link's declared protocols, and the one fold that decides a mounted call
// actual against the state its resource is solved in.
//
// The state machine itself is domain/typestate's, the coordinate space is
// domain/typestate/statecell's, and the declared edges are
// analysis/program/target/protocol's sealed authority. This package names
// those three and adds no fourth: it declares no protocol, mints no state and
// invents no edge.
package obligation

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/program/target/protocol"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/typestate"
)

// obligationKind is which declaration an operation makes about one of its
// inputs. It is not a lattice: a declared input carries exactly one of these,
// and the absence of all three is the ordinary case of an argument no protocol
// speaks about.
type obligationKind uint8

const (
	obligationNone obligationKind = iota
	// obligationRequirement observes a state at entry and moves nothing.
	obligationRequirement
	// obligationTransition observes a state at entry and completes a declared
	// move on each of its outcome arms.
	obligationTransition
	// obligationEscape hands the resource somewhere this analysis does not
	// follow, discharging every proof about it.
	obligationEscape
)

// edge is one declared obligation of one operation input, read out of the
// sealed protocol table once at link and never re-derived per invocation.
//
// A requirement carries the state it observes and no arm. A transition carries
// the state it leaves and the states its outcome arms complete to; the arms
// are held together because a fold answers one call rather than one outcome,
// so the successor of a transition with several arms is their join.
type edge struct {
	kind     obligationKind
	observed typestate.State
	arrivals []typestate.State
}

// obligationKey addresses one declared obligation: the protocol that declares
// it, the operation that performs it, and the fixed actual position of the
// input it speaks about. The position is Pack's answer rather than this
// package's - an operation input coordinate is a formal, and which actual
// carries it is the interpretation Pack sealed for that operation.
type obligationKey struct {
	protocol  vocabulary.Protocol
	operation vocabulary.Operation
	actual    uint32
}

// inputPosition answers the fixed actual position one operation input
// coordinate occupies in a mounted call row.
type inputPosition func(vocabulary.Operation, vocabulary.InputSource) (int, bool)

// authority is the sealed reading of one link's protocol declarations: the
// state machine of every protocol, and the obligation every operation input
// carries under it.
//
// It is built once, from the sealed table and the sealed input selectors, and
// it is immutable afterwards. A fold reads it; nothing writes it.
type authority struct {
	definitions map[vocabulary.Protocol]typestate.Definition
	edges       map[obligationKey]edge
	// governed is the inverse of edges: which protocols speak about one
	// operation's actual at all. It is what says a call actual carries an
	// obligation, so the cells a judgment reads are the cells the declaration
	// named rather than every cell the resource has.
	governed map[siteKey][]vocabulary.Protocol
}

// siteKey is one operation's actual position, without the protocol: the
// address the governed index answers protocols at.
type siteKey struct {
	operation vocabulary.Operation
	actual    uint32
}

// sealAuthority reads the declared protocols out of the sealed table.
//
// position is a parameter rather than a captured schema so this reading states
// exactly what it consults: the declared edges, and where each declared input
// lands in a call row. Nothing else about a call enters here.
func sealAuthority(table *protocol.Table, position inputPosition) (authority, bool) {
	if table == nil || position == nil {
		return authority{}, false
	}
	sealed := authority{
		definitions: make(map[vocabulary.Protocol]typestate.Definition, table.ProtocolCount()),
		edges:       make(map[obligationKey]edge),
		governed:    make(map[siteKey][]vocabulary.Protocol),
	}
	for index := 0; index < table.ProtocolCount(); index++ {
		handle, handleOK := table.ProtocolAt(index)
		if !handleOK {
			return authority{}, false
		}
		if _, duplicate := sealed.definitions[handle]; duplicate {
			return authority{}, false
		}
		definition, states, definitionOK := sealDefinition(table, handle)
		if !definitionOK {
			return authority{}, false
		}
		sealed.definitions[handle] = definition
		if !sealed.sealEdges(table, handle, states, position) {
			return authority{}, false
		}
	}
	return sealed, true
}

// sealDefinition projects one protocol's declared states and edges into the
// state machine this domain judges against, and returns the state handles it
// resolved so the obligation reading resolves each one exactly once.
//
// The names are the table's own: a state is the string the declaration
// spelled, so a verdict names the state the author wrote rather than a handle
// only this analysis can read.
func sealDefinition(table *protocol.Table, handle vocabulary.Protocol) (typestate.Definition, map[vocabulary.State]typestate.State, bool) {
	definition := typestate.Definition{Protocol: protocolName(handle)}
	states := make(map[vocabulary.State]typestate.State, table.StateCount(handle))
	for index := 0; index < table.StateCount(handle); index++ {
		state, stateOK := table.StateAt(handle, index)
		if !stateOK {
			return typestate.Definition{}, nil, false
		}
		spelling, spellingOK := table.StateName(handle, state)
		if !spellingOK {
			return typestate.Definition{}, nil, false
		}
		named, namedOK := typestate.StateFromString(spelling)
		if !namedOK {
			return typestate.Definition{}, nil, false
		}
		if _, duplicate := states[state]; duplicate {
			return typestate.Definition{}, nil, false
		}
		states[state] = named
		definition.States = append(definition.States, named)
		final, finalOK := table.StateFinal(handle, state)
		if !finalOK {
			return typestate.Definition{}, nil, false
		}
		if final {
			definition.FinalStates = append(definition.FinalStates, named)
		}
	}
	for index := 0; index < table.TransitionCount(handle); index++ {
		_, _, _, from, transitionOK := table.TransitionAt(handle, index)
		if !transitionOK {
			return typestate.Definition{}, nil, false
		}
		departure, departureOK := states[from]
		if !departureOK {
			return typestate.Definition{}, nil, false
		}
		for arm := 0; arm < table.TransitionOutcomeCount(handle, index); arm++ {
			_, to, armOK := table.TransitionOutcomeAt(handle, index, arm)
			if !armOK {
				return typestate.Definition{}, nil, false
			}
			arrival, arrivalOK := states[to]
			if !arrivalOK {
				return typestate.Definition{}, nil, false
			}
			definition.Transitions = append(definition.Transitions, typestate.TransitionDecl{From: departure, To: arrival})
		}
	}
	return definition.Normalized(), states, true
}

// sealEdges records what every declared operation input of one protocol
// obliges, addressed by the actual position that input occupies.
func (sealed authority) sealEdges(table *protocol.Table, handle vocabulary.Protocol, states map[vocabulary.State]typestate.State, position inputPosition) bool {
	for index := 0; index < table.ProtocolRequirementCount(handle); index++ {
		operation, input, state, rowOK := table.ProtocolRequirementAt(handle, index)
		if !rowOK {
			return false
		}
		observed, observedOK := states[state]
		if !observedOK {
			return false
		}
		if !sealed.record(handle, operation, input, position, edge{kind: obligationRequirement, observed: observed}) {
			return false
		}
	}
	for index := 0; index < table.TransitionCount(handle); index++ {
		operation, kind, ordinal, from, rowOK := table.TransitionAt(handle, index)
		if !rowOK {
			return false
		}
		departure, departureOK := states[from]
		if !departureOK {
			return false
		}
		moved := edge{kind: obligationTransition, observed: departure}
		for arm := 0; arm < table.TransitionOutcomeCount(handle, index); arm++ {
			_, to, armOK := table.TransitionOutcomeAt(handle, index, arm)
			if !armOK {
				return false
			}
			arrival, arrivalOK := states[to]
			if !arrivalOK {
				return false
			}
			moved.arrivals = append(moved.arrivals, arrival)
		}
		if !sealed.record(handle, operation, vocabulary.InputSource{Kind: kind, Ordinal: ordinal}, position, moved) {
			return false
		}
	}
	for index := 0; index < table.EscapeCount(handle); index++ {
		operation, kind, ordinal, rowOK := table.EscapeAt(handle, index)
		if !rowOK {
			return false
		}
		if !sealed.record(handle, operation, vocabulary.InputSource{Kind: kind, Ordinal: ordinal}, position, edge{kind: obligationEscape}) {
			return false
		}
	}
	return true
}

// record places one declared obligation at the actual position its input
// coordinate occupies. A coordinate Pack has no interpretation for addresses
// no actual of a call row, so it records nothing rather than being guessed
// into a position.
//
// Two declarations at one position are refused rather than merged: which of
// them a fold applied would decide the verdict, and a reading that picked one
// would be a second authority over the declaration.
func (sealed authority) record(handle vocabulary.Protocol, operation vocabulary.Operation, input vocabulary.InputSource, position inputPosition, declared edge) bool {
	actual, actualOK := position(operation, input)
	if !actualOK || actual < 0 {
		return true
	}
	key := obligationKey{protocol: handle, operation: operation, actual: uint32(actual)}
	if _, duplicate := sealed.edges[key]; duplicate {
		return false
	}
	sealed.edges[key] = declared
	site := siteKey{operation: operation, actual: uint32(actual)}
	sealed.governed[site] = append(sealed.governed[site], handle)
	return true
}

// protocolsAt are the protocols that declare an obligation about one
// operation's actual, in the order the sealed table declared them. A call
// actual no protocol speaks about answers the empty set, which is what says
// this rule reads no cell for it.
func (sealed authority) protocolsAt(operation vocabulary.Operation, actual uint32) []vocabulary.Protocol {
	return sealed.governed[siteKey{operation: operation, actual: actual}]
}

// obligationAt is the obligation one operation declares about the actual at
// position under one protocol.
func (sealed authority) obligationAt(handle vocabulary.Protocol, operation vocabulary.Operation, actual uint32) (edge, bool) {
	declared, found := sealed.edges[obligationKey{protocol: handle, operation: operation, actual: actual}]
	return declared, found
}

// definitionFor is the state machine of one protocol.
func (sealed authority) definitionFor(handle vocabulary.Protocol) (typestate.Definition, bool) {
	definition, found := sealed.definitions[handle]
	return definition, found
}

// protocolName is the spelling one sealed protocol handle is judged under. The
// table numbers protocols and does not name them, so the name is the handle's
// own ordinal: a Definition's Protocol field is an identity inside one
// judgment rather than a diagnostic surface.
func protocolName(handle vocabulary.Protocol) typestate.Protocol {
	return typestate.Protocol("protocol/" + strconv.FormatUint(uint64(handle), 10))
}
