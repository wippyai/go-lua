package source

import (
	"github.com/wippyai/go-lua/analysis/engine"
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
		!fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{owner: owner, schema: schema}
	implementation, ok := packowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[packdomain.Value, packdomain.Source]{
		OperandContent: func(source packdomain.Source) (packdomain.Source, [32]byte, bool) {
			id, ok := source.ContentID()
			return source, [32]byte(id), ok && id.Available()
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[packdomain.Value, packdomain.Source]) engine.RuleResult[packdomain.Value] {
			source, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[packdomain.Value]{}
			}
			root, rootOK := source.Root()
			fact, valueOK := schema.SourceValue(source)
			if !rootOK || !valueOK || !schema.Admit(root, fact) {
				return engine.RuleResult[packdomain.Value]{}
			}
			return engine.Staged(frame, fact)
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
	rule.implementation = implementation
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
