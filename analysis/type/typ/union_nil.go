package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// UnionWithoutNil returns this normalized union without nil-capable members.
// It preserves the existing member hash vector for unchanged members so
// projection-style operations such as truthiness narrowing do not rehash large
// recursive union members just to remove nil from an already-normalized union.
func UnionWithoutNil(u *Union) Type {
	return ProjectUnionMembers(u, func(member Type) Type {
		if member == nil || member.Kind() == kind.Nil {
			return Never
		}
		if opt, ok := member.(*Optional); ok {
			if opt.Inner == nil || opt.Inner.Kind() == kind.Never || opt.Inner.Kind() == kind.Nil {
				return Never
			}
			return opt.Inner
		}
		return member
	})
}
