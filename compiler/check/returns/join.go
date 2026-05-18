package returns

import (
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

// ReturnTypesRefine reports whether a refines b (element-wise subtype).
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
		if !subtype.IsSubtype(ai, bi) {
			return false
		}
	}
	return true
}

// ReturnTypesExtendRecord reports whether a extends b by adding record fields.
// This treats record field supersets as refinements for return vectors.
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
	if ReturnTypesStopRecursiveStructuralGrowth(a, b) {
		return a, true
	}
	if ReturnTypesStopRecursiveStructuralGrowth(b, a) {
		return b, true
	}
	if ReturnTypesRefineFalsyMapKeys(a, b) {
		return a, true
	}
	if ReturnTypesRefineFalsyMapKeys(b, a) {
		return b, true
	}
	if ReturnTypesRefine(a, b) {
		if ReturnTypesAllNil(a) && !ReturnTypesAllNil(b) {
			return b, true
		}
		if ReturnTypesNestedNilOnlyRegression(a, b) {
			return b, true
		}
		return a, true
	}
	if ReturnTypesRefine(b, a) {
		if ReturnTypesAllNil(b) && !ReturnTypesAllNil(a) {
			return a, true
		}
		if ReturnTypesNestedNilOnlyRegression(b, a) {
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
	if (ReturnTypesExtendRecord(a, b) || ReturnTypesElideOptional(a, b)) && !ReturnTypesNestedNilOnlyRegression(a, b) {
		return a, true
	}
	if (ReturnTypesExtendRecord(b, a) || ReturnTypesElideOptional(b, a)) && !ReturnTypesNestedNilOnlyRegression(b, a) {
		return b, true
	}
	return nil, false
}

// ReturnTypesRefineFalsyMapKeys reports whether candidate is the same map-like
// shape as baseline after removing stale falsy key members from baseline. This
// handles fixed-point rounds where an early branch-insensitive dynamic index
// observes a key as `string | false`, then the solved guard proves the actual
// write key is `string`.
func ReturnTypesRefineFalsyMapKeys(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	strict := false
	for i := range candidate {
		refines, changed := typeRefinesFalsyMapKey(candidate[i], baseline[i])
		if !refines {
			return false
		}
		if changed {
			strict = true
		}
	}
	return strict
}

func typeRefinesFalsyMapKey(candidate, baseline typ.Type) (bool, bool) {
	candidate = unwrapStructuralShape(candidate)
	baseline = unwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil {
		return candidate == baseline, false
	}
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}

	switch b := baseline.(type) {
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			return false, false
		}
		return mapKeyTruthyRefinement(c.Key, c.Value, b.Key, b.Value)
	case *typ.Record:
		if c, ok := candidate.(*typ.Map); ok {
			if len(b.Fields) != 0 || b.Metatable != nil || !b.HasMapComponent() {
				return false, false
			}
			return mapKeyTruthyRefinement(c.Key, c.Value, b.MapKey, b.MapValue)
		}
		c, ok := candidate.(*typ.Record)
		if !ok || !c.HasMapComponent() || !b.HasMapComponent() {
			return false, false
		}
		if c.Open && !b.Open {
			return false, false
		}
		if len(c.Fields) != len(b.Fields) {
			return false, false
		}
		for _, bf := range b.Fields {
			cf := c.GetField(bf.Name)
			if cf == nil || cf.Optional != bf.Optional || cf.Readonly != bf.Readonly || !typ.TypeEquals(cf.Type, bf.Type) {
				return false, false
			}
		}
		if (c.Metatable == nil) != (b.Metatable == nil) || (c.Metatable != nil && !typ.TypeEquals(c.Metatable, b.Metatable)) {
			return false, false
		}
		return mapKeyTruthyRefinement(c.MapKey, c.MapValue, b.MapKey, b.MapValue)
	default:
		return false, false
	}
}

func mapKeyTruthyRefinement(candidateKey, candidateValue, baselineKey, baselineValue typ.Type) (bool, bool) {
	if !typ.TypeEquals(candidateValue, baselineValue) {
		return false, false
	}
	refinedKey := narrow.ToTruthy(baselineKey)
	if refinedKey == nil || refinedKey.Kind().IsNever() || typ.TypeEquals(refinedKey, baselineKey) {
		return false, false
	}
	if typ.TypeEquals(candidateKey, refinedKey) || subtype.IsSubtype(candidateKey, refinedKey) {
		return true, true
	}
	return false, false
}

// ReturnTypesNestedNilOnlyRegression reports whether candidate's apparent
// refinement only adds nested nil facts over a more useful baseline shape. A
// required `nil` field or `unknown -> nil` field does not help callers, but it
// can make iterative structural facts oscillate with later non-nil evidence.
func ReturnTypesNestedNilOnlyRegression(candidate, baseline []typ.Type) bool {
	if len(candidate) == 0 || len(baseline) == 0 || len(candidate) != len(baseline) {
		return false
	}
	for i := range candidate {
		if typeNestedNilOnlyRegression(candidate[i], baseline[i]) {
			return true
		}
	}
	return false
}

