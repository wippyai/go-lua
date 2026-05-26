package returnsummary

import (
	"github.com/wippyai/go-lua/types/domain/value"
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
		if typ.ContainsRecursive(a[i]) || typ.ContainsRecursive(b[i]) {
			if !value.SameConvergedFact(a[i], b[i]) {
				return false
			}
			continue
		}
		if !value.FactTypeEqual(a[i], b[i]) {
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
	if unknownOnly(a) && AllNil(b) {
		return b, true
	}
	if unknownOnly(b) && AllNil(a) {
		return a, true
	}
	if RepairsNever(a, b) {
		return a, true
	}
	if RepairsNever(b, a) {
		return b, true
	}
	if RefinesSoftContainers(a, b) {
		return mergeElementwise(b, a), true
	}
	if RefinesSoftContainers(b, a) {
		return mergeElementwise(a, b), true
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
	if upper, ok := SelfEmbeddingUpperBound(a, b); ok {
		return upper, true
	}
	if MutuallyRefines(a, b) {
		return selectEquivalentRefinementVector(a, b), true
	}
	if FreshEmptyTableSeedNeedsJoin(a, b) {
		return joinFreshEmptyTableSeedVector(a, b), true
	}
	if Refines(a, b) && !UnsafePrecisionDrop(b, a) {
		if AllNil(a) && !AllNil(b) {
			return b, true
		}
		if NestedNilOnlyRegression(a, b) {
			return b, true
		}
		return a, true
	}
	if Refines(b, a) && !UnsafePrecisionDrop(a, b) {
		if AllNil(b) && !AllNil(a) {
			return a, true
		}
		if NestedNilOnlyRegression(b, a) {
			return a, true
		}
		return b, true
	}
	if RefinesWithNilPadding(a, b) {
		return mergePaddedRefinement(a, b), true
	}
	if RefinesWithNilPadding(b, a) {
		return mergePaddedRefinement(b, a), true
	}
	if FillsNilSlots(a, b) {
		return a, true
	}
	if FillsNilSlots(b, a) {
		return b, true
	}
	if (RecordExtensionUpperBound(a, b) || ElidesOptional(a, b)) && !NestedNilOnlyRegression(a, b) {
		return a, true
	}
	if (RecordExtensionUpperBound(b, a) || ElidesOptional(b, a)) && !NestedNilOnlyRegression(b, a) {
		return b, true
	}
	return nil, false
}

// SelfEmbeddingUpperBound lifts the value-domain recursive-growth admission law
// to return vectors. It returns the element-wise finite upper bound only when
// every changed slot belongs to the same self-embedding family.
func SelfEmbeddingUpperBound(a, b []typ.Type) ([]typ.Type, bool) {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return nil, false
	}
	out := make([]typ.Type, len(a))
	changed := false
	for i := range a {
		left := a[i]
		right := b[i]
		if value.FactTypeEqual(left, right) {
			out[i] = left
			continue
		}
		upper, ok := value.SelfEmbeddingUpperBound(left, right)
		if !ok {
			return nil, false
		}
		out[i] = upper
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
}

// RecordExtensionUpperBound reports whether candidate is a monotone record
// extension upper bound for baseline in every return slot.
func RecordExtensionUpperBound(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	for i := range candidate {
		upper, ok := value.RecordExtensionUpperBound(candidate[i], baseline[i])
		if !ok || !value.FactTypeEqual(upper, candidate[i]) {
			return false
		}
	}
	return true
}

// FreshEmptyTableSeedNeedsJoin reports whether a fresh empty-table branch is
// competing with a structured sequence branch. In function-return products, the
// empty table is a branch outcome, not a refinement that should win before the
// return-slot join law can admit the structured container.
func FreshEmptyTableSeedNeedsJoin(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if freshEmptyTableSeedCompetesWithSequence(a[i], b[i]) ||
			freshEmptyTableSeedCompetesWithSequence(b[i], a[i]) {
			return true
		}
	}
	return false
}

func freshEmptyTableSeedCompetesWithSequence(seed, structured typ.Type) bool {
	return unwrap.IsEmptyRecord(seed) && sequenceLikeReturnSlot(structured)
}

func joinFreshEmptyTableSeedVector(a, b []typ.Type) []typ.Type {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		left := typ.Nil
		if i < len(a) && a[i] != nil {
			left = a[i]
		}
		right := typ.Nil
		if i < len(b) && b[i] != nil {
			right = b[i]
		}
		if freshEmptyTableSeedCompetesWithSequence(left, right) ||
			freshEmptyTableSeedCompetesWithSequence(right, left) {
			out[i] = typ.JoinReturnSlot(left, right)
			continue
		}
		out[i] = mergeReturnSlot(left, right)
	}
	return out
}

