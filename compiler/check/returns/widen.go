package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenFacts merges two interproc fact bundles.
func WidenFacts(prev, next api.Facts) api.Facts {
	out := api.Facts{
		ParamHints:         WidenParamHints(prev.ParamHints, next.ParamHints),
		CapturedTypes:      WidenCapturedTypes(prev.CapturedTypes, next.CapturedTypes),
		FieldWrites:        WidenFieldWrites(prev.FieldWrites, next.FieldWrites),
		CapturedContainers: WidenCapturedContainerMutations(prev.CapturedContainers, next.CapturedContainers),
		ConstructorFields:  WidenConstructorFields(prev.ConstructorFields, next.ConstructorFields),
	}
	if len(prev.Callables)+len(next.Callables) > 0 {
		out.Callables = make(api.Callables, len(prev.Callables)+len(next.Callables))
		for fn, fact := range prev.Callables {
			if next.Callables != nil {
				fact.Sig = maybeWidenFunctionForConvergence(fact.Sig)
			}
			out.Callables[fn] = fact
		}
		for fn, fact := range next.Callables {
			existing, ok := out.Callables[fn]
			if prev.Callables != nil {
				if ok && existing.Sig != nil {
					fact.Sig = mergeLiteralSig(prev.Callables[fn].Sig, fact.Sig)
				}
				fact.Sig = maybeWidenFunctionForConvergence(fact.Sig)
			}
			out.Callables[fn] = fact
		}
	}

	symbols := make(map[cfg.SymbolID]bool, len(prev.FunctionFacts)+len(next.FunctionFacts))
	for sym := range prev.FunctionFacts {
		symbols[sym] = true
	}
	for sym := range next.FunctionFacts {
		symbols[sym] = true
	}
	if len(symbols) == 0 {
		return out
	}

	out.FunctionFacts = make(api.FunctionFacts, len(symbols))
	for _, sym := range cfg.SortedSymbolIDs(symbols) {
		prevFact := prev.FunctionFacts[sym]
		nextFact := next.FunctionFacts[sym]
		reconciled := ReconcileFunctionFact(ReconcileFunctionFactInput{
			ExistingSummary:  prevFact.Summary,
			ExistingNarrow:   prevFact.Narrow,
			ExistingFunc:     prevFact.Func,
			CandidateSummary: nextFact.Summary,
			CandidateNarrow:  nextFact.Narrow,
			CandidateFunc:    nextFact.Func,
		})
		out.FunctionFacts[sym] = api.FunctionFact{
			Summary: widenReturnVectorForConvergence(reconciled.Summary),
			Narrow:  widenReturnVectorForConvergence(reconciled.Narrow),
			Func:    maybeWidenTypeForConvergence(reconciled.Func),
		}
	}
	if len(out.FunctionFacts) == 0 {
		out.FunctionFacts = nil
	}
	return out
}

// WidenReturnSummaries merges return summaries through the canonical
// return-summary merge policy shared by all iterative channels.
func WidenReturnSummaries(prev, next api.ReturnSummaries) api.ReturnSummaries {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.ReturnSummaries, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = widenReturnVectorForConvergence(NormalizeReturnVector(prev[sym]))
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		rets := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = widenReturnVectorForConvergence(MergeReturnSummary(existing, rets))
		} else {
			merged[sym] = widenReturnVectorForConvergence(NormalizeReturnVector(rets))
		}
	}
	return merged
}

func shouldUseMonotoneReturnJoin(a, b []typ.Type) bool {
	for _, t := range a {
		if hasHigherOrderGrowthRisk(t) {
			return true
		}
	}
	for _, t := range b {
		if hasHigherOrderGrowthRisk(t) {
			return true
		}
	}
	return false
}

func hasHigherOrderGrowthRisk(t typ.Type) bool {
	if t == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		switch n := node.(type) {
		case *typ.Function:
			for _, ret := range n.Returns {
				if typeContainsFunction(ret) {
					return true, false
				}
			}
		case *typ.Record:
			if recordHasSelfRecursiveMethod(n) {
				return true, false
			}
		}
		return false, true
	})
}

func typeContainsFunction(t typ.Type) bool {
	if t == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		// Interface method signatures are behavioral contracts, not first-class
		// returned function values. Ignore them for higher-order growth risk.
		if _, ok := node.(*typ.Interface); ok {
			return false, false
		}
		if _, ok := node.(*typ.Function); ok {
			return true, false
		}
		return false, true
	})
}

func recordHasSelfRecursiveMethod(r *typ.Record) bool {
	if r == nil {
		return false
	}
	for _, f := range r.Fields {
		if methodTypeHasSelfRecursiveReturn(f.Type, r) {
			return true
		}
	}
	if r.HasMapComponent() && methodTypeHasSelfRecursiveReturn(r.MapValue, r) {
		return true
	}
	return false
}

