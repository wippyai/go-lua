package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// PathFact is a finite abstract-state fact for a concrete flow path.
type PathFact struct {
	Path constraint.Path
	Type typ.Type
}

// TypeAt returns the type for a path at a CFG point using canonical keys.
func (s *Solution) TypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if path.IsEmpty() || s.pkResolver == nil {
		return nil
	}

	if len(path.Segments) == 0 {
		baseType, baseKey := s.rootBaseTypeAt(p, path)
		if baseKey == "" {
			return baseType
		}
		return s.mergeFieldAssignmentsAt(p, baseType, string(baseKey))
	}

	// Get canonical key for this path at this point
	fullKey := s.pkResolver.KeyAt(p, path)
	if fullKey == "" {
		// No visible version exists; use declared evidence.
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			if len(path.Segments) == 0 {
				return declaredType
			}
			if d, ok := deriveTypeFrom(s.resolver, declaredType, path.Segments); ok {
				return d
			}
		}
		return nil
	}

	// Get base key (path without segments)
	basePath := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	baseKey := s.pkResolver.KeyAt(p, basePath)

	full := s.projectedValueAtPoint(p, string(fullKey))
	base := s.valueAtPoint(p, string(baseKey))
	if base != nil && typ.IsUnknown(base) {
		rootDeclared := s.lookupDeclaredType(basePath)
		if rootDeclared != nil && !typ.IsUnknown(rootDeclared) {
			base = rootDeclared
		}
	}

	if base == nil && full == nil {
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			if len(path.Segments) == 0 {
				return declaredType
			}
			if d, ok := deriveTypeFrom(s.resolver, declaredType, path.Segments); ok {
				return d
			}
		}
	}

	var derived typ.Type
	if base != nil {
		if d, ok := deriveTypeFrom(s.resolver, base, path.Segments); ok {
			derived = d
		}
	}

	if full != nil && full.Kind().IsPlaceholder() && derived != nil {
		full = derived
	}
	if derived != nil && derived.Kind().IsPlaceholder() && full != nil {
		derived = full
	}

	var candidate typ.Type
	if full != nil {
		candidate = full
	} else {
		candidate = derived
	}
	return s.projectPresenceAtPoint(p, string(fullKey), candidate)
}

// reconcileFieldPathAgainstAnnotatedDeclared restores declared optionality on a
// field-path value read off an ANNOTATED root. A literal constructed under an
// explicit annotation seeds precise non-nil field facts; reading a field whose
// declared type permits nil must keep the precise shape but stay optional, since
// the construction is not a nil-guard. Unannotated roots carry no declared
// contract and are left untouched so guard-narrowed values keep their proof.
func (s *Solution) reconcileFieldPathAgainstAnnotatedDeclared(path constraint.Path, candidate typ.Type) typ.Type {
	if candidate == nil || len(path.Segments) == 0 || path.Symbol == 0 || s.inputs == nil {
		return candidate
	}
	// Use the explicit declared annotation as the contract. An annotated root's
	// DeclaredTypes entry is the source annotation (its literal initializer lives
	// separately in LiteralTypes), so a field whose declared type permits nil keeps
	// that presence even when the flow tracks a precise non-nil construction value.
	declaredRoot := s.inputs.DeclaredTypes[path.Symbol]
	if declaredRoot == nil {
		return candidate
	}
	declaredField, ok := deriveTypeFrom(s.resolver, declaredRoot, path.Segments)
	if !ok || declaredField == nil {
		return candidate
	}
	if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(candidate, declaredField); ok && reconciled != nil {
		return reconciled
	}
	return candidate
}

func (s *Solution) rootBaseTypeAt(p cfg.Point, path constraint.Path) (typ.Type, constraint.PathKey) {
	if s == nil || path.IsEmpty() || len(path.Segments) != 0 || s.pkResolver == nil {
		return nil, ""
	}

	fullKey := s.pkResolver.KeyAt(p, path)
	if fullKey == "" {
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			return declaredType, ""
		}
		return nil, ""
	}

	basePath := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	baseKey := s.pkResolver.KeyAt(p, basePath)

	full := s.valueAtPoint(p, string(fullKey))
	base := s.valueAtPoint(p, string(baseKey))
	if base != nil && typ.IsUnknown(base) {
		rootDeclared := s.lookupDeclaredType(basePath)
		if rootDeclared != nil && !typ.IsUnknown(rootDeclared) {
			base = rootDeclared
		}
	}

	if base == nil && full == nil {
		declaredType := s.lookupDeclaredType(path)
		if declaredType != nil {
			return declaredType, baseKey
		}
	}

	baseType := full
	if baseType == nil {
		baseType = base
	}

	// For annotated symbols, prefer sealed declarations as the base. Refinable
	// structural annotations still admit more precise flow evidence for their
	// placeholder slots.
	if s.inputs != nil && s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[path.Symbol] {
		if declared := s.lookupDeclaredType(path); declared != nil {
			if baseType == nil || !annotationAcceptsFlowType(declared, baseType) {
				baseType = declared
			}
		}
	}
	if typ.IsUnknown(baseType) {
		if declared := s.lookupDeclaredType(path); declared != nil && !typ.IsUnknown(declared) {
			baseType = declared
		}
	}
	if declared := s.lookupDeclaredType(path); declared != nil && baseType != nil && !typ.TypeEquals(baseType, declared) {
		if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(baseType, declared); ok && reconciled != nil {
			baseType = reconciled
		}
	}

	return baseType, baseKey
}

