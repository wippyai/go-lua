package typ

import "github.com/wippyai/go-lua/analysis/internal/recursion"

func rewriteFunction(v *Function, orig Type, fn func(Type) (Type, bool), guard recursion.Guard, memo map[rewriteKey]Type) Type {
	changed := false

	var params []Param
	for i, p := range v.Params {
		newType := rewriteDepth(p.Type, fn, guard, memo)
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
		newRet := rewriteDepth(r, fn, guard, memo)
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
		variadic = rewriteDepth(v.Variadic, fn, guard, memo)
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
	returnsSrc := v.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return buildFunctionType(
		v.TypeParams,
		paramSrc,
		variadic,
		returnsSrc,
		v.Effects,
		v.Spec,
		v.Refinement,
	)
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
	return buildRecordType(fieldsSrc, staticMembersSrc, metatable, mapKey, mapValue, v.Open, true, false)
}
