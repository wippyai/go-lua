package relation

import . "github.com/wippyai/go-lua/analysis/type/typ"

// NormalizeUnionForJoin applies the union normalization policy explicitly
// requested by relation and join code.
//
// typ.NewUnion still owns the legacy constructor behavior today. Keeping the
// request behind this relation helper makes the join policy boundary explicit
// without changing the underlying representation semantics.
func NormalizeUnionForJoin(members ...Type) Type {
	return NewUnion(members...)
}
