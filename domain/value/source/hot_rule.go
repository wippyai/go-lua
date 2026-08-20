package source

import (
	"github.com/wippyai/go-lua/analysis/engine"
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
		!fragment.semantic.Available() {
		return nil, false
	}
	implementation, ok := valueowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[value.Value, value.SourceSeed]{
		OperandContent: sourceSeedContent,
		Fold: func(frame engine.Frame[value.Value, value.SourceSeed]) engine.RuleResult[value.Value] {
			seed, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[value.Value]{}
			}
			_, fact, resultOK := sourceResultForSchema(owner.Schema(), seed)
			if !resultOK {
				return engine.RuleResult[value.Value]{}
			}
			return engine.Staged(frame, fact)
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

// resolveOperand redeems the preissued SourceSeed through Value's own mounted
// occurrence inverse. No Program term, Flow traversal, or Link inverse is
// reconstructed on the hot path, and no mount-scoped issuer stands between.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.SourceSeed, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.SourceSeed{}, false
	}
	schema := rule.owner.Schema()
	seed, ok := schema.SourceSeedForMountedOccurrence(coords.Mount, coords.Occurrence)
	_, _, valid := sourceResultForSchema(schema, seed)
	return seed, ok && valid
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
