package subst

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

// ExpandInstantiated expands generic instantiations to their Lua
// table-normalized structural form.
//
// For Instantiated{Generic: Array<T>, TypeArgs: [number]}, returns the Array
// body with T replaced by number. This enables structural comparison and
// field/method lookup on instantiated generics.
//
// Does not enforce generic constraints; use subtype checking for that.
func ExpandInstantiated(t typ.Type) typ.Type {
	if t == nil || !inspect.ContainsInstantiated(t) {
		return t
	}
	memo := getExpandMemo()
	defer putExpandMemo(memo)
	guard := typ.GuardForDepth(typ.DefaultRecursionDepth)
	return expandInstantiatedGuardMode(t, guard, memo, expandModeStructural)
}

// ExpandInstantiatedChanged expands generic instantiations and reports whether
// the result differs from the input.
func ExpandInstantiatedChanged(t typ.Type) (typ.Type, bool) {
	expanded := ExpandInstantiated(t)
	if expanded == nil || expanded == t {
		return nil, false
	}
	return expanded, true
}

const expandMemoMaxEntries = 2048

type expandMode uint8

const (
	expandModeStructural expandMode = iota
	expandModeTablePolicy
)

type expandMemoKey struct {
	t    typ.Type
	mode expandMode
}

var expandMemoPool = sync.Pool{
	New: func() any {
		return make(map[expandMemoKey]typ.Type, 32)
	},
}

func getExpandMemo() map[expandMemoKey]typ.Type {
	return expandMemoPool.Get().(map[expandMemoKey]typ.Type)
}

func putExpandMemo(m map[expandMemoKey]typ.Type) {
	if len(m) > expandMemoMaxEntries {
		expandMemoPool.Put(make(map[expandMemoKey]typ.Type, 32))
		return
	}
	clear(m)
	expandMemoPool.Put(m)
}

func expandInstantiatedGuardMode(t typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	if t == nil {
		return t
	}
	if mode == expandModeStructural && !inspect.ContainsInstantiated(t) {
		return t
	}

	key := expandMemoKey{t: t, mode: mode}
	if cached, ok := memo[key]; ok {
		return cached
	}

	next, ok := guard.Enter(t)
	if !ok {
		return t
	}

	orig := t
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == t {
			break
		}
		t = ann.Inner
	}

	key = expandMemoKey{t: orig, mode: mode}
	memo[key] = orig
	result := expandInstantiatedCore(t, orig, next, memo, mode)
	memo[key] = result
	return result
}

func isRecursiveInstantiated(t typ.Type) bool {
	if inst, ok := t.(*typ.Instantiated); ok {
		return inspect.ContainsRecursive(inst) || genericBodySelfInstantiates(inst.Generic)
	}
	return false
}

func genericBodySelfInstantiates(g *typ.Generic) bool {
	if g == nil || g.Body == nil {
		return false
	}
	found := false
	transform.Rewrite(g.Body, func(n typ.Type) (typ.Type, bool) {
		if found {
			return n, true
		}
		if inst, ok := n.(*typ.Instantiated); ok && inst.Generic == g {
			found = true
			return n, true
		}
		return nil, false
	})
	return found
}