// ConditionAt returns the full DNF condition at a CFG point.
func (s *Solution) ConditionAt(p cfg.Point) constraint.Condition {
	if s.pointConditions == nil {
		return constraint.TrueCondition()
	}
	if cond, ok := s.pointConditions[p]; ok {
		return cond
	}
	return s.conditionAtLinearPredecessor(p, 0)
}

func (s *Solution) conditionAtLinearPredecessor(p cfg.Point, depth int) constraint.Condition {
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
			predCond = s.conditionAtLinearPredecessor(pred, depth+1)
		}
	} else {
		predCond = s.conditionAtLinearPredecessor(pred, depth+1)
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

// ProvesTypeAt reports whether path conditions at point p prove that path is a
// subtype of t. This is a condition-only proof query: it does not consult
// declared types or inferred body overlays, so callers can distinguish
// locally-proven refinements from caller preconditions.
func (s *Solution) ProvesTypeAt(p cfg.Point, path constraint.Path, t typ.Type) bool {
	if s == nil || s.inputs == nil || t == nil {
		return false
	}
	cond := s.ConditionAt(p)
	return s.conditionProvesType(cond, path, t)
}

// ConditionTypeAt returns the path type proven by propagated branch conditions
// at point p.
//
// This is the proof/query sibling of NarrowedTypeAt. It deliberately builds a
// finite environment from declared/literal/sibling inputs plus the queried base
// path, then applies the point condition through ProductDomain. It does not call
// NarrowedTypeAt, baseTypeAt, or transfer-derived ancestor queries, so preflow
// evidence synthesis can ask branch-proof questions without re-entering full
// abstract-state transfer.
func (s *Solution) ConditionTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	return s.conditionedTypeAt(p, path, nil)
}

// ConditionedTypeAt returns the path type proven by the point condition plus
// an additional expression-local condition. It is the proof-query surface for
// short-circuit expressions: callers provide the local guard, while the flow
// domain owns canonical path resolution, product-domain projection, and bottom
// handling.
func (s *Solution) ConditionedTypeAt(p cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	return s.conditionedTypeAt(p, path, &extra)
}

// ConditionedSeedTypeAt returns the type of queryPath proven from a caller
// supplied seed product under the point condition plus an expression-local
// condition.
//
// This is the canonical projection for pre-transfer expression scopes where the
// root type comes from an overlay (for example a loop variable or parameter
// product) instead of DeclaredTypes. The flow product domain still owns path
// resolution, condition application, ancestor narrowing, and descendant
// projection.
func (s *Solution) ConditionedSeedTypeAt(p cfg.Point, seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path, extra constraint.Condition) typ.Type {
	if s == nil || s.pkResolver == nil || seedPath.IsEmpty() || queryPath.IsEmpty() || seedType == nil {
		return nil
	}
	cond := s.ConditionAt(p)
	if extra.HasConstraints() || extra.IsFalse() {
		cond = constraint.And(cond, extra)
	}
	if cond.IsFalse() {
		return typ.Never
	}
	if cond.IsTrue() || !cond.HasConstraints() {
		return s.deriveSeedType(seedPath, seedType, queryPath)
	}
	projected := s.projectPointConditionForPath(p, seedPath, cond)
	if !projected.HasConstraints() {
		return s.deriveSeedType(seedPath, seedType, queryPath)
	}
	return s.applyConditionProof(p, seedType, seedPath, queryPath, projected)
}

func (s *Solution) deriveSeedType(seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path) typ.Type {
	if seedPath.Equal(queryPath) {
		return seedType
	}
	if !isDescendantOf(queryPath, seedPath) {
		return nil
	}
	relative := queryPath.Segments[len(seedPath.Segments):]
	derived, ok := deriveTypeFrom(s.resolver, seedType, relative)
	if !ok {
		return nil
	}
	return derived
}

