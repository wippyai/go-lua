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
	directProof := p.directHasTypeProof(cond, path)
	rootPath := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	rootType := p.rootTypeAt(point, rootPath)
	if rootType == nil {
		return directProof
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
	proof := p.applyConditionProof(point, rootType, rootPath, path, projected)
	if proof == nil && directProof != nil {
		return directProof
	}
	return proof
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
	return PathIdentityKey(path)
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

	proofEnv := p.conditionProofValueEnv(point, seedPath, seedType, constraints)
	env := constraint.Env{
		ResolveType: p.ResolveType,
		Resolver:    p.Resolver,
		PathTypeAt:  proofEnv.TypeAt,
		ResolvePath: proofEnv.ResolvePath,
	}
	dom := NewConditionProofDomain(env)
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

func projectedDescendantRead(dom *ConditionProofDomain, resolver narrow.Resolver, seedKey constraint.PathKey, seedType typ.Type, seedPath, queryPath constraint.Path) (typ.Type, bool) {
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

type conditionProofValueEnv struct {
	projector ConditionProofProjector
	point     cfg.Point
	seedPath  constraint.Path
	seedType  typ.Type
	values    map[constraint.PathKey]typ.Type
}

func (p ConditionProofProjector) conditionProofValueEnv(point cfg.Point, seedPath constraint.Path, seedType typ.Type, constraints []constraint.Constraint) conditionProofValueEnv {
	seedPath = normalizeConstraintPathForQuery(seedPath)
	env := conditionProofValueEnv{
		projector: p,
		point:     point,
		seedPath:  seedPath,
		seedType:  seedType,
		values:    make(map[constraint.PathKey]typ.Type, len(constraints)+2),
	}
	if seedKey := env.ResolvePath(seedPath); seedKey != "" && seedType != nil {
		env.values[seedKey] = seedType
	}
	for _, c := range constraints {
		constraint.VisitPaths(c, func(path constraint.Path) bool {
			env.RecordPath(path)
			return false
		})
	}
	return env
}

func (e conditionProofValueEnv) TypeAt(key constraint.PathKey) typ.Type {
	return e.values[key]
}

func (e conditionProofValueEnv) ResolvePath(path constraint.Path) constraint.PathKey {
	return e.projector.resolvePath(e.point, path)
}

func (e conditionProofValueEnv) RecordPath(path constraint.Path) {
	if path.IsEmpty() {
		return
	}
	path = normalizeConstraintPathForQuery(path)
	key := e.ResolvePath(path)
	if key == "" {
		return
	}
	if _, exists := e.values[key]; exists {
		return
	}
	if e.seedType != nil && isDescendantOf(path, e.seedPath) {
		relative := path.Segments[len(e.seedPath.Segments):]
		if derived, ok := deriveTypeFrom(e.projector.Resolver, e.seedType, relative); ok {
			e.values[key] = derived
			return
		}
	}
	if path.Symbol == 0 {
		return
	}
	rootPath := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	rootType := e.projector.rootTypeAt(e.point, rootPath)
	if rootType == nil {
		return
	}
	if len(path.Segments) == 0 {
		e.values[key] = rootType
		return
	}
	if derived, ok := deriveTypeFrom(e.projector.Resolver, rootType, path.Segments); ok {
		e.values[key] = derived
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

func (p ConditionProofProjector) directHasTypeProof(cond constraint.Condition, path constraint.Path) typ.Type {
	if path.IsEmpty() || cond.IsFalse() || !cond.HasConstraints() {
		return nil
	}
	types := make([]typ.Type, 0, cond.NumDisjuncts())
	for i := 0; i < cond.NumDisjuncts(); i++ {
		proof, ok := p.directHasTypeProofDisjunct(cond.DisjunctConstraints(i), path)
		if !ok {
			return nil
		}
		if proof == nil {
			return nil
		}
		if proof.Kind().IsNever() {
			continue
		}
		types = append(types, proof)
	}
	if len(types) == 0 {
		return typ.Never
	}
	if len(types) == 1 {
		return types[0]
	}
	return typ.PruneSoftUnionMembers(typ.NewUnion(types...))
}

func (p ConditionProofProjector) directHasTypeProofDisjunct(constraints []constraint.Constraint, path constraint.Path) (typ.Type, bool) {
	var proof typ.Type
	found := false
	for _, c := range constraints {
		ht, ok := c.(constraint.HasType)
		if !ok || !conditionProofPathMatches(ht.Path, path) {
			continue
		}
		resolved := p.typeFromHasTypeKey(ht.Type)
		if resolved == nil {
			continue
		}
		found = true
		proof = intersectConjunctiveProof(proof, resolved)
	}
	return proof, found
}

func (p ConditionProofProjector) typeFromHasTypeKey(key narrow.TypeKey) typ.Type {
	resolved := narrow.ByTypeKey(typ.Any, key, p.ResolveType)
	if resolved != nil && !typ.IsAbsentOrUnknown(resolved) {
		return resolved
	}
	if p.ResolveType == nil {
		return nil
	}
	resolved = p.ResolveType(key)
	if typ.IsAbsentOrUnknown(resolved) {
		return nil
	}
	return resolved
}

func intersectConjunctiveProof(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) || subtype.IsSubtype(a, b) {
		return a
	}
	if subtype.IsSubtype(b, a) {
		return b
	}
	return typ.Never
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

func conditionProofPathMatches(cpath constraint.Path, qpath constraint.Path) bool {
	cpath = normalizeConstraintPathForQuery(cpath)
	qpath = normalizeConstraintPathForQuery(qpath)
	return cpath.Equal(qpath)
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

func normalizeConstraintPathForQuery(path constraint.Path) constraint.Path {
	if path.Symbol == 0 {
		return path
	}
	path.Version = 0
	return path
}

func isDescendantOf(child, parent constraint.Path) bool {
	if child.Symbol != parent.Symbol {
		return false
	}
	if len(child.Segments) <= len(parent.Segments) {
		return false
	}
	for i := 0; i < len(parent.Segments); i++ {
		if child.Segments[i] != parent.Segments[i] {
			return false
		}
	}
	return true
}
