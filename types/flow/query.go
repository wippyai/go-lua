package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/join"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeAt returns the type for a path at a CFG point using canonical keys.
func (s *Solution) TypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if path.IsEmpty() || s.pkResolver == nil {
		return nil
	}

	// Get canonical key for this path at this point
	fullKey := s.pkResolver.KeyAt(p, path)
	if fullKey == "" {
		// No version available - fall back to declared type
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			if len(path.Segments) == 0 {
				return declaredType
			}
			if d, ok := s.deriveTypeFrom(declaredType, path.Segments); ok {
				return d
			}
		}
		return nil
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
				return declaredType
			}
			if d, ok := s.deriveTypeFrom(declaredType, path.Segments); ok {
				return d
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
		return s.mergeFieldAssignments(baseType, string(baseKey))
	}

	var derived typ.Type
	if base != nil {
		if d, ok := s.deriveTypeFrom(base, path.Segments); ok {
			derived = d
		}
	}

	if full != nil && full.Kind().IsPlaceholder() && derived != nil {
		return derived
	}
	if derived != nil && derived.Kind().IsPlaceholder() && full != nil {
		return full
	}
	if full != nil {
		return full
	}
	return derived
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
	preds := s.inputs.Graph.Predecessors(p)
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

// NarrowedTypeAt returns the type at point p for path, narrowed by the DNF condition.
// This is a pure query that composes: baseTypeAt + ConditionAt + applyCondition.
func (s *Solution) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil {
		return nil
	}

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

	condition := s.ConditionAt(p)
	if !condition.HasConstraints() {
		return baseType
	}

	result := s.applyCondition(p, baseType, path, condition)
	return result
}

// baseTypeAt returns the base type for a path at point p, for use in narrowing.
// For child paths: prefers the narrower of explicit (TypeAt) vs parent-derived.
func (s *Solution) baseTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	explicit := s.TypeAt(p, path)

	if len(path.Segments) == 0 {
		return explicit
	}

	// For child paths, derive from narrowed parent
	derived := s.derivedTypeAt(p, path)

	// Use whichever type is available; if both, prefer the narrower one
	if explicit == nil {
		return derived
	}
	if derived == nil {
		return explicit
	}

	// Both available: prefer the narrower one
	// If derived is falsy (nil/false), prefer explicit to avoid narrowing to never
	if derived.Kind() == kind.Nil || isFalseLiteral(derived) {
		return explicit
	}

	// If explicit is narrower (e.g., string vs string?), prefer explicit
	// This happens when a field assignment provides a flow-narrowed type
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
	parentPath := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	parentNarrowed := s.NarrowedTypeAt(p, parentPath)
	if parentNarrowed == nil {
		return nil
	}
	derived, ok := s.deriveTypeFrom(parentNarrowed, path.Segments)
	if !ok {
		return nil
	}
	return derived
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
	if baseType == nil || !cond.HasConstraints() {
		return baseType
	}
	if cond.IsTrue() {
		return baseType
	}
	if cond.IsFalse() {
		return typ.Never
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
			narrowedTypes = append(narrowedTypes, narrowed)
		}
	}

	if len(narrowedTypes) == 0 {
		return typ.Never
	}
	if len(narrowedTypes) == 1 {
		return narrowedTypes[0]
	}
	return join.Types(narrowedTypes...)
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

	// Resolve constraint paths to canonical keys at query point p
	resolvePath := func(cpath constraint.Path) constraint.PathKey {
		return s.pkResolver.KeyAt(p, cpath)
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

	// Look up narrowed type using canonical key
	if narrowed := dom.TypeAt(canonicalKey); narrowed != nil {
		return narrowed
	}

	childNarrowings := dom.NarrowedChildPaths(canonicalKey)
	if len(childNarrowings) > 0 {
		return s.filterByChildNarrowings(baseType, path, childNarrowings)
	}

	return baseType
}

// filterByChildNarrowings filters a type to variants where child paths match narrowed types.
func (s *Solution) filterByChildNarrowings(baseType typ.Type, parentPath constraint.Path, children map[constraint.PathKey]typ.Type) typ.Type {
	u, ok := baseType.(*typ.Union)
	if !ok {
		return baseType
	}

	// Extract parent symbol ID from canonical key format
	parentSym := parentPath.Symbol
	var kept []typ.Type
	keys := make([]constraint.PathKey, 0, len(children))
	for key := range children {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, member := range u.Members {
		compatible := true
		for _, childKey := range keys {
			narrowedChild := children[childKey]
			// Parse child key to extract suffix
			childSym, _, suffix, ok := pathkey.ParseKey(childKey)
			if !ok || childSym != parentSym {
				continue
			}
			segs := pathkey.ParseSuffix(suffix)
			if len(segs) == 0 {
				continue
			}

			memberChild, ok := s.deriveTypeFrom(member, segs)
			if !ok || memberChild == nil {
				compatible = false
				break
			}

			if narrowedChild.Kind() == kind.Literal {
				if lit, ok := narrowedChild.(*typ.Literal); ok {
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

// HasKeyOf checks if a KeyOf constraint exists at point p for the given table and key paths.
func (s *Solution) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if s == nil || s.pkResolver == nil {
		return false
	}
	cond := s.ConditionAt(p)
	if !cond.HasConstraints() {
		return false
	}
	resolve := func(path constraint.Path) constraint.PathKey {
		return s.pkResolver.KeyAt(p, path)
	}
	return constraint.HasKeyOfConstraint(cond, tablePath, keyPath, resolve)
}
