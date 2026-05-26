package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ProjectedTypeAt returns the product-domain type visible at key, including
// structural implications from narrowed ancestors and child paths.
func (d *ProductDomain) ProjectedTypeAt(key constraint.PathKey, resolver narrow.Resolver) typ.Type {
	if d == nil || key == "" {
		return nil
	}
	current := d.TypeAt(key)

	if narrowed, ok := d.deriveFromNarrowedAncestors(key, resolver); ok {
		if current == nil {
			current = narrowed
		} else {
			current = narrow.Intersect(current, narrowed)
		}
	}

	if children := d.NarrowedChildPaths(key); len(children) > 0 {
		current = filterByChildNarrowings(current, key, children, resolver)
	}

	return current
}

func (d *ProductDomain) withProjectedJoinFacts(left, right *ProductDomain) *ProductDomain {
	if d == nil || left == nil || right == nil {
		return d
	}
	keys := productJoinSupportKeys(left, right)
	for _, key := range constraint.SortedPathKeys(keys) {
		leftType := left.projectedTypeForJoin(key)
		rightType := right.projectedTypeForJoin(key)
		if leftType == nil || rightType == nil {
			continue
		}

		joined := typejoin.Types(leftType, rightType)
		if joined == nil || joined.Kind().IsNever() {
			continue
		}

		base := d.baseTypeAt(key)
		if base != nil && typ.TypeEquals(joined, base) {
			delete(d.Type.Narrowed, key)
			continue
		}
		d.Type.Narrowed[key] = joined
	}
	return d
}

func productJoinSupportKeys(left, right *ProductDomain) map[constraint.PathKey]struct{} {
	keys := make(map[constraint.PathKey]struct{})
	addProductSupportKeys(keys, left)
	addProductSupportKeys(keys, right)
	return keys
}

func addProductSupportKeys(keys map[constraint.PathKey]struct{}, d *ProductDomain) {
	if d == nil {
		return
	}
	for _, key := range constraint.SortedPathKeys(d.Type.Narrowed) {
		addProductSupportKey(keys, key)
	}
	for _, key := range constraint.SortedPathKeys(d.Shape.Narrowed) {
		addProductSupportKey(keys, key)
	}
}

func addProductSupportKey(keys map[constraint.PathKey]struct{}, key constraint.PathKey) {
	if key == "" {
		return
	}
	keys[key] = struct{}{}
	root, suffix, ok := pathkey.ParseRootAndSuffix(key)
	if !ok || suffix == "" {
		return
	}
	segs := pathkey.ParseSuffix(suffix)
	for depth := len(segs) - 1; depth >= 0; depth-- {
		parent := constraint.PathKey(root + pathkey.SegmentsSuffix(segs[:depth]))
		if parent != "" {
			keys[parent] = struct{}{}
		}
	}
}

func (d *ProductDomain) projectedTypeForJoin(key constraint.PathKey) typ.Type {
	if d == nil || key == "" {
		return nil
	}
	if d.env.Resolver != nil {
		return d.ProjectedTypeAt(key, d.env.Resolver)
	}
	return d.TypeAt(key)
}

func (d *ProductDomain) baseTypeAt(key constraint.PathKey) typ.Type {
	if d == nil || d.env.PathTypeAt == nil || key == "" {
		return nil
	}
	return d.env.PathTypeAt(key)
}

func meetTypeAndShapeFacts(typeFact, shapeFact typ.Type, resolver narrow.Resolver) typ.Type {
	if typeFact == nil || shapeFact == nil {
		return nil
	}
	if resolver == nil {
		return narrow.Intersect(typeFact, shapeFact)
	}
	filtered := filterTypeByShapeEvidence(typeFact, shapeFact, resolver)
	if filtered == nil || filtered.Kind().IsNever() {
		return typ.Never
	}
	met := narrow.Intersect(filtered, shapeFact)
	if met == nil || met.Kind().IsNever() {
		return filtered
	}
	return met
}

