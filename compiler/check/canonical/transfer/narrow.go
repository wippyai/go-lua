package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
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

// fieldResolver is the structural field/index resolver the discriminant narrowing
// reads to look up a variant field's type. It is the pure value-domain resolver
// (types/query/core), not a parallel implementation.
var fieldResolver = querycore.Resolver()

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

func (t *Transfer) numericForBodyEdgeState(g *cfg.Graph, branch cfg.Point, out flow.PointState) flow.PointState {
	node := g.Node(branch)
	if node == nil || !node.LoopPreheaderSet {
		return out
	}
	info := g.Assign(node.LoopPreheader)
	if info == nil || info.NumericFor == nil || info.NumericFor.VarName == "" {
		return out
	}
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return out
	}
	res := flow.ClonePointState(out)
	t.seedNumericForBodyBounds(&res, target.Symbol, info.NumericFor)
	return res
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

func (t *Transfer) narrowByBranchConditionEffect(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	effect, ok := t.branchConditions.effect(point, out, info, taken)
	if !ok {
		return out
	}
	// Most condition facts only update persistent proof axes. Start with a
	// shallow edge state and detach Env only if reductions materialize a symbol
	// value refinement.
	res := out
	t.applyBorrowedEdgeConditionEffect(&res, effect)
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
	res := flow.ClonePointStateForEdgeFactEffect(out)
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

func (t *Transfer) narrowByGuardedType(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	sym := info.CondSymbol
	if sym == 0 {
		sym = t.condTestSymbol(info)
	}
	if sym == 0 || len(t.condTestSegments(info)) > 0 {
		return out
	}
	check := effectiveCheck(info.CondCheck.Kind, taken)
	var guardTruthy bool
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil:
		guardTruthy = true
	case cfg.CheckFalsy, cfg.CheckNil:
		guardTruthy = false
	default:
		return out
	}
	rels := out.Rel.GuardedTypesForGuard(sym, guardTruthy)
	if len(rels) == 0 {
		return out
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
	applied := false
	for _, rel := range rels {
		if rel.TargetSym == 0 || rel.TargetType == nil {
			continue
		}
		av, has := t.symbolValue(&res, rel.TargetSym)
		if !has || av.IsZero() {
			continue
		}
		refined := product.FromRefinedType(av, rel.TargetType)
		if refined.IsZero() {
			refined = product.FromType(rel.TargetType)
		}
		t.setNarrowedSymbol(&res, rel.TargetSym, refined)
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
func (t *Transfer) narrowByCompound(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
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
		return t.narrowOrUnionTrueEdgeAtPoint(point, out, logical)
	}
	if logical.Operator == "and" && !wantTruthy {
		return t.narrowAndUnionFalseEdgeAtPoint(point, out, logical)
	}
	operands, decomposable := compoundOperands(logical, wantTruthy)
	if !decomposable {
		return out, false
	}
	state := out
	applied := false
	for _, operand := range operands {
		narrowed, ok := t.narrowOperand(point, state, operand)
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
	return t.narrowOrUnionTrueEdgeAtPoint(0, out, logical)
}

func (t *Transfer) narrowOrUnionTrueEdgeAtPoint(point cfg.Point, out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowSameValueUnion(point, out,
		operandGuard{expr: logical.Lhs, truthy: true},
		operandGuard{expr: logical.Rhs, truthy: true},
	)
}

// narrowAndUnionFalseEdge narrows the FALSE edge of `A and B` when both operands
// constrain the same value. The edge proves `!A OR !B`; joining each operand's
// false-edge refinement gives the least value fact representable by the product
// value domain without path-splitting.
func (t *Transfer) narrowAndUnionFalseEdge(out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowAndUnionFalseEdgeAtPoint(0, out, logical)
}

func (t *Transfer) narrowAndUnionFalseEdgeAtPoint(point cfg.Point, out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowSameValueUnion(point, out,
		operandGuard{expr: logical.Lhs, truthy: false},
		operandGuard{expr: logical.Rhs, truthy: false},
	)
}

func (t *Transfer) narrowSameValueUnion(point cfg.Point, out flow.PointState, lhsGuard, rhsGuard operandGuard) (flow.PointState, bool) {
	lhs, lslot, lok := t.operandNarrowsOneSlot(point, out, lhsGuard)
	if !lok {
		return out, false
	}
	rhs, rslot, rok := t.operandNarrowsOneSlot(point, out, rhsGuard)
	if !rok || !lslot.Equal(rslot) {
		return out, false
	}
	lv, lok := t.valueBySlot(lhs, lslot)
	rv, rok := t.valueBySlot(rhs, rslot)
	if !lok || !rok {
		return out, false
	}
	joined := product.Join(lv, rv)
	res := flow.ClonePointStateForEdgeFactEffect(out)
	t.setValueBySlot(&res, lslot, joined, false)
	return res, true
}

// operandNarrowsOneSlot narrows state by one logical operand and reports the
// single value slot it refined. It returns ok=true only when the operand refines
// EXACTLY one slot, so the caller can join two disjuncts that agree on which
// value they constrain; an operand that refines no slot or more than one slot is
// not a same-value disjunct and returns ok=false.
func (t *Transfer) operandNarrowsOneSlot(point cfg.Point, state flow.PointState, guard operandGuard) (flow.PointState, flow.ValueSlot, bool) {
	narrowed, ok := t.narrowOperand(point, state, guard)
	if !ok {
		return flow.PointState{}, flow.ValueSlot{}, false
	}
	changed, ok := flow.SingleChangedValueSlot(state, narrowed)
	if !ok {
		return flow.PointState{}, flow.ValueSlot{}, false
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
func (t *Transfer) narrowOperand(point cfg.Point, state flow.PointState, og operandGuard) (flow.PointState, bool) {
	condVar, check := extraction.ExtractCondition(og.expr)
	leaf := &cfg.BranchInfo{
		CondVar:    condVar,
		CondSymbol: t.condRootSymbol(og.expr, condVar),
		CondCheck:  check,
		Condition:  og.expr,
	}
	narrowed := t.narrowEdgeInner(point, state, leaf, og.truthy, false)
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
// Modern CFGs also carry CondOrigin on the ScopeExit, which is the authoritative
// link back to the branch. The backward walk is only a legacy/degenerate fallback.
//
// When no originating branch is recoverable (a degenerate graph), the lossy bare
// reconstruction is returned only for a bare-symbol root path — a field-path guard
// declines rather than narrow the wrong (root) value. Edge polarity is selected by
// EdgeCond on the exit node's outgoing edge: the then-exit's edge to the merge is
// the TRUE edge, the else-exit's the FALSE edge.
func exitGuard(g *cfg.Graph, pred cfg.Point) (*cfg.BranchInfo, cfg.Point, bool) {
	node := g.Node(pred)
	if node == nil || node.Kind != cfg.NodeScopeExit {
		return nil, 0, false
	}
	if node.CondOriginSet {
		info := g.Branch(node.CondOrigin)
		if info == nil {
			return nil, 0, false
		}
		return info, node.CondOrigin, true
	}
	// Scope-exit replays are only authoritative when the CFG copied the tested
	// root symbol. Relational/complex guards often carry CondVar=0; matching them
	// by check kind alone can recover the wrong elseif branch and collapse a live
	// arm. The actual branch edge already applied those guards.
	if node.CondVar == 0 {
		return nil, 0, false
	}
	if node.CondCheck.Kind == cfg.CheckNone {
		return nil, 0, false
	}
	if info, branch, ok := originatingBranch(g, pred, node.CondVar, node.CondCheck); ok {
		return info, branch, true
	}
	return &cfg.BranchInfo{
		CondVar:    g.NameOf(node.CondVar),
		CondSymbol: node.CondVar,
		CondCheck:  node.CondCheck,
	}, 0, false
}

// originatingBranch finds the branch a ScopeExit copied its guard markers from: the
// nearest NodeBranch reached by a backward walk over predecessors whose CondSymbol
// and CondCheck match the markers the exit node carries. It returns that branch's
// full BranchInfo (with the intact Condition AST and field-path CondVar string), so
// the exit re-narrowing operates on the SAME path the branch tested, recovering the
// field segments the node-level markers drop. A no-match (no matching branch is a
// backward ancestor) returns nil so the caller falls back to the bare reconstruction.
func originatingBranch(g *cfg.Graph, exit cfg.Point, condSym cfg.SymbolID, check cfg.CondCheck) (*cfg.BranchInfo, cfg.Point, bool) {
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
				return info, p, true
			}
			next = append(next, g.Predecessors(p)...)
		}
		frontier = next
	}
	return nil, 0, false
}

// scopeExitGuardPathMutated reports whether the arm between branch and its
// ScopeExit wrote the path whose value the branch guard tested. A ScopeExit guard
// is historical: it speaks about the value at the branch. If the arm overwrote the
// tested path (or one of its ancestors), reapplying that guard to the current
// store would refine the replacement value with an obsolete fact.
func (t *Transfer) scopeExitGuardPathMutated(g *cfg.Graph, branch, exit cfg.Point, info *cfg.BranchInfo) bool {
	if g == nil || info == nil {
		return false
	}
	sym := t.condTestSymbol(info)
	if sym == 0 {
		return false
	}
	segments := t.condTestSegments(info)
	seen := map[cfg.Point]bool{exit: true}
	frontier := append([]cfg.Point(nil), g.Predecessors(exit)...)
	for len(frontier) > 0 {
		p := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if p == branch || seen[p] {
			continue
		}
		seen[p] = true
		if t.assignmentWritesGuardPath(g.Assign(p), sym, segments) {
			return true
		}
		frontier = append(frontier, g.Predecessors(p)...)
	}
	return false
}

func (t *Transfer) assignmentWritesGuardPath(info *cfg.AssignInfo, sym cfg.SymbolID, segments []constraint.Segment) bool {
	if info == nil || sym == 0 {
		return false
	}
	for _, target := range info.Targets {
		path, ok := t.staticPathOfAssignTarget(target)
		if !ok || path.Symbol != sym {
			continue
		}
		if pathWriteInvalidatesGuard(path.Segments, segments) {
			return true
		}
	}
	return false
}

func pathWriteInvalidatesGuard(write, guard []constraint.Segment) bool {
	if len(write) > len(guard) {
		return false
	}
	for i := range write {
		if write[i] != guard[i] {
			return false
		}
	}
	return true
}

// narrowByTypeCheck applies the value-narrowing a `local val, err = T:is(x)` guard
// proves. When the branch tests the error symbol of such an assignment and the
// chosen edge is the success edge (err proven nil — `if err == nil` true edge, or
// `if err`/`if err ~= nil` false edge), the checked value symbols narrow to the
// checked type T. It reuses the recorded TypeCheckBind; a non-type-check guard
// returns applied=false so the discriminant / cond-check paths run.
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
	res := flow.ClonePointStateForEdgeFactEffect(out)
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
	return t.narrowByCondCheckAtPoint(0, out, info, taken, atExit)
}

func (t *Transfer) narrowByCondCheckAtPoint(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) flow.PointState {
	check := effectiveCheck(info.CondCheck.Kind, taken)
	if check == cfg.CheckNone {
		return out
	}
	out = t.narrowGuardedIndexPresence(out, info, check)
	sym := t.condTestSymbol(info)
	segments := t.condTestSegments(info)
	if pathSym, pathSegments, ok := t.condTestPathInState(&out, info); ok {
		if sym == 0 {
			sym = pathSym
		}
		if len(pathSegments) > 0 {
			segments = pathSegments
		}
	}
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
	guard := conditionPathGuard{
		point:    point,
		sym:      sym,
		segments: segments,
		varPath:  info.CondVar,
		check:    check,
		typeName: info.CondCheck.TypeName,
	}
	currentAV, hasCurrent := t.symbolValue(&out, sym)
	seed := t.narrowSeed(sym, currentAV, atExit)
	baseAV, has := seed.value, seed.hasValue()

	cond := guard.condition(t)
	res := flow.ClonePointState(out)
	if cond.HasConstraints() {
		beforeAV, beforeOK := t.symbolValue(&res, sym)
		t.applyConditionEffect(&res, ConditionEffect{Fact: cond})
		if afterAV, afterOK := t.symbolValue(&res, sym); afterOK && !afterAV.IsZero() &&
			(!beforeOK || beforeAV.IsZero() || !product.Domain.Equal(beforeAV, afterAV)) {
			baseAV = afterAV
			has = true
		}
		if flow.PointStateDomain.Equal(res, flow.PointStateDomain.Bottom()) {
			return res
		}
	}
	// A positive guard on a literal index path `arr[i]` (`if arr[i]`, `arr[i] ~= nil`)
	// proves the element at index i is present, so the container holds at least i
	// elements: record `#arr >= i` on this edge. A later read of `arr[i]` in the
	// guarded body reads it in-range and drops the soundly-optional element nil
	// through the existing length proof, recovering the non-optional element exactly
	// where the guard establishes presence. The merge-LUB rebuilds the unbounded
	// length where both edges meet, so the floor never survives past the guard.
	res = t.narrowIndexPresenceLength(res, guard.sym, guard.segments, guard.check)
	guard.refineStaticMemberFact(t, &res, baseAV, has)
	if !has {
		// No tracked value to refine; the per-edge path condition still records the
		// guard for soundness. For a bare-symbol positive guard, also materialize the
		// reduced product's presence component: the structure is still dynamic, but
		// this edge has proven the symbol non-nil. Without this reduction a later
		// return projection sees "no Env fact" instead of "present dynamic".
		if guard.narrowsBareSymbolPresence() {
			if t.unannotatedParam.Contains(guard.sym) {
				return res
			}
			t.setNarrowedSymbol(&res, guard.sym, product.PresentDynamic())
		}
		return res
	}
	narrowed, ok := guard.narrowValue(baseAV)
	if !ok {
		return res
	}
	narrowedBase := baseAV
	if seed.fromDeclared() && hasCurrent && !currentAV.IsZero() && !currentAV.Covers(narrowed) {
		currentNarrowed, currentOK := guard.narrowValue(currentAV)
		if currentOK && !valueIsBottom(currentNarrowed) &&
			guard.authorizesCurrentSeed(t, &res, seed.value, currentAV) {
			narrowed = currentNarrowed
			narrowedBase = currentAV
		}
	} else if !seed.fromDeclared() && hasCurrent && !currentAV.IsZero() && !currentAV.Covers(narrowed) {
		currentNarrowed, currentOK := guard.narrowValue(currentAV)
		if !currentOK || !flow.SemanticProductReduction(currentAV, currentNarrowed) {
			return res
		}
		narrowed = currentNarrowed
		narrowedBase = currentAV
	}
	if valueIsBottom(narrowed) && missingStaticMemberGuardStaysDynamic(narrowedBase, guard.segments, guard.check, guard.typeName) {
		return res
	}
	t.setNarrowedSymbol(&res, guard.sym, narrowed)
	return res
}

func (t *Transfer) condTestPathInState(out *flow.PointState, info *cfg.BranchInfo) (cfg.SymbolID, []constraint.Segment, bool) {
	expr := condCheckedExpr(info)
	if expr == nil {
		return 0, nil, false
	}
	return t.pathSymbolInState(out, expr, nil)
}

func condCheckedExpr(info *cfg.BranchInfo) ast.Expr {
	if info == nil {
		return nil
	}
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		return info.Condition
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return not.Expr
		}
		return info.Condition
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			if _, ok := rel.Lhs.(*ast.NilExpr); ok {
				return rel.Rhs
			}
			if _, ok := rel.Rhs.(*ast.NilExpr); ok {
				return rel.Lhs
			}
		}
	}
	return nil
}

