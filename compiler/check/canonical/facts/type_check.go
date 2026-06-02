package facts

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/check/domain/guard"
)

// collectTypeCheckBinds derives immutable value-narrowing binds for assignments
// of the form `local value, err = T:is(x)`, indexed by owning FuncRef.
func collectTypeCheckBinds(p Program) []typeCheckBindRow {
	if p.TypeByName == nil {
		return nil
	}
	var out []typeCheckBindRow
	for _, r := range p.Refs {
		g := graphOf(p, r)
		if g == nil {
			continue
		}
		for _, bind := range guard.TypeCheckBinds(g, p.TypeByName) {
			out = append(out, typeCheckBindRow{FuncRef: r, Bind: bind})
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareTypeCheckBindEntry)
	return compactTypeCheckBindEntries(out)
}
