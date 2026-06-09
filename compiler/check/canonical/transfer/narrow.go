package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func refinedStr(refined, base typ.Type) string {
	switch {
	case refined == nil:
		return "<nil>"
	case refined == base:
		return "<UNCHANGED>"
	default:
		return refined.String()
	}
}

// narrow.go is the path-sensitive narrowing of the flow: the per-edge
// refinement a branch guard proves about its tested value, expressed directly
// over PointState by reusing the same value-domain narrowing primitives:
//
//   - product.NarrowPresent / FilterByKind for x ~= nil, type(x) == k;
//   - product.NarrowTruthy / NarrowFalsy for if x / if not x;
//   - narrow.ByFieldLiteral / ExcludeByFieldLiteral for x.kind == "tag"
//     discriminated-union narrowing.
//
// SOUNDNESS: a branch has two successor edges; the TRUE edge carries the guard,
// the FALSE edge its negation. The per-edge narrowed state is joined at the merge
// point by the env-domain LUB, so a branch's narrowing never survives past its
// guard (x narrowed to string on the true edge, joined with x = nil on the false
// edge, recovers x?). The narrowing only ever shrinks a value, never invents one;
// a guard the transfer cannot interpret leaves the value unchanged (precision
// loss, never unsoundness).

type narrowSeedAuthority uint8

const (
	narrowSeedNone narrowSeedAuthority = iota
	narrowSeedEnv
	narrowSeedDeclared
)

type narrowSeed struct {
	value     product.AbstractValue
	authority narrowSeedAuthority
}

func (s narrowSeed) hasValue() bool {
	return s.authority != narrowSeedNone && !s.value.IsZero()
}

func (s narrowSeed) fromDeclared() bool {
	return s.authority == narrowSeedDeclared
}

// narrowBase resolves the value the per-edge narrowing refines for symbol sym. A
// symbol declared with an annotation (`local r: A|B = ...`) narrows over its
// DECLARED type, not the precise constructor value the Env seeds: the constructor
// `{tag="a", ...}` seeds the singleton `{tag:"a",...}`, so excluding `r.tag=="a"`
// on the false edge would collapse it to Never, dropping the live variant B. The
// declared union carries every variant, so a per-edge filter keeps the consistent
// one(s) and the merge-LUB rebuilds A|B (narrowing never escapes its guard). A
// symbol with no declared type narrows over its tracked Env value as before.
//
// preferEnv overrides the declared-type base with the tracked Env value when one is
// present. A ScopeExit re-narrowing (the then/else-exit guard a post-`if` merge
// reaches) runs AFTER the branch already narrowed the value on the entering edge, so
// the Env carries the precise branch refinement (`{ok:true,value:Action}` from a
// `not r.ok` guard). Resetting that to the declared union there would discard the
// branch's work — the more so because the ScopeExit guard lost the original field
// path (it carries only the root symbol + a bare check), so a declared-base bare-
// symbol narrowing widens the refined variant back to the full union. Narrowing the
// already-refined Env value instead only ever shrinks it further (sound), and the
// constructor-singleton variant recovery the declared base provides has already run
// at the fresh branch, whose full condition AST is intact. The declared base is
// still used when the Env carries no tracked value (the symbol is unrefined here).
func (t *Transfer) narrowBase(sym cfg.SymbolID, av product.AbstractValue, preferEnv bool) (product.AbstractValue, bool) {
	seed := t.narrowSeed(sym, av, preferEnv)
	return seed.value, seed.hasValue()
}

func (t *Transfer) narrowSeed(sym cfg.SymbolID, av product.AbstractValue, preferEnv bool) narrowSeed {
	if preferEnv && !av.IsZero() {
		return narrowSeed{value: av, authority: narrowSeedEnv}
	}
	if declared, ok := t.declaredTypes[sym]; ok && declared != nil && !typ.IsAbsentOrUnknown(declared) {
		if typ.ContainsFreeTypeParam(declared) && entryHasClosedInformativeValue(av) {
			// An open generic declaration (`T`, `Result<T>`, ...) is a binder
			// constraint, not a closed runtime fact. If call-entry/context seeding has
			// already supplied a closed value, narrow that instantiated value instead of
			// resetting branch state back to the callee's binder syntax.
			return narrowSeed{value: av, authority: narrowSeedEnv}
		}
		return narrowSeed{value: product.FromType(declared), authority: narrowSeedDeclared}
	}
	if av.IsZero() {
		return narrowSeed{}
	}
	return narrowSeed{value: av, authority: narrowSeedEnv}
}

func (t *Transfer) narrowBaseFor(out flow.PointState, sym cfg.SymbolID, preferEnv bool) (product.AbstractValue, bool) {
	av, _ := t.symbolValue(&out, sym)
	return t.narrowBase(sym, av, preferEnv)
}

func (t *Transfer) setNarrowedSymbol(out *flow.PointState, sym cfg.SymbolID, av product.AbstractValue) {
	t.applyRefinementEffect(out, RefinementEffect{
		Place: Place{Root: sym},
		Kind:  RefinementSetValue,
		Value: av,
	})
}