func methodTypeHasSelfRecursiveReturn(t typ.Type, owner *typ.Record) bool {
	if t == nil || owner == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		// Interface method signatures are behavioral contracts, not concrete
		// record method bodies. Treating them as self-recursive growth risk
		// over-applies monotone widening and blocks valid summary refinement.
		if _, ok := node.(*typ.Interface); ok {
			return false, false
		}
		fn, ok := node.(*typ.Function)
		if !ok {
			return false, true
		}
		for _, ret := range fn.Returns {
			// A return every value inhabits admits the owner without
			// relating to it.
			if ret == nil || subtype.AdmitsEveryValue(ret) {
				continue
			}
			if subtype.IsSubtype(ret, owner) || subtype.IsSubtype(owner, ret) ||
				TypeExtendsRecord(ret, owner) || TypeExtendsRecord(owner, ret) {
				return true, false
			}
		}
		return false, true
	})
}

func scanType(
	t typ.Type,
	guard internal.RecursionGuard,
	visit func(node typ.Type) (stop bool, descend bool),
) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	node := t
	for {
		ann, ok := node.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == node {
			break
		}
		node = ann.Inner
	}

	if stop, descend := visit(node); stop {
		return true
	} else if !descend {
		return false
	}

	switch n := node.(type) {
	case *typ.Optional:
		return scanType(n.Inner, next, visit)
	case *typ.Union:
		for _, m := range n.Members {
			if scanType(m, next, visit) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, m := range n.Members {
			if scanType(m, next, visit) {
				return true
			}
		}
		return false
	case *typ.Array:
		return scanType(n.Element, next, visit)
	case *typ.Map:
		return scanType(n.Key, next, visit) || scanType(n.Value, next, visit)
	case *typ.Tuple:
		for _, e := range n.Elements {
			if scanType(e, next, visit) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, p := range n.Params {
			if scanType(p.Type, next, visit) {
				return true
			}
		}
		for _, r := range n.Returns {
			if scanType(r, next, visit) {
				return true
			}
		}
		return n.Variadic != nil && scanType(n.Variadic, next, visit)
	case *typ.Record:
		for _, f := range n.Fields {
			if scanType(f.Type, next, visit) {
				return true
			}
		}
		if n.Metatable != nil && scanType(n.Metatable, next, visit) {
			return true
		}
		if n.HasMapComponent() {
			return scanType(n.MapKey, next, visit) || scanType(n.MapValue, next, visit)
		}
		return false
	case *typ.Alias:
		return scanType(n.Target, next, visit)
	case *typ.Instantiated:
		for _, a := range n.TypeArgs {
			if scanType(a, next, visit) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, m := range n.Methods {
			if m.Type != nil && scanType(m.Type, next, visit) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func joinReturnVectorsMonotone(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		out[i] = joinReturnTypeMonotone(ai, bi)
	}
	return out
}

func joinReturnTypeMonotone(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	// Keep widening monotone: if one side is already an upper bound, keep it.
	if subtype.IsSubtype(a, b) || TypeExtendsRecord(a, b) || typeElidesOptional(a, b) {
		return b
	}
	if subtype.IsSubtype(b, a) || TypeExtendsRecord(b, a) || typeElidesOptional(b, a) {
		return a
	}
	// Incomparable types join as return slots, which coalesce approximations
	// of one record field by field.
	return typ.JoinReturnSlot(a, b)
}

// WidenParamHints merges two param hint maps using monotone union.
func WidenParamHints(prev, next api.ParamHints) api.ParamHints {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return filterEmptyParamHints(next)
	}
	if next == nil {
		return filterEmptyParamHints(prev)
	}
	merged := make(api.ParamHints, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		hints := prev[sym]
		if hasNonNilHint(hints) {
			merged[sym] = hints
		}
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		hints := next[sym]
		if !hasNonNilHint(hints) {
			continue
		}
		if existing := merged[sym]; existing != nil {
			merged[sym] = joinParamHintVectors(existing, hints)
		} else {
			merged[sym] = hints
		}
	}
	return merged
}

func filterEmptyParamHints(hints api.ParamHints) api.ParamHints {
	if hints == nil {
		return nil
	}
	out := make(api.ParamHints, len(hints))
	for _, sym := range cfg.SortedSymbolIDs(hints) {
		v := hints[sym]
		if hasNonNilHint(v) {
			out[sym] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hasNonNilHint(hints []typ.Type) bool {
	for _, h := range hints {
		if h != nil {
			return true
		}
	}
	return false
}

// joinParamHintVectors joins two parameter hint vectors element-wise.
func joinParamHintVectors(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	result := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		result[i] = joinParamHintIteration(ai, bi)
	}
	return result
}

// A parameter hint is rebuilt from all call sites on every iteration. Keep
// fields discovered by an earlier pass, but use the current pass's presence
// and nilability for fields it now resolves. Otherwise an early optional
// approximation sticks after every caller supplies that field.
func joinParamHintIteration(previous, current typ.Type) typ.Type {
	joined := joinIterationFact(previous, current)
	if previous != nil && current != nil &&
		!typ.TypeEquals(narrow.RemoveNil(previous), previous) &&
		typ.TypeEquals(narrow.RemoveNil(current), current) &&
		!typ.TypeEquals(narrow.RemoveNil(joined), joined) {
		return current
	}
	a, aOK := previous.(*typ.Record)
	b, bOK := current.(*typ.Record)
	r, rOK := joined.(*typ.Record)
	if !aOK || !bOK || !rOK {
		return joined
	}
	builder := typ.NewRecord().SetOpen(r.Open).SetComplete(r.Complete).SetDeclared(a.Declared && b.Declared)
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	if r.HasMapComponent() {
		builder.MapComponent(r.MapKey, r.MapValue)
	}
	changed := false
	for _, field := range r.Fields {
		if old := a.GetField(field.Name); old != nil {
			if now := b.GetField(field.Name); now != nil {
				if field.Optional != now.Optional {
					field.Optional = now.Optional
					field.InferredPresence = now.InferredPresence
					changed = true
				}
				if !typ.TypeEquals(narrow.RemoveNil(old.Type), old.Type) &&
					typ.TypeEquals(narrow.RemoveNil(now.Type), now.Type) &&
					!typ.TypeEquals(narrow.RemoveNil(field.Type), field.Type) {
					field.Type = now.Type
					changed = true
				}
			}
		}
		addIterationField(builder, field)
	}
	if !changed {
		return joined
	}
	return builder.Build()
}

// joinIterationFact joins a fact from the previous fixpoint iteration with the
// same fact from the current one: a parameter hint, a captured variable type,
// a captured field or container write, a constructor field. Both describe the
// same program values, the current one under better-resolved facts, so the
// join follows the information order of inference:
//   - an unresolved fact (nil, unknown or a soft placeholder) yields to a
//     resolved one, and soft placeholder members drop out of unions;
//   - records join field by field, keeping fields that only one iteration
//     has discovered; a mutable field is invariant, so differing resolved
//     field types resolve to the current one;
//   - resolved types otherwise join to their upper bound, so a fact that
//     widened between iterations admits the values of both.
func joinIterationFact(a, b typ.Type) typ.Type {
	return joinIterationFactAt(a, b, false)
}

// joinIterationFactAt joins two iteration facts at a covariant position, or
// at an invariant one such as a mutable record field or a map key or value.
// At an invariant position an upper bound admits only equivalent types, so
// resolved facts that are not equivalent resolve to the current one, the
// fact that describes the values at the position now.
func joinIterationFactAt(a, b typ.Type, invariant bool) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	if joined, ok := joinIterationSnapshots(a, b, invariant); ok {
		return joined
	}
	if unwrap.IsNilType(a) && !unwrap.IsNilType(b) {
		return b
	}
	if unwrap.IsNilType(b) && !unwrap.IsNilType(a) {
		return a
	}
	if typ.IsUnknown(a) {
		return b
	}
	if typ.IsUnknown(b) {
		return a
	}
	a = typ.PruneSoftUnionMembers(a)
	b = typ.PruneSoftUnionMembers(b)
	softA := typ.IsSoft(a, typ.SoftPlaceholderPolicy)
	softB := typ.IsSoft(b, typ.SoftPlaceholderPolicy)
	if softA && !softB {
		return b
	}
	if softB && !softA {
		return a
	}
	unresolvedA := hasUnresolvedKeyDomain(a)
	unresolvedB := hasUnresolvedKeyDomain(b)
	if unresolvedA && !unresolvedB {
		return b
	}
	if unresolvedB && !unresolvedA {
		return a
	}
	if !invariant {
		if joined, ok := joinIterationOptionals(a, b); ok {
			return joined
		}
	}
	if joined, ok := joinIterationFunctions(a, b); ok {
		return joined
	}
	if joined, ok := joinIterationRecords(a, b); ok {
		return joined
	}
	if joined, ok := joinIterationArrays(a, b); ok {
		return joined
	}
	if joined, ok := joinIterationMaps(a, b); ok {
		return joined
	}
	// The previous fact stays while it admits the current one, so equivalent
	// facts with different spellings do not alternate between iterations. A
	// record admits one with other fields when their field types admit nil,
	// but the fields the current fact discovered are information a record
	// without them drops, so such a previous fact does not admit it.
	previousAdmitsCurrent := subtype.IsSubtype(b, a) && coversRecordFields(a, b)
	if invariant {
		if previousAdmitsCurrent && subtype.IsSubtype(a, b) {
			return a
		}
		return b
	}
	if previousAdmitsCurrent {
		return a
	}
	if subtype.IsSubtype(a, b) {
		return b
	}
	return typ.JoinPreferNonSoft(a, b)
}

// joinIterationSnapshots joins two snapshots of one recursion identity. They
// approximate the same table in successive iterations, so their join is a
// snapshot of that identity over the join of their bodies. Both bodies refer
// to the identity through one shared variable while they are joined.
func joinIterationSnapshots(a, b typ.Type, invariant bool) (typ.Type, bool) {
	ra, ok := a.(*typ.Recursive)
	if !ok || ra.Body == nil {
		return nil, false
	}
	rb, ok := b.(*typ.Recursive)
	if !ok || rb.Body == nil || rb.ID != ra.ID {
		return nil, false
	}
	variable := &typ.Recursive{ID: ra.ID, Name: ra.Name}
	open := func(body typ.Type) typ.Type {
		return typ.Rewrite(body, func(node typ.Type) (typ.Type, bool) {
			if r, ok := node.(*typ.Recursive); ok && r.ID == variable.ID {
				return variable, true
			}
			return nil, false
		})
	}
	return typ.BindRecursiveSnapshot(variable, joinIterationFactAt(open(ra.Body), open(rb.Body), invariant)), true
}

// A map value can gain resolved fields in a later iteration, just like an
// array element or record field. Preserve the key domain while refreshing
// provisional value members.
func joinIterationMaps(a, b typ.Type) (typ.Type, bool) {
	am, ok := a.(*typ.Map)
	if !ok {
		return nil, false
	}
	bm, ok := b.(*typ.Map)
	if !ok {
		return nil, false
	}
	key := joinIterationFactAt(am.Key, bm.Key, true)
	value := joinIterationFactAt(am.Value, bm.Value, true)
	if typ.TypeEquals(key, am.Key) && typ.TypeEquals(value, am.Value) {
		return a, true
	}
	if typ.TypeEquals(key, bm.Key) && typ.TypeEquals(value, bm.Value) {
		return b, true
	}
	return typ.NewMap(key, value), true
}

// hasUnresolvedKeyDomain reports whether t is a table written by keys of
// unknown type. Such a shape records writes whose keys an iteration could not
// yet type, so it approximates the table as softly as an unknown value does.
func hasUnresolvedKeyDomain(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Map:
		return typ.IsUnknown(v.Key)
	case *typ.Record:
		return v.HasMapComponent() && typ.IsUnknown(v.MapKey)
	}
	return false
}

// coversRecordFields reports whether a keeps the record shapes of b: every
// closed record of b, at the same position in a (a union or optional member,
// a field both records have, an array element or a map value), has its fields
// in a record of a, or a is open there. An open record of b states fields a
// value has, not its shape, so its fields need no counterpart in a; the
// records at its fields and containers still do.
func coversRecordFields(a, b typ.Type) bool {
	return coversFieldsAt(a, b, make(map[[2]typ.Type]bool))
}

func coversFieldsAt(a, b typ.Type, visiting map[[2]typ.Type]bool) bool {
	pair := [2]typ.Type{a, b}
	if visiting[pair] {
		return true
	}
	visiting[pair] = true
	defer delete(visiting, pair)
	for _, bm := range fieldCoverageMembers(b) {
		covered := false
		for _, am := range fieldCoverageMembers(a) {
			if coversMemberFields(am, bm, visiting) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// fieldCoverageMembers returns the non-nil members of t, aliases unwrapped.
func fieldCoverageMembers(t typ.Type) []typ.Type {
	t = unwrap.Alias(narrow.RemoveNil(t))
	if u, ok := t.(*typ.Union); ok {
		members := make([]typ.Type, 0, len(u.Members))
		for _, m := range u.Members {
			members = append(members, unwrap.Alias(m))
		}
		return members
	}
	return []typ.Type{t}
}

func coversMemberFields(a, b typ.Type, visiting map[[2]typ.Type]bool) bool {
	if a == nil || a.Kind().IsPlaceholder() {
		return true
	}
	// An open record with no fields carries no evidence about a dynamic
	// return value. Treating it as a refinement of any loses real values.
	if isOpenTopRecordType(a) && typ.IsAny(b) {
		return false
	}
	ar, aRecord := a.(*typ.Record)
	br, bRecord := b.(*typ.Record)
	if aRecord && bRecord {
		for _, bf := range br.Fields {
			af := ar.GetField(bf.Name)
			if af == nil {
				if !br.Open && !ar.Open && !ar.HasMapComponent() {
					return false
				}
				continue
			}
			if !coversFieldsAt(af.Type, bf.Type, visiting) {
				return false
			}
		}
	}
	aValue, aOK := containerValue(a)
	bValue, bOK := containerValue(b)
	if aOK && bOK {
		return coversFieldsAt(aValue, bValue, visiting)
	}
	return true
}

// containerValue returns the element type of an array, or the value type of
// a map or a record's map component.
func containerValue(t typ.Type) (typ.Type, bool) {
	switch v := t.(type) {
	case *typ.Array:
		return v.Element, true
	case *typ.Map:
		return v.Value, true
	case *typ.Record:
		if v.HasMapComponent() {
			return v.MapValue, true
		}
	}
	return nil, false
}

// joinIterationArrays joins two array facts element by element. Array slots
// are mutable, so their elements join at an invariant position: an earlier
// approximation of the element yields to the current one instead of leaving
// the two arrays as union members. The join reuses an input it equals.
func joinIterationArrays(a, b typ.Type) (typ.Type, bool) {
	arrA, okA := a.(*typ.Array)
	arrB, okB := b.(*typ.Array)
	if !okA || !okB {
		return nil, false
	}
	elem := joinIterationFactAt(arrA.Element, arrB.Element, true)
	switch {
	case typ.TypeEquals(elem, arrA.Element):
		return a, true
	case typ.TypeEquals(elem, arrB.Element):
		return b, true
	}
	return typ.NewArray(elem), true
}

// joinIterationOptionals joins two optional record or function facts, or one
// optional and one present: their present values join with
// joinIterationFact and the result is optional. An earlier approximation of
// the record or function thereby yields to the resolved one rather than
// remaining as a separate union member. The join reuses an input it equals,
// so unchanged facts keep their identity across iterations.
func joinIterationOptionals(a, b typ.Type) (typ.Type, bool) {
	presentA := narrow.RemoveNil(a)
	presentB := narrow.RemoveNil(b)
	if typ.TypeEquals(presentA, a) && typ.TypeEquals(presentB, b) {
		return nil, false
	}
	if !joinsStructurally(presentA, presentB) {
		return nil, false
	}
	joined := typ.NewOptional(joinIterationFact(presentA, presentB))
	switch {
	case typ.TypeEquals(joined, a):
		return a, true
	case typ.TypeEquals(joined, b):
		return b, true
	}
	return joined, true
}

// joinsStructurally reports whether two present facts are both records or
// both functions, the shapes joinIterationFact joins member by member.
func joinsStructurally(a, b typ.Type) bool {
	switch a.(type) {
	case *typ.Record:
		_, ok := b.(*typ.Record)
		return ok
	case *typ.Function:
		_, ok := b.(*typ.Function)
		return ok
	}
	return false
}

// joinIterationFunctions joins two function facts with the same parameter
// shape return by return with joinIterationFact, so a return that was
// unresolved in an earlier iteration yields to its resolved type. The
// parameters are those of the current fact.
func joinIterationFunctions(a, b typ.Type) (typ.Type, bool) {
	af, ok := a.(*typ.Function)
	if !ok {
		return nil, false
	}
	bf, ok := b.(*typ.Function)
	if !ok {
		return nil, false
	}
	if len(af.Params) != len(bf.Params) ||
		(af.Variadic == nil) != (bf.Variadic == nil) || len(af.TypeParams) != len(bf.TypeParams) {
		return nil, false
	}
	// A return slot one fact lacks is unresolved there and yields to the other.
	n := max(len(af.Returns), len(bf.Returns))
	returns := make([]typ.Type, n)
	sameAsB := len(bf.Returns) == n
	for i := range n {
		var ar, br typ.Type
		if i < len(af.Returns) {
			ar = af.Returns[i]
		}
		if i < len(bf.Returns) {
			br = bf.Returns[i]
		}
		returns[i] = joinIterationFact(ar, br)
		sameAsB = sameAsB && returns[i] == br
	}
	if sameAsB {
		return b, true
	}
	return typjoin.WithReturns(bf, returns), true
}

// joinIterationRecords joins two record hints field by field with
// joinIterationFact. Records with different map-component shapes or conflicting
// metatables are not joined here.
func joinIterationRecords(a, b typ.Type) (typ.Type, bool) {
	ar, ok := a.(*typ.Record)
	if !ok {
		return nil, false
	}
	br, ok := b.(*typ.Record)
	if !ok {
		return nil, false
	}
	if ar.HasMapComponent() != br.HasMapComponent() {
		return nil, false
	}
	metatable := ar.Metatable
	switch {
	case metatable == nil:
		metatable = br.Metatable
	case br.Metatable != nil && !typ.TypeEquals(metatable, br.Metatable):
		return nil, false
	}

	// The join reuses an input record it equals, so unchanged hints keep their
	// identity across iterations.
	declared := ar.Declared && br.Declared
	complete := typ.JoinedComplete(ar, br)
	sameAsA := ar.Metatable == metatable && ar.Open == (ar.Open || br.Open) && ar.Declared == declared && ar.Complete == complete
	sameAsB := br.Metatable == metatable && br.Open == (ar.Open || br.Open) && br.Declared == declared && br.Complete == complete

	builder := typ.NewRecord().SetOpen(ar.Open || br.Open).SetComplete(complete).SetDeclared(declared)
	if metatable != nil {
		builder.Metatable(metatable)
	}
	if ar.HasMapComponent() {
		key := joinIterationFactAt(ar.MapKey, br.MapKey, true)
		value := joinIterationFactAt(ar.MapValue, br.MapValue, true)
		sameAsA = sameAsA && key == ar.MapKey && value == ar.MapValue
		sameAsB = sameAsB && key == br.MapKey && value == br.MapValue
		builder.MapComponent(key, value)
	}
	for _, fa := range ar.Fields {
		field := fa
		fb := br.GetField(fa.Name)
		if fb != nil {
			field.Optional = fa.Optional || fb.Optional
			field.InferredPresence = field.Optional && (fa.InferredPresence || fb.InferredPresence) &&
				(!fa.Optional || fa.InferredPresence) && (!fb.Optional || fb.InferredPresence)
			field.Readonly = fa.Readonly && fb.Readonly
			field.Type = joinIterationFactAt(fa.Type, fb.Type, !field.Readonly)
		}
		sameAsA = sameAsA && field == fa
		sameAsB = sameAsB && fb != nil && field == *fb
		addIterationField(builder, field)
	}
	for _, fb := range br.Fields {
		if ar.GetField(fb.Name) == nil {
			sameAsA = false
			addIterationField(builder, fb)
		}
	}
	switch {
	case sameAsA:
		return a, true
	case sameAsB:
		return b, true
	}
	return builder.Build(), true
}

func addIterationField(builder *typ.RecordBuilder, f typ.Field) {
	builder.AddField(f)
}

func mergeLiteralSig(prev, next *typ.Function) *typ.Function {
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	if merged, ok := mergeFunctionReturnsIfSameShape(prev, next); ok {
		if fn, ok := merged.(*typ.Function); ok {
			return fn
		}
	}
	if subtype.IsSubtype(prev, next) {
		return next
	}
	if subtype.IsSubtype(next, prev) {
		return prev
	}
	// Literal signatures are constrained to *typ.Function. For incomparable
	// function shapes, keep the prior stable signature instead of narrowing.
	return prev
}

func copyFunctionWithSpec(fn *typ.Function, spec typ.SpecInfo) *typ.Function {
	b := typ.Func().Effects(fn.Effects).Spec(spec).WithRefinement(fn.Refinement)
	for _, tp := range fn.TypeParams {
		b.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range fn.Params {
		if p.Optional {
			b.OptParam(p.Name, p.Type)
		} else {
			b.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		b.Variadic(fn.Variadic)
	}
	b.Returns(fn.Returns...)
	return b.Build()
}

// JoinProvedEffects carries effects proved for one body onto an existing
// callable signature without replacing its parameter or return estimates.
func JoinProvedEffects(base, proved *typ.Function) *typ.Function {
	if base == nil {
		return proved
	}
	if proved == nil {
		return base
	}
	provedSpec := contract.ExtractSpec(proved)
	if provedSpec == nil || len(provedSpec.Effects.Labels) == 0 {
		return base
	}
	var relations effect.Row
	for _, label := range provedSpec.Effects.Labels {
		switch label.(type) {
		case effect.ErrorReturn, effect.CorrelatedReturn, effect.GuardedReturnType:
			relations = relations.With(label)
		}
	}
	if len(relations.Labels) == 0 {
		return base
	}
	baseSpec := contract.ExtractSpec(base)
	var joined contract.Spec
	if baseSpec != nil {
		joined = *baseSpec
	}
	joined.Effects = effect.Union(joined.Effects, relations)
	if baseSpec != nil && baseSpec.Effects.Equals(joined.Effects) {
		return base
	}
	return copyFunctionWithSpec(base, &joined)
}

// WidenCapturedTypes merges two captured type maps using monotone join.
func WidenCapturedTypes(prev, next api.CapturedTypes) api.CapturedTypes {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.CapturedTypes, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = maybeWidenTypeForConvergence(joinIterationFact(existing, t))
		} else {
			merged[sym] = maybeWidenTypeForConvergence(t)
		}
	}
	return merged
}

// WidenFieldWrites merges field-write maps using monotone union.
func WidenFieldWrites(prev, next api.FieldWrites) api.FieldWrites {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.FieldWrites, len(prev)+len(next))
	for _, callee := range cfg.SortedSymbolIDs(prev) {
		merged[callee] = prev[callee]
	}
	for _, callee := range cfg.SortedSymbolIDs(next) {
		captured := next[callee]
		existing := merged[callee]
		if existing == nil {
			merged[callee] = captured
			continue
		}
		merged[callee] = MergeFieldWriteSymbolMaps(existing, captured, func(_ api.FieldWriteKey, prev typ.Type, next typ.Type) typ.Type {
			if prev != nil {
				joined := joinIterationFact(prev, next)
				if joined == prev {
					return prev
				}
				return widenFieldWriteForConvergence(joined)
			}
			return widenFieldWriteForConvergence(next)
		})
	}
	return merged
}

// widenFieldWriteForConvergence closes a written field type over the records
// it nests. A write such as `node.parent = current; current = node` types the
// written record against the field type of the previous iteration, so each
// iteration nests one more approximation of the record; folding the nested
// approximations yields the recursive limit of that chain.
func widenFieldWriteForConvergence(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	return foldSelfRecursiveRecords(maybeWidenTypeForConvergence(t))
}

// WidenCapturedContainerMutations merges captured container mutation maps using monotone union.
func WidenCapturedContainerMutations(prev, next api.CapturedContainerMutations) api.CapturedContainerMutations {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.CapturedContainerMutations, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		muts := next[sym]
		existing := merged[sym]
		merged[sym] = MergeCapturedContainerMutationMaps(existing, muts, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
			if prev != nil {
				next.ValueType = maybeWidenTypeForConvergence(joinIterationFact(prev.ValueType, next.ValueType))
			} else {
				next.ValueType = maybeWidenTypeForConvergence(next.ValueType)
			}
			return next
		})
	}
	return merged
}

// WidenConstructorFields merges constructor field maps using monotone join.
func WidenConstructorFields(prev, next api.ConstructorFields) api.ConstructorFields {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.ConstructorFields, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existing := merged[sym]
		if existing == nil {
			merged[sym] = fields
			continue
		}
		out := make(map[string]typ.Type, len(existing)+len(fields))
		for _, name := range cfg.SortedFieldNames(existing) {
			out[name] = existing[name]
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			t := fields[name]
			if prevType := out[name]; prevType != nil {
				out[name] = maybeWidenTypeForConvergence(joinIterationFact(prevType, t))
			} else {
				out[name] = maybeWidenTypeForConvergence(t)
			}
		}
		merged[sym] = out
	}
	return merged
}

func mergeFunctionReturnsIfSameShape(prevFn, nextFn *typ.Function) (typ.Type, bool) {
	if prevFn == nil || nextFn == nil {
		return nil, false
	}
	if len(prevFn.TypeParams) != len(nextFn.TypeParams) {
		return nil, false
	}
	if !typeParamsEqual(prevFn.TypeParams, nextFn.TypeParams) {
		return nil, false
	}
	if len(prevFn.Params) != len(nextFn.Params) {
		return nil, false
	}
	if (prevFn.Variadic == nil) != (nextFn.Variadic == nil) {
		return nil, false
	}
	if prevFn.Variadic != nil && !typ.TypeEquals(prevFn.Variadic, nextFn.Variadic) {
		return nil, false
	}
	for i := range prevFn.Params {
		if prevFn.Params[i].Optional != nextFn.Params[i].Optional {
			return nil, false
		}
		if !typ.TypeEquals(prevFn.Params[i].Type, nextFn.Params[i].Type) {
			return nil, false
		}
	}
	if len(prevFn.Returns) == 0 && len(nextFn.Returns) == 0 {
		return prevFn, true
	}
	if len(prevFn.Returns) != len(nextFn.Returns) || len(prevFn.Returns) == 0 {
		return nil, false
	}

	allowedTypeParams := make(map[string]bool, len(prevFn.TypeParams))
	for _, tp := range prevFn.TypeParams {
		if tp != nil && tp.Name != "" {
			allowedTypeParams[tp.Name] = true
		}
	}
	normalizeReturn := func(t typ.Type) typ.Type {
		if t == nil {
			return nil
		}
		return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
			tp, ok := node.(*typ.TypeParam)
			if !ok {
				return node, false
			}
			if allowedTypeParams[tp.Name] {
				return node, false
			}
			// Free type params in non-generic function returns are unstable placeholders.
			return typ.Unknown, true
		})
	}
	normalizedPrev := make([]typ.Type, len(prevFn.Returns))
	normalizedNext := make([]typ.Type, len(nextFn.Returns))
	for i := range prevFn.Returns {
		normalizedPrev[i] = normalizeReturn(prevFn.Returns[i])
		normalizedNext[i] = normalizeReturn(nextFn.Returns[i])
	}

	mergedReturns := refineSnapshotMembersVector(typjoin.ReturnVectors(normalizedPrev, normalizedNext))
	if ReturnTypesEqual(prevFn.Returns, mergedReturns) {
		return prevFn, true
	}
	if ReturnTypesEqual(nextFn.Returns, mergedReturns) {
		return nextFn, true
	}

	effects := prevFn.Effects
	if effects == nil {
		effects = nextFn.Effects
	}
	spec := prevFn.Spec
	if spec == nil {
		spec = nextFn.Spec
	}
	refinement := prevFn.Refinement
	if refinement == nil {
		refinement = nextFn.Refinement
	}

	builder := typ.Func().
		Effects(effects).
		Spec(spec).
		WithRefinement(refinement)
	for _, tp := range prevFn.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range prevFn.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if prevFn.Variadic != nil {
		builder = builder.Variadic(prevFn.Variadic)
	}
	builder = builder.Returns(mergedReturns...)
	return builder.Build(), true
}

