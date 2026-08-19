package ingress

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// HotRule is Heap allocation ingress's typed receipt-native rule vertical.
// It retains only Heap owner's opaque issuer; no legacy Rule, Composition,
// output slot, or private Heap coordinate crosses this package boundary.
type HotRule struct {
	implementation *heapowner.RuleImplementation[source.Root]
	owner          *heapowner.HotOwner
	catalog        *allocationcatalog.Catalog
}

// BindHot installs the zero-input exact-write ingress semantics for the exact
// cold Rule fragment. Heap owner alone attaches the output Factor binding.
func BindHot(fragment *SchemaFragment, owner *heapowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.Schema().Valid() ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	implementation, ok := heapowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[heapdomain.Value, source.Root]{
		// Root.New issued the complete cold classification receipt. Member,
		// transfer, and evidence paths authenticate that receipt in O(1).
		OperandContent: func(operand source.Root) (source.Root, [32]byte, bool) {
			return ingressContent(owner.Schema(), operand)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotIngressChecker(owner, fragment.semantic)),
		Transfer: func(access engine.Access[heapdomain.Value, source.Root]) bool {
			operand, operandOK := engine.Operand(access)
			_, value, resultOK := ingressResult(owner.Schema(), operand)
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
	}, func(operand source.Root) (uint64, bool) {
		index, ok := owner.Schema().KeyIndex(operand.Key())
		return uint64(index), ok && index >= 0
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (source.Root, bool) {
	return rule.ReceiptForOccurrence(coords.Mount, coords.Occurrence)
}

// AttachCatalog attaches the exact Link-local allocation occurrence issuer to
// ingress. It is a cold substitution only; the ingress transfer remains
// receipt-native and never reconstructs an allocation from an artifact row.
func (rule *HotRule) AttachCatalog(catalog *allocationcatalog.Catalog) bool {
	if rule == nil || rule.owner == nil || catalog == nil || !catalog.FencedToHeap(rule.owner.Schema()) {
		return false
	}
	rule.catalog = catalog
	return true
}

// ReceiptForOccurrence reuses the allocation catalog's exact Root receipt for
// one mounted occurrence; no new root or allocation shape is created here.
// The catalog's mount row is the sealed address, so a foreign or unmounted
// module names no row at all.
func (rule *HotRule) ReceiptForOccurrence(module, id identity.ContentID) (source.Root, bool) {
	if rule == nil || rule.owner == nil || rule.catalog == nil {
		return source.Root{}, false
	}
	mount, mountOK := rule.catalog.ForMount(module)
	root, ok := mount.RootForOccurrence(id)
	return root, mountOK && ok && root.FencedTo(rule.owner.Schema())
}

func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Root], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementation(rule.implementation)
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

func ingressContent(schema heapdomain.Schema, operand source.Root) (source.Root, [32]byte, bool) {
	id, ok := operand.ID()
	if !ok || !operand.FencedTo(schema) {
		return source.Root{}, [32]byte{}, false
	}
	return operand, [32]byte(id), true
}

func ingressResult(schema heapdomain.Schema, operand source.Root) (heapdomain.Key, heapdomain.Value, bool) {
	if !schema.Valid() || !operand.FencedTo(schema) {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	value, ok := schema.EmptyObject(operand.Key())
	return operand.Key(), value, ok
}

func hotIngressChecker(owner *heapowner.HotOwner, semantic identity.SemanticKey) engine.RuleDerivationChecker[heapdomain.Value, source.Root] {
	return func(derivation engine.RuleDerivation[heapdomain.Value, source.Root]) (engine.RuleEvidence, bool) {
		if owner == nil || !owner.Schema().Valid() || derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		id, idOK := operand.ID()
		key, expected, resultOK := ingressResult(owner.Schema(), operand)
		ref, refOK := owner.Ref(key)
		disposition, dispositionOK := derivation.DispositionAt(0)
		actual, actualOK := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		if !operandOK || !idOK || !operand.FencedTo(owner.Schema()) || !resultOK || !refOK || !derivation.OperandContentMatches([32]byte(id)) ||
			!dispositionOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.Guard().Empty() || disposition.TargetCount() != 1 ||
			!actualOK || !targetOK || !engine.TargetMatchesRef(target, ref) || !owner.Schema().Domain().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
