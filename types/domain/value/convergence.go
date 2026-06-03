package value

import (
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

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
	for _, member := range rec.StaticMembers {
		m := member
		m.Type = c.rewrite(member.Type, guard)
		if !typ.SameNode(m.Type, member.Type) {
			changed = true
		}
		addRecordStaticMember(builder, m)
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
	structuralFold       map[typ.Type]typ.Type
	activeRecursiveJoins map[recursiveProductJoinKey]*typ.Recursive
	// joinMode selects the pure least-upper-bound semantics over the convergence
	// widening semantics. Join must be the associative LUB, so a width-differing
	// record pair optionalizes its non-shared fields ({id} join {id,name} =
	// {id, name?}) instead of admitting one operand whole as the convergence
	// upper bound. The record-construction acceleration (RecordExtensionUpperBound:
	// {} -> {field = T} kept required) is a widening step that over-approximates
	// the join, so it is reserved for widen mode where Widen(a,b) >= Join(a,b).
	joinMode bool
}

type recursiveProductJoinKey struct {
	left  uint64
	right uint64
}

func newConvergenceWidenState() *convergenceWidenState {
	return &convergenceWidenState{
		growth:         newGrowthScanState(),
		seen:           make(map[typ.Type]typ.Type),
		active:         make(map[typ.Type]bool),
		structuralFold: make(map[typ.Type]typ.Type),
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
	} else {
		// Bound structural self-embedding towers. A record/map/tuple whose own
		// field/value/element re-introduces its product family below the root grows a
		// {text:{text:{text:...}}} tower across loop iterations; boundSelfEmbedding ties
		// each such knot into a finite recursive product and collapses the finite
		// unfoldings the recursive representative covers. This is the ACC widening for
		// that growth dimension, independent of callable surfaces: a plain
		// heterogeneous data record self-embeds just as a method-bearing class can.
		out = s.boundSelfEmbedding(t)
		if s.growth.hasHigherOrderGrowthRisk(out, typ.NewGuard()) {
			if folded, ok := FoldSelfEmbedding(out, out); ok {
				out = folded
			} else {
				out = subtype.WidenReturnTowerOnly(out)
			}
		}
	}
	s.seen[t] = out
	return out
}

// foldStructuralSelfEmbedding deeply ties off self-embedding structural towers.
// It folds children first, then folds the node itself, so a knot at any depth
// (a record field, map value, sequence element) is bounded into one recursive
// product instead of accumulating another unfolded level each iteration. Folding
// is a least-upper-bound-preserving widening: FoldSelfEmbedding only ties a knot
// when the node genuinely re-embeds its own family below the root, and the mu it
// returns covers every finite unfolding. Discriminant literals and field
// optionality are preserved; only self-embedding depth is collapsed.
func (s *convergenceWidenState) foldStructuralSelfEmbedding(t typ.Type) typ.Type {
	return s.foldStructuralSelfEmbeddingGuard(t, typ.NewGuard())
}

func (s *convergenceWidenState) foldStructuralSelfEmbeddingGuard(t typ.Type, guard internal.RecursionGuard) typ.Type {
	if t == nil || typ.ContainsRecursive(t) {
		return t
	}
	if out, ok := s.structuralFold[t]; ok {
		return out
	}
	next, ok := guard.Enter(t)
	if !ok {
		return t
	}
	// Top-down: tie the shallowest self-embedding knot first. Folding the outermost
	// anchor ties the entire subtree below it into one recursive product, so the
	// descent never re-walks the deep tower interior. Only nodes that do not fold
	// are descended into, bounding any remaining inner knot.
	//
	// The fold is admitted only when the recursive product is well-founded: it must
	// have a base case, an unfolding reachable without traversing the self-reference.
	// FoldSelfEmbedding ties a knot at a required field whose tower leaf omits it (the
	// {v} leaf of a {v, next:{v, next:...}} tower) into mu X.{v, next:X} with next
	// required — a product with no base case that covers no finite unfolding. Such an
	// under-approximating fold is rejected so the node falls through to the structural
	// descent and the existing element/field widening bounds it soundly. A message
	// tower mu X.{text: string | tuple(X), type:"text"} exits through the string union
	// member, so it is well-founded and admitted.
	if folded, ok := FoldSelfEmbedding(t, t); ok && recursiveProductWellFounded(folded) {
		s.structuralFold[t] = folded
		return folded
	}
	var out typ.Type
	switch v := t.(type) {
	case *typ.Optional:
		inner := s.foldStructuralSelfEmbeddingGuard(v.Inner, next)
		if typ.SameNode(inner, v.Inner) {
			out = t
		} else {
			out = typ.NewOptional(inner)
		}
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		for i, m := range v.Members {
			members[i] = s.foldStructuralSelfEmbeddingGuard(m, next)
			if !typ.SameNode(members[i], m) {
				changed = true
			}
		}
		if changed {
			out = typ.NewUnion(members...)
		} else {
			out = t
		}
	case *typ.Array:
		elem := s.foldStructuralSelfEmbeddingGuard(v.Element, next)
		if typ.SameNode(elem, v.Element) {
			out = t
		} else {
			out = typ.NewArray(elem)
		}
	case *typ.Tuple:
		elements := make([]typ.Type, len(v.Elements))
		changed := false
		for i, e := range v.Elements {
			elements[i] = s.foldStructuralSelfEmbeddingGuard(e, next)
			if !typ.SameNode(elements[i], e) {
				changed = true
			}
		}
		if changed {
			out = typ.NewTuple(elements...)
		} else {
			out = t
		}
	case *typ.Map:
		key := s.foldStructuralSelfEmbeddingGuard(v.Key, next)
		val := s.foldStructuralSelfEmbeddingGuard(v.Value, next)
		if typ.SameNode(key, v.Key) && typ.SameNode(val, v.Value) {
			out = t
		} else {
			out = typ.NewMap(key, val)
		}
	case *typ.Record:
		out = s.foldRecordChildrenSelfEmbedding(v, next)
	default:
		out = t
	}
	s.structuralFold[t] = out
	return out
}

// foldRecordChildrenSelfEmbedding rebuilds a record with each field, map slot, and
// metatable structurally folded, preserving optionality, readonly, openness, and
// discriminant literals.
func (s *convergenceWidenState) foldRecordChildrenSelfEmbedding(r *typ.Record, guard internal.RecursionGuard) *typ.Record {
	builder := typ.NewRecord()
	if r.Open {
		builder.SetOpen(true)
	}
	changed := false
	for _, f := range r.Fields {
		fieldType := s.foldStructuralSelfEmbeddingGuard(f.Type, guard)
		if !typ.SameNode(fieldType, f.Type) {
			changed = true
		}
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, fieldType)
		case f.Optional:
			builder.OptField(f.Name, fieldType)
		case f.Readonly:
			builder.ReadonlyField(f.Name, fieldType)
		default:
			builder.Field(f.Name, fieldType)
		}
	}
	for _, m := range r.StaticMembers {
		memberType := s.foldStructuralSelfEmbeddingGuard(m.Type, guard)
		if !typ.SameNode(memberType, m.Type) {
			changed = true
		}
		m.Type = memberType
		addRecordStaticMember(builder, m)
	}
	if r.HasMapComponent() {
		mapKey := s.foldStructuralSelfEmbeddingGuard(r.MapKey, guard)
		mapValue := s.foldStructuralSelfEmbeddingGuard(r.MapValue, guard)
		if !typ.SameNode(mapKey, r.MapKey) || !typ.SameNode(mapValue, r.MapValue) {
			changed = true
		}
		builder.MapComponent(mapKey, mapValue)
	}
	if r.Metatable != nil {
		mt := s.foldStructuralSelfEmbeddingGuard(r.Metatable, guard)
		if !typ.SameNode(mt, r.Metatable) {
			changed = true
		}
		builder.Metatable(mt)
	}
	if !changed {
		return r
	}
	return builder.Build()
}

