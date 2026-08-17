package index

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

func (rule *RawSetRule) check(semantic identity.SemanticKey) engine.RuleDerivationChecker[heapdomain.Value, Access] {
	return func(derivation engine.RuleDerivation[heapdomain.Value, Access]) (engine.RuleEvidence, bool) {
		// ReadCount is the number of completed product observations, not the
		// number of declared reads: staged selector routes can contribute
		// additional dynamic observations. The exact receiver proof below is
		// the required lower bound; the four staged selectors are authenticated
		// through their Selection accessors.
		if rule == nil || derivation.Rule() != semantic || derivation.InputCount() != 3 || derivation.ReadCount() < 1 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		id, idOK := operand.ID()
		receiverCoordinate, receiverOK := operand.Receiver()
		descriptor, descriptorOK := rule.payloadForWrite(operand)
		if !operandOK || !idOK || !rule.owns(operand) || !receiverOK || !descriptorOK ||
			!derivation.OperandContentMatches([32]byte(id)) || rule.runtime == nil || rule.runtime.valueReadRef == nil || !rule.runtime.valueReadRef(derivation, rule.receiver, receiverCoordinate) {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.InputCount(); index++ {
			if _, ok := derivation.InputAt(index); !ok {
				return engine.RuleEvidence{}, false
			}
		}
		for index := 0; index < derivation.InputCount(); index++ {
			input, ok := derivation.InputAt(index)
			if !ok || input.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			_, transformed := disposition.CarryTransform()
			if transformed || disposition.TransformOnly() {
				return engine.RuleEvidence{}, false
			}
			receiverCells, receiverCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.receiver)
			if !receiverCellsOK || receiverCells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			_, receiverPresent, receiverAvailable := receiverCells.At(0)
			if !receiverAvailable {
				return engine.RuleEvidence{}, false
			}
			heapCount, heapCountOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.heapRead)
			if !heapCountOK {
				return engine.RuleEvidence{}, false
			}
			if heapCount == 0 {
				if !derivationNoRouteShape(rule, derivation, disposition, operand, receiverPresent) || disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.OutputCount() != 0 || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			if !receiverPresent || disposition.Kind() != engine.RuleDispositionStaged || disposition.OutputCount() != heapCount || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			scratch := rule.takeSetScratch()
			view, viewOK := derivationRawSetView(rule, derivation, disposition, operand, scratch)
			if !viewOK || !rawSetSelectionShape(operand, descriptor.descriptor, view) {
				rule.putSetScratch(scratch)
				return engine.RuleEvidence{}, false
			}
			for ordinal := 0; ordinal < heapCount; ordinal++ {
				output, outputOK := disposition.OutputAt(ordinal)
				tag, cells, routed := engine.DerivationDispositionRouteValue(derivation, disposition, rule.heapRead, output)
				if !outputOK || !routed || cells.Count() != 1 || !routeTargetMatches(rule, operand, receiverValue(receiverCells), tag, output.Target()) {
					rule.putSetScratch(scratch)
					return engine.RuleEvidence{}, false
				}
				fact, present, available := cells.At(0)
				if !available {
					rule.putSetScratch(scratch)
					return engine.RuleEvidence{}, false
				}
				expected := rule.heapSchema().Bottom()
				if present {
					var expectedOK bool
					expected, expectedOK = rule.mutateRoute(operand, tag, fact, view)
					if !expectedOK {
						rule.putSetScratch(scratch)
						return engine.RuleEvidence{}, false
					}
				}
				if !heapdomain.Equal(expected, output.Value()) {
					rule.putSetScratch(scratch)
					return engine.RuleEvidence{}, false
				}
			}
			rule.putSetScratch(scratch)
		}
		return derivation.Accept()
	}
}

func receiverValue(cells engine.OrderedCells[valuedomain.Value]) valuedomain.Value {
	value, _, _ := cells.At(0)
	return value
}

func derivationNoRouteShape(
	rule *RawSetRule, derivation engine.RuleDerivation[heapdomain.Value, Access], disposition engine.RuleDisposition[heapdomain.Value], operand Access, receiverPresent bool,
) bool {
	keyCount, keyOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.key)
	packCount, packOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.packRead)
	sourceCount, sourceOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.source)
	if !keyOK || !packOK || !sourceOK || packCount != 0 || sourceCount != 0 {
		return false
	}
	if _, dynamic := operand.DynamicKey(); dynamic {
		if receiverPresent {
			return keyCount == 1 && derivationDynamicKeyMatches(rule, derivation, disposition, operand)
		}
		return keyCount == 0
	}
	return keyCount == 0
}