// NarrowEdge refines the out-state of guard point pred for the successor reached
// by the edge pred -> succ. When pred carries a branch guard, it narrows the
// guarded value in the returned Env by that guard (the guard on the TRUE edge, its
// negation on the FALSE edge) and records the per-edge path condition. A pred with
// no guard, an uninterpretable guard, or a value the guard cannot refine returns
// out unchanged.
//
// The guard is carried either by the branch node itself (g.Info(pred) is a
// *cfg.BranchInfo, for an intra-block read on a guarded edge) or, for an unsplit
// condition, by the then-exit / else-exit ScopeExit node the CFG copies the
// branch's CondVar/CondCheck onto (the real predecessor of a post-`if` merge, or
// the sole live predecessor after an early return in the other arm). Honoring the
// latter is what narrows a read after the merge or after an early-`return`/`error()`
// in a guarded block.
//
// It implements equation.EdgeNarrower so the equation builder applies it to each
// guarded edge before the predecessor join, and the observation surface applies the
// same refinement so a body read inside a guarded branch observes the narrowed type.
func (t *Transfer) NarrowEdge(g *cfg.Graph, pred, succ cfg.Point, out flow.PointState) flow.PointState {
	if g == nil {
		return out
	}
	// A dead predecessor out-state (its numeric component is the UNSAT bottom, the
	// state error()/a no-return call left behind) stays dead across the edge: edge
	// narrowing must not resurrect a value into an unreachable point, or the
	// successor merge would re-admit the terminated arm. The join then drops this
	// predecessor as unreachable, exactly as it should.
	if out.Num != nil && out.Num.IsUnsat() {
		return out
	}
	atExit := false
	var exitOrigin cfg.Point
	exitHasOrigin := false
	info, ok := g.Info(pred).(*cfg.BranchInfo)
	branchPred := ok && info != nil
	if !ok || info == nil {
		info, exitOrigin, exitHasOrigin = exitGuard(g, pred)
		if info == nil {
			return out
		}
		// A ScopeExit guard re-narrows a state the entering branch already refined; it
		// narrows over the tracked Env value rather than resetting to the declared type.
		atExit = true
	}
	taken, known := g.EdgeCond(pred, succ)
	if !known {
		return out
	}
	if branchPred && taken {
		out = t.genericForBodyEdgeState(g, pred, out)
		out = t.numericForBodyEdgeState(g, pred, out)
	}
	// At an exit guard whose recovered branch is a discriminant on a union symbol, a
	// single exclude over the (widened) out-state cannot see the prior dominating
	// excludes of an early-return chain (`if x.kind == k1 then return end; if x.kind
	// == k2 then return end; use x`), so it would re-admit a member a preceding guard
	// already returned. Compose every discriminant guard that dominates this exit over
	// the declared union instead, dropping each member its surviving edge excludes, so
	// the fallthrough carries the single remaining variant. A non-discriminant exit
	// guard, or one over a non-union symbol, falls through to the ordinary narrowing.
	if atExit {
		if narrowed, applied := t.narrowExitDiscriminantChain(g, pred, info, out); applied {
			return narrowed
		}
		if exitHasOrigin && t.scopeExitGuardPathMutated(g, exitOrigin, pred, info) {
			return out
		}
	}
	return t.narrowEdgeInner(pred, out, info, taken, atExit)
}

func (t *Transfer) narrowEdgeInner(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) flow.PointState {
	if flow.PointStateDomain.Equal(out, flow.PointStateDomain.Bottom()) {
		return out
	}
	out = t.narrowByBranchConditionEffect(point, out, info, taken)
	// A multi-return error guard narrows the correlated value siblings independently
	// of the tested error symbol's own refinement, so it composes with whichever base
	// narrower classifies the guard rather than short-circuiting the chain.
	out = t.narrowBySiblingNil(out, info, taken)
	out = t.narrowByGuardedType(out, info, taken)
	// A relational comparison guard (`i <= n`, `i < #arr`) bounds a numeric value on
	// the edge it holds; the bound seeds the numeric component independently of the
	// guard's value narrowing, so it composes too.
	out = t.narrowNumericComparison(out, info, taken)
	// A local type-predicate guard (`if P(arg)` or `if ok` with `local ok = P(arg)`)
	// narrows the predicate argument to the tested kind on the true edge. It refines
	// the argument independently of the truthy narrowing the cond-check applies to the
	// predicate result, so it composes with the chain.
	out = t.narrowByPredicate(out, info, taken)
	if narrowed, applied := t.narrowByCompound(point, out, info, taken); applied {
		return narrowed
	}
	if narrowed, applied := t.narrowByTypeCheck(out, info, taken); applied {
		return narrowed
	}
	if narrowed, applied := t.narrowByDiscriminant(out, info, taken); applied {
		return narrowed
	}
	if narrowed, applied := t.narrowByTypedDiscriminant(out, info, taken); applied {
		return narrowed
	}
	if narrowed, applied := t.narrowByScalarLiteralComparison(out, info, taken, atExit); applied {
		return narrowed
	}
	return t.narrowByCondCheckAtPoint(point, out, info, taken, atExit)
}
