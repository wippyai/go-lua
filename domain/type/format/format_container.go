package format

import (
	"github.com/wippyai/go-lua/domain/type/internal/recursion"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func (f *formatter) formatTuple(t *typ.Tuple, depth int, guard recursion.Guard) {
	f.write("(")
	limit := minInt(len(t.Elements), f.opts.MaxTupleElems)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(", ")
		}
		f.formatType(t.Elements[i], depth+1, guard)
	}
	if limit < len(t.Elements) {
		f.write(", ...")
	}
	f.write(")")
}
