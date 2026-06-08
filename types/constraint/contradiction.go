package constraint

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// conjunctionContradictionIndex is the canonical contradiction projection for a
// conjunction. It groups raw constraints by normalized complement classes so
// contradiction detection stays linear in the number of literals.
type conjunctionContradictionIndex struct {
	hasContradiction bool
	exact            map[uint64][]contradictionClass
	hasType          map[uint64][]hasTypeClass
	paths            map[uint64][]pathPredicateClass
}

type contradictionClass struct {
	constraint Constraint
	kind       Kind
	positive   bool
}

type exactContradictionSignature struct {
	kind     Kind
	hash     uint64
	positive bool
}

type pathPredicateBits uint8

const (
	pathPredicateTruthy pathPredicateBits = 1 << iota
	pathPredicateFalsy
	pathPredicateIsNil
	pathPredicateNotNil
)

type pathPredicateClass struct {
	path Path
	bits pathPredicateBits
}

// hasTypeClass owns both exact HasType/NotHasType negation and the stronger
// rule that two positive builtin type predicates on the same path are exclusive.
type hasTypeClass struct {
	path     Path
	ty       narrow.TypeKey
	positive bool
	builtin  kind.Kind
}

func conjunctionContradictionIndexOf(conj []Constraint) conjunctionContradictionIndex {
	var idx conjunctionContradictionIndex
	for _, ct := range conj {
		if idx.Observe(ct) {
			break
		}
	}
	return idx
}

func (idx conjunctionContradictionIndex) HasContradiction() bool {
	return idx.hasContradiction
}

// conjunctionsContradictAcross composes two already-consistent conjunctions:
// only cross-side contradictions can be introduced by the meet.
func conjunctionsContradictAcross(a, b []Constraint) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	indexed, probed := a, b
	if len(b) < len(a) {
		indexed, probed = b, a
	}
	idx := conjunctionContradictionIndexOf(indexed)
	if idx.HasContradiction() {
		return true
	}
	for _, ct := range probed {
		if idx.Contradicts(ct) {
			return true
		}
	}
	return false
}

func (idx conjunctionContradictionIndex) Contradicts(ct Constraint) bool {
	return idx.hasContradiction ||
		idx.pathPredicateContradicts(ct) ||
		idx.hasTypeContradicts(ct) ||
		idx.exactComplementContradicts(ct)
}

func (idx *conjunctionContradictionIndex) Observe(ct Constraint) bool {
	if idx == nil || idx.hasContradiction {
		return idx != nil && idx.hasContradiction
	}
	if idx.observePathPredicate(ct) ||
		idx.observeHasType(ct) ||
		idx.observeExactComplement(ct) {
		idx.hasContradiction = true
	}
	return idx.hasContradiction
}

func (idx conjunctionContradictionIndex) exactComplementContradicts(ct Constraint) bool {
	sig, ok := exactContradictionSignatureOf(ct)
	if !ok {
		return false
	}
	for _, seen := range idx.exact[sig.hash] {
		if seen.positive == sig.positive || seen.kind != sig.kind {
			continue
		}
		if sameExactContradictionClass(seen.constraint, ct, sig.kind) {
			return true
		}
	}
	return false
}

func (idx *conjunctionContradictionIndex) observeExactComplement(ct Constraint) bool {
	sig, ok := exactContradictionSignatureOf(ct)
	if !ok {
		return false
	}
	for _, seen := range idx.exact[sig.hash] {
		if seen.positive == sig.positive || seen.kind != sig.kind {
			continue
		}
		if sameExactContradictionClass(seen.constraint, ct, sig.kind) {
			return true
		}
	}
	if idx.exact == nil {
		idx.exact = make(map[uint64][]contradictionClass)
	}
	idx.exact[sig.hash] = append(idx.exact[sig.hash], contradictionClass{
		constraint: ct,
		kind:       sig.kind,
		positive:   sig.positive,
	})
	return false
}

