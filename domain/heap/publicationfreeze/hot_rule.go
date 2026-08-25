package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	effectpublication "github.com/wippyai/go-lua/domain/effect/publication"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// callCoordinateKey is the mounted module/call coordinate a resolved operand
// is looked up by. It mirrors the join OperandCoords already carries.
type callCoordinateKey struct {
	module identity.ContentID
	call   identity.ContentID
}

// HotRule is the Effect publication FreezeSeal consumer. Effect remains the
// authority that issued each published call; Call gates operation
// alternatives, Value supplies exact mounted subjects, and Heap owns the
// freeze transition.
type HotRule struct {
	implementation    *heapowner.RuleImplementation[effectpublication.CallRow]
	owner             *heapowner.HotOwner
	values            *valueowner.HotOwner
	calls             *callowner.HotOwner
	effects           *effectowner.HotOwner
	preparedByID      map[identity.ContentID]*preparedCall
	callsByCoordinate map[callCoordinateKey]effectpublication.CallRow
	callRead          engine.Read[engine.OrderedCells[calldomain.Value]]
	valueRead         engine.Read[engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]]
	heapRead          engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]]
}

// BindHot binds the exact Call/Value predecessors and Heap route write. The
// Effect batch is a typed operand resolved by its mounted provenance, not a
// second Effect Factor read in the declarative DAG.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	owner *heapowner.HotOwner,
	values *valueowner.HotOwner,
	calls *callowner.HotOwner,
	effects *effectowner.HotOwner,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) ||
		values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) ||
		effects == nil || !effects.MatchesBinding(binding) || !owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() ||
		calls.Algebra() == nil || !calls.Algebra().Valid() || effects.Algebra() == nil || !effects.Algebra().Valid() ||
		!values.Schema().OwnsHeapSchema(owner.Schema()) || !fragment.semantic.Available() {
		return nil, false
	}
	linkOwner := calls.Algebra().LinkOwner()
	if !linkOwner.Available() || !values.Schema().LinkOwner().Matches(linkOwner) || !owner.Schema().LinkOwner().Matches(linkOwner) || !effects.Algebra().LinkOwner().Matches(linkOwner) {
		return nil, false
	}
	directory, directoryOK := effectpublication.Detach(effects.Algebra(), values.Schema())
	if !directoryOK {
		return nil, false
	}
	rule := &HotRule{
		owner:             owner,
		values:            values,
		calls:             calls,
		effects:           effects,
		preparedByID:      make(map[identity.ContentID]*preparedCall, len(directory.Calls)),
		callsByCoordinate: make(map[callCoordinateKey]effectpublication.CallRow, len(directory.Calls)),
	}
	for _, call := range directory.Calls {
		if !call.Available() {
			return nil, false
		}
		prepared, preparedOK := prepareCall(values.Schema(), calls.Algebra(), directory, call)
		if !preparedOK {
			return nil, false
		}
		if _, duplicate := rule.preparedByID[prepared.id]; duplicate {
			return nil, false
		}
		rule.preparedByID[prepared.id] = prepared
		coordinate := callCoordinateKey{module: call.Module, call: call.Call}
		if _, duplicate := rule.callsByCoordinate[coordinate]; duplicate {
			return nil, false
		}
		rule.callsByCoordinate[coordinate] = call
	}

	implementation, ok := heapowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[heapdomain.Value, effectpublication.CallRow]{
		OperandContent:  rule.operandContent,
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[heapdomain.Value, effectpublication.CallRow]{}, nil)
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	callRead, callOK := heapowner.AddSelectedRouteRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(call effectpublication.CallRow) (uint64, bool) {
		key, keyOK := rule.callKeyForCall(call)
		if !keyOK {
			return 0, false
		}
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), indexOK && index >= 0
	})
	if !callOK {
		return nil, false
	}
	rule.callRead = callRead
	valueRead, valueOK := heapowner.AddSelectedRouteRuleDirectOperandRead[effectpublication.CallRow, valuedomain.Value, sourceTag](implementation, fragment.valueRead, values.FactorRef(), rule.locateValues)
	if !valueOK {
		return nil, false
	}
	rule.valueRead = valueRead
	heapRead, heapOK := heapowner.AddSelectedRouteRuleDirectOperandRead[effectpublication.CallRow, heapdomain.Value, heapdomain.RawRouteTag](implementation, fragment.heapRead, owner.FactorRef(), rule.locateHeap)
	if !heapOK {
		return nil, false
	}
	rule.heapRead = heapRead
	return rule, true
}

// Implementation returns the pending Heap-owned Rule issuer.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[effectpublication.CallRow], bool) {
	if rule == nil || rule.implementation == nil || rule.owner == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (effectpublication.CallRow, bool) {
	if rule == nil || rule.callsByCoordinate == nil || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return effectpublication.CallRow{}, false
	}
	call, found := rule.callsByCoordinate[callCoordinateKey{module: coords.Mount, call: coords.Occurrence}]
	if !found || rule.preparedFor(call) == nil {
		return effectpublication.CallRow{}, false
	}
	return call, true
}

