package source

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value Source's package-owned Link-local implementation. It
// retains only Value owner's opaque receipt issuer; the shared binding,
// Factor slot, and private Value coordinate remain owned by value/owner.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.SourceSeed]
	owner          *valueowner.HotOwner
}

// BindHot installs the zero-input/exact-write SourceSeed implementation for
// this exact cold fragment. Source owns operand identity, derivation checking,
// and transfer semantics. Value owner alone binds the output Factor.
func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		owner.Schema().SourceSeedMountCount() == 0 ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	implementation, ok := valueowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[value.Value, value.SourceSeed]{
		OperandContent: sourceSeedContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, hotSourceChecker(owner, fragment.semantic)),
		Transfer: func(access engine.Access[value.Value, value.SourceSeed]) bool {
			seed, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			_, fact, resultOK := sourceResultForSchema(owner.Schema(), seed)
			if !resultOK {
				return false
			}
			rows := 0
			completed := engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, fact)
			})
			return completed && rows == 1
		},
	})
	if !ok || implementation == nil {
		return nil, false
	}
	return &HotRule{implementation: implementation, owner: owner}, true
}

// AttachMountedRule admits one complete ValueSource row before topology
// commit. Every operand and output surface is issued by this exact Value
// owner; AddRuleFromDraft seals the row atomically under the mounted witness.
func (rule *HotRule) AttachMountedRule(assembly *engine.ReceiptAssembly, mountID, pointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	issuer, mountOK := rule.ForMount(mountID)
	seed, seedOK := issuer.ReceiptForOccurrence(occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return engine.BindingRuleRowRef{}, false
	}
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(capability, mountID, pointID, occurrenceID)
	coordinate, _, resultOK := sourceResultForSchema(rule.owner.Schema(), seed)
	ref, refOK := rule.owner.Ref(coordinate)
	if !mountOK || !seedOK || !implementationOK || !occurrenceOK || !resultOK || !refOK {
		return engine.BindingRuleRowRef{}, false
	}
	admit := func(transaction *engine.RuleSourceTransaction) bool {
		return engine.AddExactWrite(transaction, ref)
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
	queued := engine.AdmitMountedRule(assembly, implementation, capability, occurrence, seed, admit, issue)
	return engine.BindingRuleRowRef{}, queued
}

// BeginReceiptCompilation starts the opaque graph attachment transaction for
// this exact Value/source issuer.
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

// AttachReceiptMember attaches one graph-owned source member using the exact
// owner-fenced SourceSeed operand.
func (rule *HotRule) AttachReceiptMember(compilation *engine.ReceiptCompilation, member engine.ReceiptRuleMember, seed value.SourceSeed) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, seed)
}

// AttachMountedReceiptMember resolves the graph-owned mounted member and the
// exact preissued SourceSeed internally, then delegates to AttachReceiptMember.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, pointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || graph == nil {
		return nil, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return nil, false
	}
	member, memberOK := graph.MountedRuleMember(capability, mountID, pointID, occurrenceID)
	issuer, issuerOK := rule.ForMount(mountID)
	seed, seedOK := issuer.ReceiptForOccurrence(occurrenceID)
	if !memberOK || !issuerOK || !seedOK {
		return nil, false
	}
	return rule.AttachReceiptMember(compilation, member, seed)
}

// MountedIssuer is Source's exact substitution authority for one concrete
// Project mount.  Artifact occurrence IDs are intentionally only meaningful
// beneath this issuer: equivalent Program mounts may reuse them.
type MountedIssuer struct {
	rule  *HotRule
	mount value.SourceSeedMount
}

// ForMount returns the mounted occurrence issuer for one exact ModuleKey.
func (rule *HotRule) ForMount(module identity.ContentID) (MountedIssuer, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil || !module.Available() {
		return MountedIssuer{}, false
	}
	mount, ok := rule.owner.Schema().SourceSeedMountForModule(module)
	return MountedIssuer{rule: rule, mount: mount}, ok
}

// ReceiptForOccurrence returns the preissued SourceSeed for one artifact
// ValueSource row. It is a direct mounted map lookup; no Program term, Flow
// traversal, or Link inverse is reconstructed on the hot path.
func (issuer MountedIssuer) ReceiptForOccurrence(id identity.ContentID) (value.SourceSeed, bool) {
	if issuer.rule == nil || issuer.rule.owner == nil || issuer.rule.owner.Schema() == nil || !id.Available() {
		return value.SourceSeed{}, false
	}
	occurrence, ok := issuer.mount.OccurrenceForID(id)
	seed, seedOK := occurrence.Seed()
	_, _, valid := sourceResultForSchema(issuer.rule.owner.Schema(), seed)
	return seed, ok && seedOK && valid
}

// ModuleID returns the exact mounted substitution identity.
func (issuer MountedIssuer) ModuleID() identity.ContentID {
	if issuer.rule == nil {
		return identity.ContentID{}
	}
	return issuer.mount.ModuleID()
}

// Implementation returns Value owner's opaque issuer only after the shared
// SchemaBinding can resolve its exact receipt. No engine RuleImplementation,
// private coordinate, slot, or binding escapes this package boundary.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.SourceSeed], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	if !ok {
		return nil, false
	}
	return rule.implementation, true
}

func hotSourceChecker(owner *valueowner.HotOwner, ruleSemantic identity.SemanticKey) engine.RuleDerivationChecker[value.Value, value.SourceSeed] {
	return func(derivation engine.RuleDerivation[value.Value, value.SourceSeed]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || derivation.Rule() != ruleSemantic || derivation.InputCount() != 0 ||
			derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		seed, ok := derivation.Operand()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		id, idOK := seed.ID()
		if !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		coordinate, expected, resultOK := sourceResultForSchema(owner.Schema(), seed)
		ref, refOK := owner.Ref(coordinate)
		if !targetOK || !resultOK || !refOK || !engine.TargetMatchesRef(target, ref) {
			return engine.RuleEvidence{}, false
		}
		actual, valueOK := disposition.Value()
		if !valueOK || !owner.Schema().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