func typeParamsEqual(a, b []*typ.TypeParam) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil {
			if a[i] != b[i] {
				return false
			}
			continue
		}
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

func widenReturnVectorForConvergence(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return rets
	}
	out := make([]typ.Type, len(rets))
	changed := false
	for i, t := range rets {
		wt := maybeWidenTypeForConvergence(t)
		out[i] = wt
		if wt != t {
			changed = true
		}
	}
	if !changed {
		return rets
	}
	return out
}

func maybeWidenTypeForConvergence(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if !hasHigherOrderGrowthRisk(t) {
		return t
	}
	return foldSelfRecursiveRecords(subtype.WidenForInference(t))
}

// selfRecursiveRecordName names the recursive types produced by
// foldSelfRecursiveRecords.
const selfRecursiveRecordName = "self"

// foldSelfRecursiveRecords closes records whose methods reach an earlier
// approximation of the record itself.
//
// A table whose methods return the table is inferred one level at a time: each
// fixpoint iteration types the methods against the table type of the previous
// iteration, so the table type T_n nests T_(n-1) in its method signatures.
// The chain T_0 <: T_1 <: ... ascends forever. Replacing every nested
// approximation T' of T (same fields, T' <: T) by a reference to T itself
// yields mu X. T[T' := X], an upper bound of the whole chain and its limit.
func foldSelfRecursiveRecords(t typ.Type) typ.Type {
	folded := make(map[*typ.Record]typ.Type)
	// One subtyping session serves the whole pass: the approximation checks
	// compare the same shared substructure many times.
	sess := subtype.NewSession()
	return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		rec, ok := node.(*typ.Record)
		if !ok {
			return nil, false
		}
		if out, ok := folded[rec]; ok {
			return out, true
		}
		out := foldSelfRecursiveRecord(rec, sess)
		if out == typ.Type(rec) {
			return nil, false
		}
		folded[rec] = out
		return out, true
	})
}