// preparedFor is the operand fence. A published call is an inert value rather
// than an owner-issued handle, so the fence is equality with the row this rule
// prepared: an operand that is not one of them, or one that carries the
// identity of a prepared call under any other field, resolves to nothing.
func (rule *HotRule) preparedFor(call effectpublication.CallRow) *preparedCall {
	if rule == nil || rule.preparedByID == nil || rule.callsByCoordinate == nil || !call.Available() {
		return nil
	}
	issued, found := rule.callsByCoordinate[callCoordinateKey{module: call.Module, call: call.Call}]
	if !found || issued != call {
		return nil
	}
	prepared := rule.preparedByID[call.ID]
	if prepared == nil || prepared.id != call.ID {
		return nil
	}
	return prepared
}

func (rule *HotRule) operandContent(call effectpublication.CallRow) (effectpublication.CallRow, [32]byte, bool) {
	if rule == nil || rule.preparedFor(call) == nil || !call.ID.Available() {
		return effectpublication.CallRow{}, [32]byte{}, false
	}
	return call, [32]byte(call.ID), true
}

func (rule *HotRule) callKeyForCall(call effectpublication.CallRow) (calldomain.Key, bool) {
	if rule == nil || rule.preparedByID == nil {
		return calldomain.Key{}, false
	}
	prepared := rule.preparedFor(call)
	if prepared == nil || !prepared.callKeyOK || !prepared.callKey.Valid() {
		return calldomain.Key{}, false
	}
	return prepared.callKey, true
}

func (rule *HotRule) callValueSelector(context engine.SelectorContext, call effectpublication.CallRow) (calldomain.Value, bool, bool) {
	if rule == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return calldomain.Value{}, false, false
	}
	cells, readable := engine.SelectorRead(context, rule.callRead)
	if !readable || cells.Count() != 1 {
		return calldomain.Value{}, false, false
	}
	value, present, available := cells.At(0)
	key, keyOK := rule.callKeyForCall(call)
	if !available || !keyOK {
		return calldomain.Value{}, false, false
	}
	if !present {
		bottom := rule.calls.Algebra().Bottom()
		return bottom, false, rule.calls.Algebra().Admits(key, bottom)
	}
	return value, true, rule.calls.Algebra().Admits(key, value)
}

func (rule *HotRule) callValueFrame(frame engine.Frame[heapdomain.Value, effectpublication.CallRow], call effectpublication.CallRow) (calldomain.Value, bool, bool) {
	if rule == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return calldomain.Value{}, false, false
	}
	cells, readable := engine.ReadValue(frame, rule.callRead)
	if !readable || cells.Count() != 1 {
		return calldomain.Value{}, false, false
	}
	value, present, available := cells.At(0)
	key, keyOK := rule.callKeyForCall(call)
	if !available || !keyOK {
		return calldomain.Value{}, false, false
	}
	if !present {
		bottom := rule.calls.Algebra().Bottom()
		return bottom, false, rule.calls.Algebra().Admits(key, bottom)
	}
	return value, true, rule.calls.Algebra().Admits(key, value)
}

func (rule *HotRule) operationGateForCall(batch *preparedCall, value calldomain.Value) (operationGate, bool) {
	if rule == nil || batch == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return operationGate{}, false
	}
	if !batch.callKeyOK || !batch.callKey.Valid() || !rule.calls.Algebra().Admits(batch.callKey, value) {
		return operationGate{}, false
	}
	var gate operationGate
	if value.IsTop() {
		gate.opaque = true
		return gate, true
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		if !targetOK {
			return operationGate{}, false
		}
		operation, operationKind := rule.calls.Algebra().ClassifyTargetOperation(target)
		switch operationKind {
		case calldomain.TargetOperationInvalid:
			return operationGate{}, false
		case calldomain.TargetOperationNone:
			// Freeze is stricter than publication placement: a valid Call
			// alternative without an operation cannot justify strong freeze.
			gate.unsupported = true
		case calldomain.TargetOperationPresent:
			if !gate.add(operation) {
				return operationGate{}, false
			}
		}
	}
	gate.opaque = value.IsOpen()
	return gate, true
}

