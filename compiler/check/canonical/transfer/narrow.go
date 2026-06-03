package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	abstractcond "github.com/wippyai/go-lua/compiler/check/abstract/cond"
	"github.com/wippyai/go-lua/compiler/check/abstract/literal"
	"github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
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

// narrow.go is the path-sensitive narrowing of the canonical flow: the per-edge
// refinement a branch guard proves about its tested value. It is the canonical
// counterpart of the legacy condition-narrowing, lifted off the legacy
// Solve/Narrow phases and expressed directly over the canonical PointState by
// reusing the SAME value-domain narrowing primitives the legacy flow applies:
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

// fieldResolver is the structural field/index resolver the discriminant narrowing
// reads to look up a variant field's type. It is the pure value-domain resolver
// (types/query/core), not a parallel implementation.
var fieldResolver = querycore.Resolver()

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
	if preferEnv && !av.IsZero() {
		return av, true
	}
	if declared, ok := t.declaredTypes[sym]; ok && declared != nil && !typ.IsAbsentOrUnknown(declared) {
		return product.FromType(declared), true
	}
	if av.IsZero() {
		return product.AbstractValue{}, false
	}
	return av, true
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
	info, ok := g.Info(pred).(*cfg.BranchInfo)
	branchPred := ok && info != nil
	if !ok || info == nil {
		info = exitGuard(g, pred)
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
	}
	return t.narrowEdgeInner(pred, out, info, taken, atExit)
}

func (t *Transfer) genericForBodyEdgeState(g *cfg.Graph, branch cfg.Point, out flow.PointState) flow.PointState {
	node := g.Node(branch)
	if node == nil || !node.LoopPreheaderSet {
		return out
	}
	info := g.Assign(node.LoopPreheader)
	if info == nil || len(info.IterExprs) == 0 {
		return out
	}
	iterCall, ok := info.IterExprs[0].(*ast.FuncCallExpr)
	if !ok {
		iterCall = nil
	}
	res := flow.ClonePointState(out)
	t.applyGenericForBinding(&res, info, iterCall, nil)
	return res
}

// narrowExitDiscriminantChain composes, over the symbol's declared union, every
// discriminant guard that dominates the exit point pred and shares the recovered
// guard's tested symbol. Each dominating guard contributes the refinement of its
// surviving edge (the arm that reaches pred): a guard whose include arm terminates
// excludes its matched variant, the early-return-chain exhaustiveness pattern. The
// composition runs entirely over the declared base, so it is independent of whatever
// widened value the exit's out-state happens to carry. It applies only when the exit
// guard itself is a discriminant on a union-typed symbol and at least two variants of
// the declared union survive composition into a strict refinement; a non-discriminant
// guard, a non-union base, or a no-op composition returns applied=false so the
// ordinary exit narrowing runs.
func (t *Transfer) narrowExitDiscriminantChain(g *cfg.Graph, pred cfg.Point, info *cfg.BranchInfo, out flow.PointState) (flow.PointState, bool) {
	d, ok := t.discriminantGuard(info.Condition)
	if !ok {
		return flow.PointState{}, false
	}
	declared, has := t.declaredTypes[d.sym]
	if !has || declared == nil || typ.IsAbsentOrUnknown(declared) {
		return flow.PointState{}, false
	}
	base := narrow.RemoveNil(declared)
	// Only a genuine literal discriminant partitions the union; a `field == value` on a
	// broad scalar field does not select a variant, so composing it over the declared
	// union would rewrite the symbol to its declared type and clobber a sibling guard's
	// field refinement. Decline so the ordinary exit narrowing runs.
	if !fieldDiscriminatesUnion(base, d.field) {
		return flow.PointState{}, false
	}
	guards := t.dominatingDiscriminants(g, pred, d.sym)
	if len(guards) == 0 {
		return flow.PointState{}, false
	}
	refined := base
	for _, gd := range guards {
		var next typ.Type
		if gd.include {
			next = narrow.ByFieldLiteral(refined, gd.field, gd.literal, fieldResolver)
		} else {
			next = narrow.ExcludeByFieldLiteral(refined, gd.field, gd.literal, fieldResolver)
		}
		if next != nil {
			refined = next
		}
	}
	// The composition must strictly reduce the union (drop at least one variant) to be
	// a refinement: a chain that leaves every declared variant live carries no
	// exhaustiveness narrowing and rebuilding the union here would only lose precision a
	// per-edge guard already established.
	if refined == nil || !unionMembersReduced(base, refined) {
		return flow.PointState{}, false
	}
	res := flow.ClonePointState(out)
	if refined.Kind().IsNever() {
		t.setNarrowedSymbol(&res, d.sym, product.Bottom())
	} else {
		t.setNarrowedSymbol(&res, d.sym, product.FromType(refined))
	}
	return res, true
}

// unionMembersReduced reports whether refined is a strict reduction of the union base
// -- Never, or a type whose top-level union member count is smaller than base's. The
// early-return-chain composition is meaningful only when it drops a variant; an equal
// or larger member count is a no-op (or a rebuild) the exit narrowing should not apply.
func unionMembersReduced(base, refined typ.Type) bool {
	if refined.Kind().IsNever() {
		return true
	}
	bu := unwrap.Union(base)
	ru := unwrap.Union(refined)
	baseN := 1
	if bu != nil {
		baseN = len(bu.Members)
	}
	refinedN := 1
	if ru != nil {
		refinedN = len(ru.Members)
	}
	return refinedN < baseN
}

// dominatingDiscriminants collects, in dominance order, every discriminant guard on
// symbol sym whose branch dominates the exit point pred and exactly one of whose arms
// reaches pred (the other terminated via an early return / error). Each such guard
// contributes its surviving edge: when the include (matching) arm terminates, the
// surviving edge is the exclude of the matched variant; when the exclude arm
// terminates, the surviving edge includes the matched variant. A guard both of whose
// arms reach pred (a plain `if` whose then-arm does not terminate) is not a dominating
// early-return guard and is skipped, since the value past it is the union join, not a
// single refined edge.
func (t *Transfer) dominatingDiscriminants(g *cfg.Graph, pred cfg.Point, sym cfg.SymbolID) []discriminant {
	var guards []discriminant
	seen := map[cfg.Point]bool{pred: true}
	frontier := append([]cfg.Point(nil), g.Predecessors(pred)...)
	for len(frontier) > 0 {
		var next []cfg.Point
		for _, p := range frontier {
			if seen[p] {
				continue
			}
			seen[p] = true
			if bi := g.Branch(p); bi != nil {
				if d, ok := t.discriminantGuard(bi.Condition); ok && d.sym == sym {
					if gd, take := t.survivingDiscriminantEdge(g, p, d, pred); take {
						guards = append(guards, gd)
					}
				}
			}
			next = append(next, g.Predecessors(p)...)
		}
		frontier = next
	}
	// The backward walk yields guards nearest-first; compose them outermost-first so a
	// later include cannot resurrect a variant an earlier exclude removed.
	for i, j := 0, len(guards)-1; i < j; i, j = i+1, j-1 {
		guards[i], guards[j] = guards[j], guards[i]
	}
	return guards
}

// survivingDiscriminantEdge resolves which edge of a dominating discriminant branch
// reaches the exit point pred, and returns the discriminant marked with that edge's
// include/exclude sense. It applies only when exactly one arm reaches pred (the other
// terminated), so the surviving edge holds unconditionally on every path to pred. A
// branch both/neither of whose arms reach pred carries no single-edge refinement.
func (t *Transfer) survivingDiscriminantEdge(g *cfg.Graph, branch cfg.Point, d discriminant, target cfg.Point) (discriminant, bool) {
	var trueSucc, falseSucc cfg.Point
	for _, s := range g.Successors(branch) {
		if taken, ok := g.EdgeCond(branch, s); ok && taken {
			trueSucc = s
		} else {
			falseSucc = s
		}
	}
	trueReaches := trueSucc != 0 && reaches(g, trueSucc, target, branch)
	falseReaches := falseSucc != 0 && reaches(g, falseSucc, target, branch)
	switch {
	case trueReaches && !falseReaches:
		// The true (matching) edge survives: include the matched variant.
		d.include = !d.negated
		return d, true
	case falseReaches && !trueReaches:
		// The false (non-matching) edge survives: exclude the matched variant.
		d.include = d.negated
		return d, true
	default:
		return discriminant{}, false
	}
}

// reaches reports whether target is reachable from start by a forward walk that never
// re-enters the branch node avoid (so the walk stays within the arm it started on).
func reaches(g *cfg.Graph, start, target, avoid cfg.Point) bool {
	if start == target {
		return true
	}
	seen := map[cfg.Point]bool{avoid: true}
	stack := []cfg.Point{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == target {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return false
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
	// A relational comparison guard (`i <= n`, `i < #arr`) bounds a numeric value on
	// the edge it holds; the bound seeds the numeric component independently of the
	// guard's value narrowing, so it composes too.
	out = t.narrowNumericComparison(out, info, taken)
	// A local type-predicate guard (`if P(arg)` or `if ok` with `local ok = P(arg)`)
	// narrows the predicate argument to the tested kind on the true edge. It refines
	// the argument independently of the truthy narrowing the cond-check applies to the
	// predicate result, so it composes with the chain.
	out = t.narrowByPredicate(out, info, taken)
	if narrowed, applied := t.narrowByCompound(out, info, taken); applied {
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
	return t.narrowByCondCheck(out, info, taken, atExit)
}

func (t *Transfer) narrowByBranchConditionEffect(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	if info == nil || info.Condition == nil || t.in.Graph == nil {
		return out
	}
	extractor := abstractcond.ConditionExtractor{
		P:             point,
		Inputs:        t.conditionExtractorInputs(),
		SymResolver:   t.conditionSymbolResolver(&out),
		ConstResolver: t.constResolverAt(point),
	}
	branches := extractor.ConstraintsFromConditionExpr(info.Condition)
	cond := branches.OnFalse
	if taken {
		cond = branches.OnTrue
	}
	if !cond.HasConstraints() {
		return out
	}
	res := flow.ClonePointState(out)
	t.applyConditionEffect(&res, ConditionEffect{Fact: cond})
	return res
}

func (t *Transfer) conditionExtractorInputs() *flow.Inputs {
	if t.in.Graph == nil {
		return nil
	}
	return &flow.Inputs{
		Graph:               t.in.Graph,
		DeclaredTypes:       t.in.Scope.DeclaredTypes,
		ConstValues:         t.in.ConstValues,
		VariantFieldOrigins: t.in.VariantFieldOrigins,
	}
}

func (t *Transfer) conditionSymbolResolver(out *flow.PointState) func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	return func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == 0 {
			return nil, false
		}
		if out != nil {
			av, has := t.symbolValue(out, sym)
			if has && !av.IsZero() {
				if pt := av.ProjectValue(); !typ.IsAbsentOrUnknown(pt) {
					return pt, true
				}
			}
		}
		if t, ok := t.declaredTypes[sym]; ok && !typ.IsAbsentOrUnknown(t) {
			return t, true
		}
		return nil, false
	}
}

// narrowBySiblingNil strips nil from the value siblings of a multi-return error
// assignment when the branch proves the error symbol nil. For `local v, err = f()`
// with f proving the (value, err) inverse pattern, the err-nil edge proves v
// non-nil, so v's flow value narrows by NarrowPresent. It resolves the tested
// symbol the same way the cond-check narrower does (the branch's CondSymbol or its
// resolved root), looks up the recorded sibling correlation for that error symbol,
// and applies the present-narrowing on the success (err == nil) edge. A branch that
// tests no recorded error symbol, or whose edge does not prove the error nil,
// returns out unchanged.
func (t *Transfer) narrowBySiblingNil(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	sym := info.CondSymbol
	if sym == 0 {
		sym = t.condTestSymbol(info)
	}
	if sym == 0 {
		return out
	}
	bind, ok := out.Rel.SiblingNil(sym)
	if !ok {
		return out
	}
	// The success edge is the one on which the error result is proven nil. A
	// field-path error test (`if err.code ~= nil`) does not prove the bare error
	// symbol nil, so the bare-symbol guard is required: a segmented test leaves the
	// correlation untouched.
	if len(t.condTestSegments(info)) > 0 {
		return out
	}
	errIsNil := effectiveCheck(info.CondCheck.Kind, taken)
	switch errIsNil {
	case cfg.CheckNil, cfg.CheckFalsy:
	default:
		return out
	}
	res := flow.ClonePointState(out)
	applied := false
	for _, vs := range bind.ValueSyms {
		if vs == 0 {
			continue
		}
		av, has := t.symbolValue(&res, vs)
		if !has || av.IsZero() {
			continue
		}
		t.setNarrowedSymbol(&res, vs, product.NarrowPresent(av))
		applied = true
	}
	if !applied {
		return out
	}
	return res
}

// narrowByCompound decomposes a short-circuit logical guard (`A and B`, `A or B`)
// the CFG records as a single branch whose Condition is a *ast.LogicalOpExpr — the
// inner operand of a chained `if A or B or C` the CFG collapses into one branch
// node. The simple narrowers cannot classify a logical operand into one CondCheck,
// so it is decomposed by short-circuit semantics on the chosen edge and each
// operand narrowed by the same per-edge machinery:
//
//   - `A and B` truthy (the true edge): BOTH operands are truthy, so each operand's
//     truthy narrowing holds; they compose left-to-right.
//   - `A or B` falsy (the false edge): BOTH operands are falsy, so each operand's
//     falsy narrowing holds; they compose. This is the `not x or not x.f` guard's
//     surviving edge, where `not x` and `not x.f` are both false, hence x and x.f
//     are both truthy.
//   - `A or B` truthy (the true edge): at least one operand holds, an existential.
//     When both operands narrow the SAME tested value, the value is the UNION of each
//     operand's narrowing (narrowOrUnionTrueEdge joins them); when they narrow
//     different values, no single value is known and the edge narrows nothing.
//   - `A and B` falsy is the dual existential. When both operands narrow the SAME
//     tested value, the value is the UNION of each operand's falsy narrowing (e.g.
//     `v and v ~= ""` false proves `v` is falsy OR `v == ""`); different values
//     are left unchanged.
//
// A non-logical condition returns applied=false so the simple narrowers run.
func (t *Transfer) narrowByCompound(out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	logical, ok := info.Condition.(*ast.LogicalOpExpr)
	if !ok {
		return out, false
	}
	// The whole condition's truthiness on this edge: a CheckTruthy branch is truthy on
	// the taken edge and falsy on the not-taken; a CheckFalsy branch is the inverse.
	wantTruthy := taken
	if info.CondCheck.Kind == cfg.CheckFalsy {
		wantTruthy = !taken
	}
	// `A or B` truthy (the true edge) proves only that at least one operand holds,
	// an existential. When both operands narrow the SAME tested value, that
	// existential pins the value to the UNION of each operand's narrowing
	// (narrow_A(x) | narrow_B(x)); when they narrow different values, nothing about
	// any single value is known. The union narrowing handles the former; the latter
	// declines (no decomposable operands) and the value is left unchanged.
	if logical.Operator == "or" && wantTruthy {
		return t.narrowOrUnionTrueEdge(out, logical)
	}
	if logical.Operator == "and" && !wantTruthy {
		return t.narrowAndUnionFalseEdge(out, logical)
	}
	operands, decomposable := compoundOperands(logical, wantTruthy)
	if !decomposable {
		return out, false
	}
	state := out
	applied := false
	for _, operand := range operands {
		narrowed, ok := t.narrowOperand(state, operand)
		if !ok {
			continue
		}
		state = narrowed
		applied = true
	}
	return state, applied
}

// narrowOrUnionTrueEdge narrows the TRUE edge of `A or B` when both operands are
// guards on the SAME tested value: at least one operand holds, so the value is the
// UNION of each operand's narrowing. It narrows each operand independently from the
// shared entering state (each asserted truthy, the polarity the surviving disjunct
// would carry), then requires both to refine EXACTLY the same single Env key. When
// they do, the two refined values are joined on that key by the value-domain LUB
// (product.Join), the sound union the existential proves. When they refine different
// keys, refine more than one key, or one operand classifies no narrowing, the edge
// proves nothing about a single value and the state is left unchanged (applied=false:
// a precision loss, never an over-narrow). Only the value join is applied; the
// per-operand path conditions are NOT recorded, since neither holds unconditionally
// on the existential edge.
func (t *Transfer) narrowOrUnionTrueEdge(out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowSameValueUnion(out,
		operandGuard{expr: logical.Lhs, truthy: true},
		operandGuard{expr: logical.Rhs, truthy: true},
	)
}

// narrowAndUnionFalseEdge narrows the FALSE edge of `A and B` when both operands
// constrain the same value. The edge proves `!A OR !B`; joining each operand's
// false-edge refinement gives the least value fact representable by the product
// value domain without path-splitting.
func (t *Transfer) narrowAndUnionFalseEdge(out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowSameValueUnion(out,
		operandGuard{expr: logical.Lhs, truthy: false},
		operandGuard{expr: logical.Rhs, truthy: false},
	)
}

func (t *Transfer) narrowSameValueUnion(out flow.PointState, lhsGuard, rhsGuard operandGuard) (flow.PointState, bool) {
	lhs, lkey, lok := t.operandNarrowsOneKey(out, lhsGuard)
	if !lok {
		return out, false
	}
	rhs, rkey, rok := t.operandNarrowsOneKey(out, rhsGuard)
	if !rok || lkey != rkey {
		return out, false
	}
	lv, lok := t.symbolValueByKey(lhs, lkey)
	rv, rok := t.symbolValueByKey(rhs, rkey)
	if !lok || !rok {
		return out, false
	}
	joined := product.Join(lv, rv)
	res := flow.ClonePointState(out)
	t.setSymbolValueByKey(&res, lkey, joined, false)
	return res, true
}

// operandNarrowsOneKey narrows state by one logical operand and reports the
// single value key it refined. It returns ok=true only when the operand refines
// EXACTLY one key, so the caller can join two disjuncts that agree on which
// value they constrain; an operand that refines no key or more than one key is
// not a same-value disjunct and returns ok=false.
func (t *Transfer) operandNarrowsOneKey(state flow.PointState, guard operandGuard) (flow.PointState, flow.ValueKey, bool) {
	narrowed, ok := t.narrowOperand(state, guard)
	if !ok {
		return flow.PointState{}, "", false
	}
	var changed flow.ValueKey
	stateFacts := flow.PointFactsOf(state)
	for k, av := range narrowed.Env {
		base, had := stateFacts.EnvValue(k)
		if had && product.Domain.Equal(base, av) {
			continue
		}
		if changed != "" {
			return flow.PointState{}, "", false
		}
		changed = k
	}
	for _, cell := range narrowed.Cells.Entries() {
		base, _ := state.Cells.Value(cell.Symbol)
		if product.Domain.Equal(base, cell.Value) {
			continue
		}
		if changed != "" {
			return flow.PointState{}, "", false
		}
		changed = flow.SymbolValueKey(cell.Symbol)
	}
	for _, cell := range state.Cells.Entries() {
		if _, ok := narrowed.Cells.Value(cell.Symbol); ok {
			continue
		}
		if changed != "" {
			return flow.PointState{}, "", false
		}
		changed = flow.SymbolValueKey(cell.Symbol)
	}
	if changed == "" {
		return flow.PointState{}, "", false
	}
	return narrowed, changed, true
}

// compoundOperands returns the operands a logical guard narrows on the chosen edge
// and whether the edge is decomposable. `A and B` truthy and `A or B` falsy prove
// both operands' polarity (each as the operand expr paired with its proven
// truthiness); the other two edges prove only an existential and narrow nothing.
func compoundOperands(logical *ast.LogicalOpExpr, wantTruthy bool) ([]operandGuard, bool) {
	switch logical.Operator {
	case "and":
		if !wantTruthy {
			return nil, false
		}
		return []operandGuard{{logical.Lhs, true}, {logical.Rhs, true}}, true
	case "or":
		if wantTruthy {
			return nil, false
		}
		return []operandGuard{{logical.Lhs, false}, {logical.Rhs, false}}, true
	default:
		return nil, false
	}
}

// operandGuard is one operand of a decomposed logical guard paired with the
// truthiness the edge proves for it.
type operandGuard struct {
	expr   ast.Expr
	truthy bool
}

// narrowOperand narrows state by one decomposed logical operand asserted to the
// given truthiness. It classifies the operand the same way the CFG classifies a
// branch condition (extraction.ExtractCondition) and resolves the tested symbol the
// same way AddCondBranch does (the root identifier of the path), then runs the
// per-edge narrowing machinery on a synthetic BranchInfo. A logical sub-operand
// recurses; a leaf flows through the discriminant / typeof / cond-check narrowers.
func (t *Transfer) narrowOperand(state flow.PointState, og operandGuard) (flow.PointState, bool) {
	condVar, check := extraction.ExtractCondition(og.expr)
	leaf := &cfg.BranchInfo{
		CondVar:    condVar,
		CondSymbol: t.condRootSymbol(og.expr, condVar),
		CondCheck:  check,
		Condition:  og.expr,
	}
	narrowed := t.narrowEdgeInner(0, state, leaf, og.truthy, false)
	if statesEqualForNarrow(narrowed, state) {
		return state, false
	}
	return narrowed, true
}

// condRootSymbol resolves the symbol a leaf condition tests: the root identifier of
// the condition's path (the x in x, x.f, type(x.f) == k, x ~= nil), mirroring
// AddCondBranch's symbol resolution so a synthetic leaf info carries the same
// CondSymbol the CFG would. A condition not rooted at a tracked identifier yields 0.
func (t *Transfer) condRootSymbol(expr ast.Expr, condVar string) cfg.SymbolID {
	if sym, ok := t.condRootSymbolFromExpr(expr); ok {
		return sym
	}
	return 0
}

// condRootSymbolFromExpr returns the root symbol a leaf condition expression tests,
// descending the value-bearing operand of each recognized shape. Static access
// recognition is delegated to Place's static-access lowering, so branch guards use
// the same path identity rules as writes, facts, casts, and param effects.
func (t *Transfer) condRootSymbolFromExpr(expr ast.Expr) (cfg.SymbolID, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return t.staticRootSymbolOfExpr(e)
	case *ast.UnaryNotOpExpr:
		return t.condRootSymbolFromExpr(e.Expr)
	case *ast.RelationalOpExpr:
		if sym, ok := t.relandRootSymbol(e.Lhs); ok {
			return sym, true
		}
		return t.relandRootSymbol(e.Rhs)
	case *ast.FuncCallExpr:
		if e.Method == "" && e.Receiver == nil && len(e.Args) == 1 {
			if fn, ok := e.Func.(*ast.IdentExpr); ok && fn.Value == "type" {
				return t.condRootSymbolFromExpr(e.Args[0])
			}
		}
	}
	return 0, false
}

