package value

import (
	"os"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// foldDbg gates the self-embedding/union-growth convergence diagnostics.
var foldDbg = os.Getenv("FOLDDBG") != ""

// NormalizeFactType canonicalizes one type before it is stored in an
// interprocedural fact slot.
func NormalizeFactType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return collapseSameFamilyRecursiveUnions(fn)
	}
	t = typ.PruneSoftUnionMembers(t)
	if !typ.ContainsRecursive(t) {
		t = typ.CoalesceProductUnion(t)
	}
	t = CollapseSequenceUnion(t, typ.JoinReturnSlot)
	t = CollapseTableTopEvidence(t)
	t = typ.PruneSoftUnionMembers(t)
	t = collapseSameFamilyRecursiveUnions(t)
	if typ.ContainsRecursive(t) {
		return t
	}
	return typ.CoalesceProductUnion(t)
}

// collapseSameFamilyRecursiveUnions deeply rewrites unions so members of one
// recursive product family collapse to a single canonical representative. A
// recursive class summary is observed with fresh recursive ids each fixpoint
// iteration; left as distinct union members (e.g. inside a method's self
// parameter) the fixpoint compares fresh ids forever and never converges. The
// collapse is by SameConvergedFact (canonical-family identity), so genuinely
// distinct families are preserved.
func collapseSameFamilyRecursiveUnions(t typ.Type) typ.Type {
	if t == nil || !typ.ContainsRecursive(t) {
		return t
	}
	return newRecursiveUnionCollapse().rewrite(t, typ.NewGuard())
}

type recursiveUnionCollapse struct {
	seen map[typ.Type]typ.Type
}

func newRecursiveUnionCollapse() *recursiveUnionCollapse {
	return &recursiveUnionCollapse{seen: make(map[typ.Type]typ.Type)}
}

func (c *recursiveUnionCollapse) rewrite(t typ.Type, guard internal.RecursionGuard) typ.Type {
	if t == nil || !typ.ContainsRecursive(t) {
		return t
	}
	if out, ok := c.seen[t]; ok {
		return out
	}
	next, ok := guard.Enter(t)
	if !ok {
		return t
	}

	switch v := t.(type) {
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		for i, m := range v.Members {
			members[i] = c.rewrite(m, next)
			if !typ.SameNode(members[i], m) {
				changed = true
			}
		}
		collapsed := collapseRecursiveUnionMembers(members)
		if !changed && len(collapsed) == len(v.Members) {
			return t
		}
		var out typ.Type
		if len(collapsed) == 1 {
			out = collapsed[0]
		} else {
			out = typ.NewUnion(collapsed...)
		}
		c.seen[t] = out
		return out
	case *typ.Optional:
		inner := c.rewrite(v.Inner, next)
		if typ.SameNode(inner, v.Inner) {
			return t
		}
		out := typ.NewOptional(inner)
		c.seen[t] = out
		return out
	case *typ.Function:
		return c.rewriteFunction(t, v, next)
	case *typ.Record:
		return c.rewriteRecord(t, v, next)
	case *typ.Array:
		elem := c.rewrite(v.Element, next)
		if typ.SameNode(elem, v.Element) {
			return t
		}
		out := typ.NewArray(elem)
		c.seen[t] = out
		return out
	case *typ.Map:
		key := c.rewrite(v.Key, next)
		val := c.rewrite(v.Value, next)
		if typ.SameNode(key, v.Key) && typ.SameNode(val, v.Value) {
			return t
		}
		out := typ.NewMap(key, val)
		c.seen[t] = out
		return out
	case *typ.Tuple:
		elements := make([]typ.Type, len(v.Elements))
		changed := false
		for i, e := range v.Elements {
			elements[i] = c.rewrite(e, next)
			if !typ.SameNode(elements[i], e) {
				changed = true
			}
		}
		if !changed {
			return t
		}
		out := typ.NewTuple(elements...)
		c.seen[t] = out
		return out
	case *typ.Recursive:
		if v.Body == nil {
			return t
		}
		// Tie the cycle off at the original node so a self-edge is left untouched;
		// only an actual member collapse below changes the body.
		c.seen[t] = t
		body := c.rewrite(v.Body, next)
		if typ.SameNode(body, v.Body) {
			return t
		}
		placeholder := typ.NewRecursivePlaceholder(v.Name)
		placeholder.SetBody(rewriteRecursiveSelf(body, v, placeholder))
		c.seen[t] = placeholder
		return placeholder
	default:
		return t
	}
}

