// Package subst provides type substitution operations for generics.
//
// These operations replace type parameters with concrete types,
// used during generic instantiation and Self type resolution.
package subst

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/internal/recursion"
	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Substitute replaces type parameters with concrete types throughout a type.
//
// Used during generic instantiation to replace TypeParam references with
// the corresponding type arguments. The subs map keys are type parameter names.
func Substitute(t typ.Type, subs map[string]typ.Type) typ.Type {
	if len(subs) == 0 {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if tp, ok := n.(*typ.TypeParam); ok {
			if sub, ok := subs[tp.Name]; ok {
				return sub, true
			}
		}
		return nil, false
	})
}

// Params replaces type parameters with corresponding type arguments.
func Params(t typ.Type, params []*typ.TypeParam, args []typ.Type) typ.Type {
	if len(params) != len(args) || len(params) == 0 {
		return t
	}
	subs := make([]paramSubstitution, 0, len(params))
	for i, p := range params {
		if p == nil || args[i] == nil {
			continue
		}
		subs = append(subs, paramSubstitution{param: p, arg: args[i]})
	}
	if len(subs) == 0 {
		return t
	}
	return substituteParams(t, subs)
}

type paramSubstitution struct {
	param *typ.TypeParam
	arg   typ.Type
	exact bool
}

func substituteParams(t typ.Type, subs []paramSubstitution) typ.Type {
	if t == nil || len(subs) == 0 {
		return t
	}
	if fn, ok := t.(*typ.Function); ok {
		return substituteFunctionParams(fn, subs)
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if tp, ok := n.(*typ.TypeParam); ok {
			if arg, found := lookupParamSubstitution(tp, subs); found {
				return arg, true
			}
			return nil, false
		}
		if fn, ok := n.(*typ.Function); ok {
			return substituteFunctionParams(fn, subs), true
		}
		return nil, false
	})
}

func substituteFunctionParams(fn *typ.Function, subs []paramSubstitution) typ.Type {
	if fn == nil || len(subs) == 0 {
		return fn
	}

	owned := functionOwnsSubstitutions(fn, subs)
	bodySubs := subs
	keptTypeParams := make([]*typ.TypeParam, 0, len(fn.TypeParams))
	for _, tp := range fn.TypeParams {
		if tp == nil {
			continue
		}
		if owned[tp] {
			continue
		}
		bodySubs = removeShadowedSubstitutions(bodySubs, tp)
		keptTypeParams = append(keptTypeParams, tp)
	}

	params := make([]typ.Param, len(fn.Params))
	paramsChanged := false
	for i, p := range fn.Params {
		pt := substituteParams(p.Type, bodySubs)
		if pt != p.Type {
			paramsChanged = true
		}
		params[i] = typ.Param{Name: p.Name, Type: pt, Optional: p.Optional}
	}

	variadic := fn.Variadic
	if variadic != nil {
		variadic = substituteParams(variadic, bodySubs)
	}

	returns := make([]typ.Type, len(fn.Returns))
	returnsChanged := false
	for i, ret := range fn.Returns {
		rt := substituteParams(ret, bodySubs)
		if rt != ret {
			returnsChanged = true
		}
		returns[i] = rt
	}

	typeParamsChanged := len(keptTypeParams) != len(fn.TypeParams)
	if !typeParamsChanged && !paramsChanged && variadic == fn.Variadic && !returnsChanged {
		return fn
	}

	builder := typ.Func()
	for _, tp := range keptTypeParams {
		builder.TypeParamRef(tp)
	}
	for _, p := range params {
		if p.Optional {
			builder.OptParam(p.Name, p.Type)
		} else {
			builder.Param(p.Name, p.Type)
		}
	}
	if variadic != nil {
		builder.Variadic(variadic)
	}
	if len(returns) > 0 {
		builder.Returns(returns...)
	}
	return builder.Build()
}

func functionOwnsSubstitutions(fn *typ.Function, subs []paramSubstitution) map[*typ.TypeParam]bool {
	owned := make(map[*typ.TypeParam]bool, len(fn.TypeParams))
	for _, tp := range fn.TypeParams {
		for _, sub := range subs {
			if tp != nil && tp == sub.param {
				owned[tp] = true
				break
			}
		}
	}
	return owned
}

func removeShadowedSubstitutions(subs []paramSubstitution, binder *typ.TypeParam) []paramSubstitution {
	if binder == nil || len(subs) == 0 {
		return subs
	}
	out := make([]paramSubstitution, 0, len(subs))
	changed := false
	for _, sub := range subs {
		if shadowsSubstitution(binder, sub) {
			changed = true
			continue
		}
		out = append(out, sub)
	}
	if !changed {
		return subs
	}
	return out
}

