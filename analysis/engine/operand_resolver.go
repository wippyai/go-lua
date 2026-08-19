package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// OperandCoords is the sealed neutral member coordinate the engine already
// holds. The owner-supplied resolver on the rule cell turns these into the
// typed operand; the engine never threads that operand through attach.
type OperandCoords struct {
	Member     identity.ContentID
	Mount      identity.ContentID
	Point      identity.ContentID
	Occurrence identity.ContentID
}

// InstallOperandResolver publishes the one owner-supplied operand resolver
// on this rule's sealed cell. A second install is rejected: one rule has
// exactly one resolver.
func (implementation *RuleImplementation[K, V, O]) InstallOperandResolver(resolve func(OperandCoords) (O, bool)) bool {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.state == nil || resolve == nil {
		return false
	}
	state := implementation.binding.state
	state.mu.Lock()
	defer state.mu.Unlock()
	hot := implementation.binding.cell.impl
	if hot == nil || hot.operandResolver != nil {
		return false
	}
	hot.operandResolver = resolve
	return true
}

// HasOperandResolver reports whether this rule's sealed cell already holds
// its owner-supplied resolver.
func (implementation *RuleImplementation[K, V, O]) HasOperandResolver() bool {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil {
		return false
	}
	return implementation.binding.cell.impl.operandResolver != nil
}

// programRule is the engine's private row issuer. It is wrapped by ProgramRule
// before crossing the schema/composition boundary; callers can only hand back
// the sealed primitive and cannot implement or forge this surface.
type programRule interface {
	// declaredRuleSchema names the cold rule this handle issues rows for.
	declaredRuleSchema() (semantic, family composition.Key, ok bool)
	// declareRuleOperand resolves one issuance's canonical operand.
	declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool)
	// declareRuleSurfaces places the cold shape's surfaces over that operand
	// at the sealed anchor the engine minted.
	declareRuleSurfaces(operand declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool)
}

type programMemberBinder interface {
	bindProgramMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, coords OperandCoords) (runtimeMember, bool)
}

// ProgramRule is one sealed, engine-owned rule row issuer. Composition emits
// these values in catalog order; the constructor consumes the aligned slice
// directly and never recovers a program from a hot schema cell.
type ProgramRule struct {
	issuer programRule
	binder programMemberBinder
}

// Available reports whether the primitive was issued by the engine.
func (program ProgramRule) Available() bool { return program.binder != nil }

func (program ProgramRule) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	if program.issuer == nil {
		return composition.Key{}, composition.Key{}, false
	}
	return program.issuer.declaredRuleSchema()
}

func (program ProgramRule) declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool) {
	if program.issuer == nil {
		return declaredRuleOperand{}, false
	}
	return program.issuer.declareRuleOperand(coords)
}

func (program ProgramRule) declareRuleSurfaces(operand declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool) {
	if program.issuer == nil {
		return declaredRuleSurfaces{}, false
	}
	return program.issuer.declareRuleSurfaces(operand, anchor)
}

func (program ProgramRule) bindProgramMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, coords OperandCoords) (runtimeMember, bool) {
	if program.binder == nil {
		return nil, false
	}
	return program.binder.bindProgramMember(plane, topology, member, coords)
}

// SealProgramRule turns the exact sealed typed implementation into the one
// primitive construction input. The generic boundary is deliberately here so
// schema composition never needs to name engine's private issuer interface.
func SealProgramRule[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O]) (ProgramRule, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return ProgramRule{}, false
	}
	return ProgramRule{issuer: implementation, binder: implementation}, true
}

// SealActivationProgramRule publishes the activation issuer through the same
// sealed primitive while preserving activation's separate admission path.
func SealActivationProgramRule(implementation *ActivationRuleImplementation) (ProgramRule, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return ProgramRule{}, false
	}
	return ProgramRule{binder: implementation}, true
}

func (implementation *RuleImplementation[K, V, O]) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.proof == nil {
		return composition.Key{}, composition.Key{}, false
	}
	semantic, family := implementation.binding.proof.semantic, implementation.binding.proof.operandFamily
	return semantic, family, semantic.Available() && family.Available()
}

func (implementation *ActivationRuleImplementation) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.proof == nil {
		return composition.Key{}, composition.Key{}, false
	}
	semantic, family := implementation.binding.proof.semantic, implementation.binding.proof.operandFamily
	return semantic, family, semantic.Available() && family.Available()
}

func (implementation *RuleImplementation[K, V, O]) resolveOperand(coords OperandCoords) (O, bool) {
	var absent O
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil || implementation.binding.cell.impl.operandResolver == nil {
		return absent, false
	}
	return implementation.binding.cell.impl.operandResolver(coords)
}