func (s *Solution) conditionedTypeAt(p cfg.Point, path constraint.Path, extra *constraint.Condition) typ.Type {
	if s == nil || s.pkResolver == nil || path.IsEmpty() {
		return nil
	}
	cond := s.ConditionAt(p)
	if extra != nil {
		cond = constraint.And(cond, *extra)
	}
	if cond.IsFalse() {
		return typ.Never
	}
	rootPath := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	rootType := s.conditionRootTypeAt(rootPath)
	if rootType == nil {
		return nil
	}
	if cond.IsTrue() || !cond.HasConstraints() {
		if len(path.Segments) == 0 {
			return rootType
		}
		return nil
	}
	projected := s.projectPointConditionForPath(p, rootPath, cond)
	if !projected.HasConstraints() {
		if len(path.Segments) == 0 {
			return rootType
		}
		return nil
	}
	return s.applyConditionProof(p, rootType, rootPath, path, projected)
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

func (s *Solution) conditionProvesType(cond constraint.Condition, path constraint.Path, t typ.Type) bool {
	if cond.IsFalse() || !cond.HasConstraints() {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		disjunct := cond.DisjunctConstraints(i)
		found := false
		for _, c := range disjunct {
			ht, ok := c.(constraint.HasType)
			if !ok || !s.pathMatches(ht.Path, path) {
				continue
			}
			resolved := s.resolveTypeKey(ht.Type)
			if typeMatches(resolved, t) || subtype.IsSubtype(resolved, t) {
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
	if s == nil || s.numericAt == nil {
		return 0, 0, false
	}
	state := s.numericAt[p]
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
	if s == nil || s.numericAt == nil {
		return "", false
	}
	state := s.numericAt[p]
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
	if s == nil || s.numericAt == nil {
		return "", 0, false
	}
	state := s.numericAt[p]
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

// LengthBoundsAt returns numeric bounds for len(path) at a CFG point.
func (s *Solution) LengthBoundsAt(p cfg.Point, path constraint.Path) (lower, upper int64, ok bool) {
	if s == nil || s.numericAt == nil {
		return 0, 0, false
	}
	state := s.numericAt[p]
	if state == nil {
		return 0, 0, false
	}
	if s.pkResolver == nil || path.IsEmpty() {
		return 0, 0, false
	}
	key := s.pkResolver.KeyAt(p, path)
	if key == "" {
		return 0, 0, false
	}
	return state.LenBoundsFor(key)
}

// NarrowedTypeAt returns the type at point p for path, narrowed by the DNF condition.
// This is a pure query that composes: baseTypeAt + ConditionAt + applyCondition.
func (s *Solution) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil {
		return nil
	}
	cacheKey, cacheable := s.narrowedTypeCacheKey(p, path)
	if cacheable {
		if s.narrowedTypeCache == nil {
			s.narrowedTypeCache = make(map[narrowedTypeCacheKey]narrowedTypeCacheValue)
		}
		if cached, ok := s.narrowedTypeCache[cacheKey]; ok {
			if cached.epoch == s.stateEpoch {
				if cached.ok {
					return cached.t
				}
				return nil
			}
		}
	}

	result := s.narrowedTypeAtWithCondition(p, path, nil)
	if cacheable {
		s.narrowedTypeCache[cacheKey] = narrowedTypeCacheValue{epoch: s.stateEpoch, t: result, ok: true}
	}
	return result
}

// NarrowedTypeAtWithCondition returns the narrowed type at point p after
// applying the point condition plus an expression-local condition.
//
// This is the canonical query for short-circuit expression scopes. Callers
// provide only the condition; the flow solution owns base-type selection,
// product-domain projection, annotation policy, numeric shape projection, and
// bottom handling.
func (s *Solution) NarrowedTypeAtWithCondition(p cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	return s.narrowedTypeAtWithCondition(p, path, &extra)
}

func (s *Solution) narrowedTypeAtWithCondition(p cfg.Point, path constraint.Path, extra *constraint.Condition) typ.Type {
	if s == nil {
		return nil
	}
	condition := s.ConditionAt(p)
	if extra != nil {
		condition = constraint.And(condition, *extra)
	}
	if condition.IsFalse() || !s.isPointReachable(p) {
		return typ.Never
	}

	baseType := s.baseTypeAt(p, path)
	if baseType == nil {
		if extra != nil {
			return s.conditionedTypeAt(p, path, extra)
		}
		return nil
	}
	// For annotated symbols, ensure base type does not drop required structure.
	// Refinable structural annotations can be replaced by comparable, more
	// precise flow evidence for their placeholder slots.
	if s.inputs != nil && len(path.Segments) == 0 && path.Symbol != 0 {
		if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[path.Symbol] {
			if declared := s.lookupDeclaredType(path); declared != nil {
				if !annotationAcceptsFlowType(declared, baseType) {
					baseType = declared
				}
			}
		}
	}

	if !condition.HasConstraints() {
		return s.applyPointNumericShapeProjection(p, path, baseType)
	}

	result := s.applyPointCondition(p, baseType, path, condition)
	if result == nil && extra != nil {
		if proof := s.conditionedTypeAt(p, path, extra); proof != nil {
			return proof
		}
	}
	return s.applyPointNumericShapeProjection(p, path, result)
}

func annotationAcceptsFlowType(declared, candidate typ.Type) bool {
	if candidate == nil {
		return false
	}
	// any is the gradual top: an explicit any annotation is a dynamic boundary
	// that erases the structure of whatever value flows into it. Keeping the
	// precise flow value as the base would let a field read off the any root
	// recover the concrete value type, defeating the boundary. The declared any
	// must win so reads through it stay dynamic.
	if typ.IsAny(declared) {
		return typ.IsAny(candidate)
	}
	if subtype.IsSubtype(candidate, declared) {
		return true
	}
	if !typ.IsRefinableAnnotation(declared) {
		return false
	}
	_, comparable := typ.ComparePrecision(candidate, declared)
	return comparable
}

// PreStateTypeAt returns the path type at the entry side of point p.
//
// Flow transfer effects attached to p describe the state after executing that
// point. Call-boundary consumers that need the caller environment before the
// callee's effects run should use this query instead of NarrowedTypeAt.
func (s *Solution) PreStateTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil {
		return nil
	}
	if pre := s.preAssignmentNarrowedTypeAt(p, path); pre != nil {
		return pre
	}
	return s.NarrowedTypeAt(p, path)
}

// ChildTypesAt returns finite child facts already materialized in the abstract
// state below path at point p. It does not derive speculative descendants from
// the parent type; callers that need recursive products should project only
// these finite facts back into the root product.
func (s *Solution) ChildTypesAt(p cfg.Point, path constraint.Path) []PathFact {
	return s.childTypesAt(p, path, false)
}

// PreStateChildTypesAt returns finite child facts on the entry side of point p.
func (s *Solution) PreStateChildTypesAt(p cfg.Point, path constraint.Path) []PathFact {
	return s.childTypesAt(p, path, true)
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

func (s *Solution) childTypesAt(p cfg.Point, path constraint.Path, preState bool) []PathFact {
	if s == nil || path.IsEmpty() || path.Symbol == 0 {
		return nil
	}
	cacheKey, cacheable := s.childTypesCacheKey(p, path, preState)
	if !s.queryCacheEnabled {
		cacheable = false
	}
	if cacheable && s.childTypesCache != nil {
		if cached, ok := s.childTypesCache[cacheKey]; ok && cached.epoch == s.stateEpoch {
			return clonePathFacts(cached.facts)
		}
	}
	var facts []PathFact
	if preState {
		facts = s.preStateChildTypesAt(p, path)
	} else {
		facts = s.childTypesAtPoint(p, path)
	}
	if cacheable {
		if s.childTypesCache == nil {
			s.childTypesCache = make(map[childTypesCacheKey]childTypesCacheValue, 8)
		}
		s.childTypesCache[cacheKey] = childTypesCacheValue{
			epoch: s.stateEpoch,
			facts: clonePathFacts(facts),
		}
	}
	return facts
}

func (s *Solution) childTypesCacheKey(p cfg.Point, path constraint.Path, preState bool) (childTypesCacheKey, bool) {
	if path.IsEmpty() || path.Symbol == 0 {
		return childTypesCacheKey{}, false
	}
	key := path.Key()
	if key == "" {
		return childTypesCacheKey{}, false
	}
	return childTypesCacheKey{point: p, path: key, preState: preState}, true
}

func (s *Solution) preStateChildTypesAt(p cfg.Point, path constraint.Path) []PathFact {
	if s.inputs == nil || s.inputs.Graph == nil {
		return s.childTypesAtPoint(p, path)
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return s.childTypesAtPoint(p, path)
	}

	joined := make(map[string]PathFact)
	seenVersions := make(map[int]struct{}, len(preds))
	for _, pred := range preds {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if ver.ID == 0 {
			continue
		}
		if _, seen := seenVersions[ver.ID]; seen {
			continue
		}
		seenVersions[ver.ID] = struct{}{}
		predPath := path
		predPath.Version = ver.ID
		for _, fact := range s.childTypesAtPoint(pred, predPath) {
			if fact.Type == nil || len(fact.Path.Segments) == 0 {
				continue
			}
			key := constraint.FormatSegments(fact.Path.Segments[len(path.Segments):])
			if prev, ok := joined[key]; ok {
				prev.Type = join.Types(prev.Type, fact.Type)
				joined[key] = prev
				continue
			}
			joined[key] = fact
		}
	}
	if len(joined) == 0 {
		return s.childTypesAtPoint(p, path)
	}
	return sortedPathFacts(joined)
}

func (s *Solution) childTypesAtPoint(p cfg.Point, path constraint.Path) []PathFact {
	if s.pkResolver == nil || path.Symbol == 0 {
		return nil
	}
	baseKey := s.keyForPathAt(p, path)
	if baseKey == "" {
		return nil
	}
	baseSym, baseVersion, baseSuffix, ok := pathkey.ParseKeyUnchecked(baseKey)
	if !ok || baseSym == 0 || baseVersion == 0 {
		return nil
	}
	baseSegs := pathkey.ParseSuffix(baseSuffix)
	if len(baseSuffix) > 0 && baseSegs == nil {
		return nil
	}

	children := make(map[string]PathFact)
	visit := func(key string) {
		sym, version, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(key))
		if !ok || sym != baseSym || version != baseVersion || suffix == baseSuffix {
			return
		}
		segs := pathkey.ParseSuffix(suffix)
		if len(suffix) > 0 && segs == nil {
			return
		}
		if len(segs) <= len(baseSegs) || !pathkey.SegmentsPrefix(baseSegs, segs) {
			return
		}
		childSeg := segs[len(baseSegs)]
		childPath := path
		childPath.Version = baseVersion
		childPath.Segments = append(append([]constraint.Segment{}, path.Segments...), childSeg)
		childKey := constraint.FormatSegments([]constraint.Segment{childSeg})
		if _, seen := children[childKey]; seen {
			return
		}
		if t := s.TypeAt(p, childPath); t != nil {
			children[childKey] = PathFact{Path: childPath, Type: t}
		}
	}
	for key := range s.values {
		visit(key)
	}
	if state := s.mutableValues[p]; len(state) > 0 {
		for key := range state {
			visit(key)
		}
	}
	return sortedPathFacts(children)
}

func (s *Solution) keyForPathAt(p cfg.Point, path constraint.Path) constraint.PathKey {
	if s == nil || s.pkResolver == nil || path.Symbol == 0 {
		return ""
	}
	if path.Version != 0 {
		return s.pkResolver.KeyAtVersion(path.Symbol, path.Version, path.Segments)
	}
	return s.pkResolver.KeyAt(p, path)
}

func sortedPathFacts(facts map[string]PathFact) []PathFact {
	if len(facts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]PathFact, 0, len(keys))
	for _, key := range keys {
		out = append(out, facts[key])
	}
	return out
}

func clonePathFacts(facts []PathFact) []PathFact {
	if len(facts) == 0 {
		return nil
	}
	out := make([]PathFact, len(facts))
	copy(out, facts)
	return out
}

// baseTypeAt returns the base type for a path at point p, for use in narrowing.
// For child paths: prefers the narrower of explicit (TypeAt) vs parent-derived.
func (s *Solution) baseTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if derived, ok := s.gradualRootDescendantTypeAt(path); ok {
		return derived
	}

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

	if childEvidenceIsLessInformativeThanParent(explicit, derived) {
		return derived
	}

	if present, ok := meetPresentChildEvidence(explicit, derived); ok {
		return present
	}

	if s.activeConditionNarrowsAncestor(p, path) && !typ.IsAbsentOrUnknown(derived) && !derived.Kind().IsPlaceholder() {
		if subtype.IsSubtype(explicit, derived) {
			return explicit
		}
		return derived
	}

	// Direct child-path facts are the product-domain authority for that path.
	// Parent-derived facts describe container shape, but may be stale after a
	// direct field/index assignment or table mutator.
	if !explicit.Kind().IsPlaceholder() {
		return explicit
	}

	// If derived is falsy (nil/false), prefer explicit to avoid narrowing to never
	if derived.Kind() == kind.Nil || isFalseLiteral(derived) {
		return explicit
	}

	if explicit.Kind().IsPlaceholder() && !derived.Kind().IsPlaceholder() {
		return derived
	}

	// If one is a subtype of the other, keep the more specific type.
	if subtype.IsSubtype(explicit, derived) {
		return explicit
	}
	if subtype.IsSubtype(derived, explicit) {
		return derived
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

// gradualRootDescendantTypeAt projects the declared type of a descendant path
// whose annotated root is the gradual top any. An explicit any annotation is a
// dynamic boundary: a value flowing into it loses its structure, so every read
// through a descendant path stays any regardless of the precise child facts the
// initializer seeded. Reading the descendant off the precise initializer instead
// would recover concrete field types and silently defeat the boundary.
func (s *Solution) gradualRootDescendantTypeAt(path constraint.Path) (typ.Type, bool) {
	if s.inputs == nil || len(path.Segments) == 0 || path.Symbol == 0 {
		return nil, false
	}
	if s.inputs.AnnotatedVars == nil || !s.inputs.AnnotatedVars[path.Symbol] {
		return nil, false
	}
	declaredRoot := s.inputs.DeclaredTypes[path.Symbol]
	if !typ.IsAny(declaredRoot) {
		return nil, false
	}
	if derived, ok := deriveTypeFrom(s.resolver, declaredRoot, path.Segments); ok {
		return derived, true
	}
	return declaredRoot, true
}

func meetPresentChildEvidence(explicit, derived typ.Type) (typ.Type, bool) {
	if explicit == nil || derived == nil || derived.Kind() == kind.Nil {
		return nil, false
	}
	inner, optional := typ.SplitNilableFieldType(explicit)
	if !optional || inner == nil || inner.Kind() == kind.Nil {
		return nil, false
	}
	if _, derivedOptional := typ.SplitNilableFieldType(derived); derivedOptional {
		return nil, false
	}
	if typ.TypeEquals(inner, derived) || subtype.IsSubtype(derived, inner) {
		return derived, true
	}
	if subtype.IsSubtype(inner, derived) {
		return inner, true
	}
	return nil, false
}

func childEvidenceIsLessInformativeThanParent(explicit, derived typ.Type) bool {
	if explicit == nil || derived == nil || derived.Kind().IsPlaceholder() {
		return false
	}
	if isEmptyRecordNoMapType(explicit) {
		return typeCarriesContainerShape(derived)
	}
	return false
}

func (s *Solution) activeConditionNarrowsAncestor(p cfg.Point, path constraint.Path) bool {
	if s == nil || len(path.Segments) == 0 {
		return false
	}
	cond := s.ConditionAt(p)
	if !cond.HasConstraints() {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		for _, c := range cond.DisjunctConstraints(i) {
			if constraintNarrowsAncestorPath(c, path) {
				return true
			}
		}
	}
	return false
}

func constraintNarrowsAncestorPath(c constraint.Constraint, path constraint.Path) bool {
	matches := func(candidate constraint.Path) bool {
		return isStrictAncestorPath(candidate, path)
	}
	// Shape-only existence proofs must not make an ancestor snapshot override a
	// direct child fact; value-refining constraints below may.
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[bool]{
		Truthy:             func(v constraint.Truthy) bool { return matches(v.Path) },
		Falsy:              func(v constraint.Falsy) bool { return matches(v.Path) },
		IsNil:              func(v constraint.IsNil) bool { return matches(v.Path) },
		NotNil:             func(v constraint.NotNil) bool { return matches(v.Path) },
		HasType:            func(v constraint.HasType) bool { return matches(v.Path) },
		NotHasType:         func(v constraint.NotHasType) bool { return matches(v.Path) },
		HasField:           func(constraint.HasField) bool { return false },
		FieldEquals:        func(v constraint.FieldEquals) bool { return matches(v.Target) },
		FieldNotEquals:     func(v constraint.FieldNotEquals) bool { return matches(v.Target) },
		IndexEquals:        func(v constraint.IndexEquals) bool { return matches(v.Target) },
		IndexNotEquals:     func(v constraint.IndexNotEquals) bool { return matches(v.Target) },
		EqPath:             func(v constraint.EqPath) bool { return matches(v.Left) || matches(v.Right) },
		NotEqPath:          func(v constraint.NotEqPath) bool { return matches(v.Left) || matches(v.Right) },
		FieldEqualsPath:    func(v constraint.FieldEqualsPath) bool { return matches(v.Target) || matches(v.Value) },
		FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) bool { return matches(v.Target) || matches(v.Value) },
		IndexEqualsPath:    func(v constraint.IndexEqualsPath) bool { return matches(v.Target) || matches(v.Value) },
		IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) bool { return matches(v.Target) || matches(v.Value) },
		KeyOf:              func(v constraint.KeyOf) bool { return matches(v.Table) || matches(v.Key) },
		Default:            func(constraint.Constraint) bool { return false },
	})
}

