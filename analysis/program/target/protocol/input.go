package protocol

import (
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Input is the immutable input to the protocol compiler. Slices are copied at
// the boundary before canonicalization occurs.
type Input struct {
	Protocols  []vocabulary.ProtocolSpec
	Operations operation.Core
}

type protocolDraft struct {
	states          []stateRow
	acquisitions    []acquisitionDraft
	transitions     []transitionDraft
	requirements    []requirementDraft
	escapes         []escapeDraft
	callbackHolders []protocolCallbackHolderDraft
}

type acquisitionDraft struct {
	operationSource vocabulary.SpecRef
	operation       vocabulary.Operation
	outcomeSource   uint32
	outcome         uint32
	result          uint32
	state           vocabulary.State
}

type transitionDraft struct {
	operationSource vocabulary.SpecRef
	operation       vocabulary.Operation
	input           vocabulary.InputSource
	from            vocabulary.State
	outcomes        []transitionOutcomeDraft
}

type transitionOutcomeDraft struct {
	outcomeSource uint32
	outcome       uint32
	to            vocabulary.State
}

type requirementDraft struct {
	operationSource vocabulary.SpecRef
	operation       vocabulary.Operation
	input           vocabulary.InputSource
	state           vocabulary.State
}

type escapeDraft struct {
	operationSource vocabulary.SpecRef
	operation       vocabulary.Operation
	input           vocabulary.InputSource
}

type protocolCallbackHolderDraft struct {
	operationSource vocabulary.SpecRef
	operation       vocabulary.Operation
	input           vocabulary.InputSource
	callbackSource  vocabulary.CallbackRef
	callback        vocabulary.CallbackID
}

func freezeProtocols(input []vocabulary.ProtocolSpec) ([]protocolDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("protocol table", len(input)); err != nil {
		return nil, err
	}
	out := make([]protocolDraft, len(input))
	for index, protocol := range input {
		draft, err := freezeProtocol(protocol)
		if err != nil {
			return nil, fmt.Errorf("target: protocol %d: %w", index, err)
		}
		out[index] = draft
	}
	return out, nil
}

func freezeProtocol(input vocabulary.ProtocolSpec) (protocolDraft, error) {
	if len(input.Acquisitions) == 0 {
		return protocolDraft{}, errors.New("has no acquisitions")
	}
	if _, err := vocabulary.CheckedStoredLength("protocol acquisition table", len(input.Acquisitions)); err != nil {
		return protocolDraft{}, err
	}
	states, stateRefs, err := freezeProtocolStates(input.States)
	if err != nil {
		return protocolDraft{}, err
	}
	draft := protocolDraft{states: states}
	draft.acquisitions = make([]acquisitionDraft, len(input.Acquisitions))
	for index, item := range input.Acquisitions {
		state, ok := resolveStateRef(stateRefs, item.State)
		if !ok || item.Operation == 0 {
			return protocolDraft{}, fmt.Errorf("acquisition %d outside scope", index)
		}
		draft.acquisitions[index] = acquisitionDraft{
			operationSource: item.Operation, outcomeSource: item.Outcome,
			result: item.Result, state: state,
		}
	}
	if _, err := vocabulary.CheckedStoredLength("protocol transition table", len(input.Transitions)); err != nil {
		return protocolDraft{}, err
	}
	draft.transitions = make([]transitionDraft, len(input.Transitions))
	for index, item := range input.Transitions {
		from, ok := resolveStateRef(stateRefs, item.From)
		if !ok || item.Operation == 0 || len(item.Outcomes) == 0 {
			return protocolDraft{}, fmt.Errorf("transition %d outside scope", index)
		}
		if _, err := vocabulary.CheckedStoredLength("protocol transition outcome table", len(item.Outcomes)); err != nil {
			return protocolDraft{}, err
		}
		outcomes := make([]transitionOutcomeDraft, len(item.Outcomes))
		for outcomeIndex, outcome := range item.Outcomes {
			to, found := resolveStateRef(stateRefs, outcome.To)
			if !found {
				return protocolDraft{}, fmt.Errorf("transition %d outcome %d state outside scope", index, outcomeIndex)
			}
			outcomes[outcomeIndex] = transitionOutcomeDraft{outcomeSource: outcome.Outcome, to: to}
		}
		draft.transitions[index] = transitionDraft{operationSource: item.Operation, input: item.Input, from: from, outcomes: outcomes}
	}
	if _, err := vocabulary.CheckedStoredLength("protocol requirement table", len(input.Requirements)); err != nil {
		return protocolDraft{}, err
	}
	draft.requirements = make([]requirementDraft, len(input.Requirements))
	for index, item := range input.Requirements {
		state, ok := resolveStateRef(stateRefs, item.State)
		if !ok || item.Operation == 0 {
			return protocolDraft{}, fmt.Errorf("requirement %d outside scope", index)
		}
		draft.requirements[index] = requirementDraft{operationSource: item.Operation, input: item.Input, state: state}
	}
	if _, err := vocabulary.CheckedStoredLength("protocol escape table", len(input.Escapes)); err != nil {
		return protocolDraft{}, err
	}
	draft.escapes = make([]escapeDraft, len(input.Escapes))
	for index, item := range input.Escapes {
		if item.Operation == 0 {
			return protocolDraft{}, fmt.Errorf("escape %d has invalid operation", index)
		}
		draft.escapes[index] = escapeDraft{operationSource: item.Operation, input: item.Input}
	}
	if _, err := vocabulary.CheckedStoredLength("protocol callback-holder table", len(input.CallbackHolders)); err != nil {
		return protocolDraft{}, err
	}
	draft.callbackHolders = make([]protocolCallbackHolderDraft, len(input.CallbackHolders))
	for index, item := range input.CallbackHolders {
		if item.Operation == 0 || item.Callback == 0 {
			return protocolDraft{}, fmt.Errorf("callback holder %d outside scope", index)
		}
		draft.callbackHolders[index] = protocolCallbackHolderDraft{
			operationSource: item.Operation,
			input:           item.Input,
			callbackSource:  item.Callback,
		}
	}
	return draft, nil
}

func freezeProtocolStates(input []vocabulary.StateSpec) ([]stateRow, []vocabulary.State, error) {
	if len(input) == 0 {
		return nil, nil, errors.New("has no states")
	}
	if _, err := vocabulary.CheckedStoredLength("protocol state table", len(input)); err != nil {
		return nil, nil, err
	}
	type authoredState struct {
		source int
		row    stateRow
	}
	states := make([]authoredState, len(input))
	for index, state := range input {
		if state.Name == "" || !utf8.ValidString(state.Name) {
			return nil, nil, fmt.Errorf("state %d has invalid name", index)
		}
		if _, err := vocabulary.CheckedStoredLength("protocol state name bytes", len(state.Name)); err != nil {
			return nil, nil, err
		}
		states[index] = authoredState{source: index, row: stateRow{name: state.Name, final: state.Final}}
	}
	sort.Slice(states, func(i, j int) bool { return states[i].row.name < states[j].row.name })
	refs := make([]vocabulary.State, len(states))
	out := make([]stateRow, len(states))
	for index, state := range states {
		if index != 0 && state.row.name == states[index-1].row.name {
			return nil, nil, errors.New("duplicates state name")
		}
		handle, err := vocabulary.CheckedStoredLength("protocol state handle", index+1)
		if err != nil {
			return nil, nil, err
		}
		refs[state.source] = vocabulary.State(handle)
		out[index] = state.row
	}
	return out, refs, nil
}

func resolveStateRef(refs []vocabulary.State, ref vocabulary.StateRef) (vocabulary.State, bool) {
	if ref == 0 || uint64(ref) > uint64(len(refs)) {
		return 0, false
	}
	return refs[uint32(ref)-1], true
}
