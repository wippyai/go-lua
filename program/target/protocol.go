package target

import (
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"
)

type protocolDraft struct {
	states          []stateRow
	acquisitions    []acquisitionDraft
	transitions     []transitionDraft
	escapes         []escapeDraft
	callbackHolders []protocolCallbackHolderDraft
}

type acquisitionDraft struct {
	operationSource SpecRef
	operation       Operation
	outcomeSource   uint32
	outcome         uint32
	result          uint32
	state           State
}

type transitionDraft struct {
	operationSource SpecRef
	operation       Operation
	input           InputSource
	from            State
	outcomes        []transitionOutcomeDraft
}

type transitionOutcomeDraft struct {
	outcomeSource uint32
	outcome       uint32
	to            State
}

type escapeDraft struct {
	operationSource SpecRef
	operation       Operation
	input           InputSource
}

type protocolCallbackHolderDraft struct {
	operationSource SpecRef
	operation       Operation
	input           InputSource
	callbackSource  CallbackRef
	callback        CallbackID
}

func freezeProtocols(input []ProtocolSpec) ([]protocolDraft, error) {
	if _, err := checkedStoredLength("protocol table", len(input)); err != nil {
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

func freezeProtocol(input ProtocolSpec) (protocolDraft, error) {
	if len(input.Acquisitions) == 0 {
		return protocolDraft{}, errors.New("has no acquisitions")
	}
	if _, err := checkedStoredLength("protocol acquisition table", len(input.Acquisitions)); err != nil {
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
	if _, err := checkedStoredLength("protocol transition table", len(input.Transitions)); err != nil {
		return protocolDraft{}, err
	}
	draft.transitions = make([]transitionDraft, len(input.Transitions))
	for index, item := range input.Transitions {
		from, ok := resolveStateRef(stateRefs, item.From)
		if !ok || item.Operation == 0 || len(item.Outcomes) == 0 {
			return protocolDraft{}, fmt.Errorf("transition %d outside scope", index)
		}
		if _, err := checkedStoredLength("protocol transition outcome table", len(item.Outcomes)); err != nil {
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
	if _, err := checkedStoredLength("protocol escape table", len(input.Escapes)); err != nil {
		return protocolDraft{}, err
	}
	draft.escapes = make([]escapeDraft, len(input.Escapes))
	for index, item := range input.Escapes {
		if item.Operation == 0 {
			return protocolDraft{}, fmt.Errorf("escape %d has invalid operation", index)
		}
		draft.escapes[index] = escapeDraft{operationSource: item.Operation, input: item.Input}
	}
	if _, err := checkedStoredLength("protocol callback-holder table", len(input.CallbackHolders)); err != nil {
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

func freezeProtocolStates(input []StateSpec) ([]stateRow, []State, error) {
	if len(input) == 0 {
		return nil, nil, errors.New("has no states")
	}
	if _, err := checkedStoredLength("protocol state table", len(input)); err != nil {
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
		if _, err := checkedStoredLength("protocol state name bytes", len(state.Name)); err != nil {
			return nil, nil, err
		}
		states[index] = authoredState{source: index, row: stateRow{name: state.Name, final: state.Final}}
	}
	sort.Slice(states, func(i, j int) bool { return states[i].row.name < states[j].row.name })
	refs := make([]State, len(states))
	out := make([]stateRow, len(states))
	for index, state := range states {
		if index != 0 && state.row.name == states[index-1].row.name {
			return nil, nil, errors.New("duplicates state name")
		}
		handle, err := checkedStoredHandle("protocol state handle", index)
		if err != nil {
			return nil, nil, err
		}
		refs[state.source] = State(handle)
		out[index] = state.row
	}
	return out, refs, nil
}

func resolveStateRef(refs []State, ref StateRef) (State, bool) {
	if ref == 0 || uint64(ref) > uint64(len(refs)) {
		return 0, false
	}
	return refs[uint32(ref)-1], true
}

func resolveProtocols(drafts []protocolDraft, operations []operationDraft, sourceOperation []Operation) error {
	outcomeRemaps := make([][]uint32, len(operations))
	for index := range operations {
		outcomes := operations[index].outcomes
		remap := make([]uint32, len(outcomes))
		for canonical, outcome := range outcomes {
			remap[outcome.source] = uint32(canonical)
		}
		outcomeRemaps[index] = remap
	}
	for index := range drafts {
		if err := drafts[index].resolve(operations, outcomeRemaps, sourceOperation); err != nil {
			return err
		}
	}
	sort.Slice(drafts, func(i, j int) bool { return compareProtocol(drafts[i], drafts[j]) < 0 })
	for index := 1; index < len(drafts); index++ {
		if sameAcquisitionTuple(drafts[index-1].acquisitions[0], drafts[index].acquisitions[0]) {
			return errors.New("target: duplicate protocol acquisition head")
		}
	}
	seen := make(map[acquisitionKey]struct{})
	for index := range drafts {
		for _, acquisition := range drafts[index].acquisitions {
			key := acquisitionKey{operation: acquisition.operation, outcome: acquisition.outcome, result: acquisition.result}
			if _, exists := seen[key]; exists {
				return errors.New("target: acquisition belongs to multiple protocols")
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

type acquisitionKey struct {
	operation       Operation
	outcome, result uint32
}

func (d *protocolDraft) resolve(operations []operationDraft, outcomeRemaps [][]uint32, sourceOperation []Operation) error {
	resolveOperation := func(ref SpecRef) (Operation, *operationDraft, []uint32, error) {
		if ref == 0 || uint64(ref) > uint64(len(sourceOperation)) {
			return 0, nil, nil, errors.New("target: protocol references unknown operation")
		}
		op := sourceOperation[uint32(ref)-1]
		if op == 0 || uint64(op) > uint64(len(operations)) {
			return 0, nil, nil, errors.New("target: protocol references unresolved operation")
		}
		index := uint32(op) - 1
		return op, &operations[index], outcomeRemaps[index], nil
	}
	for index := range d.acquisitions {
		row := &d.acquisitions[index]
		op, operation, remap, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if uint64(row.outcomeSource) >= uint64(len(remap)) {
			return fmt.Errorf("target: acquisition %d outcome outside scope", index)
		}
		outcome := remap[row.outcomeSource]
		if uint64(row.result) >= uint64(len(operation.outcomes[outcome].values.types)) {
			return fmt.Errorf("target: acquisition %d result outside fixed outcome", index)
		}
		row.operation, row.outcome = op, outcome
	}
	sort.Slice(d.acquisitions, func(i, j int) bool { return compareAcquisition(d.acquisitions[i], d.acquisitions[j]) < 0 })
	for index := 1; index < len(d.acquisitions); index++ {
		if sameAcquisitionTuple(d.acquisitions[index-1], d.acquisitions[index]) {
			return errors.New("target: duplicate protocol acquisition")
		}
	}
	for index := range d.transitions {
		row := &d.transitions[index]
		op, operation, remap, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if !validAuthoredInputSource(row.input, operation.valueFormalCount(), operation.valuesVars) {
			return fmt.Errorf("target: transition %d has source outside scope", index)
		}
		row.operation = op
		for outcomeIndex := range row.outcomes {
			if uint64(row.outcomes[outcomeIndex].outcomeSource) >= uint64(len(remap)) {
				return fmt.Errorf("target: transition %d outcome %d outside scope", index, outcomeIndex)
			}
			outcome := remap[row.outcomes[outcomeIndex].outcomeSource]
			row.outcomes[outcomeIndex].outcome = outcome
		}
		sort.Slice(row.outcomes, func(i, j int) bool { return row.outcomes[i].outcome < row.outcomes[j].outcome })
		for outcomeIndex := 1; outcomeIndex < len(row.outcomes); outcomeIndex++ {
			if row.outcomes[outcomeIndex-1].outcome == row.outcomes[outcomeIndex].outcome {
				return fmt.Errorf("target: transition %d duplicates outcome", index)
			}
		}
	}
	sort.Slice(d.transitions, func(i, j int) bool { return compareTransition(d.transitions[i], d.transitions[j]) < 0 })
	for index := 1; index < len(d.transitions); index++ {
		if compareTransitionKey(d.transitions[index-1], d.transitions[index]) == 0 {
			return errors.New("target: duplicate protocol transition")
		}
	}
	for index := range d.escapes {
		row := &d.escapes[index]
		op, operation, _, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if !validAuthoredInputSource(row.input, operation.valueFormalCount(), operation.valuesVars) {
			return fmt.Errorf("target: escape %d has source outside scope", index)
		}
		row.operation = op
	}
	sort.Slice(d.escapes, func(i, j int) bool { return compareEscape(d.escapes[i], d.escapes[j]) < 0 })
	for index := 1; index < len(d.escapes); index++ {
		if compareEscapeKey(d.escapes[index-1], d.escapes[index]) == 0 {
			return errors.New("target: duplicate protocol escape")
		}
	}
	for index := range d.callbackHolders {
		row := &d.callbackHolders[index]
		op, operation, _, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if len(operation.bindings) == 0 {
			return fmt.Errorf("target: callback holder %d operation is not source-visible", index)
		}
		if !validAuthoredInputSource(row.input, operation.valueFormalCount(), operation.valuesVars) {
			return fmt.Errorf("target: callback holder %d has source outside scope", index)
		}
		row.operation = op
	}
	return d.canonicalizeStates()
}

// resolveProtocolCallbackHolders runs after appendOperation has assigned the
// canonical CallbackID range. Protocol shape can be resolved earlier, but a
// CallbackRef intentionally has no globally meaningful identity until then.
func resolveProtocolCallbackHolders(protocols []protocolDraft, operations []operationDraft) error {
	for protocolIndex := range protocols {
		draft := &protocols[protocolIndex]
		for holderIndex := range draft.callbackHolders {
			holder := &draft.callbackHolders[holderIndex]
			if holder.operation == 0 || uint64(holder.operation) > uint64(len(operations)) || holder.callbackSource == 0 {
				return fmt.Errorf("target: protocol %d callback holder %d unresolved", protocolIndex, holderIndex)
			}
			callback, found := operations[uint32(holder.operation)-1].callbackBySource(int(holder.callbackSource - 1))
			if !found || callback.sealed == 0 {
				return fmt.Errorf("target: protocol %d callback holder %d callback outside operation scope", protocolIndex, holderIndex)
			}
			if !retainedCallbackLifecycle(callback.lifecycle) {
				return fmt.Errorf("target: protocol %d callback holder %d callback is not retained", protocolIndex, holderIndex)
			}
			holder.callback = callback.sealed
		}
		sort.Slice(draft.callbackHolders, func(left, right int) bool {
			return compareProtocolCallbackHolder(draft.callbackHolders[left], draft.callbackHolders[right]) < 0
		})
		for index := 1; index < len(draft.callbackHolders); index++ {
			if compareProtocolCallbackHolder(draft.callbackHolders[index-1], draft.callbackHolders[index]) == 0 {
				return fmt.Errorf("target: protocol %d has duplicate callback holder", protocolIndex)
			}
		}
	}
	// Protocol handles are assigned only after this point. Re-sort after every
	// semantic row has been resolved so declaration order cannot affect them.
	sort.Slice(protocols, func(i, j int) bool { return compareProtocol(protocols[i], protocols[j]) < 0 })
	return nil
}

// canonicalizeStates gives nominal states alpha-invariant local coordinates.
// Names remain diagnostics only: roots and edges entirely determine traversal
// order. This preserves state cardinality and graph topology; it never merges
// otherwise bisimilar states.
func (d *protocolDraft) canonicalizeStates() error {
	if len(d.states) == 0 {
		return errors.New("target: protocol has no states")
	}
	assigned := make([]State, len(d.states)+1)
	ordered := make([]State, 0, len(d.states))
	queue := make([]State, 0, len(d.states))
	assign := func(state State) error {
		if state == 0 || uint64(state) > uint64(len(d.states)) {
			return errors.New("target: protocol state outside scope")
		}
		if assigned[state] != 0 {
			return nil
		}
		assigned[state] = State(len(ordered) + 1)
		ordered = append(ordered, state)
		queue = append(queue, state)
		return nil
	}
	// Acquisitions were already sorted by their semantic root tuple.
	for _, acquisition := range d.acquisitions {
		if err := assign(acquisition.state); err != nil {
			return err
		}
	}
	type edge struct {
		operation Operation
		input     InputSource
		outcome   uint32
		to        State
	}
	outgoing := make([][]edge, len(d.states)+1)
	for _, transition := range d.transitions {
		if transition.from == 0 || uint64(transition.from) > uint64(len(d.states)) {
			return errors.New("target: protocol transition state outside scope")
		}
		for _, outcome := range transition.outcomes {
			outgoing[transition.from] = append(outgoing[transition.from], edge{
				operation: transition.operation, input: transition.input, outcome: outcome.outcome, to: outcome.to,
			})
		}
	}
	// d.transitions is ordered by (operation,input,from), and every outcome
	// vector by outcome. Filtering it into each bucket therefore already gives
	// the required (operation,input kind,input ordinal,outcome) edge order.
	for head := 0; head < len(queue); head++ {
		from := queue[head]
		for _, item := range outgoing[from] {
			if err := assign(item.to); err != nil {
				return err
			}
		}
	}
	if len(ordered) != len(d.states) {
		return errors.New("target: protocol has unreachable state")
	}
	states := make([]stateRow, len(d.states))
	for index, old := range ordered {
		states[index] = d.states[uint32(old)-1]
	}
	d.states = states
	for index := range d.acquisitions {
		d.acquisitions[index].state = assigned[d.acquisitions[index].state]
	}
	for index := range d.transitions {
		transition := &d.transitions[index]
		transition.from = assigned[transition.from]
		for outcome := range transition.outcomes {
			transition.outcomes[outcome].to = assigned[transition.outcomes[outcome].to]
		}
	}
	sort.Slice(d.acquisitions, func(i, j int) bool { return compareAcquisition(d.acquisitions[i], d.acquisitions[j]) < 0 })
	sort.Slice(d.transitions, func(i, j int) bool { return compareTransition(d.transitions[i], d.transitions[j]) < 0 })
	return nil
}

func compareAcquisition(a, b acquisitionDraft) int {
	if a.operation != b.operation {
		if a.operation < b.operation {
			return -1
		}
		return 1
	}
	if a.outcome != b.outcome {
		if a.outcome < b.outcome {
			return -1
		}
		return 1
	}
	if a.result != b.result {
		if a.result < b.result {
			return -1
		}
		return 1
	}
	if a.state != b.state {
		if a.state < b.state {
			return -1
		}
		return 1
	}
	return 0
}
func sameAcquisitionTuple(a, b acquisitionDraft) bool {
	return a.operation == b.operation && a.outcome == b.outcome && a.result == b.result
}
func compareProtocol(a, b protocolDraft) int {
	limit := len(a.acquisitions)
	if len(b.acquisitions) < limit {
		limit = len(b.acquisitions)
	}
	for i := 0; i < limit; i++ {
		if order := compareAcquisition(a.acquisitions[i], b.acquisitions[i]); order != 0 {
			return order
		}
	}
	if len(a.acquisitions) < len(b.acquisitions) {
		return -1
	}
	if len(a.acquisitions) > len(b.acquisitions) {
		return 1
	}
	limit = len(a.callbackHolders)
	if len(b.callbackHolders) < limit {
		limit = len(b.callbackHolders)
	}
	for i := 0; i < limit; i++ {
		if order := compareProtocolCallbackHolder(a.callbackHolders[i], b.callbackHolders[i]); order != 0 {
			return order
		}
	}
	if len(a.callbackHolders) < len(b.callbackHolders) {
		return -1
	}
	if len(a.callbackHolders) > len(b.callbackHolders) {
		return 1
	}
	return 0
}

func compareProtocolCallbackHolder(left, right protocolCallbackHolderDraft) int {
	if left.operation < right.operation {
		return -1
	}
	if left.operation > right.operation {
		return 1
	}
	if compared := compareInputSource(left.input, right.input); compared != 0 {
		return compared
	}
	if left.callback < right.callback {
		return -1
	}
	if left.callback > right.callback {
		return 1
	}
	return 0
}
func compareTransitionKey(a, b transitionDraft) int {
	if a.operation != b.operation {
		if a.operation < b.operation {
			return -1
		}
		return 1
	}
	if order := compareInputSource(a.input, b.input); order != 0 {
		return order
	}
	if a.from < b.from {
		return -1
	}
	if a.from > b.from {
		return 1
	}
	return 0
}
func compareTransition(a, b transitionDraft) int { return compareTransitionKey(a, b) }
func compareEscapeKey(a, b escapeDraft) int {
	if a.operation != b.operation {
		if a.operation < b.operation {
			return -1
		}
		return 1
	}
	return compareInputSource(a.input, b.input)
}
func compareEscape(a, b escapeDraft) int {
	return compareEscapeKey(a, b)
}

func (c *Contract) appendProtocols(input []protocolDraft) error {
	if _, err := checkedStoredRange("protocol table", len(c.protocols), len(input)); err != nil {
		return err
	}
	for index := range input {
		draft := &input[index]
		row := protocolRow{}
		var err error
		row.states, err = appendStoredRange(&c.states, draft.states, "protocol state table")
		if err != nil {
			return err
		}
		acquisitions := make([]acquisitionRow, len(draft.acquisitions))
		for i, item := range draft.acquisitions {
			acquisitions[i] = acquisitionRow{operation: item.operation, outcome: item.outcome, result: item.result, state: item.state}
		}
		row.acquisitions, err = appendStoredRange(&c.acquisitions, acquisitions, "protocol acquisition table")
		if err != nil {
			return err
		}
		row.transitions, err = c.appendProtocolTransitions(draft.transitions)
		if err != nil {
			return err
		}
		escapes := make([]escapeRow, len(draft.escapes))
		for itemIndex, item := range draft.escapes {
			escapes[itemIndex] = escapeRow{operation: item.operation, input: item.input}
		}
		row.escapes, err = appendStoredRange(&c.escapes, escapes, "protocol escape table")
		if err != nil {
			return err
		}
		holders := make([]protocolCallbackHolderRow, len(draft.callbackHolders))
		for itemIndex, item := range draft.callbackHolders {
			if item.operation == 0 || item.callback == 0 {
				return errors.New("target: unresolved protocol callback holder")
			}
			holders[itemIndex] = protocolCallbackHolderRow{operation: item.operation, input: item.input, callback: item.callback}
		}
		row.callbackHolders, err = appendStoredRange(&c.callbackHolders, holders, "protocol callback-holder table")
		if err != nil {
			return err
		}
		c.protocols = append(c.protocols, row)
	}
	return nil
}

func (c *Contract) appendProtocolTransitions(input []transitionDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("protocol transition table", len(c.transitions), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, item := range input {
		outcomes := make([]transitionOutcomeRow, len(item.outcomes))
		for i, outcome := range item.outcomes {
			outcomes[i] = transitionOutcomeRow{outcome: outcome.outcome, to: outcome.to}
		}
		rangeItems, appendErr := appendStoredRange(&c.transitionOutcomes, outcomes, "protocol transition outcome table")
		if appendErr != nil {
			return indexRange{}, appendErr
		}
		c.transitions = append(c.transitions, transitionRow{operation: item.operation, input: item.input, from: item.from, outcomes: rangeItems})
	}
	return rangeOut, nil
}