func filterTypeByShapeEvidence(t, shape typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || shape == nil {
		return nil
	}
	if t.Kind().IsPlaceholder() || shape.Kind().IsPlaceholder() {
		return t
	}
	if a, ok := t.(*typ.Alias); ok {
		return filterTypeByShapeEvidence(a.Target, shape, resolver)
	}
	if expanded := unwrap.Instantiated(t); expanded != nil && expanded != t {
		return filterTypeByShapeEvidence(expanded, shape, resolver)
	}
	if a, ok := shape.(*typ.Alias); ok {
		return filterTypeByShapeEvidence(t, a.Target, resolver)
	}
	if expanded := unwrap.Instantiated(shape); expanded != nil && expanded != shape {
		return filterTypeByShapeEvidence(t, expanded, resolver)
	}
	if opt, ok := t.(*typ.Optional); ok {
		return filterTypeByShapeEvidence(opt.Inner, shape, resolver)
	}
	if u, ok := t.(*typ.Union); ok {
		kept := make([]typ.Type, 0, len(u.Members))
		for _, member := range u.Members {
			filtered := filterTypeByShapeEvidence(member, shape, resolver)
			if filtered != nil && !filtered.Kind().IsNever() {
				kept = append(kept, filtered)
			}
		}
		if len(kept) == 0 {
			return typ.Never
		}
		if len(kept) == 1 {
			return kept[0]
		}
		return typ.NewUnion(kept...)
	}
	if u, ok := shape.(*typ.Union); ok {
		for _, member := range u.Members {
			if shapeEvidenceAdmitsType(t, member, resolver) {
				return t
			}
		}
		return typ.Never
	}
	if shapeEvidenceAdmitsType(t, shape, resolver) {
		return t
	}
	return typ.Never
}

func shapeEvidenceAdmitsType(t, shape typ.Type, resolver narrow.Resolver) bool {
	if t == nil || shape == nil {
		return false
	}
	if t.Kind().IsPlaceholder() || shape.Kind().IsPlaceholder() {
		return true
	}
	shape = unwrap.Alias(shape)
	if expanded := unwrap.Instantiated(shape); expanded != nil && expanded != shape {
		shape = expanded
	}
	if rec, ok := shape.(*typ.Record); ok {
		return recordShapeEvidenceAdmitsType(t, rec, resolver)
	}
	return narrow.TypesOverlap(t, shape)
}

func recordShapeEvidenceAdmitsType(t typ.Type, shape *typ.Record, resolver narrow.Resolver) bool {
	if shape == nil {
		return true
	}
	for _, field := range shape.Fields {
		if field.Optional {
			continue
		}
		fieldType, ok := resolver.Field(t, field.Name)
		if !ok || fieldType == nil {
			return false
		}
		if field.Type != nil && !narrow.TypesOverlap(fieldType, field.Type) {
			return false
		}
	}
	if shape.HasMapComponent() && shape.MapKey != nil {
		value, ok := resolver.Index(t, shape.MapKey)
		if !ok || value == nil {
			return false
		}
		if shape.MapValue != nil && !narrow.TypesOverlap(value, shape.MapValue) {
			return false
		}
	}
	return true
}

func (d *ProductDomain) deriveFromNarrowedAncestors(targetKey constraint.PathKey, resolver narrow.Resolver) (typ.Type, bool) {
	targetSym, targetVersion, targetSuffix, ok := pathkey.ParseKeyUnchecked(targetKey)
	if !ok || targetSuffix == "" {
		return nil, false
	}
	targetSegs := pathkey.ParseSuffix(targetSuffix)

	seen := make(map[constraint.PathKey]bool)
	candidates := make([]constraint.PathKey, 0, len(d.Type.Narrowed)+len(d.Shape.Narrowed))
	for _, key := range constraint.SortedPathKeys(d.Type.Narrowed) {
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, key)
	}
	for _, key := range constraint.SortedPathKeys(d.Shape.Narrowed) {
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, key)
	}

	var combined typ.Type
	for _, candidateKey := range candidates {
		ancestorType := d.TypeAt(candidateKey)
		if ancestorType == nil {
			continue
		}
		sym, version, suffix, ok := pathkey.ParseKeyUnchecked(candidateKey)
		if !ok || sym != targetSym || version != targetVersion {
			continue
		}
		ancestorSegs := pathkey.ParseSuffix(suffix)
		if len(ancestorSegs) >= len(targetSegs) {
			continue
		}
		if !pathkey.SegmentsPrefix(ancestorSegs, targetSegs) {
			continue
		}

		remaining := targetSegs[len(ancestorSegs):]
		derived, ok := deriveTypeFrom(resolver, ancestorType, remaining)
		if !ok || derived == nil {
			continue
		}
		if combined == nil {
			combined = derived
		} else {
			combined = narrow.Intersect(combined, derived)
		}
	}

	return combined, combined != nil
}

