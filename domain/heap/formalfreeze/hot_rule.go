package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type actualTag uint64

func canonicalActualTag(index int) (actualTag, bool) {
	if index < 0 {
		return 0, false
	}
	tag := uint64(index) + 1
	if tag == 0 {
		return 0, false
	}
	return actualTag(tag), true
}

// HotRule is the receipt-native Target FormalEffectFreeze consumer. It
// retains only owner-fenced schemas, Call's mounted invocation receipts,
// Pack's direct mounted-actual projection authority, and the already-sealed
// Target contract.
type HotRule struct {
	implementation *heapowner.RuleImplementation[operand]
	owner          *heapowner.HotOwner
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	contract       *contract.Contract
	packs          *packdomain.Schema
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	actualRead     engine.Read[engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]]]
	heapRead       engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]]
}

// BindHot binds one formal-freeze Rule through Heap's exact owner. Call and
// Value are read through their own owner forms; no raw Call, Program, Target,
// Pack, Effect, or Placement object enters the callback.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	owner *heapowner.HotOwner,
	values *valueowner.HotOwner,
	calls *callowner.HotOwner,
	targetContract *contract.Contract,
	packSchema *packdomain.Schema,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) ||
		values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) ||
		targetContract == nil || !owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() ||
		calls.Algebra() == nil || !calls.Algebra().Valid() || !values.Schema().LinkOwner().Matches(calls.Algebra().LinkOwner()) ||
		!calls.OwnsTargetContract(targetContract) || packSchema == nil || !packSchema.LinkOwner().Available() ||
		!packSchema.LinkOwner().Matches(calls.Algebra().LinkOwner()) || !values.Schema().OwnsHeapSchema(owner.Schema()) ||
		!fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{owner: owner, values: values, calls: calls, contract: targetContract, packs: packSchema}
	implementation, ok := heapowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[heapdomain.Value, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return operandContent(rule.packs, calls.Algebra(), candidate)
		},
		Fold: rule.fold,
	}, engine.HotCarrySpec[heapdomain.Value, operand]{}, nil)
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	callRead, callOK := heapowner.AddSelectedRouteRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(candidate operand) (uint64, bool) {
		_, _, _, _, rowOK := mountedForOperand(rule.packs, calls.Algebra(), candidate)
		if !rowOK {
			return 0, false
		}
		index, indexOK := calls.Algebra().KeyIndex(candidate.key)
		return uint64(index), indexOK && index >= 0
	})
	if !callOK {
		return nil, false
	}
	rule.callRead = callRead
	actualRead, actualOK := heapowner.AddSelectedRouteRuleDirectOperandRead[operand, valuedomain.Value, actualTag](implementation, fragment.actualRead, values.FactorRef(), rule.locateActual)
	if !actualOK {
		return nil, false
	}
	rule.actualRead = actualRead
	heapRead, heapOK := heapowner.AddSelectedRouteRuleDirectOperandRead[operand, heapdomain.Value, heapdomain.RawRouteTag](implementation, fragment.heapRead, owner.FactorRef(), rule.locateHeap)
	if !heapOK {
		return nil, false
	}
	rule.heapRead = heapRead
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

// Implementation returns the pending Heap-owned Rule issuer.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil || rule.owner == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule publishes the exact engine Rule only after the shared
// SchemaBinding has sealed.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil {
		return operand{}, false
	}
	return operandForOccurrence(rule.packs, rule.calls.Algebra(), coords.Mount, coords.Occurrence)
}

func (rule *HotRule) actual(candidate operand) (packdomain.MountedActualProjection, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil {
		return packdomain.MountedActualProjection{}, false
	}
	actual, _, _, _, ok := mountedForOperand(rule.packs, rule.calls.Algebra(), candidate)
	return actual, ok
}

func (rule *HotRule) coordinateForActual(actual packdomain.MountedActualProjection, index int) (valuedomain.Coordinate, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || index < 0 || index >= actual.ActualCount() {
		return valuedomain.Coordinate{}, false
	}
	source, sourceOK := actual.ActualAt(index)
	if !sourceOK {
		return valuedomain.Coordinate{}, false
	}
	return rule.values.Schema().CoordinateForMountedSemantic(source.Module(), source.ID())
}