// relandRootSymbol resolves the root symbol of a relational operand. A literal or
// unrecognized operand reports false so the other side is tried.
func (t *Transfer) relandRootSymbol(expr ast.Expr) (cfg.SymbolID, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return t.staticRootSymbolOfExpr(e)
	case *ast.FuncCallExpr:
		return t.condRootSymbolFromExpr(e)
	}
	return 0, false
}

// statesEqualForNarrow reports whether a narrowing left the abstract edge state
// unchanged. Compound guards compose every proven fact, not only Env/Cells
// refinements: a decomposed operand may contribute the path condition, a static
// member fact, key presence, or another product axis even when the value lattice
// already had no sharper type to install.
func statesEqualForNarrow(a, b flow.PointState) bool {
	return flow.PointStateDomain.Equal(a, b)
}

// narrowByTypedDiscriminant applies a discriminated-union narrowing for a guard of
// the shape base.field == other (or ~=), where other is an identifier whose value
// the flow tracks (e.g. a typed channel handle). It narrows base's union to the
// members whose field type intersects other's type (the include edge) and excludes
// the members whose field is exactly other's type (the exclude edge). It is the
// typed counterpart of narrowByDiscriminant: where that compares a field to a
// literal tag, this compares it to a value of a sealed (literal-discriminated)
// variant type, the channel-select idiom result.channel == events_ch. A guard whose
// other side is not a tracked typed identifier, or whose field types do not
// discriminate, returns applied=false.
func (t *Transfer) narrowByTypedDiscriminant(out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	g, otherType, ok := t.typedDiscriminantGuard(out, info.Condition)
	if !ok {
		return out, false
	}
	av, has := t.symbolValue(&out, g.sym)
	if !has || av.IsZero() {
		return out, false
	}
	base := av.ProjectValue()
	if base == nil {
		return out, false
	}
	include := taken != g.negated
	var refine func(typ.Type) typ.Type
	if include {
		// Keep members whose field can be the other variant: intersect the field
		// type with other's type, dropping a member whose field cannot overlap. Two
		// sealed-variant records with conflicting literal discriminants (channel:
		// {__tag: "event"} vs channel: {__tag: "timeout"}) do not overlap, so the
		// member is impossible on the include edge and is dropped (Never). Without the
		// overlap test, narrow.Intersect would synthesize a non-empty structural
		// intersection of the disjoint records and wrongly keep the member.
		refine = func(ft typ.Type) typ.Type {
			if !narrow.TypesOverlap(ft, otherType) {
				return typ.Never
			}
			if typ.TypeEquals(ft, otherType) {
				return ft
			}
			return narrow.Intersect(ft, otherType)
		}
	} else {
		// Exclude members whose field is exactly the other variant; a member whose
		// field is a broader type is kept (it might hold a different value).
		refine = func(ft typ.Type) typ.Type {
			if fieldExactlyType(ft, otherType) {
				return typ.Never
			}
			return ft
		}
	}
	refined := mapUnionField(base, g.field, refine, false)
	if refined == nil || refined == base {
		return out, false
	}
	res := flow.ClonePointState(out)
	if refined.Kind().IsNever() {
		t.setNarrowedSymbol(&res, g.sym, product.Bottom())
	} else {
		t.setNarrowedSymbol(&res, g.sym, product.FromType(refined))
	}
	return res, true
}

// fieldExactlyType reports whether the field type and other denote the same variant
// (mutually subtype), the condition under which a `~=` edge can soundly exclude the
// member. A broader field type is not excluded.
func fieldExactlyType(ft, other typ.Type) bool {
	if typ.TypeEquals(ft, other) {
		return true
	}
	return subtype.IsSubtype(ft, other) && subtype.IsSubtype(other, ft)
}

// typedDiscriminantGuard recognizes base.field == other / base.field ~= other where
// base binds to a tracked symbol and other is an identifier whose value the flow
// tracks. It returns the discriminant (with a nil literal — the comparison is by
// type) and other's resolved type. Only a discriminating other type (a record
// carrying a literal field, the sealed-variant shape) qualifies, so a plain value
// equality that does not discriminate a union is left to the other narrowers.
func (t *Transfer) typedDiscriminantGuard(out flow.PointState, expr ast.Expr) (discriminant, typ.Type, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return discriminant{}, nil, false
	}
	negated := false
	switch rel.Operator {
	case "==":
	case "~=":
		negated = true
	default:
		return discriminant{}, nil, false
	}
	if d, ot, ok := t.typedDiscriminantFromSides(out, rel.Lhs, rel.Rhs, negated); ok {
		return d, ot, true
	}
	return t.typedDiscriminantFromSides(out, rel.Rhs, rel.Lhs, negated)
}

func (t *Transfer) typedDiscriminantFromSides(out flow.PointState, access, value ast.Expr, negated bool) (discriminant, typ.Type, bool) {
	attr, ok := access.(*ast.AttrGetExpr)
	if !ok {
		return discriminant{}, nil, false
	}
	field, ok := staticAttrFieldName(attr)
	if !ok {
		return discriminant{}, nil, false
	}
	baseIdent, ok := attr.Object.(*ast.IdentExpr)
	if !ok {
		return discriminant{}, nil, false
	}
	sym := t.symbolOf(baseIdent)
	if sym == 0 {
		return discriminant{}, nil, false
	}
	otherIdent, ok := value.(*ast.IdentExpr)
	if !ok {
		return discriminant{}, nil, false
	}
	otherType := t.trackedIdentType(out, otherIdent)
	if !isDiscriminatingType(otherType) {
		return discriminant{}, nil, false
	}
	return discriminant{sym: sym, field: field, negated: negated}, otherType, true
}

// trackedIdentType resolves an identifier's value type for typed path-equality
// guards. The live product value is preferred when it is discriminating, but an
// immutable declared singleton variant is allowed to recover precision the value
// product admitted away (for example a parameter `ch: ChanInt` whose live product
// projection is `{__tag: string}`). The declared recovery is sound because it is a
// flow-insensitive upper bound on every value the symbol may hold; the overlap
// check prevents using a stale declared fact when the live product is already
// contradictory.
func (t *Transfer) trackedIdentType(out flow.PointState, ident *ast.IdentExpr) typ.Type {
	sym := t.symbolOf(ident)
	if sym == 0 {
		return nil
	}
	var declared typ.Type
	if t.declaredTypes != nil {
		declared = t.declaredTypes[sym]
	}
	av, ok := t.symbolValue(&out, sym)
	if ok && !av.IsZero() {
		projected := av.ProjectValue()
		if !typ.IsAbsentOrUnknown(projected) {
			if isDiscriminatingType(projected) {
				return projected
			}
			if isDiscriminatingType(declared) && narrow.TypesOverlap(projected, declared) {
				return declared
			}
			return projected
		}
	}
	if !typ.IsAbsentOrUnknown(declared) {
		return declared
	}
	return nil
}