func sequenceLikeReturnSlot(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Optional:
		return sequenceLikeReturnSlot(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if member == nil || unwrap.IsNilType(member) {
				continue
			}
			if sequenceLikeReturnSlot(member) {
				return true
			}
		}
	}
	return false
}

// MutuallyRefines reports semantic equivalence under the return-summary
// refinement relation even when the surface representation differs, such as a
// named alias versus its expanded structural union.
func MutuallyRefines(a, b []typ.Type) bool {
	return Refines(a, b) && Refines(b, a)
}

func selectEquivalentRefinementVector(a, b []typ.Type) []typ.Type {
	if len(a) != len(b) {
		return a
	}
	out := make([]typ.Type, len(a))
	for i := range a {
		out[i] = SelectEquivalentReturnSlot(a[i], b[i])
	}
	return out
}

// SelectEquivalentReturnSlot chooses a deterministic representative for two
// semantically equivalent return slot types.
func SelectEquivalentReturnSlot(a, b typ.Type) typ.Type {
	if typ.SameNode(a, b) {
		return a
	}
	if sameReturnSlotWithoutRecursiveDescent(a, b) {
		return a
	}
	aAlias := hasAliasSurface(a)
	bAlias := hasAliasSurface(b)
	switch {
	case aAlias && !bAlias:
		return a
	case bAlias && !aAlias:
		return b
	case a == nil:
		return b
	case b == nil:
		return a
	case a.Hash() <= b.Hash():
		return a
	default:
		return b
	}
}

func hasAliasSurface(t typ.Type) bool {
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return true
	case *typ.Optional:
		return hasAliasSurface(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if member == nil || unwrap.IsNilType(member) {
				continue
			}
			if hasAliasSurface(member) {
				return true
			}
		}
	}
	return false
}

// RefinesWithNilPadding reports whether candidate refines baseline when
// missing Lua return slots are interpreted as explicit nil.
func RefinesWithNilPadding(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) == len(baseline) {
		return false
	}
	maxLen := len(candidate)
	if len(baseline) > maxLen {
		maxLen = len(baseline)
	}
	strict := false
	for i := 0; i < maxLen; i++ {
		c := typ.Nil
		if i < len(candidate) && candidate[i] != nil {
			c = candidate[i]
		}
		b := typ.Nil
		if i < len(baseline) && baseline[i] != nil {
			b = baseline[i]
		}
		if !subtype.IsSubtype(c, b) {
			return false
		}
		if !typ.TypeEquals(c, b) {
			strict = true
		}
	}
	return strict
}

