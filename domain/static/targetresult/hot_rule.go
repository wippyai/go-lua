package targetresult

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// HotRule is the owner-local Static projection for one mounted result slot.
// It retains only sealed owners, the exact Target contract, and an engine read
// receipt. No Program directory, Value fact, or parallel coordinate geometry
// is retained.
type HotRule struct {
	implementation *staticowner.RuleImplementation[valuedomain.MountedCallResultSlot]
	statics        *staticowner.HotOwner
	calls          *callowner.HotOwner
	contract       *contract.Contract
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
}

// BindHot binds the selected mounted result rule to the exact shared schema
// binding. The Static owner supplies the output Factor; Call supplies the one
// exact predecessor fact and Target contract authority.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	statics *staticowner.HotOwner,
	calls *callowner.HotOwner,
	targetContract *contract.Contract,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || statics == nil || !statics.MatchesBinding(binding) ||
		statics.Classes() == nil || statics.ValueSchema() == nil || calls == nil || !calls.MatchesBinding(binding) ||
		calls.Algebra() == nil || !calls.Algebra().Valid() || targetContract == nil ||
		!calls.OwnsTargetContract(targetContract) || statics.LinkID() != calls.LinkID() || !fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{statics: statics, calls: calls, contract: targetContract}
	implementation, bound := staticowner.BindSeedRuleDirect(statics, fragment.slot, fragment.write, engine.HotRuleSpec[staticdomain.TypeFact, valuedomain.MountedCallResultSlot]{
		OperandContent:  targetResultContent,
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, func(row valuedomain.MountedCallResultSlot) (uint64, bool) {
		coordinate, coordinateOK := row.Coordinate()
		index, indexOK := statics.ValueSchema().CoordinateIndex(coordinate)
		ordinal, ordinalOK := row.Ordinal()
		return uint64(index), coordinateOK && indexOK && ordinalOK && ordinal == 0
	})
	if !bound || implementation == nil {
		return nil, false
	}
	callRead, callOK := staticowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(row valuedomain.MountedCallResultSlot) (uint64, bool) {
		module, moduleOK := row.Module()
		callID, callOK := row.CallID()
		mounted, mountedOK := calls.Algebra().MountedCallForOccurrence(module, callID)
		key, keyOK := calls.Algebra().KeyForMountedCall(mounted)
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), moduleOK && callOK && mountedOK && keyOK && indexOK && key.IsApplication()
	})
	if !callOK {
		return nil, false
	}
	rule.implementation, rule.callRead = implementation, callRead
	return rule, true
}

func (rule *HotRule) valid() bool {
	return rule != nil && rule.statics != nil && rule.statics.Classes() != nil && rule.statics.ValueSchema() != nil &&
		rule.calls != nil && rule.calls.Algebra() != nil && rule.calls.Algebra().Valid() && rule.contract != nil &&
		rule.calls.OwnsTargetContract(rule.contract) && rule.statics.LinkID() == rule.calls.LinkID()
}

// resolveOperand reissues Value's exact existing mounted CallResultSlot. It
// admits only ordinal zero; later result slots belong to another result
// geometry and must not be guessed into this rule.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.MountedCallResultSlot, bool) {
	if !rule.valid() || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return valuedomain.MountedCallResultSlot{}, false
	}
	row, ok := rule.statics.ValueSchema().MountedCallResultSlotFor(coords.Mount, coords.Occurrence, 0)
	return row, ok && rule.statics.ValueSchema().OwnsMountedCallResultSlot(row)
}

