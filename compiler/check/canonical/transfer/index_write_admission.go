package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func (t *Transfer) recordIndexWriteAdmission(
	out *flow.PointState,
	effect WriteEffect,
	key product.AbstractValue,
	value product.AbstractValue,
) {
	if out == nil || effect.IndexTarget.Kind != cfg.TargetIndex || effect.IndexTarget.Key == nil {
		return
	}
	if key.IsZero() || value.IsZero() {
		return
	}
	if !admissibleIndexWriteProofValue(key) || !admissibleIndexWriteProofValue(value) {
		return
	}
	targetPath, ok := indexWriteTargetPath(effect.Place)
	if !ok || targetPath.IsEmpty() || t.indexWriteTargetSealed(targetPath) {
		return
	}
	fact := flow.IndexWriteAdmissionFact{
		Target: flow.IndexWriteAdmissionPathKey(targetPath),
		Key:    key,
		Value:  value,
	}
	if keyPath, ok := t.staticPathOfExpr(effect.IndexTarget.Key); ok {
		fact.KeyPath = flow.IndexWriteAdmissionPathKey(keyPath)
	}
	if valuePath, ok := t.staticPathOfExpr(effect.Source); ok {
		fact.ValuePath = flow.IndexWriteAdmissionPathKey(valuePath)
	}
	out.IndexWrites = out.IndexWrites.With(fact)
}

func admissibleIndexWriteProofValue(av product.AbstractValue) bool {
	if av.IsZero() {
		return false
	}
	t := product.ProjectValueOrUnknown(av)
	return !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t)
}

func indexWriteTargetPath(place Place) (constraint.Path, bool) {
	if place.Root == 0 || len(place.Steps) == 0 {
		return constraint.Path{}, false
	}
	if place.Steps[len(place.Steps)-1].Kind != PlaceStepDynamicIndex {
		return constraint.Path{}, false
	}
	prefix := Place{
		Root:     place.Root,
		RootName: place.RootName,
		Steps:    append([]PlaceStep(nil), place.Steps[:len(place.Steps)-1]...),
	}
	return prefix.StaticPath()
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
