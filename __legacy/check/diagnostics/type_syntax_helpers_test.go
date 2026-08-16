package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func containsTypeParamSyntax(t typ.Type) bool {
	return containsTypeParamSyntaxDepth(t, nil, 0)
}

func containsTypeParamSyntaxDepth(t typ.Type, seen map[typ.Type]bool, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.TypeParam, *typ.Ref:
		return true
	}
	if seen == nil {
		seen = make(map[typ.Type]bool)
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return containsTypeParamSyntaxDepth(child, seen, depth+1)
	})
}
