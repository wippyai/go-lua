package format

import (
	"github.com/wippyai/go-lua/domain/type/internal/recursion"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func (f *formatter) formatUnion(u *typ.Union, depth int, guard recursion.Guard) {
	f.formatMembers(u.Members, " | ", depth, guard)
}

func (f *formatter) formatIntersection(u *typ.Intersection, depth int, guard recursion.Guard) {
	f.formatMembers(u.Members, " & ", depth, guard)
}

// formatMembers writes members joined by sep, truncating beyond MaxUnionMembers
// with a trailing sep+"...".
func (f *formatter) formatMembers(members []typ.Type, sep string, depth int, guard recursion.Guard) {
	limit := minInt(len(members), f.opts.MaxUnionMembers)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(sep)
		}
		f.formatType(members[i], depth+1, guard)
	}
	if limit < len(members) {
		f.write(sep + "...")
	}
}
