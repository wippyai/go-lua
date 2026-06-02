package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// ConditionProofProjector is the producer-neutral condition-proof interpreter.
// It projects a finite DNF condition over a finite root/seed type environment,
// using the caller's normalized path resolver. This is proof-only: it must not
// call back into a full narrowed-state query.
type ConditionProofProjector struct {
	Resolver    narrow.Resolver
	ResolveType narrow.TypeResolver
	ConditionAt func(cfg.Point) constraint.Condition
	RootTypeAt  func(cfg.Point, constraint.Path) typ.Type
	ResolvePath func(cfg.Point, constraint.Path) constraint.PathKey
}

// ProvesTypeAt reports whether the point condition proves path is a subtype of t.
func (p ConditionProofProjector) ProvesTypeAt(point cfg.Point, path constraint.Path, t typ.Type) bool {
	if t == nil {
		return false
	}
	return p.conditionProvesType(p.pointCondition(point, nil), path, t)
}

// ConditionTypeAt returns the type proven for path by the point condition alone.
func (p ConditionProofProjector) ConditionTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	return p.conditionedTypeAt(point, path, nil)
}

// ConditionedTypeAt returns the type proven for path by the point condition plus
// an expression-local condition.
func (p ConditionProofProjector) ConditionedTypeAt(point cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	return p.conditionedTypeAt(point, path, &extra)
}

// ConditionedSeedTypeAt returns the type of queryPath proven from seedType under
// the point condition plus an expression-local condition.
func (p ConditionProofProjector) ConditionedSeedTypeAt(point cfg.Point, seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path, extra constraint.Condition) typ.Type {
	if seedPath.IsEmpty() || queryPath.IsEmpty() || seedType == nil {
		return nil
	}
	cond := p.pointCondition(point, &extra)
	if cond.IsFalse() {
		return typ.Never
	}
	if cond.IsTrue() || !cond.HasConstraints() {
		return p.deriveSeedType(seedPath, seedType, queryPath)
	}
	projected := projectConditionForPath(cond, seedPath)
	if !projected.HasConstraints() {
		return p.deriveSeedType(seedPath, seedType, queryPath)
	}
	return p.applyConditionProof(point, seedType, seedPath, queryPath, projected)
}

func (p ConditionProofProjector) conditionedTypeAt(point cfg.Point, path constraint.Path, extra *constraint.Condition) typ.Type {
	if path.IsEmpty() {
		return nil
	}
	cond := p.pointCondition(point, extra)
	if cond.IsFalse() {
		return typ.Never
	}
	rootPath := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	rootType := p.rootTypeAt(point, rootPath)
	if rootType == nil {
		return nil
	}
	if cond.IsTrue() || !cond.HasConstraints() {
		if len(path.Segments) == 0 {
			return rootType
		}
		return nil
	}
	projected := projectConditionForPath(cond, rootPath)
	if !projected.HasConstraints() {
		if len(path.Segments) == 0 {
			return rootType
		}
		return nil
	}
	return p.applyConditionProof(point, rootType, rootPath, path, projected)
}

func (p ConditionProofProjector) pointCondition(point cfg.Point, extra *constraint.Condition) constraint.Condition {
	cond := constraint.TrueCondition()
	if p.ConditionAt != nil {
		cond = p.ConditionAt(point)
	}
	if extra != nil {
		cond = constraint.And(cond, *extra)
	}
	return cond
}

func (p ConditionProofProjector) rootTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	if p.RootTypeAt == nil || path.IsEmpty() || path.Symbol == 0 {
		return nil
	}
	return p.RootTypeAt(point, path)
}

func (p ConditionProofProjector) resolvePath(point cfg.Point, path constraint.Path) constraint.PathKey {
	if path.IsEmpty() {
		return ""
	}
	path = normalizeConstraintPathForQuery(path)
	if p.ResolvePath != nil {
		if key := p.ResolvePath(point, path); key != "" {
			return key
		}
	}
	return path.Key()
}

func (p ConditionProofProjector) deriveSeedType(seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path) typ.Type {
	if seedPath.Equal(queryPath) {
		return seedType
	}
	if !isDescendantOf(queryPath, seedPath) {
		return nil
	}
	relative := queryPath.Segments[len(seedPath.Segments):]
	derived, ok := deriveTypeFrom(p.Resolver, seedType, relative)
	if !ok {
		return nil
	}
	return derived
}