func expandInstantiatedCore(t typ.Type, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	switch v := t.(type) {
	case *typ.Instantiated:
		if v.Generic == nil || len(v.TypeArgs) != len(v.Generic.TypeParams) || v.Generic.Body == nil {
			return orig
		}
		body := Params(v.Generic.Body, v.Generic.TypeParams, v.TypeArgs)
		body = Self(body, orig)
		return expandInstantiatedGuardMode(body, guard, memo, expandModeTablePolicy)
	case *typ.Optional:
		inner := expandInstantiatedGuardMode(v.Inner, guard, memo, mode)
		if inner == v.Inner {
			return orig
		}
		return typeexpr.Optional(inner)
	case *typ.Union:
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
	case *typ.Intersection:
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
	case *typ.Array:
		elem := expandInstantiatedGuardMode(v.Element, guard, memo, mode)
		if elem == v.Element {
			return orig
		}
		return typ.NewArray(elem)
	case *typ.Map:
		key := expandInstantiatedGuardMode(v.Key, guard, memo, mode)
		value := expandInstantiatedGuardMode(v.Value, guard, memo, mode)
		if mode == expandModeTablePolicy {
			return typetable.NewMap(key, value)
		}
		if key == v.Key && value == v.Value {
			return orig
		}
		return typetable.NewMap(key, value)
	case *typ.ReadonlyMap:
		key := expandInstantiatedGuardMode(v.Key, guard, memo, mode)
		value := expandInstantiatedGuardMode(v.Value, guard, memo, mode)
		if mode == expandModeTablePolicy {
			return typetable.NewReadonlyMap(key, value)
		}
		if key == v.Key && value == v.Value {
			return orig
		}
		return typetable.NewReadonlyMap(key, value)
	case *typ.Tuple:
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
	case *typ.Function:
		changed := false
		var params []typ.Param
		for i, p := range v.Params {
			newType := p.Type
			if !isRecursiveInstantiated(p.Type) {
				newType = expandInstantiatedGuardMode(p.Type, guard, memo, mode)
			}
			if newType != p.Type {
				if params == nil {
					params = make([]typ.Param, len(v.Params))
					copy(params, v.Params)
				}
				changed = true
				params[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional}
			} else if params != nil {
				params[i] = p
			}
		}

		var returns []typ.Type
		for i, r := range v.Returns {
			newRet := expandInstantiatedGuardMode(r, guard, memo, mode)
			if newRet != r {
				if returns == nil {
					returns = make([]typ.Type, len(v.Returns))
					copy(returns, v.Returns)
				}
				changed = true
				returns[i] = newRet
			} else if returns != nil {
				returns[i] = r
			}
		}

		variadic := v.Variadic
		if v.Variadic != nil {
			newVariadic := expandInstantiatedGuardMode(v.Variadic, guard, memo, mode)
			if newVariadic != v.Variadic {
				changed = true
				variadic = newVariadic
			}
		}

		if !changed {
			return orig
		}

		paramsSrc := v.Params
		if params != nil {
			paramsSrc = params
		}
		returnsSrc := v.Returns
		if returns != nil {
			returnsSrc = returns
		}
		return typ.RebuildFunction(typ.FunctionParts{
			TypeParams: v.TypeParams,
			Params:     paramsSrc,
			Variadic:   variadic,
			Returns:    returnsSrc,
		})
	case *typ.Record:
		changed := false
		var fields []typ.Field
		for i, f := range v.Fields {
			newType := expandInstantiatedGuardMode(f.Type, guard, memo, mode)
			if newType != f.Type {
				if fields == nil {
					fields = make([]typ.Field, len(v.Fields))
					copy(fields, v.Fields)
				}
				changed = true
				fields[i] = typ.Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
			} else if fields != nil {
				fields[i] = f
			}
		}

		var staticMembers []typ.StaticMember
		for i, m := range v.StaticMembers {
			newType := expandInstantiatedGuardMode(m.Type, guard, memo, mode)
			if newType != m.Type {
				if staticMembers == nil {
					staticMembers = make([]typ.StaticMember, len(v.StaticMembers))
					copy(staticMembers, v.StaticMembers)
				}
				changed = true
				staticMembers[i] = typ.StaticMember{
					Kind:     m.Kind,
					Name:     m.Name,
					Index:    m.Index,
					Type:     newType,
					Optional: m.Optional,
					Readonly: m.Readonly,
				}
			} else if staticMembers != nil {
				staticMembers[i] = m
			}
		}

		metatable := v.Metatable
		if v.Metatable != nil {
			newMetatable := expandInstantiatedGuardMode(v.Metatable, guard, memo, mode)
			if newMetatable != v.Metatable {
				changed = true
				metatable = newMetatable
			}
		}

		mapKey := v.MapKey
		mapValue := v.MapValue
		if v.HasMapComponent() {
			mapKey = expandInstantiatedGuardMode(v.MapKey, guard, memo, mode)
			if mapKey != v.MapKey {
				changed = true
			}
			mapValue = expandInstantiatedGuardMode(v.MapValue, guard, memo, mode)
			if mapValue != v.MapValue {
				changed = true
			}
		}

		if !changed && mode == expandModeStructural {
			return orig
		}

		fieldsSrc := v.Fields
		if fields != nil {
			fieldsSrc = fields
		}
		staticMembersSrc := v.StaticMembers
		if staticMembers != nil {
			staticMembersSrc = staticMembers
		}
		return typetable.RebuildRecord(typ.RecordParts{
			Fields:        fieldsSrc,
			StaticMembers: staticMembersSrc,
			Metatable:     metatable,
			MapKey:        mapKey,
			MapValue:      mapValue,
			Open:          v.Open,
			AssumeSorted:  true,
		})
	case *typ.Alias:
		target := expandInstantiatedGuardMode(v.Target, guard, memo, mode)
		if target == v.Target {
			return orig
		}
		return typ.NewAlias(v.Name, target)
	case *typ.Interface:
		changed := false
		var methods []typ.Method
		for idx := range v.Methods {
			m := v.Methods[idx]
			newType := expandInstantiatedGuardMode(m.Type, guard, memo, mode)
			fn, ok := newType.(*typ.Function)
			if !ok {
				fn = m.Type
			}
			if fn != m.Type {
				if methods == nil {
					methods = make([]typ.Method, len(v.Methods))
					copy(methods, v.Methods)
				}
				changed = true
				methods[idx] = typ.Method{Name: m.Name, Type: fn}
			} else if methods != nil {
				methods[idx] = m
			}
		}
		if !changed {
			return orig
		}
		return typ.NewInterface(v.Name, methods)
	case *typ.Ref, *typ.Generic:
		return orig
	default:
		return orig
	}
}
