package typ

import "github.com/wippyai/go-lua/analysis/internal/recursion"

func (f *formatter) formatUnion(u *Union, depth int, guard recursion.Guard) {
	limit := minInt(len(u.Members), f.opts.MaxUnionMembers)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(" | ")
		}
		f.formatType(u.Members[i], depth+1, guard)
	}
	if limit < len(u.Members) {
		f.write(" | ...")
	}
}

func (f *formatter) formatIntersection(u *Intersection, depth int, guard recursion.Guard) {
	limit := minInt(len(u.Members), f.opts.MaxUnionMembers)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(" & ")
		}
		f.formatType(u.Members[i], depth+1, guard)
	}
	if limit < len(u.Members) {
		f.write(" & ...")
	}
}
