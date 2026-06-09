package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/literal"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

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
	res := flow.ClonePointStateForEdgeFactEffect(out)
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
	res := flow.ClonePointStateForEdgeFactEffect(out)
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
	otherType := t.trackedExprType(out, value)
	if !isDiscriminatingType(otherType) {
		return discriminant{}, nil, false
	}
	return discriminant{sym: sym, field: field, negated: negated}, otherType, true
}

func (t *Transfer) trackedExprType(out flow.PointState, expr ast.Expr) typ.Type {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		return t.trackedIdentType(out, ident)
	}
	path, ok := t.staticPathOfExpr(expr)
	if !ok || path.IsEmpty() {
		return nil
	}
	var declared typ.Type
	if path.Symbol != 0 && t.declaredTypes != nil {
		declared = t.declaredTypes[path.Symbol]
	}
	evidence := flow.PointFactsOf(out).PathTypeEvidence(path, declared)
	if evidence.Declared.State == flow.StateResolved {
		declared = evidence.Declared.Type
	} else {
		declared = nil
	}
	if evidence.Current.State == flow.StateResolved {
		projected := evidence.Current.Type
		if isDiscriminatingType(projected) {
			return projected
		}
		if isDiscriminatingType(declared) && narrow.TypesOverlap(projected, declared) {
			return declared
		}
		return projected
	}
	if !typ.IsAbsentOrUnknown(declared) {
		return declared
	}
	return nil
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
	res := flow.ClonePointStateForEdgeFactEffect(out)
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
	lens := structuralPath(g.prefix)
	union, ok := lens.Read(root)
	if !ok || union == nil {
		return out, false
	}
	include := taken != g.negated
	refined, ok := narrowDiscriminantUnion(union, g.field, g.literal, include)
	if !ok {
		return out, false
	}
	rewritten := lens.Refine(root, func(typ.Type) typ.Type { return refined })
	if rewritten == nil || rewritten == root {
		return out, false
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
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
	lens := structuralPath(prefix)
	if !av.IsZero() {
		if lens.HasMultiUnion(av.ProjectValue()) {
			return av, true
		}
	}
	if declared, ok := t.declaredTypes[sym]; ok && declared != nil && !typ.IsAbsentOrUnknown(declared) {
		if lens.HasMultiUnion(declared) {
			return product.FromType(declared), true
		}
	}
	if !av.IsZero() {
		return av, true
	}
	return t.narrowBase(sym, av, false)
}

// narrowDiscriminantUnion refines a discriminated union by a literal on its tag field:
// it keeps the matching variant on the include edge (narrow.ByFieldLiteral) or excludes
// it on the exclude edge (narrow.ExcludeByFieldLiteral). A genuine literal-discriminant
// guard reads base.field, which presupposes base is non-nil on BOTH edges (nil.field
// errors at runtime before the comparison), so an optional/nil wrapper an array-index or
// captured-optional source carries is stripped before refinement, leaving only the live
// record variants the exclude edge must also drop. A plain `field == value` on a broad
// scalar field (not a discriminant) is left un-stripped so it remains a no-op and
// never rewrites a sibling guard's field refinement. ok=false reports an
// unchanged base.
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