func (c *recursiveUnionCollapse) rewriteFunction(orig typ.Type, fn *typ.Function, guard internal.RecursionGuard) typ.Type {
	builder := typ.Func().ReserveParams(len(fn.Params))
	changed := false
	for _, tp := range fn.TypeParams {
		constraint := c.rewrite(tp.Constraint, guard)
		if !typ.SameNode(constraint, tp.Constraint) {
			changed = true
		}
		builder.TypeParam(tp.Name, constraint)
	}
	for _, p := range fn.Params {
		paramType := c.rewrite(p.Type, guard)
		if !typ.SameNode(paramType, p.Type) {
			changed = true
		}
		if p.Optional {
			builder.OptParam(p.Name, paramType)
		} else {
			builder.Param(p.Name, paramType)
		}
	}
	if fn.Variadic != nil {
		variadic := c.rewrite(fn.Variadic, guard)
		if !typ.SameNode(variadic, fn.Variadic) {
			changed = true
		}
		builder.Variadic(variadic)
	}
	if len(fn.Returns) > 0 {
		returns := make([]typ.Type, len(fn.Returns))
		for i, r := range fn.Returns {
			returns[i] = c.rewrite(r, guard)
			if !typ.SameNode(returns[i], r) {
				changed = true
			}
		}
		builder.Returns(returns...)
	}
	if !changed {
		return orig
	}
	builder.Effects(fn.Effects)
	builder.Spec(fn.Spec)
	builder.WithRefinement(fn.Refinement)
	out := builder.Build()
	c.seen[orig] = out
	return out
}

func (c *recursiveUnionCollapse) rewriteRecord(orig typ.Type, rec *typ.Record, guard internal.RecursionGuard) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	changed := false
	if rec.Metatable != nil {
		mt := c.rewrite(rec.Metatable, guard)
		if !typ.SameNode(mt, rec.Metatable) {
			changed = true
		}
		builder.Metatable(mt)
	}
	if rec.HasMapComponent() {
		key := c.rewrite(rec.MapKey, guard)
		val := c.rewrite(rec.MapValue, guard)
		if !typ.SameNode(key, rec.MapKey) || !typ.SameNode(val, rec.MapValue) {
			changed = true
		}
		builder.MapComponent(key, val)
	}
	for _, field := range rec.Fields {
		f := field
		f.Type = c.rewrite(field.Type, guard)
		if !typ.SameNode(f.Type, field.Type) {
			changed = true
		}
		addRecordField(builder, f)
	}
	if !changed {
		return orig
	}
	out := builder.Build()
	c.seen[orig] = out
	return out
}

// collapseRecursiveUnionMembers drops union members already represented by a
// recursive family member: a duplicate observation of the same converged family,
// or a finite unfolding the recursive family already covers. A recursive class is
// observed both as the family (mu) and as finite unfoldings of it within one
// union (e.g. a method's self parameter as Inferred#N | {fields...}); left
// distinct, the fixpoint compares fresh recursive ids forever. Distinct families
// and members no recursive member covers are retained.
func collapseRecursiveUnionMembers(members []typ.Type) []typ.Type {
	// First pass keeps one representative per recursive family. Order-independence
	// matters: a finite unfolding may appear before or after the recursive family
	// member, so recursive members are admitted first, then finite members covered
	// by an admitted recursive family are dropped in the second pass.
	recursives := make([]typ.Type, 0, len(members))
	for _, member := range members {
		if !typ.ContainsRecursive(member) {
			continue
		}
		if !sameConvergedFamilyKept(recursives, member) {
			recursives = append(recursives, member)
		}
	}
	out := make([]typ.Type, 0, len(members))
	for _, member := range members {
		if typ.ContainsRecursive(member) {
			if sameConvergedFamilyKept(out, member) {
				continue
			}
			out = append(out, member)
			continue
		}
		if recursiveFamilyCovers(recursives, member) {
			continue
		}
		out = append(out, member)
	}
	return out
}

func sameConvergedFamilyKept(kept []typ.Type, member typ.Type) bool {
	for _, k := range kept {
		if typ.ContainsRecursive(k) && SameConvergedFact(k, member) {
			return true
		}
	}
	return false
}