func derivationDynamicKeyMatches(
	rule *RawSetRule,
	derivation engine.RuleDerivation[heapdomain.Value, Access],
	disposition engine.RuleDisposition[heapdomain.Value],
	operand Access,
) bool {
	coordinate, coordinateOK := operand.DynamicKey()
	if !coordinateOK {
		return false
	}
	selected := derivationSelectionValue(derivation, disposition, rule.key, nil, uint64(1), func(ordinal int) bool {
		return rule.runtime != nil && rule.runtime.valueSelectionRef != nil && rule.runtime.valueSelectionRef(derivation, disposition, rule.key, ordinal, coordinate)
	})
	// The no-route disposition is still authenticated by the exact dynamic-key
	// selection. A selected-but-absent cell is lawful only because this caller
	// already proved Heap, Pack, and source selections are all empty.
	return selected.valid && selected.found
}

func derivationRawSetView(
	rule *RawSetRule,
	derivation engine.RuleDerivation[heapdomain.Value, Access],
	disposition engine.RuleDisposition[heapdomain.Value],
	operand Access,
	scratch *rawSetScratch,
) (rawSetView, bool) {
	if rule == nil || scratch == nil {
		return rawSetView{}, false
	}
	view := rawSetView{}
	var ok bool
	view.keyCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.key)
	if !ok {
		return rawSetView{}, false
	}
	view.heapCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.heapRead)
	if !ok {
		return rawSetView{}, false
	}
	view.packCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.packRead)
	if !ok {
		return rawSetView{}, false
	}
	view.sourceCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.source)
	if !ok {
		return rawSetView{}, false
	}
	if !buildDerivationIndex(derivation, disposition, rule.packRead, view.packCount, &scratch.pack) ||
		!buildDerivationIndex(derivation, disposition, rule.source, view.sourceCount, &scratch.source) {
		return rawSetView{}, false
	}
	if _, dynamic := operand.DynamicKey(); dynamic && view.keyCount == 1 {
		coordinate, coordinateOK := operand.DynamicKey()
		if !coordinateOK || rule.runtime == nil || rule.runtime.valueSelectionRef == nil {
			return rawSetView{}, false
		}
		view.key = derivationSelectionValue(derivation, disposition, rule.key, nil, uint64(1), func(ordinal int) bool {
			return rule.runtime.valueSelectionRef(derivation, disposition, rule.key, ordinal, coordinate)
		})
		if !view.key.valid || !view.key.found {
			return rawSetView{}, false
		}
	}
	descriptor, descriptorOK := rule.payloadForWrite(operand)
	if !descriptorOK {
		return rawSetView{}, false
	}
	view.pack = func(tag heapdomain.RawPayloadTag) rawSelected[pack.Value] {
		payload := descriptor
		payloadOK := payload.tag == tag
		root, rootOK := payload.descriptor.payload.Root()
		if !payloadOK || payload.descriptor.kind != rawPayloadTail || !rootOK || rule.runtime == nil || rule.runtime.packSelectionRef == nil {
			return rawSelected[pack.Value]{}
		}
		return derivationSelectionValue(derivation, disposition, rule.packRead, &scratch.pack, tag, func(ordinal int) bool {
			return rule.runtime.packSelectionRef(derivation, disposition, rule.packRead, ordinal, root)
		})
	}
	view.source = func(tag rawSourceTag) rawSelected[valuedomain.Value] {
		source, sourceOK := sourceAt(rule.sourcesFor(operand), tag)
		if !sourceOK || rule.runtime == nil || rule.runtime.sourceSelectionRef == nil {
			return rawSelected[valuedomain.Value]{}
		}
		return derivationSelectionValue(derivation, disposition, rule.source, &scratch.source, tag, func(ordinal int) bool {
			return rule.runtime.sourceSelectionRef(derivation, disposition, rule.source, ordinal, source.coordinate)
		})
	}
	return view, true
}

func routeTargetMatches(rule *RawSetRule, operand Access, receiver valuedomain.Value, tag heapdomain.RawRouteTag, target engine.RuleTarget) bool {
	if rule == nil || tag == 0 || !rule.owns(operand) {
		return false
	}
	if rule.topology == nil || !operand.valid() || operand.topology != rule.topology {
		return false
	}
	routeIndex, found := rule.topology.staticByTag[tag]
	if !found || routeIndex == 0 || uint64(routeIndex) > uint64(len(rule.topology.static)) {
		return false
	}
	route := rule.topology.static[routeIndex-1]
	return route.tag == tag && rule.heapTarget(target, route.key)
}