// isDiscriminatingType reports whether t is a sealed-variant type that can
// discriminate a union by value equality on a field. Two shapes qualify:
//
//   - a record carrying at least one literal-typed field (the __tag / kind
//     discriminant a setmetatable-sealed variant records use); or
//   - a generic instantiation carrying a concrete type argument (Channel<Event>,
//     the channel-select handle): two instantiations of the same generic with
//     distinct type arguments are provably disjoint (narrow.TypesOverlap routes
//     Instantiated pairs through instantiatedTypesOverlap, which requires equal
//     type args), so a `result.channel == events_ch` guard selects the case whose
//     channel field is exactly Channel<Event> and drops the disjoint cases.
//
// A non-record / non-instantiation, a record with no literal field, or an
// instantiation whose type arguments are themselves gradual (any/unknown) cannot
// soundly narrow a union by value equality.
func isDiscriminatingType(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Record:
		for _, f := range v.Fields {
			if _, isLit := f.Type.(*typ.Literal); isLit {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		if v.Generic == nil || len(v.TypeArgs) == 0 {
			return false
		}
		// A gradual type argument makes two instantiations indistinguishable
		// (Channel<any> overlaps everything), so it cannot discriminate.
		for _, a := range v.TypeArgs {
			if a == nil || typ.IsAny(a) || typ.IsUnknown(a) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// fieldDiscriminatesUnion reports whether field is a literal discriminant of the
// union base -- a tagged-union tag (kind/__tag/...) whose value distinguishes the
// variants. It requires base to unwrap (through an optional) to a multi-member union
// at least one of whose members types the field as a literal, the shape a genuine
// discriminated union carries. A guard on a non-union base, or on a union field whose
// type is a broad scalar (a `field == ""` on a `string?` field), is an ordinary value
// equality that does not partition the union, so it is left to the plain narrowers and
// the discriminant-specific nil-strip / exhaustiveness composition do not engage.
func fieldDiscriminatesUnion(base typ.Type, field string) bool {
	u := unwrap.Union(base)
	if u == nil || len(u.Members) < 2 {
		return false
	}
	for _, m := range u.Members {
		ft, ok := fieldResolver.Field(m, field)
		if !ok || ft == nil {
			continue
		}
		if _, isLit := unwrap.Alias(ft).(*typ.Literal); isLit {
			return true
		}
	}
	return false
}

// exitGuard synthesizes the branch guard a then-exit / else-exit ScopeExit node
// carries for unsplit conditions. The CFG copies a branch's CondVar (the tested
// ROOT symbol) and CondCheck onto both arm-exit ScopeExit nodes
// (compiler/cfg/stmt.go IfStmt), so a post-`if` merge and a read after an early
// return in the other arm both reach a predecessor that holds the guard markers but
// is NOT a *cfg.BranchInfo. This reconstructs the equivalent BranchInfo so the same
// narrowing helpers fire on those preds. Split short-circuit conditions do not copy
// these markers; their per-edge facts are already represented by the branch edges.
//
// The node carries only the ROOT symbol, never the field path: a field-path guard
// (`if current.updated_at == nil`) copies the root `current` symbol and the nil
// check onto the exit node, dropping the `.updated_at` segment. Re-narrowing the
// BARE root by that check would be UNSOUND — `current.updated_at == nil` proves
// nothing about `current` itself, yet a bare nil-check on the then-arm would pin
// `current` to nil and corrupt a later `return current`. So the originating branch
// is recovered by a backward walk to the nearest matching NodeBranch, whose intact
// BranchInfo (full Condition AST + field-path CondVar) drives the narrowing: the
// field-path narrowing then refines the FIELD slot, leaving the root symbol intact.
// The branch is always a backward ancestor of both its arm exits (condEntry ->
// thenStart/elseStart -> ... -> thenExit/elseExit), so the walk finds it.
//
// When no originating branch is recoverable (a degenerate graph), the lossy bare
// reconstruction is returned only for a bare-symbol root path — a field-path guard
// declines rather than narrow the wrong (root) value. Edge polarity is selected by
// EdgeCond on the exit node's outgoing edge: the then-exit's edge to the merge is
// the TRUE edge, the else-exit's the FALSE edge.
func exitGuard(g *cfg.Graph, pred cfg.Point) *cfg.BranchInfo {
	node := g.Node(pred)
	if node == nil || node.Kind != cfg.NodeScopeExit {
		return nil
	}
	if node.CondVar == 0 && node.CondCheck.Kind == cfg.CheckNone {
		return nil
	}
	if info := originatingBranch(g, pred, node.CondVar, node.CondCheck); info != nil {
		return info
	}
	return &cfg.BranchInfo{
		CondVar:    g.NameOf(node.CondVar),
		CondSymbol: node.CondVar,
		CondCheck:  node.CondCheck,
	}
}

// originatingBranch finds the branch a ScopeExit copied its guard markers from: the
// nearest NodeBranch reached by a backward walk over predecessors whose CondSymbol
// and CondCheck match the markers the exit node carries. It returns that branch's
// full BranchInfo (with the intact Condition AST and field-path CondVar string), so
// the exit re-narrowing operates on the SAME path the branch tested, recovering the
// field segments the node-level markers drop. A no-match (no matching branch is a
// backward ancestor) returns nil so the caller falls back to the bare reconstruction.
func originatingBranch(g *cfg.Graph, exit cfg.Point, condSym cfg.SymbolID, check cfg.CondCheck) *cfg.BranchInfo {
	seen := map[cfg.Point]bool{exit: true}
	frontier := append([]cfg.Point(nil), g.Predecessors(exit)...)
	for len(frontier) > 0 {
		var next []cfg.Point
		for _, p := range frontier {
			if seen[p] {
				continue
			}
			seen[p] = true
			if info := g.Branch(p); info != nil && info.CondSymbol == condSym && info.CondCheck == check {
				return info
			}
			next = append(next, g.Predecessors(p)...)
		}
		frontier = next
	}
	return nil
}

// narrowByTypeCheck applies the value-narrowing a `local val, err = T:is(x)` guard
// proves. When the branch tests the error symbol of such an assignment and the
// chosen edge is the success edge (err proven nil — `if err == nil` true edge, or
// `if err`/`if err ~= nil` false edge), the checked value symbols narrow to the
// checked type T. It reuses the recorded TypeCheckBind (the canonical counterpart of
// the legacy Type:is PredicateLink); a non-type-check guard returns applied=false so
// the discriminant / cond-check paths run.
func (t *Transfer) narrowByTypeCheck(out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	if t.typeCheckByErr == nil || info.CondSymbol == 0 {
		return out, false
	}
	bind, ok := t.typeCheckByErr[info.CondSymbol]
	if !ok {
		return out, false
	}
	// The success edge is the one on which the error result is proven nil.
	errIsNil := effectiveCheck(info.CondCheck.Kind, taken)
	switch errIsNil {
	case cfg.CheckNil, cfg.CheckFalsy:
	default:
		return out, false
	}
	val := product.FromType(bind.Type)
	if val.IsZero() {
		return out, false
	}
	res := flow.ClonePointState(out)
	for _, sym := range bind.NarrowSyms {
		if sym == 0 {
			continue
		}
		t.setNarrowedSymbol(&res, sym, val)
	}
	return res, true
}

// narrowByCondCheck applies the simple condition-check guard the CFG pre-extracted
// onto the branch (truthy/falsy/nil/not-nil/typeof). The taken flag selects the
// edge polarity: a true edge applies the guard as stated, a false edge its
// negation.
//
// The tested path is either the bare symbol (CondVar resolves to no field
// segments) or a field path under it (if x.f, type(x.f) == k). A field-path guard
// narrows the FIELD slot inside the base symbol's record value, leaving the rest
// of the record untouched, so a read of that field path observes the refinement
// while the base symbol stays its full type.
func (t *Transfer) narrowByCondCheck(out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) flow.PointState {
	check := effectiveCheck(info.CondCheck.Kind, taken)
	if check == cfg.CheckNone {
		return out
	}
	out = t.narrowGuardedIndexPresence(out, info, check)
	sym := t.condTestSymbol(info)
	if sym == 0 {
		return out
	}
	// A comparison guard (`op.type == tag`, `a ~= b`) whose CondCheck fell through to a
	// bare truthy/falsy carries no truthiness fact about its operand: the truthiness of
	// `a == b` says nothing about whether `a` itself is truthy. The CFG records such a
	// comparison as CheckTruthy on the comparison's root symbol (here `op`), but no
	// presence/type/discriminant narrower claimed it (those carry CheckNil / CheckTypeEqual
	// / their own discriminant handling and ran before this fall-through). Narrowing the
	// operand by truthy/falsy here would corrupt it -- on the false edge NarrowFalsy(any)
	// pins the operand to `false?`. Decline so the operand keeps its value; the comparison
	// is still type-checked through the ordinary expression demand.
	if comparisonTruthyOnOperand(info.Condition, check) {
		return out
	}
	segments := t.condTestSegments(info)
	baseAV, has := t.narrowBaseFor(out, sym, atExit)

	cond := condForCheck(sym, segments, info.CondVar, check, info.CondCheck.TypeName)
	res := flow.ClonePointState(out)
	if cond.HasConstraints() {
		t.applyConditionEffect(&res, ConditionEffect{Fact: cond})
	}
	// A positive guard on a literal index path `arr[i]` (`if arr[i]`, `arr[i] ~= nil`)
	// proves the element at index i is present, so the container holds at least i
	// elements: record `#arr >= i` on this edge. A later read of `arr[i]` in the
	// guarded body reads it in-range and drops the soundly-optional element nil
	// through the existing length proof, recovering the non-optional element exactly
	// where the guard establishes presence. The merge-LUB rebuilds the unbounded
	// length where both edges meet, so the floor never survives past the guard.
	res = t.narrowIndexPresenceLength(res, sym, segments, check)
	t.refineStaticMemberFactForCheck(&res, sym, segments, baseAV, has, check, info.CondCheck.TypeName)
	if !has {
		// No tracked value to refine; the per-edge path condition still records the
		// guard for soundness. For a bare-symbol positive guard, also materialize the
		// reduced product's presence component: the structure is still dynamic, but
		// this edge has proven the symbol non-nil. Without this reduction a later
		// return projection sees "no Env fact" instead of "present dynamic".
		if len(segments) == 0 {
			switch check {
			case cfg.CheckTruthy, cfg.CheckNotNil:
				t.setNarrowedSymbol(&res, sym, product.PresentDynamic())
			}
		}
		return res
	}
	narrowed, ok := narrowAtPath(baseAV, segments, check, info.CondCheck.TypeName)
	if !ok {
		return res
	}
	t.setNarrowedSymbol(&res, sym, narrowed)
	return res
}

func (t *Transfer) narrowGuardedIndexPresence(out flow.PointState, info *cfg.BranchInfo, check cfg.CondCheckKind) flow.PointState {
	access := guardedIndexPresenceAccess(info, check)
	if access == nil {
		return out
	}
	effect, ok := t.guardedIndexPresenceEffect(access)
	if !ok {
		return out
	}
	res := flow.ClonePointState(out)
	t.applyKeyProvenanceEffect(&res, effect)
	return res
}

func (t *Transfer) guardedIndexPresenceEffect(access *ast.AttrGetExpr) (KeyProvenanceEffect, bool) {
	if access == nil {
		return KeyProvenanceEffect{}, false
	}
	if _, isStatic := staticMemberKey(access); isStatic {
		return KeyProvenanceEffect{}, false
	}
	tablePath, ok := t.containerExprPath(access.Object)
	if !ok || tablePath.IsEmpty() {
		return KeyProvenanceEffect{}, false
	}
	keyPath, ok := t.dynamicIndexKeyPath(access.Key)
	if !ok || keyPath.IsEmpty() {
		return KeyProvenanceEffect{}, false
	}
	return KeyProvenanceEffect{
		Kind:      KeyProvenanceGuardedIndex,
		TablePath: tablePath,
		KeyPath:   keyPath,
	}, true
}

func guardedIndexPresenceAccess(info *cfg.BranchInfo, check cfg.CondCheckKind) *ast.AttrGetExpr {
	if info == nil || !checkProvesIndexPresence(check) {
		return nil
	}
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		return attrAccess(info.Condition)
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return attrAccess(not.Expr)
		}
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			return nilComparisonAttrAccess(rel)
		}
	}
	return nil
}

func checkProvesIndexPresence(check cfg.CondCheckKind) bool {
	return check == cfg.CheckTruthy || check == cfg.CheckNotNil
}

func attrAccess(expr ast.Expr) *ast.AttrGetExpr {
	attr, _ := expr.(*ast.AttrGetExpr)
	return attr
}

func nilComparisonAttrAccess(rel *ast.RelationalOpExpr) *ast.AttrGetExpr {
	if rel == nil {
		return nil
	}
	if _, ok := rel.Rhs.(*ast.NilExpr); ok {
		return attrAccess(rel.Lhs)
	}
	if _, ok := rel.Lhs.(*ast.NilExpr); ok {
		return attrAccess(rel.Rhs)
	}
	return nil
}

func (t *Transfer) refineStaticMemberFactForCheck(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, baseAV product.AbstractValue, hasBase bool, check cfg.CondCheckKind, typeName string) {
	place, ok := staticPlace(sym, segments)
	if !ok {
		return
	}
	t.applyStaticMemberRefinementEffect(out, StaticMemberRefinementEffect{
		Place:    place,
		Base:     baseAV,
		HasBase:  hasBase,
		Check:    check,
		TypeName: typeName,
	})
}

func staticMemberGuardImpliesPresence(check cfg.CondCheckKind, typeName string) bool {
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil:
		return true
	case cfg.CheckTypeEqual:
		return kind.FromString(typeName) != kind.Nil && kind.FromString(typeName) != kind.Unknown
	case cfg.CheckTypeNot:
		return kind.FromString(typeName) == kind.Nil
	default:
		return false
	}
}

// narrowByPredicate applies the value-narrowing a local type-predicate guard
// proves. A predicate is a local function whose body returns a builtin
// `type(param) == kind` test on one of its parameters (recorded as a guard-owned
// predicate function fact, the canonical counterpart of the legacy
// LocalTypePredicateEvidence). On the edge
// the predicate result holds true, the argument passed at the tested parameter
// narrows to that kind, exactly as a direct `type(arg) == kind` guard would.
//
// The narrowing is ONE-SIDED: a predicate body is a conjunction of conditions
// (`type(v) == "number" and v > 0`), so a false result does not prove the argument
// is NOT the kind. Only the true edge narrows; the false edge leaves the argument
// its declared (gradual) type, preserving the legacy PredicateLink direction and
// the soundness boundary the false-branch adversarial cases pin.
//
// It recognizes both forms: the direct call `if P(arg)` (the branch condition is the
// call, or its negation under `if not P(arg)`), and the assigned result
// `local ok = P(arg); if ok` (the branch tests the recorded ok symbol).
func (t *Transfer) narrowByPredicate(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	if len(t.predicateByFunc) == 0 && len(t.predicateByCondSym) == 0 {
		return out
	}
	argSym, kind, trueEdge, ok := t.predicateGuardForBranch(info, taken)
	if !ok || argSym == 0 || kind == "" || !trueEdge {
		return out
	}
	if _, known := narrow.KnownBuiltinTypeKey(kind); !known {
		return out
	}
	baseAV, has := t.narrowBaseFor(out, argSym, false)
	if !has {
		return out
	}
	narrowed, ok := narrowAtPath(baseAV, nil, cfg.CheckTypeEqual, kind)
	if !ok {
		return out
	}
	res := flow.ClonePointState(out)
	t.setNarrowedSymbol(&res, argSym, narrowed)
	return res
}

// predicateGuardForBranch resolves the predicate-narrowing a branch carries: the
// argument symbol proved to be the tested kind, and whether the taken edge is the
// edge on which the predicate result holds true. It recognizes the direct call
// `if P(arg)` (and its `if not P(arg)` negation, where the proven edge is the false
// edge) and the assigned result `if ok` (with `local ok = P(arg)` recorded). A
// branch that tests no predicate yields ok=false.
func (t *Transfer) predicateGuardForBranch(info *cfg.BranchInfo, taken bool) (argSym cfg.SymbolID, kind string, trueEdge, ok bool) {
	// Assigned form: the branch tests the ok symbol of `local ok = P(arg)`.
	if info.CondSymbol != 0 {
		if g, found := t.predicateByCondSym[info.CondSymbol]; found {
			// The bare-symbol truthy/falsy check selects the edge: a truthy resolved check
			// is the predicate-true edge. A segmented test (`if ok.field`) does not test the
			// predicate result, so it carries no predicate fact.
			if len(t.condTestSegments(info)) > 0 {
				return 0, "", false, false
			}
			check := effectiveCheck(info.CondCheck.Kind, taken)
			return g.NarrowSym, g.Kind, check == cfg.CheckTruthy, true
		}
	}
	// Direct form: the branch condition is the predicate call, optionally negated.
	cond := info.Condition
	trueEdge = taken
	if not, isNot := cond.(*ast.UnaryNotOpExpr); isNot {
		cond = not.Expr
		trueEdge = !taken
	}
	call, isCall := cond.(*ast.FuncCallExpr)
	if !isCall || call == nil {
		return 0, "", false, false
	}
	sym, kindName, found := t.predicateCallNarrow(call)
	if !found {
		return 0, "", false, false
	}
	return sym, kindName, trueEdge, true
}

// predicateCallNarrow resolves a predicate call `P(arg)` to the argument symbol it
// narrows and the kind it proves: P's symbol must name a recorded guard predicate
// function, and the argument at the tested parameter index must resolve to an
// identifier symbol.
// A method-like call, an unrecognized callee, or a non-identifier argument yields
// found=false.
func (t *Transfer) predicateCallNarrow(call *ast.FuncCallExpr) (argSym cfg.SymbolID, kind string, found bool) {
	if call == nil || len(t.predicateByFunc) == 0 {
		return 0, "", false
	}
	fnIdent, ok := call.Func.(*ast.IdentExpr)
	if !ok || fnIdent == nil {
		return 0, "", false
	}
	fnSym := t.symbolOf(fnIdent)
	if fnSym == 0 {
		return 0, "", false
	}
	fact, ok := t.predicateByFunc[fnSym]
	if !ok || fact.Kind == "" {
		return 0, "", false
	}
	if fact.ParamIndex < 0 || fact.ParamIndex >= len(call.Args) {
		return 0, "", false
	}
	argIdent, ok := call.Args[fact.ParamIndex].(*ast.IdentExpr)
	if !ok || argIdent == nil {
		return 0, "", false
	}
	sym := t.symbolOf(argIdent)
	if sym == 0 {
		return 0, "", false
	}
	return sym, fact.Kind, true
}

// comparisonTruthyOnOperand reports whether the guard condition is a value comparison
// (`a == b`, `a ~= b`, `a < b`, ...) whose resolved check is the fall-through truthy /
// falsy, in which case the operand's truthiness is NOT constrained by the comparison's
// result. A comparison the CFG could interpret as a presence/type guard carries
// CheckNil / CheckNotNil / CheckTypeEqual / CheckTypeNot instead (and is narrowed
// correctly); only the uninterpreted comparison reduces to the bare truthy/falsy that
// would otherwise project the comparison's truthiness onto its root operand symbol. A
// `not (a == b)` wraps the comparison in a CheckFalsy, so the inner comparison is
// unwrapped. A non-comparison condition (a bare value / field path) returns false so the
// ordinary truthy/falsy narrowing runs.
func comparisonTruthyOnOperand(cond ast.Expr, check cfg.CondCheckKind) bool {
	switch check {
	case cfg.CheckTruthy, cfg.CheckFalsy:
	default:
		return false
	}
	expr := cond
	if not, ok := expr.(*ast.UnaryNotOpExpr); ok {
		expr = not.Expr
	}
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return false
	}
	switch rel.Operator {
	case "==", "~=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

// narrowIndexPresenceLength records `#arr >= i` on the guarded edge when the guard
// is a positive presence check (truthy / not-nil) on a literal index path arr[i]
// rooted at sym. A present element at index i implies the container has at least i
// elements, the sound length floor an in-range index read consults to drop its
// optional element. A negative check, a non-index or non-literal-index path, or a
// missing numeric component leaves the state unchanged. The numeric component is
// cloned before the floor is applied so the shared predecessor state is never
// mutated; the merge-LUB rebuilds the unbounded length past the guard.
func (t *Transfer) narrowIndexPresenceLength(res flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind) flow.PointState {
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil:
	default:
		return res
	}
	if len(segments) != 1 {
		return res
	}
	seg := segments[0]
	if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 {
		return res
	}
	t.applyNumericEffect(&res, NumericEffect{
		Ops: []NumericOp{{
			Kind:  NumericLenGeConst,
			Key:   constraint.PathKey(flow.SymbolValueKey(sym)),
			Const: int64(seg.Index),
		}},
		RequireExisting: true,
	})
	return res
}

// narrowNumericComparison seeds the numeric component with the integer bound a
// relational comparison guard proves on the edge it holds. It recognizes a guard
// `var OP bound` where var is a tracked symbol and bound is an integer constant or
// `#container` over a tracked sequence, on either side of the comparison. On the
// taken edge the comparison's effective operator holds; on the not-taken edge its
// negation. The proven `var <= c` / `var >= c` is applied as a constant bound, and
// `var <= #container (+/- k)` as a symbolic length reference, the same bound forms
// a numeric-for loop seeds, so a body read `container[var]` consults them through
// the in-range index narrowing. A guard the helper cannot classify as a numeric
// comparison on a tracked symbol leaves the state unchanged (precision loss, never
// unsoundness). The numeric component is cloned before the bound is applied so the
// shared predecessor state is never mutated; the merge-LUB rebuilds the unbounded
// range past the guard.
func (t *Transfer) narrowNumericComparison(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	if info == nil {
		return out
	}
	rel, ok := info.Condition.(*ast.RelationalOpExpr)
	if !ok {
		return out
	}
	// A `#container OP const` length guard (`#x > 0`, `#x >= 1`, `#x ~= 0`, `#x == 0`)
	// bounds the container's length on the edge it holds, raising the proven length
	// floor (or ceiling) the same numeric component an in-range index read consults.
	// It is recognized before the index-variable orientation because the guarded value
	// is `#container`, not a tracked integer variable.
	if narrowed, applied := t.narrowLengthGuard(out, rel, info, taken); applied {
		return narrowed
	}
	op := rel.Operator
	switch op {
	case "<", "<=", ">", ">=":
	default:
		return out
	}
	// Resolve which side is the tracked integer variable and which is the bound,
	// orienting the operator so it always reads `var OP bound`.
	varExpr, boundExpr, op, ok := t.orientComparison(rel.Lhs, rel.Rhs, op)
	if !ok {
		return out
	}
	idxIdent, ok := varExpr.(*ast.IdentExpr)
	if !ok {
		return out
	}
	idxSym := t.symbolOf(idxIdent)
	if idxSym == 0 {
		return out
	}
	// The CFG records the whole comparison branch as a truthy check; the taken edge
	// holds the comparison as written, the not-taken edge its logical negation.
	if !effectiveTruthy(info.CondCheck.Kind, taken) {
		op = negateComparisonOp(op)
	}
	idxKey := constraint.PathKey(flow.SymbolValueKey(idxSym))
	if c, ok := t.constInt(boundExpr); ok {
		ops := numericConstComparisonOps(idxKey, op, c)
		if len(ops) == 0 {
			return out
		}
		res := flow.ClonePointState(out)
		t.applyNumericEffect(&res, NumericEffect{Ops: ops})
		return res
	}
	// `var <= #container` / `var < #container`: a symbolic length reference. Only the
	// upper-bound senses bound the index by the container length; a lower-bound sense
	// does not establish in-range presence and is left unseeded.
	if arrKey, off, ok := t.lengthBoundComparison(boundExpr, op); ok {
		res := flow.ClonePointState(out)
		t.applyNumericEffect(&res, NumericEffect{
			Ops: []NumericOp{{
				Kind:   NumericVarLeLenOffset,
				Key:    idxKey,
				Other:  arrKey,
				Offset: off,
			}},
		})
		return res
	}
	return out
}

