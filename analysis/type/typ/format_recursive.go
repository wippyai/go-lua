package typ

import "github.com/wippyai/go-lua/analysis/internal/recursion"

func (f *formatter) formatType(t Type, depth int, guard recursion.Guard) {
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

	VisitWithGuard(t, guard, struct{}{}, func(next recursion.Guard) Visitor[struct{}] {
		return Visitor[struct{}]{
			Union: func(u *Union) struct{} {
				f.formatUnion(u, depth, next)
				return struct{}{}
			},
			Intersection: func(u *Intersection) struct{} {
				f.formatIntersection(u, depth, next)
				return struct{}{}
			},
			Optional: func(o *Optional) struct{} {
				f.formatType(o.Inner, depth+1, next)
				f.write("?")
				return struct{}{}
			},
			Array: func(a *Array) struct{} {
				f.formatType(a.Element, depth+1, next)
				f.write("[]")
				return struct{}{}
			},
			Map: func(m *Map) struct{} {
				f.write("{[")
				f.formatType(m.Key, depth+1, next)
				f.write("]: ")
				f.formatType(m.Value, depth+1, next)
				f.write("}")
				return struct{}{}
			},
			ReadonlyMap: func(m *ReadonlyMap) struct{} {
				f.write("readonly {[")
				f.formatType(m.Key, depth+1, next)
				f.write("]: ")
				f.formatType(m.Value, depth+1, next)
				f.write("}")
				return struct{}{}
			},
			Tuple: func(tu *Tuple) struct{} {
				f.formatTuple(tu, depth, next)
				return struct{}{}
			},
			Function: func(fn *Function) struct{} {
				f.formatFunction(fn, depth, next)
				return struct{}{}
			},
			Record: func(r *Record) struct{} {
				f.formatRecord(r, depth, next)
				return struct{}{}
			},
			Literal: func(l *Literal) struct{} {
				f.write(l.String())
				return struct{}{}
			},
			Ref: func(r *Ref) struct{} {
				f.write(r.Name)
				return struct{}{}
			},
			Alias: func(a *Alias) struct{} {
				if a.Name != "" {
					f.write(a.Name)
				} else if a.Target != nil {
					f.formatType(a.Target, depth+1, next)
				} else {
					f.write("alias")
				}
				return struct{}{}
			},
			Platform: func(p *Platform) struct{} {
				f.write(p.Name)
				return struct{}{}
			},
			TypeParam: func(p *TypeParam) struct{} {
				f.write(p.Name)
				if p.Constraint != nil {
					f.write(" : ")
					f.formatType(p.Constraint, depth+1, next)
				}
				return struct{}{}
			},
			Generic: func(g *Generic) struct{} {
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
			Instantiated: func(i *Instantiated) struct{} {
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
			Interface: func(i *Interface) struct{} {
				if i.Name != "" {
					f.write(i.Name)
				} else {
					f.write("interface{...}")
				}
				return struct{}{}
			},
			Recursive: func(r *Recursive) struct{} {
				f.write(r.String())
				return struct{}{}
			},
			Meta: func(m *Meta) struct{} {
				f.write("typeof(")
				f.formatType(m.Of, depth+1, next)
				f.write(")")
				return struct{}{}
			},
			Default: func(tt Type) struct{} {
				f.write(tt.String())
				return struct{}{}
			},
		}
	})
}
