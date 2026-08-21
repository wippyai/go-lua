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

// sealedRuleCell is the engine-owned construction surface of one canonical
// sealed schema Rule cell. The capability resolver below returns this cell
// directly; no callback-bearing issuer is constructed.
// Ordinary and activation cells share only the schema/row identity and member
// bind operation. Ordinary cells additionally expose their operand/surface
// declaration, while activation cells use their separate admission plane.
type sealedRuleCell interface {
	schemaRuleBindingCell
	declaredRuleSchema() (semantic, family composition.Key, ok bool)
	bindMember(plane *programPlane, topology *equation.Topology, member equation.RuleMember, operand declaredRuleOperand) (runtimeMember, bool)
}

// ordinarySealedRuleCell is the declaration-capable subset of a sealed Rule
// cell. Activation cells intentionally do not implement these two methods:
// their trigger read and candidate transport are admitted through the
// activation-specific inventory.
type ordinarySealedRuleCell interface {
	sealedRuleCell
	declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool)
	declareRuleSurfaces(operand declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool)
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	if cell == nil || !cell.sealedRuleComplete() || cell.impl == nil {
		return composition.Key{}, composition.Key{}, false
	}
	semantic, family := cell.impl.ruleSemantic, cell.impl.operandFamily
	return semantic, family, semantic.Available() && family.Available()
}

func (cell *schemaActivationRuleBindingCell) declaredRuleSchema() (composition.Key, composition.Key, bool) {
	if cell == nil || !cell.schemaRuleComplete() || cell.schema == nil {
		return composition.Key{}, composition.Key{}, false
	}
	shape, shapeOK := cell.schema.ruleShapeAt(cell.ordinal)
	if !shapeOK {
		return composition.Key{}, composition.Key{}, false
	}
	semantic, family := cell.schema.ruleSemanticAt(cell.ordinal), shape.OperandFamily
	return semantic, family, semantic.Available() && family.Available()
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool) {
	if cell == nil || !cell.sealedRuleComplete() || cell.impl == nil || cell.impl.operandResolver == nil || cell.impl.operandContent == nil {
		return declaredRuleOperand{}, false
	}
	operand, resolved := cell.impl.operandResolver(coords)
	if !resolved {
		return declaredRuleOperand{}, false
	}
	canonical, digest, contentOK := cell.impl.operandContent(operand)
	if !contentOK || digest == [32]byte{} {
		return declaredRuleOperand{}, false
	}
	return declaredRuleOperand{value: canonical, digest: digest}, true
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) declareRuleSurfaces(declared declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool) {
	if cell == nil || !cell.sealedRuleComplete() || cell.impl == nil {
		return declaredRuleSurfaces{}, false
	}
	operand, typed := declared.value.(O)
	if !typed {
		return declaredRuleSurfaces{}, false
	}
	semantic, semanticOK := semanticKeyFromComposition(cell.impl.ruleSemantic)
	if !semanticOK {
		return declaredRuleSurfaces{}, false
	}
	reads, writes, carries, ok := placeSchemaRuleSurfaces(cell, semantic, anchor, operand)
	if !ok {
		return declaredRuleSurfaces{}, false
	}
	return declaredRuleSurfaces{reads: reads, writes: writes, carries: carries}, true
}
