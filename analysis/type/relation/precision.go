package relation

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

type typePair struct {
	a uintptr
	b uintptr
}

// MorePrecise reports whether candidate carries strictly more type information
// than baseline under the gradual-typing precision relation.
//
// Precision is not subtyping: a type can be more precise because it replaces
// any/unknown with concrete structure in the same product shape. This is used
// when two analyses describe the same runtime expression and the checker must
// keep the proof with the most evidence.
func MorePrecise(candidate, baseline Type) bool {
	strict, comparable := ComparePrecision(candidate, baseline)
	return comparable && strict
}

// ComparePrecision compares two same-expression type descriptions.
//
// The first return value is true when candidate is strictly more precise than
// baseline. The second return value is true when the two shapes are comparable
// in the precision relation. Equal types are comparable but not strict.
func ComparePrecision(candidate, baseline Type) (bool, bool) {
	return comparePrecision(candidate, baseline, 0, &precisionSeen{})
}

// productFamilyHash returns a stable structural family hash for product-domain
// relations that must compare recursive products coinductively without
// unfolding them by concrete node identity.
func productFamilyHash(t Type) uint64 {
	return identity.ProductFamilyHash(t)
}

// sameProductFamily reports whether two recursive product observations describe
// the same fixed-point family with equal precision. It is the canonical
// recursive-product equality relation for union/member dedupe and convergence
// checks; generic TypeEquals remains exact structural equality.
func sameProductFamily(a, b Type) bool {
	return identity.SameProductFamilyWithPrecision(a, b, ComparePrecision)
}

