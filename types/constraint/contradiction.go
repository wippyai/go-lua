package constraint

import "github.com/wippyai/go-lua/types/kind"

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
	positive   bool
}

type pathContradictionKind uint8

const (
	pathContradictionTruthy pathContradictionKind = iota + 1
	pathContradictionIsNil
)

type pathContradictionClass struct {
	kind     pathContradictionKind
	path     Path
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
	if canon, ok := exactPathContradictionClassOf(ct); ok {
		return idx.observeExactPathComplement(canon)
	}
	canon, positive, ok := exactContradictionClassOf(ct)
	if !ok {
		return false
	}
	h := canon.Hash()
	for _, seen := range idx.exact[h] {
		if seen.positive == positive {
			continue
		}
		if seen.constraint.Equals(canon) {
			return true
		}
	}
	if idx.exact == nil {
		idx.exact = make(map[uint64][]contradictionClass)
	}
	idx.exact[h] = append(idx.exact[h], contradictionClass{
		constraint: canon,
		positive:   positive,
	})
	return false
}

func (idx *conjunctionContradictionIndex) observeExactPathComplement(canon pathContradictionClass) bool {
	h := pathContradictionClassHash(canon)
	for _, seen := range idx.exactPath[h] {
		if seen.positive == canon.positive || seen.kind != canon.kind {
			continue
		}
		if seen.path.Equal(canon.path) {
			return true
		}
	}
	if idx.exactPath == nil {
		idx.exactPath = make(map[uint64][]pathContradictionClass)
	}
	idx.exactPath[h] = append(idx.exactPath[h], canon)
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

func exactPathContradictionClassOf(ct Constraint) (pathContradictionClass, bool) {
	if !constraintCanContradict(ct) {
		return pathContradictionClass{}, false
	}
	switch v := ct.(type) {
	case Truthy:
		return pathContradictionClass{kind: pathContradictionTruthy, path: v.Path, positive: true}, true
	case Falsy:
		return pathContradictionClass{kind: pathContradictionTruthy, path: v.Path, positive: false}, true
	case IsNil:
		return pathContradictionClass{kind: pathContradictionIsNil, path: v.Path, positive: true}, true
	case NotNil:
		return pathContradictionClass{kind: pathContradictionIsNil, path: v.Path, positive: false}, true
	default:
		return pathContradictionClass{}, false
	}
}

func pathContradictionClassHash(c pathContradictionClass) uint64 {
	h := uint64(c.kind)
	return (h << 1) ^ c.path.Hash()
}

func exactContradictionClassOf(ct Constraint) (Constraint, bool, bool) {
	if !constraintCanContradict(ct) {
		return nil, false, false
	}
	switch v := ct.(type) {
	case Truthy:
		return Truthy{Path: v.Path}, true, true
	case Falsy:
		return Truthy{Path: v.Path}, false, true
	case IsNil:
		return IsNil{Path: v.Path}, true, true
	case NotNil:
		return IsNil{Path: v.Path}, false, true
	case HasType:
		return HasType{Path: v.Path, Type: v.Type}, true, true
	case NotHasType:
		return HasType{Path: v.Path, Type: v.Type}, false, true
	case EqPath:
		return NewEqPath(v.Left, v.Right), true, true
	case NotEqPath:
		return NewEqPath(v.Left, v.Right), false, true
	case FieldEquals:
		return FieldEquals{Target: v.Target, Field: v.Field, Value: v.Value}, true, true
	case FieldNotEquals:
		return FieldEquals{Target: v.Target, Field: v.Field, Value: v.Value}, false, true
	case IndexEquals:
		return IndexEquals{Target: v.Target, Key: v.Key, Value: v.Value}, true, true
	case IndexNotEquals:
		return IndexEquals{Target: v.Target, Key: v.Key, Value: v.Value}, false, true
	case FieldEqualsPath:
		return FieldEqualsPath{Target: v.Target, Field: v.Field, Value: v.Value}, true, true
	case FieldNotEqualsPath:
		return FieldEqualsPath{Target: v.Target, Field: v.Field, Value: v.Value}, false, true
	case IndexEqualsPath:
		return IndexEqualsPath{Target: v.Target, Key: v.Key, Value: v.Value}, true, true
	case IndexNotEqualsPath:
		return IndexEqualsPath{Target: v.Target, Key: v.Key, Value: v.Value}, false, true
	case VariantCaseEquals:
		return VariantCaseEquals{Target: v.Target, OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex}, true, true
	case VariantCaseNotEquals:
		return VariantCaseEquals{Target: v.Target, OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex}, false, true
	default:
		return nil, false, false
	}
}

func constraintCanContradict(ct Constraint) bool {
	return rootStableConstraint(ct) || versionScopedMutableConstraint(ct)
}
