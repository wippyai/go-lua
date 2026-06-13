package discriminant

import (
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expandInstantiated(t typ.Type) (typ.Type, bool) {
	expanded := subst.ExpandInstantiated(t)
	if expanded == nil || expanded == t {
		return nil, false
	}
	return expanded, true
}
