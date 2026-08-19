package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value/allocation's receipt-native transformed-carry Rule issuer.
// It retains only Value owner's typed issuer; operand provenance and transform
// callbacks remain package-private.
type HotRule struct {
	implementation *valueowner.RuleImplementation[operand]
	catalog        *allocationcatalog.Catalog
	owner          *valueowner.HotOwner
}

// BindHot binds the exact allocation fragment through Value's one-carry
// receipt lane. No legacy Rule or generic slot fallback is available.
func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner, heap heapdomain.Schema, catalog *allocationcatalog.Catalog) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		catalog == nil || !catalog.FencedTo(heap, owner.Schema()) ||
		!fragment.semantic.Available() || !fragment.transform.Available() || !fragment.evidence.Available() ||
		!identity.DistinctKeys(fragment.semantic, fragment.transform, fragment.evidence) {
		return nil, false
	}
	schema := owner.Schema()
	implementation, ok := valueowner.BindCarryRule(owner, fragment.slot, fragment.carry, fragment.write, engine.HotRuleSpec[value.Value, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return allocationOperandContentForSchema(schema, candidate)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotAllocationChecker(owner, fragment.semantic, fragment.transform)),
		Transfer: func(access engine.Access[value.Value, operand]) bool {
			allocation, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			if allocation.result == nil {
				return false
			}
			fresh, freshOK := allocation.result.Fresh()
			return freshOK && engine.Product(access, func(row engine.Row) bool {
				return engine.StageValue(access, row, fresh)
			})
		},
	}, engine.HotCarrySpec[value.Value, operand]{
		Apply: func(allocation operand, prior value.Value) (value.Value, bool) {
			if allocation.result == nil {
				return value.Value{}, false
			}
			return allocation.result.Age(prior)
		},
	}, func(allocation operand) (uint64, bool) {
		index, indexOK := schema.CoordinateIndex(allocation.coordinate)
		return uint64(index), indexOK
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, catalog: catalog, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	issuer, ok := rule.ForMount(coords.Mount)
	if !ok {
		return operand{}, false
	}
	return issuer.ReceiptForOccurrence(coords.Occurrence)
}

// MountedIssuer is Value allocation's exact mount-scoped operand issuer.
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

// ReceiptForOccurrence returns the exact presealed Value allocation operand.
func (issuer MountedIssuer) ReceiptForOccurrence(id identity.ContentID) (operand, bool) {
	if issuer.rule == nil || issuer.rule.owner == nil || issuer.rule.owner.Schema() == nil || !issuer.mount.OwnedBy(issuer.rule.catalog) {
		return operand{}, false
	}
	key, ok := issuer.mount.KeyForOccurrence(id)
	if !ok {
		return operand{}, false
	}
	return allocationOperandForSchema(issuer.rule.owner.Schema(), key)
}

// Implementation returns the typed pending issuer until the shared binding
// seals, after which Value owner resolves it to the exact engine receipt.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

func (rule *HotRule) ProgramAttach() (engine.RuleProgramAttach, bool) {
	return valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
}

func hotAllocationChecker(owner *valueowner.HotOwner, ruleSemantic, transformSemantic identity.SemanticKey) engine.RuleDerivationChecker[value.Value, operand] {
	return func(derivation engine.RuleDerivation[value.Value, operand]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || derivation.Rule() != ruleSemantic || derivation.InputCount() != 1 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		if _, ok := derivation.InputAt(0); !ok {
			return engine.RuleEvidence{}, false
		}
		allocation, operandOK := derivation.Operand()
		canonical, _, contentOK := allocationOperandContentForSchema(owner.Schema(), allocation)
		if !operandOK || !contentOK || !derivation.OperandContentMatches(canonical.digest) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		semantic, transformed := disposition.CarryTransform()
		if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TransformOnly() ||
			!transformed || semantic != transformSemantic || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		actual, valueOK := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		ref, refOK := owner.Ref(canonical.coordinate)
		if !valueOK || !targetOK || !refOK || !owner.Schema().Equal(actual, canonical.fresh) || !engine.TargetMatchesRef(target, ref) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
