package transfer

import (
	"fmt"
	"os"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/compiler/check/abstract/literal"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// zprobeNarrow traces discriminant/cond-check narrowing when ZNARROW is set. The
// trace is appended to the file ZNARROW names (stderr is swallowed in the test
// sandbox), or to stderr when ZNARROW is the literal "1".
func zprobeNarrow(format string, args ...interface{}) {
	dst := os.Getenv("ZNARROW")
	if dst == "" {
		return
	}
	line := fmt.Sprintf("[ZNARROW] "+format+"\n", args...)
	if dst == "1" {
		fmt.Fprint(os.Stderr, line)
		return
	}
	f, err := os.OpenFile(dst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprint(f, line)
}

// projectStr renders an abstract value's projected type for the ZNARROW probe,
// tolerating the zero value (a declined narrowing) without projecting it.
func projectStr(av product.AbstractValue) string {
	if av.IsZero() {
		return "<zero>"
	}
	return fmt.Sprintf("%v", av.ProjectValue())
}

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

// NarrowEdge refines the out-state of guard point pred for the successor reached
// by the edge pred -> succ. When pred carries a branch guard, it narrows the
// guarded value in the returned Env by that guard (the guard on the TRUE edge, its
// negation on the FALSE edge) and records the per-edge path condition. A pred with
// no guard, an uninterpretable guard, or a value the guard cannot refine returns
// out unchanged.
//
// The guard is carried either by the branch node itself (g.Info(pred) is a
// *cfg.BranchInfo, for an intra-block read on a guarded edge) or by the then-exit /
// else-exit ScopeExit node the CFG copies the branch's CondVar/CondCheck onto (the
// real predecessor of a post-`if` merge, or the sole live predecessor after an
// early return in the other arm). Honoring the latter is what narrows a read after
// the merge or after an early-`return`/`error()` in a guarded block.
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
	zprobeNarrow("NarrowEdge pred=%v succ=%v taken=%v known=%v atExit=%v", pred, succ, taken, known, atExit)
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
			zprobeNarrow("  -> exitDiscriminantChain")
			return narrowed
		}
		if narrowed, applied := t.narrowExitAssignsPresent(g, pred, info, taken, out); applied {
			zprobeNarrow("  -> exitAssignsPresent")
			return narrowed
		}
	}
	return t.narrowEdgeInner(out, info, taken, atExit)
}

// narrowExitAssignsPresent narrows the `if not m[<lit k>] then m[<lit k>] = v end`
// fill-if-absent idiom on the then-arm exit. The guard's then-arm runs only when the
// key is ABSENT, so the ordinary exit narrowing would refine m[k] toward nil there;
// but the arm then ASSIGNS m[k], making the key present unconditionally on every path
// that reaches this exit. The other (else) arm reaches the merge with the key already
// present (the surviving falsy/nil edge proves it). So both predecessors of the merge
// hold m[k] present, and the merge-LUB keeps it present -- the canonical present-key
// fact, expressed by overlaying a record that pins the key to its non-optional value
// over the preserved map component.
//
// It applies only when the recovered guard tests a single literal-keyed index path
// (one field/string-index segment under the base symbol), this exit is the arm where
// the key was proven ABSENT (the edge's effective check is falsy/nil), and an
// assignment to that EXACT path dominates this exit within the arm. A guard on a
// different shape, an exit the assignment does not dominate, or a base the narrowing
// cannot refine returns applied=false so the ordinary exit narrowing runs (the
// surviving else arm still carries the present key, a precision loss not unsoundness).
func (t *Transfer) narrowExitAssignsPresent(g *cfg.Graph, pred cfg.Point, info *cfg.BranchInfo, taken bool, out flow.PointState) (flow.PointState, bool) {
	sym := t.condTestSymbol(info)
	if sym == 0 {
		return flow.PointState{}, false
	}
	segments := t.condTestSegments(info)
	if len(segments) != 1 {
		return flow.PointState{}, false
	}
	seg := segments[0]
	if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
		return flow.PointState{}, false
	}
	// This exit must be the arm on which the guard proved the key ABSENT: only there
	// does the then-arm assignment supply the presence the merge needs. The edge's
	// effective check is falsy/nil exactly on that arm (the `not m[k]` true edge / the
	// `m[k] == nil` true edge); the opposite arm already carries the present key.
	switch effectiveCheck(info.CondCheck.Kind, taken) {
	case cfg.CheckFalsy, cfg.CheckNil:
	default:
		return flow.PointState{}, false
	}
	if !t.armAssignsKeyPath(g, pred, sym, seg) {
		return flow.PointState{}, false
	}
	key := symKey(sym)
	baseAV, has := t.narrowBase(sym, out.Env[key], true)
	if !has {
		return flow.PointState{}, false
	}
	// The assignment proves the key present: refine the path by the POSITIVE presence
	// check (CheckTruthy), the same per-member field narrowing the surviving else edge
	// already applied, so this exit's value matches it and the merge keeps the key.
	narrowed, ok := narrowAtPath(baseAV, segments, cfg.CheckTruthy, "")
	if !ok {
		return flow.PointState{}, false
	}
	res := cloneForNarrow(out)
	res.Env[key] = narrowed
	zprobeNarrow("exitAssignsPresent sym=%d seg=%q narrowed=%v", sym, seg.Name, projectStr(narrowed))
	return res, true
}

