package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// AssignmentProvenanceEffect is the transfer-local publication payload for path
// facts introduced by a local assignment. A plain alias assignment (`b = a`) can
// carry both value-origin identity for later index-write readback and
// key-presence proofs where the assigned value is later used as the dynamic key.
type AssignmentProvenanceEffect struct {
	TargetPath constraint.Path
	SourcePath constraint.Path
	Value      product.AbstractValue
}

// ArrayElementKeyProvenanceEffect is the transfer-local publication payload for
// a local assignment from an indexed array element. If the source array is proven
// to hold keys for one or more tables, the assigned target is a key for those
// tables.
type ArrayElementKeyProvenanceEffect struct {
	TargetPath constraint.Path
	ArrayPath  constraint.Path
	Value      product.AbstractValue
}

func (t *Transfer) assignmentProvenanceEffect(
	target cfg.AssignTarget,
	src ast.Expr,
	val product.AbstractValue,
) (AssignmentProvenanceEffect, bool) {
	return t.assignmentProvenanceEffectWithSourceSymbol(target, src, 0, val)
}

func (t *Transfer) assignmentProvenanceEffectWithSourceSymbol(
	target cfg.AssignTarget,
	src ast.Expr,
	sourceSym cfg.SymbolID,
	val product.AbstractValue,
) (AssignmentProvenanceEffect, bool) {
	if src == nil {
		return AssignmentProvenanceEffect{}, false
	}
	targetPath, ok := t.exactStaticAssignTargetPath(target)
	if !ok || targetPath.Symbol == 0 || targetPath.IsEmpty() {
		return AssignmentProvenanceEffect{}, false
	}
	sourcePath, ok := t.staticPathOfExpr(src)
	if !ok || sourcePath.Symbol == 0 || sourcePath.IsEmpty() {
		sourcePath, ok = staticPathOfExprWithRootSymbol(src, sourceSym)
		if !ok || sourcePath.Symbol == 0 || sourcePath.IsEmpty() {
			return AssignmentProvenanceEffect{}, false
		}
	}
	return AssignmentProvenanceEffect{
		TargetPath: targetPath,
		SourcePath: sourcePath,
		Value:      val,
	}, true
}

func (t *Transfer) arrayElementKeyProvenanceEffect(
	target cfg.AssignTarget,
	src ast.Expr,
	val product.AbstractValue,
) (ArrayElementKeyProvenanceEffect, bool) {
	targetPath, ok := t.exactStaticAssignTargetPath(target)
	if !ok || targetPath.Symbol == 0 || targetPath.IsEmpty() {
		return ArrayElementKeyProvenanceEffect{}, false
	}
	attr, ok := src.(*ast.AttrGetExpr)
	if !ok || attr == nil || attr.Key == nil {
		return ArrayElementKeyProvenanceEffect{}, false
	}
	arrayPath, ok := t.containerExprPath(attr.Object)
	if !ok || arrayPath.IsEmpty() {
		return ArrayElementKeyProvenanceEffect{}, false
	}
	return ArrayElementKeyProvenanceEffect{
		TargetPath: targetPath,
		ArrayPath:  arrayPath,
		Value:      val,
	}, true
}

func (t *Transfer) applyAssignmentProvenanceEffect(
	out *flow.PointState,
	effect AssignmentProvenanceEffect,
) bool {
	if out == nil || effect.TargetPath.IsEmpty() || effect.SourcePath.IsEmpty() {
		return false
	}
	return flow.ApplyAssignmentAliasPathTransaction(out, flow.AssignmentAliasPathTransaction{
		TargetPath:  effect.TargetPath,
		SourcePath:  effect.SourcePath,
		SourceValue: effect.Value,
	})
}

func (t *Transfer) applyArrayElementKeyProvenanceEffect(
	out *flow.PointState,
	effect ArrayElementKeyProvenanceEffect,
) bool {
	if out == nil || effect.TargetPath.IsEmpty() || effect.ArrayPath.IsEmpty() {
		return false
	}
	return flow.ApplyArrayElementKeyPathTransaction(out, flow.ArrayElementKeyPathTransaction{
		TargetPath: effect.TargetPath,
		ArrayPath:  effect.ArrayPath,
		KeyValue:   effect.Value,
	})
}
