package closed

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Heap closed allocation's typed receipt-native Rule issuer. The
// operand is constructor-issued source.Closed; hot callbacks never invoke
// NewClosed, FieldOrigin, or any Link/Flow topology query.
type HotRule struct {
	implementation *heapowner.RuleImplementation[source.Closed]
	catalog        *allocationcatalog.Catalog
	heapOwner      *heapowner.HotOwner
	heap           heapdomain.Schema
	values         *valuedomain.Schema
	valueOwner     *valueowner.HotOwner
}

// Implementation resolves the typed receipt only after SchemaBinding seals.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Closed], bool) {
	if rule == nil || rule.heapOwner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.heapOwner, rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) ProgramAttach() (engine.RuleProgramAttach, bool) {
	return heapowner.ResolveRuleImplementationFor(rule.heapOwner, rule.implementation)
}

// BindHot binds the exact heterogeneous Heap/Value read surface, ordinary
// carry, and exact Heap write through the two owner-issued FactorRefs.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, heapOwner *heapowner.HotOwner, valueOwner *valueowner.HotOwner, catalog *allocationcatalog.Catalog) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || heapOwner == nil || !heapOwner.MatchesBinding(binding) || valueOwner == nil || !valueOwner.MatchesBinding(binding) || valueOwner.Schema() == nil || !heapOwner.Schema().Valid() ||
		catalog == nil || !catalog.FencedToSummaryOwner(heapOwner.Schema(), valueOwner.Schema(), valueOwner) ||
		fragment.valueSummary.Kind() != engine.SchemaFormReadSummary {
		return nil, false
	}
	heapSchema, values := heapOwner.Schema(), valueOwner.Schema()
	var runtimeHeapRead engine.Read[engine.OrderedCells[heapdomain.Value]]
	var runtimeValueRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	implementation, runtimeHeapRead, runtimeValueRead, ok := heapowner.BindExactAndSummaryReadAndCarry[source.Closed, heapdomain.Value, valuedomain.Value, engine.OrderedCells[valuedomain.Value]](
		heapOwner, fragment.slot, fragment.heapRead, heapOwner.FactorRef(), fragment.valueRead, valueOwner.FactorRef(), fragment.valueSummary,
		fragment.carry, fragment.write, engine.HotRuleSpec[heapdomain.Value, source.Closed]{
			OperandContent: func(candidate source.Closed) (source.Closed, [32]byte, bool) {
				return hotClosedContent(heapSchema, values, valueOwner, candidate)
			},
			Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotClosedChecker(heapOwner, valueOwner, fragment.transform, fragment.semantic, &runtimeHeapRead, &runtimeValueRead)),
			Transfer: func(access engine.Access[heapdomain.Value, source.Closed]) bool {
				operand, operandOK := engine.Operand(access)
				if !operandOK || !operand.FencedTo(heapSchema, values) {
					return false
				}
				return engine.Product(access, func(row engine.Row) bool {
					heapCells, heapOK := engine.ReadValue(access, row, runtimeHeapRead)
					valueCells, valueOK := engine.ReadValue(access, row, runtimeValueRead)
					if !heapOK || !valueOK || heapCells.Count() != 1 {
						return false
					}
					predecessor, present, available := heapCells.At(0)
					if !available {
						return false
					}
					if !present {
						return engine.NoCandidate(access, row)
					}
					next, normal, resultOK := resultClosed(heapSchema, values, operand, predecessor, valueCells)
					if !resultOK || !normal {
						return engine.NoCandidate(access, row)
					}
					return engine.StageValue(access, row, next)
				})
			},
		}, engine.HotCarrySpec[heapdomain.Value, source.Closed]{
			Apply: func(operand source.Closed, prior heapdomain.Value) (heapdomain.Value, bool) {
				if !operand.FencedTo(heapSchema, values) {
					return heapdomain.Value{}, false
				}
				return heapSchema.Age(prior, operand.Key())
			},
		}, func(operand source.Closed) (uint64, bool) {
			index, ok := heapSchema.KeyIndex(operand.Key())
			return uint64(index), ok && index >= 0
		}, func(operand source.Closed) (uint64, bool) {
			index, ok := heapSchema.KeyIndex(operand.Key())
			return uint64(index), ok && index >= 0
		}, valueowner.SummarySurfaceAdmit(valueOwner, func(operand source.Closed) valueowner.SummaryReceipt {
			return operand.SummaryReceipt()
		}))
	if !ok || implementation == nil {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, catalog: catalog, heapOwner: heapOwner, heap: heapSchema, values: values, valueOwner: valueOwner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (source.Closed, bool) {
	issuer, ok := rule.ForMount(coords.Mount)
	if !ok {
		return source.Closed{}, false
	}
	return issuer.ReceiptForOccurrence(coords.Occurrence)
}

type MountedIssuer struct {
	rule  *HotRule
	mount allocationcatalog.Mount
}

func (rule *HotRule) ForMount(module identity.ContentID) (MountedIssuer, bool) {
	if rule == nil || rule.catalog == nil {
		return MountedIssuer{}, false
	}
	mount, ok := rule.catalog.ForMount(module)
	return MountedIssuer{rule: rule, mount: mount}, ok && mount.OwnedBy(rule.catalog)
}

func (issuer MountedIssuer) ReceiptForOccurrence(id identity.ContentID) (source.Closed, bool) {
	if issuer.rule == nil || !issuer.mount.OwnedBy(issuer.rule.catalog) {
		return source.Closed{}, false
	}
	closed, ok := issuer.mount.ClosedForOccurrence(id)
	return closed, ok && closed.FencedTo(issuer.rule.heap, issuer.rule.values) && closed.SummaryReceipt().IssuedBy(issuer.rule.summaryOwner())
}

func (rule *HotRule) summaryOwner() *valueowner.HotOwner {
	if rule == nil || rule.catalog == nil {
		return nil
	}
	// The catalog exact-fences this same owner during BindHot; the typed
	// implementation callback retains it as well. This helper keeps the
	// mounted issuance check explicit without exposing catalog internals.
	return rule.valueOwner
}

func hotClosedContent(heapSchema heapdomain.Schema, values *valuedomain.Schema, valueOwner *valueowner.HotOwner, candidate source.Closed) (source.Closed, [32]byte, bool) {
	if !candidate.FencedTo(heapSchema, values) {
		return source.Closed{}, [32]byte{}, false
	}
	receipt := candidate.SummaryReceipt()
	if receipt.Width() != candidate.CoordinateCount() || !receipt.IssuedBy(valueOwner) {
		return source.Closed{}, [32]byte{}, false
	}
	id, ok := candidate.ID()
	if !ok || [32]byte(id) == ([32]byte{}) {
		return source.Closed{}, [32]byte{}, false
	}
	return candidate, [32]byte(id), true
}

func hotClosedChecker(heapOwner *heapowner.HotOwner, valueOwner *valueowner.HotOwner, transform, semantic identity.SemanticKey, heapRead *engine.Read[engine.OrderedCells[heapdomain.Value]], valueRead *engine.Read[engine.OrderedCells[valuedomain.Value]]) engine.RuleDerivationChecker[heapdomain.Value, source.Closed] {
	return func(derivation engine.RuleDerivation[heapdomain.Value, source.Closed]) (engine.RuleEvidence, bool) {
		if heapOwner == nil || valueOwner == nil || heapRead == nil || valueRead == nil || !heapOwner.Schema().Valid() || valueOwner.Schema() == nil || derivation.Rule() != semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 2 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		heapSchema, values := heapOwner.Schema(), valueOwner.Schema()
		id, idOK := operand.ID()
		ref, refOK := heapOwner.Ref(operand.Key())
		if !operandOK || !operand.FencedTo(heapSchema, values) || !idOK || !derivation.OperandContentMatches([32]byte(id)) || !refOK {
			return engine.RuleEvidence{}, false
		}
		input, inputOK := derivation.InputAt(0)
		if !inputOK || input.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		if !heapowner.ReadMatches(heapOwner, derivation, *heapRead, operand.Key()) || !valueowner.MatchSummaryReceipt(valueOwner, operand.SummaryReceipt(), derivation, *valueRead) {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			predecessorCells, predecessorOK := engine.DerivationDispositionReadValue(derivation, disposition, *heapRead)
			inputs, inputsOK := engine.DerivationDispositionReadValue(derivation, disposition, *valueRead)
			if !predecessorOK || !inputsOK || predecessorCells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			predecessor, present, available := predecessorCells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !present {
				_, transformed := disposition.CarryTransform()
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 || disposition.TransformOnly() || transformed {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			next, normal, resultOK := resultClosed(heapSchema, values, operand, predecessor, inputs)
			if !resultOK {
				return engine.RuleEvidence{}, false
			}
			if !normal {
				_, transformed := disposition.CarryTransform()
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 || disposition.TransformOnly() || transformed {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			actual, actualOK := disposition.Value()
			target, targetOK := disposition.TargetAt(0)
			actualTransform, transformed := disposition.CarryTransform()
			if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !actualOK || !targetOK || disposition.TransformOnly() || !transformed || actualTransform != transform || !engine.TargetMatchesRef(target, ref) || !heapSchema.Domain().Equal(actual, next) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