func shadowsSubstitution(binder *typ.TypeParam, sub paramSubstitution) bool {
	if binder == nil || sub.param == nil {
		return false
	}
	return binder == sub.param || binder.Name == sub.param.Name || binder.Equals(sub.param)
}

func lookupParamSubstitution(tp *typ.TypeParam, subs []paramSubstitution) (typ.Type, bool) {
	if tp == nil {
		return nil, false
	}
	for _, sub := range subs {
		if sub.param == nil {
			continue
		}
		if tp == sub.param || (!sub.exact && tp.Equals(sub.param)) {
			return sub.arg, true
		}
	}
	return nil, false
}

// Self replaces Self type references with a concrete type.
// Does not recurse into Interface types because Self inside an Interface
// is a separate binding that refers to that Interface's implementor.
func Self(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	if !containsSubstitutableSelf(t, make(map[typ.Type]bool)) {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		return nil, false
	})
}

func containsSubstitutableSelf(t typ.Type, seen map[typ.Type]bool) bool {
	if t == nil {
		return false
	}
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if t.Kind() == kind.Self {
		return true
	}
	if _, ok := t.(*typ.Interface); ok {
		return false
	}
	if _, ok := t.(*typ.Recursive); ok {
		return false
	}
	if typ.ContainsRecursive(t) && !containsSurfaceSelf(t) {
		return false
	}
	if seen[t] {
		return false
	}
	if len(seen) >= selfSubstitutionNodeBudget {
		return false
	}
	seen[t] = true
	switch v := t.(type) {
	case *typ.Optional:
		return containsSubstitutableSelf(v.Inner, seen)
	case *typ.Union:
		for _, member := range v.Members {
			if containsSubstitutableSelf(member, seen) {
				return true
			}
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			if containsSubstitutableSelf(member, seen) {
				return true
			}
		}
	case *typ.Array:
		return containsSubstitutableSelf(v.Element, seen)
	case *typ.Map:
		return containsSubstitutableSelf(v.Key, seen) ||
			containsSubstitutableSelf(v.Value, seen)
	case *typ.ReadonlyMap:
		return containsSubstitutableSelf(v.Key, seen) ||
			containsSubstitutableSelf(v.Value, seen)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if containsSubstitutableSelf(elem, seen) {
				return true
			}
		}
	case *typ.Function:
		for _, param := range v.Params {
			if containsSubstitutableSelf(param.Type, seen) {
				return true
			}
		}
		if containsSubstitutableSelf(v.Variadic, seen) {
			return true
		}
		for _, ret := range v.Returns {
			if containsSubstitutableSelf(ret, seen) {
				return true
			}
		}
	case *typ.Record:
		for _, field := range v.Fields {
			if containsSubstitutableSelf(field.Type, seen) {
				return true
			}
		}
		for _, member := range v.StaticMembers {
			if containsSubstitutableSelf(member.Type, seen) {
				return true
			}
		}
		if containsSubstitutableSelf(v.Metatable, seen) {
			return true
		}
		if v.HasMapComponent() {
			return containsSubstitutableSelf(v.MapKey, seen) ||
				containsSubstitutableSelf(v.MapValue, seen)
		}
	case *typ.Alias:
		return containsSubstitutableSelf(v.Target, seen)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if containsSubstitutableSelf(arg, seen) {
				return true
			}
		}
	}
	return false
}

func containsSurfaceSelf(t typ.Type) bool {
	return containsSurfaceSelfSeen(t, make(map[typ.Type]bool))
}