func recursiveFamilyCovers(recursives []typ.Type, member typ.Type) bool {
	for _, rec := range recursives {
		if RecursiveEvidenceCovers(rec, member) {
			return true
		}
	}
	return false
}

// WidenForConvergence applies the finite-height approximation needed for
// higher-order structural growth.
func WidenForConvergence(t typ.Type) typ.Type {
	return newConvergenceWidenState().widen(t)
}

// ConvergenceWidening carries memoized value-domain widening state across a
// single product merge.
type ConvergenceWidening struct {
	state *convergenceWidenState
}

// NewConvergenceWidening creates a reusable widening context for one
// interprocedural product merge.
func NewConvergenceWidening() *ConvergenceWidening {
	return &ConvergenceWidening{state: newConvergenceWidenState()}
}

// Type widens one value type using the context's shared growth-risk memo.
func (w *ConvergenceWidening) Type(t typ.Type) typ.Type {
	if w == nil || w.state == nil {
		return WidenForConvergence(t)
	}
	return w.state.widen(t)
}

// Function widens one function type using the context's shared growth-risk memo.
func (w *ConvergenceWidening) Function(fn *typ.Function) *typ.Function {
	if w == nil || w.state == nil {
		return WidenFunctionForConvergence(fn)
	}
	return w.state.widenFunction(fn)
}

// Merge merges two value types using the context's shared convergence memo.
func (w *ConvergenceWidening) Merge(existing, candidate typ.Type) typ.Type {
	if w == nil || w.state == nil {
		return MergeForConvergence(existing, candidate)
	}
	return w.state.merge(existing, candidate)
}

// HasHigherOrderGrowthRisk reports growth risk using the context's shared scan
// memo.
func (w *ConvergenceWidening) HasHigherOrderGrowthRisk(t typ.Type) bool {
	if w == nil || w.state == nil || w.state.growth == nil {
		return HasHigherOrderGrowthRisk(t)
	}
	return w.state.growth.hasHigherOrderGrowthRisk(t, typ.NewGuard())
}

type convergenceWidenState struct {
	growth               *growthScanState
	seen                 map[typ.Type]typ.Type
	active               map[typ.Type]bool
	activeRecursiveJoins map[recursiveProductJoinKey]*typ.Recursive
}

type recursiveProductJoinKey struct {
	left  uint64
	right uint64
}

func newConvergenceWidenState() *convergenceWidenState {
	return &convergenceWidenState{
		growth: newGrowthScanState(),
		seen:   make(map[typ.Type]typ.Type),
		active: make(map[typ.Type]bool),
	}
}

func (s *convergenceWidenState) widen(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if !typ.ContainsRecursive(t) {
		t = typ.CoalesceProductUnion(t)
	}
	if out, ok := s.seen[t]; ok {
		return out
	}
	if s.active[t] {
		return t
	}
	s.active[t] = true
	defer delete(s.active, t)

	var out typ.Type
	if fn := unwrap.Function(t); fn != nil {
		out = s.widenFunction(fn)
	} else if !s.growth.hasHigherOrderGrowthRisk(t, typ.NewGuard()) {
		out = t
	} else if folded, ok := FoldSelfEmbedding(t, t); ok {
		out = folded
	} else {
		out = subtype.WidenForInference(t)
	}
	s.seen[t] = out
	return out
}

// WidenFunctionForConvergence applies convergence widening to a function type.
func WidenFunctionForConvergence(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	return newConvergenceWidenState().widenFunction(fn)
}

func (s *convergenceWidenState) widenFunction(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	if out, ok := s.seen[fn]; ok {
		if widened, ok := out.(*typ.Function); ok {
			return widened
		}
	}
	if s.active[fn] {
		return fn
	}
	s.active[fn] = true
	defer delete(s.active, fn)

	widened := s.widenFunctionSlots(fn)
	if !s.growth.hasHigherOrderGrowthRisk(widened, typ.NewGuard()) {
		s.seen[fn] = widened
		return widened
	}
	if folded, ok := FoldSelfEmbedding(widened, widened); ok {
		if out, ok := folded.(*typ.Function); ok {
			s.seen[fn] = out
			return out
		}
	}
	if out, ok := subtype.WidenForInference(widened).(*typ.Function); ok {
		s.seen[fn] = out
		return out
	}
	s.seen[fn] = widened
	return widened
}