// narrowByScalarLiteralComparison refines a tested place by a scalar literal
// equality/inequality guard (`x == "tag"`, `x ~= ""`, `obj.kind == true`). Field
// discriminants have a stronger union-aware path and run before this helper; this
// fallback covers ordinary scalar slots and locals. It is an edge transfer: the
// taken flag decides whether the comparison as written holds or its negation holds.
func (t *Transfer) narrowByScalarLiteralComparison(out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) (flow.PointState, bool) {
	if info == nil {
		return out, false
	}
	rel, ok := info.Condition.(*ast.RelationalOpExpr)
	if !ok {
		return out, false
	}
	if rel.Operator != "==" && rel.Operator != "~=" {
		return out, false
	}
	comparisonHolds := effectiveTruthy(info.CondCheck.Kind, taken)
	includeLiteral := (rel.Operator == "==" && comparisonHolds) || (rel.Operator == "~=" && !comparisonHolds)

	if sym, segments, lit, ok := t.scalarLiteralComparisonPath(out, rel); ok && sym != 0 && lit != nil {
		baseAV, has := t.narrowBaseFor(out, sym, atExit)
		if !has {
			if includeLiteral && len(segments) == 0 {
				res := flow.ClonePointState(out)
				t.setNarrowedSymbol(&res, sym, product.FromType(lit))
				return res, true
			}
			return out, false
		}
		base := baseAV.ProjectValue()
		if base == nil {
			return out, false
		}
		refine := func(ft typ.Type) typ.Type {
			return narrowByLiteralEquality(ft, lit, includeLiteral)
		}
		refined := refineAtPath(base, segments, refine)
		if refined == nil || typ.TypeEquals(refined, base) {
			return out, false
		}
		res := flow.ClonePointState(out)
		if refined.Kind().IsNever() {
			t.setNarrowedSymbol(&res, sym, product.Bottom())
			return res, true
		}
		t.setNarrowedSymbol(&res, sym, product.FromType(refined))
		return res, true
	}

	place, lit, ok := t.scalarLiteralComparisonPlace(&out, rel)
	if !ok || place.Root == 0 || lit == nil {
		return out, false
	}
	baseAV, has := t.narrowBaseFor(out, place.Root, atExit)
	if !has {
		return out, false
	}
	base := baseAV.ProjectValue()
	if base == nil {
		return out, false
	}
	refined := narrowLiteralAtPlace(base, place.Steps, lit, includeLiteral)
	if refined == nil || typ.TypeEquals(refined, base) {
		return out, false
	}
	res := flow.ClonePointState(out)
	if refined.Kind().IsNever() {
		t.setNarrowedSymbol(&res, place.Root, product.Bottom())
		return res, true
	}
	t.setNarrowedSymbol(&res, place.Root, product.FromType(refined))
	return res, true
}

func (t *Transfer) scalarLiteralComparisonPlace(out *flow.PointState, rel *ast.RelationalOpExpr) (Place, *typ.Literal, bool) {
	if lit, ok := literalValue(rel.Rhs); ok {
		if p, ok := t.comparisonPlace(out, rel.Lhs); ok {
			return p, lit, true
		}
	}
	if lit, ok := literalValue(rel.Lhs); ok {
		if p, ok := t.comparisonPlace(out, rel.Rhs); ok {
			return p, lit, true
		}
	}
	return Place{}, nil, false
}

func (t *Transfer) comparisonPlace(out *flow.PointState, expr ast.Expr) (Place, bool) {
	if out == nil {
		return Place{}, false
	}
	p, ok := t.placeOfExpr(out, expr, nil)
	if !ok || p.Root == 0 || len(p.Steps) == 0 {
		return Place{}, false
	}
	if _, ok := p.StaticPath(); ok {
		return Place{}, false
	}
	return p, true
}

func (t *Transfer) scalarLiteralComparisonPath(out flow.PointState, rel *ast.RelationalOpExpr) (cfg.SymbolID, []constraint.Segment, *typ.Literal, bool) {
	if lit, ok := literalValue(rel.Rhs); ok {
		if sym, segments, ok := t.scalarComparisonAccess(&out, rel.Lhs); ok {
			return sym, segments, lit, true
		}
	}
	if lit, ok := literalValue(rel.Lhs); ok {
		if sym, segments, ok := t.scalarComparisonAccess(&out, rel.Rhs); ok {
			return sym, segments, lit, true
		}
	}
	return 0, nil, nil, false
}

func (t *Transfer) scalarComparisonAccess(out *flow.PointState, expr ast.Expr) (cfg.SymbolID, []constraint.Segment, bool) {
	sym, segments, ok := t.pathSymbolInState(out, expr, nil)
	if !ok {
		return 0, nil, false
	}
	return sym, segments, true
}

func narrowLiteralAtPlace(t typ.Type, steps []PlaceStep, lit *typ.Literal, include bool) typ.Type {
	if len(steps) == 0 {
		return narrowByLiteralEquality(t, lit, include)
	}
	step := steps[0]
	switch step.Kind {
	case PlaceStepStaticMember:
		seg, ok := place.SegmentFromMemberKey(step.Member)
		if !ok {
			return t
		}
		return narrowLiteralAtStaticSegment(t, seg, steps[1:], lit, include)
	case PlaceStepDynamicIndex:
		keyType := step.Key.ProjectValue()
		if keyType == nil {
			return t
		}
		return narrowLiteralAtDynamicIndex(t, keyType, steps[1:], lit, include)
	default:
		return t
	}
}

func narrowLiteralAtStaticSegment(t typ.Type, seg constraint.Segment, rest []PlaceStep, lit *typ.Literal, include bool) typ.Type {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return mapUnionField(t, seg.Name, func(ft typ.Type) typ.Type {
			return narrowLiteralAtPlace(ft, rest, lit, include)
		}, !include)
	case constraint.SegmentIndexInt:
		return refineAtPath(t, []constraint.Segment{seg}, func(ft typ.Type) typ.Type {
			return narrowLiteralAtPlace(ft, rest, lit, include)
		})
	default:
		return t
	}
}

func narrowLiteralAtDynamicIndex(t typ.Type, keyType typ.Type, rest []PlaceStep, lit *typ.Literal, include bool) typ.Type {
	if t == nil || keyType == nil {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(t)
		if expanded == nil || expanded == t {
			return t
		}
		return narrowLiteralAtDynamicIndex(expanded, keyType, rest, lit, include)
	case *typ.Union:
		kept := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			refined := narrowLiteralAtDynamicIndex(m, keyType, rest, lit, include)
			if refined == nil || refined.Kind().IsNever() {
				continue
			}
			kept = append(kept, refined)
		}
		if len(kept) == 0 {
			return typ.Never
		}
		return typ.NewUnion(kept...)
	case *typ.Optional:
		refined := narrowLiteralAtDynamicIndex(v.Inner, keyType, rest, lit, include)
		if refined == nil || refined.Kind().IsNever() {
			if include {
				return typ.Never
			}
			return t
		}
		return refined
	default:
		slot, ok := querycore.RuntimeIndex(t, keyType)
		if !ok || slot == nil {
			if include {
				return typ.Never
			}
			return t
		}
		refined := narrowLiteralAtPlace(slot, rest, lit, include)
		if refined == nil || refined.Kind().IsNever() {
			return typ.Never
		}
		// A non-literal dynamic key cannot be written back to one stable member
		// path. The refinement is therefore a root-union filter only; exact
		// literal dynamic keys are normalized through Place.StaticPath earlier.
		return t
	}
}

func narrowByLiteralEquality(t typ.Type, lit *typ.Literal, include bool) typ.Type {
	if lit == nil {
		return t
	}
	if include {
		if t == nil || !narrow.TypesOverlap(t, lit) {
			return typ.Never
		}
		return lit
	}
	return excludeExactLiteral(t, lit)
}

func excludeExactLiteral(t typ.Type, lit *typ.Literal) typ.Type {
	if t == nil || lit == nil {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Alias:
		inner := excludeExactLiteral(v.Target, lit)
		if inner == nil || inner.Kind().IsNever() {
			return inner
		}
		if typ.TypeEquals(inner, v.Target) {
			return t
		}
		return typ.NewAlias(v.Name, inner)
	case *typ.Instantiated:
		if expanded := subst.ExpandInstantiated(v); expanded != nil && expanded != v {
			return excludeExactLiteral(expanded, lit)
		}
		return t
	case *typ.Optional:
		inner := excludeExactLiteral(v.Inner, lit)
		if inner == nil || inner.Kind().IsNever() {
			return typ.Nil
		}
		if typ.TypeEquals(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		kept := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			next := excludeExactLiteral(member, lit)
			if next == nil || next.Kind().IsNever() {
				changed = true
				continue
			}
			if !typ.TypeEquals(next, member) {
				changed = true
			}
			kept = append(kept, next)
		}
		if !changed {
			return t
		}
		if len(kept) == 0 {
			return typ.Never
		}
		return typ.NewUnion(kept...)
	case *typ.Literal:
		if typ.LiteralEquals(v, lit) {
			return typ.Never
		}
		return t
	default:
		return t
	}
}

// narrowLengthGuard seeds the container length bound a `#container OP const` guard
// proves on the edge it holds. It orients the comparison so the `#container` side is
// the value and the integer constant the bound, applies the comparison's
// proven-edge operator, and translates it into a length floor / ceiling on the
// container's numeric key:
//
//   - `#x >  c`  proves  #x >= c+1  (floor c+1)
//   - `#x >= c`  proves  #x >= c    (floor c)
//   - `#x <  c`  proves  #x <= c-1  (ceiling c-1)
//   - `#x <= c`  proves  #x <= c    (ceiling c)
//   - `#x == c`  proves  #x >= c AND #x <= c
//   - `#x ~= 0`  proves  #x >= 1    (a sequence length is non-negative, so != 0 is >= 1)
//
// A `~= c` for c other than 0 proves nothing soundly (the length may sit above or
// below c), so it is declined. A floor of c >= 1 on the true edge lets a later
// in-range index read `x[1]` / `x[#x]` in the guarded block drop its soundly-optional
// element through the same length proof a bounded loop seeds. The numeric component is
// cloned before the bound is applied so the shared predecessor state is never mutated;
// the merge-LUB rebuilds the unbounded length past the guard. A comparison neither side
// of which is `#container` over a tracked sequence, or whose other side is not an integer
// constant, reports applied=false so the index-variable orientation runs.
func (t *Transfer) narrowLengthGuard(out flow.PointState, rel *ast.RelationalOpExpr, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	switch rel.Operator {
	case "<", "<=", ">", ">=", "==", "~=":
	default:
		return out, false
	}
	place, arrKey, c, op, ok := t.orientLengthComparison(rel.Lhs, rel.Rhs, rel.Operator)
	if !ok {
		return out, false
	}
	// The CFG records the comparison branch as a truthy/falsy check; the taken edge
	// holds the comparison as written, the not-taken edge its logical negation.
	if !effectiveTruthy(info.CondCheck.Kind, taken) {
		op = negateLengthOp(op)
	}
	floor, ceil, hasFloor, hasCeil := lengthBoundFromOp(op, c)
	if !hasFloor && !hasCeil {
		return out, false
	}
	ops := make([]NumericOp, 0, 2)
	if hasFloor {
		ops = append(ops, NumericOp{Kind: NumericLenGeConst, Key: arrKey, Const: floor})
	}
	if hasCeil {
		ops = append(ops, NumericOp{Kind: NumericLenLeConst, Key: arrKey, Const: ceil})
	}
	res := flow.ClonePointState(out)
	t.applyNumericEffect(&res, NumericEffect{Ops: ops})
	if hasFloor && floor > 0 {
		t.applyRefinementEffect(&res, RefinementEffect{
			Place:     place,
			Kind:      RefinementLengthLowerBound,
			LengthMin: floor,
			PreferEnv: true,
		})
	}
	return res, true
}

// orientLengthComparison resolves a `#container OP const` comparison into the
// container's numeric key, the integer constant, and the operator oriented so it
// reads `#container OP const`. The `#container` side may be either operand; when it
// is on the right the operator is flipped. A comparison neither side of which is
// `#container` over a tracked sequence, or whose other side is not an integer
// constant, reports ok=false.
func (t *Transfer) orientLengthComparison(lhs, rhs ast.Expr, op string) (Place, constraint.PathKey, int64, string, bool) {
	if place, arrKey, ok := t.lenExprContainerPlace(lhs); ok {
		if c, ok := t.constInt(rhs); ok {
			return place, arrKey, c, op, true
		}
	}
	if place, arrKey, ok := t.lenExprContainerPlace(rhs); ok {
		if c, ok := t.constInt(lhs); ok {
			return place, arrKey, c, flipComparisonOp(op), true
		}
	}
	return Place{}, "", 0, op, false
}

// negateLengthOp returns the logical negation of a relational operator over the
// integer length, including the equality operators the index-variable negation does
// not handle (not (a == b) is a ~= b).
func negateLengthOp(op string) string {
	switch op {
	case "==":
		return "~="
	case "~=":
		return "=="
	default:
		return negateComparisonOp(op)
	}
}

// lengthBoundFromOp translates a proven `#x OP c` comparison into the inclusive
// integer length floor and/or ceiling it establishes. A strict bound is tightened to
// its integer neighbor (`#x > c` is `#x >= c+1`). An equality bounds both ends; an
// inequality bounds the length only when c is 0 (a non-negative length that is not 0
// is at least 1). An operator/constant that proves no usable bound reports both
// has-flags false.
func lengthBoundFromOp(op string, c int64) (floor, ceil int64, hasFloor, hasCeil bool) {
	switch op {
	case ">":
		return c + 1, 0, true, false
	case ">=":
		return c, 0, true, false
	case "<":
		return 0, c - 1, false, true
	case "<=":
		return 0, c, false, true
	case "==":
		return c, c, true, true
	case "~=":
		if c == 0 {
			return 1, 0, true, false
		}
		return 0, 0, false, false
	default:
		return 0, 0, false, false
	}
}

// orientComparison resolves which operand of a relational comparison is the
// candidate index variable and which is the bound, returning them in `var OP bound`
// orientation with the operator flipped when the variable is on the right. It
// prefers an identifier operand as the variable; when both or neither are
// identifiers it picks the left as the variable. A comparison with no usable
// operand reports ok=false.
func (t *Transfer) orientComparison(lhs, rhs ast.Expr, op string) (ast.Expr, ast.Expr, string, bool) {
	_, lIdent := lhs.(*ast.IdentExpr)
	_, rIdent := rhs.(*ast.IdentExpr)
	switch {
	case lIdent:
		return lhs, rhs, op, true
	case rIdent:
		return rhs, lhs, flipComparisonOp(op), true
	default:
		return nil, nil, op, false
	}
}

// flipComparisonOp swaps the sides of a relational operator (a < b  <=>  b > a).
func flipComparisonOp(op string) string {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	}
	return op
}

// negateComparisonOp returns the logical negation of a relational operator over
// integers: not (a < b) is a >= b, etc.
func negateComparisonOp(op string) string {
	switch op {
	case "<":
		return ">="
	case "<=":
		return ">"
	case ">":
		return "<="
	case ">=":
		return "<"
	}
	return op
}

// effectiveTruthy reports whether the branch condition holds on the chosen edge: a
// CheckTruthy branch holds on the taken edge, a CheckFalsy on the not-taken. A
// comparison the CFG could not classify as a presence/type guard falls through to
// one of these bare truthiness checks.
func effectiveTruthy(check cfg.CondCheckKind, taken bool) bool {
	switch check {
	case cfg.CheckFalsy:
		return !taken
	default:
		return taken
	}
}

// lengthBoundComparison recognizes a `var <= #container` / `var < #container`
// upper bound, returning the container's numeric key and the inclusive integer
// offset (a strict `<` is `<= #container - 1`). Only the upper-bound senses (`<`,
// `<=`) bound the index by the container length; a lower-bound sense proves no
// in-range presence and reports ok=false.
func (t *Transfer) lengthBoundComparison(boundExpr ast.Expr, op string) (constraint.PathKey, int64, bool) {
	arrKey, ok := t.lenExprContainerKey(boundExpr)
	if !ok {
		return "", 0, false
	}
	switch op {
	case "<=":
		return arrKey, 0, true
	case "<":
		return arrKey, -1, true
	default:
		return "", 0, false
	}
}

// condTestSegments resolves the field segments of the path the guard tests. A bare
// symbol test (if x, x ~= nil) has none; a field-path test (if x.f) has the field
// chain under the base symbol. The segments are derived from the condition AST when
// the branch carries it, else from the CondVar path string (the form a ScopeExit
// guard preserves after the branch's condition AST is dropped).
func (t *Transfer) condTestSegments(info *cfg.BranchInfo) []constraint.Segment {
	if seg := t.condTestSegmentsFromAST(info); seg != nil {
		return seg
	}
	root := extraction.ExtractRootName(info.CondVar)
	if root == "" || root == info.CondVar {
		return nil
	}
	return pathkey.ParseSuffix(info.CondVar[len(root):])
}

// condTestSegmentsFromAST extracts the tested field path's segments directly from
// the branch condition AST: the field chain of the truthy/falsy operand or of a
// type(path) == k / path == nil comparand. Returns nil when the test is on a bare
// symbol or an unrecognized shape (the CondVar string fallback then applies).
func (t *Transfer) condTestSegmentsFromAST(info *cfg.BranchInfo) []constraint.Segment {
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		return staticAccessSegments(info.Condition)
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return staticAccessSegments(not.Expr)
		}
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			if seg := staticAccessSegments(rel.Lhs); seg != nil {
				return seg
			}
			return staticAccessSegments(rel.Rhs)
		}
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
		return t.typeofArgSegments(info.Condition)
	}
	return nil
}

// staticAccessSegments returns the segments of a static access expression. Bare
// symbols report nil so the caller narrows the symbol value itself.
func staticAccessSegments(expr ast.Expr) []constraint.Segment {
	segs, ok := staticSegmentsOfExpr(expr)
	if !ok || len(segs) == 0 {
		return nil
	}
	return segs
}

