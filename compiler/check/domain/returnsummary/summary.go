package returnsummary

import (
	"github.com/wippyai/go-lua/compiler/check/domain/value"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Equal checks whether two return vectors are structurally equal.
func Equal(a, b []typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !typ.TypeEquals(a[i], b[i]) {
			return false
		}
	}
	return true
}

// AllNil reports whether every return slot is explicit nil.
func AllNil(rets []typ.Type) bool {
	if len(rets) == 0 {
		return false
	}
	for _, t := range rets {
		if t == nil || t.Kind() != kind.Nil {
			return false
		}
	}
	return true
}

// Refines reports whether a is an element-wise subtype refinement of b.
func Refines(a, b []typ.Type) bool {
	if len(a) == 0 {
		return false
	}
	if len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ai := a[i]
		bi := b[i]
		if ai == nil || bi == nil {
			if ai == nil && bi == nil {
				continue
			}
			return false
		}
		if !subtype.IsSubtype(ai, bi) {
			return false
		}
	}
	return true
}

// ExtendsRecord reports whether a refines b by adding record fields.
func ExtendsRecord(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if _, ok := a[i].(*typ.Record); !ok {
			return false
		}
		if !value.ExtendsRecord(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ElidesOptional reports whether a refines b by removing nil/optional parts.
func ElidesOptional(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if !value.ElidesOptional(a[i], b[i]) {
			return false
		}
	}
	return true
}

// SelectPreferred picks a canonical winner when one return vector is strictly
// preferable to the other without requiring a join.
func SelectPreferred(a, b []typ.Type) ([]typ.Type, bool) {
	if RepairsNever(a, b) {
		return a, true
	}
	if RepairsNever(b, a) {
		return b, true
	}
	if RefinesSoftContainers(a, b) {
		return a, true
	}
	if RefinesSoftContainers(b, a) {
		return b, true
	}
	if StopsRecursiveStructuralGrowth(a, b) {
		return a, true
	}
	if StopsRecursiveStructuralGrowth(b, a) {
		return b, true
	}
	if RefinesFalsyMapKeys(a, b) {
		return a, true
	}
	if RefinesFalsyMapKeys(b, a) {
		return b, true
	}
	if Refines(a, b) {
		if AllNil(a) && !AllNil(b) {
			return b, true
		}
		if NestedNilOnlyRegression(a, b) {
			return b, true
		}
		return a, true
	}
	if Refines(b, a) {
		if AllNil(b) && !AllNil(a) {
			return a, true
		}
		if NestedNilOnlyRegression(b, a) {
			return a, true
		}
		return b, true
	}
	if FillsNilSlots(a, b) {
		return a, true
	}
	if FillsNilSlots(b, a) {
		return b, true
	}
	if (ExtendsRecord(a, b) || ElidesOptional(a, b)) && !NestedNilOnlyRegression(a, b) {
		return a, true
	}
	if (ExtendsRecord(b, a) || ElidesOptional(b, a)) && !NestedNilOnlyRegression(b, a) {
		return b, true
	}
	return nil, false
}

// RefinesSoftContainers reports whether candidate preserves the same table
// shape while replacing soft placeholder element/value evidence with concrete
// evidence.
func RefinesSoftContainers(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	strict := false
	for i := range candidate {
		refines, changed := value.RefinesSoftContainer(candidate[i], baseline[i])
		if !refines {
			return false
		}
		if changed {
			strict = true
		}
	}
	return strict
}

// RefinesFalsyMapKeys reports whether candidate is the same table-derived shape
// as baseline after removing stale falsy members from baseline.
func RefinesFalsyMapKeys(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	strict := false
	for i := range candidate {
		refines, changed := value.RefinesFalsyMapKey(candidate[i], baseline[i])
		if !refines {
			return false
		}
		if changed {
			strict = true
		}
	}
	return strict
}

// NestedNilOnlyRegression reports whether candidate's apparent refinement only
// adds nested nil facts over a more useful baseline shape.
func NestedNilOnlyRegression(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	for i := range candidate {
		if value.NestedNilOnlyRegression(candidate[i], baseline[i]) {
			return true
		}
	}
	return false
}

// StopsRecursiveStructuralGrowth reports whether growing embeds the same
// structural container shape as stable beneath its root.
func StopsRecursiveStructuralGrowth(stable, growing []typ.Type) bool {
	if len(stable) == 0 || len(growing) == 0 || len(stable) != len(growing) {
		return false
	}

	strict := false
	for i := range stable {
		s := stable[i]
		g := growing[i]
		if s == nil || g == nil {
			return false
		}
		if typ.TypeEquals(s, g) {
			continue
		}
		if typ.IsAbsentOrUnknown(s) || !value.CanSelfEmbed(s) {
			return false
		}
		if !value.ShallowStructuralShapeEquals(g, s) {
			return false
		}
		if !value.ContainsNestedStructuralShape(g, s) {
			return false
		}
		strict = true
	}
	return strict
}

// SelectRefining prefers candidate only when it directionally refines baseline.
func SelectRefining(candidate, baseline []typ.Type) ([]typ.Type, bool) {
	if Refines(candidate, baseline) {
		if AllNil(candidate) && !AllNil(baseline) {
			return baseline, true
		}
		return candidate, true
	}
	if FillsNilSlots(candidate, baseline) {
		return candidate, true
	}
	if ExtendsRecord(candidate, baseline) || ElidesOptional(candidate, baseline) {
		return candidate, true
	}
	return nil, false
}

// FillsNilSlots reports whether a improves b by replacing nil-only slots with
// concrete return evidence while staying compatible on other slots.
func FillsNilSlots(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	strict := false
	for i := range a {
		ai := a[i]
		bi := b[i]
		if ai == nil || bi == nil {
			return false
		}
		if unwrap.IsNilType(bi) && !unwrap.IsNilType(ai) {
			strict = true
			continue
		}
		if typ.TypeEquals(ai, bi) {
			continue
		}
		if subtype.IsSubtype(ai, bi) || value.ExtendsRecord(ai, bi) || value.ElidesOptional(ai, bi) {
			continue
		}
		return false
	}
	return strict
}

// RepairsNever reports whether candidate is a runtime-possible repair of
// baseline by replacing nested never artifacts while otherwise widening
// compatibly.
func RepairsNever(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	strict := false
	for i := range candidate {
		if candidate[i] == nil || baseline[i] == nil {
			return false
		}
		if typ.TypeEquals(candidate[i], baseline[i]) {
			continue
		}
		if !repairsNever(candidate[i], baseline[i]) {
			return false
		}
		strict = true
	}
	return strict
}

func repairsNever(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil {
		return false
	}
	if !containsNever(baseline) || containsNever(candidate) {
		return false
	}
	ok, strict := neverRepairRelation(candidate, baseline)
	return ok && strict
}

func neverRepairRelation(candidate, baseline typ.Type) (bool, bool) {
	if candidate == nil || baseline == nil {
		return false, false
	}
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}

	candidate = unwrap.Alias(candidate)
	baseline = unwrap.Alias(baseline)
	if candidate == nil || baseline == nil {
		return false, false
	}

	if typ.IsNever(baseline) {
		return !typ.IsNever(candidate), !typ.IsNever(candidate)
	}
	if !containsNever(baseline) {
		return false, false
	}

	switch b := baseline.(type) {
	case *typ.Optional:
		c, ok := candidate.(*typ.Optional)
		if !ok {
			return false, false
		}
		return neverRepairRelation(c.Inner, b.Inner)
	case *typ.Union:
		c, ok := candidate.(*typ.Union)
		if !ok || len(c.Members) != len(b.Members) {
			return false, false
		}
		used := make([]bool, len(c.Members))
		strict := false
		for _, bm := range b.Members {
			matched := false
			for j, cm := range c.Members {
				if used[j] || !typ.TypeEquals(cm, bm) {
					continue
				}
				used[j] = true
				matched = true
				break
			}
			if matched {
				continue
			}
			for j, cm := range c.Members {
				if used[j] {
					continue
				}
				ok, repaired := neverRepairRelation(cm, bm)
				if !ok {
					continue
				}
				used[j] = true
				matched = true
				if repaired {
					strict = true
				}
				break
			}
			if !matched {
				return false, false
			}
		}
		return true, strict
	case *typ.Record:
		c, ok := candidate.(*typ.Record)
		if !ok || c.Open != b.Open || c.HasMapComponent() != b.HasMapComponent() || len(c.Fields) != len(b.Fields) {
			return false, false
		}
		strict := false
		for _, bf := range b.Fields {
			cf := c.GetField(bf.Name)
			if cf == nil || cf.Optional != bf.Optional || cf.Readonly != bf.Readonly {
				return false, false
			}
			ok, repaired := neverRepairRelation(cf.Type, bf.Type)
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
		}
		if b.HasMapComponent() {
			ok, repaired := neverRepairRelation(c.MapKey, b.MapKey)
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
			ok, repaired = neverRepairRelation(c.MapValue, b.MapValue)
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
		}
		if b.Metatable != nil || c.Metatable != nil {
			if b.Metatable == nil || c.Metatable == nil {
				return false, false
			}
			ok, repaired := neverRepairRelation(c.Metatable, b.Metatable)
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
		}
		return true, strict
	case *typ.Array:
		c, ok := candidate.(*typ.Array)
		if !ok {
			return false, false
		}
		return neverRepairRelation(c.Element, b.Element)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			return false, false
		}
		keyOK, keyStrict := neverRepairRelation(c.Key, b.Key)
		if !keyOK {
			return false, false
		}
		valOK, valStrict := neverRepairRelation(c.Value, b.Value)
		if !valOK {
			return false, false
		}
		return true, keyStrict || valStrict
	case *typ.Tuple:
		c, ok := candidate.(*typ.Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			return false, false
		}
		strict := false
		for i := range b.Elements {
			ok, repaired := neverRepairRelation(c.Elements[i], b.Elements[i])
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
		}
		return true, strict
	case *typ.Function:
		c, ok := candidate.(*typ.Function)
		if !ok || !sameFunctionShapeForRepair(c, b) || len(c.Returns) != len(b.Returns) {
			return false, false
		}
		for i := range b.Params {
			if c.Params[i].Name != b.Params[i].Name ||
				c.Params[i].Optional != b.Params[i].Optional ||
				!typ.TypeEquals(c.Params[i].Type, b.Params[i].Type) {
				return false, false
			}
		}
		switch {
		case (c.Variadic == nil) != (b.Variadic == nil):
			return false, false
		case c.Variadic != nil && !typ.TypeEquals(c.Variadic, b.Variadic):
			return false, false
		}
		strict := false
		for i := range b.Returns {
			ok, repaired := neverRepairRelation(c.Returns[i], b.Returns[i])
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
		}
		return true, strict
	default:
		return false, false
	}
}

