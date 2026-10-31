package constraint

import "github.com/wippyai/go-lua/types/cfg"

// VersionRef identifies one SSA version of a symbol.
type VersionRef struct {
	Symbol  cfg.SymbolID
	Version int
}

// PathMapper rewrites one path of a constraint. It returns the rewritten path
// and whether the rewrite applies to that path.
type PathMapper func(Path) (Path, bool)

// MapPaths rewrites the paths of c with fn.
//
// A constraint over one path maps to nil when fn does not apply to it. A
// constraint over two paths maps to nil when fn applies to neither, and keeps
// the original of a side fn does not apply to.
func MapPaths(c Constraint, fn PathMapper) Constraint {
	return VisitConstraint(c, ConstraintVisitor[Constraint]{
		Truthy: func(v Truthy) Constraint {
			if p, ok := fn(v.Path); ok {
				return Truthy{Path: p}
			}
			return nil
		},
		Falsy: func(v Falsy) Constraint {
			if p, ok := fn(v.Path); ok {
				return Falsy{Path: p}
			}
			return nil
		},
		IsNil: func(v IsNil) Constraint {
			if p, ok := fn(v.Path); ok {
				return IsNil{Path: p}
			}
			return nil
		},
		NotNil: func(v NotNil) Constraint {
			if p, ok := fn(v.Path); ok {
				return NotNil{Path: p}
			}
			return nil
		},
		HasType: func(v HasType) Constraint {
			if p, ok := fn(v.Path); ok {
				return HasType{Path: p, Type: v.Type}
			}
			return nil
		},
		NotHasType: func(v NotHasType) Constraint {
			if p, ok := fn(v.Path); ok {
				return NotHasType{Path: p, Type: v.Type}
			}
			return nil
		},
		HasField: func(v HasField) Constraint {
			if p, ok := fn(v.Path); ok {
				return HasField{Path: p, Field: v.Field}
			}
			return nil
		},
		FieldEquals: func(v FieldEquals) Constraint {
			if p, ok := fn(v.Target); ok {
				return FieldEquals{Target: p, Field: v.Field, Value: v.Value}
			}
			return nil
		},
		FieldNotEquals: func(v FieldNotEquals) Constraint {
			if p, ok := fn(v.Target); ok {
				return FieldNotEquals{Target: p, Field: v.Field, Value: v.Value}
			}
			return nil
		},
		IndexEquals: func(v IndexEquals) Constraint {
			if p, ok := fn(v.Target); ok {
				return IndexEquals{Target: p, Key: v.Key, Value: v.Value}
			}
			return nil
		},
		IndexNotEquals: func(v IndexNotEquals) Constraint {
			if p, ok := fn(v.Target); ok {
				return IndexNotEquals{Target: p, Key: v.Key, Value: v.Value}
			}
			return nil
		},
		EqPath: func(v EqPath) Constraint {
			if left, right, ok := mapPathPair(v.Left, v.Right, fn); ok {
				return NewEqPath(left, right)
			}
			return nil
		},
		NotEqPath: func(v NotEqPath) Constraint {
			if left, right, ok := mapPathPair(v.Left, v.Right, fn); ok {
				return NewNotEqPath(left, right)
			}
			return nil
		},
		FieldEqualsPath: func(v FieldEqualsPath) Constraint {
			if target, value, ok := mapPathPair(v.Target, v.Value, fn); ok {
				return FieldEqualsPath{Target: target, Field: v.Field, Value: value}
			}
			return nil
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) Constraint {
			if target, value, ok := mapPathPair(v.Target, v.Value, fn); ok {
				return FieldNotEqualsPath{Target: target, Field: v.Field, Value: value}
			}
			return nil
		},
		IndexEqualsPath: func(v IndexEqualsPath) Constraint {
			if target, value, ok := mapPathPair(v.Target, v.Value, fn); ok {
				return IndexEqualsPath{Target: target, Key: v.Key, Value: value}
			}
			return nil
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) Constraint {
			if target, value, ok := mapPathPair(v.Target, v.Value, fn); ok {
				return IndexNotEqualsPath{Target: target, Key: v.Key, Value: value}
			}
			return nil
		},
		KeyOf: func(v KeyOf) Constraint {
			if table, key, ok := mapPathPair(v.Table, v.Key, fn); ok {
				return KeyOf{Table: table, Key: key}
			}
			return nil
		},
		Default: func(Constraint) Constraint {
			return c
		},
	})
}

func mapPathPair(a, b Path, fn PathMapper) (Path, Path, bool) {
	mappedA, okA := fn(a)
	mappedB, okB := fn(b)
	if !okA && !okB {
		return a, b, false
	}
	if !okA {
		mappedA = a
	}
	if !okB {
		mappedB = b
	}
	return mappedA, mappedB, true
}

// RenameVersions rewrites every path of c bound to a version in renames
// (symbol and version ID) to the version it is renamed to. Constraints over
// other paths are returned unchanged.
func RenameVersions(c Condition, renames map[VersionRef]int) Condition {
	if len(renames) == 0 || !c.HasConstraints() {
		return c
	}
	rename := func(p Path) (Path, bool) {
		if p.Symbol == 0 || p.Version == 0 {
			return p, true
		}
		if to, ok := renames[VersionRef{Symbol: p.Symbol, Version: p.Version}]; ok {
			p.Version = to
		}
		return p, true
	}
	out := make([][]Constraint, 0, len(c.Disjuncts))
	for _, conj := range c.Disjuncts {
		mapped := make([]Constraint, 0, len(conj))
		for _, ct := range conj {
			mapped = append(mapped, MapPaths(ct, rename))
		}
		out = append(out, canonicalizeConjunction(mapped))
	}
	return normalizeCondition(Condition{Disjuncts: out})
}