func isStrictAncestorPath(ancestor, path constraint.Path) bool {
	if ancestor.IsEmpty() || path.IsEmpty() || len(ancestor.Segments) >= len(path.Segments) {
		return false
	}
	return pathkey.PathRelated(path, ancestor) && pathkey.SegmentsPrefix(ancestor.Segments, path.Segments)
}

func typeCarriesContainerShape(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return typeCarriesContainerShape(v.Target)
	case *typ.Array, *typ.Map:
		return true
	case *typ.Record:
		return len(v.Fields) > 0 || v.HasMapComponent()
	default:
		return false
	}
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
		ancestorNarrowed := s.narrowedAncestorBaseTypeAt(p, ancestor)
		if ancestorNarrowed == nil {
			continue
		}
		derived, ok := s.deriveTypeFromAt(p, ancestor, ancestorNarrowed, path.Segments[cut:])
		if ok {
			return derived
		}
	}
	return nil
}

// deriveTypeFromAt projects relative segments from a narrowed ancestor like
// deriveTypeFrom, but applies the solved index-read presence proof at every
// int-index step. A sequence element c[k] is nil-eligible by default; the
// container's proven length lower bound (literal/length fact) removes that nil
// when it covers k, so a deeper field read c[k].field stays precise under a
// proof and stays nil-eligible without one (sound).
func (s *Solution) deriveTypeFromAt(p cfg.Point, ancestor constraint.Path, base typ.Type, segs []constraint.Segment) (typ.Type, bool) {
	current := base
	containerPath := ancestor
	for i, seg := range segs {
		next, ok := deriveTypeFrom(s.resolver, current, segs[i:i+1])
		if !ok || next == nil {
			return nil, false
		}
		if seg.Kind == constraint.SegmentIndexInt && seg.Index >= 1 {
			next = s.refineIndexReadAt(p, current, next, IndexKeyDescriptor{
				ContainerPath: containerPath,
				HasLiteral:    true,
				LiteralIndex:  int64(seg.Index),
			})
		}
		current = next
		containerPath = appendSegment(containerPath, seg)
	}
	return current, true
}