func (rule *HotRule) locateActual(context engine.SelectorContext, candidate operand) bool {
	actual, ok := rule.actual(candidate)
	if !ok || rule.values == nil {
		return false
	}
	for index := 0; index < actual.ActualCount(); index++ {
		tag, tagOK := canonicalActualTag(index)
		coordinate, coordinateOK := rule.coordinateForActual(actual, index)
		if !tagOK || !coordinateOK || !valueowner.SelectRouteTyped(rule.values, context, coordinate, tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) locateHeap(context engine.SelectorContext, candidate operand) bool {
	actual, actualOK := rule.actual(candidate)
	if !actualOK || rule.owner == nil || rule.values == nil || rule.calls == nil {
		return false
	}
	callCells, callOK := engine.SelectorRead(context, rule.callRead)
	if !callOK || callCells.Count() != 1 {
		return false
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	if !callAvailable {
		return false
	}
	if !callPresent {
		return true
	}
	actualSelection, selectionOK := engine.SelectorRead(context, rule.actualRead)
	if !selectionOK {
		return false
	}
	var inline [formalFreezeInlineWidth]actualObservation
	observations, observationsOK := formalFreezeObservationBuffer(actual.ActualCount(), inline[:])
	if !observationsOK || !rule.selectorObservations(context, actual, actualSelection, observations) {
		return false
	}
	plan, planOK := planFor(rule.packs, rule.calls.Algebra(), rule.owner.Schema(), rule.values.Schema(), rule.contract, candidate.mounted, callFact, observations)
	if !planOK {
		return false
	}
	for index := 0; index < plan.Count(); index++ {
		route, routeOK := plan.At(index)
		if !routeOK || !heapowner.SelectRouteTyped(rule.owner, context, route.Key, route.Tag) {
			return false
		}
	}
	return true
}

func formalFreezeObservationBuffer(count int, inline []actualObservation) ([]actualObservation, bool) {
	if count < 0 {
		return nil, false
	}
	if count <= cap(inline) {
		return inline[:count], true
	}
	return make([]actualObservation, count), true
}

func (rule *HotRule) selectorObservations(context engine.SelectorContext, actual packdomain.MountedActualProjection, selection engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]], observations []actualObservation) bool {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK || count != actual.ActualCount() || len(observations) != count {
		return false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		tag, cells, selected := engine.SelectorSelectionAt(context, selection, ordinal)
		expectedTag, expectedTagOK := canonicalActualTag(ordinal)
		if !expectedTagOK || tag != expectedTag || !selected || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		coordinate, coordinateOK := rule.coordinateForActual(actual, ordinal)
		if !available || !coordinateOK || present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
			return false
		}
		observations[ordinal] = actualObservation{fact: fact, present: present, valid: true}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[heapdomain.Value, operand]) engine.RuleResult[heapdomain.Value] {
	candidate, candidateOK := engine.Operand(frame)
	if !candidateOK || rule == nil || rule.owner == nil || rule.values == nil || rule.calls == nil {
		return engine.RuleResult[heapdomain.Value]{}
	}
	actual, actualOK := rule.actual(candidate)
	if !actualOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	callCells, callOK := engine.ReadValue(frame, rule.callRead)
	actualSelection, actualOK := engine.ReadValue(frame, rule.actualRead)
	heapSelection, heapOK := engine.ReadValue(frame, rule.heapRead)
	if !callOK || !actualOK || !heapOK || callCells.Count() != 1 {
		return engine.RuleResult[heapdomain.Value]{}
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	if !callAvailable {
		return engine.RuleResult[heapdomain.Value]{}
	}
	count, countOK := engine.SelectionCount(frame, heapSelection)
	if !countOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	if !callPresent {
		if count == 0 {
			return engine.NoSelection(frame, heapSelection)
		}
		return engine.RuleResult[heapdomain.Value]{}
	}
	var inline [formalFreezeInlineWidth]actualObservation
	observations, observationsOK := formalFreezeObservationBuffer(actual.ActualCount(), inline[:])
	if !observationsOK || !rule.accessObservations(frame, actual, actualSelection, observations) {
		return engine.RuleResult[heapdomain.Value]{}
	}
	plan, planOK := planFor(rule.packs, rule.calls.Algebra(), rule.owner.Schema(), rule.values.Schema(), rule.contract, candidate.mounted, callFact, observations)
	if !planOK || count != plan.Count() {
		return engine.RuleResult[heapdomain.Value]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, heapSelection)
	}
	schema := rule.owner.Schema()
	return engine.Routed(frame, heapSelection, func(tag heapdomain.RawRouteTag, cells engine.OrderedCells[heapdomain.Value]) (heapdomain.Value, bool) {
		route, routeOK := routeForTag(plan, tag)
		if !routeOK || cells.Count() != 1 {
			return heapdomain.Value{}, false
		}
		predecessor, present, available := cells.At(0)
		if !available {
			return heapdomain.Value{}, false
		}
		if !present {
			// A selected route with no predecessor has no Normal branch. The
			// routed output must still settle one exact Heap target, so Bottom is
			// the empty normal image rather than a fabricated frozen object.
			return schema.Bottom(), true
		}
		reference, referenceOK := schema.Reference(route.Key, materialization.Recent)
		if !referenceOK {
			return heapdomain.Value{}, false
		}
		branches, freezeOK := schema.ShallowFreeze(predecessor, reference)
		if !freezeOK {
			return heapdomain.Value{}, false
		}
		next, normalOK := branches.Normal(route.Key)
		if !normalOK {
			return schema.Bottom(), true
		}
		return next, true
	})
}

func (rule *HotRule) accessObservations(frame engine.Frame[heapdomain.Value, operand], actual packdomain.MountedActualProjection, selection engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]], observations []actualObservation) bool {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return false
	}
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != actual.ActualCount() || len(observations) != count {
		return false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		tag, cells, selected := engine.SelectionAt(frame, selection, ordinal)
		expectedTag, expectedTagOK := canonicalActualTag(ordinal)
		if !expectedTagOK || tag != expectedTag || !selected || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		coordinate, coordinateOK := rule.coordinateForActual(actual, ordinal)
		if !available || !coordinateOK || present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
			return false
		}
		observations[ordinal] = actualObservation{fact: fact, present: present, valid: true}
	}
	return true
}
