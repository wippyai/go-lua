package ingress

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
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
	})
	if !ok || implementation == nil {
		return nil, false
	}
	return &HotRule{implementation: implementation, owner: owner}, true
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

type MountedIssuer struct {
	rule  *HotRule
	mount allocationcatalog.Mount
}

// ForMount returns HeapIngress's exact mounted allocation issuer.
func (rule *HotRule) ForMount(module identity.ContentID) (MountedIssuer, bool) {
	if rule == nil || rule.catalog == nil {
		return MountedIssuer{}, false
	}
	mount, ok := rule.catalog.ForMount(module)
	return MountedIssuer{rule: rule, mount: mount}, ok && mount.OwnedBy(rule.catalog)
}

// ReceiptForOccurrence reuses the allocation catalog's exact Root receipt;
// no new root or allocation shape is created here.
func (issuer MountedIssuer) ReceiptForOccurrence(id identity.ContentID) (source.Root, bool) {
	if issuer.rule == nil || issuer.rule.owner == nil || issuer.rule.catalog == nil || !issuer.mount.OwnedBy(issuer.rule.catalog) {
		return source.Root{}, false
	}
	root, ok := issuer.mount.RootForOccurrence(id)
	return root, ok && root.FencedTo(issuer.rule.owner.Schema())
}

// AttachMountedOccurrence is the sole HeapIngress artifact attachment bridge.
// The mounted catalog supplies the exact operand and Heap owner supplies the
// write Ref; no factor ordinal or equation surface crosses this boundary.
func (rule *HotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || rule.catalog == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	operand, operandOK := issuer.ReceiptForOccurrence(occurrenceID)
	implementation, implementationOK := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	write, writeOK := rule.owner.Ref(operand.Key())
	if !issuerOK || !operandOK || !implementationOK || !writeOK {
		return engine.BindingRuleRowRef{}, false
	}
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	if !occurrenceOK {
		return engine.BindingRuleRowRef{}, false
	}
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	if !transactionOK || !engine.AddExactWrite(transaction, write) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(mountedCapability(rule.implementation), func() bool {
		sourceReceipt, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(sourceReceipt)
		writePart, writePartOK := implementation.ReceiptWritePart(sourceReceipt, 0)
		if !sourceOK || !draftOK || !writePartOK || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// AttachMountedReceiptMember resolves both the committed graph member and its
// exact mounted Root internally.  The caller never receives the private Heap
// coordinate or the allocation operand used by the runtime implementation.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil || graph == nil {
		return nil, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	operand, operandOK := issuer.ReceiptForOccurrence(occurrenceID)
	member, memberOK := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	implementation, implementationOK := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !issuerOK || !operandOK || !memberOK || !implementationOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}

// Implementation returns Heap owner's opaque rule issuer only after the
// shared binding seals. The engine receipt and Heap coordinate stay private.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Root], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
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

func hotIngressChecker(owner *heapowner.HotOwner, semantic engine.SemanticKey) engine.RuleDerivationChecker[heapdomain.Value, source.Root] {
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
