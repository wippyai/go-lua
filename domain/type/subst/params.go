package subst

import (
	"github.com/wippyai/go-lua/domain/type/transform"
	"github.com/wippyai/go-lua/domain/type/typ"
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
	return substituteParamsIterative(t, subs)
}

func substituteFunctionParams(fn *typ.Function, subs []paramSubstitution) typ.Type {
	return substituteParamsIterative(fn, subs)
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
	// An anonymous formal has no lexical name, so name shadowing does not
	// apply to it; it shadows only itself or a formal it is equal to.
	return binder == sub.param ||
		(binder.Name != "" && binder.Name == sub.param.Name) ||
		binder.Equals(sub.param)
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