func typeNestedNilOnlyRegression(candidate, baseline typ.Type) bool {
	candidate = unwrapStructuralShape(candidate)
	baseline = unwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	if unwrap.IsNilType(candidate) {
		return typ.IsAny(baseline) || typ.IsUnknown(baseline) || unwrap.IsOptionalLike(baseline)
	}

	switch c := candidate.(type) {
	case *typ.Record:
		b, ok := baseline.(*typ.Record)
		if !ok {
			return false
		}
		for _, cf := range c.Fields {
			bf := b.GetField(cf.Name)
			if bf == nil {
				continue
			}
			if unwrap.IsNilType(cf.Type) && (bf.Optional || typ.IsAny(bf.Type) || typ.IsUnknown(bf.Type) || unwrap.IsOptionalLike(bf.Type)) {
				return true
			}
			if typeNestedNilOnlyRegression(cf.Type, bf.Type) {
				return true
			}
		}
		if c.HasMapComponent() && b.HasMapComponent() {
			return typeNestedNilOnlyRegression(c.MapValue, b.MapValue)
		}
	case *typ.Array:
		if b, ok := baseline.(*typ.Array); ok {
			return typeNestedNilOnlyRegression(c.Element, b.Element)
		}
	case *typ.Map:
		if b, ok := baseline.(*typ.Map); ok {
			return typeNestedNilOnlyRegression(c.Value, b.Value)
		}
	case *typ.Tuple:
		b, ok := baseline.(*typ.Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			return false
		}
		for i := range c.Elements {
			if typeNestedNilOnlyRegression(c.Elements[i], b.Elements[i]) {
				return true
			}
		}
	case *typ.Function:
		b, ok := baseline.(*typ.Function)
		if !ok || len(c.Returns) != len(b.Returns) {
			return false
		}
		for i := range c.Returns {
			if typeNestedNilOnlyRegression(c.Returns[i], b.Returns[i]) {
				return true
			}
		}
	}
	return false
}

// ReturnTypesStopRecursiveStructuralGrowth reports whether growing embeds the
// same structural container shape as stable beneath its root. Recursive table
// builders such as deep-copy helpers otherwise look like ever-more-specific
// refinements: {[string]: any} -> {[string]: {[string]: nil}} -> ... . The
// existing top-level shape is already a sound upper bound, so keep it once the
// candidate starts feeding that shape back through one of its children.
func ReturnTypesStopRecursiveStructuralGrowth(stable, growing []typ.Type) bool {
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
		if typ.IsAbsentOrUnknown(s) || !typeCanSelfEmbed(s) {
			return false
		}
		if !shallowStructuralShapeEquals(g, s) {
			return false
		}
		if !typeContainsNestedStructuralShape(g, s) {
			return false
		}
		strict = true
	}
	return strict
}

func typeContainsNestedStructuralShape(haystack, needle typ.Type) bool {
	return typeContainsNestedStructuralShapeDepth(haystack, needle, make(map[typ.Type]bool), false)
}