// typeofArgSegments returns the field segments of the value tested by a
// type(path) == k guard when that argument is a field path under an identifier
// root. A bare-identifier argument yields nil (the symbol value is narrowed).
func (t *Transfer) typeofArgSegments(expr ast.Expr) []constraint.Segment {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return nil
	}
	for _, side := range []ast.Expr{rel.Lhs, rel.Rhs} {
		call, ok := side.(*ast.FuncCallExpr)
		if !ok || call.Method != "" || call.Receiver != nil || len(call.Args) != 1 {
			continue
		}
		fn, ok := call.Func.(*ast.IdentExpr)
		if !ok || fn.Value != "type" {
			continue
		}
		return staticAccessSegments(call.Args[0])
	}
	return nil
}

// narrowAtPath narrows the value the guard tests inside av. With no segments it
// narrows the bare symbol value directly (the union/scalar refinement). With field
// segments it narrows the field path inside av's structural type: a union is
// filtered to the members whose field path survives the check, and each surviving
// member's field slot is narrowed to the check's refinement (string? -> string for
// a present/truthy guard), reusing the value-domain field narrowers.
//
// When no member's field path survives, the field narrowing yields Never: the edge
// is impossible for this value (a discriminant guard on a value already pinned to
// the other variant), so the base symbol narrows to Never and the edge's reads are
// unreachable. The merge-LUB recovers the live value where both edges meet, so the
// Never never survives past the guard. It returns ok=false (a precision loss, never
// unsoundness) only when no refinement applies — an index segment, an unresolvable
// field, or an unchanged base.
func narrowAtPath(av product.AbstractValue, segments []constraint.Segment, check cfg.CondCheckKind, typeName string) (product.AbstractValue, bool) {
	if len(segments) == 0 {
		return narrowValue(av, check, typeName)
	}
	base := av.ProjectValue()
	if base == nil {
		return product.AbstractValue{}, false
	}
	refined := narrowFieldPath(base, segments, check, typeName)
	if refined == nil || refined == base {
		return product.AbstractValue{}, false
	}
	if refined.Kind().IsNever() {
		// An impossible edge: the base narrows to the lattice Bottom (the empty
		// value), so the edge's reads are unreachable. product.Bottom is the sound
		// Never carrier the env join already handles; FromType(Never) would synthesize
		// a shape the join cannot project.
		return product.Bottom(), true
	}
	return product.FromType(refined), true
}

// narrowFieldPath narrows the field path (segments) inside t by the check. It
// descends a union per member, keeping only members whose field path survives, and
// rebuilds the leaf field slot with its refined type. Returns Never when no member
// survives (an impossible edge the caller declines) and t unchanged when the check
// does not refine the field. Only static field segments are followed; an index
// segment yields t unchanged.
func narrowFieldPath(t typ.Type, segments []constraint.Segment, check cfg.CondCheckKind, typeName string) typ.Type {
	if t == nil || len(segments) == 0 {
		return t
	}
	seg := segments[0]
	if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
		return t
	}
	if len(segments) == 1 {
		refine, absentKeeps, ok := fieldRefiner(check, typeName)
		if !ok {
			return t
		}
		return mapUnionField(t, seg.Name, refine, absentKeeps)
	}
	// A nested field path narrows the inner field path within the leaf record of
	// the outer field; rebuild the outer field with the recursively narrowed inner.
	// A member whose leaf field path collapses is dropped (absentKeeps=false: a
	// member that cannot reach the guarded inner path does not survive a positive
	// guard); the merge-LUB recovers the value where both edges meet.
	return mapUnionField(t, seg.Name, func(ft typ.Type) typ.Type {
		return narrowFieldPath(ft, segments[1:], check, typeName)
	}, false)
}

// fieldRefiner maps a branch check to the per-member field refinement it imposes
// and whether a member that LACKS the field survives the guard. A positive guard
// (truthy/present/typeof-equal) drops a member without the field (it cannot satisfy
// the guard); a negative guard (falsy/nil/typeof-not) keeps it (an absent field
// reads nil, which is falsy / not the excluded kind). It reuses the value-domain
// scalar narrowers on the field's own type. ok=false leaves the base unchanged.
func fieldRefiner(check cfg.CondCheckKind, typeName string) (refine func(typ.Type) typ.Type, absentKeeps bool, ok bool) {
	switch check {
	case cfg.CheckTruthy:
		return narrow.ToTruthy, false, true
	case cfg.CheckFalsy:
		return narrow.ToFalsy, true, true
	case cfg.CheckNotNil:
		return narrow.RemoveNil, false, true
	case cfg.CheckNil:
		return func(typ.Type) typ.Type { return typ.Nil }, true, true
	case cfg.CheckTypeEqual:
		key, known := narrow.KnownBuiltinTypeKey(typeName)
		if !known {
			return nil, false, false
		}
		return func(ft typ.Type) typ.Type { return narrow.ByTypeKey(ft, key, nil) }, false, true
	case cfg.CheckTypeNot:
		key, known := narrow.KnownBuiltinTypeKey(typeName)
		if !known {
			return nil, false, false
		}
		return func(ft typ.Type) typ.Type { return narrow.ExcludeByTypeKey(ft, key, nil) }, true, true
	default:
		return nil, false, false
	}
}

// mapUnionField rebuilds t with the field slot of each record member replaced by
// refine(fieldType). It descends a union per member, unwrapping each member's alias
// so an aliased record (the common shape of a named-type union member) is narrowed
// as its record, and drops a member whose refined field is Never. A member that
// LACKS the field is dropped when absentKeeps is false (a positive guard the absent
// field cannot satisfy) or kept unchanged when true (a negative guard the absent
// field trivially satisfies). An all-dropped union becomes Never (an impossible
// edge). A non-record member with no resolvable field is left unchanged.
func mapUnionField(t typ.Type, field string, refine func(typ.Type) typ.Type, absentKeeps bool) typ.Type {
	if t == nil {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Instantiated:
		// A generic instantiation (Result<Greeting> = {ok: true, value: T} |
		// {ok: false, error: string}) carries its discriminated union behind the
		// instantiation; unwrap.Alias does not expand it, so the per-member field
		// filter never sees the variants. Expand the instantiation to its
		// substituted body and narrow that, so a `if r.ok` guard drops the false
		// variant exactly as it would for a non-generic union. A body that does not
		// expand (no resolvable generic) leaves t unchanged.
		expanded := subst.ExpandInstantiated(t)
		if expanded == nil || expanded == t {
			return t
		}
		return mapUnionField(expanded, field, refine, absentKeeps)
	case *typ.Union:
		kept := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			refined := mapUnionField(m, field, refine, absentKeeps)
			if refined == nil || refined.Kind().IsNever() {
				continue
			}
			kept = append(kept, refined)
		}
		if len(kept) == 0 {
			return typ.Never
		}
		return typ.NewUnion(kept...)
	case *typ.Optional:
		return mapUnionField(v.Inner, field, refine, absentKeeps)
	case *typ.Intersection:
		// An intersection is a conjunction of records (PageInfo & {data_func: ...}).
		// The narrowed field lives in whichever conjunct declares it; that conjunct is
		// rebuilt with the refined field while the others are kept unchanged (they do
		// not constrain the field). A conjunct whose field refines to Never makes the
		// whole intersection impossible (Never); a field no conjunct declares falls to
		// the absentKeeps decision, like a record without the field.
		found := false
		members := make([]typ.Type, len(v.Members))
		for i, m := range v.Members {
			if _, ok := fieldResolver.Field(m, field); !ok {
				members[i] = m
				continue
			}
			found = true
			refined := mapUnionField(m, field, refine, absentKeeps)
			if refined == nil || refined.Kind().IsNever() {
				return typ.Never
			}
			members[i] = refined
		}
		if !found {
			if absentKeeps {
				return t
			}
			return typ.Never
		}
		return typ.NewIntersection(members...)
	case *typ.Record:
		ft, ok := fieldResolver.Field(v, field)
		if !ok || ft == nil {
			if absentKeeps {
				return t
			}
			return typ.Never
		}
		refined := refine(ft)
		if refined == nil || refined.Kind().IsNever() {
			return typ.Never
		}
		return typ.ExtendRecordWithField(v, field, refined)
	case *typ.Map:
		// A map base (`{[K]: V}`, or the `{[any]: any}` a `type(x) == "table"` guard
		// produces from `any`) carries no static field, but a guard on a single key
		// (`if m["root"]`, `type(x.f) == "number"`) proves THAT key's value is the
		// refined type on the positive edge. Overlay a record that pins the proven key
		// while PRESERVING the map component, so the proven key reads its non-optional
		// refined type (field precedence) and every other key stays its soundly-optional
		// map value (the map fallback): narrowing one key never asserts another present.
		// The merge-LUB at the post-guard join rebuilds the bare map where both edges
		// meet, so the per-key refinement never survives past its guard.
		refined := refine(v.Value)
		if refined == nil {
			return t
		}
		if refined.Kind().IsNever() {
			if absentKeeps {
				return t
			}
			return typ.Never
		}
		return typ.NewRecord().SetOpen(true).Field(field, refined).MapComponent(v.Key, v.Value).Build()
	default:
		// A gradual `any` base resolves EVERY field to `any` (fieldResolver.Field(any, f)
		// succeeds with `any`), so a positive guard on a field (`type(x.f) == k`, `if x.f`)
		// proves only that f's refined type holds on this edge while the rest of the value
		// stays gradual. Overlay an OPEN record that pins the proven field over an `any`
		// fallback: a read of x.f observes the refinement and any other field read stays
		// `any` (gradual preserved, never over-narrowed). This must run before the resolver
		// fall-through below, which — seeing `any` resolve the field — would otherwise
		// return the base unchanged and drop the guard's refinement.
		if typ.IsAny(t) {
			refined := refine(typ.Any)
			if refined == nil {
				return t
			}
			if refined.Kind().IsNever() {
				if absentKeeps {
					return t
				}
				return typ.Never
			}
			// A COMPLETE scalar proof (`type(x.f) == "string"|"number"|"boolean"`,
			// which filters the gradual `any` down to a primitive) pins that gradual
			// field to a concrete, assignable type. For a nested proof such as
			// `type(x.f.g) == "number"`, the recursive call returns an open dynamic
			// record overlay for `f` with only the proven nested field pinned; this is
			// also admissible because it is still a gradual table for every other key.
			//
			// A truthy/presence guard leaves `any`, and a direct `type(x.f) == "table"`
			// proof yields the broad `{[any]: any}` map, which proves only table-ness
			// with gradual elements. Neither establishes a scalar slot a typed boundary
			// can consume, so leave the gradual base unchanged in those cases.
			if !gradualAnyFieldProofAdmissible(refined) {
				return t
			}
			return typ.NewRecord().SetOpen(true).Field(field, refined).MapComponent(typ.Any, typ.Any).Build()
		}
		ft, ok := fieldResolver.Field(t, field)
		if !ok || ft == nil {
			return t
		}
		refined := refine(ft)
		if refined == nil || refined.Kind().IsNever() {
			return typ.Never
		}
		return t
	}
}

func gradualAnyFieldProofAdmissible(refined typ.Type) bool {
	if refined == nil {
		return false
	}
	if refined.Kind().IsPrimitive() {
		return true
	}
	rec, ok := unwrap.Alias(refined).(*typ.Record)
	return ok && rec != nil
}

// condTestSymbol resolves the symbol the branch guard tests. The CFG resolves
// CondSymbol for a bare identifier and a nil comparison, but leaves it 0 for a
// type(x) == k guard (its root identifier is the typeof-call argument, which the
// CFG's root scan does not descend into). For that case the symbol is recovered
// from the condition expression directly, so a typeof guard narrows the tested
// value.
func (t *Transfer) condTestSymbol(info *cfg.BranchInfo) cfg.SymbolID {
	if info.CondSymbol != 0 {
		return info.CondSymbol
	}
	if info.CondCheck.Kind == cfg.CheckTypeEqual || info.CondCheck.Kind == cfg.CheckTypeNot {
		return t.typeofArgSymbol(info.Condition)
	}
	return 0
}

// typeofArgSymbol extracts the base symbol of the value tested by a type(x) == k
// guard: the root identifier of the typeof call's single argument, whether that
// argument is a bare identifier (type(x)) or a field path (type(x.f.g), whose root
// identifier x binds the symbol and whose field segments condTestSegments supplies).
// A type-argument not rooted at an identifier yields 0.
func (t *Transfer) typeofArgSymbol(expr ast.Expr) cfg.SymbolID {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return 0
	}
	for _, side := range []ast.Expr{rel.Lhs, rel.Rhs} {
		call, ok := side.(*ast.FuncCallExpr)
		if !ok || call.Method != "" || call.Receiver != nil || len(call.Args) != 1 {
			continue
		}
		fn, ok := call.Func.(*ast.IdentExpr)
		if !ok || fn.Value != "type" {
			continue
		}
		if sym, _, ok := t.pathSymbol(call.Args[0]); ok {
			return sym
		}
	}
	return 0
}

// effectiveCheck resolves the check that holds on the chosen edge: the guard
// itself on the true edge, its negation on the false edge. CheckLimit (the
// numeric-for loop bound) carries no value narrowing.
func effectiveCheck(k cfg.CondCheckKind, taken bool) cfg.CondCheckKind {
	if taken {
		switch k {
		case cfg.CheckTruthy, cfg.CheckFalsy, cfg.CheckNil, cfg.CheckNotNil, cfg.CheckTypeEqual, cfg.CheckTypeNot:
			return k
		default:
			return cfg.CheckNone
		}
	}
	switch k {
	case cfg.CheckTruthy:
		return cfg.CheckFalsy
	case cfg.CheckFalsy:
		return cfg.CheckTruthy
	case cfg.CheckNil:
		return cfg.CheckNotNil
	case cfg.CheckNotNil:
		return cfg.CheckNil
	case cfg.CheckTypeEqual:
		return cfg.CheckTypeNot
	case cfg.CheckTypeNot:
		return cfg.CheckTypeEqual
	default:
		return cfg.CheckNone
	}
}

// narrowValue refines av by the resolved check, reusing the value-domain narrowing
// primitives. typeName is the Lua typeof name for the type checks.
func narrowValue(av product.AbstractValue, check cfg.CondCheckKind, typeName string) (product.AbstractValue, bool) {
	switch check {
	case cfg.CheckTruthy:
		return product.NarrowTruthy(av), true
	case cfg.CheckFalsy:
		return product.NarrowFalsy(av), true
	case cfg.CheckNotNil:
		return product.NarrowPresent(av), true
	case cfg.CheckNil:
		// A value the guard proves is nil narrows to nil exactly.
		return product.FromType(typ.Nil), true
	case cfg.CheckTypeEqual:
		k := kind.FromString(typeName)
		if k == kind.Unknown {
			return product.AbstractValue{}, false
		}
		return product.FilterByKind(av, k), true
	case cfg.CheckTypeNot:
		k := kind.FromString(typeName)
		if k == kind.Unknown {
			return product.AbstractValue{}, false
		}
		return product.ExcludeByKind(av, k), true
	default:
		return product.AbstractValue{}, false
	}
}

// conditionRefinedCaptureValue snapshots a captured symbol under the current
// point condition. Branch transfer may carry some precision in Cond rather than
// in the raw symbol slot; closure creation must observe the same proof ordinary
// reads observe, or a closure allocated inside `type(x) == "number"` enters with
// `x: unknown`. For a DNF condition, refine per disjunct and join the results so
// the snapshot remains a sound over-approximation of the reachable states.
func (t *Transfer) conditionRefinedCaptureValue(out *flow.PointState, sym cfg.SymbolID, base product.AbstractValue, hasBase bool) (product.AbstractValue, bool) {
	if out == nil || sym == 0 || !out.Cond.HasConstraints() {
		return product.AbstractValue{}, false
	}
	if !hasBase || base.IsZero() {
		base = product.Top()
	}
	var joined product.AbstractValue
	hadDisjunct := false
	for i := 0; i < out.Cond.NumDisjuncts(); i++ {
		cur := base
		touched := false
		for _, c := range out.Cond.DisjunctConstraints(i) {
			next, ok := conditionConstraintNarrowValue(cur, sym, c)
			if !ok {
				continue
			}
			cur = next
			touched = true
		}
		if !touched && !hasBase {
			cur = product.Top()
		}
		if !hadDisjunct {
			joined = cur
			hadDisjunct = true
			continue
		}
		joined = product.Join(joined, cur)
	}
	if !hadDisjunct {
		return product.AbstractValue{}, false
	}
	return joined, true
}

func conditionConstraintNarrowValue(av product.AbstractValue, sym cfg.SymbolID, c constraint.Constraint) (product.AbstractValue, bool) {
	switch cc := c.(type) {
	case constraint.Truthy:
		return conditionPathNarrowValue(av, sym, cc.Path, cfg.CheckTruthy, "")
	case constraint.Falsy:
		return conditionPathNarrowValue(av, sym, cc.Path, cfg.CheckFalsy, "")
	case constraint.IsNil:
		return conditionPathNarrowValue(av, sym, cc.Path, cfg.CheckNil, "")
	case constraint.NotNil:
		return conditionPathNarrowValue(av, sym, cc.Path, cfg.CheckNotNil, "")
	case constraint.HasType:
		return conditionPathNarrowTypeKey(av, sym, cc.Path, cc.Type, false)
	case constraint.NotHasType:
		return conditionPathNarrowTypeKey(av, sym, cc.Path, cc.Type, true)
	case constraint.FieldEquals:
		return conditionFieldLiteralValue(av, sym, cc.Target, cc.Field, cc.Value, false)
	case constraint.FieldNotEquals:
		return conditionFieldLiteralValue(av, sym, cc.Target, cc.Field, cc.Value, true)
	case constraint.VariantCaseEquals:
		return conditionVariantOriginCaseValue(av, sym, cc.Target, cc.OriginFamily, cc.CaseIndex, true)
	case constraint.VariantCaseNotEquals:
		return conditionVariantOriginCaseValue(av, sym, cc.Target, cc.OriginFamily, cc.CaseIndex, false)
	default:
		return product.AbstractValue{}, false
	}
}

