package typ

import "github.com/wippyai/go-lua/analysis/internal/recursion"

func (f *formatter) formatFunction(fn *Function, depth int, guard recursion.Guard) {
	f.write("fun")
	if len(fn.TypeParams) > 0 {
		f.write("<")
		limit := minInt(len(fn.TypeParams), f.opts.MaxTypeParams)
		for i := 0; i < limit; i++ {
			if i > 0 {
				f.write(", ")
			}
			f.formatType(fn.TypeParams[i], depth+1, guard)
		}
		if limit < len(fn.TypeParams) {
			f.write(", ...")
		}
		f.write(">")
	}
	f.write("(")
	limit := minInt(len(fn.Params), f.opts.MaxParams)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(", ")
		}
		param := fn.Params[i]
		if param.Name != "" {
			f.write(param.Name)
			f.write(": ")
		}
		f.formatType(param.Type, depth+1, guard)
		if param.Optional {
			f.write("?")
		}
	}
	if limit < len(fn.Params) {
		f.write(", ...")
	}
	if fn.Variadic != nil {
		if len(fn.Params) > 0 {
			f.write(", ")
		}
		f.write("...")
		f.formatType(fn.Variadic, depth+1, guard)
	}
	f.write(")")

	if len(fn.Returns) > 0 {
		f.write(" -> ")
		if len(fn.Returns) == 1 {
			f.formatType(fn.Returns[0], depth+1, guard)
		} else {
			f.write("(")
			limit = minInt(len(fn.Returns), f.opts.MaxReturns)
			for i := 0; i < limit; i++ {
				if i > 0 {
					f.write(", ")
				}
				f.formatType(fn.Returns[i], depth+1, guard)
			}
			if limit < len(fn.Returns) {
				f.write(", ...")
			}
			f.write(")")
		}
	}

	if !fn.Effect.Pure() {
		f.write(" ! ")
		f.write(fn.Effect.String())
	}
}
