package manifesttarget

import (
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

// lifecycleKind names the protocol relation one signature lifecycle label
// declares. It exists so the declaration effect row is walked exactly once,
// beside the ownership row, and no lifecycle label can fall out of that walk
// unnoticed.
type lifecycleKind uint8

const (
	lifecycleTransitionDeclaration lifecycleKind = iota + 1
	lifecycleEscapeDeclaration
	lifecycleParameterAcquisition
)

// lifecycleDeclaration is one protocol-addressed row read from a declaration
// signature. Param is the authored effect.ParamRef index and keeps its signed
// spelling: resolving it is the protocol projection's decision, not the
// reader's.
type lifecycleDeclaration struct {
	Kind     lifecycleKind
	Protocol typestate.Protocol
	From     typestate.State
	To       typestate.State
	Param    int
}

// protocolDraft accumulates the rows of one declared state machine while the
// catalogue is walked. States come from the manifest FSM, so a state coordinate
// is fixed before any operation row refers to it.
type protocolDraft struct {
	definition typestate.Definition
	states     []vocabulary.StateSpec
	stateRefs  map[typestate.State]vocabulary.StateRef

	acquisitions []vocabulary.AcquisitionSpec
	transitions  []vocabulary.TransitionSpec
	escapes      []vocabulary.EscapeSpec
}

// protocols projects the manifest typestate declarations into the sealed
// protocol vocabulary. The manifest FSM owns the state set and its finality;
// operation laws own acquisition, and signature lifecycle labels own the
// transition and escape relations. Nothing here is derived from a member name,
// a return type, or a provider identity.
func protocols(catalogue *authoredCatalogue, declarations *manifest.Catalogue) ([]vocabulary.ProtocolSpec, error) {
	definitions := declarations.TypestateProtocols()
	if len(definitions) == 0 {
		return nil, nil
	}
	names := make([]typestate.Protocol, 0, len(definitions))
	drafts := make(map[typestate.Protocol]*protocolDraft, len(definitions))
	for protocol, definition := range definitions {
		names = append(names, protocol)
		drafts[protocol] = newProtocolDraft(definition)
	}
	sort.Slice(names, func(left, right int) bool { return names[left] < names[right] })

	for _, declaration := range declarations.Functions() {
		path := declaration.CanonicalPath()
		ref, err := catalogue.require(path)
		if err != nil {
			return nil, err
		}
		if law, ok := declaration.Operation(); ok {
			if err := appendAcquisitions(drafts, path, ref, law); err != nil {
				return nil, err
			}
		}
		if err := appendLifecycleRelations(catalogue, drafts, path, ref); err != nil {
			return nil, err
		}
	}

	out := make([]vocabulary.ProtocolSpec, 0, len(names))
	for _, protocol := range names {
		draft := drafts[protocol]
		if len(draft.acquisitions) == 0 {
			return nil, fmt.Errorf(
				"target catalogue: typestate protocol %q declares no acquisition; the sealed protocol vocabulary identifies a protocol by the result slots that create it",
				protocol,
			)
		}
		out = append(out, vocabulary.ProtocolSpec{
			Acquisitions: draft.acquisitions,
			States:       draft.states,
			Transitions:  draft.transitions,
			Escapes:      draft.escapes,
		})
	}
	return out, nil
}

func newProtocolDraft(definition typestate.Definition) *protocolDraft {
	normalized := definition.Normalized()
	draft := &protocolDraft{
		definition: normalized,
		states:     make([]vocabulary.StateSpec, 0, len(normalized.States)),
		stateRefs:  make(map[typestate.State]vocabulary.StateRef, len(normalized.States)),
	}
	for index, state := range normalized.States {
		draft.states = append(draft.states, vocabulary.StateSpec{Name: state.String(), Final: normalized.IsFinal(state)})
		draft.stateRefs[state] = vocabulary.StateRef(index + 1)
	}
	return draft
}

func (draft *protocolDraft) stateRef(state typestate.State) (vocabulary.StateRef, error) {
	ref, ok := draft.stateRefs[state]
	if !ok {
		return 0, fmt.Errorf("protocol %q does not declare state %q", draft.definition.Protocol, state)
	}
	return ref, nil
}

func requireProtocolDraft(drafts map[typestate.Protocol]*protocolDraft, protocol typestate.Protocol) (*protocolDraft, error) {
	draft, ok := drafts[protocol]
	if !ok {
		return nil, fmt.Errorf("lifecycle protocol %q is not declared as a typestate state machine", protocol)
	}
	return draft, nil
}

// appendAcquisitions projects the operation-law acquisition rows. The result
// coordinate crosses unchanged: the target compiler is the authority on
// whether the named outcome owns that fixed slot.
func appendAcquisitions(drafts map[typestate.Protocol]*protocolDraft, path string, ref operationRef, law moduleio.Operation) error {
	for index, acquisition := range law.Acquisitions {
		protocol, ok := typestate.ProtocolFromString(acquisition.Protocol)
		if !ok {
			return fmt.Errorf("target catalogue: %s acquisition %d has no protocol", path, index)
		}
		draft, err := requireProtocolDraft(drafts, protocol)
		if err != nil {
			return fmt.Errorf("target catalogue: %s acquisition %d: %w", path, index, err)
		}
		state, ok := typestate.StateFromString(acquisition.State)
		if !ok {
			return fmt.Errorf("target catalogue: %s acquisition %d has no state", path, index)
		}
		stateRef, err := draft.stateRef(state)
		if err != nil {
			return fmt.Errorf("target catalogue: %s acquisition %d: %w", path, index, err)
		}
		draft.acquisitions = append(draft.acquisitions, vocabulary.AcquisitionSpec{
			Operation: vocabulary.SpecRef(ref), Outcome: acquisition.Outcome,
			Result: acquisition.Result, State: stateRef,
		})
	}
	return nil
}

// appendLifecycleRelations projects the signature lifecycle labels of one
// declaration. A transition applies on the operation's normal arms only: an
// operation that throws states no completed move, and treating the throw arm
// as one would discharge an obligation the provider never discharged.
func appendLifecycleRelations(catalogue *authoredCatalogue, drafts map[typestate.Protocol]*protocolDraft, path string, ref operationRef) error {
	rows := catalogue.lifecycles[path]
	if len(rows) == 0 {
		return nil
	}
	operation := catalogue.at(ref)
	for index, row := range rows {
		draft, err := requireProtocolDraft(drafts, row.Protocol)
		if err != nil {
			return fmt.Errorf("target catalogue: %s lifecycle %d: %w", path, index, err)
		}
		if row.Kind == lifecycleParameterAcquisition {
			return fmt.Errorf(
				"target catalogue: %s lifecycle %d acquires protocol %q through parameter %d; the sealed protocol vocabulary acquires a fixed result slot, so state the acquisition as an operation-law result declaration",
				path, index, row.Protocol, row.Param,
			)
		}
		input, err := lifecycleInput(operation, row.Param)
		if err != nil {
			return fmt.Errorf("target catalogue: %s lifecycle %d: %w", path, index, err)
		}
		switch row.Kind {
		case lifecycleEscapeDeclaration:
			draft.escapes = append(draft.escapes, vocabulary.EscapeSpec{Operation: vocabulary.SpecRef(ref), Input: input})
		case lifecycleTransitionDeclaration:
			from, fromErr := draft.stateRef(row.From)
			if fromErr != nil {
				return fmt.Errorf("target catalogue: %s lifecycle %d: %w", path, index, fromErr)
			}
			to, toErr := draft.stateRef(row.To)
			if toErr != nil {
				return fmt.Errorf("target catalogue: %s lifecycle %d: %w", path, index, toErr)
			}
			outcomes := normalOutcomes(operation)
			if len(outcomes) == 0 {
				return fmt.Errorf(
					"target catalogue: %s lifecycle %d moves protocol %q but the operation has no normal outcome to complete it",
					path, index, row.Protocol,
				)
			}
			transition := vocabulary.TransitionSpec{Operation: vocabulary.SpecRef(ref), Input: input, From: from}
			for _, outcome := range outcomes {
				transition.Outcomes = append(transition.Outcomes, vocabulary.TransitionOutcomeSpec{Outcome: outcome, To: to})
			}
			draft.transitions = append(draft.transitions, transition)
		default:
			return fmt.Errorf("target catalogue: %s lifecycle %d has no relation", path, index)
		}
	}
	return nil
}

// lifecycleInput resolves an authored effect.ParamRef against the operation's
// fixed input geometry. A negative index selects an argument counted from the
// end of a runtime call; the sealed contract has no such coordinate, so that
// declaration is refused rather than approximated by a fixed ordinal.
func lifecycleInput(operation *vocabulary.OperationSpec, param int) (vocabulary.InputSource, error) {
	if param < 0 {
		return vocabulary.InputSource{}, fmt.Errorf("subject parameter %d is counted from the call end and has no fixed input coordinate", param)
	}
	if param >= len(operation.Input.Fixed) {
		return vocabulary.InputSource{}, fmt.Errorf("subject parameter %d is outside the operation's %d fixed inputs", param, len(operation.Input.Fixed))
	}
	return vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(param)}, nil
}

func normalOutcomes(operation *vocabulary.OperationSpec) []uint32 {
	var out []uint32
	for index, outcome := range operation.Outcomes {
		if outcome.Kind == flowkind.OutcomeNormal {
			out = append(out, uint32(index))
		}
	}
	return out
}
