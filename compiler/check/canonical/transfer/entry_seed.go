package transfer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// EntryReachabilityEffect lifts bottom-only axes to their reachable identity at a
// function entry point. The graph entry has no predecessor, so the solver's bottom
// seed must become the empty numeric environment and no-op cell-effect summary
// before ordinary parameter entry seeds run.
type EntryReachabilityEffect struct{}

func (t *Transfer) applyEntryReachabilityEffect(out *flow.PointState, _ EntryReachabilityEffect) bool {
	if out == nil {
		return false
	}
	changed := false
	if out.Num == nil || out.Num.IsUnsat() {
		out.Num = numeric.NewState()
		changed = true
	}
	if constraint.Domain.Equal(out.Cond, constraint.Domain.Bottom()) {
		out.Cond = constraint.Domain.Top()
		changed = true
	}
	if out.Rel.IsBottom() {
		out.Rel = flow.PointRelationsDomain.Top()
		changed = true
	}
	if out.ReturnRel.IsBottom() {
		out.ReturnRel = flow.ReturnRelationsDomain.Top()
		changed = true
	}
	if out.CellEffects.IsBottom() {
		out.CellEffects = flow.CaptureEffectsIdentity()
		changed = true
	}
	if out.ReceiverEffects.IsBottom() {
		out.ReceiverEffects = flow.ReceiverEffectsIdentity()
		changed = true
	}
	if out.StaticMembers.IsBottom() {
		out.StaticMembers = flow.StaticMemberFactsDomain.Top()
		changed = true
	}
	if out.KeyPresence.IsBottom() {
		out.KeyPresence = flow.KeyPresenceFactsDomain.Top()
		changed = true
	}
	if out.ValueOrigins.IsBottom() {
		out.ValueOrigins = flow.ValueOriginFactsDomain.Top()
		changed = true
	}
	if out.IndexWrites.IsBottom() {
		out.IndexWrites = flow.IndexWriteAdmissionFactsDomain.Top()
		changed = true
	}
	return changed
}

// EntrySeedEffect is the entry-point reducer for one parameter slot. It composes
// the sources visible at function entry in a single product-state write:
// declared annotation, exact caller-provided entry value, and, only for
// unannotated parameters, body-demand contract. Declared annotations are source
// authority; body demand is an obligation, not proof about a declared dynamic
// slot. Exact entry evidence may still refine dynamic/soft annotation interiors.
type EntrySeedEffect struct {
	Symbol   cfg.SymbolID
	Declared typ.Type
	Entry    product.AbstractValue
	Contract product.AbstractValue
}

func (t *Transfer) applyEntrySeedEffect(out *flow.PointState, effect EntrySeedEffect) bool {
	if out == nil || effect.Symbol == 0 {
		return false
	}
	av := entrySeedValue(effect.Declared, effect.Entry, effect.Contract)
	if av.IsZero() {
		return false
	}
	t.setSymbolValue(out, effect.Symbol, av, false)
	return true
}

func (t *Transfer) closedDeclaredParamBySlot(out *flow.PointState) map[int]typ.Type {
	if t == nil || out == nil || len(t.declaredParamBySlot) == 0 || len(t.in.Scope.ParamSymbols) == 0 {
		return t.declaredParamBySlot
	}
	typeParams := collectDeclaredParamTypeParams(t.declaredParamBySlot)
	if len(typeParams) == 0 {
		return t.declaredParamBySlot
	}

	evidence := make([]typ.Type, len(t.in.Scope.ParamSymbols))
	for slot, sym := range t.in.Scope.ParamSymbols {
		if sym == 0 {
			continue
		}
		av, ok := t.symbolValue(out, sym)
		if !ok || av.IsZero() {
			continue
		}
		if got := av.ProjectValue(); got != nil {
			evidence[slot] = got
		}
	}
	return closeDeclaredParamTypes(t.declaredParamBySlot, typeParams, evidence)
}

func closeDeclaredParamTypes(declared map[int]typ.Type, typeParams []*typ.TypeParam, evidence []typ.Type) map[int]typ.Type {
	if len(declared) == 0 || len(typeParams) == 0 || len(evidence) == 0 {
		return declared
	}
	typeVarArgs := make([]typ.Type, len(typeParams))
	for i := range typeParams {
		typeVarArgs[i] = typ.NewTypeVar(i + 1)
	}

	cs := constraint.NewInferSet()
	for slot, got := range evidence {
		if got == nil || typ.ContainsFreeTypeParam(got) {
			continue
		}
		pattern := declared[slot]
		if pattern == nil {
			continue
		}
		pattern = subst.Params(pattern, typeParams, typeVarArgs)
		pattern = subst.ExpandInstantiated(pattern)
		got = subst.ExpandInstantiated(got)
		constraint.MatchContra(pattern, got, cs)
	}

	solution, err := cs.Solve()
	if err != nil || len(solution) == 0 {
		return declared
	}
	typeArgs := make([]typ.Type, len(typeParams))
	solvedAny := false
	for i, tp := range typeParams {
		if tp == nil {
			typeArgs[i] = typ.Unknown
			continue
		}
		solved := solution[i+1]
		if solved == nil {
			typeArgs[i] = typ.Unknown
			continue
		}
		if tp.Constraint != nil && !typ.IsAbsentOrUnknown(solved) && !subtype.IsSubtype(solved, tp.Constraint) {
			typeArgs[i] = typ.Unknown
			continue
		}
		typeArgs[i] = solved
		solvedAny = true
	}
	if !solvedAny {
		return declared
	}
	out := make(map[int]typ.Type, len(declared))
	changed := false
	for slot, t := range declared {
		closed := subst.Params(t, typeParams, typeArgs)
		if !typ.TypeEquals(closed, t) {
			changed = true
		}
		out[slot] = closed
	}
	if !changed {
		return declared
	}
	return out
}

