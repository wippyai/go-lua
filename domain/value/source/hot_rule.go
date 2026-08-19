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
	}, func(seed value.SourceSeed) (uint64, bool) {
		coordinate, _, ok := sourceResultForSchema(owner.Schema(), seed)
		index, indexOK := owner.Schema().CoordinateIndex(coordinate)
		return uint64(index), ok && indexOK
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

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.SourceSeed, bool) {
	issuer, ok := rule.ForMount(coords.Mount)
	if !ok {
		return value.SourceSeed{}, false
	}
	return issuer.ReceiptForOccurrence(coords.Occurrence)
}

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

func (rule *HotRule) ProgramDeclaration() (engine.RuleProgramDeclaration, bool) {
	return valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
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
