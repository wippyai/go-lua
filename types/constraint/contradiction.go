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
	exactPath        map[uint64][]pathContradictionClass
	hasType          map[uint64][]hasTypeBuiltinClass
	truthy           map[uint64][]Path
	isNil            map[uint64][]Path
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

type pathContradictionKind uint8

const (
	pathContradictionTruthy pathContradictionKind = iota + 1
	pathContradictionIsNil
)

type pathContradictionClass struct {
	constraint Constraint
	kind       pathContradictionKind
	positive   bool
}

type pathContradictionSignature struct {
	kind     pathContradictionKind
	hash     uint64
	positive bool
}

type hasTypeBuiltinClass struct {
	path Path
	kind kind.Kind
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

func (idx *conjunctionContradictionIndex) Observe(ct Constraint) bool {
	if idx == nil || idx.hasContradiction {
		return idx != nil && idx.hasContradiction
	}
	if idx.observeExactComplement(ct) ||
		idx.observeBuiltinHasType(ct) ||
		idx.observeTruthyNil(ct) {
		idx.hasContradiction = true
	}
	return idx.hasContradiction
}

func (idx *conjunctionContradictionIndex) observeExactComplement(ct Constraint) bool {
	if sig, ok := exactPathContradictionSignatureOf(ct); ok {
		return idx.observeExactPathComplement(sig, ct)
	}
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

func (idx *conjunctionContradictionIndex) observeExactPathComplement(sig pathContradictionSignature, ct Constraint) bool {
	for _, seen := range idx.exactPath[sig.hash] {
		if seen.positive == sig.positive || seen.kind != sig.kind {
			continue
		}
		if pathContradictionClassPath(seen.constraint).Equal(pathContradictionClassPath(ct)) {
			return true
		}
	}
	if idx.exactPath == nil {
		idx.exactPath = make(map[uint64][]pathContradictionClass)
	}
	idx.exactPath[sig.hash] = append(idx.exactPath[sig.hash], pathContradictionClass{
		constraint: ct,
		kind:       sig.kind,
		positive:   sig.positive,
	})
	return false
}

func (idx *conjunctionContradictionIndex) observeBuiltinHasType(ct Constraint) bool {
	hasType, ok := ct.(HasType)
	if !ok || !constraintCanContradict(ct) {
		return false
	}
	builtinKind, ok := hasType.Type.BuiltinKind()
	if !ok {
		return false
	}
	key := hasType.Path.Hash()
	for _, seen := range idx.hasType[key] {
		if seen.path.Equal(hasType.Path) && seen.kind != builtinKind {
			return true
		}
	}
	if idx.hasType == nil {
		idx.hasType = make(map[uint64][]hasTypeBuiltinClass)
	}
	idx.hasType[key] = append(idx.hasType[key], hasTypeBuiltinClass{path: hasType.Path, kind: builtinKind})
	return false
}

func (idx *conjunctionContradictionIndex) observeTruthyNil(ct Constraint) bool {
	switch v := ct.(type) {
	case Truthy:
		if !rootStablePath(v.Path) {
			return false
		}
		key := v.Path.Hash()
		if pathSetContains(idx.isNil[key], v.Path) {
			return true
		}
		if idx.truthy == nil {
			idx.truthy = make(map[uint64][]Path)
		}
		idx.truthy[key] = appendPathSet(idx.truthy[key], v.Path)
	case IsNil:
		if !rootStablePath(v.Path) {
			return false
		}
		key := v.Path.Hash()
		if pathSetContains(idx.truthy[key], v.Path) {
			return true
		}
		if idx.isNil == nil {
			idx.isNil = make(map[uint64][]Path)
		}
		idx.isNil[key] = appendPathSet(idx.isNil[key], v.Path)
	}
	return false
}

func pathSetContains(paths []Path, want Path) bool {
	for _, path := range paths {
		if path.Equal(want) {
			return true
		}
	}
	return false
}

func appendPathSet(paths []Path, path Path) []Path {
	if pathSetContains(paths, path) {
		return paths
	}
	return append(paths, path)
}

func exactPathContradictionSignatureOf(ct Constraint) (pathContradictionSignature, bool) {
	if !constraintCanContradict(ct) {
		return pathContradictionSignature{}, false
	}
	switch v := ct.(type) {
	case Truthy:
		return pathContradictionSignature{kind: pathContradictionTruthy, hash: pathContradictionClassHash(pathContradictionTruthy, v.Path), positive: true}, true
	case Falsy:
		return pathContradictionSignature{kind: pathContradictionTruthy, hash: pathContradictionClassHash(pathContradictionTruthy, v.Path), positive: false}, true
	case IsNil:
		return pathContradictionSignature{kind: pathContradictionIsNil, hash: pathContradictionClassHash(pathContradictionIsNil, v.Path), positive: true}, true
	case NotNil:
		return pathContradictionSignature{kind: pathContradictionIsNil, hash: pathContradictionClassHash(pathContradictionIsNil, v.Path), positive: false}, true
	default:
		return pathContradictionSignature{}, false
	}
}

func pathContradictionClassHash(kind pathContradictionKind, path Path) uint64 {
	h := uint64(kind)
	return (h << 1) ^ path.Hash()
}

func pathContradictionClassPath(ct Constraint) Path {
	switch v := ct.(type) {
	case Truthy:
		return v.Path
	case Falsy:
		return v.Path
	case IsNil:
		return v.Path
	case NotNil:
		return v.Path
	default:
		return Path{}
	}
}

func exactContradictionSignatureOf(ct Constraint) (exactContradictionSignature, bool) {
	if !constraintCanContradict(ct) {
		return exactContradictionSignature{}, false
	}
	switch v := ct.(type) {
	case HasType:
		h := internal.HashCombine(hashPathConstraint(KindHasType, v.Path), v.Type.Hash64())
		return exactContradictionSignature{kind: KindHasType, hash: h, positive: true}, true
	case NotHasType:
		h := internal.HashCombine(hashPathConstraint(KindHasType, v.Path), v.Type.Hash64())
		return exactContradictionSignature{kind: KindHasType, hash: h, positive: false}, true
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
	case KindHasType:
		ap, at, aOK := exactHasTypeClass(a)
		bp, bt, bOK := exactHasTypeClass(b)
		return aOK && bOK && ap.Equal(bp) && at.Equal(bt)
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

func exactHasTypeClass(ct Constraint) (Path, narrow.TypeKey, bool) {
	switch v := ct.(type) {
	case HasType:
		return v.Path, v.Type, true
	case NotHasType:
		return v.Path, v.Type, true
	default:
		return Path{}, narrow.TypeKey{}, false
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
