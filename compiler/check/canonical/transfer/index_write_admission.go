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

// dynamicIndexWritePathTransaction lowers transfer-owned source syntax into the
// flow-domain transaction. Flow owns publication; transfer only supplies stable
// table/key/value paths and the already-proven product values.
func (t *Transfer) dynamicIndexWritePathTransaction(
	target cfg.AssignTarget,
	source ast.Expr,
	tablePath constraint.Path,
	keyValue product.AbstractValue,
	writtenValue product.AbstractValue,
	readbackValue product.AbstractValue,
) (flow.DynamicIndexWritePathTransaction, bool) {
	if target.Kind != cfg.TargetIndex || target.Key == nil || tablePath.IsEmpty() || writtenValue.IsZero() {
		return flow.DynamicIndexWritePathTransaction{}, false
	}
	keyPath := constraint.Path{}
	if path, ok := t.staticPathOfExpr(target.Key); ok {
		keyPath = path
	}
	valuePath := constraint.Path{}
	if path, ok := t.staticPathOfExpr(source); ok {
		valuePath = path
	}
	return flow.DynamicIndexWritePathTransaction{
		TablePath:     tablePath,
		KeyPath:       keyPath,
		KeyValue:      keyValue,
		ValuePath:     valuePath,
		WrittenValue:  writtenValue,
		ReadbackValue: readbackValue,
	}, true
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
