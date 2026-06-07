package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func (t *Transfer) dynamicIndexWriteProof(
	effect WriteEffect,
	key product.AbstractValue,
	value product.AbstractValue,
) (flow.MapWriteProof, bool) {
	if effect.IndexTarget.Kind != cfg.TargetIndex || effect.IndexTarget.Key == nil {
		return flow.MapWriteProof{}, false
	}
	targetPath, ok := effect.Place.FinalDynamicIndexTargetPath()
	if !ok || targetPath.IsEmpty() {
		return flow.MapWriteProof{}, false
	}
	keyPath := constraint.Path{}
	if path, ok := t.staticPathOfExpr(effect.IndexTarget.Key); ok {
		keyPath = path
	}
	valuePath := constraint.Path{}
	if path, ok := t.staticPathOfExpr(effect.Source); ok {
		valuePath = path
	}
	proof, ok := flow.MapWriteTransactionOfPath(flow.MapWritePathTransaction{
		TablePath:              targetPath,
		KeyPath:                keyPath,
		KeyValue:               key,
		ValuePath:              valuePath,
		Value:                  value,
		AllowOpaqueKeyReadback: t.indexWriteTargetSealed(targetPath) || !keyPath.IsEmpty(),
	})
	if !ok {
		return flow.MapWriteProof{}, false
	}
	return proof, true
}

func (t *Transfer) applySymbolicDynamicIndexWriteProof(
	out *flow.PointState,
	target cfg.AssignTarget,
	src ast.Expr,
	value product.AbstractValue,
) bool {
	if out == nil || target.Kind != cfg.TargetIndex || value.IsZero() {
		return false
	}
	tablePath, ok := t.staticContainerPathOfAssignTarget(target)
	if !ok || tablePath.IsEmpty() {
		return false
	}
	keyPath := constraint.Path{}
	if target.Key != nil {
		keyPath, _ = t.staticPathOfExpr(target.Key)
	}
	if keyPath.IsEmpty() {
		return false
	}
	valuePath := constraint.Path{}
	if src != nil {
		valuePath, _ = t.staticPathOfExpr(src)
	}
	return flow.ApplyMapWritePathTransaction(out, flow.MapWritePathTransaction{
		TablePath:              tablePath,
		KeyPath:                keyPath,
		KeyValue:               product.FromType(typ.Unknown),
		ValuePath:              valuePath,
		Value:                  value,
		AllowOpaqueKeyReadback: true,
	})
}

func (t *Transfer) refineByIndexWriteAdmission(
	out *flow.PointState,
	e *ast.AttrGetExpr,
) (product.AbstractValue, bool) {
	if out == nil || e == nil || e.Object == nil || e.Key == nil {
		return product.AbstractValue{}, false
	}
	targetPath, ok := t.staticPathOfExpr(e.Object)
	if !ok || targetPath.IsEmpty() {
		return product.AbstractValue{}, false
	}
	key, ok := t.evalExpr(out, e.Key, nil)
	if !ok || key.IsZero() {
		return product.AbstractValue{}, false
	}
	facts := flow.PointFactsOf(*out)
	keyPath := constraint.Path{}
	if path, ok := t.staticPathOfExpr(e.Key); ok {
		keyPath = path
	}
	admitted, ok := facts.DynamicIndexReadback(flow.DynamicIndexReadbackQuery{
		Target:           targetPath,
		KeyPath:          keyPath,
		KeyValue:         key,
		FollowKeyAliases: true,
	})
	if !ok || admitted.IsZero() {
		return product.AbstractValue{}, false
	}
	return admitted, true
}

func (t *Transfer) indexWriteTargetSealed(path constraint.Path) bool {
	declared, ok := t.declaredTypeForStaticPath(path)
	if !ok || declared == nil || typ.IsAbsentOrUnknown(declared) {
		return false
	}
	return !typ.IsRefinableAnnotation(declared)
}

func (t *Transfer) declaredTypeForStaticPath(path constraint.Path) (typ.Type, bool) {
	if t == nil || path.Symbol == 0 {
		return nil, false
	}
	root, ok := t.declaredTypes[path.Symbol]
	if !ok || root == nil {
		return nil, false
	}
	cur := root
	for _, seg := range path.Segments {
		next, ok := declaredSegmentType(cur, seg)
		if !ok || next == nil {
			return root, true
		}
		cur = next
	}
	return cur, true
}

func (t *Transfer) declaredTypeForExactStaticPath(path constraint.Path) (typ.Type, bool) {
	if t == nil || path.Symbol == 0 {
		return nil, false
	}
	cur, ok := t.declaredTypes[path.Symbol]
	if !ok || cur == nil {
		return nil, false
	}
	for _, seg := range path.Segments {
		next, ok := declaredSegmentType(cur, seg)
		if !ok || next == nil {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func declaredSegmentType(t typ.Type, seg constraint.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case constraint.SegmentField:
		return querycore.Field(t, seg.Name)
	case constraint.SegmentIndexString:
		return querycore.Index(t, typ.LiteralString(seg.Name))
	case constraint.SegmentIndexInt:
		return querycore.Index(t, typ.LiteralInt(int64(seg.Index)))
	default:
		return nil, false
	}
}
