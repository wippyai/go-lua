package bodyresult

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type bodyReturnPlan struct {
	boundaries []valuedomain.ReturnBoundary
}

type HotRule struct {
	implementation *valueowner.RuleImplementation[valuedomain.MountedCallResultSlot]
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	byBody         map[calldomain.Body]bodyReturnPlan
	semantic       identity.SemanticKey
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	returnRead     engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]]
}

func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, values *valueowner.HotOwner, calls *callowner.HotOwner) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || values == nil || !values.MatchesBinding(binding) || values.Schema() == nil || !values.Schema().Valid() ||
		calls == nil || !calls.MatchesBinding(binding) || calls.Algebra() == nil || !calls.Algebra().Valid() ||
		!values.Schema().LinkOwner().Matches(calls.Algebra().LinkOwner()) || !fragment.semantic.Available() {
		return nil, false
	}
	plan, planOK := sealBodyReturnPlan(values.Schema(), calls.Algebra())
	if !planOK {
		return nil, false
	}
	rule := &HotRule{values: values, calls: calls, byBody: plan, semantic: fragment.semantic}
	implementation, bound := valueowner.BindSelectedRuleDirect(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), engine.HotRuleSpec[valuedomain.Value, valuedomain.MountedCallResultSlot]{
		OperandContent:  rule.operandContent,
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[valuedomain.Value, valuedomain.MountedCallResultSlot]{}, func(row valuedomain.MountedCallResultSlot) (uint64, bool) {
		coordinate, coordinateOK := row.Coordinate()
		index, indexOK := values.Schema().CoordinateIndex(coordinate)
		ordinal, ordinalOK := row.Ordinal()
		return uint64(index), coordinateOK && indexOK && ordinalOK && ordinal == 0
	})
	if !bound || implementation == nil {
		return nil, false
	}
	callRead, callOK := valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(row valuedomain.MountedCallResultSlot) (uint64, bool) {
		module, moduleOK := row.Module()
		call, callOK := row.CallID()
		coordinate, coordinateOK := calls.Algebra().CallCoordinateForOccurrence(module, call)
		index, indexOK := coordinate.CoordinateIndex()
		return index, moduleOK && callOK && coordinateOK && indexOK
	})
	if !callOK {
		return nil, false
	}
	returnRead, returnOK := valueowner.AddSelectedRuleDirectOperandRead[valuedomain.MountedCallResultSlot, valuedomain.Value, uint64](implementation, fragment.returnRead, values.FactorRef(), rule.locateReturns)
	if !returnOK {
		return nil, false
	}
	rule.implementation, rule.callRead, rule.returnRead = implementation, callRead, returnRead
	return rule, true
}

func sealBodyReturnPlan(values *valuedomain.Schema, calls *calldomain.Algebra) (map[calldomain.Body]bodyReturnPlan, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() {
		return nil, false
	}
	bodies := calls.Bodies()
	plan := make(map[calldomain.Body]bodyReturnPlan, bodies.Count())
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		module, moduleOK := body.ModuleKey()
		path, pathOK := body.BodyPath()
		boundaries, boundariesOK := values.ReturnBoundariesForBody(module, path)
		if !bodyOK || !moduleOK || !pathOK {
			return nil, false
		}
		// A callable body with no authored ReturnBoundary falls off its end and
		// returns no values. Keep the empty plan as that exact Lua case; it is
		// not missing analysis evidence.
		if !boundariesOK {
			boundaries = nil
		}
		for _, boundary := range boundaries {
			owner, ownerOK := boundary.BodyID()
			if !ownerOK || owner != path || !values.OwnsReturnBoundary(boundary) {
				return nil, false
			}
		}
		plan[body] = bodyReturnPlan{boundaries: boundaries}
	}
	return plan, len(plan) == bodies.Count()
}

type returnSelection struct {
	tags    []uint64
	hasBody bool
	nilCase bool
	top     bool
}