func (s *convergenceWidenState) widenFunctionSlots(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		constraint := s.widen(tp.Constraint)
		if !typ.SameNode(constraint, tp.Constraint) {
			changed = true
		}
		builder.TypeParam(tp.Name, constraint)
	}
	for _, p := range fn.Params {
		paramType := s.widen(p.Type)
		if !typ.SameNode(paramType, p.Type) {
			changed = true
		}
		if p.Optional {
			builder.OptParam(p.Name, paramType)
		} else {
			builder.Param(p.Name, paramType)
		}
	}
	if fn.Variadic != nil {
		variadic := s.widen(fn.Variadic)
		if !typ.SameNode(variadic, fn.Variadic) {
			changed = true
		}
		builder.Variadic(variadic)
	}
	returns := make([]typ.Type, len(fn.Returns))
	for i, ret := range fn.Returns {
		returns[i] = s.widen(ret)
		if !typ.SameNode(returns[i], ret) {
			changed = true
		}
	}
	if len(returns) > 0 {
		builder.Returns(returns...)
	}
	builder.Effects(fn.Effects)
	builder.Spec(fn.Spec)
	builder.WithRefinement(fn.Refinement)
	if !changed {
		return fn
	}
	return builder.Build()
}

// JoinPrecise merges non-function value facts inside one analysis iteration.
func JoinPrecise(existing, candidate typ.Type) typ.Type {
	existing = NormalizeFactType(existing)
	candidate = NormalizeFactType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if upper, ok := FunctionEvidenceUpperBound(existing, candidate); ok {
		return upper
	}
	if typ.ContainsRecursive(existing) || typ.ContainsRecursive(candidate) {
		return newConvergenceWidenState().merge(existing, candidate)
	}
	return typejoin.Types(existing, candidate)
}

// MergeForConvergence merges non-function value facts at a fixpoint boundary.
func MergeForConvergence(existing, candidate typ.Type) typ.Type {
	return newConvergenceWidenState().merge(existing, candidate)
}