func collectDeclaredParamTypeParams(declared map[int]typ.Type) []*typ.TypeParam {
	if len(declared) == 0 {
		return nil
	}
	var out []*typ.TypeParam
	seen := make(map[*typ.TypeParam]bool)
	seenTypes := make(map[typ.Type]bool)
	slots := make([]int, 0, len(declared))
	for slot := range declared {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	for _, slot := range slots {
		collectTypeParams(declared[slot], seen, seenTypes, &out)
	}
	return out
}

func collectTypeParams(t typ.Type, seen map[*typ.TypeParam]bool, seenTypes map[typ.Type]bool, out *[]*typ.TypeParam) {
	if t == nil || seenTypes[t] {
		return
	}
	seenTypes[t] = true
	typ.Visit(t, typ.Visitor[struct{}]{
		TypeParam: func(tp *typ.TypeParam) struct{} {
			if tp != nil && !seen[tp] {
				seen[tp] = true
				*out = append(*out, tp)
			}
			return struct{}{}
		},
		Function: func(fn *typ.Function) struct{} {
			for _, p := range fn.Params {
				collectTypeParams(p.Type, seen, seenTypes, out)
			}
			collectTypeParams(fn.Variadic, seen, seenTypes, out)
			for _, r := range fn.Returns {
				collectTypeParams(r, seen, seenTypes, out)
			}
			return struct{}{}
		},
		Record: func(r *typ.Record) struct{} {
			for _, f := range r.Fields {
				collectTypeParams(f.Type, seen, seenTypes, out)
			}
			collectTypeParams(r.MapKey, seen, seenTypes, out)
			collectTypeParams(r.MapValue, seen, seenTypes, out)
			collectTypeParams(r.Metatable, seen, seenTypes, out)
			return struct{}{}
		},
		Array: func(a *typ.Array) struct{} {
			collectTypeParams(a.Element, seen, seenTypes, out)
			return struct{}{}
		},
		Map: func(m *typ.Map) struct{} {
			collectTypeParams(m.Key, seen, seenTypes, out)
			collectTypeParams(m.Value, seen, seenTypes, out)
			return struct{}{}
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) struct{} {
			collectTypeParams(m.Key, seen, seenTypes, out)
			collectTypeParams(m.Value, seen, seenTypes, out)
			return struct{}{}
		},
		Tuple: func(tup *typ.Tuple) struct{} {
			for _, e := range tup.Elements {
				collectTypeParams(e, seen, seenTypes, out)
			}
			return struct{}{}
		},
		Optional: func(o *typ.Optional) struct{} {
			collectTypeParams(o.Inner, seen, seenTypes, out)
			return struct{}{}
		},
		Union: func(u *typ.Union) struct{} {
			for _, m := range u.Members {
				collectTypeParams(m, seen, seenTypes, out)
			}
			return struct{}{}
		},
		Intersection: func(in *typ.Intersection) struct{} {
			for _, m := range in.Members {
				collectTypeParams(m, seen, seenTypes, out)
			}
			return struct{}{}
		},
		Alias: func(a *typ.Alias) struct{} {
			collectTypeParams(a.Target, seen, seenTypes, out)
			return struct{}{}
		},
		Instantiated: func(inst *typ.Instantiated) struct{} {
			for _, arg := range inst.TypeArgs {
				collectTypeParams(arg, seen, seenTypes, out)
			}
			return struct{}{}
		},
		Default: func(typ.Type) struct{} {
			return struct{}{}
		},
	})
}

func entrySeedValue(declared typ.Type, entry, contract product.AbstractValue) product.AbstractValue {
	av := entrySeedDeclaredValue(declared, entry)
	if av.IsZero() && !entry.IsZero() {
		av = entry
	}
	if !contract.IsZero() {
		if !paramevidence.IsInformative(contract.ProjectValue()) {
			return av
		}
		if av.IsZero() {
			av = contract
		}
		// With a declared annotation or exact caller entry value, keep `av` as the
		// declared/entry composition. Refining it by the demand creates a
		// self-fulfilling precondition: the parameter becomes what the body demands,
		// guard complements vanish, and declared `any` is mistaken for concrete proof.
	}
	return av
}

func entrySeedDeclaredValue(declared typ.Type, entry product.AbstractValue) product.AbstractValue {
	if declared == nil {
		return product.AbstractValue{}
	}
	if typ.ContainsFreeTypeParam(declared) && entryHasClosedInformativeValue(entry) {
		// An open generic annotation (`T`, `{T}`, ...) is a binder constraint, not
		// a closed runtime fact. Once the call-entry context supplies a closed value,
		// entry state must carry that value so the body is interpreted under the
		// instantiated call, not under the callee's binder syntax.
		return entry
	}
	if !entry.IsZero() {
		if evidence := entry.ProjectValue(); evidence != nil && !typ.IsAbsentOrUnknown(evidence) {
			if refined, changed := value.RefineStructuralAnnotation(declared, evidence, typ.JoinPreferNonSoft); changed {
				return product.FromType(refined)
			}
		}
	}
	return product.FromType(declared)
}

func entryHasClosedInformativeValue(entry product.AbstractValue) bool {
	if entry.IsZero() {
		return false
	}
	t := entry.ProjectValue()
	return t != nil && !typ.IsAbsentOrUnknown(t) && !typ.ContainsFreeTypeParam(t)
}
