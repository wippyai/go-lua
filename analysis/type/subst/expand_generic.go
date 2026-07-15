package subst

import (
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expandInstantiatedGeneric(v *typ.Instantiated, orig typ.Type, state *expandState) typ.Type {
	if v.Generic == nil || len(v.TypeArgs) != len(v.Generic.TypeParams) || v.Generic.Body == nil {
		return orig
	}
	if active := state.matchingActive(v); active != nil {
		active.used = true
		return active.mu
	}
	// A recursive generic that changes its arguments describes a generative
	// (non-regular) family, so no finite ordinary μ graph can represent its
	// infinite unfolding. Preserve the exact symbolic instantiation at that
	// recurrence boundary instead of approximating or unfolding forever.
	if state.hasActiveGeneric(v.Generic) {
		return orig
	}
	active := &activeInstantiation{
		generic: v.Generic,
		args:    append([]typ.Type(nil), v.TypeArgs...),
		mu:      typ.NewRecursivePlaceholder(v.Generic.Name),
	}
	state.active = append(state.active, active)
	defer func() { state.active = state.active[:len(state.active)-1] }()

	body := Params(v.Generic.Body, v.Generic.TypeParams, v.TypeArgs)
	body = Self(body, orig)
	body = expandInstantiatedGuardMode(body, state, expandModeTablePolicy)
	if !active.used {
		return body
	}
	active.mu.SetBody(body)
	return active.mu
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
