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
	cell, cellOK := implementation.sealedRuleCell()
	if !cellOK || cell.state == nil || resolve == nil {
		return false
	}
	state := cell.state
	state.mu.Lock()
	defer state.mu.Unlock()
	hot := cell.impl
	if hot == nil || hot.operandResolver != nil {
		return false
	}
	hot.operandResolver = resolve
	return true
}

// HasOperandResolver reports whether this rule's sealed cell already holds
// its owner-supplied resolver.
func (implementation *RuleImplementation[K, V, O]) HasOperandResolver() bool {
	cell, ok := implementation.sealedRuleCell()
	if !ok || cell.impl == nil {
		return false
	}
	return cell.impl.operandResolver != nil
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
	programMemberBinder
}

// programMemberBinder is the shared member-row lane. The ordinary issuer
// embeds it through programRule; activation keeps the same typed lane without
// duplicating the bind method in a second interface.
type programMemberBinder interface {
	bindProgramMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, operand declaredRuleOperand) (runtimeMember, bool)
}

// ProgramRule is one sealed, engine-owned rule row issuer. Composition emits
// these values in catalog order; the constructor consumes the aligned slice
// directly and never recovers a program from a hot schema cell.
type ProgramRule struct {
	rule       programRule
	activation programMemberBinder
}

// Available reports whether the primitive was issued by the engine.
// Exactly one issuer lane is valid: ordinary Rule or activation, never both.
func (program ProgramRule) Available() bool {
	return (program.rule != nil) != (program.activation != nil)
}

func (program ProgramRule) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	if !program.Available() || program.rule == nil {
		return composition.Key{}, composition.Key{}, false
	}
	return program.rule.declaredRuleSchema()
}

func (program ProgramRule) declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool) {
	if !program.Available() || program.rule == nil {
		return declaredRuleOperand{}, false
	}
	return program.rule.declareRuleOperand(coords)
}

func (program ProgramRule) declareRuleSurfaces(operand declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool) {
	if !program.Available() || program.rule == nil {
		return declaredRuleSurfaces{}, false
	}
	return program.rule.declareRuleSurfaces(operand, anchor)
}

func (program ProgramRule) bindProgramMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, operand declaredRuleOperand) (runtimeMember, bool) {
	if !program.Available() {
		return nil, false
	}
	if program.rule != nil {
		return program.rule.bindProgramMember(plane, topology, member, operand)
	}
	if program.activation != nil {
		return program.activation.bindProgramMember(plane, topology, member, operand)
	}
	return nil, false
}

// SealProgramRule turns the exact sealed typed implementation into the one
// primitive construction input. The generic boundary is deliberately here so
// schema composition never needs to name engine's private issuer interface.
func SealProgramRule[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O]) (ProgramRule, bool) {
	if _, ok := implementation.sealedRuleCell(); !ok {
		return ProgramRule{}, false
	}
	program := ProgramRule{rule: implementation}
	if !program.Available() {
		return ProgramRule{}, false
	}
	return program, true
}

// SealActivationProgramRule publishes the activation issuer through the same
// sealed primitive while preserving activation's separate admission path.
func SealActivationProgramRule(implementation *ActivationRuleImplementation) (ProgramRule, bool) {
	cell, ok := implementation.sealedActivationCell()
	if !ok || !cell.schemaRuleComplete() {
		return ProgramRule{}, false
	}
	program := ProgramRule{activation: implementation}
	if !program.Available() {
		return ProgramRule{}, false
	}
	return program, true
}

func (implementation *RuleImplementation[K, V, O]) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	cell, ok := implementation.sealedRuleCell()
	if !ok || cell == nil || cell.impl == nil {
		return composition.Key{}, composition.Key{}, false
	}
	semantic, family := cell.impl.ruleSemantic, cell.impl.operandFamily
	return semantic, family, semantic.Available() && family.Available()
}

func (implementation *ActivationRuleImplementation) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	cell, ok := implementation.sealedActivationCell()
	if !ok || !cell.schemaRuleComplete() {
		return composition.Key{}, composition.Key{}, false
	}
	shape, shapeOK := cell.schema.ruleShapeAt(implementation.ordinal)
	if !shapeOK {
		return composition.Key{}, composition.Key{}, false
	}
	semantic, family := cell.schema.ruleSemanticAt(implementation.ordinal), shape.OperandFamily
	return semantic, family, semantic.Available() && family.Available()
}

func (implementation *RuleImplementation[K, V, O]) resolveOperand(coords OperandCoords) (O, bool) {
	var absent O
	cell, ok := implementation.sealedRuleCell()
	if !ok || cell.impl == nil || cell.impl.operandResolver == nil {
		return absent, false
	}
	return cell.impl.operandResolver(coords)
}
