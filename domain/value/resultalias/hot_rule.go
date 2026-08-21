package resultalias

import (
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type aliasOperationPlan struct {
	sources []uint32
}

// targetAliasPlan is one immutable Target plan per binding.  It is deliberately
// operation-indexed rather than an operand table: ResultAlias has one Value
// operand per admitted mounted CallResultSlot, never one operand per operation.
type targetAliasPlan struct {
	byOperation map[vocabulary.Operation]aliasOperationPlan
}

func newTargetAliasPlan(target *contract.Contract) (targetAliasPlan, bool) {
	if target == nil {
		return targetAliasPlan{}, false
	}
	plan := targetAliasPlan{byOperation: make(map[vocabulary.Operation]aliasOperationPlan)}
	for operationIndex := 0; operationIndex < target.Operations.OperationCount(); operationIndex++ {
		operation, operationOK := target.Operations.OperationAt(operationIndex)
		if !operationOK || operation == 0 {
			return targetAliasPlan{}, false
		}
		operationPlan := aliasOperationPlan{}
		seenSources := make(map[uint32]struct{})
		for outcome := 0; outcome < target.Operations.OutcomeCount(operation); outcome++ {
			for aliasIndex := 0; aliasIndex < target.Operations.ResultAliasCount(operation, outcome); aliasIndex++ {
				result, kind, source, aliasOK := target.Operations.ResultAliasAt(operation, outcome, aliasIndex)
				if !aliasOK {
					return targetAliasPlan{}, false
				}
				// This Value rule owns the canonical CallResultSlot at authored
				// ordinal 0. Other result ordinals belong to
				// a different output geometry and contribute nothing here.
				if result != 0 {
					continue
				}
				if kind != vocabulary.InputSourceValueFormal || int(source) >= target.Operations.InputFormalCount(operation) {
					// The compiler rejects non-ValueFormal aliases. Keep the
					// consumer fail-closed if an equivalent malformed Contract
					// reaches a late binder.
					return targetAliasPlan{}, false
				}
				if _, duplicate := seenSources[source]; !duplicate {
					seenSources[source] = struct{}{}
					operationPlan.sources = append(operationPlan.sources, source)
				}
			}
		}
		if len(operationPlan.sources) != 0 {
			sort.Slice(operationPlan.sources, func(left, right int) bool { return operationPlan.sources[left] < operationPlan.sources[right] })
			plan.byOperation[operation] = operationPlan
		}
	}
	return plan, true
}

// HotRule is the Value-owned selected ResultAlias transfer. It retains only
// owner-fenced axes, one global Target alias plan, and Pack's direct mounted
// actual projection. Artifact output geometry is already sealed in the Value
// operand; no Program/Boundary result row is reopened on the hot path.
type HotRule struct {
	implementation *valueowner.RuleImplementation[valuedomain.MountedCallResultSlot]
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	contract       *contract.Contract
	pack           *packdomain.Schema
	plan           targetAliasPlan
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	actualRead     engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]]
}

