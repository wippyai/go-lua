package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// pathTypeOrigin records where the type of a path at a point comes from.
type pathTypeOrigin uint8

const (
	// pathTypeProjected: the type is projected from the path's root value or
	// declared type, as that value stands before any branch narrowing. A root
	// path's own type is never projected.
	pathTypeProjected pathTypeOrigin = iota
	// pathTypeRecorded: the type is a fact recorded for the path itself by an
	// assignment to it, valid until the next write to the path.
	pathTypeRecorded
)

// TypeAt returns the type for a path at a CFG point using canonical keys.
func (s *Solution) TypeAt(p cfg.Point, path constraint.Path) typ.Type {
	t, _ := s.typeAtWithOrigin(p, path)
	return t
}

// typeAtWithOrigin returns the type for a path at a CFG point and whether it is
// a fact recorded for the path or a projection from its root.
func (s *Solution) typeAtWithOrigin(p cfg.Point, path constraint.Path) (typ.Type, pathTypeOrigin) {
	if path.IsEmpty() || s.pkResolver == nil {
		return nil, pathTypeProjected
	}

	// Get canonical key for this path at this point
	fullKey := s.pkResolver.KeyAt(p, path)
	if fullKey == "" {
		// No version available - fall back to declared type
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			if len(path.Segments) == 0 {
				return declaredType, pathTypeProjected
			}
			if d, ok := s.deriveTypeFrom(declaredType, path.Segments); ok {
				return d, pathTypeProjected
			}
		}
		return nil, pathTypeProjected
	}

	// Get base key (path without segments)
	basePath := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	baseKey := s.pkResolver.KeyAt(p, basePath)

	full := s.values[string(fullKey)]
	base := s.values[string(baseKey)]

	if base == nil && full == nil {
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			if len(path.Segments) == 0 {
				return declaredType, pathTypeProjected
			}
			if d, ok := s.deriveTypeFrom(declaredType, path.Segments); ok {
				return d, pathTypeProjected
			}
		}
	}

	if len(path.Segments) == 0 {
		baseType := full
		if baseType == nil {
			baseType = base
		}
		// For annotated symbols, prefer the declared type as the base.
		// This keeps annotations authoritative while still allowing field overlays.
		if s.inputs != nil && s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[path.Symbol] {
			if declared := s.lookupDeclaredType(path); declared != nil {
				if baseType == nil || !subtype.IsSubtype(baseType, declared) {
					baseType = declared
				}
			}
		}
		return s.mergeFieldAssignments(baseType, string(baseKey)), pathTypeRecorded
	}

	var derived typ.Type
	if base != nil {
		if d, ok := s.deriveTypeFrom(base, path.Segments); ok {
			derived = d
		}
	}

	// A recorded any write describes the actual child value even when the
	// enclosing open record does not yet list that field. A nil projection
	// from the record cannot replace the dynamic value stored by the write.
	if full != nil && (derived == nil || typ.IsAny(full) || !full.Kind().IsPlaceholder()) {
		return full, pathTypeRecorded
	}
	return derived, pathTypeProjected
}

// ConditionAt returns the full DNF condition at a CFG point.
func (s *Solution) ConditionAt(p cfg.Point) constraint.Condition {
	if s.pointConditions == nil {
		return constraint.TrueCondition()
	}
	if cond, ok := s.pointConditions[p]; ok {
		return cond
	}
	return s.conditionAtFallback(p, 0)
}

func (s *Solution) conditionAtFallback(p cfg.Point, depth int) constraint.Condition {
	if typ.DepthExceeded(depth) {
		return constraint.TrueCondition()
	}
	if s.inputs == nil || s.inputs.Graph == nil {
		return constraint.TrueCondition()
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) != 1 {
		return constraint.TrueCondition()
	}
	pred := preds[0]
	var predCond constraint.Condition
	if s.pointConditions != nil {
		if c, ok := s.pointConditions[pred]; ok {
			predCond = c
		} else {
			predCond = s.conditionAtFallback(pred, depth+1)
		}
	} else {
		predCond = s.conditionAtFallback(pred, depth+1)
	}

	edgeCond := constraint.TrueCondition()
	if ec, ok := s.edgeConditions[edgeKey{from: pred, to: p}]; ok && (ec.HasConstraints() || ec.IsFalse()) {
		edgeCond = ec
	}

	if predCond.IsFalse() {
		return predCond
	}
	if edgeCond.IsTrue() {
		return predCond
	}
	return constraint.And(predCond, edgeCond)
}