func (s *convergenceWidenState) merge(existing, candidate typ.Type) typ.Type {
	existing = NormalizeFactType(existing)
	candidate = NormalizeFactType(candidate)
	if existing == nil {
		return s.widen(candidate)
	}
	if candidate == nil {
		return s.widen(existing)
	}
	if sameValueNodeOrAcyclicEqual(existing, candidate) {
		return existing
	}
	if unwrap.IsNilType(existing) && !unwrap.IsNilType(candidate) {
		return candidate
	}
	if unwrap.IsNilType(candidate) && !unwrap.IsNilType(existing) {
		return existing
	}
	if typ.IsAny(existing) || typ.IsAny(candidate) {
		return typ.Any
	}
	if typ.IsUnknown(existing) {
		return candidate
	}
	if typ.IsUnknown(candidate) {
		return existing
	}
	if joined, ok := s.joinRecursiveProducts(existing, candidate); ok {
		return joined
	}
	if upper, ok := convergenceUpperBound(existing, candidate); ok {
		return upper
	}
	existing = s.widen(existing)
	candidate = s.widen(candidate)
	if sameValueNodeOrAcyclicEqual(existing, candidate) {
		return existing
	}
	if unwrap.IsNilType(existing) && !unwrap.IsNilType(candidate) {
		return candidate
	}
	if unwrap.IsNilType(candidate) && !unwrap.IsNilType(existing) {
		return existing
	}
	if upper, ok := convergenceUpperBound(existing, candidate); ok {
		return upper
	}
	if typ.IsAny(existing) || typ.IsAny(candidate) {
		return typ.Any
	}
	if typ.IsUnknown(existing) {
		return candidate
	}
	if typ.IsUnknown(candidate) {
		return existing
	}
	if joined, ok := JoinStructuralShape(existing, candidate, s.merge); ok {
		return s.widen(joined)
	}
	if joined, ok := JoinStructuralUnionShape(existing, candidate, s.merge); ok {
		return s.widen(joined)
	}
	if joined, ok := s.joinRecursiveProducts(existing, candidate); ok {
		return joined
	}
	if typ.ContainsRecursive(existing) || typ.ContainsRecursive(candidate) {
		return s.joinRecursiveConvergenceUnion(existing, candidate)
	}
	if refines, _ := RefinesFalsyMapKey(candidate, existing); refines {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	return typejoin.Types(existing, candidate)
}

func (s *convergenceWidenState) joinRecursiveConvergenceUnion(existing, candidate typ.Type) typ.Type {
	members := make([]typ.Type, 0, 4)
	members = appendConvergenceUnionMembers(members, existing)
	members = appendConvergenceUnionMembers(members, candidate)
	if len(members) == 0 {
		return typ.Never
	}
	members = s.reduceConvergenceUnionMembers(members)
	if len(members) == 0 {
		return typ.Never
	}
	if len(members) == 1 {
		return typ.PruneSoftUnionMembers(members[0])
	}
	return typ.PruneSoftUnionMembers(typ.PruneLessPreciseRefinableUnionMembers(typ.NewUnion(members...)))
}

func appendConvergenceUnionMembers(out []typ.Type, t typ.Type) []typ.Type {
	if t == nil {
		return out
	}
	if u, ok := unwrap.Alias(t).(*typ.Union); ok {
		for _, member := range u.Members {
			out = appendConvergenceUnionMembers(out, member)
		}
		return out
	}
	return append(out, t)
}

func (s *convergenceWidenState) reduceConvergenceUnionMembers(members []typ.Type) []typ.Type {
	if len(members) < 2 {
		return members
	}
	out := make([]typ.Type, 0, len(members))
	for _, member := range members {
		out = s.admitConvergenceUnionMember(out, member)
	}
	return out
}

func (s *convergenceWidenState) admitConvergenceUnionMember(members []typ.Type, candidate typ.Type) []typ.Type {
	if candidate == nil || typ.IsAbsentOrUnknown(candidate) {
		return members
	}
	if foldDbg && len(members) >= 3 {
		println("FOLDDBG admit union members=", len(members), "rec(cand)=", ContainsRecursiveDbg(candidate), "cand=", DbgString(candidate))
	}
	for i, existing := range members {
		if SameConvergedFact(existing, candidate) {
			return members
		}
		if joined, ok := s.joinRecursiveProducts(existing, candidate); ok {
			members[i] = joined
			return members
		}
		// Prefer the precise convergence bounds first (top-array absorption,
		// existing-recursive coverage, record extension). They keep the simplest
		// finite representative; the knot-closing fold is the fallback that ties off
		// a growing self-embedding tower no simpler bound already absorbs.
		if upper, ok := convergenceUpperBound(existing, candidate); ok {
			members[i] = upper
			return members
		}
		// Close a self-embedding tower into a recursive family: when one member is
		// a deeper unfolding of the other's family, fold it into a recursive upper
		// bound so progressive unfoldings cannot accumulate as distinct members and
		// grow the union without bound.
		if folded, ok := s.foldSelfEmbeddingUpperBound(existing, candidate); ok {
			members[i] = folded
			return members
		}
		if joined, ok := JoinStructuralShape(existing, candidate, s.merge); ok {
			members[i] = s.widen(joined)
			return members
		}
		if joined, ok := JoinStructuralUnionShape(existing, candidate, s.merge); ok {
			members[i] = s.widen(joined)
			return members
		}
	}
	return append(members, candidate)
}

// foldSelfEmbeddingUpperBound folds a self-embedding tower of two union members
// into a recursive family. When one member is a below-root self-embedding
// unfolding of the other, FoldSelfEmbedding ties the knot into a mu node; the
// result is admitted only when it is a verified upper bound of both members, so
// replacing the growing union with the canonical recursive representative loses no
// values and gives the fixpoint a finite representative.
func (s *convergenceWidenState) foldSelfEmbeddingUpperBound(existing, candidate typ.Type) (typ.Type, bool) {
	for _, pair := range [2][2]typ.Type{{existing, candidate}, {candidate, existing}} {
		folded, ok := FoldSelfEmbedding(pair[0], pair[1])
		if foldDbg {
			println("FOLDDBG fold try foldOK=", ok, "anchor=", DbgString(pair[0]), "obs=", DbgString(pair[1]))
		}
		if !ok {
			continue
		}
		folded = s.widen(folded)
		if folded == nil {
			continue
		}
		coversBoth := Covers(folded, existing) && Covers(folded, candidate)
		if foldDbg {
			println("FOLDDBG folded coversBoth=", coversBoth, "folded=", DbgString(folded))
		}
		if coversBoth {
			return folded, true
		}
	}
	return nil, false
}

func (s *convergenceWidenState) joinRecursiveProducts(a, b typ.Type) (typ.Type, bool) {
	ar, okA := unwrap.Alias(a).(*typ.Recursive)
	br, okB := unwrap.Alias(b).(*typ.Recursive)
	if !okA || !okB || ar == nil || br == nil || ar.Body == nil || br.Body == nil {
		return nil, false
	}
	if !sameRecursiveProductFamily(ar, br) {
		return nil, false
	}
	if recursiveProductsEquivalent(ar, br) {
		return ar, true
	}
	if RecursiveEvidenceCovers(ar, br) && RecursiveEvidenceCovers(br, ar) {
		return ar, true
	}
	key := recursiveProductJoinKeyFor(ar, br)
	if active := s.activeRecursiveJoin(key); active != nil {
		return active, true
	}
	name := ar.Name
	if name == "" {
		name = br.Name
	}
	merged := typ.NewRecursivePlaceholder(name)
	s.enterRecursiveJoin(key, merged)
	defer s.leaveRecursiveJoin(key, merged)
	left := rewriteRecursiveSelf(ar.Body, ar, merged)
	right := rewriteRecursiveSelf(br.Body, br, merged)
	body, ok := JoinStructuralShape(left, right, s.merge)
	if !ok {
		return nil, false
	}
	merged.SetBody(body)
	return merged, true
}

func recursiveProductJoinKeyFor(a, b *typ.Recursive) recursiveProductJoinKey {
	if a == nil || b == nil {
		return recursiveProductJoinKey{}
	}
	ah := typ.ProductFamilyHash(a)
	bh := typ.ProductFamilyHash(b)
	if ah > bh {
		ah, bh = bh, ah
	}
	return recursiveProductJoinKey{left: ah, right: bh}
}

func (s *convergenceWidenState) activeRecursiveJoin(key recursiveProductJoinKey) *typ.Recursive {
	if s == nil || s.activeRecursiveJoins == nil {
		return nil
	}
	return s.activeRecursiveJoins[key]
}

func (s *convergenceWidenState) enterRecursiveJoin(key recursiveProductJoinKey, merged *typ.Recursive) {
	if s == nil || merged == nil {
		return
	}
	if s.activeRecursiveJoins == nil {
		s.activeRecursiveJoins = make(map[recursiveProductJoinKey]*typ.Recursive)
	}
	s.activeRecursiveJoins[key] = merged
}

func (s *convergenceWidenState) leaveRecursiveJoin(key recursiveProductJoinKey, merged *typ.Recursive) {
	if s == nil || s.activeRecursiveJoins == nil {
		return
	}
	if s.activeRecursiveJoins[key] == merged {
		delete(s.activeRecursiveJoins, key)
	}
}

func sameRecursiveProductFamily(a, b *typ.Recursive) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != "" && b.Name != "" {
		return a.Name == b.Name
	}
	return typ.ProductFamilyHash(a) == typ.ProductFamilyHash(b)
}