// BindHot binds the one ordinal-0 result-slot operand rule. The Target contract is
// authenticated against Call's retained contract. Pack is retained solely for
// its sealed mounted-actual projection; no Pack value factor is read by this
// rule.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	values *valueowner.HotOwner,
	calls *callowner.HotOwner,
	targetContract *contract.Contract,
	packSchema *packdomain.Schema,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || values == nil || !values.MatchesBinding(binding) ||
		calls == nil || !calls.MatchesBinding(binding) || values.Schema() == nil || !values.Schema().Valid() ||
		calls.Algebra() == nil || !calls.Algebra().Valid() || targetContract == nil ||
		!values.Schema().LinkOwner().Matches(calls.Algebra().LinkOwner()) || !calls.OwnsTargetContract(targetContract) || packSchema == nil ||
		!packSchema.LinkOwner().Matches(calls.Algebra().LinkOwner()) || !fragment.semantic.Available() {
		return nil, false
	}
	plan, planOK := newTargetAliasPlan(targetContract)
	if !planOK {
		return nil, false
	}
	rule := &HotRule{values: values, calls: calls, contract: targetContract, pack: packSchema, plan: plan}
	implementation, bound := valueowner.BindSelectedRuleDirect(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), engine.HotRuleSpec[valuedomain.Value, valuedomain.MountedCallResultSlot]{
		OperandContent: func(row valuedomain.MountedCallResultSlot) (valuedomain.MountedCallResultSlot, [32]byte, bool) {
			return resultAliasContent(values.Schema(), row)
		},
		Fold: rule.fold,
	}, engine.HotCarrySpec[valuedomain.Value, valuedomain.MountedCallResultSlot]{}, func(row valuedomain.MountedCallResultSlot) (uint64, bool) {
		coordinate, coordinateOK := row.Coordinate()
		index, indexOK := values.Schema().CoordinateIndex(coordinate)
		return uint64(index), coordinateOK && indexOK
	})
	if !bound || implementation == nil {
		return nil, false
	}
	callRead, callOK := valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(row valuedomain.MountedCallResultSlot) (uint64, bool) {
		module, occurrence, occurrenceOK := mountedOccurrence(row)
		key, keyOK := projectCallKey(calls.Algebra(), module, occurrence, occurrenceOK)
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), keyOK && indexOK && key.IsApplication()
	})
	if !callOK {
		return nil, false
	}
	actualRead, actualOK := valueowner.AddSelectedRuleDirectOperandRead[valuedomain.MountedCallResultSlot, valuedomain.Value, uint64](implementation, fragment.actualRead, values.FactorRef(), rule.locateActual)
	if !actualOK {
		return nil, false
	}
	rule.implementation, rule.callRead, rule.actualRead = implementation, callRead, actualRead
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func resultAliasContent(schema *valuedomain.Schema, row valuedomain.MountedCallResultSlot) (valuedomain.MountedCallResultSlot, [32]byte, bool) {
	if schema == nil || !schema.OwnsMountedCallResultSlot(row) {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	module, moduleOK := row.Module()
	call, callOK := row.CallID()
	linkID := schema.LinkID()
	if !moduleOK || !callOK || !module.Available() || !call.Available() || !linkID.Available() {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.value.resultalias.v1\x00"))
	_, _ = hash.Write(linkID[:])
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(call[:])
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return row, digest, digest != ([32]byte{})
}

func projectCallKey(algebra *calldomain.Algebra, module, occurrence identity.ContentID, ok bool) (calldomain.Key, bool) {
	if algebra == nil || !algebra.Valid() || !ok || !module.Available() || !occurrence.Available() {
		return calldomain.Key{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(module, occurrence)
	_, callID, mountedModule, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	key, keyOK := algebra.KeyForMountedCall(mounted)
	return key, mountedOK && identityOK && keyOK && key.IsApplication() && key.Valid() && callID == occurrence && mountedModule == module
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.MountedCallResultSlot, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return valuedomain.MountedCallResultSlot{}, false
	}
	row, rowOK := rule.values.Schema().MountedCallResultSlotFor(coords.Mount, coords.Occurrence, 0)
	return row, rowOK && rule.values.Schema().OwnsMountedCallResultSlot(row)
}

func mountedOccurrence(row valuedomain.MountedCallResultSlot) (module, occurrence identity.ContentID, ok bool) {
	module, moduleOK := row.Module()
	call, callOK := row.CallID()
	return module, call, moduleOK && callOK && module.Available() && call.Available()
}

func (rule *HotRule) mountedActual(row valuedomain.MountedCallResultSlot) (packdomain.MountedActualProjection, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || rule.calls == nil || rule.calls.Algebra() == nil || rule.pack == nil ||
		!rule.values.Schema().LinkOwner().Matches(rule.calls.Algebra().LinkOwner()) || !rule.pack.LinkOwner().Matches(rule.calls.Algebra().LinkOwner()) {
		return packdomain.MountedActualProjection{}, false
	}
	module, occurrence, occurrenceOK := mountedOccurrence(row)
	if !occurrenceOK {
		return packdomain.MountedActualProjection{}, false
	}
	mounted, mountedOK := rule.calls.Algebra().MountedCallForOccurrence(module, occurrence)
	key, keyOK := rule.calls.Algebra().KeyForMountedCall(mounted)
	_, callID, mountedModule, _, _, identityOK := rule.calls.Algebra().MountedCallIdentity(mounted)
	actual, actualOK := rule.pack.MountedActualProjection(module, occurrence)
	return actual, mountedOK && keyOK && key.Valid() && key.IsApplication() && identityOK && callID == occurrence && mountedModule == module &&
		actualOK && actual.Valid() && actual.OwnedBy(rule.pack)
}

type aliasSelection struct {
	sources []uint32
	aliased bool
	top     bool
}

// selectedAliases validates one exact Call fact and returns the union of all
// result-0 ValueFormal source ordinals named by its selected operation
// alternatives. Body targets and operations with no alias are intentionally
// ignored. Opaque Call alternatives are conservative Top.
func (rule *HotRule) selectedAliases(actual packdomain.MountedActualProjection, fact calldomain.Value) (aliasSelection, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || rule.calls == nil || rule.calls.Algebra() == nil || rule.pack == nil ||
		!actual.Valid() || !actual.OwnedBy(rule.pack) || !callValueValid(fact) {
		return aliasSelection{}, false
	}
	if fact.IsTop() || fact.HasOpaqueAlternative() {
		return aliasSelection{top: true}, true
	}
	if fact.IsEmpty() || fact.KnownTargetCount() == 0 {
		return aliasSelection{}, true
	}
	selection := aliasSelection{}
	for index := 0; index < fact.KnownTargetCount(); index++ {
		target, targetOK := fact.KnownTargetAt(index)
		if !targetOK || !rule.calls.Algebra().OwnsTarget(target) {
			return aliasSelection{}, false
		}
		operation, operationOK := target.Operation()
		if !operationOK {
			// Body targets are handled by ordinary body-result/Pack
			// consumers and do not carry ResultAlias declarations.
			continue
		}
		operationPlan, hasAliases := rule.plan.byOperation[operation]
		if !hasAliases {
			continue
		}
		selection.aliased = true
		selection.sources = append(selection.sources, operationPlan.sources...)
	}
	if !selection.aliased {
		return selection, true
	}
	if _, hasTail := actual.TailID(); hasTail {
		// A mounted runtime suffix has no fixed Value coordinate. Even when
		// a selected alias names a known prefix formal, the unrepresented
		// suffix can carry additional call evidence; widen instead of treating
		// the fixed projection as complete.
		selection.top = true
		return selection, true
	}
	if len(selection.sources) > 1 {
		sort.Slice(selection.sources, func(left, right int) bool { return selection.sources[left] < selection.sources[right] })
		write := 1
		for _, source := range selection.sources[1:] {
			if source != selection.sources[write-1] {
				selection.sources[write] = source
				write++
			}
		}
		selection.sources = selection.sources[:write]
	}
	for _, source := range selection.sources {
		if uint64(source) >= uint64(actual.ActualCount()) {
			selection.top = true
			return selection, true
		}
		semantic, semanticOK := actual.ActualAt(int(source))
		if !semanticOK || rule.values == nil || rule.values.Schema() == nil {
			selection.top = true
			return selection, true
		}
		if _, coordinateOK := rule.values.Schema().CoordinateForMountedSemantic(semantic.Module(), semantic.ID()); !coordinateOK {
			// A declared fixed alias whose mounted Value coordinate is absent
			// is unresolved evidence, not proof of no alias. Widen safely.
			selection.top = true
			return selection, true
		}
	}
	return selection, true
}

func (rule *HotRule) locateActual(context engine.SelectorContext, row valuedomain.MountedCallResultSlot) bool {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return false
	}
	callCells, callOK := engine.SelectorRead(context, rule.callRead)
	if !callOK || callCells.Count() != 1 {
		return false
	}
	callFact, present, available := callCells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	actual, actualOK := rule.mountedActual(row)
	selection, selectionOK := rule.selectedAliases(actual, callFact)
	if !actualOK || !selectionOK {
		return false
	}
	if selection.top || !selection.aliased {
		return true
	}
	for _, sourceOrdinal := range selection.sources {
		source, sourceOK := actual.ActualAt(int(sourceOrdinal))
		coordinate, coordinateOK := rule.values.Schema().CoordinateForMountedSemantic(source.Module(), source.ID())
		if !sourceOK || !coordinateOK || !valueowner.SelectRouteTyped(rule.values, context, coordinate, uint64(sourceOrdinal)+1) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[valuedomain.Value, valuedomain.MountedCallResultSlot]) engine.RuleResult[valuedomain.Value] {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return engine.RuleResult[valuedomain.Value]{}
	}
	operand, operandOK := engine.Operand(frame)
	if !operandOK || !rule.values.Schema().OwnsMountedCallResultSlot(operand) {
		return engine.RuleResult[valuedomain.Value]{}
	}
	callCells, callOK := engine.ReadValue(frame, rule.callRead)
	actualSelection, actualOK := engine.ReadValue(frame, rule.actualRead)
	if !callOK || !actualOK || callCells.Count() != 1 {
		return engine.RuleResult[valuedomain.Value]{}
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	if !callAvailable {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if !callPresent {
		return engine.NoCandidate(frame)
	}
	actual, mountedOK := rule.mountedActual(operand)
	selection, selectionOK := rule.selectedAliases(actual, callFact)
	if !mountedOK || !selectionOK {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if selection.top {
		return engine.Staged(frame, rule.values.Schema().Top())
	}
	if !selection.aliased {
		return engine.NoCandidate(frame)
	}
	count, countOK := engine.SelectionCount(frame, actualSelection)
	if !countOK || count != len(selection.sources) {
		return engine.RuleResult[valuedomain.Value]{}
	}
	combined := rule.values.Schema().Bottom()
	presentAny := false
	seen := make([]bool, len(selection.sources))
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.SelectionAt(frame, actualSelection, index)
		if !selected || cells.Count() != 1 || tag == 0 || tag-1 > uint64(^uint32(0)) {
			return engine.RuleResult[valuedomain.Value]{}
		}
		sourceOrdinal := uint32(tag - 1)
		sourceIndex := sourceOrdinalIndex(selection.sources, sourceOrdinal)
		if sourceIndex < 0 || seen[sourceIndex] {
			return engine.RuleResult[valuedomain.Value]{}
		}
		// Selection order is physical Unit order, not authored ordinal. Track
		// the canonical tag set directly instead of replaying prior rows.
		seen[sourceIndex] = true
		fact, present, available := cells.At(0)
		if !available {
			return engine.RuleResult[valuedomain.Value]{}
		}
		if present {
			source, sourceOK := actual.ActualAt(int(sourceOrdinal))
			coordinate, coordinateOK := rule.values.Schema().CoordinateForMountedSemantic(source.Module(), source.ID())
			if !sourceOK || !coordinateOK || !rule.values.Schema().AdmitsCoordinate(coordinate, fact) {
				return engine.RuleResult[valuedomain.Value]{}
			}
			if !presentAny {
				combined, presentAny = fact, true
			} else {
				combined, presentAny = joinValues(rule.values.Schema(), combined, fact)
				if !presentAny {
					return engine.RuleResult[valuedomain.Value]{}
				}
			}
		}
	}
	if !presentAny || combined.IsBottom() {
		return engine.NoCandidate(frame)
	}
	return engine.Staged(frame, combined)
}

func sourceOrdinalIndex(sources []uint32, source uint32) int {
	index := sort.Search(len(sources), func(index int) bool { return sources[index] >= source })
	if index >= len(sources) || sources[index] != source {
		return -1
	}
	return index
}

func joinValues(schema *valuedomain.Schema, left, right valuedomain.Value) (valuedomain.Value, bool) {
	if schema == nil {
		return valuedomain.Value{}, false
	}
	return schema.Join(left, right)
}

func callValueValid(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}

// Implementation returns the pending Value-owned issuer.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[valuedomain.MountedCallResultSlot], bool) {
	if rule == nil || rule.implementation == nil || rule.values == nil {
		return nil, false
	}
	return rule.implementation, true
}

// SealProgramRule publishes the exact generic engine Rule after the shared
// binding has sealed. The engine receives no ResultAlias-specific behavior.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || rule.values == nil || rule.implementation == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}
