package subst

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expandFunction(v *typ.Function, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
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
			params[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional, Receiver: p.Receiver}
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
}