func recursiveProductsEquivalent(a, b typ.Type) bool {
	return SameConvergedFact(a, b)
}

func rewriteRecursiveSelf(t typ.Type, from, to *typ.Recursive) typ.Type {
	if t == nil || from == nil || to == nil || from == to {
		return t
	}
	return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		if typ.IsRecursiveRef(node, from) {
			return to, true
		}
		return nil, false
	})
}

func convergenceUpperBound(existing, candidate typ.Type) (typ.Type, bool) {
	if upper, ok := FunctionEvidenceUpperBound(existing, candidate); ok {
		return upper, true
	}
	if ElidesOptional(candidate, existing) {
		return candidate, true
	}
	if ElidesOptional(existing, candidate) {
		return existing, true
	}
	if upper, ok := PrecisionEvidenceUpperBound(existing, candidate); ok {
		return upper, true
	}
	if upper, ok := DynamicMapUpperBound(existing, candidate); ok {
		return upper, true
	}
	if upper, ok := SelfEmbeddingUpperBound(existing, candidate); ok {
		if foldDbg {
			println("FOLDDBG convergenceUpperBound via SelfEmbeddingUpperBound")
		}
		return upper, true
	}
	if upper, ok := RecordExtensionUpperBound(existing, candidate); ok {
		if foldDbg {
			println("FOLDDBG convergenceUpperBound via RecordExtensionUpperBound")
		}
		return upper, true
	}
	if upper, ok := RecursiveUnionUpperBound(existing, candidate); ok {
		if foldDbg {
			println("FOLDDBG convergenceUpperBound via RecursiveUnionUpperBound")
		}
		return upper, true
	}
	return nil, false
}