func (rule *HotRule) selectedReturns(row valuedomain.MountedCallResultSlot, fact calldomain.Value) (returnSelection, bool) {
	if !rule.valid() || !callValueValid(fact) {
		return returnSelection{}, false
	}
	ordinal, ordinalOK := row.Ordinal()
	if !ordinalOK || ordinal != 0 {
		return returnSelection{}, false
	}
	if fact.IsTop() || fact.HasOpaqueAlternative() {
		return returnSelection{top: true}, true
	}
	selection := returnSelection{}
	seen := make(map[uint64]struct{})
	for index := 0; index < fact.KnownTargetCount(); index++ {
		target, targetOK := fact.KnownTargetAt(index)
		if !targetOK || !rule.calls.Algebra().OwnsTarget(target) {
			return returnSelection{}, false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		bodyPlan, planned := rule.byBody[body]
		if !planned {
			return returnSelection{}, false
		}
		selection.hasBody = true
		if len(bodyPlan.boundaries) == 0 {
			selection.nilCase = true
			continue
		}
		for _, boundary := range bodyPlan.boundaries {
			if boundary.MemberCount() == 0 {
				if boundary.HasTail() {
					selection.top = true
				} else {
					selection.nilCase = true
				}
				continue
			}
			member, memberOK := boundary.MemberAt(0)
			coordinate, coordinateOK := member.Coordinate()
			coordinateIndex, indexOK := rule.values.Schema().CoordinateIndex(coordinate)
			tag := uint64(coordinateIndex) + 1
			if !memberOK || !coordinateOK || !indexOK || tag == 0 {
				return returnSelection{}, false
			}
			if _, duplicate := seen[tag]; !duplicate {
				seen[tag] = struct{}{}
				selection.tags = append(selection.tags, tag)
			}
		}
	}
	sort.Slice(selection.tags, func(left, right int) bool { return selection.tags[left] < selection.tags[right] })
	return selection, true
}

func (rule *HotRule) locateReturns(context engine.SelectorContext, row valuedomain.MountedCallResultSlot) bool {
	cells, ok := engine.SelectorRead(context, rule.callRead)
	if !ok || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	selection, selected := rule.selectedReturns(row, fact)
	if !selected || selection.top {
		return selected
	}
	for _, tag := range selection.tags {
		coordinate, coordinateOK := rule.values.Schema().CoordinateAt(int(tag - 1))
		if !coordinateOK || !valueowner.SelectRouteTyped(rule.values, context, coordinate, tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[valuedomain.Value, valuedomain.MountedCallResultSlot]) engine.RuleResult[valuedomain.Value] {
	if !rule.valid() {
		return engine.RuleResult[valuedomain.Value]{}
	}
	row, rowOK := engine.Operand(frame)
	callCells, callOK := engine.ReadValue(frame, rule.callRead)
	returnCells, returnsOK := engine.ReadValue(frame, rule.returnRead)
	if !rowOK || !callOK || !returnsOK || callCells.Count() != 1 {
		return engine.RuleResult[valuedomain.Value]{}
	}
	fact, present, available := callCells.At(0)
	if !available {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if !present {
		return engine.NoCandidate(frame)
	}
	selection, selected := rule.selectedReturns(row, fact)
	if !selected {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if selection.top {
		return engine.Staged(frame, rule.values.Schema().Top())
	}
	if !selection.hasBody {
		return engine.NoCandidate(frame)
	}
	count, countOK := engine.SelectionCount(frame, returnCells)
	if !countOK || count != len(selection.tags) {
		return engine.RuleResult[valuedomain.Value]{}
	}
	combined := rule.values.Schema().Bottom()
	presentAny := false
	if selection.nilCase {
		var nilOK bool
		combined, nilOK = rule.values.Schema().Nil()
		if !nilOK {
			return engine.RuleResult[valuedomain.Value]{}
		}
		presentAny = true
	}
	seen := make(map[uint64]struct{}, count)
	for index := 0; index < count; index++ {
		tag, cells, itemOK := engine.SelectionAt(frame, returnCells, index)
		if !itemOK || cells.Count() != 1 || !containsTag(selection.tags, tag) {
			return engine.RuleResult[valuedomain.Value]{}
		}
		if _, duplicate := seen[tag]; duplicate {
			return engine.RuleResult[valuedomain.Value]{}
		}
		seen[tag] = struct{}{}
		value, valuePresent, valueAvailable := cells.At(0)
		coordinate, coordinateOK := rule.values.Schema().CoordinateAt(int(tag - 1))
		if !valueAvailable || !coordinateOK || valuePresent && !rule.values.Schema().AdmitsCoordinate(coordinate, value) {
			return engine.RuleResult[valuedomain.Value]{}
		}
		if !valuePresent {
			continue
		}
		if !presentAny {
			combined, presentAny = value, true
		} else {
			var joined bool
			combined, joined = rule.values.Schema().Join(combined, value)
			if !joined {
				return engine.RuleResult[valuedomain.Value]{}
			}
		}
	}
	if !presentAny || combined.IsBottom() {
		return engine.NoCandidate(frame)
	}
	return engine.Staged(frame, combined)
}

func containsTag(tags []uint64, tag uint64) bool {
	index := sort.Search(len(tags), func(index int) bool { return tags[index] >= tag })
	return index < len(tags) && tags[index] == tag
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.MountedCallResultSlot, bool) {
	if !rule.valid() || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return valuedomain.MountedCallResultSlot{}, false
	}
	row, ok := rule.values.Schema().MountedCallResultSlotFor(coords.Mount, coords.Occurrence, 0)
	return row, ok && rule.values.Schema().OwnsMountedCallResultSlot(row)
}

// operandContent declares the operand under Value's owner-issued row identity
// framed by this rule's semantic key. Result-alias binds over the same slot
// row, so the framing is what keeps the two operand entities apart.
func (rule *HotRule) operandContent(row valuedomain.MountedCallResultSlot) (valuedomain.MountedCallResultSlot, [32]byte, bool) {
	ordinal, ordinalOK := row.Ordinal()
	// This rule is the sole transfer for the fixed first return position.
	// A later result slot has its own semantic owner; accepting it here would
	// give the result-zero rule a second, incompatible output authority.
	if !rule.valid() || !ordinalOK || ordinal != 0 {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	id, idOK := row.ID()
	if !idOK {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	framed, framedOK := rule.values.Schema().FrameOperandIdentity(rule.semantic, id)
	if !framedOK {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	return row, [32]byte(framed), true
}

func (rule *HotRule) valid() bool {
	return rule != nil && rule.values != nil && rule.values.Schema() != nil && rule.values.Schema().Valid() && rule.calls != nil && rule.calls.Algebra() != nil && rule.calls.Algebra().Valid() && rule.semantic.Available() && len(rule.byBody) == rule.calls.Algebra().Bodies().Count()
}

func callValueValid(value calldomain.Value) bool {
	return value.IsTop() || value.IsOpen() || value.IsComplete() || value.IsEmpty()
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[valuedomain.MountedCallResultSlot], bool) {
	if !rule.valid() || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}