// targetResultContent uses the mounted row's own immutable identity. The
// digest is merely the engine's operand-content key; it is not a new domain
// coordinate or receipt.
func targetResultContent(row valuedomain.MountedCallResultSlot) (valuedomain.MountedCallResultSlot, [32]byte, bool) {
	if _, ok := row.Ordinal(); !ok {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	ordinal, _ := row.Ordinal()
	if ordinal != 0 {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	module, moduleOK := row.Module()
	callID, callOK := row.CallID()
	slotID, slotOK := row.SlotID()
	if !moduleOK || !callOK || !slotOK || !module.Available() || !callID.Available() || !slotID.Available() {
		return valuedomain.MountedCallResultSlot{}, [32]byte{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.static.targetresult/v1\x00"))
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(callID[:])
	_, _ = hash.Write(slotID[:])
	var ordinalBytes [4]byte
	binary.BigEndian.PutUint32(ordinalBytes[:], ordinal)
	_, _ = hash.Write(ordinalBytes[:])
	var content [32]byte
	copy(content[:], hash.Sum(nil))
	return row, content, true
}

func (rule *HotRule) fold(frame engine.Frame[staticdomain.TypeFact, valuedomain.MountedCallResultSlot]) engine.RuleResult[staticdomain.TypeFact] {
	if !rule.valid() {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	row, rowOK := engine.Operand(frame)
	cells, readOK := engine.ReadValue(frame, rule.callRead)
	if !rowOK || !readOK || cells.Count() != 1 {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	if !present {
		return engine.NoCandidate(frame)
	}
	projected, selected := rule.project(row, fact)
	if !selected {
		return engine.NoCandidate(frame)
	}
	return engine.Staged(frame, projected)
}

// project computes the only positive semantic result of this rule. It
// deliberately returns no candidate for open/opaque Call values, body-only
// alternatives, operations without a concrete normal result, and malformed or
// foreign Target alternatives. In particular, uncertainty never becomes Top.
func (rule *HotRule) project(row valuedomain.MountedCallResultSlot, fact calldomain.Value) (staticdomain.TypeFact, bool) {
	if !rule.valid() || !callValueValid(fact) || !rule.statics.ValueSchema().OwnsMountedCallResultSlot(row) {
		return staticdomain.TypeFact{}, false
	}
	ordinal, ordinalOK := row.Ordinal()
	if !ordinalOK || ordinal != 0 {
		return staticdomain.TypeFact{}, false
	}
	if fact.IsTop() || fact.HasOpaqueAlternative() || fact.IsEmpty() || fact.KnownTargetCount() == 0 {
		return staticdomain.TypeFact{}, false
	}
	classes := rule.statics.Classes()
	combined := classes.TypeBottom()
	selected := false
	for index := 0; index < fact.KnownTargetCount(); index++ {
		target, targetOK := fact.KnownTargetAt(index)
		if !targetOK || !rule.calls.Algebra().OwnsTarget(target) {
			return staticdomain.TypeFact{}, false
		}
		operation, operationKind := rule.calls.Algebra().ClassifyTargetOperation(target)
		if operationKind == calldomain.TargetOperationInvalid {
			return staticdomain.TypeFact{}, false
		}
		if operationKind == calldomain.TargetOperationNone {
			continue
		}
		result, resultOK := normalResultType(rule.contract, operation, 0)
		if !resultOK {
			return staticdomain.TypeFact{}, false
		}
		candidate, candidateOK := classes.TypeFactForTarget(rule.contract, result)
		if !candidateOK || !candidate.Valid() {
			return staticdomain.TypeFact{}, false
		}
		if !selected {
			combined, selected = candidate, true
		} else {
			combined = classes.JoinTypeFact(combined, candidate)
		}
	}
	if !selected || !combined.Valid() || combined.IsBottom() {
		return staticdomain.TypeFact{}, false
	}
	return combined, true
}

func normalResultType(target *contract.Contract, operation vocabulary.Operation, result int) (vocabulary.Type, bool) {
	if target == nil || operation == 0 || result < 0 || operation >= vocabulary.Operation(^uint32(0)) {
		return 0, false
	}
	found := vocabulary.Type(0)
	for outcome := 0; outcome < target.Operations.OutcomeCount(operation); outcome++ {
		outcomeKind, values, outcomeOK := target.Operations.OutcomeAt(operation, outcome)
		if !outcomeOK || outcomeKind != flowkind.OutcomeNormal {
			continue
		}
		typ, typeOK := target.Operations.ValuesAt(values, result)
		if !typeOK || typ == 0 {
			return 0, false
		}
		if found == 0 {
			found = typ
			continue
		}
		// Multiple normal outcomes can describe the same operation. This rule
		// has no separate outcome axis, so a portable single TypeFact result is
		// possible only when the authored type handle agrees exactly.
		if found != typ {
			return 0, false
		}
	}
	return found, found != 0
}

func callValueValid(value calldomain.Value) bool {
	return value.IsTop() || value.IsOpen() || value.IsComplete() || value.IsEmpty()
}

// Implementation resolves Static's owner-fenced sealed engine issuer.
func (rule *HotRule) Implementation() (*staticowner.RuleImplementation[valuedomain.MountedCallResultSlot], bool) {
	if rule == nil || rule.implementation == nil || rule.statics == nil {
		return nil, false
	}
	_, ok := staticowner.ResolveRuleImplementationFor(rule.statics, rule.implementation)
	return rule.implementation, ok
}