func (idx conjunctionContradictionIndex) hasTypeContradicts(ct Constraint) bool {
	path, ty, positive, ok := hasTypeClassOf(ct)
	if !ok {
		return false
	}
	key := path.Hash()
	builtinKind, _ := ty.BuiltinKind()
	for _, seen := range idx.hasType[key] {
		if !seen.path.Equal(path) {
			continue
		}
		if seen.positive != positive && seen.ty.Equal(ty) {
			return true
		}
		if positive && seen.positive &&
			builtinKind != kind.Unknown && seen.builtin != kind.Unknown &&
			seen.builtin != builtinKind {
			return true
		}
	}
	return false
}

func (idx *conjunctionContradictionIndex) observeHasType(ct Constraint) bool {
	path, ty, positive, ok := hasTypeClassOf(ct)
	if !ok {
		return false
	}
	key := path.Hash()
	builtinKind, _ := ty.BuiltinKind()
	for _, seen := range idx.hasType[key] {
		if !seen.path.Equal(path) {
			continue
		}
		if seen.positive != positive && seen.ty.Equal(ty) {
			return true
		}
		if positive && seen.positive &&
			builtinKind != kind.Unknown && seen.builtin != kind.Unknown &&
			seen.builtin != builtinKind {
			return true
		}
	}
	if idx.hasType == nil {
		idx.hasType = make(map[uint64][]hasTypeClass)
	}
	idx.hasType[key] = append(idx.hasType[key], hasTypeClass{
		path:     path,
		ty:       ty,
		positive: positive,
		builtin:  builtinKind,
	})
	return false
}

func hasTypeClassOf(ct Constraint) (Path, narrow.TypeKey, bool, bool) {
	if !constraintCanContradict(ct) {
		return Path{}, narrow.TypeKey{}, false, false
	}
	switch v := ct.(type) {
	case HasType:
		return v.Path, v.Type, true, true
	case NotHasType:
		return v.Path, v.Type, false, true
	default:
		return Path{}, narrow.TypeKey{}, false, false
	}
}

func (idx conjunctionContradictionIndex) pathPredicateContradicts(ct Constraint) bool {
	path, bits, ok := pathPredicateOf(ct)
	if !ok {
		return false
	}
	key := path.Hash()
	for _, seen := range idx.paths[key] {
		if !seen.path.Equal(path) {
			continue
		}
		if pathPredicateContradicts(seen.bits, bits, path) {
			return true
		}
	}
	return false
}

func (idx *conjunctionContradictionIndex) observePathPredicate(ct Constraint) bool {
	path, bits, ok := pathPredicateOf(ct)
	if !ok {
		return false
	}
	key := path.Hash()
	for i, seen := range idx.paths[key] {
		if !seen.path.Equal(path) {
			continue
		}
		if pathPredicateContradicts(seen.bits, bits, path) {
			return true
		}
		idx.paths[key][i].bits |= bits
		return false
	}
	if idx.paths == nil {
		idx.paths = make(map[uint64][]pathPredicateClass)
	}
	idx.paths[key] = append(idx.paths[key], pathPredicateClass{path: path, bits: bits})
	return false
}

func pathPredicateOf(ct Constraint) (Path, pathPredicateBits, bool) {
	if !constraintCanContradict(ct) {
		return Path{}, 0, false
	}
	switch v := ct.(type) {
	case Truthy:
		return v.Path, pathPredicateTruthy, true
	case Falsy:
		return v.Path, pathPredicateFalsy, true
	case IsNil:
		return v.Path, pathPredicateIsNil, true
	case NotNil:
		return v.Path, pathPredicateNotNil, true
	default:
		return Path{}, 0, false
	}
}

func pathPredicateContradicts(seen, next pathPredicateBits, path Path) bool {
	if next&pathPredicateTruthy != 0 && seen&pathPredicateFalsy != 0 {
		return true
	}
	if next&pathPredicateFalsy != 0 && seen&pathPredicateTruthy != 0 {
		return true
	}
	if next&pathPredicateIsNil != 0 && seen&pathPredicateNotNil != 0 {
		return true
	}
	if next&pathPredicateNotNil != 0 && seen&pathPredicateIsNil != 0 {
		return true
	}
	if rootStablePath(path) {
		if next&pathPredicateTruthy != 0 && seen&pathPredicateIsNil != 0 {
			return true
		}
		if next&pathPredicateIsNil != 0 && seen&pathPredicateTruthy != 0 {
			return true
		}
	}
	return false
}