// recursiveProductWellFounded reports whether a recursive product admits a base
// case: a finite unfolding reachable without traversing the recursive reference.
// A well-founded mu covers its finite unfoldings (the over-approximation half of a
// sound widening); a mu without a base case (mu X.{next: X} with next required)
// denotes only the infinite value and covers nothing finite, so folding into it
// under-approximates.
func recursiveProductWellFounded(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Recursive)
	if !ok || rec == nil || rec.Body == nil {
		return false
	}
	return recursiveBodyHasBaseCase(rec.Body, rec, typ.NewGuard())
}

// recursiveBodyHasBaseCase reports whether body can be inhabited by a finite value
// that does not traverse self. A node not mentioning self is itself a base case; a
// union exits through any base-case member; a record needs every required field to
// have a base case (optional fields and a map component may be omitted/empty); an
// array or map exits through the empty container; a tuple needs every element to
// have a base case; an optional always exits through nil.
func recursiveBodyHasBaseCase(body typ.Type, self *typ.Recursive, guard internal.RecursionGuard) bool {
	if body == nil {
		return false
	}
	if !mentionsRecursiveSelf(body, self) {
		return true
	}
	next, ok := guard.Enter(body)
	if !ok {
		return false
	}
	switch v := unwrap.Alias(body).(type) {
	case *typ.Recursive:
		if typ.IsRecursiveRef(v, self) {
			return false
		}
		return v.Body != nil && recursiveBodyHasBaseCase(v.Body, self, next)
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if recursiveBodyHasBaseCase(member, self, next) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map:
		return true
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if !recursiveBodyHasBaseCase(elem, self, next) {
				return false
			}
		}
		return true
	case *typ.Record:
		for _, field := range v.Fields {
			if field.Optional {
				continue
			}
			if !recursiveBodyHasBaseCase(field.Type, self, next) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func mentionsRecursiveSelf(t typ.Type, self *typ.Recursive) bool {
	found := false
	typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		if typ.IsRecursiveRef(node, self) {
			found = true
		}
		return nil, false
	})
	return found
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
	if out, ok := subtype.WidenReturnTowerOnly(widened).(*typ.Function); ok {
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
//
// The merge result is bounded so the boundary is a true widening (ACC): the merge
// is the least upper bound, and bounding closes the one dimension a plain LUB does
// not — the structural depth that grows when successive loop iterations contribute
// same-family records whose own field/element re-introduces the family (a record
// one reads as a recursive alternative of another). Folding those self-embedding
// towers into one finite recursive product makes the ascending chain stabilize. The
// bound is the self-embedding fold alone, not full widening: the LUB's other
// precision (discriminated union members, exact field widths) must be preserved, so
// the width-coalescing of CoalesceProductUnion is not applied here.
func MergeForConvergence(existing, candidate typ.Type) typ.Type {
	s := newConvergenceWidenState()
	return s.boundSelfEmbedding(s.merge(existing, candidate))
}

// boundSelfEmbedding applies the ACC self-embedding fold to a merge result without
// the union width-coalescing the full widen performs, so discriminated-union and
// field-width precision survive the fixpoint boundary while self-embedding towers
// are still bounded into a finite recursive product.
func (s *convergenceWidenState) boundSelfEmbedding(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	out := s.foldStructuralSelfEmbedding(t)
	if typ.ContainsRecursive(out) {
		out = collapseSameFamilyRecursiveUnions(out)
	}
	return out
}

// JoinForConvergence is the associative least-upper-bound over two value facts.
//
// It shares the convergence merge's structural dispatch (recursive product
// folding, gradual unknown/any handling, union coalescing) but selects the pure
// LUB semantics: a width-differing record pair optionalizes its non-shared fields
// rather than admitting one operand whole, so the result is the unique least
// upper bound and Join stays associative. The construction-history acceleration
// that keeps a freshly added field required belongs to WidenForConvergence.
func JoinForConvergence(existing, candidate typ.Type) typ.Type {
	s := newConvergenceWidenState()
	s.joinMode = true
	return s.merge(existing, candidate)
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
	// The convergence upper bound runs first in both modes. Its laws are
	// least-upper-bound-preserving (they return the operand that already covers the
	// other: precision evidence absorbing a dynamic any[] / dynamic map slot, an
	// elided optional, a function-evidence projection, a recursive-family cover);
	// returning the covering operand is exactly Join(A,B)=B when A<=B. The one
	// over-approximating law, RecordExtensionUpperBound, is suppressed under joinMode
	// inside convergenceUpperBound, so a width-differing record pair falls through to
	// the optionalizing structural join below rather than being admitted whole.
	//
	// Running the structural join before these covers checks is unsound for Join: it
	// can mis-fold an imprecise-but-covered operand (e.g. {[string]: any[]} joined
	// with a refined {[string]: {...}[]}) into a spurious self-embedding recursion
	// instead of yielding the covering refined operand.
	if upper, ok := s.convergenceUpperBound(existing, candidate); ok {
		return upper
	}
	if s.joinMode {
		// In Join mode the optionalizing structural join is the least upper bound
		// for shape-compatible records/maps/sequences the convergence upper bound did
		// not already absorb (a width-differing record pair: {id} join {id,name} =
		// {id, name?}).
		if joined, ok := JoinStructuralShape(existing, candidate, s.merge); ok {
			return joined
		}
		if joined, ok := JoinStructuralUnionShape(existing, candidate, s.merge); ok {
			return joined
		}
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
	if upper, ok := s.convergenceUpperBound(existing, candidate); ok {
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

// reduceConvergenceUnionMembers computes the order-independent canonical
// least-upper-bound of a union's members. The merge of an unordered member set
// must be a semilattice operation: commutative, associative, and idempotent. A
// single encounter-order left-fold is none of these, because greedily folding a
// member into the first compatible group it meets can strand two members of one
// family in separate groups when no later member happens to bridge them; the
// surviving group then exposes a less precise field (a converged record field
// can flip string -> unknown purely on Go map-iteration order).
//
// The reduction is therefore a confluent closure rather than a left-fold:
//   - members are canonically ordered so representative selection is total and
//     order-independent (it never decides precision, only the surface among
//     equally-precise alternatives);
//   - groups are merged pairwise with a commutative binary join until no pair
//     merges, so the final partition is the coarsest one the join admits,
//     independent of the order members arrived in.
func (s *convergenceWidenState) reduceConvergenceUnionMembers(members []typ.Type) []typ.Type {
	if len(members) < 2 {
		return members
	}
	groups := make([]typ.Type, 0, len(members))
	for _, member := range members {
		if member == nil || typ.IsAbsentOrUnknown(member) {
			continue
		}
		groups = append(groups, member)
	}
	if len(groups) < 2 {
		return groups
	}
	sortConvergenceUnionMembers(groups)
	return s.closeConvergenceUnionGroups(groups)
}

// sortConvergenceUnionMembers imposes a total order over the member set so the
// closure's representative selection is deterministic. The key is cycle-stable:
// ProductFamilyHash is the coinductive structural fold (it ignores fresh
// recursive node ids), and convergenceMemberOrderKey is the recursion-id-blind
// structural tiebreak that resolves genuine hash collisions without consulting
// any pointer or allocation counter. The order never decides precision; it only
// fixes which canonical surface is chosen among equally-precise alternatives.
func sortConvergenceUnionMembers(members []typ.Type) {
	if len(members) < 2 {
		return
	}
	type slot struct {
		typ  typ.Type
		hash uint64
		key  string
	}
	slots := make([]slot, len(members))
	for i, m := range members {
		slots[i] = slot{typ: m, hash: typ.ProductFamilyHash(m), key: convergenceMemberOrderKey(m)}
	}
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].hash != slots[j].hash {
			return slots[i].hash < slots[j].hash
		}
		return slots[i].key < slots[j].key
	})
	for i, s := range slots {
		members[i] = s.typ
	}
}

// convergenceMemberOrderKey renders a cycle-stable structural key for one type.
// Recursive references are encoded by the de Bruijn depth of their binder, never
// by node identity, so two observations of the same family with fresh recursive
// ids produce the same key.
func convergenceMemberOrderKey(t typ.Type) string {
	var b strings.Builder
	w := convergenceOrderKeyWriter{
		b:      &b,
		budget: convergenceOrderKeyNodeBudget,
		active: make(map[typ.Type]int),
	}
	w.write(t, nil)
	return b.String()
}

const convergenceOrderKeyNodeBudget = 4096

type convergenceOrderKeyWriter struct {
	b         *strings.Builder
	budget    int
	nodes     int
	exhausted bool
	active    map[typ.Type]int
}

func (w *convergenceOrderKeyWriter) enter(t typ.Type) bool {
	if t == nil {
		return true
	}
	if w.budget <= 0 {
		return true
	}
	w.nodes++
	if w.nodes <= w.budget {
		return true
	}
	if !w.exhausted {
		w.b.WriteString("...;")
	}
	w.exhausted = true
	return false
}

func (w *convergenceOrderKeyWriter) write(t typ.Type, binders []*typ.Recursive) {
	if t == nil {
		w.b.WriteString("nil;")
		return
	}
	t = unwrap.Alias(t)
	if !w.enter(t) {
		return
	}
	if idx, ok := w.active[t]; ok {
		w.b.WriteString("ref#")
		w.b.WriteString(strconv.Itoa(len(w.active) - 1 - idx))
		w.b.WriteByte(';')
		return
	}
	w.active[t] = len(w.active)
	defer delete(w.active, t)

	switch v := t.(type) {
	case *typ.Recursive:
		if idx, ok := recursiveBinderIndex(v, binders); ok {
			w.b.WriteString("self#")
			w.b.WriteString(strconv.Itoa(idx))
			w.b.WriteByte(';')
			return
		}
		if v.Body == nil {
			w.b.WriteString("mu?;")
			return
		}
		w.b.WriteString("mu:")
		w.b.WriteString(v.Name)
		w.b.WriteByte('(')
		w.write(v.Body, append(binders, v))
		w.b.WriteString(");")
	case *typ.Union:
		members := append([]typ.Type(nil), v.Members...)
		keys := make([]string, len(members))
		for i, m := range members {
			var sub strings.Builder
			prev := w.b
			w.b = &sub
			w.write(m, binders)
			w.b = prev
			keys[i] = sub.String()
		}
		sort.Strings(keys)
		w.b.WriteString("U[")
		for _, k := range keys {
			w.b.WriteString(k)
		}
		w.b.WriteString("];")
	case *typ.Optional:
		w.b.WriteString("opt(")
		w.write(v.Inner, binders)
		w.b.WriteString(");")
	case *typ.Array:
		w.b.WriteString("arr(")
		w.write(v.Element, binders)
		w.b.WriteString(");")
	case *typ.Map:
		w.b.WriteString("map(")
		w.write(v.Key, binders)
		w.b.WriteByte(',')
		w.write(v.Value, binders)
		w.b.WriteString(");")
	case *typ.Record:
		w.writeRecord(v, binders)
	case *typ.Function:
		w.b.WriteString("fn[")
		for _, p := range v.Params {
			w.b.WriteString(p.Name)
			if p.Optional {
				w.b.WriteByte('?')
			}
			w.b.WriteByte(':')
			w.write(p.Type, binders)
		}
		if v.Variadic != nil {
			w.b.WriteString("...")
			w.write(v.Variadic, binders)
		}
		w.b.WriteString("->")
		for _, r := range v.Returns {
			w.write(r, binders)
		}
		w.b.WriteString("];")
	default:
		if ref, ok := recursiveBinderIndex(t, binders); ok {
			w.b.WriteString("self#")
			w.b.WriteString(strconv.Itoa(ref))
			w.b.WriteByte(';')
			return
		}
		w.b.WriteString(t.Kind().String())
		w.b.WriteByte('#')
		w.b.WriteString(strconv.FormatUint(t.Hash(), 16))
		w.b.WriteByte(';')
	}
}

func (w *convergenceOrderKeyWriter) writeRecord(r *typ.Record, binders []*typ.Recursive) {
	w.b.WriteString("rec{")
	if r.Open {
		w.b.WriteString("open;")
	}
	fields := append([]typ.Field(nil), r.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, f := range fields {
		w.b.WriteString(f.Name)
		if f.Optional {
			w.b.WriteByte('?')
		}
		w.b.WriteByte(':')
		w.write(f.Type, binders)
	}
	if r.HasMapComponent() {
		w.b.WriteString("[map]")
		w.write(r.MapKey, binders)
		w.b.WriteByte(',')
		w.write(r.MapValue, binders)
	}
	w.b.WriteString("};")
}

// recursiveBinderIndex returns the de Bruijn depth of t when it is a reference to
// one of the enclosing recursive binders, counting from the innermost binder.
func recursiveBinderIndex(t typ.Type, binders []*typ.Recursive) (int, bool) {
	for i := len(binders) - 1; i >= 0; i-- {
		if typ.IsRecursiveRef(t, binders[i]) {
			return len(binders) - 1 - i, true
		}
	}
	return 0, false
}

// binderIndexOf returns the de Bruijn depth of a recursive node already in scope,
// so re-encountering a binder during the structural walk closes the cycle instead
// of recursing into its body forever.
func binderIndexOf(rec *typ.Recursive, binders []*typ.Recursive) (int, bool) {
	for i := len(binders) - 1; i >= 0; i-- {
		if binders[i] == rec {
			return len(binders) - 1 - i, true
		}
	}
	return 0, false
}

// closeConvergenceUnionGroups merges groups to a fixpoint. Each pass scans every
// unordered pair; on a successful merge the merged representative replaces the
// pair and the scan restarts so the new representative is re-tested against all
// remaining groups. Because the binary merge is a join (a least upper bound for
// the family it joins), the closure reaches the same coarsest partition for any
// canonical input order, so the result is associative and order-independent.
func (s *convergenceWidenState) closeConvergenceUnionGroups(groups []typ.Type) []typ.Type {
	for {
		merged := false
		for i := 0; i < len(groups); i++ {
			for j := i + 1; j < len(groups); j++ {
				joined, ok := s.mergeConvergenceUnionPair(groups[i], groups[j])
				if !ok {
					continue
				}
				groups[i] = joined
				groups = append(groups[:j], groups[j+1:]...)
				merged = true
				break
			}
			if merged {
				break
			}
		}
		if !merged {
			break
		}
	}
	sortConvergenceUnionMembers(groups)
	return groups
}

// mergeConvergenceUnionPair is the commutative binary join over two union
// members. Every join law is probed in both operand orders so the merge is
// symmetric: mergeConvergenceUnionPair(a,b) and (b,a) admit the same upper bound.
func (s *convergenceWidenState) mergeConvergenceUnionPair(a, b typ.Type) (typ.Type, bool) {
	if SameConvergedFact(a, b) {
		return a, true
	}
	if joined, ok := s.joinRecursiveProducts(a, b); ok {
		return joined, true
	}
	// Prefer the precise convergence bounds first (top-array absorption,
	// recursive coverage, record extension). They keep the simplest finite
	// representative and are least-upper-bound-preserving (they return the operand
	// that already covers the other), so running them before the structural join
	// stops a covered imprecise member from mis-folding into a spurious recursion.
	// RecordExtensionUpperBound is suppressed under joinMode inside
	// convergenceUpperBound, so a width-differing record pair still falls through to
	// the optionalizing structural join below rather than being admitted whole.
	if upper, ok := s.convergenceUpperBound(a, b); ok {
		return upper, true
	}
	if s.joinMode {
		// Join mode optionalizes width-differing record members the convergence upper
		// bound did not absorb instead of taking a record extension whole.
		if joined, ok := s.joinStructuralUnionPair(a, b); ok {
			return joined, true
		}
	}
	// Close a self-embedding tower into a recursive family: when one member is a
	// deeper unfolding of the other's family, fold it into a recursive upper bound
	// so progressive unfoldings cannot accumulate as distinct members and grow the
	// union without bound.
	if folded, ok := s.foldSelfEmbeddingUpperBound(a, b); ok {
		return folded, true
	}
	if joined, ok := s.joinStructuralUnionPair(a, b); ok {
		return s.widen(joined), true
	}
	return nil, false
}

// joinStructuralUnionPair runs the structural table joins in both operand orders
// so a directed structural join (record/map/sequence) admits the same shape
// regardless of which member is the accumulator.
func (s *convergenceWidenState) joinStructuralUnionPair(a, b typ.Type) (typ.Type, bool) {
	if joined, ok := JoinStructuralShape(a, b, s.merge); ok {
		return joined, true
	}
	if joined, ok := JoinStructuralShape(b, a, s.merge); ok {
		return joined, true
	}
	if joined, ok := JoinStructuralUnionShape(a, b, s.merge); ok {
		return joined, true
	}
	return nil, false
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
		if !ok {
			continue
		}
		folded = s.widen(folded)
		if folded == nil {
			continue
		}
		coversBoth := Covers(folded, existing) && Covers(folded, candidate)
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

func (s *convergenceWidenState) convergenceUpperBound(existing, candidate typ.Type) (typ.Type, bool) {
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
		return upper, true
	}
	// RecordExtensionUpperBound admits a construction-history extension whole, with
	// the freshly added field kept required. That over-approximates the join (the
	// LUB optionalizes the non-shared field), so it is a widening step only; Join
	// reaches the optionalizing structural join above instead.
	if !s.joinMode {
		if upper, ok := RecordExtensionUpperBound(existing, candidate); ok {
			return upper, true
		}
	}
	if upper, ok := RecursiveUnionUpperBound(existing, candidate); ok {
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
