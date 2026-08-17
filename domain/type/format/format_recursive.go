package format

import (
	"github.com/wippyai/go-lua/domain/type/inspect"
	"github.com/wippyai/go-lua/domain/type/internal/recursion"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func (f *formatter) formatType(t typ.Type, depth int, guard recursion.Guard) {
	if f.truncated {
		return
	}
	if t == nil {
		f.write("nil")
		return
	}
	if f.opts.MaxDepth > 0 && depth > f.opts.MaxDepth {
		f.write("...")
		return
	}
	f.nodes++
	if f.opts.MaxNodes > 0 && f.nodes > f.opts.MaxNodes {
		f.write("...")
		f.truncated = true
		return
	}

	visitWithGuard(t, guard, struct{}{}, func(next recursion.Guard) inspect.Visitor[struct{}] {
		return inspect.Visitor[struct{}]{
			Union: func(u *typ.Union) struct{} {
				f.formatUnion(u, depth, next)
				return struct{}{}
			},
			Intersection: func(u *typ.Intersection) struct{} {
				f.formatIntersection(u, depth, next)
				return struct{}{}
			},
			Optional: func(o *typ.Optional) struct{} {
				f.formatType(o.Inner, depth+1, next)
				f.write("?")
				return struct{}{}
			},
			Array: func(a *typ.Array) struct{} {
				f.formatType(a.Element, depth+1, next)
				f.write("[]")
				return struct{}{}
			},
			Map: func(m *typ.Map) struct{} {
				f.write("{[")
				f.formatType(m.Key, depth+1, next)
				f.write("]: ")
				f.formatType(m.Value, depth+1, next)
				f.write("}")
				return struct{}{}
			},
			ReadonlyMap: func(m *typ.ReadonlyMap) struct{} {
				f.write("readonly {[")
				f.formatType(m.Key, depth+1, next)
				f.write("]: ")
				f.formatType(m.Value, depth+1, next)
				f.write("}")
				return struct{}{}
			},
			Tuple: func(tu *typ.Tuple) struct{} {
				f.formatTuple(tu, depth, next)
				return struct{}{}
			},
			Function: func(fn *typ.Function) struct{} {
				f.formatFunction(fn, depth, next)
				return struct{}{}
			},
			Record: func(r *typ.Record) struct{} {
				f.formatRecord(r, depth, next)
				return struct{}{}
			},
			Literal: func(l *typ.Literal) struct{} {
				f.write(l.String())
				return struct{}{}
			},
			Ref: func(r *typ.Ref) struct{} {
				f.write(r.Name)
				return struct{}{}
			},
			Alias: func(a *typ.Alias) struct{} {
				if a.Name != "" {
					f.write(a.Name)
				} else if a.Target != nil {
					f.formatType(a.Target, depth+1, next)
				} else {
					f.write("alias")
				}
				return struct{}{}
			},
			TypeParam: func(p *typ.TypeParam) struct{} {
				f.write(p.Name)
				if p.Constraint != nil {
					f.write(" : ")
					f.formatType(p.Constraint, depth+1, next)
				}
				return struct{}{}
			},
			Generic: func(g *typ.Generic) struct{} {
				f.write(g.Name)
				f.write("<")
				limit := minInt(len(g.TypeParams), f.opts.MaxTypeParams)
				for i := 0; i < limit; i++ {
					if i > 0 {
						f.write(", ")
					}
					f.formatType(g.TypeParams[i], depth+1, next)
				}
				if limit < len(g.TypeParams) {
					f.write(", ...")
				}
				f.write(">")
				return struct{}{}
			},
			Instantiated: func(i *typ.Instantiated) struct{} {
				if i.Generic != nil && i.Generic.Name != "" {
					f.write(i.Generic.Name)
				} else {
					f.write("inst")
				}
				f.write("<")
				limit := minInt(len(i.TypeArgs), f.opts.MaxTypeParams)
				for idx := 0; idx < limit; idx++ {
					if idx > 0 {
						f.write(", ")
					}
					f.formatType(i.TypeArgs[idx], depth+1, next)
				}
				if limit < len(i.TypeArgs) {
					f.write(", ...")
				}
				f.write(">")
				return struct{}{}
			},
			Interface: func(i *typ.Interface) struct{} {
				if i.Name != "" {
					f.write(i.Name)
				} else {
					f.write("interface{...}")
				}
				return struct{}{}
			},
			Recursive: func(r *typ.Recursive) struct{} {
				f.write(r.String())
				return struct{}{}
			},
			Meta: func(m *typ.Meta) struct{} {
				f.write("typeof(")
				f.formatType(m.Of, depth+1, next)
				f.write(")")
				return struct{}{}
			},
			Default: func(tt typ.Type) struct{} {
				f.write(tt.String())
				return struct{}{}
			},
		}
	})
}

func visitWithGuard[R any](
	t typ.Type,
	guard recursion.Guard,
	onCycle R,
	build func(next recursion.Guard) inspect.Visitor[R],
) R {
	if t == nil {
		return onCycle
	}
	next, ok := guard.Enter()
	if !ok {
		return onCycle
	}
	return inspect.Visit(t, build(next))
}
