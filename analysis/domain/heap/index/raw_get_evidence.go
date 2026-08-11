package index

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
)

func (rule *RawGetRule) check(semantic engine.SemanticKey) engine.RuleDerivationChecker[valuedomain.Value, Access] {
	return func(derivation engine.RuleDerivation[valuedomain.Value, Access]) (engine.RuleEvidence, bool) {
		if rule == nil || derivation.Rule() != semantic || derivation.InputCount() != 4 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		id, idOK := operand.ID()
		receiverCoordinate, receiverOK := operand.Receiver()
		resultCoordinate, resultOK := operand.Result()
		receiverRef, receiverRefOK := rule.values.Locate(receiverCoordinate)
		resultRef, resultRefOK := rule.values.Locate(resultCoordinate)
		if !operandOK || !idOK || !rule.owns(operand) || !receiverOK || !resultOK || !receiverRefOK || !resultRefOK ||
			!derivation.OperandContentMatches([32]byte(id)) || !engine.DerivationReadMatchesRef(derivation, rule.receiver, receiverRef) {
			return engine.RuleEvidence{}, false
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
			receiver, receiverPresent, receiverAvailable := receiverCells.At(0)
			if !receiverAvailable {
				return engine.RuleEvidence{}, false
			}
			if !receiverPresent {
				if !derivationSelectionsEmpty(rule, derivation, disposition) || disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			scratch := rule.takeScratch()
			view, viewOK := derivationRawGetView(rule, derivation, disposition, operand, scratch)
			if !viewOK {
				rule.putScratch(scratch)
				return engine.RuleEvidence{}, false
			}
			expected, contributed, reduced := rule.reduce(operand, receiver, view)
			rule.putScratch(scratch)
			if !reduced {
				return engine.RuleEvidence{}, false
			}
			if !contributed {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			actual, actualOK := disposition.Value()
			target, targetOK := disposition.TargetAt(0)
			if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !actualOK || !targetOK ||
				!engine.TargetMatchesRef(target, resultRef) || !rule.values.Schema().Equal(actual, expected) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}

func derivationSelectionsEmpty(rule *RawGetRule, derivation engine.RuleDerivation[valuedomain.Value, Access], disposition engine.RuleDisposition[valuedomain.Value]) bool {
	keyCount, keyOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.key)
	callCount, callOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.call)
	heapCount, heapOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.heapRead)
	packCount, packOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.packRead)
	sourceCount, sourceOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.sourceRead)
	return keyOK && callOK && heapOK && packOK && sourceOK && keyCount == 0 && callCount == 0 && heapCount == 0 && packCount == 0 && sourceCount == 0
}

