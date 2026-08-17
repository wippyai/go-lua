package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
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
	})
	if !ok || implementation == nil {
		return nil, false
	}
	catalog, catalogOK := SealCatalog(schema)
	if !catalogOK {
		return nil, false
	}
	return &HotRule{implementation: implementation, catalog: catalog, owner: owner}, true
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

// ReceiptForID returns the exact preissued GlobalBinding for one Host-global
// identity in O(1).
func (rule *HotRule) ReceiptForID(id identity.ContentID) (identity.ContentID, bool) {
	if rule == nil || rule.catalog == nil {
		return identity.ContentID{}, false
	}
	return rule.catalog.ReceiptForID(id)
}

// ReceiptForOccurrence is the narrow attachment spelling used by artifact
// receipt assembly. Value bootstrap IDs are stable Host-global identities.
func (rule *HotRule) ReceiptForOccurrence(id identity.ContentID) (identity.ContentID, bool) {
	return rule.ReceiptForID(id)
}

// AttachLinkOccurrence lowers one Link-global ValueBootstrap row. The exact
// bootstrap witness supplied to ReceiptAssembly owns the occurrence; this
// package contributes only its matching preissued GlobalBinding and write Ref.
func (rule *HotRule) AttachLinkOccurrence(assembly *engine.ReceiptAssembly, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	binding, bindingOK := rule.ReceiptForOccurrence(occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	result, resultOK := globalResultForSchema(rule.owner.Schema(), binding)
	write, writeOK := rule.owner.Ref(result.coordinate)
	if !bindingOK || !implementationOK || !resultOK || !writeOK {
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
	queued := engine.AdmitLinkRule(assembly, implementation, capability, occurrence, binding, admit, issue)
	return engine.BindingRuleRowRef{}, queued
}

// AttachLinkReceiptMember resolves the committed Link-global member and its
// private GlobalBinding internally. ValueBootstrap is emitted once for the
// whole Link and therefore has no mount or reusable-point argument.
func (rule *HotRule) AttachLinkReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil || graph == nil {
		return nil, false
	}
	binding, bindingOK := rule.ReceiptForOccurrence(occurrenceID)
	member, memberOK := graph.LinkRuleMember(linkCapability(rule.implementation), occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !bindingOK || !memberOK || !implementationOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, binding)
}

// BeginReceiptCompilation starts the opaque graph attachment transaction for
// this exact Link-global ValueBootstrap issuer.
func (rule *HotRule) BeginReceiptCompilation(graph *engine.ReceiptGraph) (*engine.ReceiptCompilation, bool) {
	if rule == nil || rule.owner == nil {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.BeginReceiptCompilation(implementation, graph)
}

// AttachReceiptMember attaches one graph-owned global bootstrap member with
// the exact owner-fenced GlobalBinding operand.
func (rule *HotRule) AttachReceiptMember(compilation *engine.ReceiptCompilation, member engine.ReceiptRuleMember, binding identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, binding)
}

// Implementation returns the typed pending issuer until SchemaBinding seals.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[identity.ContentID], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
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
