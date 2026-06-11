package typ

import "github.com/wippyai/go-lua/analysis/internal/recursion"

func rewriteFunction(v *Function, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	changed := false

	var typeParams []*TypeParam
	var typeParamReplacements map[*TypeParam]*TypeParam
	for i, p := range v.TypeParams {
		newParamType := rewriteDepth(p, fn, guard, memo)
		newParam, ok := newParamType.(*TypeParam)
		if !ok || newParam == p {
			if typeParams != nil {
				typeParams[i] = p
			}
			continue
		}
		if typeParams == nil {
			typeParams = make([]*TypeParam, len(v.TypeParams))
			copy(typeParams, v.TypeParams)
		}
		changed = true
		typeParams[i] = newParam
		if typeParamReplacements == nil {
			typeParamReplacements = make(map[*TypeParam]*TypeParam)
		}
		typeParamReplacements[p] = newParam
	}
	rewriteChild := rewriteFnWithTypeParamReplacements(fn, typeParamReplacements)

	var params []Param
	for i, p := range v.Params {
		newType := rewriteDepth(p.Type, rewriteChild, guard, memo)
		if newType != p.Type {
			if params == nil {
				params = make([]Param, len(v.Params))
				copy(params, v.Params)
			}
			changed = true
			params[i] = Param{Name: p.Name, Type: newType, Optional: p.Optional}
		} else if params != nil {
			params[i] = p
		}
	}

	var returns []Type
	for i, r := range v.Returns {
		newRet := rewriteDepth(r, rewriteChild, guard, memo)
		if newRet != r {
			if returns == nil {
				returns = make([]Type, len(v.Returns))
				copy(returns, v.Returns)
			}
			changed = true
			returns[i] = newRet
		} else if returns != nil {
			returns[i] = r
		}
	}

	var variadic Type
	if v.Variadic != nil {
		variadic = rewriteDepth(v.Variadic, rewriteChild, guard, memo)
		if variadic != v.Variadic {
			changed = true
		}
	}

	if !changed {
		return orig
	}

	paramSrc := v.Params
	if params != nil {
		paramSrc = params
	}
	typeParamSrc := v.TypeParams
	if typeParams != nil {
		typeParamSrc = typeParams
	}
	returnsSrc := v.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return buildFunctionType(
		typeParamSrc,
		paramSrc,
		variadic,
		returnsSrc,
		v.Effect,
	)
}

func rewriteMeta(v *Meta, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	of := rewriteDepth(v.Of, fn, guard, memo)
	if of == v.Of {
		return orig
	}
	return NewMeta(of)
}

func rewriteTypeParam(v *TypeParam, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	constraint := rewriteDepth(v.Constraint, fn, guard, memo)
	if constraint == v.Constraint {
		return orig
	}
	return NewTypeParam(v.Name, constraint)
}

func rewriteGeneric(v *Generic, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	changed := false

	var typeParams []*TypeParam
	var typeParamReplacements map[*TypeParam]*TypeParam
	for i, p := range v.TypeParams {
		newParamType := rewriteDepth(p, fn, guard, memo)
		newParam, ok := newParamType.(*TypeParam)
		if !ok || newParam == p {
			if typeParams != nil {
				typeParams[i] = p
			}
			continue
		}
		if typeParams == nil {
			typeParams = make([]*TypeParam, len(v.TypeParams))
			copy(typeParams, v.TypeParams)
		}
		changed = true
		typeParams[i] = newParam
		if typeParamReplacements == nil {
			typeParamReplacements = make(map[*TypeParam]*TypeParam)
		}
		typeParamReplacements[p] = newParam
	}

	body := rewriteDepth(v.Body, rewriteFnWithTypeParamReplacements(fn, typeParamReplacements), guard, memo)
	if body != v.Body {
		changed = true
	}
	if !changed {
		return orig
	}
	typeParamSrc := v.TypeParams
	if typeParams != nil {
		typeParamSrc = typeParams
	}
	return NewGeneric(v.Name, typeParamSrc, body)
}

func rewriteFnWithTypeParamReplacements(fn func(Type) (Type, bool), replacements map[*TypeParam]*TypeParam) func(Type) (Type, bool) {
	if len(replacements) == 0 {
		return fn
	}
	return func(t Type) (Type, bool) {
		if tp, ok := t.(*TypeParam); ok {
			if replacement, ok := replacements[tp]; ok {
				return replacement, true
			}
		}
		return fn(t)
	}
}

func rewriteRecursive(v *Recursive, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	if v.Body == nil {
		return orig
	}
	var replacement *Recursive
	replacementNode := func() *Recursive {
		if replacement == nil {
			replacement = NewRecursivePlaceholder(v.Name)
		}
		return replacement
	}
	selfAware := func(t Type) (Type, bool) {
		if IsRecursiveRef(t, v) {
			return replacementNode(), true
		}
		return fn(t)
	}
	body := rewriteDepth(v.Body, selfAware, guard, memo)
	if body == v.Body {
		return orig
	}
	replacement = replacementNode()
	replacement.SetBody(body)
	return replacement
}

func rewriteRecord(v *Record, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	changed := false

	var fields []Field
	for i, f := range v.Fields {
		newType := rewriteDepth(f.Type, fn, guard, memo)
		if newType != f.Type {
			if fields == nil {
				fields = make([]Field, len(v.Fields))
				copy(fields, v.Fields)
			}
			changed = true
			fields[i] = Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
		} else if fields != nil {
			fields[i] = f
		}
	}
	var staticMembers []StaticMember
	for i, m := range v.StaticMembers {
		newType := rewriteDepth(m.Type, fn, guard, memo)
		if newType != m.Type {
			if staticMembers == nil {
				staticMembers = make([]StaticMember, len(v.StaticMembers))
				copy(staticMembers, v.StaticMembers)
			}
			changed = true
			staticMembers[i].Type = newType
		} else if staticMembers != nil {
			staticMembers[i] = m
		}
	}

	var metatable Type
	if v.Metatable != nil {
		metatable = rewriteDepth(v.Metatable, fn, guard, memo)
		if metatable != v.Metatable {
			changed = true
		}
	}

	mapKey := v.MapKey
	mapValue := v.MapValue
	if v.HasMapComponent() {
		mapKey = rewriteDepth(v.MapKey, fn, guard, memo)
		if mapKey != v.MapKey {
			changed = true
		}
		mapValue = rewriteDepth(v.MapValue, fn, guard, memo)
		if mapValue != v.MapValue {
			changed = true
		}
	}

	if !changed {
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
	return buildRecordType(fieldsSrc, staticMembersSrc, metatable, mapKey, mapValue, v.Open, true)
}