// IsNonNilAt reports whether the condition at p proves the value at path
// present: every disjunct holds a Truthy or NotNil fact for it. It decides
// presence where the type cannot express it, as for a value typed any that a
// guard proved truthy.
func (s *Solution) IsNonNilAt(p cfg.Point, path constraint.Path) bool {
	if s == nil || s.pkResolver == nil || path.IsEmpty() {
		return false
	}
	cond := s.ConditionAt(p)
	if !cond.HasConstraints() || cond.IsFalse() {
		return false
	}
	want := s.pkResolver.KeyAt(p, path)
	if want == "" {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		// Copies preserve identity. Follow only equalities that hold in this
		// disjunct, so every reachable path must still prove presence.
		equalKeys := map[string]bool{string(want): true}
		items := cond.DisjunctConstraints(i)

		for changed := true; changed; {
			changed = false
			for _, c := range items {
				eq, ok := c.(constraint.EqPath)
				if !ok {
					continue
				}
				left := string(s.pkResolver.KeyAt(p, eq.Left))
				right := string(s.pkResolver.KeyAt(p, eq.Right))
				if left == "" || right == "" {
					continue
				}
				if equalKeys[left] && !equalKeys[right] {
					equalKeys[right], changed = true, true
				}
				if equalKeys[right] && !equalKeys[left] {
					equalKeys[left], changed = true, true
				}
			}
		}
		found := false
		for _, c := range items {
			var factPath constraint.Path
			switch v := c.(type) {
			case constraint.Truthy:
				factPath = v.Path
			case constraint.NotNil:
				factPath = v.Path
			default:
				continue
			}
			if equalKeys[string(s.pkResolver.KeyAt(p, factPath))] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ExcludesTypeAt checks if a NotHasType constraint applies to the path at point p.
func (s *Solution) ExcludesTypeAt(p cfg.Point, path constraint.Path, t typ.Type) bool {
	if s == nil || s.inputs == nil || t == nil {
		return false
	}
	cond := s.ConditionAt(p)
	return s.conditionExcludesType(cond, path, t)
}

func (s *Solution) conditionExcludesType(cond constraint.Condition, path constraint.Path, t typ.Type) bool {
	if cond.IsFalse() || !cond.HasConstraints() {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		disjunct := cond.DisjunctConstraints(i)
		found := false
		for _, c := range disjunct {
			if nht, ok := c.(constraint.NotHasType); ok {
				if s.pathMatches(nht.Path, path) && typeMatches(s.resolveTypeKey(nht.Type), t) {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *Solution) pathMatches(cpath constraint.Path, qpath constraint.Path) bool {
	if cpath.Symbol != 0 && qpath.Symbol != 0 {
		return cpath.Symbol == qpath.Symbol
	}
	if cpath.Symbol != 0 || qpath.Symbol != 0 {
		return false
	}
	if cpath.IsPlaceholder() {
		return cpath.Root == qpath.Root
	}
	return false
}

func typeMatches(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Hash() == b.Hash() {
		return true
	}
	switch a.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
		return a.Kind() == b.Kind()
	default:
		return false
	}
}

// ConstValueAt returns a constant value for a name at a CFG point.
func (s *Solution) ConstValueAt(p cfg.Point, name string) *ConstValue {
	if name == "" || s.inputs == nil || s.inputs.ConstValues == nil {
		return nil
	}
	sym, ok := s.inputs.Graph.SymbolAt(p, name)
	if !ok {
		return nil
	}
	return s.ConstValueAtSym(p, sym)
}

// ConstValueAtSym returns a constant value for a SymbolID at a CFG point.
func (s *Solution) ConstValueAtSym(p cfg.Point, sym cfg.SymbolID) *ConstValue {
	if sym == 0 || s.inputs == nil || s.inputs.ConstValues == nil {
		return nil
	}
	atPoints := s.inputs.ConstValues[sym]
	if atPoints == nil {
		return nil
	}
	val := atPoints[p]
	if val != nil && val.Kind == ConstUnknown {
		return nil
	}
	return val
}

// IsEdgeUnreachable returns true if edge's numeric constraints are unsatisfiable.
func (s *Solution) IsEdgeUnreachable(from, to cfg.Point) bool {
	return s.unsatEdges[edgeKey{from: from, to: to}]
}

// BoundsAt returns the integer bounds for a variable at a CFG point.
func (s *Solution) BoundsAt(p cfg.Point, name string) (lower, upper int64, ok bool) {
	if s == nil || s.numericStates == nil {
		return 0, 0, false
	}
	state := s.numericStates[p]
	if state == nil {
		return 0, 0, false
	}
	if s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil {
		return 0, 0, false
	}
	sym, found := s.inputs.Graph.SymbolAt(p, name)
	if !found || sym == 0 {
		return 0, 0, false
	}
	path := constraint.Path{Root: name, Symbol: sym}
	key := s.pkResolver.KeyAt(p, path)
	if key == "" {
		return 0, 0, false
	}
	return state.BoundsFor(key)
}

// ArrayLenBoundAt returns the array key if the variable has an array length upper bound.
func (s *Solution) ArrayLenBoundAt(p cfg.Point, varName string) (arrKey string, ok bool) {
	if s == nil || s.numericStates == nil {
		return "", false
	}
	state := s.numericStates[p]
	if state == nil {
		return "", false
	}
	if s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil {
		return "", false
	}
	sym, found := s.inputs.Graph.SymbolAt(p, varName)
	if !found || sym == 0 {
		return "", false
	}
	path := constraint.Path{Root: varName, Symbol: sym}
	key := s.pkResolver.KeyAt(p, path)
	if key == "" {
		return "", false
	}
	pathKey, ok := state.LenRefFor(key)
	return string(pathKey), ok
}

// ArrayLenBoundWithOffsetAt returns the array key and offset for a symbolic length bound.
func (s *Solution) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (arrKey string, offset int64, ok bool) {
	if s == nil || s.numericStates == nil {
		return "", 0, false
	}
	state := s.numericStates[p]
	if state == nil {
		return "", 0, false
	}
	if s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil {
		return "", 0, false
	}
	sym, found := s.inputs.Graph.SymbolAt(p, varName)
	if !found || sym == 0 {
		return "", 0, false
	}
	path := constraint.Path{Root: varName, Symbol: sym}
	key := s.pkResolver.KeyAt(p, path)
	if key == "" {
		return "", 0, false
	}
	pathKey, off, ok := state.LenRefWithOffsetFor(key)
	if !ok {
		return "", 0, false
	}
	return string(pathKey), off, true
}

// NarrowedTypeAt returns the type at point p for path, narrowed by the DNF condition.
// This is a pure query that composes: baseTypeAt + ConditionAt + applyCondition.
func (s *Solution) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil {
		return nil
	}
	cacheKey, cacheable := s.narrowedTypeCacheKey(p, path)
	if !s.queryCacheEnabled {
		cacheable = false
	}
	if cacheable {
		if s.narrowedTypeCache == nil {
			s.narrowedTypeCache = make(map[narrowedTypeCacheKey]narrowedTypeCacheValue)
		}
		if cached, ok := s.narrowedTypeCache[cacheKey]; ok {
			if cached.ok {
				return cached.t
			}
			return nil
		}
	}

	result := s.narrowedTypeUnder(p, path, s.ConditionAt(p))
	if cacheable {
		s.narrowedTypeCache[cacheKey] = narrowedTypeCacheValue{t: result, ok: result != nil}
	}
	return result
}

// narrowedTypeUnder returns the type at point p for path, narrowed by
// condition.
func (s *Solution) narrowedTypeUnder(p cfg.Point, path constraint.Path, condition constraint.Condition) typ.Type {
	baseType := s.baseTypeAt(p, path)
	if baseType == nil {
		return nil
	}
	// For annotated symbols, ensure base type does not drop required structure.
	// If the base type is not a subtype of the declared type, fall back to declared.
	if s.inputs != nil && len(path.Segments) == 0 && path.Symbol != 0 {
		if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[path.Symbol] {
			if declared := s.lookupDeclaredType(path); declared != nil {
				if !subtype.IsSubtype(baseType, declared) {
					baseType = declared
				}
			}
		}
	}
	if condition.IsFalse() {
		return typ.Never
	}
	if !condition.HasConstraints() {
		return baseType
	}
	baseType = s.narrowRecordFieldAliases(p, path, baseType, condition)
	return s.applyCondition(p, baseType, path, condition)
}

// narrowRecordFieldAliases carries a branch refinement through a field that
// was initialized from a scalar path, such as t.engine = engine. A refinement
// of engine also describes t.engine until that field is written again.
func (s *Solution) narrowRecordFieldAliases(p cfg.Point, path constraint.Path, t typ.Type, condition constraint.Condition) typ.Type {
	if len(path.Segments) != 0 || s.pkResolver == nil {
		return t
	}
	rec, ok := t.(*typ.Record)
	if !ok {
		return t
	}
	for _, field := range rec.Fields {
		sourceKey := s.aliasSourceKeyAt(p, path.Field(field.Name))
		if sourceKey == "" {
			continue
		}
		sym, version, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(sourceKey))
		if !ok || sym == 0 || version == 0 || suffix != "" || sym == path.Symbol {
			continue
		}
		sourcePath := constraint.Path{Symbol: sym, Version: version}
		sourceType := s.values[sourceKey]
		if sourceType == nil {
			sourceType = s.baseTypeAt(p, sourcePath)
		}
		if sourceType == nil {
			continue
		}
		narrowed := s.applyCondition(p, sourceType, sourcePath, condition)
		if narrowed != nil && subtype.IsSubtype(narrowed, field.Type) && !typ.TypeEquals(narrowed, field.Type) {
			field.Type = narrowed
			rec = rec.WithField(field)
		}
	}
	return rec
}

// NarrowTypeAt narrows t, a type the caller holds for path, by the condition
// reaching p. It serves callers that know the value's type from outside the
// solution, such as assignment inference running before the full solve.
func (s *Solution) NarrowTypeAt(p cfg.Point, path constraint.Path, t typ.Type) typ.Type {
	return s.NarrowTypeAssuming(p, path, t, constraint.TrueCondition())
}

// NarrowTypeAssuming narrows t, a type the caller holds for path, by the
// condition reaching p conjoined with extra.
func (s *Solution) NarrowTypeAssuming(p cfg.Point, path constraint.Path, t typ.Type, extra constraint.Condition) typ.Type {
	if s == nil || t == nil || path.IsEmpty() {
		return t
	}
	return s.applyCondition(p, t, path, constraint.And(s.ConditionAt(p), extra))
}

// NarrowedTypeAssuming returns the type at p for path narrowed by the
// condition reaching p conjoined with extra. An operand evaluated only when
// another operand's condition holds, such as the right operand of `and`, is
// typed with that condition as extra, so an expression guard narrows exactly
// as the same guard on a branch does.
func (s *Solution) NarrowedTypeAssuming(p cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	if s == nil {
		return nil
	}
	return s.narrowedTypeUnder(p, path, constraint.And(s.ConditionAt(p), extra))
}

func (s *Solution) narrowedTypeCacheKey(p cfg.Point, path constraint.Path) (narrowedTypeCacheKey, bool) {
	if path.IsEmpty() {
		return narrowedTypeCacheKey{}, false
	}

	if s.pkResolver != nil {
		if key := s.pkResolver.KeyAt(p, path); key != "" {
			return narrowedTypeCacheKey{point: p, path: key}, true
		}
	}

	if key := path.Key(); key != "" {
		return narrowedTypeCacheKey{point: p, path: key}, true
	}

	return narrowedTypeCacheKey{}, false
}

// baseTypeAt returns the base type for a path at point p, for use in narrowing.
//
// A child path has two sources: its own type at p (TypeAt) and the type derived
// from its closest narrowed ancestor. When the own type is only a projection of
// the root value as it stands before narrowing, the ancestor-derived type is the
// same projection taken after narrowing, so it wins. When the own type is a fact
// recorded by an assignment to the path, both describe the same value and the
// more specific one wins.
func (s *Solution) baseTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	explicit, origin := s.typeAtWithOrigin(p, path)

	if len(path.Segments) == 0 {
		return explicit
	}

	derived := s.derivedTypeAt(p, path)

	if explicit == nil {
		return derived
	}
	if derived == nil {
		return explicit
	}

	// A falsy derived type (nil/false) would narrow the path to never.
	if derived.Kind() == kind.Nil || isFalseLiteral(derived) {
		return explicit
	}

	if origin == pathTypeProjected {
		return derived
	}

	if derived.Kind().IsPlaceholder() && !explicit.Kind().IsPlaceholder() {
		return explicit
	}
	if explicit.Kind().IsPlaceholder() && !derived.Kind().IsPlaceholder() {
		return derived
	}
	if subtype.IsSubtype(explicit, derived) {
		return explicit
	}
	if subtype.IsSubtype(derived, explicit) {
		return derived
	}

	// An assignment of T to a path whose ancestor projects T? keeps T.
	if opt, ok := derived.(*typ.Optional); ok {
		if typ.TypeEquals(explicit, opt.Inner) {
			return explicit
		}
	}

	return derived
}

// derivedTypeAt derives a child path's type from the narrowed parent.
func (s *Solution) derivedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if len(path.Segments) == 0 {
		return nil
	}

	// Walk from the closest ancestor to the root and derive from the first
	// narrowed ancestor we can prove at this point. This preserves narrowing
	// learned on intermediate paths (for example, x.field ~= nil) when querying
	// deeper descendants (for example, x.field[1].value).
	for cut := len(path.Segments) - 1; cut >= 0; cut-- {
		ancestor := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
		if cut > 0 {
			ancestor.Segments = append(ancestor.Segments, path.Segments[:cut]...)
		}
		ancestorNarrowed := s.NarrowedTypeAt(p, ancestor)
		if ancestorNarrowed == nil {
			continue
		}
		derived, ok := s.deriveTypeFrom(ancestorNarrowed, path.Segments[cut:])
		if ok {
			return derived
		}
	}
	return nil
}

// isFalseLiteral returns true if t is literal false.
func isFalseLiteral(t typ.Type) bool {
	if t == nil {
		return false
	}
	lit, ok := t.(*typ.Literal)
	if !ok {
		return false
	}
	b, ok := lit.Value.(bool)
	return ok && !b
}

// applyCondition narrows baseType using a DNF condition.
func (s *Solution) applyCondition(p cfg.Point, baseType typ.Type, path constraint.Path, cond constraint.Condition) typ.Type {
	if baseType == nil {
		return baseType
	}
	if cond.IsFalse() {
		return typ.Never
	}
	if !cond.HasConstraints() || cond.IsTrue() {
		return baseType
	}

	var narrowedTypes []typ.Type
	for i := 0; i < cond.NumDisjuncts(); i++ {
		disjunct := cond.DisjunctConstraints(i)
		if len(disjunct) == 0 {
			narrowedTypes = append(narrowedTypes, baseType)
			continue
		}
		narrowed := s.applyConstraints(p, baseType, path, disjunct)
		if narrowed != nil && !narrowed.Kind().IsNever() {
			// A disjunct that leaves the value unresolved admits every value,
			// so the union over the disjuncts is unresolved too; NewUnion
			// would drop it as carrying no information.
			if typ.IsUnknown(narrowed) {
				return narrowed
			}
			narrowedTypes = append(narrowedTypes, narrowed)
		}
	}

	if len(narrowedTypes) == 0 {
		return typ.Never
	}
	if len(narrowedTypes) == 1 {
		return narrowedTypes[0]
	}
	return typ.PruneSoftUnionMembers(typ.NewUnion(narrowedTypes...))
}

// applyConstraints narrows baseType using constraints that apply to path at point p.
func (s *Solution) applyConstraints(p cfg.Point, baseType typ.Type, path constraint.Path, constraints []constraint.Constraint) typ.Type {
	if baseType == nil || len(constraints) == 0 {
		return baseType
	}
	constraints = pathkey.FilterConstraintsForPath(constraints, path)
	if len(constraints) == 0 {
		return baseType
	}

	// Use canonical versioned key only
	canonicalKey := s.pkResolver.KeyAt(p, path)
	if canonicalKey == "" {
		return baseType
	}

	valueMap := s.buildPointValueMap(p, path, baseType, constraints)
	resolvePathCache := s.scratchResolvedPathMap
	if resolvePathCache == nil {
		resolvePathCache = make(map[constraint.PathKey]constraint.PathKey, len(constraints)*2)
		s.scratchResolvedPathMap = resolvePathCache
	}
	clear(resolvePathCache)

	// Resolve constraint paths to canonical keys at query point p
	resolvePath := func(cpath constraint.Path) constraint.PathKey {
		cpath = normalizeConstraintPathForQuery(cpath)
		rawKey := cpath.Key()
		if resolved, ok := resolvePathCache[rawKey]; ok {
			return resolved
		}
		resolved := s.pkResolver.KeyAt(p, cpath)
		resolvePathCache[rawKey] = resolved
		return resolved
	}

	env := s.constraintEnv()
	env.PathTypeAt = func(key constraint.PathKey) typ.Type {
		return valueMap[key]
	}
	env.ResolvePath = resolvePath

	dom := NewProductDomain(env)
	if !dom.ApplyCondition(constraint.FromConstraints(constraints...)) {
		return nil
	}

	current := baseType
	if narrowed := dom.TypeAt(canonicalKey); narrowed != nil {
		current = narrowed
	}

	if narrowed, ok := s.deriveFromNarrowedAncestors(canonicalKey, dom); ok {
		if current == nil {
			current = narrowed
		} else {
			current = narrow.Intersect(current, narrowed)
		}
	}

	childNarrowings := dom.NarrowedChildPaths(canonicalKey)
	if len(childNarrowings) > 0 {
		current = s.filterByChildNarrowings(current, path, childNarrowings)
	}

	return current
}

// deriveFromNarrowedAncestors projects narrowed ancestor path types down to a target key.
//
// Example: if `x.y` is narrowed to non-nil record, querying `x.y.z` should derive
// `z` from that narrowed ancestor even when `x.y.z` has no direct narrowing entry.
func (s *Solution) deriveFromNarrowedAncestors(targetKey constraint.PathKey, dom *ProductDomain) (typ.Type, bool) {
	targetSym, targetVersion, targetSuffix, ok := pathkey.ParseKeyUnchecked(targetKey)
	if !ok {
		return nil, false
	}
	targetSegs := s.parseSuffixCached(targetSuffix)

	seen := make(map[constraint.PathKey]bool)
	candidates := make([]constraint.PathKey, 0, len(dom.Type.Narrowed)+len(dom.Shape.Narrowed))
	for _, key := range constraint.SortedPathKeys(dom.Type.Narrowed) {
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, key)
	}
	for _, key := range constraint.SortedPathKeys(dom.Shape.Narrowed) {
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, key)
	}

	var combined typ.Type
	for _, candidateKey := range candidates {
		ancestorType := dom.TypeAt(candidateKey)
		if ancestorType == nil {
			continue
		}
		sym, version, suffix, ok := pathkey.ParseKeyUnchecked(candidateKey)
		if !ok || sym != targetSym || version != targetVersion {
			continue
		}
		ancestorSegs := s.parseSuffixCached(suffix)
		if len(ancestorSegs) >= len(targetSegs) {
			continue
		}
		if !pathkey.SegmentsPrefix(ancestorSegs, targetSegs) {
			continue
		}

		remaining := targetSegs[len(ancestorSegs):]
		derived, ok := s.deriveTypeFrom(ancestorType, remaining)
		if !ok || derived == nil {
			continue
		}
		if combined == nil {
			combined = derived
		} else {
			combined = narrow.Intersect(combined, derived)
		}
	}

	if combined == nil {
		return nil, false
	}
	return combined, true
}

