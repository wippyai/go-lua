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
// canonical mounted row; callbacks join that row to Value's coordinate and
// atom relations, Pack's call-root row, Heap's allocation rows, and Call's
// target rows without retaining a second mounted-call directory.
type HotRule struct {
	binding        *engine.SchemaBinding
	fragment       *SchemaFragment
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	heaps          heapdomain.Schema
	packs          *packdomain.Schema
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
	implementation, runtimeRead, ok := callowner.BindHeterogeneousExactReadRule(calls, fragment.slot, fragment.read, fragment.value, fragment.write, engine.HotRuleSpec[calldomain.Value, calldomain.MountedCall]{
		OperandContent:  hot.operandContent,
		OperandResolver: hot.resolveOperand,
		Fold: func(frame engine.Frame[calldomain.Value, calldomain.MountedCall]) engine.RuleResult[calldomain.Value] {
			mounted, mountedOK := engine.Operand(frame)
			bound, siteOK := hot.siteForMounted(mounted)
			if !mountedOK || !siteOK {
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
			result, resultOK := reduce(bound, fact)
			if !resultOK {
				return engine.RuleResult[calldomain.Value]{}
			}
			return engine.Staged(frame, result)
		},
	}, func(mounted calldomain.MountedCall) (uint64, bool) {
		bound, boundOK := hot.siteForMounted(mounted)
		coordinate, coordinateOK := bound.valueCoordinate()
		index, indexOK := values.Schema().CoordinateIndex(coordinate)
		return uint64(index), boundOK && coordinateOK && indexOK
	}, func(mounted calldomain.MountedCall) (uint64, bool) {
		bound, boundOK := hot.siteForMounted(mounted)
		key, keyOK := bound.callKey()
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), boundOK && keyOK && indexOK && index >= 0
	})
	if !ok {
		return nil, false
	}
	hot.read = runtimeRead
	hot.implementation = implementation
	return hot, true
}

// resolveOperand reads Call's owner-fenced occurrence inverse directly. The
// site validation joins only sealed owner rows and rejects a foreign mount,
// occurrence, Heap, Pack, Value schema, or Call algebra before publication.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (calldomain.MountedCall, bool) {
	if !rule.valid() {
		return calldomain.MountedCall{}, false
	}
	mounted, ok := rule.calls.Algebra().MountedCallForOccurrence(coords.Mount, coords.Occurrence)
	if !ok {
		return calldomain.MountedCall{}, false
	}
	_, siteOK := rule.siteForMounted(mounted)
	return mounted, siteOK
}

// siteForMounted joins the canonical owner rows needed by dispatch. The site
// is ephemeral: it is neither retained by HotRule nor published as a second
// mounted-call directory.
func (rule *HotRule) siteForMounted(mounted calldomain.MountedCall) (site, bool) {
	if !rule.valid() {
		return site{}, false
	}
	algebra := rule.calls.Algebra()
	applicationID, occurrenceID, moduleID, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	canonical, occurrenceOK := algebra.MountedCallForOccurrence(moduleID, occurrenceID)
	if !identityOK || !occurrenceOK || canonical != mounted || !applicationID.Available() || !moduleID.Available() || !occurrenceID.Available() || !algebra.OwnsMountedModule(moduleID) {
		return site{}, false
	}
	bound, ok := newSite(algebra, rule.values.Schema(), rule.heaps, rule.packs, applicationID)
	return bound, ok && bound.mounted == mounted && bound.matchesSchemas(rule.heaps, rule.packs)
}

func (rule *HotRule) operandContent(mounted calldomain.MountedCall) (calldomain.MountedCall, [32]byte, bool) {
	bound, ok := rule.siteForMounted(mounted)
	id, idOK := bound.contentID()
	if !ok || !idOK || !id.Available() {
		return calldomain.MountedCall{}, [32]byte{}, false
	}
	return mounted, [32]byte(id), true
}

func (rule *HotRule) valid() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.values == nil || !rule.values.MatchesBinding(rule.binding) || rule.calls == nil || !rule.calls.MatchesBinding(rule.binding) || rule.values.Schema() == nil || rule.calls.Algebra() == nil || !rule.calls.Algebra().Valid() || !rule.heaps.Valid() || rule.packs == nil {
		return false
	}
	linkOwner := rule.calls.Algebra().LinkOwner()
	return linkOwner.Available() && rule.values.Schema().LinkOwner().Matches(linkOwner) && rule.heaps.LinkOwner().Matches(linkOwner) && rule.packs.LinkOwner().Matches(linkOwner) && rule.values.Schema().OwnsHeapSchema(rule.heaps)
}