func derivationRawGetView(
	rule *RawGetRule,
	derivation engine.RuleDerivation[valuedomain.Value, Access],
	disposition engine.RuleDisposition[valuedomain.Value],
	operand Access,
	scratch *rawGetScratch,
) (rawGetView, bool) {
	if scratch == nil {
		return rawGetView{}, false
	}
	view := rawGetView{scratch: scratch}
	var ok bool
	view.keyCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.key)
	if !ok {
		return rawGetView{}, false
	}
	view.callCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.call)
	if !ok {
		return rawGetView{}, false
	}
	view.heapCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.heapRead)
	if !ok {
		return rawGetView{}, false
	}
	view.packCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.packRead)
	if !ok {
		return rawGetView{}, false
	}
	view.sourceCount, ok = engine.DerivationDispositionSelectionCount(derivation, disposition, rule.sourceRead)
	if !ok {
		return rawGetView{}, false
	}
	if !buildDerivationIndex(derivation, disposition, rule.call, view.callCount, &scratch.call) ||
		!buildDerivationIndex(derivation, disposition, rule.heapRead, view.heapCount, &scratch.heap) ||
		!buildDerivationIndex(derivation, disposition, rule.packRead, view.packCount, &scratch.pack) ||
		!buildDerivationIndex(derivation, disposition, rule.sourceRead, view.sourceCount, &scratch.value) {
		return rawGetView{}, false
	}
	if _, dynamic := operand.DynamicKey(); dynamic {
		coordinate, coordinateOK := operand.DynamicKey()
		ref, refOK := rule.values.Locate(coordinate)
		if !coordinateOK || !refOK {
			return rawGetView{}, false
		}
		view.key = derivationSelectionValue(derivation, disposition, rule.key, nil, uint64(1), func(ordinal int) bool {
			return engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.key, ordinal, ref)
		})
		if !view.key.valid || !view.key.found {
			return rawGetView{}, false
		}
	}
	view.call = func(tag uint64) rawSelected[calldomain.Value] {
		if tag == 0 || tag > uint64(len(rule.topology.freshApps)) {
			return rawSelected[calldomain.Value]{}
		}
		key, keyOK := rule.calls.Algebra().KeyForApplication(rule.topology.freshApps[tag-1].application)
		ref, refOK := rule.calls.Locate(key)
		if !keyOK || !refOK {
			return rawSelected[calldomain.Value]{}
		}
		return derivationSelectionValue(derivation, disposition, rule.call, &scratch.call, tag, func(ordinal int) bool {
			return engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.call, ordinal, ref)
		})
	}
	view.heap = func(tag heapdomain.RawRouteTag, key heapdomain.Key) rawSelected[heapdomain.Value] {
		ref, ok := rule.heap.Locate(key)
		if !ok {
			return rawSelected[heapdomain.Value]{}
		}
		return derivationSelectionValue(derivation, disposition, rule.heapRead, &scratch.heap, tag, func(ordinal int) bool {
			return engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.heapRead, ordinal, ref)
		})
	}
	view.pack = func(tag heapdomain.RawPayloadTag) rawSelected[pack.Value] {
		payload, payloadOK := payloadAt(rule.payloads, tag)
		root, rootOK := payload.payload.Root()
		ref, refOK := rule.packs.Locate(root)
		if !payloadOK || !rootOK || !refOK {
			return rawSelected[pack.Value]{}
		}
		return derivationSelectionValue(derivation, disposition, rule.packRead, &scratch.pack, tag, func(ordinal int) bool {
			return engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.packRead, ordinal, ref)
		})
	}
	view.source = func(tag rawSourceTag) rawSelected[valuedomain.Value] {
		source, sourceOK := sourceAt(rule.sources, tag)
		ref, refOK := rule.values.Locate(source.coordinate)
		if !sourceOK || !refOK {
			return rawSelected[valuedomain.Value]{}
		}
		return derivationSelectionValue(derivation, disposition, rule.sourceRead, &scratch.value, tag, func(ordinal int) bool {
			return engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.sourceRead, ordinal, ref)
		})
	}
	return view, true
}

func derivationSelectionValue[Out any, O any, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](
	derivation engine.RuleDerivation[Out, O], disposition engine.RuleDisposition[Out],
	selection engine.Read[engine.Selection[Tag, engine.OrderedCells[S]]], index *rawSelectionIndex, want Tag, matches func(int) bool,
) rawSelected[S] {
	ordinal := -1
	if index == nil {
		count, ok := engine.DerivationDispositionSelectionCount(derivation, disposition, selection)
		if !ok || count != 1 {
			return rawSelected[S]{}
		}
		ordinal = 0
	} else if found, ok := index.ordinal(uint64(want)); ok {
		ordinal = found
	} else {
		return rawSelected[S]{valid: true}
	}
	tag, cells, selected := engine.DerivationDispositionSelectionAt(derivation, disposition, selection, ordinal)
	if !selected || tag != want || cells.Count() != 1 || matches == nil || !matches(ordinal) {
		return rawSelected[S]{}
	}
	value, present, available := cells.At(0)
	if !available {
		return rawSelected[S]{}
	}
	return rawSelected[S]{value: value, present: present, found: true, valid: true}
}

func buildDerivationIndex[Out any, O any, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](derivation engine.RuleDerivation[Out, O], disposition engine.RuleDisposition[Out], selection engine.Read[engine.Selection[Tag, engine.OrderedCells[S]]], count int, index *rawSelectionIndex) bool {
	return index.build(count, func(ordinal int) (uint64, bool) {
		tag, cells, selected := engine.DerivationDispositionSelectionAt(derivation, disposition, selection, ordinal)
		if !selected || cells.Count() != 1 {
			return 0, false
		}
		_, _, available := cells.At(0)
		return uint64(tag), available
	})
}
