package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// HotRule is Heap bootstrap's receipt-native zero-input rule. Root already
// contains the complete immutable Heap value issued at cold construction, so
// callbacks perform only exact owner/receipt checks.
type HotRule struct {
	implementation *heapowner.RuleImplementation[Root]
	owner          *heapowner.HotOwner
	catalog        *Catalog
}

// BindHot attaches the exact write implementation without reopening Host,
// Target, Link, or the BootEntry denominator.
func BindHot(fragment *SchemaFragment, owner *heapowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.Schema().Valid() ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	schema := owner.Schema()
	implementation, ok := heapowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[heapdomain.Value, Root]{
		OperandContent: func(root Root) (Root, [32]byte, bool) {
			return contentForSchema(schema, root)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotBootstrapChecker(owner, fragment.semantic)),
		Transfer: func(access engine.Access[heapdomain.Value, Root]) bool {
			root, operandOK := engine.Operand(access)
			_, value, resultOK := resultForSchema(schema, root)
			if !operandOK || !resultOK {
				return false
			}
			rows := 0
			complete := engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, value)
			})
			return complete && rows == 1
		},
	}, func(root Root) (uint64, bool) {
		key, _, ok := resultForSchema(schema, root)
		index, indexOK := schema.KeyIndex(key)
		return uint64(index), ok && indexOK && index >= 0
	})
	if !ok || implementation == nil {
		return nil, false
	}
	catalog, catalogOK := SealCatalog(schema)
	if !catalogOK {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, owner: owner, catalog: catalog}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (Root, bool) {
	if rule == nil || rule.catalog == nil {
		return Root{}, false
	}
	root, _, ok := rule.catalog.ReceiptForID(coords.Occurrence)
	return root, ok
}

// Catalog returns Heap/bootstrap's immutable Link-global BootRoot directory.
func (rule *HotRule) Catalog() *Catalog {
	if rule == nil {
		return nil
	}
	return rule.catalog
}

func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[Root], bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func hotBootstrapChecker(owner *heapowner.HotOwner, semantic identity.SemanticKey) engine.RuleDerivationChecker[heapdomain.Value, Root] {
	return func(derivation engine.RuleDerivation[heapdomain.Value, Root]) (engine.RuleEvidence, bool) {
		if owner == nil || !owner.Schema().Valid() || derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		root, operandOK := derivation.Operand()
		id, idOK := root.ID()
		key, expected, resultOK := resultForSchema(owner.Schema(), root)
		disposition, dispositionOK := derivation.DispositionAt(0)
		if !operandOK || !idOK || !resultOK || !derivation.OperandContentMatches([32]byte(id)) || !dispositionOK ||
			disposition.Kind() != engine.RuleDispositionStaged || disposition.Guard().Empty() || disposition.TargetCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		actual, valueOK := disposition.Value()
		if !targetOK || !valueOK || !owner.TargetMatches(target, key) || !owner.Schema().Domain().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