func appendSegment(path constraint.Path, seg constraint.Segment) constraint.Path {
	out := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	out.Segments = make([]constraint.Segment, 0, len(path.Segments)+1)
	out.Segments = append(out.Segments, path.Segments...)
	out.Segments = append(out.Segments, seg)
	return out
}

func (s *Solution) narrowedAncestorBaseTypeAt(p cfg.Point, ancestor constraint.Path) typ.Type {
	if len(ancestor.Segments) != 0 {
		return s.NarrowedTypeAt(p, ancestor)
	}
	baseType, _ := s.rootBaseTypeAt(p, ancestor)
	if baseType == nil {
		return nil
	}
	cond := s.ConditionAt(p)
	if !cond.HasConstraints() || cond.IsTrue() {
		return s.applyPointNumericShapeProjection(p, ancestor, baseType)
	}
	narrowed := s.applyPointCondition(p, baseType, ancestor, cond)
	return s.applyPointNumericShapeProjection(p, ancestor, narrowed)
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
	return s.applyConditionWithFilter(p, baseType, path, cond, true)
}

func (s *Solution) applyPointCondition(p cfg.Point, baseType typ.Type, path constraint.Path, cond constraint.Condition) typ.Type {
	if baseType == nil || !cond.HasConstraints() || cond.IsTrue() || cond.IsFalse() {
		return s.applyConditionWithFilter(p, baseType, path, cond, true)
	}
	projected := s.projectPointConditionForPath(p, path, cond)
	return s.applyConditionWithFilter(p, baseType, path, projected, false)
}