func sameFunctionShapeForRepair(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return false
	}
	if !typeParamsEqual(a.TypeParams, b.TypeParams) {
		return false
	}
	return len(a.Params) == len(b.Params)
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

func containsNever(t typ.Type) bool {
	seen := make(map[typ.Type]bool)
	return containsNeverMemo(t, seen)
}

func containsNeverMemo(t typ.Type, seen map[typ.Type]bool) bool {
	if t == nil {
		return false
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	if typ.IsNever(t) {
		return true
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return containsNeverMemo(o.Inner, seen)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if containsNeverMemo(m, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if containsNeverMemo(m, seen) {
					return true
				}
			}
			return false
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, e := range tup.Elements {
				if containsNeverMemo(e, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return containsNeverMemo(a.Element, seen)
		},
		Map: func(m *typ.Map) bool {
			return containsNeverMemo(m.Key, seen) || containsNeverMemo(m.Value, seen)
		},
		Record: func(r *typ.Record) bool {
			for _, f := range r.Fields {
				if containsNeverMemo(f.Type, seen) {
					return true
				}
			}
			if r.HasMapComponent() {
				return containsNeverMemo(r.MapKey, seen) || containsNeverMemo(r.MapValue, seen)
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
			for _, p := range fn.Params {
				if containsNeverMemo(p.Type, seen) {
					return true
				}
			}
			if fn.Variadic != nil && containsNeverMemo(fn.Variadic, seen) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsNeverMemo(ret, seen) {
					return true
				}
			}
			return false
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}

// Normalize replaces nil slots with explicit nil types in a copied vector.
func Normalize(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	out := make([]typ.Type, len(rets))
	copy(out, rets)
	return NormalizeOwned(out)
}

// NormalizeOwned replaces nil slots with explicit nil types in an owned vector.
func NormalizeOwned(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	for i, t := range rets {
		if t == nil {
			rets[i] = typ.Nil
		}
	}
	return rets
}

// Canonical returns a vector with explicit nil slots, reusing the input when it
// is already canonical.
func Canonical(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	for i, t := range rets {
		if t != nil {
			continue
		}
		out := make([]typ.Type, len(rets))
		copy(out, rets)
		out[i] = typ.Nil
		for j := i + 1; j < len(out); j++ {
			if out[j] == nil {
				out[j] = typ.Nil
			}
		}
		return out
	}
	return rets
}

// NormalizeAndPrune canonicalizes nil slots and removes soft union members.
func NormalizeAndPrune(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	var out []typ.Type
	for i, ret := range rets {
		normalized := ret
		if normalized == nil {
			normalized = typ.Nil
		}
		pruned := typ.PruneSoftUnionMembers(normalized)
		if out != nil {
			out[i] = pruned
			continue
		}
		if pruned == ret {
			continue
		}
		out = make([]typ.Type, len(rets))
		copy(out, rets[:i])
		out[i] = pruned
	}
	if out != nil {
		return out
	}
	return rets
}

// Merge applies the canonical return-summary merge policy shared by iterative
// channels.
func Merge(existing, candidate []typ.Type) []typ.Type {
	existing = NormalizeAndPrune(existing)
	candidate = NormalizeAndPrune(candidate)
	if len(existing) == 0 {
		return candidate
	}
	if len(candidate) == 0 {
		return existing
	}
	if replaced, ok := replaceOpenTopWithStructured(existing, candidate); ok {
		existing = NormalizeAndPrune(replaced)
	}
	if RepairsNever(existing, candidate) {
		return existing
	}
	if RepairsNever(candidate, existing) {
		return candidate
	}
	if shouldUseMonotoneJoin(existing, candidate) {
		return NormalizeAndPrune(joinMonotone(existing, candidate))
	}
	if preferred, ok := SelectPreferred(existing, candidate); ok {
		return NormalizeAndPrune(preferred)
	}
	return NormalizeAndPrune(typjoin.ReturnVectors(existing, candidate))
}

// WidenForConvergence merges return vectors at a recursive fixpoint boundary.
func WidenForConvergence(prev, next []typ.Type) []typ.Type {
	prev = NormalizeAndPrune(prev)
	next = NormalizeAndPrune(next)
	if len(prev) == 0 {
		return WidenVectorForConvergence(next)
	}
	if len(next) == 0 {
		return WidenVectorForConvergence(prev)
	}

	merged := Merge(prev, next)
	if UnsafePrecisionDrop(prev, merged) {
		merged = prev
	}
	return WidenVectorForConvergence(NormalizeAndPrune(merged))
}

// WidenVectorForConvergence applies element-wise convergence widening.
func WidenVectorForConvergence(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return rets
	}
	out := make([]typ.Type, len(rets))
	changed := false
	for i, t := range rets {
		wt := value.WidenForConvergence(t)
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

// UnsafePrecisionDrop reports whether a merged vector lost prior evidence.
func UnsafePrecisionDrop(prev, merged []typ.Type) bool {
	if len(prev) == 0 || len(merged) == 0 || len(prev) != len(merged) {
		return false
	}
	for i := range prev {
		if value.UnsafePrecisionDrop(prev[i], merged[i]) {
			return true
		}
	}
	return false
}

func shouldUseMonotoneJoin(a, b []typ.Type) bool {
	for _, t := range a {
		if value.HasHigherOrderGrowthRisk(t) {
			return true
		}
	}
	for _, t := range b {
		if value.HasHigherOrderGrowthRisk(t) {
			return true
		}
	}
	return false
}

func joinMonotone(a, b []typ.Type) []typ.Type {
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
		out[i] = joinTypeMonotone(ai, bi)
	}
	return out
}

func joinTypeMonotone(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	if subtype.IsSubtype(a, b) || value.ExtendsRecord(a, b) || value.ElidesOptional(a, b) {
		return b
	}
	if subtype.IsSubtype(b, a) || value.ExtendsRecord(b, a) || value.ElidesOptional(b, a) {
		return a
	}
	return typ.JoinPreferNonSoft(a, b)
}

// AlignFunction applies the canonical return-summary winner to a function type.
func AlignFunction(fn *typ.Function, summary []typ.Type) (*typ.Function, bool) {
	if fn == nil {
		return nil, false
	}

	normalizedSummary := NormalizeAndPrune(summary)
	if len(normalizedSummary) == 0 {
		return fn, false
	}

	current := NormalizeAndPrune(fn.Returns)
	if len(current) == 0 {
		aligned := typjoin.WithReturns(fn, normalizedSummary)
		return aligned, aligned != nil
	}
	merged := Merge(current, normalizedSummary)
	if Equal(current, merged) {
		return fn, false
	}

	aligned := typjoin.WithReturns(fn, merged)
	if aligned == nil {
		return fn, false
	}
	return aligned, true
}

func replaceOpenTopWithStructured(current, summary []typ.Type) ([]typ.Type, bool) {
	if len(current) == 0 || len(summary) == 0 || len(current) != len(summary) {
		return nil, false
	}
	out := append([]typ.Type(nil), current...)
	changed := false
	for i := range out {
		if !value.IsOpenTopRecord(out[i]) {
			continue
		}
		if !value.IsStructuredTableShape(summary[i]) {
			continue
		}
		out[i] = summary[i]
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
}

// ApplyToFunctionType applies summary-derived returns to a function signature.
// If both summary and signature returns are empty, it attaches unknown to keep
// call-site checking conservative.
func ApplyToFunctionType(fn *typ.Function, summary []typ.Type) *typ.Function {
	if fn == nil {
		return nil
	}
	if len(summary) == 0 {
		if len(fn.Returns) > 0 {
			return fn
		}
		return typjoin.WithReturns(fn, []typ.Type{typ.Unknown})
	}
	if aligned, changed := AlignFunction(fn, summary); changed {
		return aligned
	}
	if len(fn.Returns) > 0 {
		return fn
	}
	return typjoin.WithReturns(fn, NormalizeAndPrune(summary))
}