func mergePaddedRefinement(candidate, baseline []typ.Type) []typ.Type {
	maxLen := len(candidate)
	if len(baseline) > maxLen {
		maxLen = len(baseline)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		if i < len(candidate) && i < len(baseline) {
			out[i] = mergeReturnSlot(baseline[i], candidate[i])
			continue
		}
		if i < len(candidate) {
			out[i] = typ.CoalesceCompatibleRecordAlternatives(candidate[i])
			continue
		}
		out[i] = baseline[i]
	}
	return out
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
	if !containsNeverReturnSlot(baseline) {
		return false
	}
	strict := false
	for i := range candidate {
		if candidate[i] == nil || baseline[i] == nil {
			return false
		}
		if sameNeverRepairType(candidate[i], baseline[i]) {
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
	if !containsNeverType(baseline) {
		return false
	}
	ok, strict := newNeverRepairState().repair(candidate, baseline)
	return ok && strict
}

func containsNeverReturnSlot(rets []typ.Type) bool {
	for _, ret := range rets {
		if containsNeverType(ret) {
			return true
		}
	}
	return false
}

func containsNeverType(t typ.Type) bool {
	return typ.ContainsNever(t)
}

type neverRepairKey struct {
	candidate typ.Type
	baseline  typ.Type
}

type neverRepairResult struct {
	ok     bool
	strict bool
}

type neverRepairState struct {
	done   map[neverRepairKey]neverRepairResult
	active map[neverRepairKey]bool
}

func newNeverRepairState() *neverRepairState {
	return &neverRepairState{
		done:   make(map[neverRepairKey]neverRepairResult),
		active: make(map[neverRepairKey]bool),
	}
}

func (state *neverRepairState) repair(candidate, baseline typ.Type) (bool, bool) {
	if candidate == nil || baseline == nil {
		return false, false
	}
	if sameNeverRepairType(candidate, baseline) {
		return true, false
	}

	candidate = unwrap.Alias(candidate)
	baseline = unwrap.Alias(baseline)
	if candidate == nil || baseline == nil {
		return false, false
	}
	key := neverRepairKey{candidate: candidate, baseline: baseline}
	if cached, ok := state.done[key]; ok {
		return cached.ok, cached.strict
	}
	if state.active[key] {
		return true, false
	}

	if typ.IsNever(baseline) {
		return !typ.IsNever(candidate), !typ.IsNever(candidate)
	}

	state.active[key] = true
	var result neverRepairResult
	defer func() {
		delete(state.active, key)
		state.done[key] = result
	}()

	switch b := baseline.(type) {
	case *typ.Optional:
		c, ok := candidate.(*typ.Optional)
		if !ok {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		result.ok, result.strict = state.repair(c.Inner, b.Inner)
		return result.ok, result.strict
	case *typ.Union:
		c, ok := candidate.(*typ.Union)
		if !ok || len(c.Members) != len(b.Members) {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		used := make([]bool, len(c.Members))
		strict := false
		for _, bm := range b.Members {
			matched := false
			for j, cm := range c.Members {
				if used[j] || !sameNeverRepairType(cm, bm) {
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
				ok, repaired := state.repair(cm, bm)
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
				result = neverRepairResult{}
				return result.ok, result.strict
			}
		}
		result = neverRepairResult{ok: true, strict: strict}
		return result.ok, result.strict
	case *typ.Record:
		c, ok := candidate.(*typ.Record)
		if !ok || c.Open != b.Open || c.HasMapComponent() != b.HasMapComponent() || len(c.Fields) != len(b.Fields) {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		strict := false
		for _, bf := range b.Fields {
			cf := c.GetField(bf.Name)
			if cf == nil || cf.Optional != bf.Optional || cf.Readonly != bf.Readonly {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			ok, repaired := state.repair(cf.Type, bf.Type)
			if !ok {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			if repaired {
				strict = true
			}
		}
		if b.HasMapComponent() {
			ok, repaired := state.repair(c.MapKey, b.MapKey)
			if !ok {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			if repaired {
				strict = true
			}
			ok, repaired = state.repair(c.MapValue, b.MapValue)
			if !ok {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			if repaired {
				strict = true
			}
		}
		if b.Metatable != nil || c.Metatable != nil {
			if b.Metatable == nil || c.Metatable == nil {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			ok, repaired := state.repair(c.Metatable, b.Metatable)
			if !ok {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			if repaired {
				strict = true
			}
		}
		result = neverRepairResult{ok: true, strict: strict}
		return result.ok, result.strict
	case *typ.Array:
		c, ok := candidate.(*typ.Array)
		if !ok {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		result.ok, result.strict = state.repair(c.Element, b.Element)
		return result.ok, result.strict
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		keyOK, keyStrict := state.repair(c.Key, b.Key)
		if !keyOK {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		valOK, valStrict := state.repair(c.Value, b.Value)
		if !valOK {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		result = neverRepairResult{ok: true, strict: keyStrict || valStrict}
		return result.ok, result.strict
	case *typ.Tuple:
		c, ok := candidate.(*typ.Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		strict := false
		for i := range b.Elements {
			ok, repaired := state.repair(c.Elements[i], b.Elements[i])
			if !ok {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			if repaired {
				strict = true
			}
		}
		result = neverRepairResult{ok: true, strict: strict}
		return result.ok, result.strict
	case *typ.Function:
		c, ok := candidate.(*typ.Function)
		if !ok || !sameFunctionShapeForRepair(c, b) || len(c.Returns) != len(b.Returns) {
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		for i := range b.Params {
			if c.Params[i].Name != b.Params[i].Name ||
				c.Params[i].Optional != b.Params[i].Optional ||
				!sameNeverRepairType(c.Params[i].Type, b.Params[i].Type) {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
		}
		switch {
		case (c.Variadic == nil) != (b.Variadic == nil):
			result = neverRepairResult{}
			return result.ok, result.strict
		case c.Variadic != nil && !sameNeverRepairType(c.Variadic, b.Variadic):
			result = neverRepairResult{}
			return result.ok, result.strict
		}
		strict := false
		for i := range b.Returns {
			ok, repaired := state.repair(c.Returns[i], b.Returns[i])
			if !ok {
				result = neverRepairResult{}
				return result.ok, result.strict
			}
			if repaired {
				strict = true
			}
		}
		result = neverRepairResult{ok: true, strict: strict}
		return result.ok, result.strict
	default:
		result = neverRepairResult{}
		return result.ok, result.strict
	}
}

func sameNeverRepairType(a, b typ.Type) bool {
	if typ.SameNode(a, b) {
		return true
	}
	if a == nil || b == nil {
		return a == b
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		return false
	}
	return typ.EqualityHash(a) == typ.EqualityHash(b) && typ.TypeEquals(a, b)
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

// DemoteInferredDynamicAny rewrites dynamic-any placeholders in inferred
// return products to unknown. Explicit declared returns keep their source
// contract in FunctionFact; this vector law is for unannotated public inference
// where an any-derived value is not proof of a concrete contract.
func DemoteInferredDynamicAny(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	var out []typ.Type
	for i, ret := range rets {
		demoted := demoteInferredDynamicAnyType(ret)
		if out != nil {
			out[i] = demoted
			continue
		}
		if !typ.TypeEquals(ret, demoted) {
			out = make([]typ.Type, len(rets))
			copy(out, rets[:i])
			out[i] = demoted
		}
	}
	if out != nil {
		return out
	}
	return rets
}

func demoteInferredDynamicAnyType(t typ.Type) typ.Type {
	return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		if typ.IsAny(node) {
			return typ.Unknown, true
		}
		return nil, false
	})
}

// Merge applies the canonical return-summary merge policy shared by iterative
// product facts.
func Merge(existing, candidate []typ.Type) []typ.Type {
	return MergeWithConvergence(value.NewConvergenceWidening(), existing, candidate)
}

// MergeWithConvergence applies the canonical return-summary merge policy using
// the caller's convergence context. Interprocedural product merges use this so
// nested return slots share one growth-risk/widening memo.
func MergeWithConvergence(widening *value.ConvergenceWidening, existing, candidate []typ.Type) []typ.Type {
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
	if WholeSlotExplicitAnyNeedsJoin(existing, candidate) {
		return NormalizeAndPrune(mergeElementwise(existing, candidate))
	}
	if NilSlotReturnAlternativeNeedsJoin(existing, candidate) {
		return NormalizeAndPrune(mergeElementwise(existing, candidate))
	}
	if preferred, ok := SelectPreferred(existing, candidate); ok {
		return NormalizeAndPrune(preferred)
	}
	if merged, ok := mergeEvidenceVectorWith(widening, existing, candidate); ok {
		return NormalizeAndPrune(merged)
	}
	if shouldUseMonotoneJoinWith(widening, existing, candidate) {
		return NormalizeAndPrune(joinMonotoneWith(widening, existing, candidate))
	}
	return NormalizeAndPrune(mergeElementwise(existing, candidate))
}

// NilSlotReturnAlternativeNeedsJoin reports whether two return vectors disagree
// because one branch produced nil in a slot where another branch produced
// non-nil evidence. This is branch union, not a subtype refinement: selecting
// the nil-bearing vector would erase the non-nil branch, and selecting the
// non-nil vector would erase the nil branch.
//
// A shared non-nil equal anchor slot proves the two vectors are the same
// function-return product gaining evidence, not two competing branches. In that
// case the nil slot is missing evidence to be filled, so this returns false and
// lets the fill law win without injecting nilability.
func NilSlotReturnAlternativeNeedsJoin(a, b []typ.Type) bool {
	if sharesNonNilAnchorSlot(a, b) {
		return false
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		var left, right typ.Type
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if nilSlotCompetesWithEvidence(left, right) || nilSlotCompetesWithEvidence(right, left) {
			return true
		}
	}
	return false
}

// sharesNonNilAnchorSlot reports whether the vectors have the same arity and at
// least one slot that is non-nil and equal in both, identifying them as the same
// return product rather than competing branches.
func sharesNonNilAnchorSlot(a, b []typ.Type) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil || unwrap.IsNilType(a[i]) || unwrap.IsNilType(b[i]) {
			continue
		}
		if value.FactTypeEqual(a[i], b[i]) {
			return true
		}
	}
	return false
}

func nilSlotCompetesWithEvidence(nilCandidate, evidence typ.Type) bool {
	if nilCandidate == nil || evidence == nil {
		return false
	}
	return unwrap.IsNilType(nilCandidate) && !unwrap.IsNilType(evidence)
}

// WholeSlotExplicitAnyNeedsJoin reports whether a return branch carries an
// explicit dynamic-any outcome. In that case a concrete sibling branch must not
// win via ordinary subtype preference, because the dynamic branch is itself a
// runtime outcome. Unknown is not included here: it is missing analysis evidence
// and concrete interpreter evidence may replace it.
func WholeSlotExplicitAnyNeedsJoin(a, b []typ.Type) bool {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		var left, right typ.Type
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if dynamicAnyOutcomeWithNonNilPeer(left, right) || dynamicAnyOutcomeWithNonNilPeer(right, left) {
			return true
		}
	}
	return false
}

func dynamicAnyOutcomeWithNonNilPeer(t, peer typ.Type) bool {
	return explicitAnyWithNonNilPeer(t, peer) || explicitDynamicAnyTableWithNonNilPeer(t, peer)
}

func explicitAnyWithNonNilPeer(t, peer typ.Type) bool {
	if t == nil || peer == nil || unwrap.IsNilType(peer) {
		return false
	}
	t = unwrap.Alias(t)
	return typ.IsAny(t)
}

func explicitDynamicAnyTableWithNonNilPeer(t, peer typ.Type) bool {
	if t == nil || peer == nil || unwrap.IsNilType(peer) {
		return false
	}
	return isDynamicAnyTable(unwrap.Alias(t))
}

func isDynamicAnyTable(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Map:
		return typ.IsAny(unwrap.Alias(v.Key)) && typ.IsAny(unwrap.Alias(v.Value))
	case *typ.Record:
		return len(v.Fields) == 0 &&
			v.HasMapComponent() &&
			typ.IsAny(unwrap.Alias(v.MapKey)) &&
			typ.IsAny(unwrap.Alias(v.MapValue))
	default:
		return false
	}
}

func explicitAnyJoin(a, b typ.Type) (typ.Type, bool) {
	if dynamicAnyOutcomeWithNonNilPeer(a, b) {
		return unwrap.Alias(a), true
	}
	if dynamicAnyOutcomeWithNonNilPeer(b, a) {
		return unwrap.Alias(b), true
	}
	return nil, false
}

func mergeElementwise(existing, candidate []typ.Type) []typ.Type {
	if len(existing) == 0 {
		return candidate
	}
	if len(candidate) == 0 {
		return existing
	}
	maxLen := len(existing)
	if len(candidate) > maxLen {
		maxLen = len(candidate)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var a, b typ.Type
		if i < len(existing) {
			a = existing[i]
		} else {
			a = typ.Nil
		}
		if i < len(candidate) {
			b = candidate[i]
		} else {
			b = typ.Nil
		}
		out[i] = mergeReturnSlot(a, b)
	}
	return out
}

func mergeReturnSlot(existing, candidate typ.Type) typ.Type {
	if top, ok := explicitAnyJoin(existing, candidate); ok {
		return top
	}
	if freshEmptyTableSeedCompetesWithSequence(existing, candidate) ||
		freshEmptyTableSeedCompetesWithSequence(candidate, existing) {
		return typ.JoinReturnSlot(existing, candidate)
	}
	if merged, ok := mergeEvidenceReturnSlot(existing, candidate); ok {
		return preserveReturnSlotNilability(merged, existing, candidate)
	}
	if refinesReturnSlot(candidate, existing) {
		return preserveReturnSlotNilability(typ.CoalesceCompatibleRecordAlternatives(candidate), existing, candidate)
	}
	if refines, changed := value.RefinesSoftContainer(candidate, existing); refines && changed {
		return preserveReturnSlotNilability(candidate, existing, candidate)
	}
	if refines, changed := value.RefinesSoftContainer(existing, candidate); refines && changed {
		return preserveReturnSlotNilability(existing, existing, candidate)
	}
	if refines, changed := value.RefinesFalsyMapKey(candidate, existing); refines && changed {
		return preserveReturnSlotNilability(candidate, existing, candidate)
	}
	if refines, changed := value.RefinesFalsyMapKey(existing, candidate); refines && changed {
		return preserveReturnSlotNilability(existing, existing, candidate)
	}
	return typ.JoinReturnSlot(existing, candidate)
}

func mergeEvidenceVectorWith(widening *value.ConvergenceWidening, existing, candidate []typ.Type) ([]typ.Type, bool) {
	if len(existing) == 0 || len(candidate) == 0 || len(existing) != len(candidate) {
		return nil, false
	}
	out := make([]typ.Type, len(existing))
	changed := false
	for i := range existing {
		a := existing[i]
		b := candidate[i]
		if a == nil {
			a = typ.Nil
		}
		if b == nil {
			b = typ.Nil
		}
		if value.FactTypeEqual(a, b) {
			out[i] = a
			continue
		}
		merged, ok := mergeEvidenceReturnSlotWith(widening, a, b)
		if !ok {
			return nil, false
		}
		out[i] = preserveReturnSlotNilability(merged, a, b)
		if !value.FactTypeEqual(out[i], a) || !value.FactTypeEqual(out[i], b) {
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	return out, true
}

func mergeEvidenceReturnSlot(existing, candidate typ.Type) (typ.Type, bool) {
	return mergeEvidenceReturnSlotWith(value.NewConvergenceWidening(), existing, candidate)
}

func mergeEvidenceReturnSlotWith(widening *value.ConvergenceWidening, existing, candidate typ.Type) (typ.Type, bool) {
	if existing == nil || candidate == nil || unwrap.IsNilType(existing) || unwrap.IsNilType(candidate) {
		return nil, false
	}
	if typ.IsUnknown(unwrap.Alias(existing)) || typ.IsUnknown(unwrap.Alias(candidate)) ||
		typ.IsAny(unwrap.Alias(existing)) || typ.IsAny(unwrap.Alias(candidate)) {
		return nil, false
	}
	if upper, ok := value.FunctionEvidenceUpperBound(existing, candidate); ok {
		return upper, true
	}
	if !value.SameEvidenceFamily(existing, candidate) {
		return nil, false
	}
	var merged typ.Type
	if widening != nil {
		merged = widening.Merge(existing, candidate)
	} else {
		merged = value.MergeForConvergence(existing, candidate)
	}
	if merged == nil {
		return nil, false
	}
	return merged, true
}

func refinesReturnSlot(candidate, existing typ.Type) bool {
	if candidate == nil || existing == nil || unwrap.IsNilType(candidate) || typ.TypeEquals(candidate, existing) ||
		value.NestedNilOnlyRegression(candidate, existing) {
		return false
	}
	return subtype.IsSubtype(candidate, existing) ||
		value.ExtendsRecord(candidate, existing) ||
		value.ElidesOptional(candidate, existing)
}

func preserveReturnSlotNilability(preferred, existing, candidate typ.Type) typ.Type {
	if preferred == nil || unwrap.IsOptionalLike(preferred) || unwrap.IsNilType(preferred) {
		return preferred
	}
	if returnSlotContributesNil(existing) || returnSlotContributesNil(candidate) {
		return typ.JoinReturnSlot(preferred, typ.Nil)
	}
	return preferred
}

// returnSlotContributesNil reports whether a merge source genuinely carries a
// nil branch outcome. A bare unknown/any placeholder is missing evidence, not a
// nilable outcome, so it must not inject nilability into a concrete refinement.
func returnSlotContributesNil(t typ.Type) bool {
	if unwrap.IsNilType(t) {
		return true
	}
	alias := unwrap.Alias(t)
	if typ.IsAbsentOrUnknown(alias) || typ.IsAny(alias) {
		return false
	}
	return unwrap.IsOptionalLike(t)
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
	if unknownOnly(prev) && !unknownOnly(next) {
		return WidenVectorForConvergence(next)
	}
	if unknownOnly(next) && !unknownOnly(prev) {
		return WidenVectorForConvergence(prev)
	}
	if refined, ok := SelectRefining(next, prev); ok && !UnsafePrecisionDrop(prev, refined) {
		return WidenVectorForConvergence(NormalizeAndPrune(refined))
	}

	merged := Merge(prev, next)
	if UnsafePrecisionDrop(prev, merged) {
		merged = prev
	}
	return WidenVectorForConvergence(NormalizeAndPrune(merged))
}

func unknownOnly(rets []typ.Type) bool {
	if len(rets) == 0 {
		return false
	}
	for _, ret := range rets {
		if ret == nil || !typ.IsUnknown(ret) {
			return false
		}
	}
	return true
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
	return shouldUseMonotoneJoinWith(value.NewConvergenceWidening(), a, b)
}

func shouldUseMonotoneJoinWith(widening *value.ConvergenceWidening, a, b []typ.Type) bool {
	for _, t := range a {
		if returnSlotNeedsMonotoneJoinWith(widening, t) {
			return true
		}
	}
	for _, t := range b {
		if returnSlotNeedsMonotoneJoinWith(widening, t) {
			return true
		}
	}
	return false
}

func returnSlotNeedsMonotoneJoin(t typ.Type) bool {
	return returnSlotNeedsMonotoneJoinWith(value.NewConvergenceWidening(), t)
}

func returnSlotNeedsMonotoneJoinWith(widening *value.ConvergenceWidening, t typ.Type) bool {
	return typ.ContainsRecursive(t) || widening.HasHigherOrderGrowthRisk(t)
}

func joinMonotone(a, b []typ.Type) []typ.Type {
	return joinMonotoneWith(value.NewConvergenceWidening(), a, b)
}

func joinMonotoneWith(widening *value.ConvergenceWidening, a, b []typ.Type) []typ.Type {
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
		out[i] = joinTypeMonotoneWith(widening, ai, bi)
	}
	return out
}

func joinTypeMonotone(a, b typ.Type) typ.Type {
	return joinTypeMonotoneWith(value.NewConvergenceWidening(), a, b)
}

func joinTypeMonotoneWith(widening *value.ConvergenceWidening, a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if sameReturnSlotWithoutRecursiveDescent(a, b) {
		return a
	}
	if returnSlotNeedsMonotoneJoinWith(widening, a) || returnSlotNeedsMonotoneJoinWith(widening, b) {
		return widening.Merge(a, b)
	}
	if subtype.IsSubtype(a, b) || value.ExtendsRecord(a, b) || value.ElidesOptional(a, b) {
		return b
	}
	if subtype.IsSubtype(b, a) || value.ExtendsRecord(b, a) || value.ElidesOptional(b, a) {
		return a
	}
	return typ.JoinPreferNonSoft(a, b)
}

func sameReturnSlotWithoutRecursiveDescent(a, b typ.Type) bool {
	if typ.SameNode(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		return false
	}
	return typ.EqualityHash(a) == typ.EqualityHash(b) && typ.TypeEquals(a, b)
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
	if Equal(current, normalizedSummary) || EquivalentEvidence(current, normalizedSummary) {
		return fn, false
	}
	if replaced, changed := replaceUnknownReturnPlaceholders(current, normalizedSummary); changed {
		current = replaced
		if Equal(current, normalizedSummary) {
			aligned := typjoin.WithReturns(fn, current)
			return aligned, aligned != nil
		}
	}
	if preserved, changed := preserveCurrentAgainstExplicitAnySummary(current, normalizedSummary); changed {
		normalizedSummary = preserved
		if Equal(current, normalizedSummary) {
			return fn, false
		}
	}
	merged := Merge(current, normalizedSummary)
	if Equal(current, merged) || EquivalentEvidence(current, merged) {
		return fn, false
	}

	aligned := typjoin.WithReturns(fn, merged)
	if aligned == nil {
		return fn, false
	}
	return aligned, true
}

// EquivalentEvidence reports whether two return vectors describe the same
// value-domain evidence set even if their recursive product nodes differ.
func EquivalentEvidence(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		left := a[i]
		right := b[i]
		if value.FactTypeEqual(left, right) {
			continue
		}
		if !value.Covers(left, right) || !value.Covers(right, left) {
			return false
		}
	}
	return true
}

func replaceUnknownReturnPlaceholders(current, summary []typ.Type) ([]typ.Type, bool) {
	if len(current) == 0 || len(summary) == 0 {
		return nil, false
	}
	maxLen := len(current)
	if len(summary) < maxLen {
		maxLen = len(summary)
	}
	var out []typ.Type
	for i := 0; i < maxLen; i++ {
		if !typ.IsUnknown(current[i]) || summary[i] == nil || typ.IsUnknown(summary[i]) {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(current))
			copy(out, current)
		}
		out[i] = summary[i]
	}
	if out == nil {
		return nil, false
	}
	return out, true
}

func preserveCurrentAgainstExplicitAnySummary(current, summary []typ.Type) ([]typ.Type, bool) {
	if len(current) == 0 || len(summary) == 0 || len(current) != len(summary) {
		return nil, false
	}
	var out []typ.Type
	for i := range summary {
		if !explicitAnyWithNonNilPeer(summary[i], current[i]) {
			continue
		}
		if typ.IsAny(unwrap.Alias(current[i])) || typ.IsUnknown(unwrap.Alias(current[i])) {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(summary))
			copy(out, summary)
		}
		out[i] = current[i]
	}
	if out == nil {
		return nil, false
	}
	return out, true
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