// armAssignsKeyPath reports whether an assignment to the exact index/field path
// base[seg] (the symbol sym at the single field/string-index segment seg) dominates
// the arm exit. It walks the assignment nodes backward from the exit (the arm-local
// region terminating at this exit) and matches a TargetField / TargetIndex write whose
// base resolves to sym and whose single field/key equals seg. The walk stays within
// the arm by stopping at a point with no predecessors or already seen, so it never
// crosses out of the guarded arm into unrelated code.
func (t *Transfer) armAssignsKeyPath(g *cfg.Graph, exit cfg.Point, sym cfg.SymbolID, seg constraint.Segment) bool {
	seen := map[cfg.Point]bool{}
	frontier := append([]cfg.Point(nil), g.Predecessors(exit)...)
	for len(frontier) > 0 {
		var next []cfg.Point
		for _, p := range frontier {
			if seen[p] {
				continue
			}
			seen[p] = true
			if info := g.Assign(p); info != nil {
				for _, target := range info.Targets {
					if t.targetWritesKeyPath(target, sym, seg) {
						return true
					}
				}
			}
			// A branch node bounds the arm: the assignment that fills the key sits between
			// the guard branch and this exit, so the backward walk need not pass the
			// guard. Continuing past a branch would leave the arm; stop the walk there.
			if g.Branch(p) != nil {
				continue
			}
			next = append(next, g.Predecessors(p)...)
		}
		frontier = next
	}
	return false
}

// targetWritesKeyPath reports whether an assignment target writes the exact single
// key path base[seg]: a TargetField whose base symbol is sym and whose field chain is
// the single field seg, or a TargetIndex on sym whose static literal key equals seg.
// A multi-segment write, a different base, or a dynamic key does not match.
func (t *Transfer) targetWritesKeyPath(target cfg.AssignTarget, sym cfg.SymbolID, seg constraint.Segment) bool {
	switch target.Kind {
	case cfg.TargetField:
		if len(target.FieldPath) != 1 || target.FieldPath[0] != seg.Name {
			return false
		}
		return t.targetBaseSymbol(target) == sym
	case cfg.TargetIndex:
		name, ok := staticFieldName(target.Key)
		if !ok || name != seg.Name {
			return false
		}
		return t.targetBaseSymbol(target) == sym
	default:
		return false
	}
}