func conditionVariantOriginCaseValue(av product.AbstractValue, sym cfg.SymbolID, path constraint.Path, family uint64, caseIndex int, equal bool) (product.AbstractValue, bool) {
	if path.Symbol != sym || len(path.Segments) != 0 {
		return product.AbstractValue{}, false
	}
	return product.NarrowVariantOriginCase(av, family, caseIndex, equal)
}

func conditionPathNarrowValue(av product.AbstractValue, sym cfg.SymbolID, path constraint.Path, check cfg.CondCheckKind, typeName string) (product.AbstractValue, bool) {
	if path.Symbol != sym {
		return product.AbstractValue{}, false
	}
	return narrowAtPath(av, path.Segments, check, typeName)
}

func conditionPathNarrowTypeKey(av product.AbstractValue, sym cfg.SymbolID, path constraint.Path, key narrow.TypeKey, exclude bool) (product.AbstractValue, bool) {
	if path.Symbol != sym || key.IsZero() {
		return product.AbstractValue{}, false
	}
	if k, ok := key.BuiltinKind(); ok {
		check := cfg.CheckTypeEqual
		if exclude {
			check = cfg.CheckTypeNot
		}
		return narrowAtPath(av, path.Segments, check, k.String())
	}
	if len(path.Segments) != 0 {
		return product.AbstractValue{}, false
	}
	base := product.ProjectValueOrUnknown(av)
	if base == nil {
		return product.AbstractValue{}, false
	}
	var refined typ.Type
	if exclude {
		refined = narrow.ExcludeByTypeKey(base, key, nil)
	} else {
		refined = narrow.ByTypeKey(base, key, nil)
	}
	if refined == nil {
		return product.AbstractValue{}, false
	}
	return product.FromType(refined), true
}

func conditionFieldLiteralValue(av product.AbstractValue, sym cfg.SymbolID, target constraint.Path, field string, lit *typ.Literal, exclude bool) (product.AbstractValue, bool) {
	if target.Symbol != sym || len(target.Segments) != 0 || field == "" || lit == nil {
		return product.AbstractValue{}, false
	}
	base := product.ProjectValueOrUnknown(av)
	if base == nil {
		return product.AbstractValue{}, false
	}
	var refined typ.Type
	if exclude {
		refined = narrow.ExcludeByFieldLiteral(base, field, lit, fieldResolver)
	} else {
		refined = narrow.ByFieldLiteral(base, field, lit, fieldResolver)
	}
	if refined == nil {
		return product.AbstractValue{}, false
	}
	return product.FromType(refined), true
}

// condForCheck builds the per-edge path condition for the resolved check on the
// tested path. It is the canonical Cond half of the narrowing: the constraint that
// holds on this edge, joined into the edge's PointState.Cond.
func condForCheck(sym cfg.SymbolID, segments []constraint.Segment, varPath string, check cfg.CondCheckKind, typeName string) constraint.Condition {
	path := constraint.Path{Root: extraction.ExtractRootName(varPath), Symbol: sym, Segments: segments}
	switch check {
	case cfg.CheckTruthy:
		return constraint.FromConstraints(constraint.Truthy{Path: path})
	case cfg.CheckFalsy:
		return constraint.FromConstraints(constraint.Falsy{Path: path})
	case cfg.CheckNil:
		return constraint.FromConstraints(constraint.IsNil{Path: path})
	case cfg.CheckNotNil:
		return constraint.FromConstraints(constraint.NotNil{Path: path})
	case cfg.CheckTypeEqual:
		if key := typeKeyFor(typeName); !key.IsZero() {
			return constraint.FromConstraints(constraint.HasType{Path: path, Type: key})
		}
	case cfg.CheckTypeNot:
		if key := typeKeyFor(typeName); !key.IsZero() {
			return constraint.FromConstraints(constraint.NotHasType{Path: path, Type: key})
		}
	}
	return constraint.TrueCondition()
}

// typeKeyFor maps a Lua typeof name to the narrow.TypeKey the condition's HasType
// constraint carries. An unrecognized name yields the zero key.
func typeKeyFor(typeName string) narrow.TypeKey {
	key, ok := narrow.KnownBuiltinTypeKey(typeName)
	if !ok {
		return narrow.TypeKey{}
	}
	return key
}

// narrowByDiscriminant applies a discriminated-union narrowing for a guard of the
// shape base.field == "tag" (or base.field ~= "tag"). It detects the discriminant
// equality directly from the branch's condition expression (the CFG records such a
// relational guard as a plain truthy check, so the literal is recovered from the
// AST), then narrows the base value's union to the variant whose field matches the
// literal (the TRUE edge) or excludes that variant (the FALSE edge), reusing
// narrow.ByFieldLiteral / ExcludeByFieldLiteral. It reports whether a discriminant
// guard was recognized; a non-discriminant condition is left to the CondCheck path.
func (t *Transfer) narrowByDiscriminant(out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	g, ok := t.discriminantGuard(info.Condition)
	if !ok {
		return out, false
	}
	if len(g.prefix) > 0 {
		return t.narrowByMemberDiscriminant(out, g, taken)
	}
	av, _ := t.symbolValue(&out, g.sym)
	baseAV, has := t.discriminantBase(g.sym, av)
	if !has {
		return out, false
	}
	base := baseAV.ProjectValue()
	if base == nil {
		return out, false
	}
	// On the false edge the equality is negated: == becomes the exclusion and
	// ~= becomes the inclusion.
	include := taken != g.negated
	refined, ok := narrowDiscriminantUnion(base, g.field, g.literal, include)
	if !ok {
		// An unchanged base carries no refinement; leave it to the plain join.
		return out, false
	}
	res := flow.ClonePointState(out)
	if refined.Kind().IsNever() {
		// An impossible edge (the discriminant pins the value to the other variant):
		// the base narrows to the lattice Bottom so the edge's reads are unreachable,
		// and the merge-LUB recovers the live value where both edges meet.
		t.setNarrowedSymbol(&res, g.sym, product.Bottom())
	} else {
		t.setNarrowedSymbol(&res, g.sym, product.FromType(refined))
	}
	return res, true
}

// narrowByMemberDiscriminant narrows a member-access discriminant `root[prefix].field
// == literal` (`if receipt.output.kind == "rendered"`). The union the discriminant
// partitions lives at the path g.prefix inside the root symbol's record value
// (receipt.output), so the refinement is applied to the value at that path and the
// narrowed value is written back into the root record, leaving the rest of the record
// untouched. A read of root[prefix] in the guarded body then observes the narrowed
// variant exactly as a bare-symbol discriminant narrows a directly-tracked union. The
// root record value is required from the live Env (no declared-singleton recovery is
// needed: the field's type comes from the record, which is already the declared union
// member). A root the flow does not track, a non-record root, or a path that does not
// resolve to a discriminable union leaves the state unchanged (a precision loss).
func (t *Transfer) narrowByMemberDiscriminant(out flow.PointState, g discriminant, taken bool) (flow.PointState, bool) {
	av, _ := t.symbolValue(&out, g.sym)
	baseAV, has := t.discriminantPathBase(g.sym, av, g.prefix)
	if !has {
		return out, false
	}
	root := baseAV.ProjectValue()
	if root == nil {
		return out, false
	}
	union, ok := readFieldPath(root, g.prefix)
	if !ok || union == nil {
		return out, false
	}
	include := taken != g.negated
	refined, ok := narrowDiscriminantUnion(union, g.field, g.literal, include)
	if !ok {
		return out, false
	}
	rewritten := refineAtPath(root, g.prefix, func(typ.Type) typ.Type { return refined })
	if rewritten == nil || rewritten == root {
		return out, false
	}
	res := flow.ClonePointState(out)
	if rewritten.Kind().IsNever() {
		t.setNarrowedSymbol(&res, g.sym, product.Bottom())
	} else {
		t.setNarrowedSymbol(&res, g.sym, product.FromType(rewritten))
	}
	return res, true
}

// discriminantPathBase is the member-path counterpart to discriminantBase. A
// guard such as `r.payload.kind == "a"` partitions the value at `r.payload`, not
// necessarily the root value directly. If the live root already carries a
// multi-member union at that path, narrow it to preserve sequential composition.
// Otherwise recover the declared root, so an annotated constructor singleton
// (`local r: A|B = {payload={kind="a"}}`) can still take the else edge to B.
func (t *Transfer) discriminantPathBase(sym cfg.SymbolID, av product.AbstractValue, prefix []constraint.Segment) (product.AbstractValue, bool) {
	if len(prefix) == 0 {
		return t.discriminantBase(sym, av)
	}
	if !av.IsZero() {
		if pathHasMultiUnion(av.ProjectValue(), prefix) {
			return av, true
		}
	}
	if declared, ok := t.declaredTypes[sym]; ok && declared != nil && !typ.IsAbsentOrUnknown(declared) {
		if pathHasMultiUnion(declared, prefix) {
			return product.FromType(declared), true
		}
	}
	if !av.IsZero() {
		return av, true
	}
	return t.narrowBase(sym, av, false)
}

func pathHasMultiUnion(root typ.Type, prefix []constraint.Segment) bool {
	target, ok := readFieldPath(root, prefix)
	if !ok || target == nil {
		return false
	}
	u := unwrap.Union(target)
	return u != nil && len(u.Members) > 1
}

// narrowDiscriminantUnion refines a discriminated union by a literal on its tag field:
// it keeps the matching variant on the include edge (narrow.ByFieldLiteral) or excludes
// it on the exclude edge (narrow.ExcludeByFieldLiteral). A genuine literal-discriminant
// guard reads base.field, which presupposes base is non-nil on BOTH edges (nil.field
// errors at runtime before the comparison), so an optional/nil wrapper an array-index or
// captured-optional source carries is stripped before refinement, leaving only the live
// record variants the exclude edge must also drop. A plain `field == value` on a broad
// scalar field (not a discriminant) is left un-stripped so it stays the legacy no-op and
// never rewrites a sibling guard's field refinement. ok=false reports an unchanged base.
func narrowDiscriminantUnion(base typ.Type, field string, lit *typ.Literal, include bool) (typ.Type, bool) {
	if fieldDiscriminatesUnion(base, field) {
		base = narrow.RemoveNil(base)
	}
	var refined typ.Type
	if include {
		refined = narrow.ByFieldLiteral(base, field, lit, fieldResolver)
	} else {
		refined = narrow.ExcludeByFieldLiteral(base, field, lit, fieldResolver)
	}
	if refined == nil || refined == base {
		return nil, false
	}
	return refined, true
}

// readFieldPath resolves the type at a field path inside t, descending each static
// field/string-index segment via the value-domain field resolver. An empty path returns
// t. A segment that does not resolve (a non-record value, a missing field, an index
// segment) reports ok=false so the caller declines.
func readFieldPath(t typ.Type, segments []constraint.Segment) (typ.Type, bool) {
	cur := t
	for _, seg := range segments {
		if cur == nil {
			return nil, false
		}
		switch seg.Kind {
		case constraint.SegmentField:
			ft, ok := fieldResolver.Field(cur, seg.Name)
			if !ok || ft == nil {
				return nil, false
			}
			cur = ft
		case constraint.SegmentIndexString:
			it, ok := fieldResolver.Index(cur, typ.LiteralString(seg.Name))
			if !ok || it == nil {
				return nil, false
			}
			cur = it
		case constraint.SegmentIndexInt:
			it, ok := fieldResolver.Index(cur, typ.LiteralInt(int64(seg.Index)))
			if !ok || it == nil {
				return nil, false
			}
			cur = it
		default:
			return nil, false
		}
	}
	return cur, true
}

// refineAtPath rebuilds t with the value at field path segments replaced by
// refine(value), descending each static field/string-index segment and reusing the same
// per-member record rebuild mapUnionField applies for the leaf-field narrowing. A
// single-segment path applies refine to that field directly; a deeper path recurses into
// the field's value. It is the write-back counterpart of readFieldPath, used to install a
// member-path discriminant's narrowed union back into its enclosing record. An empty path
// applies refine to t itself.
func refineAtPath(t typ.Type, segments []constraint.Segment, refine func(typ.Type) typ.Type) typ.Type {
	if len(segments) == 0 {
		return refine(t)
	}
	seg := segments[0]
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		if len(segments) == 1 {
			return mapUnionField(t, seg.Name, refine, false)
		}
		return mapUnionField(t, seg.Name, func(ft typ.Type) typ.Type {
			return refineAtPath(ft, segments[1:], refine)
		}, false)
	case constraint.SegmentIndexInt:
		tuple, ok := unwrap.Alias(t).(*typ.Tuple)
		if !ok {
			return t
		}
		idx := seg.Index - 1
		if idx < 0 || idx >= len(tuple.Elements) {
			return t
		}
		next := refineAtPath(tuple.Elements[idx], segments[1:], refine)
		if next == nil || typ.TypeEquals(next, tuple.Elements[idx]) {
			return t
		}
		elems := append([]typ.Type(nil), tuple.Elements...)
		elems[idx] = next
		return typ.NewTuple(elems...)
	default:
		return t
	}
}

// discriminantBase selects the value a discriminant guard refines for sym. The
// choice composes two requirements:
//
//   - SEQUENTIAL EXCLUSION (exhaustiveness): a chain of `if x.kind == k then return`
//     early-returns narrows x on each exclude edge; the second guard must compose with
//     the first's refinement (Output minus RenderOutput minus IndexOutput = AuditOutput),
//     not reset to the full declared union. The dataflow Env already carries the prior
//     edge's refinement, so when the tracked value is itself a (multi-member) union --
//     a shape that only arises from the declared union or a prior union-narrowing of it
//     -- it is the tightest sound base and narrowing over it preserves the composition.
//
//   - CONSTRUCTOR-SINGLETON RECOVERY: an annotated `local r: A|B = {tag="a",...}` seeds
//     the precise singleton {tag:"a",...}; the annotation is authoritative, so the else
//     edge of `if r.tag == "a"` must recover sibling variant B (per the declaration),
//     not collapse to Never over the singleton. The Env there holds a single record
//     (below the union's variant granularity), so the declared union is restored.
//
// The discriminator is structural: a tracked value that unwraps to a multi-member union
// (optionally behind an Optional, the array-index / captured-optional source) is the
// composition base; any other tracked value (a constructor singleton, a scalar, none)
// falls to narrowBase, which restores the declared union for annotation authority.
func (t *Transfer) discriminantBase(sym cfg.SymbolID, av product.AbstractValue) (product.AbstractValue, bool) {
	if !av.IsZero() {
		if u := unwrap.Union(av.ProjectValue()); u != nil && len(u.Members) > 1 {
			return av, true
		}
	}
	return t.narrowBase(sym, av, false)
}

// discriminant is a recognized base.field == literal (or ~=) guard.
type discriminant struct {
	sym cfg.SymbolID
	// prefix is the field path from the root symbol sym down to the union value the
	// discriminant refines. It is empty for a bare-symbol discriminant (`if x.tag ==
	// "a"`, x the union); it carries the intermediate segments for a member-access
	// discriminant (`if receipt.output.kind == "rendered"`, prefix [output], the
	// union value lives at receipt.output, the discriminant field "kind" is read off
	// it). The narrowing refines the value at sym[prefix] and writes the refined value
	// back into that path, leaving the rest of the root record untouched.
	prefix  []constraint.Segment
	field   string
	literal *typ.Literal
	negated bool // the guard is base.field ~= literal
	// include records the refinement sense the dominating-discriminant chain resolved
	// for this guard's surviving edge: true keeps the matched variant, false excludes
	// it. It is meaningful only for a guard the exit-chain composition selected; the
	// per-edge discriminant narrowing derives include from taken/negated directly.
	include bool
}

// discriminantGuard recognizes a discriminated-union equality guard in expr:
// base.field == "tag" or base.field ~= "tag", where base is an identifier the
// graph binds to a symbol and the right side is a literal. Returns false for any
// other shape.
func (t *Transfer) discriminantGuard(expr ast.Expr) (discriminant, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return discriminant{}, false
	}
	negated := false
	switch rel.Operator {
	case "==":
	case "~=":
		negated = true
	default:
		return discriminant{}, false
	}
	// The discriminant access may be on either side; the literal is the other.
	if d, ok := t.discriminantFromSides(rel.Lhs, rel.Rhs, negated); ok {
		return d, true
	}
	return t.discriminantFromSides(rel.Rhs, rel.Lhs, negated)
}

func (t *Transfer) discriminantFromSides(access, value ast.Expr, negated bool) (discriminant, bool) {
	attr, ok := access.(*ast.AttrGetExpr)
	if !ok {
		return discriminant{}, false
	}
	field, ok := staticAttrFieldName(attr)
	if !ok {
		return discriminant{}, false
	}
	lit, ok := literalValue(value)
	if !ok {
		return discriminant{}, false
	}
	basePath, ok := t.staticPathOfExpr(attr.Object)
	if !ok || basePath.Symbol == 0 {
		return discriminant{}, false
	}
	// A bare-symbol discriminant (`if x.tag == "a"`): the access object is the
	// symbol bound to the union directly. A member-access discriminant
	// (`receipt.output.kind == "rendered"`) refines the union value at the static
	// prefix `receipt.output`; the discriminant field is read inside that value.
	if len(basePath.Segments) == 0 {
		return discriminant{sym: basePath.Symbol, field: field, literal: lit, negated: negated}, true
	}
	return discriminant{sym: basePath.Symbol, prefix: basePath.Segments, field: field, literal: lit, negated: negated}, true
}

// literalValue resolves a literal expression (string/number/bool) to its singleton
// literal type, the value a discriminant guard compares against.
func literalValue(expr ast.Expr) (*typ.Literal, bool) {
	switch expr.(type) {
	case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.FalseExpr:
		return literal.FromExpr(expr)
	default:
		return nil, false
	}
}

