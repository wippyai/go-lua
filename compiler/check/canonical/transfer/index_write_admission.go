package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// DynamicIndexWriteProofEffect is the transfer reducer payload for the path
// facts proven by one admitted dynamic-index write. Key-presence is lightweight
// and path based; value readback is stricter because it can carry large product
// values through loops.
type DynamicIndexWriteProofEffect struct {
	TablePath              constraint.Path
	KeyPath                constraint.Path
	Key                    product.AbstractValue
	ValuePath              constraint.Path
	Value                  product.AbstractValue
	AllowOpaqueKeyReadback bool
}

func (t *Transfer) dynamicIndexWriteProofEffect(
	effect WriteEffect,
	key product.AbstractValue,
	value product.AbstractValue,
) (DynamicIndexWriteProofEffect, bool) {
	if effect.IndexTarget.Kind != cfg.TargetIndex || effect.IndexTarget.Key == nil {
		return DynamicIndexWriteProofEffect{}, false
	}
	targetPath, ok := indexWriteTargetPath(effect.Place)
	if !ok || targetPath.IsEmpty() {
		return DynamicIndexWriteProofEffect{}, false
	}
	keyPath := constraint.Path{}
	if path, ok := t.staticPathOfExpr(effect.IndexTarget.Key); ok {
		keyPath = path
	}
	proof := DynamicIndexWriteProofEffect{
		TablePath:              targetPath,
		KeyPath:                keyPath,
		Key:                    key,
		Value:                  value,
		AllowOpaqueKeyReadback: t.indexWriteTargetSealed(targetPath) || !keyPath.IsEmpty(),
	}
	if valuePath, ok := t.staticPathOfExpr(effect.Source); ok {
		proof.ValuePath = valuePath
	}
	return proof, true
}

func (t *Transfer) applyDynamicIndexWriteProofEffect(
	out *flow.PointState,
	effect DynamicIndexWriteProofEffect,
) bool {
	if out == nil || effect.TablePath.IsEmpty() {
		return false
	}
	proof, ok := flow.MapWriteProofFromPaths(
		effect.TablePath,
		effect.KeyPath,
		effect.Key,
		effect.ValuePath,
		effect.Value,
		effect.AllowOpaqueKeyReadback,
	)
	if !ok {
		return false
	}
	return flow.ApplyMapWriteProof(out, proof)
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
	proof, ok := flow.MapWriteProofFromPaths(
		tablePath,
		keyPath,
		product.FromType(typ.Unknown),
		valuePath,
		value,
		true,
	)
	if !ok {
		return false
	}
	return flow.ApplyMapWriteProof(out, proof)
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
	keyType := flow.NormalizeDynamicKeyType(product.ProjectValueOrUnknown(key))
	targetAddr, ok := flow.StableAddressOfPath(targetPath)
	if !ok {
		return product.AbstractValue{}, false
	}
	keyPaths := t.indexWriteReadKeyPaths(out, e.Key)
	for _, keyPath := range keyPaths {
		keyAddr, ok := flow.StableAddressOfPath(keyPath)
		if !ok {
			continue
		}
		if admitted, ok := out.IndexWrites.AdmissionAtAddress(flow.IndexWriteAddressQuery{
			Target:     targetAddr,
			KeyPath:    keyAddr,
			HasKeyPath: true,
			KeyValue:   product.FromType(keyType),
		}); ok && !admitted.IsZero() {
			return admitted, true
		}
	}
	if !indexWriteReadCanUseKeyValueOnly(keyType) {
		return product.AbstractValue{}, false
	}
	admitted, ok := out.IndexWrites.AdmissionAtAddress(flow.IndexWriteAddressQuery{
		Target:   targetAddr,
		KeyValue: product.FromType(keyType),
	})
	if !ok || admitted.IsZero() {
		return product.AbstractValue{}, false
	}
	return admitted, true
}

func (t *Transfer) indexWriteReadKeyPaths(out *flow.PointState, key ast.Expr) []constraint.Path {
	keyPath, ok := t.staticPathOfExpr(key)
	if !ok || keyPath.Symbol == 0 {
		return nil
	}
	var outPaths []constraint.Path
	seen := map[constraint.PathKey]struct{}{}
	queue := []constraint.Path{keyPath}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curAddr, ok := flow.StableAddressOfPath(cur)
		if !ok {
			continue
		}
		curKey := curAddr.Key()
		if _, ok := seen[curKey]; ok {
			continue
		}
		seen[curKey] = struct{}{}
		outPaths = append(outPaths, cur)
		if out == nil {
			continue
		}
		for _, use := range out.PathAliases.AliasesCoveringPath(cur) {
			source, ok := indexWritePathFromKey(use.Alias.Source)
			if !ok {
				continue
			}
			for _, seg := range use.Remainder {
				source = source.Append(seg)
			}
			if !source.IsEmpty() {
				queue = append(queue, source)
			}
		}
		for _, use := range out.ValueOrigins.OriginsCoveringPath(cur) {
			if use.Origin.Kind != flow.ValueOriginAssignmentAlias {
				continue
			}
			source, ok := indexWritePathFromKey(use.Origin.Source)
			if !ok {
				continue
			}
			for _, seg := range use.Remainder {
				source = source.Append(seg)
			}
			if !source.IsEmpty() {
				queue = append(queue, source)
			}
		}
	}
	return outPaths
}

func indexWritePathFromKey(key constraint.PathKey) (constraint.Path, bool) {
	addr, ok := flow.StableAddressFromKey(key)
	if !ok {
		return constraint.Path{}, false
	}
	return addr.Path()
}

func indexWriteReadCanUseKeyValueOnly(keyType typ.Type) bool {
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		return false
	}
	return typ.UnwrapAnnotated(keyType).Kind() == kind.Literal
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