func (t *Transfer) resolveTypeKey(key narrow.TypeKey) typ.Type {
	if key.Kind == narrow.TypeKeyBuiltin {
		builtin, ok := key.BuiltinKind()
		if !ok {
			return nil
		}
		return narrow.TypeForKind(builtin)
	}
	if t == nil || t.typeKey == nil {
		return nil
	}
	return t.typeKey(key)
}

func (t *Transfer) narrowGuardedIndexPresence(out flow.PointState, info *cfg.BranchInfo, check cfg.CondCheckKind) flow.PointState {
	access := guardedIndexPresenceAccess(info, check)
	if access == nil {
		return out
	}
	effect, ok := t.guardedIndexPresenceTransaction(access)
	if !ok {
		return out
	}
	// Guarded-index presence publishes only finite key-provenance proof axes.
	res := out
	t.applyKeyProvenancePathTransaction(&res, effect)
	return res
}

func (t *Transfer) guardedIndexPresenceTransaction(access *ast.AttrGetExpr) (flow.KeyProvenancePathTransaction, bool) {
	if access == nil {
		return flow.KeyProvenancePathTransaction{}, false
	}
	if _, isStatic := staticMemberKey(access); isStatic {
		return flow.KeyProvenancePathTransaction{}, false
	}
	tablePath, ok := t.containerExprPath(access.Object)
	if !ok || tablePath.IsEmpty() {
		return flow.KeyProvenancePathTransaction{}, false
	}
	keyPath, ok := t.dynamicIndexKeyPath(access.Key)
	if !ok || keyPath.IsEmpty() {
		return flow.KeyProvenancePathTransaction{}, false
	}
	return flow.KeyProvenancePathTransaction{
		Kind:      flow.KeyProvenanceGuardedIndex,
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

func missingStaticMemberGuardStaysDynamic(base product.AbstractValue, segments []constraint.Segment, check cfg.CondCheckKind, typeName string) bool {
	if len(segments) == 0 || !staticMemberGuardImpliesPresence(check, typeName) || base.IsZero() {
		return false
	}
	baseType := base.ProjectValue()
	return !fieldPathResolves(baseType, segments) && querycore.MissingFieldReadsNil(baseType)
}

func fieldPathResolves(t typ.Type, segments []constraint.Segment) bool {
	if t == nil || len(segments) == 0 {
		return false
	}
	current := t
	for _, seg := range segments {
		if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
			return false
		}
		next, ok := fieldResolver.Field(current, seg.Name)
		if !ok || next == nil {
			return false
		}
		current = next
	}
	return true
}

// narrowByPredicate applies the value-narrowing a local type-predicate guard
// proves. A predicate is a local function whose body returns a builtin
// `type(param) == kind` test on one of its parameters (recorded as a guard-owned
// predicate function fact). On the edge
// the predicate result holds true, the argument passed at the tested parameter
// narrows to that kind, exactly as a direct `type(arg) == kind` guard would.
//
// The narrowing is ONE-SIDED: a predicate body is a conjunction of conditions
// (`type(v) == "number" and v > 0`), so a false result does not prove the argument
// is NOT the kind. Only the true edge narrows; the false edge leaves the argument
// its declared (gradual) type, preserving the one-sided soundness boundary the
// false-branch adversarial cases pin.
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
	res := flow.ClonePointStateForEdgeFactEffect(out)
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
	next, ok := flow.ProductConditionReductionValue(flow.ProductConditionReduction{
		Symbol:   sym,
		Base:     base,
		HasBase:  hasBase,
		Fact:     out.Cond,
		Facts:    flow.PointFactsOfBorrowed(out),
		Resolver: fieldResolver,
	})
	if !ok || next.IsZero() {
		return product.AbstractValue{}, false
	}
	return next, true
}

func (t *Transfer) versionedPath(point cfg.Point, path constraint.Path) constraint.Path {
	return domainpath.WithVersion(path, t.in.Graph, point)
}

func (t *Transfer) versionedStaticPathOfExpr(point cfg.Point, expr ast.Expr) (constraint.Path, bool) {
	path, ok := t.staticPathOfExpr(expr)
	if !ok {
		return constraint.Path{}, false
	}
	return t.versionedPath(point, path), true
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
