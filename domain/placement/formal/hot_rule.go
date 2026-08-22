package formal

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type actualTag uint64

// formalObservationInlineWidth bounds the fixed footprint of the two
// selector-stage consumers below. Ordinary mounted calls have a small formal
// actual prefix; eight cells cover that common case while keeping the hot
// frame bounded. Wider calls use explicit per-invocation overflow storage;
// HotRule retains no state and engine stages share no mutable scratch.
const formalObservationInlineWidth = 8

// canonicalActualTag is the one-based authored actual ordinal carried by the
// staged Value selection.  Selection rows are sorted by the engine's physical
// Unit order, so the tag is the semantic order that lets the checker recover
// the exact mounted actual.  Keep the conversion guarded: a malformed route
// tag must fail closed before it is narrowed to an int index.
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

// HotRule is the receipt-native Target FormalEffects consumer. The hot path
// retains only owner-fenced schemas, Call's mounted invocation receipts,
// Pack's direct mounted-actual projection authority, and the Target
// contract's already-sealed formal-row authority.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	contract       *contract.Contract
	packs          *packdomain.Schema
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	actualRead     engine.Read[engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead  engine.Read[engine.Selection[routeTag, engine.OrderedCells[placement.Placement]]]
}

// BindHot binds one formal ownership Rule through the exact Placement owner.
// Calls and Values are read through their own owner forms; no raw Call,
// Program, Target, Pack, Effect, or Publication object enters the callback.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	owner *placementowner.HotOwner,
	values *valueowner.HotOwner,
	calls *callowner.HotOwner,
	targetContract *contract.Contract,
	packSchema *packdomain.Schema,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) ||
		targetContract == nil || !owner.Schema().Valid() || values.Schema() == nil ||
		!values.Schema().Valid() || calls.Algebra() == nil || !calls.Algebra().Valid() ||
		!values.Schema().LinkOwner().Matches(calls.Algebra().LinkOwner()) ||
		!calls.OwnsTargetContract(targetContract) ||
		packSchema == nil || !packSchema.LinkOwner().Available() || !packSchema.LinkOwner().Matches(calls.Algebra().LinkOwner()) ||
		!values.Schema().OwnsHeapSchema(owner.Schema().Heap()) || !fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{
		owner: owner, values: values, calls: calls, contract: targetContract,
		packs: packSchema,
	}
	implementation, ok := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placement.Placement, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return operandContent(rule.packs, calls.Algebra(), candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[placement.Placement, operand]{}, nil)
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	callRead, callOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(candidate operand) (uint64, bool) {
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
	actualRead, actualOK := placementowner.AddSelectedRuleDirectOperandRead[operand, valuedomain.Value, actualTag](implementation, fragment.actualRead, values.FactorRef(), rule.locateActual)
	if !actualOK {
		return nil, false
	}
	rule.actualRead = actualRead
	placementRead, placementOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placement.Placement, routeTag](implementation, fragment.placementRead, owner.FactorRef(), rule.locatePlacement)
	if !placementOK {
		return nil, false
	}
	rule.placementRead = placementRead
	return rule, true
}

// Implementation returns the pending Placement-owned Rule issuer.
func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil || rule.owner == nil {
		return nil, false
	}
	_, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil {
		return operand{}, false
	}
	candidate, ok := operandForOccurrence(rule.packs, rule.calls.Algebra(), coords.Mount, coords.Occurrence)
	if !ok {
		return operand{}, false
	}
	return candidate, true
}

func (rule *HotRule) actual(candidate operand) (packdomain.MountedActualProjection, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil {
		return packdomain.MountedActualProjection{}, false
	}
	actual, _, _, _, ok := mountedForOperand(rule.packs, rule.calls.Algebra(), candidate)
	return actual, ok
}

func (rule *HotRule) coordinateForActual(actual packdomain.MountedActualProjection, index int) (valuedomain.Coordinate, bool) {
	if rule == nil || rule.values == nil || index < 0 || index >= actual.ActualCount() {
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
		selected := false
		if coordinateOK {
			selected = valueowner.SelectRouteTyped(rule.values, context, coordinate, tag)
		}
		if !tagOK || !coordinateOK || !selected {
			return false
		}
	}
	return true
}

func (rule *HotRule) locatePlacement(context engine.SelectorContext, candidate operand) bool {
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
	actualSelection, actualOK := engine.SelectorRead(context, rule.actualRead)
	if !actualOK {
		return false
	}
	var inline [formalObservationInlineWidth]actualObservation
	observations, bufferOK := formalObservationBuffer(actual.ActualCount(), inline[:])
	if !bufferOK || !rule.selectorObservations(context, actual, actualSelection, observations) {
		return false
	}
	plan, planOK := planFor(rule.packs, rule.calls.Algebra(), rule.owner.Schema(), rule.values.Schema(), rule.contract, candidate.mounted, callFact, observations)
	if !planOK {
		return false
	}
	if plan.allUnknown {
		selected := 0
		for dense := 0; dense < plan.schema.DenseKeyCount(); dense++ {
			key, keyOK := plan.schema.KeyAt(dense)
			if !keyOK {
				return false
			}
			if key.Kind() != heap.RootAllocation {
				continue
			}
			tag, tagOK := routeTagForDense(plan.schema, key, dense, placement.None, true)
			if !tagOK || !placementowner.SelectRouteTyped(rule.owner, context, key, tag) {
				return false
			}
			selected++
		}
		return selected == plan.routeCount()
	}
	for index := 0; index < plan.routeCount(); index++ {
		route, routeOK := plan.routeAt(index)
		if !routeOK || !placementowner.SelectRouteTyped(rule.owner, context, route.key, route.tag) {
			return false
		}
	}
	return true
}

