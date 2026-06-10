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

// ProjectedTypeAt returns the condition-proof type visible at key, including
// structural implications from narrowed ancestors and child paths.
func (d *ConditionProofDomain) ProjectedTypeAt(key constraint.PathKey, resolver narrow.Resolver) typ.Type {
	if d == nil || key == "" {
		return nil
	}
	ancestor, hasAncestor := d.deriveFromNarrowedAncestors(key, resolver)
	return d.projectedTypeAtWithEvidence(key, resolver, ancestor, hasAncestor, d.NarrowedChildPaths(key))
}

func (d *ConditionProofDomain) projectedTypeAtWithEvidence(
	key constraint.PathKey,
	resolver narrow.Resolver,
	ancestor typ.Type,
	hasAncestor bool,
	children map[constraint.PathKey]typ.Type,
) typ.Type {
	current := d.TypeAt(key)

	if hasAncestor {
		if current == nil {
			current = ancestor
		} else {
			current = narrow.Intersect(current, ancestor)
		}
	}

	if len(children) > 0 {
		current = filterByChildNarrowings(current, key, children, resolver)
	}

	return current
}

func (d *ConditionProofDomain) withProjectedJoinFacts(left, right *ConditionProofDomain) *ConditionProofDomain {
	if d == nil || left == nil || right == nil {
		return d
	}
	leftProjection := newConditionProjectionView(left)
	rightProjection := newConditionProjectionView(right)
	keys := conditionJoinSupportKeys(leftProjection, rightProjection)
	for _, key := range keys {
		leftType := leftProjection.ProjectedTypeAt(key)
		rightType := rightProjection.ProjectedTypeAt(key)
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

func conditionJoinSupportKeys(left, right conditionProjectionView) []constraint.PathKey {
	var keys pathKeyList
	keys.AddList(left.SupportKeys())
	keys.AddList(right.SupportKeys())
	return keys.SortedValues()
}

type conditionProjectionView struct {
	domain        *ConditionProofDomain
	resolver      narrow.Resolver
	narrowed      pathKeySet
	support       pathKeyList
	childByParent map[constraint.PathKey]map[constraint.PathKey]typ.Type
}

// newConditionProjectionView indexes one condition proof state's projection evidence for
// multi-key queries such as join. One-off reads can use ConditionProofDomain directly;
// joins should not rescan every narrowed path for every support key.
func newConditionProjectionView(d *ConditionProofDomain) conditionProjectionView {
	view := conditionProjectionView{
		domain:        d,
		childByParent: make(map[constraint.PathKey]map[constraint.PathKey]typ.Type),
	}
	if d == nil {
		return view
	}
	view.resolver = d.env.Resolver
	if d.Type != nil {
		for _, key := range constraint.SortedPathKeys(d.Type.Narrowed) {
			view.addNarrowing(key, d.Type.Narrowed[key])
		}
	}
	if d.Shape != nil {
		for _, key := range constraint.SortedPathKeys(d.Shape.Narrowed) {
			view.addNarrowing(key, d.Shape.Narrowed[key])
		}
	}
	return view
}

func (v conditionProjectionView) SupportKeys() []constraint.PathKey {
	return v.support.SortedValues()
}

func (v conditionProjectionView) ProjectedTypeAt(key constraint.PathKey) typ.Type {
	if v.domain == nil || key == "" {
		return nil
	}
	ancestor, hasAncestor := v.deriveFromNarrowedAncestors(key)
	return v.domain.projectedTypeAtWithEvidence(key, v.resolver, ancestor, hasAncestor, v.NarrowedChildPaths(key))
}

func (v conditionProjectionView) NarrowedChildPaths(parentKey constraint.PathKey) map[constraint.PathKey]typ.Type {
	if parentKey == "" || v.childByParent == nil {
		return nil
	}
	children := v.childByParent[parentKey]
	if len(children) == 0 {
		return nil
	}
	return children
}

func (v *conditionProjectionView) addNarrowing(key constraint.PathKey, t typ.Type) {
	if key == "" || t == nil {
		return
	}
	v.narrowed.Add(key)
	v.addSupportKey(key)
	for _, parent := range productProjectionAncestorKeys(key) {
		children := v.childByParent[parent]
		if children == nil {
			children = make(map[constraint.PathKey]typ.Type)
			v.childByParent[parent] = children
		}
		if existing, ok := children[key]; ok {
			children[key] = narrow.Intersect(existing, t)
		} else {
			children[key] = t
		}
	}
}

func (v conditionProjectionView) deriveFromNarrowedAncestors(targetKey constraint.PathKey) (typ.Type, bool) {
	if v.domain == nil || v.resolver == nil || targetKey == "" {
		return nil, false
	}
	targetSym, targetVersion, targetSuffix, ok := pathkey.ParseKeyUnchecked(targetKey)
	if !ok || targetSuffix == "" {
		return nil, false
	}
	targetSegs := pathkey.ParseSuffix(targetSuffix)
	var combined typ.Type
	for _, candidateKey := range productProjectionAncestorKeys(targetKey) {
		if !v.narrowed.Contains(candidateKey) {
			continue
		}
		ancestorType := v.domain.TypeAt(candidateKey)
		if ancestorType == nil {
			continue
		}
		sym, version, suffix, ok := pathkey.ParseKeyUnchecked(candidateKey)
		if !ok || sym != targetSym || version != targetVersion {
			continue
		}
		ancestorSegs := pathkey.ParseSuffix(suffix)
		if len(ancestorSegs) >= len(targetSegs) || !pathkey.SegmentsPrefix(ancestorSegs, targetSegs) {
			continue
		}
		remaining := targetSegs[len(ancestorSegs):]
		derived, ok := deriveTypeFrom(v.resolver, ancestorType, remaining)
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

func (v *conditionProjectionView) addSupportKey(key constraint.PathKey) {
	if key == "" {
		return
	}
	v.support.Add(key)
	root, suffix, ok := pathkey.ParseRootAndSuffix(key)
	if !ok || suffix == "" {
		return
	}
	segs := pathkey.ParseSuffix(suffix)
	for depth := len(segs) - 1; depth >= 0; depth-- {
		parent := constraint.PathKey(root + pathkey.SegmentsSuffix(segs[:depth]))
		v.support.Add(parent)
	}
}

func productProjectionAncestorKeys(key constraint.PathKey) []constraint.PathKey {
	root, suffix, ok := pathkey.ParseRootAndSuffix(key)
	if !ok || suffix == "" {
		return nil
	}
	segs := pathkey.ParseSuffix(suffix)
	if len(segs) == 0 {
		return nil
	}
	var out pathKeyList
	for depth := len(segs) - 1; depth >= 0; depth-- {
		parent := constraint.PathKey(root + pathkey.SegmentsSuffix(segs[:depth]))
		if parent != "" {
			out.Add(parent)
		}
	}
	return out.SortedValues()
}

func (d *ConditionProofDomain) baseTypeAt(key constraint.PathKey) typ.Type {
	if d == nil || key == "" {
		return nil
	}
	return d.env.LookupPathType(key)
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

func (d *ConditionProofDomain) deriveFromNarrowedAncestors(targetKey constraint.PathKey, resolver narrow.Resolver) (typ.Type, bool) {
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
