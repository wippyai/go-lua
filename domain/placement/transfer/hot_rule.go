package transfer

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the mounted, invocation-local Target-transfer consumer.  It
// retains only immutable Link authorities and engine read receipts; every
// actual observation and route plan belongs to the current invocation.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	contract       *contract.Contract
	packs          *packdomain.Schema
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	actualRead     engine.Read[engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead  engine.Read[engine.Selection[routeTag, engine.OrderedCells[placement.Fact]]]
}

// BindHot binds one exact Target-transfer Rule to the Link's Value, Call,
// Pack, Target, and Placement authorities.  Pack actuals and Target transfer
// rows are consumed by the hot callbacks; no Effect authority is accepted.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	owner *placementowner.HotOwner,
	values *valueowner.HotOwner,
	calls *callowner.HotOwner,
	targetContract *contract.Contract,
	packSchema *packdomain.Schema,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) ||
		values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) ||
		targetContract == nil || !owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() ||
		calls.Algebra() == nil || !calls.Algebra().Valid() || !calls.OwnsTargetContract(targetContract) ||
		packSchema == nil || !packSchema.LinkOwner().Available() || !packSchema.LinkOwner().Matches(calls.Algebra().LinkOwner()) ||
		!values.Schema().LinkOwner().Matches(calls.Algebra().LinkOwner()) || !values.Schema().OwnsHeapSchema(owner.Schema().Heap()) ||
		!fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{owner: owner, values: values, calls: calls, contract: targetContract, packs: packSchema}
	implementation, ok := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placement.Fact, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return operandContent(rule.packs, calls.Algebra(), candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[placement.Fact, operand]{}, nil)
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
	placementRead, placementOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placement.Fact, routeTag](implementation, fragment.placementRead, owner.FactorRef(), rule.locatePlacement)
	if !placementOK {
		return nil, false
	}
	rule.placementRead = placementRead
	return rule, true
}

// Implementation resolves the owner-fenced sealed Rule issuer.
func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil || rule.owner == nil {
		return nil, false
	}
	_, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil || rule.calls.Algebra() == nil ||
		!coords.Mount.Available() || !coords.Occurrence.Available() {
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

func (rule *HotRule) locateActual(context engine.SelectorContext, candidate operand) bool {
	actual, ok := rule.actual(candidate)
	if !ok || rule.values == nil {
		return false
	}
	for index := 0; index < actual.ActualCount(); index++ {
		tag, tagOK := canonicalActualTag(index)
		coordinate, coordinateOK := coordinateForActual(rule.values.Schema(), actual, index)
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
	if rule == nil || rule.owner == nil || rule.values == nil || rule.calls == nil {
		return false
	}
	actual, actualOK := rule.actual(candidate)
	if !actualOK {
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
	actualSelection, actualReadOK := engine.SelectorRead(context, rule.actualRead)
	if !actualReadOK {
		return false
	}
	var inline [8]actualObservation
	observations, bufferOK := observationBuffer(actual.ActualCount(), inline[:])
	if !bufferOK || !rule.selectorObservations(context, actual, actualSelection, observations) {
		return false
	}
	plan, planOK := planFor(rule.packs, rule.calls.Algebra(), rule.owner.Schema(), rule.values.Schema(), rule.contract, candidate.mounted, callFact, observations)
	if !planOK {
		return false
	}
	for index := 0; index < plan.routeCount(); index++ {
		route, routeOK := plan.routeAt(index)
		if !routeOK || !placementowner.SelectRouteTyped(rule.owner, context, route.key, route.tag) {
			return false
		}
	}
	return true
}

func observationBuffer(count int, inline []actualObservation) ([]actualObservation, bool) {
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
		if !expectedTagOK || tag != expectedTag || !selected || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		coordinate, coordinateOK := coordinateForActual(rule.values.Schema(), actual, ordinal)
		if !available || !coordinateOK || present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
			return false
		}
		observations[ordinal] = actualObservation{fact: fact, present: present, valid: true}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placement.Fact, operand]) engine.RuleResult[placement.Fact] {
	candidate, candidateOK := engine.Operand(frame)
	if !candidateOK || rule == nil || rule.owner == nil || rule.values == nil || rule.calls == nil {
		return engine.RuleResult[placement.Fact]{}
	}
	actual, actualOK := rule.actual(candidate)
	callCells, callOK := engine.ReadValue(frame, rule.callRead)
	actualSelection, actualReadOK := engine.ReadValue(frame, rule.actualRead)
	placementSelection, placementOK := engine.ReadValue(frame, rule.placementRead)
	if !actualOK || !callOK || !actualReadOK || !placementOK || callCells.Count() != 1 {
		return engine.RuleResult[placement.Fact]{}
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	if !callAvailable {
		return engine.RuleResult[placement.Fact]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK {
		return engine.RuleResult[placement.Fact]{}
	}
	if !callPresent {
		if count == 0 {
			return engine.NoSelection(frame, placementSelection)
		}
		return engine.RuleResult[placement.Fact]{}
	}
	var inline [8]actualObservation
	observations, bufferOK := observationBuffer(actual.ActualCount(), inline[:])
	if !bufferOK || !rule.accessObservations(frame, actual, actualSelection, observations) {
		return engine.RuleResult[placement.Fact]{}
	}
	plan, planOK := planFor(rule.packs, rule.calls.Algebra(), rule.owner.Schema(), rule.values.Schema(), rule.contract, candidate.mounted, callFact, observations)
	if !planOK || count != plan.routeCount() {
		return engine.RuleResult[placement.Fact]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag routeTag, cells engine.OrderedCells[placement.Fact]) (placement.Fact, bool) {
		_, routeOK := routeForTag(plan, tag)
		if !routeOK || cells.Count() != 1 {
			return placement.BottomFact(), false
		}
		current, present, available := cells.At(0)
		current, currentOK := placement.AuthenticateFactCell(current, present, available)
		if !currentOK {
			return placement.BottomFact(), false
		}
		return placement.DisplaceFactChecked(current, placement.Send)
	})
}

func (rule *HotRule) accessObservations(frame engine.Frame[placement.Fact, operand], actual packdomain.MountedActualProjection, selection engine.Selection[actualTag, engine.OrderedCells[valuedomain.Value]], observations []actualObservation) bool {
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
		if !expectedTagOK || tag != expectedTag || !selected || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		coordinate, coordinateOK := coordinateForActual(rule.values.Schema(), actual, ordinal)
		if !available || !coordinateOK || present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
			return false
		}
		observations[ordinal] = actualObservation{fact: fact, present: present, valid: true}
	}
	return true
}