func (s *Solution) applyConditionProof(p cfg.Point, seedType typ.Type, seedPath, queryPath constraint.Path, cond constraint.Condition) typ.Type {
	if seedType == nil || !cond.HasConstraints() {
		return seedType
	}
	if cond.IsTrue() {
		return seedType
	}
	if cond.IsFalse() {
		return typ.Never
	}

	narrowedTypes := make([]typ.Type, 0, cond.NumDisjuncts())
	satisfiable := false
	noProjection := false
	projectedBottom := false
	for i := 0; i < cond.NumDisjuncts(); i++ {
		disjunct := cond.DisjunctConstraints(i)
		if len(disjunct) == 0 {
			satisfiable = true
			if len(queryPath.Segments) == 0 {
				narrowedTypes = append(narrowedTypes, seedType)
			} else {
				noProjection = true
			}
			continue
		}
		proof := s.applyConditionProofConstraints(p, seedType, seedPath, queryPath, disjunct)
		switch proof.status {
		case conditionProjectionUnsat:
			continue
		case conditionProjectionNone:
			satisfiable = true
			noProjection = true
		case conditionProjectionType:
			satisfiable = true
			if proof.typ == nil {
				noProjection = true
				continue
			}
			if proof.typ.Kind().IsNever() {
				projectedBottom = true
				continue
			}
			narrowedTypes = append(narrowedTypes, proof.typ)
		}
	}

	if len(narrowedTypes) == 0 {
		if satisfiable && noProjection {
			return nil
		}
		if satisfiable && projectedBottom {
			return typ.Never
		}
		return typ.Never
	}
	if len(narrowedTypes) == 1 {
		return narrowedTypes[0]
	}
	return typ.PruneSoftUnionMembers(typ.NewUnion(narrowedTypes...))
}