// FunctionEvidenceUpperBound admits a solved function projection over the
// uninformative function seed used while a function literal's body facts are
// still converging. This is a value-domain fact law, not general source-level
// function subtyping: callers use it only while merging abstract evidence.
func FunctionEvidenceUpperBound(a, b typ.Type) (typ.Type, bool) {
	af := unwrap.Function(a)
	bf := unwrap.Function(b)
	if af == nil || bf == nil {
		return nil, false
	}
	switch {
	case UninformativeFunctionSeed(af) && !UninformativeFunctionSeed(bf):
		return bf, true
	case !UninformativeFunctionSeed(af) && UninformativeFunctionSeed(bf):
		return af, true
	default:
		return nil, false
	}
}

// UninformativeFunctionSeed reports the placeholder shape emitted before a
// function literal has a solved FunctionFact projection.
func UninformativeFunctionSeed(fn *typ.Function) bool {
	return fn != nil &&
		len(fn.TypeParams) == 0 &&
		len(fn.Params) == 0 &&
		fn.Variadic == nil &&
		len(fn.Returns) == 0 &&
		fn.Effects == nil &&
		fn.Spec == nil &&
		fn.Refinement == nil
}

// PrecisionEvidenceUpperBound preserves the most precise same-shape evidence
// before self-embedding repair runs. Dynamic container slots such as any[] are
// imprecise evidence for a table family; they must not cause a recursive fold or
// erase concrete element evidence during fixpoint convergence.
func PrecisionEvidenceUpperBound(a, b typ.Type) (typ.Type, bool) {
	if upper, ok := precisionEvidenceUpperBoundDirected(a, b); ok {
		return upper, true
	}
	return precisionEvidenceUpperBoundDirected(b, a)
}

func precisionEvidenceUpperBoundDirected(candidate, baseline typ.Type) (typ.Type, bool) {
	if precisionEvidenceCovers(candidate, baseline) {
		return candidate, true
	}
	candidateInner, candidateNilable := SplitNilable(candidate)
	baselineInner, _ := SplitNilable(baseline)
	if candidateNilable && candidateInner != nil && precisionEvidenceCovers(candidateInner, baselineInner) {
		return candidate, true
	}
	if baselineInner != nil && !sameValueNodeOrAcyclicEqual(baselineInner, baseline) && precisionEvidenceCovers(candidate, baselineInner) {
		return candidate, true
	}
	return nil, false
}

func precisionEvidenceCovers(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || sameValueNodeOrAcyclicEqual(candidate, baseline) {
		return false
	}
	if refines, changed := RefinesSoftContainer(candidate, baseline); refines && changed {
		return true
	}
	if !precisionEvidenceComparable(candidate, baseline) {
		return false
	}
	return typ.MorePrecise(candidate, baseline)
}

func precisionEvidenceComparable(a, b typ.Type) bool {
	return dynamicArrayEvidenceComparable(a, b)
}

func dynamicArrayEvidenceComparable(a, b typ.Type) bool {
	aa := arrayEvidenceShape(a)
	ba := arrayEvidenceShape(b)
	return aa != nil && ba != nil && (typ.ContainsAny(aa.Element) || typ.ContainsAny(ba.Element))
}

func arrayEvidenceShape(t typ.Type) *typ.Array {
	seen := make(map[typ.Type]bool)
	for {
		t = typ.UnwrapAnnotated(t)
		if t == nil || seen[t] {
			return nil
		}
		seen[t] = true
		switch v := t.(type) {
		case *typ.Alias:
			t = v.UnaliasedTarget()
		case *typ.Optional:
			t = v.Inner
		case *typ.Recursive:
			if v.Body == nil || v.Body == t {
				return nil
			}
			t = v.Body
		case *typ.Array:
			return v
		default:
			return nil
		}
	}
}

// DynamicMapUpperBound preserves the finite top map when one side already
// represents arbitrary dynamic table keys and values. Nilability is part of the
// outer product, so it is retained when either input may be nil.
func DynamicMapUpperBound(a, b typ.Type) (typ.Type, bool) {
	aInner, aNilable := SplitNilable(a)
	bInner, bNilable := SplitNilable(b)
	if upper, ok := dynamicMapUpperBoundInner(aInner, bInner); ok {
		if aNilable || bNilable {
			return typ.NewOptional(upper), true
		}
		return upper, true
	}
	return nil, false
}

