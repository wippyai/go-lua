package relation

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizeUnionForJoin applies the union normalization policy explicitly
// requested by relation and join code.
func NormalizeUnionForJoin(members ...Type) Type {
	return normalize.UnionForJoin(members...)
}

// NormalizeUnionForProjection applies the union policy for projected values,
// such as Lua field and callable return projections.
func NormalizeUnionForProjection(members ...Type) Type {
	return normalize.UnionForProjection(members...)
}
