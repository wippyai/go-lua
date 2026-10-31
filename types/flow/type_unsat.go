package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// checkTypeConstraints marks edges unsatisfiable when an edge condition
// contradicts a stable literal- or nil-typed parameter path. Marks also feed
// typeUnsatEdges, the only unsat source for dead-point closure: numeric
// unsat edges stay advisory as before.
func (s *Solution) checkTypeConstraints() {
	if s == nil || s.inputs == nil || len(s.edgeConditions) == 0 {
		return
	}
	keys := make([]edgeKey, 0, len(s.edgeConditions))
	for key := range s.edgeConditions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})
	for _, key := range keys {
		if s.unsatEdges[key] {
			continue
		}
		cond := s.edgeConditions[key]
		if !cond.HasConstraints() {
			continue
		}
		if s.edgeTypeContradicted(cond) {
			s.unsatEdges[key] = true
			if s.typeUnsatEdges == nil {
				s.typeUnsatEdges = make(map[edgeKey]bool)
			}
			s.typeUnsatEdges[key] = true
		}
	}
}

// edgeTypeContradicted reports whether every disjunct of the edge condition
// carries a constraint that narrows a literal- or nil-typed path to Never.
func (s *Solution) edgeTypeContradicted(cond constraint.Condition) bool {
	if !cond.HasConstraints() {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		if !s.disjunctTypeContradicted(cond.DisjunctConstraints(i)) {
			return false
		}
	}
	return cond.NumDisjuncts() > 0
}

// disjunctTypeContradicted reports whether one constraint in the conjunction
// alone contradicts a literal- or nil-typed path. A contradicting constraint
// is ignored when another constraint in the same conjunction mentions the
// same symbol: companions can refine the path first, so the declared type in
// isolation does not decide feasibility.
func (s *Solution) disjunctTypeContradicted(conj []constraint.Constraint) bool {
	for i, c := range conj {
		sym, ok := s.contradictingSymbol(c)
		if !ok {
			continue
		}
		if conjunctMentionsSymbol(conj, i, sym) {
			continue
		}
		return true
	}
	return false
}

// contradictingSymbol returns the path symbol when the constraint narrows a
// literal- or nil-typed path to Never.
func (s *Solution) contradictingSymbol(c constraint.Constraint) (cfg.SymbolID, bool) {
	switch v := c.(type) {
	case constraint.HasType:
		base, ok := s.literalOrNilBase(v.Path)
		if !ok || !hasTypeContradicts(s, base, v.Type) {
			return 0, false
		}
		return v.Path.Symbol, true
	case constraint.NotHasType:
		base, ok := s.literalOrNilBase(v.Path)
		if !ok || !notHasTypeContradicts(s, base, v.Type) {
			return 0, false
		}
		return v.Path.Symbol, true
	case constraint.IsNil:
		base, ok := s.literalOrNilBase(v.Path)
		if !ok || canBeNil(base) {
			return 0, false
		}
		return v.Path.Symbol, true
	case constraint.NotNil:
		base, ok := s.literalOrNilBase(v.Path)
		if !ok || canBeNonNil(base) {
			return 0, false
		}
		return v.Path.Symbol, true
	default:
		return 0, false
	}
}

// conjunctMentionsSymbol reports whether any constraint besides index i
// mentions the symbol.
func conjunctMentionsSymbol(conj []constraint.Constraint, skip int, sym cfg.SymbolID) bool {
	if sym == 0 {
		return false
	}
	for i, c := range conj {
		if i == skip {
			continue
		}
		for _, p := range c.Paths() {
			if p.Symbol == sym {
				return true
			}
		}
	}
	return false
}

// literalOrNilBase returns the declared type of a plain variable path when it
// is exactly a literal or nil. Only stable parameters qualify: locals, loop
// variables, and upvalues can hold a different value at the edge than their
// declared type shows, and reassigned parameters can too.
func (s *Solution) literalOrNilBase(path constraint.Path) (typ.Type, bool) {
	if s == nil || s.inputs == nil || path.Symbol == 0 || len(path.Segments) != 0 {
		return nil, false
	}
	if !s.stableParamSymbol(path.Symbol) {
		return nil, false
	}
	base := s.inputs.DeclaredTypes[path.Symbol]
	if base == nil {
		return nil, false
	}
	base = unwrap.Alias(base)
	if base == nil {
		return nil, false
	}
	if _, ok := base.(*typ.Literal); ok {
		return base, true
	}
	if base.Kind() == kind.Nil {
		return base, true
	}
	return nil, false
}