func comparePrecision(candidate, baseline Type, depth int, seen *precisionSeen) (strict bool, comparable bool) {
	if candidate == baseline {
		return false, true
	}
	if baseline == nil {
		return false, false
	}
	if IsAbsentOrUnknown(baseline) || baseline.Kind().IsPlaceholder() {
		return candidate != nil && !IsAbsentOrUnknown(candidate) && !candidate.Kind().IsPlaceholder(), true
	}
	if candidate == nil || IsAbsentOrUnknown(candidate) || candidate.Kind().IsPlaceholder() {
		return false, false
	}
	if candidate.Kind() == kind.Integer && baseline.Kind() == kind.Number {
		return true, true
	}

	if equivalentLocalRefAlias(candidate, baseline) {
		return false, true
	}

	cacheKey, cacheOK := precisionFamilyPairKey(candidate, baseline, seen)
	if cacheOK && seen != nil && seen.results != nil {
		if cached, ok := seen.results[cacheKey]; ok {
			return cached.strict, cached.comparable
		}
	}

	var repeated bool
	var leave func()
	seen, repeated, leave = enterPrecisionPair(candidate, baseline, seen, cacheKey, cacheOK)
	if repeated {
		return false, true
	}
	if leave != nil {
		defer leave()
	}
	if cacheOK && seen != nil {
		defer func() {
			if seen.results == nil {
				seen.results = make(map[precisionFamilyPair]precisionResult)
			}
			seen.results[cacheKey] = precisionResult{strict: strict, comparable: comparable}
		}()
	}

	if c, ok := candidate.(*Alias); ok {
		return comparePrecision(c.UnaliasedTarget(), baseline, depth+1, seen)
	}
	if b, ok := baseline.(*Alias); ok {
		return comparePrecision(candidate, b.UnaliasedTarget(), depth+1, seen)
	}

	if normalized, ok := normalizePrecisionUnion(candidate, seen); ok {
		return comparePrecision(normalized, baseline, depth+1, seen)
	}
	if normalized, ok := normalizePrecisionUnion(baseline, seen); ok {
		return comparePrecision(candidate, normalized, depth+1, seen)
	}

	if precisionCanUseAcyclicEquality(candidate, baseline) && TypeEquals(candidate, baseline) {
		return false, true
	}

	if c, ok := candidate.(*Union); ok {
		if b, ok := baseline.(*Union); ok {
			return compareUnionToUnionPrecision(c, b, depth+1, seen)
		}
	}

	switch b := baseline.(type) {
	case *Union:
		return comparePrecisionAgainstUnion(candidate, b, depth+1, seen)
	case *Recursive:
		if b.Body == nil {
			return false, false
		}
		return comparePrecision(candidate, b.Body, depth+1, seen)
	}

	switch c := candidate.(type) {
	case *Union:
		return compareUnionPrecision(c, baseline, depth+1, seen)
	case *Literal:
		return compareLiteralPrecision(c, baseline)
	case *Record:
		b, ok := baseline.(*Record)
		if !ok {
			return false, false
		}
		return compareRecordPrecision(c, b, depth+1, seen)
	case *Optional:
		b, ok := baseline.(*Optional)
		if !ok {
			return false, false
		}
		return comparePrecision(c.Inner, b.Inner, depth+1, seen)
	case *Tuple:
		b, ok := baseline.(*Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			return false, false
		}
		return comparePrecisionSlices(c.Elements, b.Elements, depth+1, seen)
	case *Array:
		b, ok := baseline.(*Array)
		if !ok {
			return false, false
		}
		return comparePrecision(c.Element, b.Element, depth+1, seen)
	case *Map:
		switch b := baseline.(type) {
		case *Map:
			keyStrict, ok := comparePrecision(c.Key, b.Key, depth+1, seen)
			if !ok {
				return false, false
			}
			valueStrict, ok := comparePrecision(c.Value, b.Value, depth+1, seen)
			if !ok {
				return false, false
			}
			return keyStrict || valueStrict, true
		case *Record:
			return compareMapRecordPrecision(c, b, depth+1, seen)
		default:
			return false, false
		}
	case *ReadonlyMap:
		b, ok := baseline.(*ReadonlyMap)
		if !ok {
			return false, false
		}
		keyStrict, ok := comparePrecision(c.Key, b.Key, depth+1, seen)
		if !ok {
			return false, false
		}
		valueStrict, ok := comparePrecision(c.Value, b.Value, depth+1, seen)
		if !ok {
			return false, false
		}
		return keyStrict || valueStrict, true
	case *Instantiated:
		b, ok := baseline.(*Instantiated)
		if !ok || c.Generic == nil || b.Generic == nil || !TypeEquals(c.Generic, b.Generic) || len(c.TypeArgs) != len(b.TypeArgs) {
			return false, false
		}
		return compareGenericTypeArgsPrecision(c.TypeArgs, b.TypeArgs, depth+1, seen)
	case *Function:
		b, ok := baseline.(*Function)
		if !ok || len(c.Params) != len(b.Params) || len(c.Returns) != len(b.Returns) || (c.Variadic == nil) != (b.Variadic == nil) {
			return false, false
		}
		strict := false
		for i := range c.Params {
			if c.Params[i].Optional != b.Params[i].Optional {
				return false, false
			}
			paramStrict, ok := comparePrecision(c.Params[i].Type, b.Params[i].Type, depth+1, seen)
			if !ok {
				return false, false
			}
			strict = strict || paramStrict
		}
		returnStrict, ok := comparePrecisionSlices(c.Returns, b.Returns, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || returnStrict
		if c.Variadic != nil {
			variadicStrict, ok := comparePrecision(c.Variadic, b.Variadic, depth+1, seen)
			if !ok {
				return false, false
			}
			strict = strict || variadicStrict
		}
		return strict, true
	case *Recursive:
		if c.Body == nil {
			return false, false
		}
		if b, ok := baseline.(*Recursive); ok {
			if c.Name != b.Name || b.Body == nil {
				return false, false
			}
			return comparePrecision(c.Body, b.Body, depth+1, seen)
		}
		return comparePrecision(c.Body, baseline, depth+1, seen)
	default:
		return false, false
	}
}

func precisionCanUseAcyclicEquality(candidate, baseline Type) bool {
	return !ContainsRecursive(candidate) && !ContainsRecursive(baseline)
}

func normalizePrecisionUnion(t Type, seen *precisionSeen) (normalized Type, changed bool) {
	u, ok := t.(*Union)
	if !ok || u == nil || len(u.Members) < 2 {
		return nil, false
	}
	if ContainsRecursive(u) {
		return nil, false
	}
	if seen != nil {
		if seen.normalizedUnions != nil {
			if cached, ok := seen.normalizedUnions[t]; ok {
				return cached.typ, cached.changed
			}
		}
		if seen.normalizingUnions != nil && seen.normalizingUnions[t] {
			return nil, false
		}
		if seen.normalizingUnions == nil {
			seen.normalizingUnions = make(map[Type]bool)
		}
		seen.normalizingUnions[t] = true
		defer func() {
			delete(seen.normalizingUnions, t)
			if seen.normalizedUnions == nil {
				seen.normalizedUnions = make(map[Type]precisionUnionNormalization)
			}
			seen.normalizedUnions[t] = precisionUnionNormalization{typ: normalized, changed: changed}
		}()
	}
	state := newReturnJoinState()
	state.recursiveFamilyFold = true
	slotJoin := state.slotJoinOrDefault(nil)
	members := state.coalesceProductUnionMembersWithSlotJoin(u.Members, slotJoin)
	if !sameTypeSlice(u.Members, members) {
		return NormalizeUnionForJoin(members...), true
	}
	candidate := state.coalesceCompatibleRecordAlternativesWithSlotJoin(u, slotJoin)
	if candidate == nil || candidate == t {
		return nil, false
	}
	return candidate, true
}

func coalescePrecisionUnionMembers(types []Type) []Type {
	state := newReturnJoinState()
	state.recursiveFamilyFold = true
	return state.coalesceProductUnionMembersWithSlotJoin(types, state.slotJoinOrDefault(nil))
}

type precisionSeen struct {
	nodes             map[typePair]int
	families          map[precisionFamilyPair]int
	results           map[precisionFamilyPair]precisionResult
	familyHashes      map[Type]uint64
	normalizedUnions  map[Type]precisionUnionNormalization
	normalizingUnions map[Type]bool
}

type precisionFamilyPair struct {
	candidate uint64
	baseline  uint64
}

type precisionResult struct {
	strict     bool
	comparable bool
}

type precisionUnionNormalization struct {
	typ     Type
	changed bool
}

func enterPrecisionPair(candidate, baseline Type, seen *precisionSeen, familyPair precisionFamilyPair, hasFamilyPair bool) (*precisionSeen, bool, func()) {
	if seen == nil {
		seen = &precisionSeen{}
	}
	releaseNodes := false
	releaseFamilies := false

	cp := TypePointer(candidate)
	bp := TypePointer(baseline)
	if cp != 0 || bp != 0 {
		if seen.nodes == nil {
			seen.nodes = make(map[typePair]int)
		}
		pair := typePair{a: cp, b: bp}
		if seen.nodes[pair] > 0 {
			return seen, true, nil
		}
		seen.nodes[pair]++
		releaseNodes = true
	}

	if hasFamilyPair {
		if seen.families == nil {
			seen.families = make(map[precisionFamilyPair]int)
		}
		if seen.families[familyPair] > 0 {
			if releaseNodes {
				seen.nodes[typePair{a: cp, b: bp}]--
			}
			return seen, true, nil
		}
		seen.families[familyPair]++
		releaseFamilies = true
	}

	if !releaseNodes && !releaseFamilies {
		return seen, false, nil
	}
	return seen, false, func() {
		if releaseNodes {
			pair := typePair{a: cp, b: bp}
			if seen.nodes[pair] <= 1 {
				delete(seen.nodes, pair)
			} else {
				seen.nodes[pair]--
			}
		}
		if releaseFamilies {
			if seen.families[familyPair] <= 1 {
				delete(seen.families, familyPair)
			} else {
				seen.families[familyPair]--
			}
		}
	}
}

func precisionFamilyPairKey(candidate, baseline Type, seen *precisionSeen) (precisionFamilyPair, bool) {
	if !ContainsRecursive(candidate) && !ContainsRecursive(baseline) {
		return precisionFamilyPair{}, false
	}
	candidateHash := productFamilyHashWithSeen(candidate, seen)
	baselineHash := productFamilyHashWithSeen(baseline, seen)
	return precisionFamilyPair{candidate: candidateHash, baseline: baselineHash}, true
}

func productFamilyHashWithSeen(t Type, seen *precisionSeen) uint64 {
	return identity.ProductFamilyHashWithCache(t, productFamilyHashCache(seen))
}

func productFamilyHashCache(seen *precisionSeen) map[Type]uint64 {
	if seen != nil {
		if seen.familyHashes == nil {
			seen.familyHashes = make(map[Type]uint64)
		}
		return seen.familyHashes
	}
	return nil
}

func comparePrecisionSlices(candidate, baseline []Type, depth int, seen *precisionSeen) (bool, bool) {
	if len(candidate) != len(baseline) {
		return false, false
	}
	strict := false
	for i := range candidate {
		memberStrict, ok := comparePrecision(candidate[i], baseline[i], depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func compareGenericTypeArgsPrecision(candidate, baseline []Type, depth int, seen *precisionSeen) (bool, bool) {
	if len(candidate) != len(baseline) {
		return false, false
	}
	strict := false
	for i := range candidate {
		memberStrict, ok := compareGenericTypeArgPrecision(candidate[i], baseline[i], depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func compareGenericTypeArgPrecision(candidate, baseline Type, depth int, seen *precisionSeen) (bool, bool) {
	if TypeEquals(candidate, baseline) {
		return false, true
	}
	if b, ok := baseline.(*TypeParam); ok {
		if _, ok := candidate.(*TypeParam); ok {
			return false, false
		}
		if b.Constraint == nil {
			return true, true
		}
		_, comparable := comparePrecision(candidate, b.Constraint, depth+1, seen)
		return comparable, comparable
	}
	if _, ok := candidate.(*TypeParam); ok {
		return false, false
	}
	return comparePrecision(candidate, baseline, depth+1, seen)
}

func compareRecordPrecision(candidate, baseline *Record, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || baseline == nil {
		return false, false
	}
	if candidate.Open && !baseline.Open {
		return false, false
	}
	strict := baseline.Open && !candidate.Open

	if baseline.HasMapComponent() {
		if !candidate.HasMapComponent() {
			return false, false
		}
		keyStrict, ok := comparePrecision(candidate.MapKey, baseline.MapKey, depth+1, seen)
		if !ok {
			return false, false
		}
		valueStrict, ok := comparePrecision(candidate.MapValue, baseline.MapValue, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || keyStrict || valueStrict
	} else if candidate.HasMapComponent() {
		strict = true
	}

	// Metatables drive __index field resolution, so records with divergent
	// metatables are not the same product family even when their direct fields
	// match. A baseline without a metatable is comparable to any candidate; a
	// candidate metatable then refines it. When both carry a metatable, compare
	// them structurally so metatable-divergent records stay distinct.
	switch {
	case baseline.Metatable == nil:
		if candidate.Metatable != nil {
			strict = true
		}
	case candidate.Metatable == nil:
		return false, false
	default:
		metaStrict, ok := comparePrecision(candidate.Metatable, baseline.Metatable, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || metaStrict
	}

	for _, baselineField := range baseline.Fields {
		candidateField := candidate.GetField(baselineField.Name)
		if candidateField == nil {
			if baselineField.Optional {
				strict = true
				continue
			}
			return false, false
		}
		if candidateField.Readonly != baselineField.Readonly {
			return false, false
		}
		if candidateField.Optional && !baselineField.Optional {
			return false, false
		}
		if baselineField.Optional && !candidateField.Optional {
			strict = true
		}
		fieldStrict, ok := comparePrecision(candidateField.Type, baselineField.Type, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || fieldStrict
	}
	if len(candidate.Fields) > len(baseline.Fields) {
		strict = true
	}
	for _, baselineMember := range baseline.StaticMembers {
		candidateMember := candidate.GetStaticMember(baselineMember.Kind, baselineMember.Name, baselineMember.Index)
		if candidateMember == nil {
			if baselineMember.Optional {
				strict = true
				continue
			}
			return false, false
		}
		if candidateMember.Readonly != baselineMember.Readonly {
			return false, false
		}
		if candidateMember.Optional && !baselineMember.Optional {
			return false, false
		}
		if baselineMember.Optional && !candidateMember.Optional {
			strict = true
		}
		memberStrict, ok := comparePrecision(candidateMember.Type, baselineMember.Type, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	if len(candidate.StaticMembers) > len(baseline.StaticMembers) {
		strict = true
	}
	return strict, true
}

func compareMapRecordPrecision(candidate *Map, baseline *Record, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || baseline == nil || !baseline.HasMapComponent() || len(baseline.Fields) != 0 || len(baseline.StaticMembers) != 0 || baseline.Metatable != nil {
		return false, false
	}
	keyStrict, ok := comparePrecision(candidate.Key, baseline.MapKey, depth+1, seen)
	if !ok {
		return false, false
	}
	valueStrict, ok := comparePrecision(candidate.Value, baseline.MapValue, depth+1, seen)
	if !ok {
		return false, false
	}
	return baseline.Open || keyStrict || valueStrict, true
}

func compareUnionPrecision(candidate *Union, baseline Type, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || len(candidate.Members) == 0 {
		return false, false
	}
	strict := false
	for _, member := range candidate.Members {
		memberStrict, ok := comparePrecision(member, baseline, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func compareUnionToUnionPrecision(candidate, baseline *Union, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || baseline == nil || len(candidate.Members) == 0 || len(baseline.Members) == 0 {
		return false, false
	}
	strict := len(candidate.Members) < len(baseline.Members)
	for _, member := range candidate.Members {
		memberStrict, ok := comparePrecisionAgainstUnion(member, baseline, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func comparePrecisionAgainstUnion(candidate Type, baseline *Union, depth int, seen *precisionSeen) (bool, bool) {
	if baseline == nil || len(baseline.Members) == 0 {
		return false, false
	}
	for _, member := range baseline.Members {
		if _, ok := comparePrecision(candidate, member, depth+1, seen); ok {
			return true, true
		}
	}
	return false, false
}

func compareLiteralPrecision(candidate *Literal, baseline Type) (bool, bool) {
	if candidate == nil || baseline == nil {
		return false, false
	}
	switch baseline.Kind() {
	case candidate.Base:
		return true, true
	case kind.Number:
		// Only a numeric literal is in the number family; an integer literal is
		// strictly more precise, a number literal is equal precision. A literal
		// of any other base (e.g. a string literal) is not comparable to number.
		if candidate.Base == kind.Integer || candidate.Base == kind.Number {
			return candidate.Base == kind.Integer, true
		}
		return false, false
	default:
		return false, false
	}
}

func equivalentLocalRefAlias(candidate, baseline Type) bool {
	cRef, cIsRef := candidate.(*Ref)
	bAlias, bIsAlias := baseline.(*Alias)
	if cIsRef && bIsAlias && cRef.Module == "" && cRef.Name == bAlias.Name {
		return true
	}
	cAlias, cIsAlias := candidate.(*Alias)
	bRef, bIsRef := baseline.(*Ref)
	return cIsAlias && bIsRef && bRef.Module == "" && bRef.Name == cAlias.Name
}