type conditionProjectionStatus uint8

const (
	conditionProjectionUnsat conditionProjectionStatus = iota
	conditionProjectionNone
	conditionProjectionType
)

type conditionProjectionResult struct {
	typ    typ.Type
	status conditionProjectionStatus
}

func (s *Solution) applyConditionProofConstraints(p cfg.Point, seedType typ.Type, seedPath, queryPath constraint.Path, constraints []constraint.Constraint) conditionProjectionResult {
	if seedType == nil || len(constraints) == 0 {
		return conditionProjectionResult{typ: seedType, status: conditionProjectionType}
	}
	canonicalKey := s.pkResolver.KeyAt(p, queryPath)
	if canonicalKey == "" {
		return conditionProjectionResult{status: conditionProjectionNone}
	}

	dom, _ := s.conditionProofProductDomainAt(p, seedPath, seedType, constraints)
	if dom == nil {
		return conditionProjectionResult{status: conditionProjectionNone}
	}
	if !dom.ApplyCondition(constraint.FromConstraints(constraints...)) {
		return conditionProjectionResult{status: conditionProjectionUnsat}
	}
	projected := dom.ProjectedTypeAt(canonicalKey, s.resolver)
	if projected == nil {
		return conditionProjectionResult{status: conditionProjectionNone}
	}
	return conditionProjectionResult{typ: projected, status: conditionProjectionType}
}

func (s *Solution) applyConditionWithFilter(p cfg.Point, baseType typ.Type, path constraint.Path, cond constraint.Condition, filter bool) typ.Type {
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
		var narrowed typ.Type
		if filter {
			narrowed = s.applyConstraints(p, baseType, path, disjunct)
		} else {
			narrowed = s.applyFilteredConstraints(p, baseType, path, disjunct)
		}
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
	return typ.PruneSoftUnionMembers(typ.NewUnion(narrowedTypes...))
}

// applyConstraints narrows baseType using constraints that apply to path at point p.
func (s *Solution) applyConstraints(p cfg.Point, baseType typ.Type, path constraint.Path, constraints []constraint.Constraint) typ.Type {
	if baseType == nil || len(constraints) == 0 {
		return baseType
	}
	constraints = pathkey.FilterConstraintsForPath(constraints, path)
	return s.applyFilteredConstraints(p, baseType, path, constraints)
}

func (s *Solution) applyFilteredConstraints(p cfg.Point, baseType typ.Type, path constraint.Path, constraints []constraint.Constraint) typ.Type {
	if baseType == nil || len(constraints) == 0 {
		return baseType
	}
	if len(constraints) == 0 {
		return baseType
	}

	// Use canonical versioned key only
	canonicalKey := s.pkResolver.KeyAt(p, path)
	if canonicalKey == "" {
		return baseType
	}

	dom, _ := s.productDomainAt(p, path, baseType, constraints)
	if dom == nil {
		return baseType
	}
	if !dom.ApplyCondition(constraint.FromConstraints(constraints...)) {
		return nil
	}

	return dom.ProjectedTypeAt(canonicalKey, s.resolver)
}