func containsSurfaceSelfSeen(t typ.Type, seen map[typ.Type]bool) bool {
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if t.Kind() == kind.Self {
		return true
	}
	if _, ok := t.(*typ.Interface); ok {
		return false
	}
	if _, ok := t.(*typ.Recursive); ok {
		return false
	}
	if seen[t] {
		return false
	}
	if len(seen) >= selfSubstitutionNodeBudget {
		return false
	}
	seen[t] = true
	switch v := t.(type) {
	case *typ.Optional:
		return containsSurfaceSelfSeen(v.Inner, seen)
	case *typ.Union:
		for _, member := range v.Members {
			if containsSurfaceSelfSeen(member, seen) {
				return true
			}
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			if containsSurfaceSelfSeen(member, seen) {
				return true
			}
		}
	case *typ.Array:
		return containsSurfaceSelfSeen(v.Element, seen)
	case *typ.Map:
		return containsSurfaceSelfSeen(v.Key, seen) || containsSurfaceSelfSeen(v.Value, seen)
	case *typ.ReadonlyMap:
		return containsSurfaceSelfSeen(v.Key, seen) || containsSurfaceSelfSeen(v.Value, seen)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if containsSurfaceSelfSeen(elem, seen) {
				return true
			}
		}
	case *typ.Function:
		for _, param := range v.Params {
			if containsSurfaceSelfSeen(param.Type, seen) {
				return true
			}
		}
		if containsSurfaceSelfSeen(v.Variadic, seen) {
			return true
		}
		for _, ret := range v.Returns {
			if containsSurfaceSelfSeen(ret, seen) {
				return true
			}
		}
	case *typ.Record:
		for _, field := range v.Fields {
			if containsSurfaceSelfSeen(field.Type, seen) {
				return true
			}
		}
		for _, member := range v.StaticMembers {
			if containsSurfaceSelfSeen(member.Type, seen) {
				return true
			}
		}
		if containsSurfaceSelfSeen(v.Metatable, seen) {
			return true
		}
		if v.HasMapComponent() {
			return containsSurfaceSelfSeen(v.MapKey, seen) || containsSurfaceSelfSeen(v.MapValue, seen)
		}
	case *typ.Alias:
		return containsSurfaceSelfSeen(v.Target, seen)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if containsSurfaceSelfSeen(arg, seen) {
				return true
			}
		}
	}
	return false
}

const selfSubstitutionNodeBudget = 2048

// SelfValue replaces free Self references in a runtime value type. Nested
// function and interface types bind their own Self, so substitution stops at
// those boundaries.
func SelfValue(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		switch n.(type) {
		case *typ.Function, *typ.Interface:
			return n, true
		}
		return nil, false
	})
}

// ExpandInstantiated expands generic instantiations to their structural form.
//
// For Instantiated{Generic: Array<T>, TypeArgs: [number]}, returns the Array
// body with T replaced by number. This enables structural comparison and
// field/method lookup on instantiated generics.
//
// Does not enforce generic constraints; use subtype checking for that.
func ExpandInstantiated(t typ.Type) typ.Type {
	if t == nil || !typ.ContainsInstantiated(t) {
		return t
	}
	memo := getExpandMemo()
	defer putExpandMemo(memo)
	guard := typ.GuardForDepth(typ.DefaultRecursionDepth)
	return expandInstantiatedGuard(t, guard, memo)
}

const expandMemoMaxEntries = 2048

var expandMemoPool = sync.Pool{
	New: func() any {
		return make(map[typ.Type]typ.Type, 32)
	},
}

func getExpandMemo() map[typ.Type]typ.Type {
	return expandMemoPool.Get().(map[typ.Type]typ.Type)
}

func putExpandMemo(m map[typ.Type]typ.Type) {
	if len(m) > expandMemoMaxEntries {
		expandMemoPool.Put(make(map[typ.Type]typ.Type, 32))
		return
	}
	clear(m)
	expandMemoPool.Put(m)
}

