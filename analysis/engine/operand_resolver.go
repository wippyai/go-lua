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

// RuleProgramDeclaration is the erased declaration of one sealed rule cell.
// The engine enumerates neutral member coordinates and this handle declares
// the row it issues at each of them; the typed operand never leaves the cell.
//
// Every method is unexported, so the engine is the only package that can
// publish one: a Link hands the construction inventory the handle its sealed
// cell issued and never a declaration of its own.
type RuleProgramDeclaration interface {
	// declaredRuleSchema names the cold rule this handle issues rows for.
	declaredRuleSchema() (semantic, family composition.Key, ok bool)
	// declareRuleOperand resolves one issuance's canonical operand.
	declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool)
	// declareRuleSurfaces places the cold shape's surfaces over that operand
	// at the sealed anchor the engine minted.
	declareRuleSurfaces(operand declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool)
	// bindProgramMember mints the runtime row of one published member.
	bindProgramMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, coords OperandCoords) (runtimeMember, bool)
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

// RuleProgramSource is the hot-cell surface that publishes the one program
// declaration for a mounted or Link rule. The composition recovers it from the
// opaque cell; the engine never switches on a domain type.
type RuleProgramSource interface {
	ProgramDeclaration() (RuleProgramDeclaration, bool)
}

func (implementation *RuleImplementation[K, V, O]) resolveOperand(coords OperandCoords) (O, bool) {
	var absent O
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil || implementation.binding.cell.impl.operandResolver == nil {
		return absent, false
	}
	return implementation.binding.cell.impl.operandResolver(coords)
}
