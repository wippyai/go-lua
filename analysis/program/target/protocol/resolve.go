package protocol

import (
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func resolveProtocols(drafts []protocolDraft, operations operation.Core) error {
	for index := range drafts {
		if err := drafts[index].resolve(operations); err != nil {
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
	operation       vocabulary.Operation
	outcome, result uint32
}

func (d *protocolDraft) resolve(operations operation.Core) error {
	resolveOperation := func(ref vocabulary.SpecRef) (vocabulary.Operation, error) {
		if ref == 0 || uint64(ref) > uint64(operations.SourceCount()) {
			return 0, errors.New("target: protocol references unknown operation")
		}
		op, ok := operations.SourceOperation(int(ref) - 1)
		if !ok {
			return 0, errors.New("target: protocol references unresolved operation")
		}
		return op, nil
	}
	for index := range d.acquisitions {
		row := &d.acquisitions[index]
		op, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if uint64(row.outcomeSource) >= uint64(operations.OutcomeCount(op)) {
			return fmt.Errorf("target: acquisition %d outcome outside scope", index)
		}
		outcome := row.outcomeSource
		slots, slotsOK := operations.OutcomeValueSlots(op, int(outcome))
		if !slotsOK || row.result >= slots {
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
		op, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if !validAuthoredInputSource(row.input, operations.InputFormalCount(op), uint32(operations.ValuesVarCount(op))) {
			return fmt.Errorf("target: transition %d has source outside scope", index)
		}
		row.operation = op
		for outcomeIndex := range row.outcomes {
			if uint64(row.outcomes[outcomeIndex].outcomeSource) >= uint64(operations.OutcomeCount(op)) {
				return fmt.Errorf("target: transition %d outcome %d outside scope", index, outcomeIndex)
			}
			outcome := row.outcomes[outcomeIndex].outcomeSource
			row.outcomes[outcomeIndex].outcome = outcome
		}
		sort.Slice(row.outcomes, func(i, j int) bool { return row.outcomes[i].outcome < row.outcomes[j].outcome })
		for outcomeIndex := 1; outcomeIndex < len(row.outcomes); outcomeIndex++ {
			if row.outcomes[outcomeIndex-1].outcome == row.outcomes[outcomeIndex].outcome {
				return fmt.Errorf("target: transition %d duplicates outcome", index)
			}
		}
	}
	sort.Slice(d.transitions, func(i, j int) bool { return compareTransitionKey(d.transitions[i], d.transitions[j]) < 0 })
	for index := 1; index < len(d.transitions); index++ {
		if compareTransitionKey(d.transitions[index-1], d.transitions[index]) == 0 {
			return errors.New("target: duplicate protocol transition")
		}
	}
	for index := range d.requirements {
		row := &d.requirements[index]
		op, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if !validAuthoredInputSource(row.input, operations.InputFormalCount(op), uint32(operations.ValuesVarCount(op))) {
			return fmt.Errorf("target: requirement %d has source outside scope", index)
		}
		row.operation = op
	}
	sort.Slice(d.requirements, func(i, j int) bool { return compareRequirementKey(d.requirements[i], d.requirements[j]) < 0 })
	for index := 1; index < len(d.requirements); index++ {
		if compareRequirementKey(d.requirements[index-1], d.requirements[index]) == 0 {
			return errors.New("target: duplicate protocol requirement")
		}
	}
	for index := range d.escapes {
		row := &d.escapes[index]
		op, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if !validAuthoredInputSource(row.input, operations.InputFormalCount(op), uint32(operations.ValuesVarCount(op))) {
			return fmt.Errorf("target: escape %d has source outside scope", index)
		}
		row.operation = op
	}
	sort.Slice(d.escapes, func(i, j int) bool { return compareEscapeKey(d.escapes[i], d.escapes[j]) < 0 })
	for index := 1; index < len(d.escapes); index++ {
		if compareEscapeKey(d.escapes[index-1], d.escapes[index]) == 0 {
			return errors.New("target: duplicate protocol escape")
		}
	}
	for index := range d.callbackHolders {
		row := &d.callbackHolders[index]
		op, err := resolveOperation(row.operationSource)
		if err != nil {
			return err
		}
		if operations.BindingCount(op) == 0 {
			return fmt.Errorf("target: callback holder %d operation is not source-visible", index)
		}
		if !validAuthoredInputSource(row.input, operations.InputFormalCount(op), uint32(operations.ValuesVarCount(op))) {
			return fmt.Errorf("target: callback holder %d has source outside scope", index)
		}
		row.operation = op
	}
	return d.canonicalizeStates()
}

// resolveProtocolCallbackHolders runs after appendOperation has assigned the
// canonical CallbackID range. Protocol shape can be resolved earlier, but a
// CallbackRef intentionally has no globally meaningful identity until then.
func resolveProtocolCallbackHolders(protocols []protocolDraft, operations operation.Core) error {
	for protocolIndex := range protocols {
		draft := &protocols[protocolIndex]
		for holderIndex := range draft.callbackHolders {
			holder := &draft.callbackHolders[holderIndex]
			if holder.operation == 0 || uint64(holder.operation) > uint64(operations.SourceCount()) || holder.callbackSource == 0 {
				return fmt.Errorf("target: protocol %d callback holder %d unresolved", protocolIndex, holderIndex)
			}
			callbackIndex := uint64(holder.callbackSource - 1)
			if callbackIndex >= uint64(operations.CallbackCount(holder.operation)) {
				return fmt.Errorf("target: protocol %d callback holder %d callback outside operation scope", protocolIndex, holderIndex)
			}
			callback, callbackOK := operations.CallbackAt(holder.operation, int(callbackIndex))
			if !callbackOK || callback == 0 {
				return fmt.Errorf("target: protocol %d callback holder %d callback outside operation scope", protocolIndex, holderIndex)
			}
			lifecycle, lifecycleOK := operations.CallbackLifecycle(callback)
			if !lifecycleOK || !retainedCallbackLifecycle(lifecycle) {
				return fmt.Errorf("target: protocol %d callback holder %d callback is not retained", protocolIndex, holderIndex)
			}
			holder.callback = callback
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
	assigned := make([]vocabulary.State, len(d.states)+1)
	ordered := make([]vocabulary.State, 0, len(d.states))
	queue := make([]vocabulary.State, 0, len(d.states))
	assign := func(state vocabulary.State) error {
		if state == 0 || uint64(state) > uint64(len(d.states)) {
			return errors.New("target: protocol state outside scope")
		}
		if assigned[state] != 0 {
			return nil
		}
		assigned[state] = vocabulary.State(len(ordered) + 1)
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
	// Keep only transition indexes in the temporary adjacency buckets. The
	// transition and outcome rows themselves are already canonical and remain
	// the single source of operation/input/outcome ordering and destinations.
	outgoing := make([][]int, len(d.states)+1)
	for transitionIndex := range d.transitions {
		transition := &d.transitions[transitionIndex]
		if transition.from == 0 || uint64(transition.from) > uint64(len(d.states)) {
			return errors.New("target: protocol transition state outside scope")
		}
		outgoing[transition.from] = append(outgoing[transition.from], transitionIndex)
	}
	// d.transitions is ordered by (operation,input,from), and every outcome
	// vector by outcome. Filtering it into each bucket therefore already gives
	// the required (operation,input kind,input ordinal,outcome) edge order.
	for head := 0; head < len(queue); head++ {
		from := queue[head]
		for _, transitionIndex := range outgoing[from] {
			for _, outcome := range d.transitions[transitionIndex].outcomes {
				if err := assign(outcome.to); err != nil {
					return err
				}
			}
		}
	}
	if len(ordered) != len(d.states) {
		return errors.New("target: protocol has unreachable state")
	}
	// Reorder the existing state rows in place. `ordered` is the permutation
	// from canonical position to the old local coordinate; consume it as the
	// cycles are applied so no second state-row slice is retained.
	for index := range d.states {
		if ordered[index] == vocabulary.State(index+1) {
			continue
		}
		temporary := d.states[index]
		position := index
		for {
			source := int(ordered[position]) - 1
			ordered[position] = vocabulary.State(position + 1)
			if source == index {
				d.states[position] = temporary
				break
			}
			d.states[position] = d.states[source]
			position = source
		}
	}
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
	// A requirement observes a state; it introduces no edge, so it takes no
	// part in the traversal above and only follows the coordinates the roots
	// and edges already fixed.
	for index := range d.requirements {
		d.requirements[index].state = assigned[d.requirements[index].state]
	}
	sort.Slice(d.acquisitions, func(i, j int) bool { return compareAcquisition(d.acquisitions[i], d.acquisitions[j]) < 0 })
	sort.Slice(d.transitions, func(i, j int) bool { return compareTransitionKey(d.transitions[i], d.transitions[j]) < 0 })
	sort.Slice(d.requirements, func(i, j int) bool { return compareRequirementKey(d.requirements[i], d.requirements[j]) < 0 })
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

func compareRequirementKey(a, b requirementDraft) int {
	if a.operation != b.operation {
		if a.operation < b.operation {
			return -1
		}
		return 1
	}
	if order := compareInputSource(a.input, b.input); order != 0 {
		return order
	}
	if a.state < b.state {
		return -1
	}
	if a.state > b.state {
		return 1
	}
	return 0
}

func compareEscapeKey(a, b escapeDraft) int {
	if a.operation != b.operation {
		if a.operation < b.operation {
			return -1
		}
		return 1
	}
	return compareInputSource(a.input, b.input)
}

func validAuthoredInputSource(source vocabulary.InputSource, valueFormals int, valuesVars uint32) bool {
	switch source.Kind {
	case vocabulary.InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(valueFormals)
	case vocabulary.InputSourceValuesVar:
		return uint64(source.Ordinal) < uint64(valuesVars)
	default:
		return false
	}
}

func retainedCallbackLifecycle(lifecycle vocabulary.CallbackLifecycle) bool {
	return lifecycle >= vocabulary.CallbackRetainedOptionalOnce && lifecycle <= vocabulary.CallbackRetainedRequiredMany
}

func compareInputSource(left, right vocabulary.InputSource) int {
	if left.Kind < right.Kind {
		return -1
	}
	if left.Kind > right.Kind {
		return 1
	}
	if left.Ordinal < right.Ordinal {
		return -1
	}
	if left.Ordinal > right.Ordinal {
		return 1
	}
	return 0
}
