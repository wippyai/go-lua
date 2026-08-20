package source

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
)

// HotRule is Pack/source's typed Rule issuer. It retains only the
// Pack owner's opaque typed issuer; the Rule slot, output Factor, and private
// coordinate remain owned by the exact SchemaBinding.
type HotRule struct {
	implementation *packowner.RuleImplementation[packdomain.Source]
	owner          *packowner.HotOwner
	schema         *packdomain.Schema
}

// BindHot binds one exact callback-free source fragment to one exact Pack
// schema owner. There is no legacy Rule or Composition fallback here.
func BindHot(fragment *SchemaFragment, owner *packowner.HotOwner, schema *packdomain.Schema) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.OwnsSchema(schema) ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	implementation, ok := packowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[packdomain.Value, packdomain.Source]{
		OperandContent: sourceContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, hotSourceChecker(owner, schema, fragment.semantic)),
		Transfer: func(access engine.Access[packdomain.Value, packdomain.Source]) bool {
			source, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			root, rootOK := source.Root()
			fact, valueOK := schema.SourceValue(source)
			if !rootOK || !valueOK || !schema.Admit(root, fact) {
				return false
			}
			rows := 0
			completed := engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, fact)
			})
			return completed && rows == 1
		},
	}, func(source packdomain.Source) (uint64, bool) {
		root, rootOK := source.Root()
		_, valueOK := schema.SourceValue(source)
		index, indexOK := schema.RootOrder(root)
		return uint64(index), rootOK && valueOK && indexOK && index >= 0
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, owner: owner, schema: schema}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (packdomain.Source, bool) {
	if rule == nil || rule.owner == nil || rule.schema == nil ||
		!rule.owner.OwnsSchema(rule.schema) ||
		!coords.Mount.Available() || !coords.Occurrence.Available() {
		return packdomain.Source{}, false
	}
	source, ok := rule.schema.SourceForMountedOccurrence(coords.Mount, coords.Occurrence)
	if !ok {
		return packdomain.Source{}, false
	}
	return source, true
}

// Implementation returns the typed pending issuer. It resolves to the exact
// engine rule only after the shared SchemaBinding has sealed.
func (rule *HotRule) Implementation() (*packowner.RuleImplementation[packdomain.Source], bool) {
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
	implementation, ok := packowner.ResolveRuleImplementation(rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func hotSourceChecker(owner *packowner.HotOwner, schema *packdomain.Schema, ruleSemantic identity.SemanticKey) engine.RuleDerivationChecker[packdomain.Value, packdomain.Source] {
	return func(derivation engine.RuleDerivation[packdomain.Value, packdomain.Source]) (engine.RuleEvidence, bool) {
		if owner == nil || schema == nil || derivation.Rule() != ruleSemantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		source, ok := derivation.Operand()
		id, idOK := source.ContentID()
		if !ok || !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		root, rootOK := source.Root()
		expected, valueOK := schema.SourceValue(source)
		ref, refOK := owner.Ref(root)
		disposition, dispositionOK := derivation.DispositionAt(0)
		if !rootOK || !valueOK || !refOK || !dispositionOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if !targetOK || !actualOK || !engine.TargetMatchesRef(target, ref) || !schema.Lattice().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
