package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value/bootstrap's receipt-native GlobalBinding Rule issuer. It
// retains no host mapping or Value coordinate; those remain owner/schema
// authorities consulted by the typed callbacks.
type HotRule struct {
	implementation *valueowner.RuleImplementation[identity.ContentID]
	catalog        *Catalog
	owner          *valueowner.HotOwner
}

// BindHot binds one exact bootstrap fragment to Value's exact Factor owner.
// All malformed binding, host mapping, and target initial-value cases fail at
// the typed callbacks; no legacy declaration path is consulted.
func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || !identity.DistinctKeys(fragment.semantic, fragment.evidence) {
		return nil, false
	}
	schema := owner.Schema()
	implementation, ok := valueowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[value.Value, identity.ContentID]{
		OperandContent: globalContentForSchema(schema),
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, hotBootstrapChecker(owner, fragment.semantic)),
		Transfer: func(access engine.Access[value.Value, identity.ContentID]) bool {
			binding, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			result, resultOK := globalResultForSchema(schema, binding)
			if !resultOK {
				return false
			}
			rows := 0
			return engine.Product(access, func(row engine.Row) bool {
				rows++
				if rows != 1 {
					return false
				}
				if result.absent {
					return engine.NoCandidate(access, row)
				}
				return engine.StageValue(access, row, result.fact)
			}) && rows == 1
		},
	}, func(binding identity.ContentID) (uint64, bool) {
		result, resultOK := globalResultForSchema(schema, binding)
		index, indexOK := schema.CoordinateIndex(result.coordinate)
		return uint64(index), resultOK && indexOK
	})
	if !ok || implementation == nil {
		return nil, false
	}
	catalog, catalogOK := SealCatalog(schema)
	if !catalogOK {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, catalog: catalog, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (identity.ContentID, bool) {
	if rule == nil || rule.catalog == nil {
		return identity.ContentID{}, false
	}
	return rule.catalog.ReceiptForID(coords.Occurrence)
}

// Catalog returns Value/bootstrap's immutable Link-global operand directory.
// It is exposed as an opaque owner-issued substitution authority; no Host
// mapping is reconstructed by consumers.
func (rule *HotRule) Catalog() *Catalog {
	if rule == nil {
		return nil
	}
	return rule.catalog
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[identity.ContentID], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func hotBootstrapChecker(owner *valueowner.HotOwner, ruleSemantic identity.SemanticKey) engine.RuleDerivationChecker[value.Value, identity.ContentID] {
	return func(derivation engine.RuleDerivation[value.Value, identity.ContentID]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || derivation.Rule() != ruleSemantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		binding, operandOK := derivation.Operand()
		canonical, digest, contentOK := globalContentForSchema(owner.Schema())(binding)
		result, resultOK := globalResultForSchema(owner.Schema(), binding)
		if !operandOK || !contentOK || canonical != binding || !resultOK || !derivation.OperandContentMatches(digest) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		if !ok || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
			return engine.RuleEvidence{}, false
		}
		if result.absent {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}
		ref, refOK := owner.Ref(result.coordinate)
		targetRef, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !refOK || !targetOK || !actualOK ||
			!engine.TargetMatchesRef(targetRef, ref) || !owner.Schema().Equal(actual, result.fact) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