func (p ConditionProofProjector) applyConditionProof(point cfg.Point, seedType typ.Type, seedPath, queryPath constraint.Path, cond constraint.Condition) typ.Type {
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
		proof := p.applyConditionProofConstraints(point, seedType, seedPath, queryPath, disjunct)
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

func (p ConditionProofProjector) applyConditionProofConstraints(point cfg.Point, seedType typ.Type, seedPath, queryPath constraint.Path, constraints []constraint.Constraint) conditionProjectionResult {
	if seedType == nil || len(constraints) == 0 {
		return conditionProjectionResult{typ: seedType, status: conditionProjectionType}
	}
	canonicalKey := p.resolvePath(point, queryPath)
	if canonicalKey == "" {
		return conditionProjectionResult{status: conditionProjectionNone}
	}

	values := p.conditionProofValueMap(point, seedPath, seedType, constraints)
	env := constraint.Env{
		ResolveType: p.ResolveType,
		Resolver:    p.Resolver,
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			return values[key]
		},
		ResolvePath: func(path constraint.Path) constraint.PathKey {
			return p.resolvePath(point, path)
		},
	}
	dom := NewProductDomain(env)
	if !dom.ApplyCondition(constraint.FromConstraints(constraints...)) {
		return conditionProjectionResult{status: conditionProjectionUnsat}
	}
	projected := dom.ProjectedTypeAt(canonicalKey, p.Resolver)
	if projected == nil {
		seedKey := p.resolvePath(point, seedPath)
		if descendant, ok := projectedDescendantRead(dom, p.Resolver, seedKey, seedType, seedPath, queryPath); ok {
			return conditionProjectionResult{typ: descendant, status: conditionProjectionType}
		}
		return conditionProjectionResult{status: conditionProjectionNone}
	}
	return conditionProjectionResult{typ: projected, status: conditionProjectionType}
}

func projectedDescendantRead(dom *ProductDomain, resolver narrow.Resolver, seedKey constraint.PathKey, seedType typ.Type, seedPath, queryPath constraint.Path) (typ.Type, bool) {
	if dom == nil || resolver == nil || seedKey == "" || !isDescendantOf(queryPath, seedPath) {
		return nil, false
	}
	seed := dom.ProjectedTypeAt(seedKey, resolver)
	if seed == nil {
		return nil, false
	}
	if typ.TypeEquals(seed, seedType) {
		return nil, false
	}
	relative := queryPath.Segments[len(seedPath.Segments):]
	if derived, ok := deriveTypeFrom(resolver, seed, relative); ok && derived != nil {
		return derived, true
	}
	return typ.Nil, true
}

func (p ConditionProofProjector) conditionProofValueMap(point cfg.Point, seedPath constraint.Path, seedType typ.Type, constraints []constraint.Constraint) map[constraint.PathKey]typ.Type {
	values := make(map[constraint.PathKey]typ.Type, len(constraints)+2)
	seedPath = normalizeConstraintPathForQuery(seedPath)
	if seedKey := p.resolvePath(point, seedPath); seedKey != "" && seedType != nil {
		values[seedKey] = seedType
	}
	for _, c := range constraints {
		constraint.VisitPaths(c, func(path constraint.Path) bool {
			p.recordConditionProofPath(point, values, seedPath, seedType, path)
			return false
		})
	}
	return values
}

func (p ConditionProofProjector) recordConditionProofPath(point cfg.Point, values map[constraint.PathKey]typ.Type, seedPath constraint.Path, seedType typ.Type, path constraint.Path) {
	if path.IsEmpty() {
		return
	}
	path = normalizeConstraintPathForQuery(path)
	key := p.resolvePath(point, path)
	if key == "" {
		return
	}
	if _, exists := values[key]; exists {
		return
	}
	if seedType != nil && isDescendantOf(path, seedPath) {
		relative := path.Segments[len(seedPath.Segments):]
		if derived, ok := deriveTypeFrom(p.Resolver, seedType, relative); ok {
			values[key] = derived
			return
		}
	}
	if path.Symbol == 0 {
		return
	}
	rootPath := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	rootType := p.rootTypeAt(point, rootPath)
	if rootType == nil {
		return
	}
	if len(path.Segments) == 0 {
		values[key] = rootType
		return
	}
	if derived, ok := deriveTypeFrom(p.Resolver, rootType, path.Segments); ok {
		values[key] = derived
	}
}

func (p ConditionProofProjector) conditionProvesType(cond constraint.Condition, path constraint.Path, t typ.Type) bool {
	if cond.IsFalse() || !cond.HasConstraints() {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		disjunct := cond.DisjunctConstraints(i)
		found := false
		for _, c := range disjunct {
			ht, ok := c.(constraint.HasType)
			if !ok || !conditionProofPathMatches(ht.Path, path) {
				continue
			}
			resolved := typ.Type(nil)
			if p.ResolveType != nil {
				resolved = p.ResolveType(ht.Type)
			}
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

func conditionProofPathMatches(cpath constraint.Path, qpath constraint.Path) bool {
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

// ConditionProofStructuralPathKey lowers a query path to the normalized
// structural key used by canonical point-state components.
func ConditionProofStructuralPathKey(path constraint.Path) constraint.PathKey {
	return KeyPresencePathKey(path)
}