func dynamicMapUpperBoundInner(a, b typ.Type) (typ.Type, bool) {
	am, aMap := UnwrapStructuralShape(a).(*typ.Map)
	bm, bMap := UnwrapStructuralShape(b).(*typ.Map)
	switch {
	case aMap && bMap && isDynamicMapTop(am):
		return am, true
	case aMap && bMap && isDynamicMapTop(bm):
		return bm, true
	default:
		return nil, false
	}
}

func isDynamicMapTop(m *typ.Map) bool {
	return m != nil && typ.IsAny(m.Key) && typ.IsAny(m.Value)
}

// UnsafePrecisionDrop reports whether merged lost a previously possible branch
// from prev while appearing as a subtype refinement.
func UnsafePrecisionDrop(prev, merged typ.Type) bool {
	if prev == nil || merged == nil || prev == merged {
		return false
	}
	if typ.IsAny(prev) || typ.IsUnknown(prev) {
		return !typ.IsAny(merged) && !typ.IsUnknown(merged)
	}
	if typ.ContainsRecursive(prev) || typ.ContainsRecursive(merged) {
		return false
	}
	if typ.TypeEquals(prev, merged) {
		return false
	}
	if upper, ok := FunctionEvidenceUpperBound(prev, merged); ok && typ.TypeEquals(upper, merged) {
		return false
	}
	if ElidesOptional(merged, prev) {
		return false
	}
	if refines, _ := RefinesSoftContainer(merged, prev); refines {
		return false
	}
	if refines, _ := RefinesFalsyMapKey(merged, prev); refines {
		return false
	}
	switch p := UnwrapStructuralShape(prev).(type) {
	case *typ.Union:
		if unionStrictMemberSubset(merged, p) {
			return true
		}
		if subtype.IsSubtype(merged, p) && !subtype.IsSubtype(p, merged) {
			return true
		}
	case *typ.Record:
		m, ok := UnwrapStructuralShape(merged).(*typ.Record)
		if !ok {
			break
		}
		for _, pf := range p.Fields {
			mf := m.GetField(pf.Name)
			if mf != nil && UnsafePrecisionDrop(pf.Type, mf.Type) {
				return true
			}
		}
		if p.HasMapComponent() && m.HasMapComponent() && UnsafePrecisionDrop(p.MapValue, m.MapValue) {
			return true
		}
	case *typ.Array:
		if m, ok := UnwrapStructuralShape(merged).(*typ.Array); ok {
			return UnsafePrecisionDrop(p.Element, m.Element)
		}
	case *typ.Map:
		if m, ok := UnwrapStructuralShape(merged).(*typ.Map); ok {
			return UnsafePrecisionDrop(p.Key, m.Key) || UnsafePrecisionDrop(p.Value, m.Value)
		}
	case *typ.Tuple:
		m, ok := UnwrapStructuralShape(merged).(*typ.Tuple)
		if !ok || len(p.Elements) != len(m.Elements) {
			break
		}
		for i := range p.Elements {
			if UnsafePrecisionDrop(p.Elements[i], m.Elements[i]) {
				return true
			}
		}
	case *typ.Function:
		m, ok := UnwrapStructuralShape(merged).(*typ.Function)
		if !ok {
			break
		}
		for i := 0; i < len(p.Params) && i < len(m.Params); i++ {
			if UnsafePrecisionDrop(p.Params[i].Type, m.Params[i].Type) {
				return true
			}
		}
		for i := 0; i < len(p.Returns) && i < len(m.Returns); i++ {
			if UnsafePrecisionDrop(p.Returns[i], m.Returns[i]) {
				return true
			}
		}
	}

	if subtype.IsSubtype(merged, prev) && !subtype.IsSubtype(prev, merged) {
		if _, ok := RecordExtensionUpperBound(merged, prev); ok {
			return false
		}
		return true
	}
	return false
}

func unionStrictMemberSubset(candidate typ.Type, baseline *typ.Union) bool {
	if baseline == nil {
		return false
	}
	candidateMembers := UnionMembers(candidate)
	if len(candidateMembers) == 0 {
		candidateMembers = []typ.Type{candidate}
	}
	if len(candidateMembers) >= len(baseline.Members) {
		return false
	}
	for _, member := range candidateMembers {
		found := false
		for _, baseMember := range baseline.Members {
			if typ.TypeEquals(member, baseMember) {
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