func normalizeConstraintPathForQuery(path constraint.Path) constraint.Path {
	if path.Symbol == 0 {
		return path
	}
	// Conditions are propagated through CFG edges and can keep historical SSA
	// version ids. At query time we must interpret related constraint paths at
	// the point's visible version; assignment-kill already handles stale facts.
	path.Version = 0
	return path
}

// filterByChildNarrowings filters a type to variants where child paths match narrowed types.
func (s *Solution) filterByChildNarrowings(baseType typ.Type, parentPath constraint.Path, children map[constraint.PathKey]typ.Type) typ.Type {
	u, ok := baseType.(*typ.Union)
	if !ok {
		return baseType
	}

	parentSym := parentPath.Symbol

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
		segs := s.parseSuffixCached(suffix)
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
			memberChild, ok := s.deriveTypeFrom(member, child.segs)
			if !ok || memberChild == nil {
				// A missing field can read as nil. A nil constraint therefore
				// cannot rule out a member without a projected field type.
				if child.narrowed.Kind() == kind.Nil {
					continue
				}
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

// IsPointDead returns true if the given CFG point is unreachable due to divergence.
func (s *Solution) IsPointDead(p cfg.Point) bool {
	if s == nil || s.inputs == nil || s.inputs.DeadPoints == nil {
		return false
	}
	return s.inputs.DeadPoints[p]
}

// HasKeyOf checks if a KeyOf constraint exists at point p for the given table
// and key paths, read as operands of the statement at p.
func (s *Solution) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if s == nil {
		return false
	}
	return s.hasKeyOfUnder(p, tablePath, keyPath, s.ConditionAt(p))
}

// HasKeyOfAssuming checks for a KeyOf fact under the condition reaching p
// conjoined with extra.
func (s *Solution) HasKeyOfAssuming(p cfg.Point, tablePath, keyPath constraint.Path, extra constraint.Condition) bool {
	if s == nil {
		return false
	}
	return s.hasKeyOfUnder(p, tablePath, keyPath, constraint.And(s.ConditionAt(p), extra))
}

func (s *Solution) hasKeyOfUnder(p cfg.Point, tablePath, keyPath constraint.Path, cond constraint.Condition) bool {
	if s.pkResolver == nil || !cond.HasConstraints() {
		return false
	}
	resolve := func(path constraint.Path) constraint.PathKey {
		return s.pkResolver.KeyAt(p, s.operandPath(p, path))
	}
	return constraint.HasKeyOfConstraint(cond, tablePath, keyPath, resolve)
}

// operandPath pins path to the version its symbol has as an operand of the
// statement at p. The statement reads its operands before it writes, so when
// it assigns the symbol, as t[k] = t[k] + 1 assigns t, the operand is the
// version its predecessors agree on rather than the one visible after p.
func (s *Solution) operandPath(p cfg.Point, path constraint.Path) constraint.Path {
	if path.Symbol == 0 || path.Version != 0 || s.inputs == nil || s.inputs.Graph == nil {
		return path
	}
	after := s.inputs.Graph.VisibleVersion(p, path.Symbol)
	var before cfg.Version
	for i, pred := range graphPredecessors(s.inputs.Graph, p) {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if i == 0 {
			before = ver
			continue
		}
		if ver.ID != before.ID {
			return path
		}
	}
	if before.IsZero() || before.ID == after.ID {
		return path
	}
	path.Version = before.ID
	return path
}

func (s *Solution) parseSuffixCached(suffix string) []constraint.Segment {
	if suffix == "" {
		return nil
	}
	cache := s.scratchParsedSuffixes
	if cache == nil {
		cache = make(map[string][]constraint.Segment, 32)
		s.scratchParsedSuffixes = cache
	}
	if segs, ok := cache[suffix]; ok {
		return segs
	}
	segs := pathkey.ParseSuffix(suffix)
	cache[suffix] = segs
	return segs
}