// applyAssertNarrow refines out by the truthy refinement an assert(cond, ...) call
// proves about its first argument in the CONTINUATION after the call. assert raises
// when cond is falsy, so a body reaching the next point has cond truthy; the
// continuation therefore narrows the asserted value exactly as the TRUE edge of an
// `if cond` branch would. It recognizes the same condition shapes the branch
// narrowing does — a bare value (assert(x) / assert(x.f), truthy), a not-nil / nil
// comparison (assert(x ~= nil), assert(x == nil)), and a discriminant equality
// (assert(x.tag == "a")) — reusing the value-domain narrowers. A refinement that
// collapses the asserted value to the lattice Bottom proves the continuation
// unreachable: dead reports true so the caller terminates the flow (assert(false),
// or assert of an always-false comparison), the same way error() does.
//
// An argument shape the narrowing does not interpret leaves out unchanged and dead
// false (a precision loss, never unsoundness): the assert still type-checks its
// argument through the ordinary call-arg demand.
func (t *Transfer) applyAssertNarrow(out *flow.PointState, call *ast.FuncCallExpr) (dead bool) {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	arg := call.Args[0]
	// assert(<falsy literal>) always raises: the continuation is unreachable.
	if isAlwaysFalsyLiteral(arg) {
		return true
	}
	sym, segments, check, ok := t.assertCondition(arg)
	if ok {
		return t.narrowAssertPath(out, sym, segments, check)
	}
	if t.narrowAssertDiscriminant(out, arg) {
		return false
	}
	return false
}

// assertCondition resolves the asserted argument to the (symbol, field path, check)
// the continuation narrows. A bare identifier or field path is a truthy check; a
// `path ~= nil` / `path == nil` comparison is the corresponding presence check. The
// path's root must bind to a tracked symbol. Returns ok=false for any other shape.
func (t *Transfer) assertCondition(arg ast.Expr) (cfg.SymbolID, []constraint.Segment, cfg.CondCheckKind, bool) {
	switch e := arg.(type) {
	case *ast.IdentExpr:
		if sym := t.symbolOf(e); sym != 0 {
			return sym, nil, cfg.CheckTruthy, true
		}
	case *ast.AttrGetExpr:
		if sym, segs, ok := t.pathSymbol(e); ok {
			return sym, segs, cfg.CheckTruthy, true
		}
	case *ast.RelationalOpExpr:
		return t.assertNilComparison(e)
	}
	return 0, nil, cfg.CheckNone, false
}

// assertNilComparison resolves a `path ~= nil` / `path == nil` assert argument to its
// presence check on the path's symbol: `~= nil` proves the continuation not-nil,
// `== nil` proves it nil. The nil literal may be on either side. A comparison that is
// not against nil, or whose other side is not a tracked path, returns ok=false.
func (t *Transfer) assertNilComparison(rel *ast.RelationalOpExpr) (cfg.SymbolID, []constraint.Segment, cfg.CondCheckKind, bool) {
	var notNil bool
	switch rel.Operator {
	case "~=":
		notNil = true
	case "==":
		notNil = false
	default:
		return 0, nil, cfg.CheckNone, false
	}
	path, ok := assertNilPathSide(rel.Lhs, rel.Rhs)
	if !ok {
		return 0, nil, cfg.CheckNone, false
	}
	sym, segs, ok := t.pathSymbol(path)
	if !ok {
		return 0, nil, cfg.CheckNone, false
	}
	if notNil {
		return sym, segs, cfg.CheckNotNil, true
	}
	return sym, segs, cfg.CheckNil, true
}

// assertNilPathSide returns the non-nil operand of a comparison whose other operand
// is the nil literal. It reports false when neither operand is nil.
func assertNilPathSide(lhs, rhs ast.Expr) (ast.Expr, bool) {
	if _, ok := rhs.(*ast.NilExpr); ok {
		return lhs, true
	}
	if _, ok := lhs.(*ast.NilExpr); ok {
		return rhs, true
	}
	return nil, false
}

// narrowAssertPath narrows the asserted symbol (or field path under it) in out by
// the proven check and reports whether the refinement proves the continuation dead.
// It refines over the declared-type base (narrowBase) so a `local s: string?`
// parameter narrows its declared union, then writes the result back. A refinement
// that collapses the value to Bottom (an asserted comparison that cannot hold for
// the value's type, e.g. assert(x == nil) over a non-optional x) terminates the
// flow. An unrefined value leaves out unchanged.
func (t *Transfer) narrowAssertPath(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind) (dead bool) {
	return t.narrowAssertPathWithBase(out, sym, segments, check, false)
}

func (t *Transfer) narrowAssertPathWithBase(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind, preferEnv bool) (dead bool) {
	return t.narrowAssertPathWithTypeName(out, sym, segments, check, "", preferEnv)
}

func (t *Transfer) narrowAssertPathWithTypeName(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind, typeName string, preferEnv bool) (dead bool) {
	baseAV, has := t.narrowBaseFor(*out, sym, preferEnv)
	if !has {
		return false
	}
	narrowed, ok := narrowAtPath(baseAV, segments, check, typeName)
	if !ok {
		return false
	}
	if product.Domain.Equal(narrowed, product.Bottom()) {
		// The asserted condition cannot hold for this value: the continuation is
		// unreachable, so the caller terminates the flow like error().
		return true
	}
	t.setNarrowedSymbol(out, sym, narrowed)
	return false
}

// narrowAssertDiscriminant applies a discriminant-equality assert (assert(x.tag ==
// "a")) by narrowing x's union to the matching variant in out, reusing the branch
// discriminant narrowing on the TRUE edge. It reports whether a discriminant guard
// was recognized; a non-discriminant argument leaves out unchanged.
func (t *Transfer) narrowAssertDiscriminant(out *flow.PointState, arg ast.Expr) bool {
	info := &cfg.BranchInfo{Condition: arg}
	narrowed, applied := t.narrowByDiscriminant(*out, info, true)
	if !applied {
		return false
	}
	applyNarrowedEdgeState(out, narrowed)
	return true
}

// isAlwaysFalsyLiteral reports whether expr is a literal that is always falsy in Lua
// (nil or false), so an assert of it always raises and the continuation is dead.
func isAlwaysFalsyLiteral(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.NilExpr, *ast.FalseExpr:
		return true
	default:
		return false
	}
}

// ParamNarrow is one parameter-narrowing effect a function body proves on every
// live exit: parameter Param (optionally at field path Segments) satisfies Check
// (CheckTruthy / CheckNotNil / CheckNil / CheckFalsy) when the function returns
// normally. A caller applies it to the matching argument so a wrapper like `function
// check(x) assert(x) end` narrows the argument at `check(y)`. It is the relative form
// of the body's assert / guard refinement, expressed as a check (not a concrete
// type), so it applies even to an unannotated `any` parameter where the body's own
// narrowed type is unchanged.
type ParamNarrow = paramevidence.ParamNarrow

// ParamNarrowEffects extracts the parameter-narrowing effects this function's body
// proves on every normal exit: an assert(param-path[, msg]) whose continuation is
// the only live path, and an `if param-path == nil then error() end` (or `if not
// param-path then ...`) guard whose then-arm terminates. Both reduce to "the
// parameter satisfies a presence/truthy check whenever the function returns". A
// pattern testing a non-parameter value, or a guard whose then-arm does not
// terminate, yields no effect (sound: the caller simply does not narrow).
func (t *Transfer) ParamNarrowEffects() []ParamNarrow {
	g := t.in.Graph
	if g == nil || len(t.paramBySym) == 0 {
		return nil
	}
	var out []ParamNarrow
	add := func(e ParamNarrow, ok bool) {
		if !ok {
			return
		}
		out = append(out, e)
	}
	g.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "assert" || info.Call == nil || len(info.Call.Args) == 0 {
			return
		}
		// Only an assert that runs on EVERY normal return refines the parameter for
		// every caller: an assert nested in a conditional arm (`if check then
		// assert(val) end`) may be skipped, so the parameter is not narrowed on that
		// path. The assert node must dominate the exit for its effect to be sound.
		if !dominatesExit(g, p) {
			return
		}
		add(t.paramEffectFromCondition(info.Call.Args[0], false))
	})
	// A type-cast `T(param)` that dominates the exit asserts the parameter IS T on
	// every normal return: a wrapper `function expect(x) return T(x) end` forwards a
	// concrete-type narrowing to its callers, the transitive form of the call-site cast
	// narrowing. The cast appears as a call SITE (a return/assign source), so it is
	// visited through EachCallSite rather than the statement-only EachCall above.
	g.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil || info.CalleeName == "assert" {
			return
		}
		if !dominatesExit(g, p) {
			return
		}
		add(t.paramCastEffect(info.Call))
	})
	g.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info == nil || !dominatesExit(g, p) {
			return
		}
		e, ok := t.paramEffectFromGuard(g, p, info)
		add(e, ok)
	})
	return paramevidence.SortParamNarrows(out)
}

// paramEffectFromGuard derives the parameter-narrowing effect an `if cond then
// <terminates> end` guard proves on its live (continuation) edge. The guard narrows
// the parameter to the NEGATION of its tested check on the surviving edge: `if param
// == nil then error()` leaves the live path with param not-nil; `if not param then
// error()` leaves it truthy. The effect applies only when exactly one branch arm is
// live to the function exit (the other terminated via error/return), so the negated
// check holds unconditionally past the guard. A guard whose tested value is not a
// parameter path, or where neither arm terminates, yields no effect.
func (t *Transfer) paramEffectFromGuard(g *cfg.Graph, p cfg.Point, info *cfg.BranchInfo) (ParamNarrow, bool) {
	succs := g.Successors(p)
	if len(succs) != 2 {
		return ParamNarrow{}, false
	}
	var trueSucc, falseSucc cfg.Point
	for _, s := range succs {
		if taken, ok := g.EdgeCond(p, s); ok && taken {
			trueSucc = s
		} else {
			falseSucc = s
		}
	}
	if trueSucc == 0 && falseSucc == 0 {
		return ParamNarrow{}, false
	}
	trueLive := reachesExit(g, trueSucc)
	falseLive := reachesExit(g, falseSucc)
	// Exactly one arm survives to exit; the surviving edge's effective check holds on
	// every normal exit.
	switch {
	case falseLive && !trueLive:
		// The then-arm terminated: the live false edge carries the negated check.
		return t.paramEffectFromBranchEdge(info, false)
	case trueLive && !falseLive:
		return t.paramEffectFromBranchEdge(info, true)
	default:
		return ParamNarrow{}, false
	}
}

// paramEffectFromBranchEdge maps a branch's tested condition on the chosen live edge
// to the parameter effect it proves. It resolves the branch condition the same way
// the assert path does (a bare/field-path truthy test or a nil comparison), then
// applies the edge polarity: the effective check on the live edge is the parameter's
// proven refinement. A condition not on a parameter path yields no effect.
func (t *Transfer) paramEffectFromBranchEdge(info *cfg.BranchInfo, taken bool) (ParamNarrow, bool) {
	return t.paramConditionEffect(info.Condition, taken)
}

// paramConditionEffect maps a condition expression whose truth value is proven to a
// caller-portable parameter effect. It is the shared normal-return vocabulary for
// direct assertions, terminating guards, and delegated condition-argument calls:
// nil/truthiness checks, builtin type() checks, parameter equality, and a bare
// condition-argument parameter all reduce to ParamNarrow facts.
func (t *Transfer) paramConditionEffect(cond ast.Expr, taken bool) (ParamNarrow, bool) {
	if e, ok := t.paramEqEffect(cond, relEqualLive(cond, taken)); ok {
		return e, true
	}
	if e, ok := t.paramTypeProbeEffect(cond, taken); ok {
		return e, true
	}
	if e, ok := t.condArgEffect(cond, taken); ok {
		return e, true
	}
	sym, segs, baseCheck, ok := t.assertCondition(cond)
	if !ok {
		return ParamNarrow{}, false
	}
	check := effectiveCheck(baseCheck, taken)
	return t.toParamEffect(sym, segs, check, narrow.TypeKey{})
}

func (t *Transfer) paramTypeProbeEffect(cond ast.Expr, taken bool) (ParamNarrow, bool) {
	cmp, ok := guard.ExtractTypeProbeComparison(cond)
	if !ok || cmp.Probe.Key.IsZero() {
		return ParamNarrow{}, false
	}
	sym, segs, ok := t.pathSymbol(cmp.Probe.Expr)
	if !ok {
		return ParamNarrow{}, false
	}
	check := cfg.CheckTypeNot
	if cmp.Equal {
		check = cfg.CheckTypeEqual
	}
	return t.toParamEffect(sym, segs, effectiveCheck(check, taken), cmp.Probe.Key)
}

// condArgEffect recognizes a guard that tests a parameter directly as a CONDITION —
// `function maybeError(cond) if cond then error() end`, whose surviving edge proves
// cond falsy on every normal return. It emits a CondArg effect carrying the proven
// truthiness so a caller narrows the value its argument expression tests (an argument
// `x == nil` proven falsy narrows x to not-nil). The condition must be the bare
// parameter identifier or its negation (`not cond`); a comparison of the parameter to
// a value is the ordinary value effect, handled by assertCondition. Returns false for
// a non-parameter or non-bare condition.
func (t *Transfer) condArgEffect(cond ast.Expr, taken bool) (ParamNarrow, bool) {
	var ident *ast.IdentExpr
	check := cfg.CheckTruthy
	switch e := cond.(type) {
	case *ast.IdentExpr:
		ident = e
	case *ast.UnaryNotOpExpr:
		inner, ok := e.Expr.(*ast.IdentExpr)
		if !ok {
			return ParamNarrow{}, false
		}
		ident = inner
		check = cfg.CheckFalsy
	default:
		return ParamNarrow{}, false
	}
	idx, isParam := t.paramBySym[t.symbolOf(ident)]
	if !isParam {
		return ParamNarrow{}, false
	}
	proven := effectiveCheck(check, taken)
	switch proven {
	case cfg.CheckTruthy, cfg.CheckFalsy:
		return ParamNarrow{Param: idx, Check: proven, EqParam: -1, CondArg: true}, true
	default:
		return ParamNarrow{}, false
	}
}

// paramEffectFromCondition maps an asserted/guarded condition expression to the
// parameter effect it proves on its truthy (continuation) reading. taken selects the
// edge polarity for a guard; an assert always reads the truthy (taken) sense.
func (t *Transfer) paramEffectFromCondition(cond ast.Expr, _ bool) (ParamNarrow, bool) {
	return t.paramConditionEffect(cond, true)
}

// toParamEffect builds a ParamNarrow when sym is a parameter and check is a
// presence/truthy or absence/falsy refinement the body proves on every normal
// return. A not-nil/truthy wrapper (`function check(x) assert(x) end`) narrows the
// argument to non-nil at the call site; a nil/falsy wrapper (`function is_nil(x) if
// x ~= nil then error() end`) proves the argument nil, which both narrows the
// argument to nil and -- when the argument is a recorded multi-return error symbol --
// strips nil from its correlated value siblings (the call-site form of the (value,
// err) inverse correlation, applied by ApplyParamNarrows/applySiblingNilForErr).
func (t *Transfer) toParamEffect(sym cfg.SymbolID, segs []constraint.Segment, check cfg.CondCheckKind, key narrow.TypeKey) (ParamNarrow, bool) {
	idx, isParam := t.paramBySym[sym]
	if !isParam {
		return ParamNarrow{}, false
	}
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil, cfg.CheckNil, cfg.CheckFalsy:
		return ParamNarrow{Param: idx, Segments: segs, Check: check, EqParam: -1}, true
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
		if key.IsZero() {
			return ParamNarrow{}, false
		}
		if t.declaredParamAlreadyProvesTypeEffect(idx, check, key) {
			return ParamNarrow{}, false
		}
		return ParamNarrow{Param: idx, Segments: segs, Check: check, TypeKey: key, EqParam: -1}, true
	default:
		return ParamNarrow{}, false
	}
}

func (t *Transfer) declaredParamAlreadyProvesTypeEffect(idx int, check cfg.CondCheckKind, key narrow.TypeKey) bool {
	declared := t.declaredParamBySlot[idx]
	if declared == nil {
		return false
	}
	k, ok := key.BuiltinKind()
	if !ok {
		return false
	}
	target := narrow.TypeForKind(k)
	if target == nil {
		return false
	}
	switch check {
	case cfg.CheckTypeEqual:
		return subtype.IsSubtype(declared, target)
	case cfg.CheckTypeNot:
		return !narrow.TypesOverlap(declared, target)
	default:
		return false
	}
}

// paramCastEffect recognizes a type-cast `T(param)` whose single argument is one of
// this function's bare parameters, returning the effect narrowing that parameter to T.
// The cast asserts the parameter IS T on every normal return (the caller composes
// dominatesExit), so a wrapper `function expect(x) return T(x) end` forwards the
// concrete-type narrowing to its callers. A call that is not a cast, or whose argument
// is not a bare parameter, yields no effect.
func (t *Transfer) paramCastEffect(call *ast.FuncCallExpr) (ParamNarrow, bool) {
	if t.callTyper == nil || call == nil || len(call.Args) != 1 {
		return ParamNarrow{}, false
	}
	target, ok := t.callTyper.TypeCastTarget(call, func(ast.Expr) typ.Type { return typ.Unknown })
	if !ok || target == nil || typ.IsAbsentOrUnknown(target) {
		return ParamNarrow{}, false
	}
	ident, ok := call.Args[0].(*ast.IdentExpr)
	if !ok {
		return ParamNarrow{}, false
	}
	sym := t.symbolOf(ident)
	if sym == 0 {
		return ParamNarrow{}, false
	}
	idx, isParam := t.paramBySym[sym]
	if !isParam {
		return ParamNarrow{}, false
	}
	return ParamNarrow{Param: idx, EqParam: -1, CastType: target}, true
}

