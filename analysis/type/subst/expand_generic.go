package subst

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expandInstantiatedGeneric(v *typ.Instantiated, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type) typ.Type {
	if v.Generic == nil || len(v.TypeArgs) != len(v.Generic.TypeParams) || v.Generic.Body == nil {
		return orig
	}
	body := Params(v.Generic.Body, v.Generic.TypeParams, v.TypeArgs)
	body = Self(body, orig)
	return expandInstantiatedGuardMode(body, guard, memo, expandModeTablePolicy)
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