func (s *Solution) projectPointConditionForPath(p cfg.Point, path constraint.Path, cond constraint.Condition) constraint.Condition {
	if !cond.HasConstraints() || path.IsEmpty() {
		return cond
	}
	cacheKey, cacheable := s.conditionPathCacheKey(p, path, cond)
	if cacheable {
		if s.pointConditionCache == nil {
			s.pointConditionCache = make(map[conditionPathCacheKey]constraint.Condition, 16)
		}
		if cached, ok := s.pointConditionCache[cacheKey]; ok {
			return cached
		}
	}

	projected := projectConditionForPath(cond, path)
	if cacheable {
		s.pointConditionCache[cacheKey] = projected
	}
	return projected
}

func (s *Solution) conditionPathCacheKey(p cfg.Point, path constraint.Path, cond constraint.Condition) (conditionPathCacheKey, bool) {
	if path.IsEmpty() {
		return conditionPathCacheKey{}, false
	}
	pathKey := constraint.PathKey("")
	if s != nil && s.pkResolver != nil {
		pathKey = s.pkResolver.KeyAt(p, path)
	}
	if pathKey == "" {
		pathKey = path.Key()
	}
	if pathKey == "" {
		return conditionPathCacheKey{}, false
	}
	return conditionPathCacheKey{
		point:         p,
		path:          pathKey,
		conditionHash: cond.Hash(),
	}, true
}

func projectConditionForPath(cond constraint.Condition, path constraint.Path) constraint.Condition {
	if !cond.HasConstraints() || path.IsEmpty() {
		return cond
	}
	disjuncts := make([][]constraint.Constraint, 0, cond.NumDisjuncts())
	for i := 0; i < cond.NumDisjuncts(); i++ {
		disjuncts = append(disjuncts, pathkey.FilterConstraintsForPath(cond.DisjunctConstraints(i), path))
	}
	return constraint.FromDisjuncts(disjuncts)
}

func (s *Solution) productDomainAt(
	p cfg.Point,
	targetPath constraint.Path,
	baseType typ.Type,
	constraints []constraint.Constraint,
) (*ProductDomain, constraint.PathKey) {
	if s == nil || s.pkResolver == nil {
		return nil, ""
	}

	var targetKey constraint.PathKey
	if !targetPath.IsEmpty() {
		targetKey = s.pkResolver.KeyAt(p, targetPath)
	}

	valueMap := s.buildPointValueMap(p, targetPath, baseType, constraints)
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

	return NewProductDomain(env), targetKey
}

func (s *Solution) conditionProofProductDomainAt(
	p cfg.Point,
	targetPath constraint.Path,
	baseType typ.Type,
	constraints []constraint.Constraint,
) (*ProductDomain, constraint.PathKey) {
	if s == nil || s.pkResolver == nil {
		return nil, ""
	}

	targetKey := constraint.PathKey("")
	if !targetPath.IsEmpty() {
		targetKey = s.pkResolver.KeyAt(p, targetPath)
	}

	valueMap := newPointValueEnvBuilder(s, p, targetPath, baseType, constraints, pointValueEnvConditionProof).build()
	resolvePathCache := s.scratchResolvedPathMap
	if resolvePathCache == nil {
		resolvePathCache = make(map[constraint.PathKey]constraint.PathKey, len(constraints)*2)
		s.scratchResolvedPathMap = resolvePathCache
	}
	clear(resolvePathCache)

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

	return NewProductDomain(env), targetKey
}

func (s *Solution) conditionRootTypeAt(path constraint.Path) typ.Type {
	if s == nil || path.IsEmpty() || path.Symbol == 0 {
		return nil
	}
	return s.lookupDeclaredType(constraint.Path{Root: path.Root, Symbol: path.Symbol})
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

// IsPointDead returns true if the given CFG point is unreachable.
func (s *Solution) IsPointDead(p cfg.Point) bool {
	return !s.isPointReachable(p)
}

func (s *Solution) isPointReachable(p cfg.Point) bool {
	if s == nil {
		return true
	}
	if s.inputs != nil && s.inputs.DeadPoints != nil && s.inputs.DeadPoints[p] {
		return false
	}
	cond := s.ConditionAt(p)
	if cond.IsFalse() {
		return false
	}
	if s.reachabilityCache != nil {
		if cached, ok := s.reachabilityCache[p]; ok {
			return cached.reachable
		}
	}

	reachable := true
	if !cond.IsTrue() && cond.HasConstraints() {
		reachable = s.computeConstrainedPointReachable(p, cond)
	}
	if reachable {
		reachable = s.pointNumericShapeReachable(p, cond)
	}
	if s.reachabilityCache == nil {
		s.reachabilityCache = make(map[cfg.Point]reachabilityCacheValue, 8)
	}
	s.reachabilityCache[p] = reachabilityCacheValue{
		reachable: reachable,
	}
	return reachable
}

func (s *Solution) transferPointReachable(p cfg.Point) bool {
	return s.isPointReachable(p)
}

func (s *Solution) computeConstrainedPointReachable(p cfg.Point, cond constraint.Condition) bool {
	dom, _ := s.productDomainAt(p, constraint.Path{}, nil, cond.AllConstraints())
	if dom == nil {
		return true
	}
	return dom.CanSatisfyCondition(cond)
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
