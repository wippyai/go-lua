package dispatch

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Call dispatch's exact-read Rule binder. Its operand is Call's
// canonical mounted row; bind time joins it once to Value's coordinate and
// Pack's call-root identity. Hot callbacks consume the resulting dense private
// rows and never reopen that cross-domain join.
type HotRule struct {
	binding        *engine.SchemaBinding
	fragment       *SchemaFragment
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	heaps          heapdomain.Schema
	packs          *packdomain.Schema
	rows           []dispatchRow
	read           engine.Read[engine.OrderedCells[valuedomain.Value]]
	implementation *callowner.HeterogeneousRuleImplementation[valuedomain.Value, calldomain.MountedCall]
}

// BindHot binds the closed Value-read/Call-write dispatch lane through typed
// FactorRefs. Heap and Pack remain exact row owners used at the reducer
// boundary; no Program or Flow topology is reopened by the callbacks.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, values *valueowner.HotOwner, calls *callowner.HotOwner, heaps heapdomain.Schema, packs *packdomain.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) || values.Schema() == nil || calls.Algebra() == nil || !calls.Algebra().Valid() || !heaps.Valid() || packs == nil || !fragment.semantic.Available() {
		return nil, false
	}
	linkOwner := calls.Algebra().LinkOwner()
	if !linkOwner.Available() || !values.Schema().LinkOwner().Matches(linkOwner) || !heaps.LinkOwner().Matches(linkOwner) || !packs.LinkOwner().Matches(linkOwner) || !values.Schema().OwnsHeapSchema(heaps) {
		return nil, false
	}
	hot := &HotRule{binding: binding, fragment: fragment, values: values, calls: calls, heaps: heaps, packs: packs}
	rows, rowsOK := sealDispatchRows(hot)
	if !rowsOK {
		return nil, false
	}
	hot.rows = rows
	implementation, runtimeRead, ok := callowner.BindHeterogeneousExactReadRule(calls, fragment.slot, fragment.read, fragment.value, fragment.write, engine.HotRuleSpec[calldomain.Value, calldomain.MountedCall]{
		OperandContent:  hot.operandContent,
		OperandResolver: hot.resolveOperand,
		Fold: func(frame engine.Frame[calldomain.Value, calldomain.MountedCall]) engine.RuleResult[calldomain.Value] {
			mounted, mountedOK := engine.Operand(frame)
			row, rowOK := hot.rowForMounted(mounted)
			if !mountedOK || !rowOK {
				return engine.RuleResult[calldomain.Value]{}
			}
			cells, readOK := engine.ReadValue(frame, hot.read)
			if !readOK || cells.Count() != 1 {
				return engine.RuleResult[calldomain.Value]{}
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleResult[calldomain.Value]{}
			}
			if !present {
				return engine.NoCandidate(frame)
			}
			result, resultOK := reduce(hot, mounted, row, fact)
			if !resultOK {
				return engine.RuleResult[calldomain.Value]{}
			}
			return engine.Staged(frame, result)
		},
	}, func(mounted calldomain.MountedCall) (uint64, bool) {
		row, rowOK := hot.rowForMounted(mounted)
		index, indexOK := values.Schema().CoordinateIndex(row.coordinate)
		return uint64(index), rowOK && indexOK
	}, func(mounted calldomain.MountedCall) (uint64, bool) {
		row, rowOK := hot.rowForMounted(mounted)
		index, indexOK := calls.Algebra().KeyIndex(row.key)
		return uint64(index), rowOK && indexOK && index >= 0
	})
	if !ok {
		return nil, false
	}
	hot.read = runtimeRead
	hot.implementation = implementation
	return hot, true
}

// resolveOperand reads Call's owner-fenced occurrence inverse directly, then
// admits only rows present in Dispatch's bind-time projection.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (calldomain.MountedCall, bool) {
	if !rule.valid() {
		return calldomain.MountedCall{}, false
	}
	mounted, ok := rule.calls.Algebra().MountedCallForOccurrence(coords.Mount, coords.Occurrence)
	if !ok {
		return calldomain.MountedCall{}, false
	}
	_, rowOK := rule.rowForMounted(mounted)
	return mounted, rowOK
}

func (rule *HotRule) rowForMounted(mounted calldomain.MountedCall) (dispatchRow, bool) {
	if !rule.valid() {
		return dispatchRow{}, false
	}
	index, ok := rule.calls.Algebra().MountedCallOrdinal(mounted)
	if !ok || index < 0 || index >= len(rule.rows) {
		return dispatchRow{}, false
	}
	bound := rule.rows[index]
	return bound, bound.key.Valid() && bound.key.IsApplication() && bound.coordinate.Valid() && bound.contentID.Available()
}

func (rule *HotRule) operandContent(mounted calldomain.MountedCall) (calldomain.MountedCall, [32]byte, bool) {
	row, ok := rule.rowForMounted(mounted)
	if !ok {
		return calldomain.MountedCall{}, [32]byte{}, false
	}
	return mounted, [32]byte(row.contentID), true
}

func (rule *HotRule) valid() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.values == nil || !rule.values.MatchesBinding(rule.binding) || rule.calls == nil || !rule.calls.MatchesBinding(rule.binding) || rule.values.Schema() == nil || rule.calls.Algebra() == nil || !rule.calls.Algebra().Valid() || !rule.heaps.Valid() || rule.packs == nil || len(rule.rows) != rule.calls.Algebra().MountedCallCount() {
		return false
	}
	linkOwner := rule.calls.Algebra().LinkOwner()
	return linkOwner.Available() && rule.values.Schema().LinkOwner().Matches(linkOwner) && rule.heaps.LinkOwner().Matches(linkOwner) && rule.packs.LinkOwner().Matches(linkOwner) && rule.values.Schema().OwnsHeapSchema(rule.heaps)
}
