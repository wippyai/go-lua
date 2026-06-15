package subst

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func expandOptional(v *typ.Optional, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	inner := expandInstantiatedGuardMode(v.Inner, guard, memo, mode)
	if inner == v.Inner {
		return orig
	}
	return typeexpr.Optional(inner)
}

func expandUnion(v *typ.Union, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	var members []typ.Type
	for i, m := range v.Members {
		newMember := expandInstantiatedGuardMode(m, guard, memo, mode)
		if newMember != m {
			if members == nil {
				members = make([]typ.Type, len(v.Members))
				copy(members, v.Members)
			}
			members[i] = newMember
		} else if members != nil {
			members[i] = m
		}
	}
	if members == nil {
		return orig
	}
	return typeexpr.Union(members...)
}

func expandIntersection(v *typ.Intersection, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	var members []typ.Type
	for i, m := range v.Members {
		newMember := expandInstantiatedGuardMode(m, guard, memo, mode)
		if newMember != m {
			if members == nil {
				members = make([]typ.Type, len(v.Members))
				copy(members, v.Members)
			}
			members[i] = newMember
		} else if members != nil {
			members[i] = m
		}
	}
	if members == nil {
		return orig
	}
	return typeexpr.Intersection(members...)
}

func expandArray(v *typ.Array, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	elem := expandInstantiatedGuardMode(v.Element, guard, memo, mode)
	if elem == v.Element {
		return orig
	}
	return typ.NewArray(elem)
}

func expandMap(v *typ.Map, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	key := expandInstantiatedGuardMode(v.Key, guard, memo, mode)
	value := expandInstantiatedGuardMode(v.Value, guard, memo, mode)
	if mode == expandModeTablePolicy {
		return typetable.NewMap(key, value)
	}
	if key == v.Key && value == v.Value {
		return orig
	}
	return typetable.NewMap(key, value)
}

func expandReadonlyMap(v *typ.ReadonlyMap, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	key := expandInstantiatedGuardMode(v.Key, guard, memo, mode)
	value := expandInstantiatedGuardMode(v.Value, guard, memo, mode)
	if mode == expandModeTablePolicy {
		return typetable.NewReadonlyMap(key, value)
	}
	if key == v.Key && value == v.Value {
		return orig
	}
	return typetable.NewReadonlyMap(key, value)
}

func expandTuple(v *typ.Tuple, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	var elems []typ.Type
	for i, e := range v.Elements {
		newElem := expandInstantiatedGuardMode(e, guard, memo, mode)
		if newElem != e {
			if elems == nil {
				elems = make([]typ.Type, len(v.Elements))
				copy(elems, v.Elements)
			}
			elems[i] = newElem
		} else if elems != nil {
			elems[i] = e
		}
	}
	if elems == nil {
		return orig
	}
	return typ.NewTuple(elems...)
}

func expandAlias(v *typ.Alias, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	target := expandInstantiatedGuardMode(v.Target, guard, memo, mode)
	if target == v.Target {
		return orig
	}
	return typ.NewAlias(v.Name, target)
}
