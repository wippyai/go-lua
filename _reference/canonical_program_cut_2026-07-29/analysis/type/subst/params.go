package subst

import (
	"github.com/wippyai/go-lua/analysis/type/transform"
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
	return transform.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
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
	return transform.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
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
		params[i] = typ.Param{Name: p.Name, Type: pt, Optional: p.Optional, Receiver: p.Receiver}
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
