package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// conditionPathGuard is the normalized condition proof a branch contributes to a
// single CFG edge. It keeps the tested root, static path, effective edge check,
// and source path together so each consumer applies the same proof instead of
// threading parallel sym/segments/check/typeName tuples.
type conditionPathGuard struct {
	point    cfg.Point
	sym      cfg.SymbolID
	segments []constraint.Segment
	varPath  string
	check    cfg.CondCheckKind
	typeName string
}

func (g conditionPathGuard) narrowsBareSymbolPresence() bool {
	return len(g.segments) == 0 && (g.check == cfg.CheckTruthy || g.check == cfg.CheckNotNil)
}

func (g conditionPathGuard) condition(t *Transfer) constraint.Condition {
	path := t.versionedPath(g.point, constraint.Path{
		Root:     extraction.ExtractRootName(g.varPath),
		Symbol:   g.sym,
		Segments: g.segments,
	})
	switch g.check {
	case cfg.CheckTruthy:
		return constraint.FromConstraints(constraint.Truthy{Path: path})
	case cfg.CheckFalsy:
		return constraint.FromConstraints(constraint.Falsy{Path: path})
	case cfg.CheckNil:
		return constraint.FromConstraints(constraint.IsNil{Path: path})
	case cfg.CheckNotNil:
		return constraint.FromConstraints(constraint.NotNil{Path: path})
	case cfg.CheckTypeEqual:
		if key := typeKeyFor(g.typeName); !key.IsZero() {
			return constraint.FromConstraints(constraint.HasType{Path: path, Type: key})
		}
	case cfg.CheckTypeNot:
		if key := typeKeyFor(g.typeName); !key.IsZero() {
			return constraint.FromConstraints(constraint.NotHasType{Path: path, Type: key})
		}
	}
	return constraint.TrueCondition()
}

func (g conditionPathGuard) narrowValue(av product.AbstractValue) (product.AbstractValue, bool) {
	return narrowAtPath(av, g.segments, g.check, g.typeName)
}

func (g conditionPathGuard) refineStaticMemberFact(t *Transfer, out *flow.PointState, base product.AbstractValue, hasBase bool) {
	place, ok := staticPlace(g.sym, g.segments)
	if !ok {
		return
	}
	t.applyStaticMemberRefinementEffect(out, StaticMemberRefinementEffect{
		Place:    place,
		Base:     base,
		HasBase:  hasBase,
		Check:    g.check,
		TypeName: g.typeName,
	})
}

// authorizesCurrentSeed is the authority boundary for declared-base narrowing. A
// declaration supplies the branch universe; the current Env value may specialize
// that universe only when the condition-proof projector can re-project the
// declared seed to an envelope covering the current value. A precise initializer
// singleton has no condition proof, so it cannot make another declared-union edge
// unreachable.
func (g conditionPathGuard) authorizesCurrentSeed(t *Transfer, out *flow.PointState, declaredBase, live product.AbstractValue) bool {
	if out == nil || g.sym == 0 || declaredBase.IsZero() || live.IsZero() || !out.Cond.HasConstraints() {
		return false
	}
	seedType := product.ProjectValueOrUnknown(declaredBase)
	if typ.IsAbsentOrUnknown(seedType) {
		return false
	}
	seedPath := constraint.Path{Symbol: g.sym}
	projectedType := flow.ConditionProofProjector{
		Resolver:    fieldResolver,
		ResolveType: t.resolveTypeKey,
		ConditionAt: func(cfg.Point) constraint.Condition { return out.Cond },
	}.ConditionedSeedTypeAt(g.point, seedPath, seedType, seedPath, constraint.TrueCondition())
	if typ.IsAbsentOrUnknown(projectedType) || typ.IsNever(projectedType) {
		return false
	}
	projected := product.FromRefinedType(declaredBase, projectedType)
	if projected.IsZero() || valueIsBottom(projected) {
		return false
	}
	return projected.Covers(live)
}
