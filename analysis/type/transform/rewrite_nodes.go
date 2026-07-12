package transform

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func rewriteFunction(v *typ.Function, orig typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	typeParams, typeParamReplacements, changed := rewriteTypeParamList(v.TypeParams, fn, guard, depth, memo)
	rewriteChild := rewriteFnWithTypeParamScope(fn, v.TypeParams, typeParamReplacements)
	childMemo := rewriteMemoForTypeParamScope(memo, v.TypeParams, typeParamReplacements)

	var params []typ.Param
	for i, p := range v.Params {
		newType := rewriteDepth(p.Type, rewriteChild, guard, depth, childMemo)
		if newType != p.Type {
			if params == nil {
				params = make([]typ.Param, len(v.Params))
				copy(params, v.Params)
			}
			changed = true
			params[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional, Receiver: p.Receiver}
		} else if params != nil {
			params[i] = p
		}
	}

	var returns []typ.Type
	for i, r := range v.Returns {
		newRet := rewriteDepth(r, rewriteChild, guard, depth, childMemo)
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
		variadic = rewriteDepth(v.Variadic, rewriteChild, guard, depth, childMemo)
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
	typeParams, typeParamReplacements, changed := rewriteTypeParamList(v.TypeParams, fn, guard, depth, memo)

	body := rewriteDepth(
		v.Body,
		rewriteFnWithTypeParamScope(fn, v.TypeParams, typeParamReplacements),
		guard,
		depth,
		rewriteMemoForTypeParamScope(memo, v.TypeParams, typeParamReplacements),
	)
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

func rewriteTypeParamList(params []*typ.TypeParam, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) ([]*typ.TypeParam, map[*typ.TypeParam]*typ.TypeParam, bool) {
	var out []*typ.TypeParam
	var replacements map[*typ.TypeParam]*typ.TypeParam
	changed := false
	for i, p := range params {
		newParam := rewriteTypeParamBinder(p, fn, guard, depth, memo)
		if newParam == p {
			if out != nil {
				out[i] = p
			}
			continue
		}
		if out == nil {
			out = make([]*typ.TypeParam, len(params))
			copy(out, params)
		}
		changed = true
		out[i] = newParam
		if replacements == nil {
			replacements = make(map[*typ.TypeParam]*typ.TypeParam)
		}
		replacements[p] = newParam
	}
	return out, replacements, changed
}

func rewriteTypeParamBinder(p *typ.TypeParam, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) *typ.TypeParam {
	if p == nil {
		return p
	}
	if replacement, ok := fn(p); ok {
		if newParam, ok := replacement.(*typ.TypeParam); ok {
			return newParam
		}
	}
	constraint := rewriteDepth(p.Constraint, fn, guard, depth, memo)
	if constraint == p.Constraint {
		return p
	}
	return typ.NewTypeParam(p.Name, constraint)
}

func rewriteFnWithTypeParamScope(fn func(typ.Type) (typ.Type, bool), binders []*typ.TypeParam, replacements map[*typ.TypeParam]*typ.TypeParam) func(typ.Type) (typ.Type, bool) {
	if len(replacements) == 0 && len(binders) == 0 {
		return fn
	}
	return func(t typ.Type) (typ.Type, bool) {
		if tp, ok := t.(*typ.TypeParam); ok {
			if replacement, ok := replacements[tp]; ok {
				return replacement, true
			}
			if typeParamShadowsBinder(tp, binders) {
				return nil, false
			}
		}
		return fn(t)
	}
}

func typeParamShadowsBinder(tp *typ.TypeParam, binders []*typ.TypeParam) bool {
	if tp == nil {
		return false
	}
	for _, binder := range binders {
		if binder == nil {
			continue
		}
		if tp == binder || tp.Name == binder.Name || tp.Equals(binder) {
			return true
		}
	}
	return false
}

func rewriteMemoForTypeParamScope(memo map[rewriteKey]typ.Type, binders []*typ.TypeParam, replacements map[*typ.TypeParam]*typ.TypeParam) map[rewriteKey]typ.Type {
	if len(binders) == 0 && len(replacements) == 0 {
		return memo
	}
	return nil
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

	fields, fieldsChanged := rewriteRecordFields(v.Fields, fn, guard, depth, memo)
	if fieldsChanged {
		changed = true
	}
	staticMembers, staticMembersChanged := rewriteRecordStaticMembers(v.StaticMembers, fn, guard, depth, memo)
	if staticMembersChanged {
		changed = true
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

func rewriteRecordFields(fields []typ.Field, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) ([]typ.Field, bool) {
	var out []typ.Field
	for i, f := range fields {
		newType := rewriteDepth(f.Type, fn, guard, depth, memo)
		if newType == f.Type {
			if out != nil {
				out[i] = f
			}
			continue
		}
		if out == nil {
			out = make([]typ.Field, len(fields))
			copy(out, fields)
		}
		out[i] = typ.Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
	}
	return out, out != nil
}

func rewriteRecordStaticMembers(members []typ.StaticMember, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) ([]typ.StaticMember, bool) {
	var out []typ.StaticMember
	for i, m := range members {
		newType := rewriteDepth(m.Type, fn, guard, depth, memo)
		if newType == m.Type {
			if out != nil {
				out[i] = m
			}
			continue
		}
		if out == nil {
			out = make([]typ.StaticMember, len(members))
			copy(out, members)
		}
		out[i].Type = newType
	}
	return out, out != nil
}