// foldSelfRecursiveRecord returns mu X. owner[T' := X] for every approximation
// T' of owner nested in owner, or owner itself when none is nested.
func foldSelfRecursiveRecord(owner *typ.Record, sess *subtype.Session) typ.Type {
	return typ.FoldApproximations(selfRecursiveRecordName, owner, func(node typ.Type) bool {
		return isRecordApproximation(node, owner, sess)
	})
}

// isRecordApproximation reports whether t is an approximation of owner: a
// record, or a recursive record, with exactly owner's fields that lies below
// owner in the information order of inference.
func isRecordApproximation(t typ.Type, owner *typ.Record, sess *subtype.Session) bool {
	shape := unwrap.Alias(t)
	if rr, ok := shape.(*typ.Recursive); ok {
		shape = rr.Body
	}
	rec, ok := shape.(*typ.Record)
	if !ok || !rec.HasSameFieldNames(owner) {
		return false
	}
	return informationBelow(t, owner, make(map[[2]typ.Type]bool), sess)
}

// informationBelow reports whether a is an earlier approximation of b: a
// subtype of b, or a type that differs from one only where an earlier
// iteration had not yet resolved a type (unknown). Records compare field by
// field, unions member by member, and recursive types coinductively.
func informationBelow(a, b typ.Type, assumed map[[2]typ.Type]bool, sess *subtype.Session) bool {
	if a == nil || typ.IsUnknown(a) {
		return true
	}
	if b == nil {
		return false
	}
	if sess.IsSubtype(a, b) {
		return true
	}
	key := [2]typ.Type{a, b}
	if assumed[key] {
		return true
	}
	assumed[key] = true

	a = unwrap.Alias(a)
	b = unwrap.Alias(b)
	if ar, ok := a.(*typ.Recursive); ok {
		return informationBelow(ar.Body, b, assumed, sess)
	}
	if br, ok := b.(*typ.Recursive); ok {
		return informationBelow(a, br.Body, assumed, sess)
	}
	if au, ok := a.(*typ.Union); ok {
		for _, m := range au.Members {
			if !informationBelow(m, b, assumed, sess) {
				return false
			}
		}
		return true
	}
	if ao, ok := a.(*typ.Optional); ok {
		return informationBelow(typ.Nil, b, assumed, sess) && informationBelow(ao.Inner, b, assumed, sess)
	}
	switch bt := b.(type) {
	case *typ.Optional:
		return unwrap.IsNilType(a) || informationBelow(a, bt.Inner, assumed, sess)
	case *typ.Union:
		for _, m := range bt.Members {
			if informationBelow(a, m, assumed, sess) {
				return true
			}
		}
		return false
	case *typ.Array:
		aa, ok := a.(*typ.Array)
		return ok && informationBelow(aa.Element, bt.Element, assumed, sess)
	case *typ.Record:
		ar, ok := a.(*typ.Record)
		if !ok || ar.HasMapComponent() != bt.HasMapComponent() {
			return false
		}
		if ar.HasMapComponent() &&
			(!informationBelow(ar.MapKey, bt.MapKey, assumed, sess) || !informationBelow(ar.MapValue, bt.MapValue, assumed, sess)) {
			return false
		}
		for _, af := range ar.Fields {
			bf := bt.GetField(af.Name)
			if bf == nil {
				if bt.Open {
					continue
				}
				return false
			}
			if af.Optional && !bf.Optional && !typ.IsUnknown(af.Type) {
				return false
			}
			if !informationBelow(af.Type, bf.Type, assumed, sess) {
				return false
			}
		}
		for _, bf := range bt.Fields {
			if !bf.Optional && ar.GetField(bf.Name) == nil && !ar.Open {
				return false
			}
		}
		return true
	}
	return false
}

func maybeWidenFunctionForConvergence(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	if widened, ok := maybeWidenTypeForConvergence(fn).(*typ.Function); ok {
		return widened
	}
	return fn
}