// paramEqEffect recognizes a parameter equality/inequality relation an
// asserted/guarded condition proves on its live reading. Equality is applied as a
// value intersection at the call site; inequality is replayed as the corresponding
// branch-edge condition so typed-discriminant and path-condition logic can consume it.
func (t *Transfer) paramEqEffect(cond ast.Expr, equalLive bool) (ParamNarrow, bool) {
	rel, ok := cond.(*ast.RelationalOpExpr)
	if !ok {
		return ParamNarrow{}, false
	}
	a, aOK := t.paramOperand(rel.Lhs)
	b, bOK := t.paramOperand(rel.Rhs)
	if !aOK || !bOK || a == b {
		return ParamNarrow{}, false
	}
	if !equalLive {
		return ParamNarrow{Param: a, EqParam: b, NotEqual: true}, true
	}
	return ParamNarrow{Param: a, EqParam: b}, true
}

// paramOperand resolves an identifier operand to its parameter index, or false when
// the operand is not a bare parameter of this function.
func (t *Transfer) paramOperand(expr ast.Expr) (int, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	sym := t.symbolOf(ident)
	if sym == 0 {
		return 0, false
	}
	idx, isParam := t.paramBySym[sym]
	return idx, isParam
}

// relEqualLive reports whether the equality (==) sense holds on the chosen edge for a
// relational condition: an `==` on the taken edge, or a `~=` on the not-taken edge
// (the surviving edge of `if a ~= b then error()`). Any other combination is the
// inequality sense, which carries no intersection effect.
func relEqualLive(cond ast.Expr, taken bool) bool {
	rel, ok := cond.(*ast.RelationalOpExpr)
	if !ok {
		return false
	}
	switch rel.Operator {
	case "==":
		return taken
	case "~=":
		return !taken
	default:
		return false
	}
}

// reachesExit reports whether the function's exit point is reachable from p by a
// forward walk over the CFG edges. A then-arm that terminates with error() (a call
// node with no successors) does not reach the exit, which is how a guard's
// terminating arm is distinguished from its live continuation.
func reachesExit(g *cfg.Graph, p cfg.Point) bool {
	if g == nil || p == 0 {
		return false
	}
	exit := g.Exit()
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{p}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == exit {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return false
}

// DelegatedCall is an exit-dominating call inside a function body that may forward a
// parameter narrowing from its callee: the call expression and, per callee argument
// position, the caller parameter index passed there (-1 when the argument is not a
// bare caller parameter). A caller whose callee narrows parameter i, and whose
// argument i is the caller's own parameter j, narrows j too — the transitive wrapper
// (`outerAssert(val)` calls `innerAssert(val)`). The mapping covers bare-parameter
// arguments only; a field-path argument is not forwarded (its narrowing would target
// the parameter's field, not the parameter, which these wrappers do not require).
type DelegatedCall = paramevidence.DelegatedCall

// ExitDominatingCalls returns the calls in this body that run on every normal return
// (they dominate the exit) paired with the caller-parameter each argument forwards.
// The driver resolves each call's callee and composes the callee's parameter effects
// with this mapping to derive the caller's transitive effects. A call that does not
// dominate the exit (a conditional call) forwards nothing: its callee's narrowing
// does not hold on the skipping path.
func (t *Transfer) ExitDominatingCalls() []DelegatedCall {
	g := t.in.Graph
	if g == nil || len(t.paramBySym) == 0 {
		return nil
	}
	var out []DelegatedCall
	g.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil || info.CalleeName == "assert" {
			return
		}
		if !dominatesExit(g, p) {
			return
		}
		argParams := make([]int, len(info.Call.Args))
		argTruthy := make([][]ParamNarrow, len(info.Call.Args))
		argFalsy := make([][]ParamNarrow, len(info.Call.Args))
		any := false
		for i, arg := range info.Call.Args {
			argParams[i] = -1
			if e, ok := t.paramConditionEffect(arg, true); ok {
				argTruthy[i] = []ParamNarrow{e}
				any = true
			}
			if e, ok := t.paramConditionEffect(arg, false); ok {
				argFalsy[i] = []ParamNarrow{e}
				any = true
			}
			ident, ok := arg.(*ast.IdentExpr)
			if !ok {
				continue
			}
			sym := t.symbolOf(ident)
			if sym == 0 {
				continue
			}
			if idx, isParam := t.paramBySym[sym]; isParam {
				argParams[i] = idx
				any = true
			}
		}
		if any {
			out = append(out, DelegatedCall{
				Call:             info.Call,
				ArgParams:        argParams,
				ArgTruthyEffects: argTruthy,
				ArgFalsyEffects:  argFalsy,
			})
		}
	})
	return out
}

// dominatesExit reports whether q is on every path from the entry to the function
// exit — equivalently, that q dominates the exit. It tests this directly: the exit
// is reachable from the entry WITHOUT passing through q iff some entry-to-exit path
// avoids q, so q dominates the exit exactly when the exit is unreachable from the
// entry once q is removed. A narrowing node that does not dominate the exit (an
// assert in a conditional arm) cannot soundly refine a parameter for every caller.
func dominatesExit(g *cfg.Graph, q cfg.Point) bool {
	if g == nil {
		return false
	}
	entry := g.Entry()
	exit := g.Exit()
	if q == entry {
		return true
	}
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{entry}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == q {
			// Paths through q do not witness an alternate route; do not expand q.
			continue
		}
		if cur == exit {
			// The exit is reachable without passing through q: q is not a dominator.
			return false
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return true
}

// ApplyParamNarrows refines the call arguments in out by the callee's parameter
// narrowing effects: an effect on parameter i narrows the i-th argument's value
// (along its field path) by the proven check, and materializes the same proof in
// PointState.Cond through ConditionEffect. It applies only to an identifier or
// field-path argument whose root the flow tracks, so a wrapper call `check(y)`
// narrows `y` exactly as the wrapper's body proved its parameter. An argument the
// flow does not track, or an effect with no matching argument, is left unchanged.
// It reports dead when a callee-proven postcondition cannot hold for the current
// argument value; the call continuation is then unreachable, like a failed assert.
func (t *Transfer) ApplyParamNarrows(out *flow.PointState, call *ast.FuncCallExpr, effects []ParamNarrow) (dead bool) {
	if call == nil || len(effects) == 0 {
		return false
	}
	for _, e := range effects {
		if e.Param < 0 || e.Param >= len(call.Args) {
			continue
		}
		t.applyParamNarrowConditionEffect(out, call, e)
		if e.IsParamEquality() {
			t.applyParamEqNarrow(out, call, e)
			continue
		}
		if e.IsParamInequality() {
			t.applyParamNeqNarrow(out, call, e)
			continue
		}
		if e.CastType != nil {
			t.applyParamCastNarrow(out, call.Args[e.Param], e.CastType)
			continue
		}
		if e.CondArg {
			t.applyParamCondNarrow(out, call.Args[e.Param], e.Check)
			continue
		}
		argSym, argSegs, ok := t.pathSymbol(call.Args[e.Param])
		if !ok {
			continue
		}
		segs := append(append([]constraint.Segment{}, argSegs...), e.Segments...)
		if !e.TypeKey.IsZero() {
			if t.narrowAssertTypePath(out, argSym, segs, e.Check, e.TypeKey) {
				return true
			}
			continue
		}
		if t.narrowAssertPath(out, argSym, segs, e.Check) {
			return true
		}
		// When the narrowed argument is a recorded multi-return error symbol proven nil
		// (a `test.is_nil(err)` wrapper that errors unless err is nil), its correlated
		// value siblings are non-nil, so strip nil from them — the same correlation a
		// branch `if err ~= nil then ... end` triggers, here proven by the wrapper call.
		if len(segs) == 0 && (e.Check == cfg.CheckNil || e.Check == cfg.CheckFalsy) {
			t.applySiblingNilForErr(out, argSym)
		}
	}
	return false
}

// applyParamNarrowConditionEffect lowers a portable callee postcondition from
// placeholder paths ($0, $1.field, ...) to the caller's normalized static argument
// paths, then writes it through ConditionEffect. This is the condition-axis half
// of parameter-effect application; value narrowing below remains the value-axis
// half. Dynamic or untracked argument paths are intentionally not fabricated: their
// placeholder substitution drops the fact, preserving soundness as precision loss.
func (t *Transfer) applyParamNarrowConditionEffect(out *flow.PointState, call *ast.FuncCallExpr, e ParamNarrow) bool {
	c, ok := paramevidence.ParamNarrowConstraint(e)
	if !ok {
		return false
	}
	args := make([]constraint.Path, len(call.Args))
	for i, arg := range call.Args {
		path, ok := t.staticPathOfExpr(arg)
		if !ok || path.Symbol == 0 {
			continue
		}
		args[i] = path
	}
	cond := constraint.FromConstraints(c).Substitute(args)
	return t.applyConditionEffect(out, ConditionEffect{Fact: cond})
}

func (t *Transfer) narrowAssertTypePath(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind, key narrow.TypeKey) (dead bool) {
	if key.Kind != narrow.TypeKeyBuiltin || key.Name == "" {
		return false
	}
	switch check {
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
	default:
		return false
	}
	return t.narrowAssertPathWithTypeName(out, sym, segments, check, key.Name, false)
}

// applySiblingNilForErr strips nil from the value siblings correlated with err
// symbol, when err has just been proven nil. It is the call-site (param-narrow)
// counterpart of narrowBySiblingNil's branch-edge narrowing.
func (t *Transfer) applySiblingNilForErr(out *flow.PointState, errSym cfg.SymbolID) {
	if out == nil || errSym == 0 {
		return
	}
	bind, ok := out.Rel.SiblingNil(errSym)
	if !ok {
		return
	}
	for _, vs := range bind.ValueSyms {
		if vs == 0 {
			continue
		}
		av, has := t.symbolValue(out, vs)
		if !has || av.IsZero() {
			continue
		}
		t.setNarrowedSymbol(out, vs, product.NarrowPresent(av))
	}
}

// applyParamCondNarrow narrows the value an argument CONDITION tests when the callee
// proves that argument's truthiness on every normal return. The proven check is the
// argument condition's truthiness (CheckFalsy for `maybeError(cond) if cond then
// error()`); applying it is equivalent to taking the branch edge on which `if arg`
// has that truthiness, so the argument's tested value is narrowed by the same per-edge
// machinery — `x == nil` proven falsy narrows x to not-nil. An argument the narrowing
// cannot classify leaves out unchanged (sound: a precision loss).
func (t *Transfer) applyParamCondNarrow(out *flow.PointState, arg ast.Expr, proven cfg.CondCheckKind) {
	wantTruthy := proven == cfg.CheckTruthy
	leaf := &cfg.BranchInfo{Condition: arg}
	condVar, check := extraction.ExtractCondition(arg)
	leaf.CondVar = condVar
	leaf.CondCheck = check
	leaf.CondSymbol = t.condRootSymbol(arg, condVar)
	narrowed := t.narrowEdgeInner(0, *out, leaf, wantTruthy, false)
	applyNarrowedEdgeState(out, narrowed)
}

// applyParamCastNarrow narrows the argument of a forwarded type-cast effect to the
// asserted type at the call site: the callee's body proved its parameter IS castType
// on every normal return (a `function expect(x) return T(x) end` wrapper), so the
// matching argument here provably IS castType too. It rewrites the argument's tracked
// value (or the field path within it) to castType, the call-site counterpart of
// applyTypeCastNarrow. An argument the flow does not track is left unchanged.
func (t *Transfer) applyParamCastNarrow(out *flow.PointState, arg ast.Expr, castType typ.Type) {
	if castType == nil || typ.IsAbsentOrUnknown(castType) {
		return
	}
	sym, segs, ok := t.pathSymbolInState(out, arg, nil)
	if !ok || sym == 0 {
		return
	}
	place, ok := staticPlace(sym, segs)
	if !ok {
		return
	}
	t.applyRefinementEffect(out, RefinementEffect{
		Place:     place,
		Kind:      RefinementTypeCast,
		Target:    castType,
		PreferEnv: true,
	})
}

// applyParamEqNarrow applies a parameter-equality effect at the call site: argument
// e.Param is narrowed to the intersection of its tracked value with argument
// e.EqParam's value, the value-domain form of the equality the callee proved (the two
// parameters are equal on every normal return, so their argument types must overlap).
// An argument the flow does not track, or an empty intersection, leaves the value
// unchanged (sound: a precision loss, never a fabricated narrowing).
func (t *Transfer) applyParamEqNarrow(out *flow.PointState, call *ast.FuncCallExpr, e ParamNarrow) {
	if !e.IsParamEquality() || e.EqParam >= len(call.Args) {
		return
	}
	t.applyArgumentEqualityProof(out, call.Args[e.Param], call.Args[e.EqParam])
	targetSym, segs, ok := t.pathSymbol(call.Args[e.Param])
	if !ok || len(segs) != 0 {
		return
	}
	targetAV, has := t.narrowBaseFor(*out, targetSym, false)
	if !has {
		return
	}
	otherAV, ok := t.evalExpr(out, call.Args[e.EqParam], nil)
	if !ok || otherAV.IsZero() {
		return
	}
	targetType := targetAV.ProjectValue()
	otherType := otherAV.ProjectValue()
	if targetType == nil || otherType == nil {
		return
	}
	refined := narrow.Intersect(targetType, otherType)
	if refined == nil || refined == targetType {
		return
	}
	if refined.Kind().IsNever() {
		t.setNarrowedSymbol(out, targetSym, product.Bottom())
		return
	}
	t.setNarrowedSymbol(out, targetSym, product.FromType(refined))
}

func (t *Transfer) applyParamNeqNarrow(out *flow.PointState, call *ast.FuncCallExpr, e ParamNarrow) {
	if out == nil || !e.IsParamInequality() || e.EqParam >= len(call.Args) {
		return
	}
	rel := &ast.RelationalOpExpr{
		Lhs:      call.Args[e.Param],
		Operator: "~=",
		Rhs:      call.Args[e.EqParam],
	}
	leaf := &cfg.BranchInfo{Condition: rel}
	narrowed := t.narrowEdgeInner(0, *out, leaf, true, false)
	applyNarrowedEdgeState(out, narrowed)
}

func (t *Transfer) applyArgumentEqualityProof(out *flow.PointState, left, right ast.Expr) {
	if out == nil || left == nil || right == nil {
		return
	}
	t.applyLengthEqualityProof(out, left, right)
	t.applyLiteralEqualityProof(out, left, right)
	t.applyLiteralEqualityProof(out, right, left)
}

func (t *Transfer) applyLengthEqualityProof(out *flow.PointState, left, right ast.Expr) {
	place, arrKey, c, op, ok := t.orientLengthComparison(left, right, "==")
	if !ok {
		return
	}
	ops := numericLengthBoundOps(arrKey, op, c)
	if len(ops) == 0 {
		return
	}
	t.applyNumericEffect(out, NumericEffect{Ops: ops})
	if floor, _, hasFloor, _ := lengthBoundFromOp(op, c); hasFloor && floor > 0 {
		t.applyRefinementEffect(out, RefinementEffect{
			Place:     place,
			Kind:      RefinementLengthLowerBound,
			LengthMin: floor,
			PreferEnv: true,
		})
	}
}

func (t *Transfer) applyLiteralEqualityProof(out *flow.PointState, access ast.Expr, value ast.Expr) {
	lit, ok := literalValue(value)
	if !ok || lit == nil {
		return
	}
	t.seedPathIndexPresence(out, access)
	if t.narrowLiteralEqualityPath(out, access, lit) {
		return
	}
	if cond := t.literalEqualityCondition(access, lit); cond.HasConstraints() {
		t.applyConditionEffect(out, ConditionEffect{Fact: cond})
	}
}

func (t *Transfer) seedPathIndexPresence(out *flow.PointState, expr ast.Expr) {
	path, ok := t.staticPathOfExpr(expr)
	if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
		return
	}
	ops := make([]NumericOp, 0, len(path.Segments))
	for i, seg := range path.Segments {
		if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 {
			continue
		}
		containerKey := flow.SymbolPathKey(path.Symbol, nil)
		if i > 0 {
			containerKey = flow.SymbolPathKey(path.Symbol, path.Segments[:i])
		}
		ops = append(ops, NumericOp{
			Kind:  NumericLenGeConst,
			Key:   containerKey,
			Const: int64(seg.Index),
		})
	}
	t.applyNumericEffect(out, NumericEffect{Ops: ops})
}

func (t *Transfer) narrowLiteralEqualityPath(out *flow.PointState, access ast.Expr, lit *typ.Literal) bool {
	attr, ok := access.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return false
	}
	field, ok := staticAttrFieldName(attr)
	if !ok || field == "" {
		return false
	}
	basePath, ok := t.staticPathOfExpr(attr.Object)
	if !ok || basePath.Symbol == 0 {
		return false
	}
	av, _ := t.symbolValue(out, basePath.Symbol)
	if av.IsZero() {
		return false
	}
	root := av.ProjectValue()
	if root == nil {
		return false
	}
	target, ok := readFieldPath(root, basePath.Segments)
	if !ok || target == nil {
		return false
	}
	refined, ok := narrowDiscriminantUnion(target, field, lit, true)
	if !ok {
		return false
	}
	rewritten := refineAtPath(root, basePath.Segments, func(typ.Type) typ.Type { return refined })
	if rewritten == nil || typ.TypeEquals(rewritten, root) {
		return false
	}
	t.setNarrowedSymbol(out, basePath.Symbol, product.FromType(rewritten))
	return true
}

func (t *Transfer) literalEqualityCondition(access ast.Expr, lit *typ.Literal) constraint.Condition {
	attr, ok := access.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return constraint.TrueCondition()
	}
	field, ok := staticAttrFieldName(attr)
	if !ok || field == "" {
		return constraint.TrueCondition()
	}
	target, ok := t.staticPathOfExpr(attr.Object)
	if !ok || target.Symbol == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromConstraints(constraint.FieldEquals{Target: target, Field: field, Value: lit})
}

func applyNarrowedEdgeState(out *flow.PointState, narrowed flow.PointState) {
	if out == nil {
		return
	}
	*out = narrowed
}