func exactContradictionSignatureOf(ct Constraint) (exactContradictionSignature, bool) {
	if !constraintCanContradict(ct) {
		return exactContradictionSignature{}, false
	}
	switch v := ct.(type) {
	case EqPath:
		eq := NewEqPath(v.Left, v.Right)
		return exactContradictionSignature{kind: KindEqPath, hash: eq.Hash(), positive: true}, true
	case NotEqPath:
		eq := NewEqPath(v.Left, v.Right)
		return exactContradictionSignature{kind: KindEqPath, hash: eq.Hash(), positive: false}, true
	case FieldEquals:
		h := hashFieldEqualsClass(v.Target, v.Field, v.Value)
		return exactContradictionSignature{kind: KindFieldEquals, hash: h, positive: true}, true
	case FieldNotEquals:
		h := hashFieldEqualsClass(v.Target, v.Field, v.Value)
		return exactContradictionSignature{kind: KindFieldEquals, hash: h, positive: false}, true
	case IndexEquals:
		h := hashIndexEqualsClass(v.Target, v.Key, v.Value)
		return exactContradictionSignature{kind: KindIndexEquals, hash: h, positive: true}, true
	case IndexNotEquals:
		h := hashIndexEqualsClass(v.Target, v.Key, v.Value)
		return exactContradictionSignature{kind: KindIndexEquals, hash: h, positive: false}, true
	case FieldEqualsPath:
		h := hashFieldEqualsPathClass(v.Target, v.Field, v.Value)
		return exactContradictionSignature{kind: KindFieldEqualsPath, hash: h, positive: true}, true
	case FieldNotEqualsPath:
		h := hashFieldEqualsPathClass(v.Target, v.Field, v.Value)
		return exactContradictionSignature{kind: KindFieldEqualsPath, hash: h, positive: false}, true
	case IndexEqualsPath:
		h := hashIndexEqualsPathClass(v.Target, v.Key, v.Value)
		return exactContradictionSignature{kind: KindIndexEqualsPath, hash: h, positive: true}, true
	case IndexNotEqualsPath:
		h := hashIndexEqualsPathClass(v.Target, v.Key, v.Value)
		return exactContradictionSignature{kind: KindIndexEqualsPath, hash: h, positive: false}, true
	case VariantCaseEquals:
		h := hashVariantCaseEqualsClass(v.Target, v.OriginFamily, v.CaseIndex)
		return exactContradictionSignature{kind: KindVariantCaseEquals, hash: h, positive: true}, true
	case VariantCaseNotEquals:
		h := hashVariantCaseEqualsClass(v.Target, v.OriginFamily, v.CaseIndex)
		return exactContradictionSignature{kind: KindVariantCaseEquals, hash: h, positive: false}, true
	default:
		return exactContradictionSignature{}, false
	}
}

func hashFieldEqualsClass(target Path, field string, value *typ.Literal) uint64 {
	h := hashPathConstraint(KindFieldEquals, target)
	h = internal.HashCombine(h, internal.FnvString(field))
	if value != nil {
		h = internal.HashCombine(h, value.Hash())
	}
	return h
}

func hashIndexEqualsClass(target Path, key typ.Type, value *typ.Literal) uint64 {
	h := hashPathConstraint(KindIndexEquals, target)
	if key != nil {
		h = internal.HashCombine(h, key.Hash())
	}
	if value != nil {
		h = internal.HashCombine(h, value.Hash())
	}
	return h
}

func hashFieldEqualsPathClass(target Path, field string, value Path) uint64 {
	h := hashPathConstraint(KindFieldEqualsPath, target)
	h = internal.HashCombine(h, internal.FnvString(field))
	return internal.HashCombine(h, value.Hash())
}

func hashIndexEqualsPathClass(target Path, key typ.Type, value Path) uint64 {
	h := hashPathConstraint(KindIndexEqualsPath, target)
	if key != nil {
		h = internal.HashCombine(h, key.Hash())
	}
	return internal.HashCombine(h, value.Hash())
}

func hashVariantCaseEqualsClass(target Path, originFamily uint64, caseIndex int) uint64 {
	h := hashPathConstraint(KindVariantCaseEquals, target)
	h = internal.HashCombine(h, originFamily)
	return internal.HashCombine(h, uint64(caseIndex+1))
}

