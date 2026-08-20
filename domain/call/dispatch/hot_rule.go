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
	if binding == nil || fragment == nil || values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) || values.Schema() == nil || calls.Algebra() == nil || !calls.Algebra().Valid() || !heaps.Valid() || packs == nil || !fragment.semantic.Available() || !fragment.evidence.Available() {
		return nil, false
	}
	linkOwner := calls.Algebra().LinkOwner()
	if !linkOwner.Available() || !values.Schema().LinkOwner().Matches(linkOwner) || !heaps.LinkOwner().Matches(linkOwner) || !packs.LinkOwner().Matches(linkOwner) || !values.Schema().OwnsHeapSchema(heaps) {
		return nil, false
	}
	hot := &HotRule{binding: binding, fragment: fragment, values: values, calls: calls, heaps: heaps, packs: packs}
	implementation, runtimeRead, ok := callowner.BindHeterogeneousExactReadRule(calls, fragment.slot, fragment.read, fragment.value, fragment.write, engine.HotRuleSpec[calldomain.Value, calldomain.MountedCall]{
		OperandContent: hot.operandContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, hotDispatchChecker(hot)),
		Transfer: func(access engine.Access[calldomain.Value, calldomain.MountedCall]) bool {
			mounted, mountedOK := engine.Operand(access)
			bound, siteOK := hot.siteForMounted(mounted)
			if !mountedOK || !siteOK {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, readOK := engine.ReadValue(access, row, hot.read)
				if !readOK || cells.Count() != 1 {
					return false
				}
				fact, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				result, resultOK := reduce(bound, fact)
				return resultOK && engine.StageValue(access, row, result)
			})
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
	if !implementation.InstallOperandResolver(hot.resolveOperand) {
		return nil, false
	}
	return hot, true
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || !rule.valid() {
		return engine.ProgramRule{}, false
	}
	implementation, ok := callowner.ResolveHeterogeneousRuleImplementation(rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
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

func hotDispatchChecker(rule *HotRule) engine.RuleDerivationChecker[calldomain.Value, calldomain.MountedCall] {
	return func(derivation engine.RuleDerivation[calldomain.Value, calldomain.MountedCall]) (engine.RuleEvidence, bool) {
		if rule == nil || rule.fragment == nil || derivation.Rule() != rule.fragment.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		mounted, operandOK := derivation.Operand()
		bound, siteOK := rule.siteForMounted(mounted)
		_, digest, contentOK := rule.operandContent(mounted)
		coordinate, coordinateOK := bound.valueCoordinate()
		key, keyOK := bound.callKey()
		if !operandOK || !siteOK || !contentOK || !coordinateOK || !keyOK || !derivation.OperandContentMatches(digest) || !valueowner.ReadMatches(rule.values, derivation, rule.read, coordinate) {
			return engine.RuleEvidence{}, false
		}
		input, inputOK := derivation.InputAt(0)
		if !inputOK || input.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
			if !cellsOK || cells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !present {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			expected, expectedOK := reduce(bound, fact)
			if !expectedOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			target, targetOK := disposition.TargetAt(0)
			ref, refOK := rule.calls.Ref(key)
			actual, actualOK := disposition.Value()
			if !targetOK || !refOK || !engine.TargetMatchesRef(target, ref) || !actualOK || !rule.calls.Algebra().Equal(actual, expected) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