// formalObservationBuffer returns caller-owned storage for one invocation.
// The returned slice is never retained by the rule or the engine; callers keep
// it live only through their immediate plan reduction.
func formalObservationBuffer(count int, inline []actualObservation) ([]actualObservation, bool) {
	if count < 0 {
		return nil, false
	}
	if count <= cap(inline) {
		return inline[:count], true
	}
	return make([]actualObservation, count), true
}

func (rule *HotRule) selectorObservations(context engine.SelectorContext, actual packdomain.MountedActualProjection, selection engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]], observations []actualObservation) bool {
	if rule == nil || rule.values == nil {
		return false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK || count != actual.ActualCount() || len(observations) != count {
		return false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		tag, cells, selected := engine.SelectorSelectionAt(context, selection, ordinal)
		expectedTag, expectedTagOK := canonicalActualTag(ordinal)
		// The engine preserves the tag as semantic evidence while it
		// canonicalizes physical Unit order. Require the exact authored ordinal
		// here so reordered, duplicate, or out-of-range tags cannot be accepted
		// by mapping them back through an unchecked integer conversion.
		if !expectedTagOK || tag != expectedTag || !selected || cells.Count() != 1 {
			return false
		}
		index := ordinal
		fact, present, available := cells.At(0)
		coordinate, coordinateOK := rule.coordinateForActual(actual, index)
		if !available || !coordinateOK || present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
			return false
		}
		observations[index] = actualObservation{fact: fact, present: present, valid: true}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placement.Placement, operand]) engine.RuleResult[placement.Placement] {
	candidate, candidateOK := engine.Operand(frame)
	if !candidateOK || rule == nil || rule.owner == nil || rule.values == nil || rule.calls == nil {
		return engine.RuleResult[placement.Placement]{}
	}
	actual, actualOK := rule.actual(candidate)
	if !actualOK {
		return engine.RuleResult[placement.Placement]{}
	}
	callCells, callOK := engine.ReadValue(frame, rule.callRead)
	actualSelection, actualOK := engine.ReadValue(frame, rule.actualRead)
	placementSelection, placementOK := engine.ReadValue(frame, rule.placementRead)
	if !callOK || !actualOK || !placementOK || callCells.Count() != 1 {
		return engine.RuleResult[placement.Placement]{}
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	if !callAvailable {
		return engine.RuleResult[placement.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK {
		return engine.RuleResult[placement.Placement]{}
	}
	if !callPresent {
		if count == 0 {
			return engine.NoSelection(frame, placementSelection)
		}
		return engine.RuleResult[placement.Placement]{}
	}
	var inline [formalObservationInlineWidth]actualObservation
	observations, bufferOK := formalObservationBuffer(actual.ActualCount(), inline[:])
	if !bufferOK || !rule.accessObservations(frame, actual, actualSelection, observations) {
		return engine.RuleResult[placement.Placement]{}
	}
	plan, planOK := planFor(rule.packs, rule.calls.Algebra(), rule.owner.Schema(), rule.values.Schema(), rule.contract, candidate.mounted, callFact, observations)
	if !planOK || count != plan.routeCount() {
		return engine.RuleResult[placement.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag routeTag, cells engine.OrderedCells[placement.Placement]) (placement.Placement, bool) {
		route, routeOK := routeForTag(plan, tag)
		if !routeOK || cells.Count() != 1 {
			return placement.Bottom, false
		}
		current, present, available := cells.At(0)
		current, currentOK := placement.AuthenticateFactorCell(current, present, available)
		if !currentOK {
			return placement.Bottom, false
		}
		if route.unknown {
			return placement.Unknown, true
		}
		return placement.DisplaceChecked(current, route.escape)
	})
}

func (rule *HotRule) accessObservations(frame engine.Frame[placement.Placement, operand], actual packdomain.MountedActualProjection, selection engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]], observations []actualObservation) bool {
	if rule == nil || rule.values == nil {
		return false
	}
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != actual.ActualCount() || len(observations) != count {
		return false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		tag, cells, selected := engine.SelectionAt(frame, selection, ordinal)
		expectedTag, expectedTagOK := canonicalActualTag(ordinal)
		// Keep the frame evaluator's checker identical to the staged locator:
		// every ordinal must carry its canonical one-based actual tag. This
		// rejects duplicate/permuted tags before any observation is admitted.
		if !expectedTagOK || tag != expectedTag || !selected || cells.Count() != 1 {
			return false
		}
		index := ordinal
		fact, present, available := cells.At(0)
		coordinate, coordinateOK := rule.coordinateForActual(actual, index)
		if !available || !coordinateOK || present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
			return false
		}
		observations[index] = actualObservation{fact: fact, present: present, valid: true}
	}
	return true
}