func typeContainsNestedStructuralShapeDepth(haystack, needle typ.Type, seen map[typ.Type]bool, belowContainer bool) bool {
	if haystack == nil || needle == nil {
		return false
	}
	if seen[haystack] {
		return false
	}
	seen[haystack] = true

	node := unwrapStructuralShape(haystack)
	if node == nil {
		return false
	}
	if belowContainer && shallowStructuralShapeEquals(node, needle) {
		return true
	}

	descend := func(child typ.Type, childBelowContainer bool) bool {
		return typeContainsNestedStructuralShapeDepth(child, needle, seen, childBelowContainer)
	}

	switch n := node.(type) {
	case *typ.Optional:
		return descend(n.Inner, belowContainer)
	case *typ.Union:
		for _, member := range n.Members {
			if descend(member, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range n.Members {
			if descend(member, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Array:
		return descend(n.Element, true)
	case *typ.Map:
		return descend(n.Key, true) || descend(n.Value, true)
	case *typ.Tuple:
		for _, elem := range n.Elements {
			if descend(elem, true) {
				return true
			}
		}
		return false
	case *typ.Record:
		for _, field := range n.Fields {
			if descend(field.Type, true) {
				return true
			}
		}
		if n.Metatable != nil && descend(n.Metatable, true) {
			return true
		}
		if n.HasMapComponent() {
			return descend(n.MapKey, true) || descend(n.MapValue, true)
		}
		return false
	case *typ.Function:
		for _, param := range n.Params {
			if descend(param.Type, true) {
				return true
			}
		}
		if n.Variadic != nil && descend(n.Variadic, true) {
			return true
		}
		for _, ret := range n.Returns {
			if descend(ret, true) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		for _, arg := range n.TypeArgs {
			if descend(arg, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, method := range n.Methods {
			if method.Type != nil && descend(method.Type, true) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func shallowStructuralShapeEquals(a, b typ.Type) bool {
	a = unwrapStructuralShape(a)
	b = unwrapStructuralShape(b)
	if a == nil || b == nil {
		return a == b
	}

	switch av := a.(type) {
	case *typ.Union:
		for _, member := range av.Members {
			if shallowStructuralShapeEquals(member, b) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range av.Members {
			if shallowStructuralShapeEquals(member, b) {
				return true
			}
		}
		return false
	}
	switch bv := b.(type) {
	case *typ.Union:
		for _, member := range bv.Members {
			if shallowStructuralShapeEquals(a, member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range bv.Members {
			if shallowStructuralShapeEquals(a, member) {
				return true
			}
		}
		return false
	}

	switch av := a.(type) {
	case *typ.Array:
		_, ok := b.(*typ.Array)
		return ok
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		return ok && shallowMapKeyShapeEquals(av.Key, bv.Key)
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		return ok && len(av.Elements) == len(bv.Elements)
	case *typ.Record:
		bv, ok := b.(*typ.Record)
		return ok && shallowRecordShapeEquals(av, bv)
	default:
		return typ.TypeEquals(a, b)
	}
}

func unwrapStructuralShape(t typ.Type) typ.Type {
	for t != nil {
		switch v := t.(type) {
		case *typ.Annotated:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		case *typ.Alias:
			if v.Target == nil || v.Target == t {
				return t
			}
			t = v.Target
		case *typ.Optional:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		default:
			return t
		}
	}
	return nil
}

func shallowMapKeyShapeEquals(a, b typ.Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	if typ.TypeEquals(a, b) {
		return true
	}
	return typ.IsAny(a) || typ.IsAny(b) || typ.IsUnknown(a) || typ.IsUnknown(b)
}

func shallowRecordShapeEquals(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.HasMapComponent() != b.HasMapComponent() {
		return false
	}
	if a.HasMapComponent() && !shallowMapKeyShapeEquals(a.MapKey, b.MapKey) {
		return false
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for _, field := range a.Fields {
		if b.GetField(field.Name) == nil {
			return false
		}
	}
	return true
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
			if cf == nil || cf.Optional != bf.Optional || cf.Readonly != bf.Readonly {
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
	copy(out, rets)
	return NormalizeReturnVectorInPlace(out)
}

// NormalizeReturnVectorInPlace replaces nil slots with explicit nil types in an
// owned return vector.
func NormalizeReturnVectorInPlace(rets []typ.Type) []typ.Type {
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

func canonicalReturnVector(rets []typ.Type) []typ.Type {
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

func normalizeAndPruneReturnVector(rets []typ.Type) []typ.Type {
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

	// Higher-order summaries are merged monotonically for fixpoint stability.
	if shouldUseMonotoneReturnJoin(existing, candidate) {
		return normalizeAndPruneReturnVector(joinReturnVectorsMonotone(existing, candidate))
	}

	if preferred, ok := SelectPreferredReturnVector(existing, candidate); ok {
		return normalizeAndPruneReturnVector(preferred)
	}

	return normalizeAndPruneReturnVector(typjoin.ReturnVectors(existing, candidate))
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
			return mergeFunctionFactsByShape(existingFn, candidateFn)
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

type functionFactVariants struct {
	funcs     []*typ.Function
	residuals []typ.Type
}

func mergeFunctionFactVariants(existing, candidate typ.Type) (typ.Type, bool) {
	existingVariants := splitFunctionFactVariants(existing)
	candidateVariants := splitFunctionFactVariants(candidate)
	if len(existingVariants.funcs) == 0 || len(candidateVariants.funcs) == 0 {
		return nil, false
	}

	all := make([]*typ.Function, 0, len(existingVariants.funcs)+len(candidateVariants.funcs))
	all = append(all, existingVariants.funcs...)
	all = append(all, candidateVariants.funcs...)
	for i := 1; i < len(all); i++ {
		if !sameFunctionShapeForFactMerge(all[0], all[i]) {
			return nil, false
		}
	}

	merged := all[0]
	for i := 1; i < len(all); i++ {
		next, _ := mergeFunctionFactsByShape(merged, all[i]).(*typ.Function)
		if next == nil {
			return nil, false
		}
		merged = next
	}

	residuals := make([]typ.Type, 0, len(existingVariants.residuals)+len(candidateVariants.residuals)+1)
	residuals = append(residuals, existingVariants.residuals...)
	residuals = append(residuals, candidateVariants.residuals...)
	if len(residuals) == 0 {
		return merged, true
	}
	residuals = append(residuals, merged)
	return typ.NewUnion(residuals...), true
}

func splitFunctionFactVariants(t typ.Type) functionFactVariants {
	var out functionFactVariants
	collectFunctionFactVariants(t, &out)
	return out
}

func collectFunctionFactVariants(t typ.Type, out *functionFactVariants) {
	if t == nil || out == nil {
		return
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Union:
		for _, member := range v.Members {
			collectFunctionFactVariants(member, out)
		}
		return
	}
	if fn := unwrap.Function(t); fn != nil {
		out.funcs = append(out.funcs, fn)
		return
	}
	out.residuals = append(out.residuals, t)
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

func mergeFunctionFactsByShape(existing, candidate *typ.Function) typ.Type {
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
	spec := existing.Spec
	if spec == nil {
		spec = candidate.Spec
	}
	if spec != nil {
		builder = builder.Spec(spec)
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
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
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