func sameExactContradictionClass(a, b Constraint, classKind Kind) bool {
	switch classKind {
	case KindEqPath:
		al, ar, aOK := exactEqPathClass(a)
		bl, br, bOK := exactEqPathClass(b)
		return aOK && bOK && al.Equal(bl) && ar.Equal(br)
	case KindFieldEquals:
		at, af, av, aOK := exactFieldEqualsClass(a)
		bt, bf, bv, bOK := exactFieldEqualsClass(b)
		return aOK && bOK && at.Equal(bt) && af == bf && typ.LiteralEquals(av, bv)
	case KindIndexEquals:
		at, ak, av, aOK := exactIndexEqualsClass(a)
		bt, bk, bv, bOK := exactIndexEqualsClass(b)
		return aOK && bOK && at.Equal(bt) && typeEqualsNilSafe(ak, bk) && typ.LiteralEquals(av, bv)
	case KindFieldEqualsPath:
		at, af, av, aOK := exactFieldEqualsPathClass(a)
		bt, bf, bv, bOK := exactFieldEqualsPathClass(b)
		return aOK && bOK && at.Equal(bt) && af == bf && av.Equal(bv)
	case KindIndexEqualsPath:
		at, ak, av, aOK := exactIndexEqualsPathClass(a)
		bt, bk, bv, bOK := exactIndexEqualsPathClass(b)
		return aOK && bOK && at.Equal(bt) && typeEqualsNilSafe(ak, bk) && av.Equal(bv)
	case KindVariantCaseEquals:
		at, af, ai, aOK := exactVariantCaseClass(a)
		bt, bf, bi, bOK := exactVariantCaseClass(b)
		return aOK && bOK && at.Equal(bt) && af == bf && ai == bi
	default:
		return false
	}
}

func exactEqPathClass(ct Constraint) (Path, Path, bool) {
	switch v := ct.(type) {
	case EqPath:
		eq := NewEqPath(v.Left, v.Right)
		return eq.Left, eq.Right, true
	case NotEqPath:
		eq := NewEqPath(v.Left, v.Right)
		return eq.Left, eq.Right, true
	default:
		return Path{}, Path{}, false
	}
}

func exactFieldEqualsClass(ct Constraint) (Path, string, *typ.Literal, bool) {
	switch v := ct.(type) {
	case FieldEquals:
		return v.Target, v.Field, v.Value, true
	case FieldNotEquals:
		return v.Target, v.Field, v.Value, true
	default:
		return Path{}, "", nil, false
	}
}

func exactIndexEqualsClass(ct Constraint) (Path, typ.Type, *typ.Literal, bool) {
	switch v := ct.(type) {
	case IndexEquals:
		return v.Target, v.Key, v.Value, true
	case IndexNotEquals:
		return v.Target, v.Key, v.Value, true
	default:
		return Path{}, nil, nil, false
	}
}

func exactFieldEqualsPathClass(ct Constraint) (Path, string, Path, bool) {
	switch v := ct.(type) {
	case FieldEqualsPath:
		return v.Target, v.Field, v.Value, true
	case FieldNotEqualsPath:
		return v.Target, v.Field, v.Value, true
	default:
		return Path{}, "", Path{}, false
	}
}

func exactIndexEqualsPathClass(ct Constraint) (Path, typ.Type, Path, bool) {
	switch v := ct.(type) {
	case IndexEqualsPath:
		return v.Target, v.Key, v.Value, true
	case IndexNotEqualsPath:
		return v.Target, v.Key, v.Value, true
	default:
		return Path{}, nil, Path{}, false
	}
}

func exactVariantCaseClass(ct Constraint) (Path, uint64, int, bool) {
	switch v := ct.(type) {
	case VariantCaseEquals:
		return v.Target, v.OriginFamily, v.CaseIndex, true
	case VariantCaseNotEquals:
		return v.Target, v.OriginFamily, v.CaseIndex, true
	default:
		return Path{}, 0, 0, false
	}
}

func typeEqualsNilSafe(a, b typ.Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equals(b)
}

func constraintCanContradict(ct Constraint) bool {
	return rootStableConstraint(ct) || versionScopedMutableConstraint(ct)
}