func expandInstantiatedGuard(t typ.Type, guard recursion.Guard, memo map[typ.Type]typ.Type) typ.Type {
	if t == nil || !typ.ContainsInstantiated(t) {
		return t
	}

	if cached, ok := memo[t]; ok {
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

	memo[orig] = orig
	result := expandInstantiatedCore(t, orig, next, memo)
	memo[orig] = result
	return result
}

func isRecursiveInstantiated(t typ.Type) bool {
	if inst, ok := t.(*typ.Instantiated); ok {
		return typ.ContainsRecursive(inst) || genericBodySelfInstantiates(inst.Generic)
	}
	return false
}

func genericBodySelfInstantiates(g *typ.Generic) bool {
	if g == nil || g.Body == nil {
		return false
	}
	found := false
	typ.Rewrite(g.Body, func(n typ.Type) (typ.Type, bool) {
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

func expandInstantiatedCore(t typ.Type, orig typ.Type, guard recursion.Guard, memo map[typ.Type]typ.Type) typ.Type {
	switch v := t.(type) {
	case *typ.Instantiated:
		if v.Generic == nil || len(v.TypeArgs) != len(v.Generic.TypeParams) || v.Generic.Body == nil {
			return orig
		}
		body := Params(v.Generic.Body, v.Generic.TypeParams, v.TypeArgs)
		body = Self(body, orig)
		return expandInstantiatedGuard(body, guard, memo)
	case *typ.Optional:
		inner := expandInstantiatedGuard(v.Inner, guard, memo)
		if inner == v.Inner {
			return orig
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		var members []typ.Type
		for i, m := range v.Members {
			newMember := expandInstantiatedGuard(m, guard, memo)
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
		return typ.NewUnion(members...)
	case *typ.Intersection:
		var members []typ.Type
		for i, m := range v.Members {
			newMember := expandInstantiatedGuard(m, guard, memo)
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
		return typ.NewIntersection(members...)
	case *typ.Array:
		elem := expandInstantiatedGuard(v.Element, guard, memo)
		if elem == v.Element {
			return orig
		}
		return typ.NewArray(elem)
	case *typ.Map:
		key := expandInstantiatedGuard(v.Key, guard, memo)
		value := expandInstantiatedGuard(v.Value, guard, memo)
		if key == v.Key && value == v.Value {
			return orig
		}
		return luatable.NewMap(key, value)
	case *typ.ReadonlyMap:
		key := expandInstantiatedGuard(v.Key, guard, memo)
		value := expandInstantiatedGuard(v.Value, guard, memo)
		if key == v.Key && value == v.Value {
			return orig
		}
		return luatable.NewReadonlyMap(key, value)
	case *typ.Tuple:
		var elems []typ.Type
		for i, e := range v.Elements {
			newElem := expandInstantiatedGuard(e, guard, memo)
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
				newType = expandInstantiatedGuard(p.Type, guard, memo)
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
			newRet := expandInstantiatedGuard(r, guard, memo)
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
			newVariadic := expandInstantiatedGuard(v.Variadic, guard, memo)
			if newVariadic != v.Variadic {
				changed = true
				variadic = newVariadic
			}
		}

		if !changed {
			return orig
		}

		builder := typ.Func()
		for _, tp := range v.TypeParams {
			builder.TypeParamRef(tp)
		}
		paramsSrc := v.Params
		if params != nil {
			paramsSrc = params
		}
		for _, p := range paramsSrc {
			if p.Optional {
				builder = builder.OptParam(p.Name, p.Type)
			} else {
				builder = builder.Param(p.Name, p.Type)
			}
		}
		if variadic != nil {
			builder = builder.Variadic(variadic)
		}
		returnsSrc := v.Returns
		if returns != nil {
			returnsSrc = returns
		}
		if len(returnsSrc) > 0 {
			builder = builder.Returns(returnsSrc...)
		}
		return builder.Build()
	case *typ.Record:
		changed := false
		var fields []typ.Field
		for i, f := range v.Fields {
			newType := expandInstantiatedGuard(f.Type, guard, memo)
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

		metatable := v.Metatable
		if v.Metatable != nil {
			newMetatable := expandInstantiatedGuard(v.Metatable, guard, memo)
			if newMetatable != v.Metatable {
				changed = true
				metatable = newMetatable
			}
		}

		mapKey := v.MapKey
		mapValue := v.MapValue
		if v.HasMapComponent() {
			mapKey = expandInstantiatedGuard(v.MapKey, guard, memo)
			if mapKey != v.MapKey {
				changed = true
			}
			mapValue = expandInstantiatedGuard(v.MapValue, guard, memo)
			if mapValue != v.MapValue {
				changed = true
			}
		}

		if !changed {
			return orig
		}

		builder := luatable.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		fieldsSrc := v.Fields
		if fields != nil {
			fieldsSrc = fields
		}
		for _, f := range fieldsSrc {
			switch {
			case f.Optional && f.Readonly:
				builder = builder.OptReadonlyField(f.Name, f.Type)
			case f.Optional:
				builder = builder.OptField(f.Name, f.Type)
			case f.Readonly:
				builder = builder.ReadonlyField(f.Name, f.Type)
			default:
				builder = builder.Field(f.Name, f.Type)
			}
		}
		if metatable != nil {
			builder = builder.Metatable(metatable)
		}
		if mapKey != nil && mapValue != nil {
			builder = builder.MapComponent(mapKey, mapValue)
		}
		return builder.Build()
	case *typ.Alias:
		target := expandInstantiatedGuard(v.Target, guard, memo)
		if target == v.Target {
			return orig
		}
		return typ.NewAlias(v.Name, target)
	case *typ.Interface:
		changed := false
		var methods []typ.Method
		for idx := range v.Methods {
			m := v.Methods[idx]
			newType := expandInstantiatedGuard(m.Type, guard, memo)
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
