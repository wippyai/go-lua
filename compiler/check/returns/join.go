package returns

import (
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReturnTypesEqual checks if two return vectors are structurally equal.
func ReturnTypesEqual(a, b []typ.Type) bool {
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

// ReturnTypesAllNil reports whether all slots are explicit nil.
func ReturnTypesAllNil(rets []typ.Type) bool {
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

// ReturnTypesRefine reports whether a refines b: element-wise a subtype of b
// that keeps the record fields b has (see coversRecordFields).
func ReturnTypesRefine(a, b []typ.Type) bool {
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
		if !subtype.IsSubtype(ai, bi) || !coversRecordFields(ai, bi) {
			return false
		}
	}
	return true
}

// ReturnTypesExtendRecord reports whether a extends b by adding record fields.
// This treats record field supersets as refinements for return summaries.
func ReturnTypesExtendRecord(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ar, ok := a[i].(*typ.Record)
		if !ok {
			return false
		}
		switch br := b[i].(type) {
		case *typ.Record:
			if !recordSuperset(ar, br) {
				return false
			}
		case *typ.Union:
			if !recordSupersetUnion(ar, br) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ReturnTypesElideOptional reports whether a refines b by removing nil/optional parts.
func ReturnTypesElideOptional(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !typeElidesOptional(a[i], b[i]) {
			return false
		}
	}
	return true
}

// SelectPreferredReturnVector picks a canonical winner when one return vector
// is strictly preferable to the other without requiring a join.
//
// Preference order:
//  1. subtype refinement (with nil-only regression protection)
//  2. record extension
//  3. optional elision
//
// The nil-only guard prevents a refined-but-empty-looking update from
// regressing an already informative summary to just nil.
func SelectPreferredReturnVector(a, b []typ.Type) ([]typ.Type, bool) {
	if ReturnTypesRepairNever(a, b) {
		return a, true
	}
	if ReturnTypesRepairNever(b, a) {
		return b, true
	}
	if ReturnTypesRefine(a, b) {
		if ReturnTypesAllNil(a) && !ReturnTypesAllNil(b) {
			return b, true
		}
		return a, true
	}
	if ReturnTypesRefine(b, a) {
		if ReturnTypesAllNil(b) && !ReturnTypesAllNil(a) {
			return a, true
		}
		return b, true
	}
	if ReturnTypesFillNilSlots(a, b) {
		return a, true
	}
	if ReturnTypesFillNilSlots(b, a) {
		return b, true
	}
	if ReturnTypesExtendRecord(a, b) || ReturnTypesElideOptional(a, b) {
		return a, true
	}
	if ReturnTypesExtendRecord(b, a) || ReturnTypesElideOptional(b, a) {
		return b, true
	}
	return nil, false
}

// SelectRefiningReturnVector prefers candidate only when it is a directional
// refinement of baseline. It never prefers baseline over candidate.
//
// This is used in iterative channels where an older baseline may be an
// under-constrained artifact; in those cases we must not lock in baseline just
// because it happens to be a subtype of the newer estimate.
func SelectRefiningReturnVector(candidate, baseline []typ.Type) ([]typ.Type, bool) {
	if ReturnTypesRefine(candidate, baseline) {
		if ReturnTypesAllNil(candidate) && !ReturnTypesAllNil(baseline) {
			return baseline, true
		}
		return candidate, true
	}
	if ReturnTypesFillNilSlots(candidate, baseline) {
		return candidate, true
	}
	if ReturnTypesExtendRecord(candidate, baseline) || ReturnTypesElideOptional(candidate, baseline) {
		return candidate, true
	}
	return nil, false
}

// ReturnTypesFillNilSlots reports whether a improves b by replacing nil-only
// slots with concrete return evidence while staying compatible on other slots.
func ReturnTypesFillNilSlots(a, b []typ.Type) bool {
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
		if subtype.IsSubtype(ai, bi) || TypeExtendsRecord(ai, bi) || typeElidesOptional(ai, bi) {
			continue
		}
		return false
	}
	return strict
}

// ReturnTypesRepairNever reports whether candidate is a runtime-possible repair
// of baseline by replacing nested never artifacts while otherwise widening
// compatibly. This lets post-flow summaries correct pre-flow bottoms such as
// `{data?: never}` -> `{data?: unknown}`.
func ReturnTypesRepairNever(candidate, baseline []typ.Type) bool {
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
		if !typeRepairsNever(candidate[i], baseline[i]) {
			return false
		}
		strict = true
	}
	return strict
}

// TypeExtendsRecord reports whether type a extends type b by adding record fields.
// This treats record field supersets as refinements when b is a record or union of records.
func TypeExtendsRecord(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	ar, ok := a.(*typ.Record)
	if !ok {
		return false
	}
	switch br := b.(type) {
	case *typ.Record:
		return recordSuperset(ar, br)
	case *typ.Union:
		return recordSupersetUnion(ar, br)
	default:
		return false
	}
}

func typeRepairsNever(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil {
		return false
	}
	if !typeContainsNever(baseline) || typeContainsNever(candidate) {
		return false
	}
	ok, strict := typeNeverRepairRelation(candidate, baseline)
	return ok && strict
}

func typeNeverRepairRelation(candidate, baseline typ.Type) (bool, bool) {
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
	if !typeContainsNever(baseline) {
		return false, false
	}

	switch b := baseline.(type) {
	case *typ.Optional:
		c, ok := candidate.(*typ.Optional)
		if !ok {
			return false, false
		}
		return typeNeverRepairRelation(c.Inner, b.Inner)
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
				ok, repaired := typeNeverRepairRelation(cm, bm)
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
			if cf == nil || cf.Optional != bf.Optional || cf.InferredPresence != bf.InferredPresence || cf.Readonly != bf.Readonly {
				return false, false
			}
			ok, repaired := typeNeverRepairRelation(cf.Type, bf.Type)
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
		}
		if b.HasMapComponent() {
			ok, repaired := typeNeverRepairRelation(c.MapKey, b.MapKey)
			if !ok {
				return false, false
			}
			if repaired {
				strict = true
			}
			ok, repaired = typeNeverRepairRelation(c.MapValue, b.MapValue)
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
			ok, repaired := typeNeverRepairRelation(c.Metatable, b.Metatable)
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
		return typeNeverRepairRelation(c.Element, b.Element)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			return false, false
		}
		keyOK, keyStrict := typeNeverRepairRelation(c.Key, b.Key)
		if !keyOK {
			return false, false
		}
		valOK, valStrict := typeNeverRepairRelation(c.Value, b.Value)
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
			ok, repaired := typeNeverRepairRelation(c.Elements[i], b.Elements[i])
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
		if !ok || !sameFunctionShapeForFactMerge(c, b) || len(c.Returns) != len(b.Returns) {
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
			ok, repaired := typeNeverRepairRelation(c.Returns[i], b.Returns[i])
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

func typeContainsNever(t typ.Type) bool {
	seen := make(map[typ.Type]bool)
	return typeContainsNeverMemo(t, seen)
}

func typeContainsNeverMemo(t typ.Type, seen map[typ.Type]bool) bool {
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
			return typeContainsNeverMemo(o.Inner, seen)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if typeContainsNeverMemo(m, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if typeContainsNeverMemo(m, seen) {
					return true
				}
			}
			return false
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, e := range tup.Elements {
				if typeContainsNeverMemo(e, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return typeContainsNeverMemo(a.Element, seen)
		},
		Map: func(m *typ.Map) bool {
			return typeContainsNeverMemo(m.Key, seen) || typeContainsNeverMemo(m.Value, seen)
		},
		Record: func(r *typ.Record) bool {
			for _, f := range r.Fields {
				if typeContainsNeverMemo(f.Type, seen) {
					return true
				}
			}
			if r.HasMapComponent() {
				return typeContainsNeverMemo(r.MapKey, seen) || typeContainsNeverMemo(r.MapValue, seen)
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
			for _, p := range fn.Params {
				if typeContainsNeverMemo(p.Type, seen) {
					return true
				}
			}
			if fn.Variadic != nil && typeContainsNeverMemo(fn.Variadic, seen) {
				return true
			}
			for _, ret := range fn.Returns {
				if typeContainsNeverMemo(ret, seen) {
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

func typeElidesOptional(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	nonNil := narrow.RemoveNil(b)
	if nonNil == nil || typ.TypeEquals(nonNil, b) {
		return false
	}
	return subtype.IsSubtype(a, nonNil)
}

func recordSuperset(newRec, oldRec *typ.Record) bool {
	if newRec == nil || oldRec == nil {
		return false
	}
	if oldRec.Metatable != nil {
		if newRec.Metatable == nil || !subtype.IsSubtype(newRec.Metatable, oldRec.Metatable) {
			return false
		}
	}
	if oldRec.HasMapComponent() {
		if !newRec.HasMapComponent() {
			return false
		}
		if !subtype.IsSubtype(newRec.MapKey, oldRec.MapKey) || !subtype.IsSubtype(newRec.MapValue, oldRec.MapValue) {
			return false
		}
	}
	oldFields := make(map[string]typ.Field, len(oldRec.Fields))
	for _, f := range oldRec.Fields {
		oldFields[f.Name] = f
	}
	for _, nf := range newRec.Fields {
		if of, ok := oldFields[nf.Name]; ok {
			if of.Optional && !nf.Optional {
				// ok: stronger requirement
			} else if !of.Optional && nf.Optional {
				return false
			}
			if of.Readonly && !nf.Readonly {
				return false
			}
			if of.Type != nil {
				if isOpenTopRecordType(nf.Type) && isStructuredTableShape(of.Type) {
					// Open-top table placeholders must not dominate structured
					// collection/record fields when selecting preferred summaries.
					return false
				}
				if nf.Type == nil || !subtype.IsSubtype(nf.Type, of.Type) {
					return false
				}
			}
			delete(oldFields, nf.Name)
		}
	}
	return len(oldFields) == 0
}

func recordSupersetUnion(newRec *typ.Record, oldUnion *typ.Union) bool {
	if newRec == nil || oldUnion == nil {
		return false
	}
	if len(oldUnion.Members) == 0 {
		return false
	}
	for _, member := range oldUnion.Members {
		oldRec, ok := member.(*typ.Record)
		if !ok {
			return false
		}
		if !recordSuperset(newRec, oldRec) {
			return false
		}
	}
	return true
}

// NormalizeReturnVector replaces nil slots with explicit nil types.
func NormalizeReturnVector(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	out := make([]typ.Type, len(rets))
	for i, t := range rets {
		if t == nil {
			out[i] = typ.Nil
		} else {
			out[i] = t
		}
	}
	return out
}

func normalizeAndPruneReturnVector(rets []typ.Type) []typ.Type {
	out := NormalizeReturnVector(rets)
	if len(out) == 0 {
		return nil
	}
	for i, ret := range out {
		out[i] = typ.PruneSoftUnionMembers(ret)
	}
	return out
}

// MergeReturnSummary applies the canonical return-summary merge policy shared by
// all iterative channels (SCC return inference, interproc fact widening, and
// summary-to-signature alignment). Centralizing this logic prevents divergent
// local merge behavior across phases.
func MergeReturnSummary(existing, candidate []typ.Type) []typ.Type {
	existing = normalizeAndPruneReturnVector(existing)
	candidate = normalizeAndPruneReturnVector(candidate)
	if len(existing) == 0 {
		return candidate
	}
	if len(candidate) == 0 {
		return existing
	}
	existing, candidate = refineSnapshotSlots(existing, candidate)
	existing = fillPendingSlots(existing, candidate)
	candidate = fillPendingSlots(candidate, existing)
	existing = fillUnknownSlots(existing, candidate)
	candidate = fillUnknownSlots(candidate, existing)
	// Canonical promotion: open-top record placeholders should not dominate
	// concrete structured return evidence (array/map/record with fields).
	if replaced, ok := replaceOpenTopWithStructured(existing, candidate); ok {
		existing = normalizeAndPruneReturnVector(replaced)
	}
	if ReturnTypesRepairNever(existing, candidate) {
		return existing
	}
	if ReturnTypesRepairNever(candidate, existing) {
		return candidate
	}
	// A concrete record result can appear only after a forwarded callee is
	// resolved. Keep that success branch alongside the earlier nil error arm;
	// a nil subtype is not a reason to discard runtime value evidence.
	if nilSlotGainsRecordEvidence(existing, candidate) {
		return candidate
	}
	if nilSlotGainsRecordEvidence(candidate, existing) {
		return existing
	}

	// Higher-order summaries are merged monotonically for fixpoint stability.
	if shouldUseMonotoneReturnJoin(existing, candidate) {
		return normalizeAndPruneReturnVector(joinReturnVectorsMonotone(existing, candidate))
	}

	if preferred, ok := SelectPreferredReturnVector(existing, candidate); ok {
		return normalizeAndPruneReturnVector(preferred)
	}

	return normalizeAndPruneReturnVector(refineSnapshotMembersVector(typjoin.ReturnVectors(existing, candidate)))
}

func nilSlotGainsRecordEvidence(old, next []typ.Type) bool {
	if len(old) != len(next) {
		return false
	}
	for i := range old {
		if !unwrap.IsNilType(old[i]) || unwrap.IsNilType(next[i]) || !unwrap.IsOptionalLike(next[i]) {
			continue
		}
		if record, ok := unwrap.Alias(narrow.RemoveNil(next[i])).(*typ.Record); ok && len(record.Fields) > 0 {
			return true
		}
	}
	return false
}

// AdvanceReturnSummary merges the previous estimate of a function's returns
// with the estimate the next fixpoint iteration infers from it. The next
// estimate supersedes a comparable previous one: it either refines it or adds
// branches the previous iteration could not infer yet, such as those behind a
// recursive call whose summary was still a placeholder. Incomparable estimates
// join, and placeholder slots (nil-only, never artifacts, open-top records)
// never displace evidence.
func AdvanceReturnSummary(prev, next []typ.Type) []typ.Type {
	prev = normalizeAndPruneReturnVector(prev)
	next = normalizeAndPruneReturnVector(next)
	if len(prev) == 0 {
		return next
	}
	if len(next) == 0 {
		return prev
	}
	prev = fillPendingSlots(prev, next)
	next = fillPendingSlots(next, prev)
	prev = fillUnknownSlots(prev, next)
	next = fillUnknownSlots(next, prev)
	if replaced, ok := replaceOpenTopWithStructured(prev, next); ok {
		prev = normalizeAndPruneReturnVector(replaced)
	}
	if replaced, ok := replaceOpenTopWithStructured(next, prev); ok {
		next = normalizeAndPruneReturnVector(replaced)
	}
	if ReturnTypesRepairNever(prev, next) {
		return prev
	}
	if ReturnTypesRepairNever(next, prev) {
		return next
	}
	if shouldUseMonotoneReturnJoin(prev, next) {
		return normalizeAndPruneReturnVector(joinReturnVectorsMonotone(prev, next))
	}
	if ReturnTypesFillNilSlots(prev, next) || (ReturnTypesAllNil(next) && !ReturnTypesAllNil(prev)) {
		return prev
	}
	if returnVectorsComparable(prev, next) {
		return next
	}
	return normalizeAndPruneReturnVector(typjoin.ReturnVectors(prev, next))
}

// fillUnknownSlots replaces unresolved slots with concrete evidence from the
// other estimate. A nil-only slot supplies no value evidence: replacing an
// unknown return with nil would prematurely close an unresolved capture.
func fillUnknownSlots(rets, other []typ.Type) []typ.Type {
	var out []typ.Type
	for i, t := range rets {
		if i >= len(other) || !typ.IsUnknown(t) || other[i] == nil || typ.IsUnknown(other[i]) || other[i].Kind() == kind.Nil {
			continue
		}
		if out == nil {
			out = append([]typ.Type(nil), rets...)
		}
		out[i] = other[i]
	}
	if out == nil {
		return rets
	}
	return out
}

// fillPendingSlots uses a later estimate of the same function return slot.
// A nil-only estimate does not close a return path whose value is still pending.
func fillPendingSlots(rets, other []typ.Type) []typ.Type {
	var out []typ.Type
	for i, t := range rets {
		if i >= len(other) || !typ.IsUnresolved(t) || other[i] == nil || typ.IsUnresolved(other[i]) || other[i].Kind() == kind.Nil {
			continue
		}
		if out == nil {
			out = append([]typ.Type(nil), rets...)
		}
		out[i] = typ.Resolve(t, other[i])
	}
	if out == nil {
		return rets
	}
	return out
}

// returnVectorsComparable reports whether one vector refines the other.
func returnVectorsComparable(a, b []typ.Type) bool {
	for _, pair := range [2][2][]typ.Type{{a, b}, {b, a}} {
		x, y := pair[0], pair[1]
		if ReturnTypesRefine(x, y) || ReturnTypesFillNilSlots(x, y) || ReturnTypesExtendRecord(x, y) || ReturnTypesElideOptional(x, y) {
			return true
		}
	}
	return false
}

// MergeFunctionFactType merges function-type facts through one canonical policy.
// This ensures all channels agree on when to preserve shape and how to merge
// returns, avoiding directional one-off behavior in individual phases.
func MergeFunctionFactType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if mergedFromVariants, ok := mergeFunctionFactVariants(existing, candidate); ok {
		return mergedFromVariants
	}
	if existingFn != nil && candidateFn != nil {
		if sameFunctionShapeForFactMerge(existingFn, candidateFn) {
			return mergeFunctionFactsByShape(existingFn, candidateFn, false)
		}
	}

	if subtype.IsSubtype(existing, candidate) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) {
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

func mergeFunctionFactVariants(existing, candidate typ.Type) (typ.Type, bool) {
	_, existingUnion := unwrap.Alias(existing).(*typ.Union)
	_, candidateUnion := unwrap.Alias(candidate).(*typ.Union)
	if !existingUnion && !candidateUnion {
		return nil, false
	}
	existingFns := functionVariantsForFactMerge(existing)
	candidateFns := functionVariantsForFactMerge(candidate)
	if len(existingFns) == 0 || len(candidateFns) == 0 {
		return nil, false
	}
	all := make([]*typ.Function, 0, len(existingFns)+len(candidateFns))
	all = append(all, existingFns...)
	all = append(all, candidateFns...)
	for i := 1; i < len(all); i++ {
		if !sameFunctionShapeForFactMerge(all[0], all[i]) {
			return nil, false
		}
	}
	merged := all[0]
	for i := 1; i < len(all); i++ {
		next, _ := mergeFunctionFactsByShape(merged, all[i], true).(*typ.Function)
		if next == nil {
			return nil, false
		}
		merged = next
	}
	return merged, true
}

func functionVariantsForFactMerge(t typ.Type) []*typ.Function {
	if t == nil {
		return nil
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		// Optional function values include nil. Do not collapse them to a plain
		// function fact or we lose optionality in merged facts.
		return nil
	case *typ.Function:
		return []*typ.Function{v}
	case *typ.Union:
		if len(v.Members) == 0 {
			return nil
		}
		var out []*typ.Function
		for _, m := range v.Members {
			fn := unwrap.Function(m)
			if fn == nil {
				// Only collapse union variants when the union is function-only.
				// Mixed unions (for example function|nil) must stay untouched.
				return nil
			}
			out = append(out, fn)
		}
		return out
	}
	if fn := unwrap.Function(t); fn != nil {
		return []*typ.Function{fn}
	}
	return nil
}

func sameFunctionShapeForFactMerge(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a.TypeParams) != len(b.TypeParams) {
		return false
	}
	if !typeParamsEqual(a.TypeParams, b.TypeParams) {
		return false
	}
	if len(a.Params) != len(b.Params) {
		return false
	}
	// Param type precision and optionality may differ across iterations.
	// Treat those as mergeable slots and reconcile in mergeFunctionFactsByShape.
	return true
}

func mergeFunctionFactsByShape(existing, candidate *typ.Function, alternatives bool) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	builder := typ.Func()
	for _, tp := range existing.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}

	for i, p := range existing.Params {
		paramType := mergeFunctionParamFactType(p.Type, candidate.Params[i].Type)
		name := p.Name
		if name == "" {
			name = candidate.Params[i].Name
		}
		optional := p.Optional || candidate.Params[i].Optional
		if optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}

	if existing.Variadic != nil || candidate.Variadic != nil {
		builder = builder.Variadic(mergeFunctionParamFactType(existing.Variadic, candidate.Variadic))
	}

	if mergedReturns := MergeReturnSummary(existing.Returns, candidate.Returns); len(mergedReturns) > 0 {
		builder = builder.Returns(mergedReturns...)
	}

	effects := existing.Effects
	if effects == nil {
		effects = candidate.Effects
	}
	if effects != nil {
		builder = builder.Effects(effects)
	}
	// Alternative callable signatures guarantee only their common effects.
	// Repeated estimates for one function are updates to the same summary.
	existingSpec := contract.ExtractSpec(existing)
	candidateSpec := contract.ExtractSpec(candidate)
	if existingSpec != nil || candidateSpec != nil {
		var merged contract.Spec
		if candidateSpec != nil {
			merged = *candidateSpec
		} else {
			merged = *existingSpec
		}
		if alternatives {
			if existingSpec == nil || candidateSpec == nil {
				merged.Effects = effect.Empty
			} else {
				merged.Effects = effect.Intersect(existingSpec.Effects, candidateSpec.Effects)
			}
		} else if existingSpec != nil && candidateSpec != nil {
			// Repeated estimates describe one body. A round that cannot yet
			// prove a return relation does not refute an earlier complete proof.
			merged.Effects = effect.Union(existingSpec.Effects, candidateSpec.Effects)
		}
		builder = builder.Spec(&merged)
	}
	refinement := existing.Refinement
	if refinement == nil {
		refinement = candidate.Refinement
	}
	if refinement != nil {
		builder = builder.WithRefinement(refinement)
	}

	return builder.Build()
}

func mergeFunctionParamFactType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existing = typ.PruneSoftUnionMembers(existing)
	candidate = typ.PruneSoftUnionMembers(candidate)
	if preferred, ok := preferStructuredRecordParam(existing, candidate); ok {
		return preferred
	}
	if typ.IsUnknown(existing) {
		return candidate
	}
	if typ.IsUnknown(candidate) {
		return existing
	}
	if typ.IsAny(existing) && !typ.IsAny(candidate) {
		return candidate
	}
	if typ.IsAny(candidate) && !typ.IsAny(existing) {
		return existing
	}
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if joined, ok := refineSnapshotTypes(existing, candidate); ok {
		return joined
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	return refineSnapshotMembers(typ.JoinPreferNonSoft(existing, candidate))
}

func preferStructuredRecordParam(existing, candidate typ.Type) (typ.Type, bool) {
	existingRec, okExisting := unwrap.Alias(existing).(*typ.Record)
	candidateRec, okCandidate := unwrap.Alias(candidate).(*typ.Record)
	if !okExisting || !okCandidate {
		return nil, false
	}

	existingOpenTop := existingRec.Open && len(existingRec.Fields) == 0 && !existingRec.HasMapComponent()
	candidateOpenTop := candidateRec.Open && len(candidateRec.Fields) == 0 && !candidateRec.HasMapComponent()
	if existingOpenTop == candidateOpenTop {
		return nil, false
	}
	if existingOpenTop {
		if candidateRec.HasMapComponent() || len(candidateRec.Fields) > 0 {
			return candidate, true
		}
	}
	if candidateOpenTop {
		if existingRec.HasMapComponent() || len(existingRec.Fields) > 0 {
			return existing, true
		}
	}
	return nil, false
}

// AlignFunctionTypeWithSummary applies the canonical return-summary winner to a
// function type. It updates function returns only when the summary is the
// preferred vector under SelectPreferredReturnVector (or when function returns
// are missing). Returns the aligned function and whether it changed.
func AlignFunctionTypeWithSummary(fn *typ.Function, summary []typ.Type) (*typ.Function, bool) {
	if fn == nil {
		return nil, false
	}

	normalizedSummary := normalizeAndPruneReturnVector(summary)
	if len(normalizedSummary) == 0 {
		return fn, false
	}

	current := normalizeAndPruneReturnVector(fn.Returns)
	if len(current) == 0 {
		aligned := typjoin.WithReturns(fn, normalizedSummary)
		return aligned, aligned != nil
	}
	// Keep one canonical merge path for summary-to-signature alignment.
	// MergeReturnSummary already handles structured promotion and refinement
	// policy, so AlignFunctionTypeWithSummary should not duplicate local logic.
	merged := MergeReturnSummary(current, normalizedSummary)
	if ReturnTypesEqual(current, merged) {
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
		if !isOpenTopRecordType(out[i]) {
			continue
		}
		if !isStructuredTableShape(summary[i]) {
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

// WithSummaryOrUnknown applies summary-derived returns to a function signature.
// If summary is empty and the signature has no returns, a single unknown return
// is attached to preserve call-site conservatism.
func WithSummaryOrUnknown(fn *typ.Function, summary []typ.Type) *typ.Function {
	if fn == nil {
		return nil
	}
	if len(summary) == 0 {
		if len(fn.Returns) > 0 {
			return fn
		}
		return typjoin.WithReturns(fn, []typ.Type{typ.Unknown})
	}
	if aligned, changed := AlignFunctionTypeWithSummary(fn, summary); changed {
		return aligned
	}
	if len(fn.Returns) > 0 {
		return fn
	}
	return typjoin.WithReturns(fn, normalizeAndPruneReturnVector(summary))
}

func isOpenTopRecordType(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return rec.Open && len(rec.Fields) == 0 && !rec.HasMapComponent()
}

func isStructuredTableShape(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Map:
		return true
	case *typ.Record:
		return v.HasMapComponent() || len(v.Fields) > 0
	default:
		return false
	}
}

// refineSnapshotSlots resolves the slots where both vectors hold snapshots of
// one recursion identity and one refines the other: both vectors take the
// refining snapshot in that slot.
func refineSnapshotSlots(existing, candidate []typ.Type) ([]typ.Type, []typ.Type) {
	var outE, outC []typ.Type
	for i := 0; i < len(existing) && i < len(candidate); i++ {
		refined, ok := refineSnapshotTypes(existing[i], candidate[i])
		if !ok {
			continue
		}
		if outE == nil {
			outE = append([]typ.Type(nil), existing...)
			outC = append([]typ.Type(nil), candidate...)
		}
		outE[i], outC[i] = refined, refined
	}
	if outE == nil {
		return existing, candidate
	}
	return outE, outC
}

// refineSnapshotTypes returns the refining one of a and b when both are
// snapshots of one recursion identity, directly or as the present value of an
// optional.
func refineSnapshotTypes(a, b typ.Type) (typ.Type, bool) {
	if a == nil || b == nil {
		return nil, false
	}
	ai, aOpt := splitOptionalSnapshot(a)
	bi, bOpt := splitOptionalSnapshot(b)
	if ai == nil || bi == nil || ai.ID != bi.ID || typ.TypeEquals(ai, bi) {
		return nil, false
	}
	refined, ok := refinedSnapshot(ai, bi)
	if !ok {
		return nil, false
	}
	if aOpt || bOpt {
		return typ.NewOptional(refined), true
	}
	return refined, true
}

// splitOptionalSnapshot returns the recursive snapshot t holds, directly or
// as the present value of an optional, and whether t is optional.
func splitOptionalSnapshot(t typ.Type) (*typ.Recursive, bool) {
	if opt, ok := t.(*typ.Optional); ok {
		r, _ := opt.Inner.(*typ.Recursive)
		return r, true
	}
	r, _ := t.(*typ.Recursive)
	return r, false
}

// refineSnapshotMembersVector refines, in every slot, the union members that
// are snapshots of one recursion identity.
func refineSnapshotMembersVector(ts []typ.Type) []typ.Type {
	var out []typ.Type
	for i, t := range ts {
		refined := refineSnapshotMembers(t)
		if refined != t && out == nil {
			out = append([]typ.Type(nil), ts...)
		}
		if out != nil {
			out[i] = refined
		}
	}
	if out == nil {
		return ts
	}
	return out
}

// refineSnapshotMembers keeps, among the members of union t that are
// snapshots of one recursion identity, only those no other member refines. A
// join of two estimates of a slot otherwise holds an earlier snapshot of a
// table beside the one that refines it.
func refineSnapshotMembers(t typ.Type) typ.Type {
	u, ok := t.(*typ.Union)
	if !ok {
		if opt, ok := t.(*typ.Optional); ok {
			if inner := refineSnapshotMembers(opt.Inner); inner != opt.Inner {
				return typ.NewOptional(inner)
			}
		}
		return t
	}
	byID := make(map[uint64]*typ.Recursive)
	refinedAny := false
	members := make([]typ.Type, 0, len(u.Members))
	for _, m := range u.Members {
		r, ok := m.(*typ.Recursive)
		if !ok || r.Body == nil {
			members = append(members, m)
			continue
		}
		if existing, seen := byID[r.ID]; seen {
			if refined, ok := refinedSnapshot(existing, r); ok {
				byID[r.ID] = refined
				refinedAny = true
				continue
			}
			members = append(members, r)
			continue
		}
		byID[r.ID] = r
	}
	if !refinedAny {
		return t
	}
	for _, r := range byID {
		members = append(members, r)
	}
	return typ.NewUnion(members...)
}

// refinedSnapshot returns the more informed of two snapshots of one recursion
// identity: the one that absorbs the other under the iteration join, which
// orders successive estimates of a table. Snapshots neither absorbs describe
// different states of the table and are not merged.
func refinedSnapshot(a, b *typ.Recursive) (*typ.Recursive, bool) {
	if a == nil || b == nil || a.ID != b.ID || a.Body == nil || b.Body == nil {
		return nil, false
	}
	variable := &typ.Recursive{ID: a.ID, Name: a.Name}
	open := func(body typ.Type) typ.Type {
		return typ.Rewrite(body, func(node typ.Type) (typ.Type, bool) {
			if r, ok := node.(*typ.Recursive); ok && r.ID == variable.ID {
				return variable, true
			}
			return nil, false
		})
	}
	bodyA, bodyB := open(a.Body), open(b.Body)
	joined := joinIterationFact(bodyA, bodyB)
	if typ.TypeEquals(joined, bodyB) {
		return b, true
	}
	if typ.TypeEquals(joined, bodyA) {
		return a, true
	}
	return nil, false
}
