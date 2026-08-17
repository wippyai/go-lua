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
	})
	if !ok || implementation == nil {
		return nil, false
	}
	catalog, catalogOK := SealCatalog(schema)
	if !catalogOK {
		return nil, false
	}
	return &HotRule{implementation: implementation, owner: owner, catalog: catalog}, true
}

// Catalog returns Heap/bootstrap's immutable Link-global BootRoot directory.
func (rule *HotRule) Catalog() *Catalog {
	if rule == nil {
		return nil
	}
	return rule.catalog
}

// AttachLinkOccurrence lowers one Link-global HeapBootstrap row. The exact
// bootstrap witness supplied to ReceiptAssembly owns the point/catalog; this
// package contributes only its matching preissued Root and Heap write Ref.
func (rule *HotRule) AttachLinkOccurrence(assembly *engine.ReceiptAssembly, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || rule.catalog == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	root, key, operandOK := rule.catalog.ReceiptForID(occurrenceID)
	implementation, implementationOK := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	write, writeOK := rule.owner.Ref(key)
	if !operandOK || !implementationOK || !writeOK {
		return engine.BindingRuleRowRef{}, false
	}
	capability := linkCapability(rule.implementation)
	occurrence, occurrenceOK := assembly.AdmitLinkRuleOccurrence(capability, occurrenceID)
	if !occurrenceOK {
		return engine.BindingRuleRowRef{}, false
	}
	admit := func(transaction *engine.RuleSourceTransaction) bool {
		return engine.AddExactWrite(transaction, write)
	}
	issue := func(source engine.RuleSurfaceSourceReceipt) bool {
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		writePart, writePartOK := implementation.ReceiptWritePart(source, 0)
		if !draftOK || !writePartOK || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	}
	queued := engine.AdmitLinkRule(assembly, implementation, capability, occurrence, root, admit, issue)
	return engine.BindingRuleRowRef{}, queued
}

// AttachLinkReceiptMember resolves the committed Link-global member and its
// private Root internally. It deliberately has no mount or reusable-point
// argument: HeapBootstrap is emitted once for the whole Link.
func (rule *HotRule) AttachLinkReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil || rule.catalog == nil || graph == nil {
		return nil, false
	}
	root, _, operandOK := rule.catalog.ReceiptForID(occurrenceID)
	member, memberOK := graph.LinkRuleMember(linkCapability(rule.implementation), occurrenceID)
	implementation, implementationOK := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !operandOK || !memberOK || !implementationOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, root)
}

// Implementation resolves only after the shared binding seals.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[Root], bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
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