// targetBaseSymbol resolves the base symbol of a field/index assignment target,
// preferring the pre-resolved BaseSymbol and falling back to the base expression's
// root identifier so a target whose symbol the builder left unresolved still matches.
func (t *Transfer) targetBaseSymbol(target cfg.AssignTarget) cfg.SymbolID {
	if target.BaseSymbol != 0 {
		return target.BaseSymbol
	}
	if ident, ok := target.Base.(*ast.IdentExpr); ok {
		return t.symbolOf(ident)
	}
	return 0
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
	res := cloneForNarrow(out)
	if refined.Kind().IsNever() {
		res.Env[symKey(d.sym)] = product.Bottom()
	} else {
		res.Env[symKey(d.sym)] = product.FromType(refined)
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

func (t *Transfer) narrowEdgeInner(out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) flow.PointState {
	zprobeNarrow("edge taken=%v condVar=%q check=%v hasCond=%v cond=%T", taken, info.CondVar, info.CondCheck.Kind, info.Condition != nil, info.Condition)
	// A multi-return error guard narrows the correlated value siblings independently
	// of the tested error symbol's own refinement, so it composes with whichever base
	// narrower classifies the guard rather than short-circuiting the chain.
	out = t.narrowBySiblingNil(out, info, taken)
	// A relational comparison guard (`i <= n`, `i < #arr`) bounds a numeric value on
	// the edge it holds; the bound seeds the numeric component independently of the
	// guard's value narrowing, so it composes too.
	out = t.narrowNumericComparison(out, info, taken)
	if narrowed, applied := t.narrowByCompound(out, info, taken); applied {
		zprobeNarrow("  -> compound")
		return narrowed
	}
	if narrowed, applied := t.narrowByTypeCheck(out, info, taken); applied {
		zprobeNarrow("  -> typeCheck")
		return narrowed
	}
	if narrowed, applied := t.narrowByDiscriminant(out, info, taken); applied {
		zprobeNarrow("  -> discriminant")
		return narrowed
	}
	if narrowed, applied := t.narrowByTypedDiscriminant(out, info, taken); applied {
		zprobeNarrow("  -> typedDiscriminant")
		return narrowed
	}
	zprobeNarrow("  -> condCheck")
	return t.narrowByCondCheck(out, info, taken, atExit)
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
	if t.siblingNilByErr == nil {
		return out
	}
	sym := info.CondSymbol
	if sym == 0 {
		sym = t.condTestSymbol(info)
	}
	if sym == 0 {
		return out
	}
	bind, ok := t.siblingNilByErr[sym]
	zprobeNarrow("siblingNil.enter sym=%d hasBind=%v binds=%d", sym, ok, len(t.siblingNilByErr))
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
	res := cloneForNarrow(out)
	applied := false
	for _, vs := range bind.ValueSyms {
		if vs == 0 {
			continue
		}
		key := symKey(vs)
		av, has := res.Env[key]
		if !has || av.IsZero() {
			continue
		}
		res.Env[key] = product.NarrowPresent(av)
		applied = true
	}
	if !applied {
		return out
	}
	zprobeNarrow("siblingNil errSym=%d valueSyms=%v", sym, bind.ValueSyms)
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
//   - `A and B` falsy proves only that at least one operand is falsy, which narrows
//     neither, so it leaves out unchanged.
//
// A non-logical condition returns applied=false so the simple narrowers run.
func (t *Transfer) narrowByCompound(out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	if os.Getenv("ZZNOCMPD") != "" {
		return out, false
	}
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
	lhs, lkey, lok := t.operandNarrowsOneKey(out, logical.Lhs)
	if !lok {
		return out, false
	}
	rhs, rkey, rok := t.operandNarrowsOneKey(out, logical.Rhs)
	if !rok || lkey != rkey {
		return out, false
	}
	joined := product.Join(lhs.Env[lkey], rhs.Env[rkey])
	res := cloneForNarrow(out)
	res.Env[lkey] = joined
	zprobeNarrow("orUnionTrueEdge key=%q lhs=%v rhs=%v joined=%v", lkey, projectStr(lhs.Env[lkey]), projectStr(rhs.Env[rkey]), projectStr(joined))
	return res, true
}

// operandNarrowsOneKey narrows state by one OR-disjunct asserted truthy and reports
// the single Env key it refined. It returns ok=true only when the operand refines
// EXACTLY one key (the value it tests), so the caller can join two disjuncts that
// agree on which value they constrain; an operand that refines no key (an
// unclassifiable guard) or more than one key (a multi-value guard) is not a
// same-value disjunct and returns ok=false.
func (t *Transfer) operandNarrowsOneKey(state flow.PointState, expr ast.Expr) (flow.PointState, string, bool) {
	narrowed, ok := t.narrowOperand(state, operandGuard{expr: expr, truthy: true})
	if !ok {
		return flow.PointState{}, "", false
	}
	var changed string
	for k, av := range narrowed.Env {
		base, had := state.Env[k]
		if had && product.Domain.Equal(base, av) {
			continue
		}
		if changed != "" {
			return flow.PointState{}, "", false
		}
		changed = k
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
	narrowed := t.narrowEdgeInner(state, leaf, og.truthy, false)
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
	if root := condRootIdent(expr); root != nil {
		return t.symbolOf(root)
	}
	return 0
}

// condRootIdent returns the root identifier a leaf condition expression tests,
// descending the value-bearing operand of each recognized shape: a bare identifier,
// an attribute path's root, a relational comparison's first identifier-rooted side
// (a nil compare or a typeof call argument), and a `not` operand. A condition not
// rooted at an identifier returns nil.
func condRootIdent(expr ast.Expr) *ast.IdentExpr {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e
	case *ast.AttrGetExpr:
		return attrRootIdent(e)
	case *ast.UnaryNotOpExpr:
		return condRootIdent(e.Expr)
	case *ast.RelationalOpExpr:
		if ident := relandRootIdent(e.Lhs); ident != nil {
			return ident
		}
		return relandRootIdent(e.Rhs)
	case *ast.FuncCallExpr:
		if e.Method == "" && e.Receiver == nil && len(e.Args) == 1 {
			if fn, ok := e.Func.(*ast.IdentExpr); ok && fn.Value == "type" {
				return condRootIdent(e.Args[0])
			}
		}
	}
	return nil
}

// relandRootIdent resolves the root identifier of a relational operand: a bare
// identifier, an attribute path's root, or a typeof call's argument root. A literal
// or unrecognized operand returns nil so the other side is tried.
func relandRootIdent(expr ast.Expr) *ast.IdentExpr {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e
	case *ast.AttrGetExpr:
		return attrRootIdent(e)
	case *ast.FuncCallExpr:
		return condRootIdent(e)
	}
	return nil
}

// statesEqualForNarrow reports whether a narrowing left the env unchanged, so a
// no-op operand does not flip the compound's applied flag. The Cond half may carry
// the per-edge path condition without an env change; an env change is the signal a
// value was refined.
func statesEqualForNarrow(a, b flow.PointState) bool {
	if len(a.Env) != len(b.Env) {
		return false
	}
	for k, av := range a.Env {
		bv, ok := b.Env[k]
		if !ok || !product.Domain.Equal(av, bv) {
			return false
		}
	}
	return true
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
	key := symKey(g.sym)
	av, has := out.Env[key]
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
	res := cloneForNarrow(out)
	if refined.Kind().IsNever() {
		res.Env[key] = product.Bottom()
	} else {
		res.Env[key] = product.FromType(refined)
	}
	return res, true
}

// fieldExactlyType reports whether the field type and other denote the same variant
// (mutually subtype), the condition under which a `~=` edge can soundly exclude the
// member. A broader field type is not excluded.
func fieldExactlyType(ft, other typ.Type) bool {
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
	field, ok := staticFieldName(attr.Key)
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

// trackedIdentType resolves an identifier's value type from the live Env (the
// tracked flow value) so the typed discriminant compares against the same value the
// transfer carries. Returns nil for an identifier the flow does not track.
func (t *Transfer) trackedIdentType(out flow.PointState, ident *ast.IdentExpr) typ.Type {
	sym := t.symbolOf(ident)
	if sym == 0 {
		return nil
	}
	av, ok := out.Env[symKey(sym)]
	if !ok || av.IsZero() {
		return nil
	}
	return av.ProjectValue()
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
// carries. The CFG copies a branch's CondVar (the tested ROOT symbol) and CondCheck
// onto both arm-exit ScopeExit nodes (compiler/cfg/stmt.go IfStmt), so a post-`if`
// merge and a read after an early return in the other arm both reach a predecessor
// that holds the guard markers but is NOT a *cfg.BranchInfo. This reconstructs the
// equivalent BranchInfo so the same narrowing helpers fire on those preds.
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
	res := cloneForNarrow(out)
	for _, sym := range bind.NarrowSyms {
		if sym == 0 {
			continue
		}
		res.Env[symKey(sym)] = val
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
	sym := t.condTestSymbol(info)
	zprobeNarrow("condCheck.enter condVar=%q check=%v condSym=%d resolvedSym=%d atExit=%v", info.CondVar, info.CondCheck.Kind, info.CondSymbol, sym, atExit)
	if sym == 0 {
		return out
	}
	check := effectiveCheck(info.CondCheck.Kind, taken)
	if check == cfg.CheckNone {
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
		zprobeNarrow("condCheck.declineComparison sym=%d check=%v", sym, check)
		return out
	}
	segments := t.condTestSegments(info)
	key := symKey(sym)
	baseAV, has := t.narrowBase(sym, out.Env[key], atExit)

	cond := condForCheck(sym, segments, info.CondVar, check, info.CondCheck.TypeName)
	res := cloneForNarrow(out)
	if cond.HasConstraints() {
		res.Cond = constraint.Domain.Join(res.Cond, cond)
	}
	// A positive guard on a literal index path `arr[i]` (`if arr[i]`, `arr[i] ~= nil`)
	// proves the element at index i is present, so the container holds at least i
	// elements: record `#arr >= i` on this edge. A later read of `arr[i]` in the
	// guarded body reads it in-range and drops the soundly-optional element nil
	// through the existing length proof, recovering the non-optional element exactly
	// where the guard establishes presence. The merge-LUB rebuilds the unbounded
	// length where both edges meet, so the floor never survives past the guard.
	res = t.narrowIndexPresenceLength(res, sym, segments, check)
	if !has {
		// No tracked value to refine; the per-edge path condition still records the
		// guard for soundness.
		zprobeNarrow("condCheck.nohas sym=%d segs=%v envZero=%v", sym, segments, out.Env[key].IsZero())
		return res
	}
	narrowed, ok := narrowAtPath(baseAV, segments, check, info.CondCheck.TypeName)
	zprobeNarrow("condCheck sym=%d segs=%v check=%v base=%v narrowed=%v ok=%v", sym, segments, check, projectStr(baseAV), projectStr(narrowed), ok)
	if !ok {
		return res
	}
	res.Env[key] = narrowed
	return res
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
	if res.Num == nil {
		return res
	}
	num := res.Num.Clone()
	num.ApplyLenGeConst(constraint.PathKey(symKey(sym)), int64(seg.Index))
	res.Num = num
	zprobeNarrow("indexPresenceLen sym=%d idx=%d", sym, seg.Index)
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
	if out.Num == nil || info == nil {
		return out
	}
	rel, ok := info.Condition.(*ast.RelationalOpExpr)
	if !ok {
		return out
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
	idxKey := constraint.PathKey(symKey(idxSym))
	if c, ok := t.constInt(boundExpr); ok {
		res := cloneForNarrow(out)
		num := res.Num.Clone()
		applyConstComparison(num, idxKey, op, c)
		res.Num = num
		zprobeNarrow("numericCmp sym=%d op=%q c=%d", idxSym, op, c)
		return res
	}
	// `var <= #container` / `var < #container`: a symbolic length reference. Only the
	// upper-bound senses bound the index by the container length; a lower-bound sense
	// does not establish in-range presence and is left unseeded.
	if arrKey, off, ok := t.lengthBoundComparison(boundExpr, op); ok {
		res := cloneForNarrow(out)
		num := res.Num.Clone()
		num.ApplyLeLenOfWithOffset(idxKey, arrKey, off)
		res.Num = num
		zprobeNarrow("numericCmpLen sym=%d off=%d", idxSym, off)
		return res
	}
	return out
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

// applyConstComparison seeds the numeric bound a `var OP c` comparison proves. A
// strict bound is tightened to the inclusive integer neighbor (var < c is var <=
// c-1), since the numeric component carries inclusive integer intervals.
func applyConstComparison(num *numeric.State, idxKey constraint.PathKey, op string, c int64) {
	switch op {
	case "<":
		num.ApplyLeConst(idxKey, c-1)
	case "<=":
		num.ApplyLeConst(idxKey, c)
	case ">":
		num.ApplyGeConst(idxKey, c+1)
	case ">=":
		num.ApplyGeConst(idxKey, c)
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
		return attrSegments(info.Condition)
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return attrSegments(not.Expr)
		}
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			if seg := attrSegments(rel.Lhs); seg != nil {
				return seg
			}
			return attrSegments(rel.Rhs)
		}
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
		return t.typeofArgSegments(info.Condition)
	}
	return nil
}

// attrSegments returns the field segments of an attribute-access expression whose
// root is an identifier (x.f.g -> [.f, .g]). It returns nil for a bare identifier
// or any non-static-field path, so a bare-symbol guard narrows the symbol value.
func attrSegments(expr ast.Expr) []constraint.Segment {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return nil
	}
	return staticAttrPath(attr)
}

// staticAttrPath collects the field segments of a chain of attribute accesses
// rooted at an identifier, in path order. A non-identifier root or a non-static
// key yields nil.
func staticAttrPath(attr *ast.AttrGetExpr) []constraint.Segment {
	var rev []constraint.Segment
	current := attr
	for {
		name, ok := staticFieldName(current.Key)
		if !ok {
			return nil
		}
		rev = append(rev, constraint.Segment{Kind: constraint.SegmentField, Name: name})
		switch obj := current.Object.(type) {
		case *ast.IdentExpr:
			segs := make([]constraint.Segment, len(rev))
			for i := range rev {
				segs[i] = rev[len(rev)-1-i]
			}
			return segs
		case *ast.AttrGetExpr:
			current = obj
		default:
			return nil
		}
	}
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
		return attrSegments(call.Args[0])
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
		if os.Getenv("ZZNOINTERFP") != "" {
			ft, ok := fieldResolver.Field(t, field)
			if !ok || ft == nil {
				return t
			}
			if refined := refine(ft); refined != nil && refined.Kind().IsNever() {
				return typ.Never
			}
			return t
		}
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
			// Only a COMPLETE scalar proof (`type(x.f) == "string"|"number"|"boolean"`,
			// which filters the gradual `any` down to a primitive) pins a gradual field to
			// a concrete, assignable type. A truthy/presence guard leaves `any`, and a
			// `type(x.f) == "table"` guard proves only table-ness with gradual elements
			// (a Map/Record, not a primitive) — neither establishes a type a typed slot
			// can soundly consume. Leave the gradual base unchanged there so the field
			// reads `any` and unproven dynamic data stays non-assignable; field/index
			// access through the `any` still flows `any`, so precision is not lost.
			if !refined.Kind().IsPrimitive() {
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
		switch arg := call.Args[0].(type) {
		case *ast.IdentExpr:
			return t.symbolOf(arg)
		case *ast.AttrGetExpr:
			if os.Getenv("ZZNOTYPEOFFP") != "" {
				return 0
			}
			if root := attrRootIdent(arg); root != nil {
				return t.symbolOf(root)
			}
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
	key := symKey(g.sym)
	av := out.Env[key]
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
	zprobeNarrow("discriminant sym=%d field=%q lit=%v include=%v base=%s refined=%v", g.sym, g.field, g.literal, include, base, refinedStr(refined, base))
	if !ok {
		// An unchanged base carries no refinement; leave it to the plain join.
		return out, false
	}
	res := cloneForNarrow(out)
	if refined.Kind().IsNever() {
		// An impossible edge (the discriminant pins the value to the other variant):
		// the base narrows to the lattice Bottom so the edge's reads are unreachable,
		// and the merge-LUB recovers the live value where both edges meet.
		res.Env[key] = product.Bottom()
	} else {
		res.Env[key] = product.FromType(refined)
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
	key := symKey(g.sym)
	av := out.Env[key]
	if av.IsZero() {
		return out, false
	}
	root := av.ProjectValue()
	if root == nil {
		return out, false
	}
	union, ok := readFieldPath(root, g.prefix)
	if !ok || union == nil {
		return out, false
	}
	include := taken != g.negated
	refined, ok := narrowDiscriminantUnion(union, g.field, g.literal, include)
	zprobeNarrow("memberDiscriminant sym=%d prefix=%v field=%q lit=%v include=%v union=%s refined=%v", g.sym, g.prefix, g.field, g.literal, include, union, refinedStr(refined, union))
	if !ok {
		return out, false
	}
	rewritten := refineAtPath(root, g.prefix, func(typ.Type) typ.Type { return refined })
	if rewritten == nil || rewritten == root {
		return out, false
	}
	res := cloneForNarrow(out)
	if rewritten.Kind().IsNever() {
		res.Env[key] = product.Bottom()
	} else {
		res.Env[key] = product.FromType(rewritten)
	}
	return res, true
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
		if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
			return nil, false
		}
		if cur == nil {
			return nil, false
		}
		ft, ok := fieldResolver.Field(cur, seg.Name)
		if !ok || ft == nil {
			return nil, false
		}
		cur = ft
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
	if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
		return t
	}
	if len(segments) == 1 {
		return mapUnionField(t, seg.Name, refine, false)
	}
	return mapUnionField(t, seg.Name, func(ft typ.Type) typ.Type {
		return refineAtPath(ft, segments[1:], refine)
	}, false)
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
	field, ok := staticFieldName(attr.Key)
	if !ok {
		return discriminant{}, false
	}
	lit, ok := literalValue(value)
	if !ok {
		return discriminant{}, false
	}
	// A bare-symbol discriminant (`if x.tag == "a"`): the access object is the
	// identifier bound to the union directly.
	if baseIdent, isIdent := attr.Object.(*ast.IdentExpr); isIdent {
		sym := t.symbolOf(baseIdent)
		if sym == 0 {
			return discriminant{}, false
		}
		return discriminant{sym: sym, field: field, literal: lit, negated: negated}, true
	}
	// A member-access discriminant (`if receipt.output.kind == "rendered"`): the access
	// object is itself a field path whose root identifier binds the symbol and whose
	// segments are the prefix to the refined union value (receipt.output). The
	// discriminant field "kind" is read off the value at that prefix.
	objAttr, ok := attr.Object.(*ast.AttrGetExpr)
	if !ok {
		return discriminant{}, false
	}
	prefix := staticAttrPath(objAttr)
	if prefix == nil {
		return discriminant{}, false
	}
	root := attrRootIdent(objAttr)
	if root == nil {
		return discriminant{}, false
	}
	sym := t.symbolOf(root)
	if sym == 0 {
		return discriminant{}, false
	}
	return discriminant{sym: sym, prefix: prefix, field: field, literal: lit, negated: negated}, true
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
		if sym, segs, ok := t.attrPathSymbol(e); ok {
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

// pathSymbol resolves an identifier or field-path expression to its base symbol and
// field segments (x -> sym, []; x.f.g -> sym, [.f, .g]). A non-path expression
// reports false.
func (t *Transfer) pathSymbol(expr ast.Expr) (cfg.SymbolID, []constraint.Segment, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym := t.symbolOf(e); sym != 0 {
			return sym, nil, true
		}
	case *ast.AttrGetExpr:
		return t.attrPathSymbol(e)
	}
	return 0, nil, false
}

// attrPathSymbol resolves a field-path attribute access to its base symbol and the
// field segments under it. A non-identifier root or a dynamic key reports false.
func (t *Transfer) attrPathSymbol(attr *ast.AttrGetExpr) (cfg.SymbolID, []constraint.Segment, bool) {
	segs := staticAttrPath(attr)
	if segs == nil {
		return 0, nil, false
	}
	root := attrRootIdent(attr)
	if root == nil {
		return 0, nil, false
	}
	sym := t.symbolOf(root)
	if sym == 0 {
		return 0, nil, false
	}
	return sym, segs, true
}

// attrRootIdent returns the identifier at the root of an attribute-access chain
// (the x in x.f.g). A chain not rooted at an identifier returns nil.
func attrRootIdent(attr *ast.AttrGetExpr) *ast.IdentExpr {
	current := attr.Object
	for {
		switch obj := current.(type) {
		case *ast.IdentExpr:
			return obj
		case *ast.AttrGetExpr:
			current = obj.Object
		default:
			return nil
		}
	}
}

// narrowAssertPath narrows the asserted symbol (or field path under it) in out by
// the proven check and reports whether the refinement proves the continuation dead.
// It refines over the declared-type base (narrowBase) so a `local s: string?`
// parameter narrows its declared union, then writes the result back. A refinement
// that collapses the value to Bottom (an asserted comparison that cannot hold for
// the value's type, e.g. assert(x == nil) over a non-optional x) terminates the
// flow. An unrefined value leaves out unchanged.
func (t *Transfer) narrowAssertPath(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind) (dead bool) {
	key := symKey(sym)
	baseAV, has := t.narrowBase(sym, out.Env[key], false)
	if !has {
		return false
	}
	narrowed, ok := narrowAtPath(baseAV, segments, check, "")
	if !ok {
		return false
	}
	if product.Domain.Equal(narrowed, product.Bottom()) {
		// The asserted condition cannot hold for this value: the continuation is
		// unreachable, so the caller terminates the flow like error().
		return true
	}
	out.Env[key] = narrowed
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
	out.Env = narrowed.Env
	out.Cond = narrowed.Cond
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
type ParamNarrow struct {
	Param    int
	Segments []constraint.Segment
	Check    cfg.CondCheckKind
	// EqParam, when >= 0, makes this a parameter-EQUALITY effect: the body proves
	// Param == EqParam on every normal return (a `function eq(a, b) if a ~= b then
	// error() end` guard, or assert(a == b)). A caller narrows argument Param to the
	// intersection of its type with argument EqParam's type. Check/Segments are unused
	// for an equality effect. A non-equality effect leaves EqParam at -1.
	EqParam int
	// CondArg marks an effect on a parameter the body uses as a CONDITION (the cond of
	// `function maybeError(cond) if cond then error() end`): the body proves the
	// parameter's truthiness (Check, here CheckFalsy for the terminating then-arm) on
	// every normal return. A caller does not narrow the argument's own type (a boolean
	// stays a boolean) but narrows the value the argument condition TESTS: an argument
	// `x == nil` proven falsy narrows x to not-nil, exactly as the false edge of
	// `if x == nil` would. A non-condition effect leaves CondArg false.
	CondArg bool
	// CastType, when non-nil, makes this a parameter TYPE-CAST effect: the body asserts
	// the parameter IS this concrete type on every normal return (a `T(param)` cast that
	// dominates the exit, the transitive form of the call-site cast narrowing). A caller
	// narrows the matching argument's value directly to CastType. Check/EqParam are
	// unused for a cast effect.
	CastType typ.Type
}

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
	seen := make(map[string]bool)
	add := func(e ParamNarrow, ok bool) {
		if !ok {
			return
		}
		key := paramNarrowKey(e)
		if seen[key] {
			return
		}
		seen[key] = true
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
	return out
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
	if e, ok := t.paramEqEffect(info.Condition, relEqualLive(info.Condition, taken)); ok {
		return e, true
	}
	if e, ok := t.condArgEffect(info.Condition, taken); ok {
		return e, true
	}
	sym, segs, baseCheck, ok := t.assertCondition(info.Condition)
	if !ok {
		return ParamNarrow{}, false
	}
	check := effectiveCheck(baseCheck, taken)
	return t.toParamEffect(sym, segs, check)
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
	if os.Getenv("ZZNOCONDARG") != "" {
		return ParamNarrow{}, false
	}
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
	// An assert reads its condition truthy, so an `assert(a == b)` proves the equality.
	if e, ok := t.paramEqEffect(cond, relEqualLive(cond, true)); ok {
		return e, true
	}
	sym, segs, check, ok := t.assertCondition(cond)
	if !ok {
		return ParamNarrow{}, false
	}
	return t.toParamEffect(sym, segs, check)
}

// toParamEffect builds a ParamNarrow when sym is a parameter and check is a
// presence/truthy or absence/falsy refinement the body proves on every normal
// return. A not-nil/truthy wrapper (`function check(x) assert(x) end`) narrows the
// argument to non-nil at the call site; a nil/falsy wrapper (`function is_nil(x) if
// x ~= nil then error() end`) proves the argument nil, which both narrows the
// argument to nil and -- when the argument is a recorded multi-return error symbol --
// strips nil from its correlated value siblings (the call-site form of the (value,
// err) inverse correlation, applied by ApplyParamNarrows/applySiblingNilForErr).
func (t *Transfer) toParamEffect(sym cfg.SymbolID, segs []constraint.Segment, check cfg.CondCheckKind) (ParamNarrow, bool) {
	idx, isParam := t.paramBySym[sym]
	if !isParam {
		return ParamNarrow{}, false
	}
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil, cfg.CheckNil, cfg.CheckFalsy:
		return ParamNarrow{Param: idx, Segments: segs, Check: check, EqParam: -1}, true
	default:
		return ParamNarrow{}, false
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

// paramEqEffect recognizes a parameter-equality refinement an asserted/guarded
// condition proves on its live reading: `a == b` (truthy/live) or `a ~= b` negated to
// `a == b` on the surviving edge, where both operands are this function's parameters.
// It returns the effect narrowing parameter A to parameter B's type. equalLive
// reports whether the equality (==) holds on the chosen reading: true for an assert(a
// == b) or for the negated false edge of `if a ~= b then error()`. A condition that
// is not a parameter-to-parameter equality, or whose live reading is inequality,
// yields no effect (a caller cannot soundly intersect on `a ~= b`).
func (t *Transfer) paramEqEffect(cond ast.Expr, equalLive bool) (ParamNarrow, bool) {
	if !equalLive {
		return ParamNarrow{}, false
	}
	rel, ok := cond.(*ast.RelationalOpExpr)
	if !ok {
		return ParamNarrow{}, false
	}
	a, aOK := t.paramOperand(rel.Lhs)
	b, bOK := t.paramOperand(rel.Rhs)
	if !aOK || !bOK || a == b {
		return ParamNarrow{}, false
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
type DelegatedCall struct {
	Call      *ast.FuncCallExpr
	ArgParams []int
}

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
		any := false
		for i, arg := range info.Call.Args {
			argParams[i] = -1
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
			out = append(out, DelegatedCall{Call: info.Call, ArgParams: argParams})
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

// paramNarrowKey is a stable dedup key for a parameter effect (param index, field
// path, check), so two equivalent guard/assert forms record one effect.
func paramNarrowKey(e ParamNarrow) string {
	if e.EqParam >= 0 {
		return "p" + itoa(uint64(e.Param)) + "=p" + itoa(uint64(e.EqParam))
	}
	if e.CastType != nil {
		return "p" + itoa(uint64(e.Param)) + "::" + itoa(e.CastType.Hash())
	}
	key := "p" + itoa(uint64(e.Param)) + ":" + itoa(uint64(uint(e.Check)))
	if e.CondArg {
		key += "c"
	}
	for _, s := range e.Segments {
		key += "." + s.Name
	}
	return key
}

// ApplyParamNarrows refines the call arguments in out by the callee's parameter
// narrowing effects: an effect on parameter i narrows the i-th argument's value
// (along its field path) by the proven check. It applies only to an identifier or
// field-path argument whose root the flow tracks, so a wrapper call `check(y)`
// narrows `y` exactly as the wrapper's body proved its parameter. An argument the
// flow does not track, or an effect with no matching argument, is left unchanged.
func (t *Transfer) ApplyParamNarrows(out *flow.PointState, call *ast.FuncCallExpr, effects []ParamNarrow) {
	if call == nil || len(effects) == 0 {
		return
	}
	for _, e := range effects {
		if e.Param < 0 || e.Param >= len(call.Args) {
			continue
		}
		if e.EqParam >= 0 {
			t.applyParamEqNarrow(out, call, e)
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
		t.narrowAssertPath(out, argSym, segs, e.Check)
		// When the narrowed argument is a recorded multi-return error symbol proven nil
		// (a `test.is_nil(err)` wrapper that errors unless err is nil), its correlated
		// value siblings are non-nil, so strip nil from them — the same correlation a
		// branch `if err ~= nil then ... end` triggers, here proven by the wrapper call.
		if len(segs) == 0 && (e.Check == cfg.CheckNil || e.Check == cfg.CheckFalsy) {
			t.applySiblingNilForErr(out, argSym)
		}
	}
}

// applySiblingNilForErr strips nil from the value siblings correlated with err
// symbol, when err has just been proven nil. It is the call-site (param-narrow)
// counterpart of narrowBySiblingNil's branch-edge narrowing.
func (t *Transfer) applySiblingNilForErr(out *flow.PointState, errSym cfg.SymbolID) {
	if t.siblingNilByErr == nil || errSym == 0 {
		return
	}
	bind, ok := t.siblingNilByErr[errSym]
	if !ok {
		return
	}
	for _, vs := range bind.ValueSyms {
		if vs == 0 {
			continue
		}
		key := symKey(vs)
		av, has := out.Env[key]
		if !has || av.IsZero() {
			continue
		}
		out.Env[key] = product.NarrowPresent(av)
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
	narrowed := t.narrowEdgeInner(*out, leaf, wantTruthy, false)
	out.Env = narrowed.Env
	out.Cond = narrowed.Cond
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
	sym, segs, ok := t.pathSymbol(arg)
	if !ok || sym == 0 {
		return
	}
	key := symKey(sym)
	if len(segs) == 0 {
		out.Env[key] = product.FromType(castType)
		return
	}
	base, has := t.narrowBase(sym, out.Env[key], true)
	if !has {
		return
	}
	refined := refineAtPath(base.ProjectValue(), segs, func(typ.Type) typ.Type { return castType })
	if refined == nil {
		return
	}
	out.Env[key] = product.FromType(refined)
}

// applyParamEqNarrow applies a parameter-equality effect at the call site: argument
// e.Param is narrowed to the intersection of its tracked value with argument
// e.EqParam's value, the value-domain form of the equality the callee proved (the two
// parameters are equal on every normal return, so their argument types must overlap).
// An argument the flow does not track, or an empty intersection, leaves the value
// unchanged (sound: a precision loss, never a fabricated narrowing).
func (t *Transfer) applyParamEqNarrow(out *flow.PointState, call *ast.FuncCallExpr, e ParamNarrow) {
	if e.EqParam < 0 || e.EqParam >= len(call.Args) {
		return
	}
	targetSym, segs, ok := t.pathSymbol(call.Args[e.Param])
	if !ok || len(segs) != 0 {
		return
	}
	targetAV, has := t.narrowBase(targetSym, out.Env[symKey(targetSym)], false)
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
		out.Env[symKey(targetSym)] = product.Bottom()
		return
	}
	out.Env[symKey(targetSym)] = product.FromType(refined)
}

// cloneForNarrow copies the PointState's mutable Env so the narrowing never
// mutates the shared predecessor state; Cond and Num are immutable/shared values
// the narrowing only reads or replaces wholesale.
func cloneForNarrow(ps flow.PointState) flow.PointState {
	env := make(map[string]product.AbstractValue, len(ps.Env))
	for k, v := range ps.Env {
		env[k] = v
	}
	return flow.PointState{Env: env, Cond: ps.Cond, Num: ps.Num}
}