func (rule *HotRule) locateValues(context engine.SelectorContext, call effectpublication.CallRow) bool {
	value, present, callOK := rule.callValueSelector(context, call)
	if !callOK {
		return false
	}
	prepared := rule.preparedFor(call)
	if prepared == nil || rule.values == nil {
		return false
	}
	if !present {
		return true
	}
	gate, gateOK := rule.operationGateForCall(prepared, value)
	if !gateOK {
		return false
	}
	sources := prepared.sourcesForGate(gate)
	for index := 0; index < sources.len(); index++ {
		source, sourceOK := sources.at(index)
		if !sourceOK || !valueowner.SelectRouteTyped(rule.values, context, source.coordinate, source.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) locateHeap(context engine.SelectorContext, call effectpublication.CallRow) bool {
	callValue, present, callOK := rule.callValueSelector(context, call)
	if !callOK || rule.owner == nil || rule.values == nil || rule.values.Schema() == nil {
		return false
	}
	if !present {
		return true
	}
	prepared := rule.preparedFor(call)
	if prepared == nil {
		return false
	}
	gate, gateOK := rule.operationGateForCall(prepared, callValue)
	if !gateOK {
		return false
	}
	selection, selectionOK := engine.SelectorRead(context, rule.valueRead)
	if !selectionOK {
		return false
	}
	sources := prepared.sourcesForGate(gate)
	facts, factsOK := rule.collectFacts(context, sources, selection)
	if !factsOK {
		return false
	}
	plan, planOK := planFor(rule.owner.Schema(), rule.values.Schema(), prepared, gate, facts)
	if !planOK {
		return false
	}
	for index := 0; index < plan.Count(); index++ {
		candidate, candidateOK := plan.At(index)
		if !candidateOK || !heapowner.SelectRouteTyped(rule.owner, context, candidate.Key, candidate.Tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) collectFacts(context engine.SelectorContext, sources sourceBuffer, selection engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]) (factBuffer, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return factBuffer{}, false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK || count != sources.len() {
		return factBuffer{}, false
	}
	var facts factBuffer
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.SelectorSelectionAt(context, selection, index)
		source, sourceOK := sources.find(tag)
		if !selected || !sourceOK || cells.Count() != 1 {
			return factBuffer{}, false
		}
		fact, valuePresent, available := cells.At(0)
		if !available || valuePresent && !rule.values.Schema().AdmitsCoordinate(source.coordinate, fact) {
			return factBuffer{}, false
		}
		if !facts.merge(rule.values.Schema(), factEntry{rowID: source.rowID, value: fact, present: valuePresent}) {
			return factBuffer{}, false
		}
	}
	return facts, true
}

func (rule *HotRule) collectFrameFacts(frame engine.Frame[heapdomain.Value, effectpublication.CallRow], sources sourceBuffer, selection engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]) (factBuffer, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return factBuffer{}, false
	}
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != sources.len() {
		return factBuffer{}, false
	}
	var facts factBuffer
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.SelectionAt(frame, selection, index)
		source, sourceOK := sources.find(tag)
		if !selected || !sourceOK || cells.Count() != 1 {
			return factBuffer{}, false
		}
		fact, valuePresent, available := cells.At(0)
		if !available || valuePresent && !rule.values.Schema().AdmitsCoordinate(source.coordinate, fact) {
			return factBuffer{}, false
		}
		if !facts.merge(rule.values.Schema(), factEntry{rowID: source.rowID, value: fact, present: valuePresent}) {
			return factBuffer{}, false
		}
	}
	return facts, true
}

func routeForTag(plan routePlan, tag heapdomain.RawRouteTag) (route, bool) {
	return recentplan.RouteForTag(plan, tag)
}

func (rule *HotRule) fold(frame engine.Frame[heapdomain.Value, effectpublication.CallRow]) engine.RuleResult[heapdomain.Value] {
	call, callRowOK := engine.Operand(frame)
	if !callRowOK || rule == nil || rule.owner == nil || rule.values == nil || rule.calls == nil {
		return engine.RuleResult[heapdomain.Value]{}
	}
	heapSelection, heapOK := engine.ReadValue(frame, rule.heapRead)
	callValue, callPresent, callOK := rule.callValueFrame(frame, call)
	if !heapOK || !callOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	count, countOK := engine.SelectionCount(frame, heapSelection)
	if !countOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	if !callPresent {
		if count != 0 {
			return engine.RuleResult[heapdomain.Value]{}
		}
		return engine.NoSelection(frame, heapSelection)
	}
	prepared := rule.preparedFor(call)
	if prepared == nil {
		return engine.RuleResult[heapdomain.Value]{}
	}
	gate, gateOK := rule.operationGateForCall(prepared, callValue)
	if !gateOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	valueSelection, valueOK := engine.ReadValue(frame, rule.valueRead)
	if !valueOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	sources := prepared.sourcesForGate(gate)
	facts, factsOK := rule.collectFrameFacts(frame, sources, valueSelection)
	if !factsOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	plan, planOK := planFor(rule.owner.Schema(), rule.values.Schema(), prepared, gate, facts)
	if !planOK || count != plan.Count() {
		return engine.RuleResult[heapdomain.Value]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, heapSelection)
	}
	schema := rule.owner.Schema()
	return engine.Routed(frame, heapSelection, func(tag heapdomain.RawRouteTag, cells engine.OrderedCells[heapdomain.Value]) (heapdomain.Value, bool) {
		candidate, candidateOK := routeForTag(plan, tag)
		if !candidateOK || cells.Count() != 1 {
			return heapdomain.Value{}, false
		}
		predecessor, present, available := cells.At(0)
		if !available {
			return heapdomain.Value{}, false
		}
		if !present {
			return schema.Bottom(), true
		}
		reference, referenceOK := schema.Reference(candidate.Key, materialization.Recent)
		if !referenceOK {
			return heapdomain.Value{}, false
		}
		branches, freezeOK := schema.ShallowFreeze(predecessor, reference)
		if !freezeOK {
			return heapdomain.Value{}, false
		}
		next, normalOK := branches.Normal(candidate.Key)
		if !normalOK {
			return schema.Bottom(), true
		}
		return next, true
	})
}