func filterByChildNarrowings(baseType typ.Type, parentKey constraint.PathKey, children map[constraint.PathKey]typ.Type, resolver narrow.Resolver) typ.Type {
	u, ok := baseType.(*typ.Union)
	if !ok {
		return baseType
	}
	parentSym, _, _, ok := pathkey.ParseKeyUnchecked(parentKey)
	if !ok {
		return baseType
	}

	type parsedChildNarrowing struct {
		narrowed typ.Type
		segs     []constraint.Segment
	}

	parsedChildren := make([]parsedChildNarrowing, 0, len(children))
	for childKey, narrowedChild := range children {
		childSym, _, suffix, ok := pathkey.ParseKeyUnchecked(childKey)
		if !ok || childSym != parentSym {
			continue
		}
		segs := pathkey.ParseSuffix(suffix)
		if len(segs) == 0 {
			continue
		}
		parsedChildren = append(parsedChildren, parsedChildNarrowing{
			narrowed: narrowedChild,
			segs:     segs,
		})
	}
	if len(parsedChildren) == 0 {
		return baseType
	}

	var kept []typ.Type
	for _, member := range u.Members {
		compatible := true
		for _, child := range parsedChildren {
			memberChild, ok := deriveTypeFrom(resolver, member, child.segs)
			if !ok || memberChild == nil {
				compatible = false
				break
			}

			if child.narrowed.Kind() == kind.Literal {
				if lit, ok := child.narrowed.(*typ.Literal); ok {
					if childLit, ok := memberChild.(*typ.Literal); ok {
						if !typ.TypeEquals(childLit, lit) {
							compatible = false
							break
						}
					}
				}
			}
		}
		if compatible {
			kept = append(kept, member)
		}
	}

	if len(kept) == 0 {
		return typ.Never
	}
	if len(kept) == 1 {
		return kept[0]
	}
	return typ.NewUnion(kept...)
}

// deriveTypeFrom extracts a nested type from base following path segments.
func deriveTypeFrom(resolver narrow.Resolver, base typ.Type, segs []constraint.Segment) (typ.Type, bool) {
	if base == nil || resolver == nil {
		return nil, false
	}

	current := base
	for _, seg := range segs {
		switch seg.Kind {
		case constraint.SegmentField:
			next, ok := resolver.Field(current, seg.Name)
			if ok && !isOpenRecordFieldMiss(current, next) {
				current = next
				break
			}
			key := typ.LiteralString(seg.Name)
			if idxNext, idxOk := resolver.Index(current, key); idxOk {
				current = idxNext
				break
			}
			if !ok {
				return nil, false
			}
			current = next
		case constraint.SegmentIndexString:
			if next, ok := resolver.Field(current, seg.Name); ok && !isOpenRecordFieldMiss(current, next) {
				current = next
				break
			}
			key := typ.LiteralString(seg.Name)
			next, ok := resolver.Index(current, key)
			if !ok {
				return nil, false
			}
			current = next
		case constraint.SegmentIndexInt:
			key := typ.LiteralInt(int64(seg.Index))
			next, ok := resolver.Index(current, key)
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
		if current == nil {
			return nil, false
		}
	}
	return current, true
}

func isOpenRecordFieldMiss(base typ.Type, result typ.Type) bool {
	if !typ.IsUnknown(result) {
		return false
	}
	rec, ok := base.(*typ.Record)
	return ok && rec.Open
}