// stableParamSymbol reports whether the symbol is a function parameter with
// no assignments in the graph, so its declared type holds at every point.
func (s *Solution) stableParamSymbol(sym cfg.SymbolID) bool {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || sym == 0 {
		return false
	}
	isParam := false
	for _, p := range s.inputs.Graph.ParamSymbols() {
		if p == sym {
			isParam = true
			break
		}
	}
	if !isParam {
		return false
	}
	declPoint, hasDecl := s.inputs.Graph.DeclarationPoint(sym)
	for _, a := range s.inputs.Assignments {
		if a.TargetPath.Symbol != sym {
			continue
		}
		if len(a.TargetPath.Segments) != 0 {
			continue
		}
		if hasDecl && a.Point == declPoint {
			continue
		}
		return false
	}
	return true
}

// hasTypeContradicts reports whether requiring key of base leaves no value.
func hasTypeContradicts(s *Solution, base typ.Type, key narrow.TypeKey) bool {
	if key.IsZero() {
		return false
	}
	resolved := s.resolveTypeKey(key)
	if resolved == nil {
		return false
	}
	resolved = unwrap.Alias(resolved)
	if resLit, ok := resolved.(*typ.Literal); ok {
		baseLit, ok := base.(*typ.Literal)
		return !ok || !typ.TypeEquals(baseLit, resLit)
	}
	return !subtype.IsSubtype(base, resolved)
}

// notHasTypeContradicts reports whether excluding key from base leaves no value.
func notHasTypeContradicts(s *Solution, base typ.Type, key narrow.TypeKey) bool {
	if key.IsZero() {
		return false
	}
	resolved := s.resolveTypeKey(key)
	if resolved == nil {
		return false
	}
	resolved = unwrap.Alias(resolved)
	if resLit, ok := resolved.(*typ.Literal); ok {
		baseLit, ok := base.(*typ.Literal)
		return ok && typ.TypeEquals(baseLit, resLit)
	}
	return subtype.IsSubtype(base, resolved)
}

// canBeNil reports whether a literal-or-nil base includes nil.
func canBeNil(base typ.Type) bool {
	if base == nil {
		return false
	}
	if _, ok := base.(*typ.Literal); ok {
		return false
	}
	return base.Kind() == kind.Nil
}

// canBeNonNil reports whether a literal-or-nil base includes a non-nil value.
func canBeNonNil(base typ.Type) bool {
	if base == nil {
		return false
	}
	if _, ok := base.(*typ.Literal); ok {
		return true
	}
	return false
}

// computeTypeDeadPoints closes deadness over type-theory unsat edges: a point
// is dead when every predecessor is dead or reaches it over a type-unsat
// edge. Numeric unsat edges stay out of the closure.
func (s *Solution) computeTypeDeadPoints() {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || len(s.typeUnsatEdges) == 0 {
		return
	}
	g := s.inputs.Graph
	dead := make(map[cfg.Point]bool)
	if s.inputs.DeadPoints != nil {
		for p := range s.inputs.DeadPoints {
			dead[p] = true
		}
	}
	order := g.RPO()
	changed := true
	for changed {
		changed = false
		for _, p := range order {
			if p == g.Entry() || dead[p] {
				continue
			}
			preds := graphPredecessors(g, p)
			if len(preds) == 0 {
				continue
			}
			allDead := true
			for _, pred := range preds {
				if !dead[pred] && !s.typeUnsatEdges[edgeKey{from: pred, to: p}] {
					allDead = false
					break
				}
			}
			if allDead {
				dead[p] = true
				changed = true
			}
		}
	}
	for p := range dead {
		if s.inputs.DeadPoints == nil || !s.inputs.DeadPoints[p] {
			if s.typeDeadPoints == nil {
				s.typeDeadPoints = make(map[cfg.Point]bool)
			}
			s.typeDeadPoints[p] = true
		}
	}
}

// pointDeadByUnsat reports whether the point is dead by unsat-edge closure.
func (s *Solution) pointDeadByUnsat(p cfg.Point) bool {
	if s == nil {
		return false
	}
	return s.typeDeadPoints != nil && s.typeDeadPoints[p]
}

// propagationDeadPoints merges declared dead points with unsat-derived ones.
func (s *Solution) propagationDeadPoints() map[cfg.Point]bool {
	var out map[cfg.Point]bool
	if s != nil && s.inputs != nil && s.inputs.DeadPoints != nil {
		out = make(map[cfg.Point]bool, len(s.inputs.DeadPoints))
		for p := range s.inputs.DeadPoints {
			out[p] = true
		}
	}
	if s == nil || s.typeDeadPoints == nil {
		return out
	}
	if out == nil {
		out = make(map[cfg.Point]bool, len(s.typeDeadPoints))
	}
	for p := range s.typeDeadPoints {
		out[p] = true
	}
	return out
}
