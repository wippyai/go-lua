package transform

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func rewriteFunction(v *typ.Function, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	changed := false

	var typeParams []*typ.TypeParam
	var typeParamReplacements map[*typ.TypeParam]*typ.TypeParam
	for i, p := range v.TypeParams {
		newParamType := rewriteDepth(p, fn, guard, depth, memo)
		newParam, ok := newParamType.(*typ.TypeParam)
		if !ok || newParam == p {
			if typeParams != nil {
				typeParams[i] = p
			}
			continue
		}
		if typeParams == nil {
			typeParams = make([]*typ.TypeParam, len(v.TypeParams))
			copy(typeParams, v.TypeParams)
		}
		changed = true
		typeParams[i] = newParam
		if typeParamReplacements == nil {
			typeParamReplacements = make(map[*typ.TypeParam]*typ.TypeParam)
		}
		typeParamReplacements[p] = newParam
	}
	rewriteChild := rewriteFnWithTypeParamReplacements(fn, typeParamReplacements)

	var params []typ.Param
	for i, p := range v.Params {
		newType := rewriteDepth(p.Type, rewriteChild, guard, depth, memo)
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
		newRet := rewriteDepth(r, rewriteChild, guard, depth, memo)
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

	var variadic typ.Type
	if v.Variadic != nil {
		variadic = rewriteDepth(v.Variadic, rewriteChild, guard, depth, memo)
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
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: typeParamSrc,
		Params:     paramSrc,
		Variadic:   variadic,
		Returns:    returnsSrc,
	})
}

func rewriteMeta(v *typ.Meta, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	of := rewriteDepth(v.Of, fn, guard, depth, memo)
	if of == v.Of {
		return orig
	}
	return typ.NewMeta(of)
}

func rewriteTypeParam(v *typ.TypeParam, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	constraint := rewriteDepth(v.Constraint, fn, guard, depth, memo)
	if constraint == v.Constraint {
		return orig
	}
	return typ.NewTypeParam(v.Name, constraint)
}

func rewriteGeneric(v *typ.Generic, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	changed := false

	var typeParams []*typ.TypeParam
	var typeParamReplacements map[*typ.TypeParam]*typ.TypeParam
	for i, p := range v.TypeParams {
		newParamType := rewriteDepth(p, fn, guard, depth, memo)
		newParam, ok := newParamType.(*typ.TypeParam)
		if !ok || newParam == p {
			if typeParams != nil {
				typeParams[i] = p
			}
			continue
		}
		if typeParams == nil {
			typeParams = make([]*typ.TypeParam, len(v.TypeParams))
			copy(typeParams, v.TypeParams)
		}
		changed = true
		typeParams[i] = newParam
		if typeParamReplacements == nil {
			typeParamReplacements = make(map[*typ.TypeParam]*typ.TypeParam)
		}
		typeParamReplacements[p] = newParam
	}

	body := rewriteDepth(v.Body, rewriteFnWithTypeParamReplacements(fn, typeParamReplacements), guard, depth, memo)
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
	return typ.NewGeneric(v.Name, typeParamSrc, body)
}

func rewriteFnWithTypeParamReplacements(fn func(typ.Type) (typ.Type, bool), replacements map[*typ.TypeParam]*typ.TypeParam) func(typ.Type) (typ.Type, bool) {
	if len(replacements) == 0 {
		return fn
	}
	return func(t typ.Type) (typ.Type, bool) {
		if tp, ok := t.(*typ.TypeParam); ok {
			if replacement, ok := replacements[tp]; ok {
				return replacement, true
			}
		}
		return fn(t)
	}
}

func rewriteRecursive(v *typ.Recursive, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	if v.Body == nil {
		return orig
	}
	var replacement *typ.Recursive
	replacementNode := func() *typ.Recursive {
		if replacement == nil {
			replacement = typ.NewRecursivePlaceholder(v.Name)
		}
		return replacement
	}
	selfAware := func(t typ.Type) (typ.Type, bool) {
		if typ.IsRecursiveRef(t, v) {
			return replacementNode(), true
		}
		return fn(t)
	}
	body := rewriteDepth(v.Body, selfAware, guard, depth, memo)
	if body == v.Body {
		return orig
	}
	replacement = replacementNode()
	replacement.SetBody(body)
	return replacement
}

func rewriteRecord(v *typ.Record, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	changed := false

	var fields []typ.Field
	for i, f := range v.Fields {
		newType := rewriteDepth(f.Type, fn, guard, depth, memo)
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
		newType := rewriteDepth(m.Type, fn, guard, depth, memo)
		if newType != m.Type {
			if staticMembers == nil {
				staticMembers = make([]typ.StaticMember, len(v.StaticMembers))
				copy(staticMembers, v.StaticMembers)
			}
			changed = true
			staticMembers[i].Type = newType
		} else if staticMembers != nil {
			staticMembers[i] = m
		}
	}

	var metatable typ.Type
	if v.Metatable != nil {
		metatable = rewriteDepth(v.Metatable, fn, guard, depth, memo)
		if metatable != v.Metatable {
			changed = true
		}
	}

	mapKey := v.MapKey
	mapValue := v.MapValue
	if v.HasMapComponent() {
		mapKey = rewriteDepth(v.MapKey, fn, guard, depth, memo)
		if mapKey != v.MapKey {
			changed = true
		}
		mapValue = rewriteDepth(v.MapValue, fn, guard, depth, memo)
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
	return typetable.RebuildRecord(typ.RecordParts{
		Fields:        fieldsSrc,
		StaticMembers: staticMembersSrc,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          v.Open,
		AssumeSorted:  true,
	})
}
