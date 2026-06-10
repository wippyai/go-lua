package typ

func containsAnyDynamic(t Type, seen map[Type]bool, depth int) bool {
	if t == nil || DepthExceeded(depth) {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if knownContainsAny(t) {
		return true
	}
	if _, ok := t.(*Recursive); ok {
		return false
	}
	if !knownContainsOpenRecursive(t) {
		return false
	}
	if seen[t] {
		return false
	}
	seen[t] = true

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return containsAnyDynamic(o.Inner, seen, depth+1)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if containsAnyDynamic(member, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if containsAnyDynamic(member, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return containsAnyDynamic(a.Element, seen, depth+1)
		},
		Map: func(m *Map) bool {
			return containsAnyDynamic(m.Key, seen, depth+1) ||
				containsAnyDynamic(m.Value, seen, depth+1)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return containsAnyDynamic(m.Key, seen, depth+1) ||
				containsAnyDynamic(m.Value, seen, depth+1)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if containsAnyDynamic(elem, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Function: func(fn *Function) bool {
			for _, param := range fn.Params {
				if containsAnyDynamic(param.Type, seen, depth+1) {
					return true
				}
			}
			if containsAnyDynamic(fn.Variadic, seen, depth+1) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsAnyDynamic(ret, seen, depth+1) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && containsAnyDynamic(param.Constraint, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
			if containsAnyDynamic(r.MapKey, seen, depth+1) ||
				containsAnyDynamic(r.MapValue, seen, depth+1) ||
				containsAnyDynamic(r.Metatable, seen, depth+1) {
				return true
			}
			for _, field := range r.Fields {
				if containsAnyDynamic(field.Type, seen, depth+1) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if containsAnyDynamic(member.Type, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Alias: func(a *Alias) bool {
			return containsAnyDynamic(a.Target, seen, depth+1)
		},
		Meta: func(m *Meta) bool {
			return containsAnyDynamic(m.Of, seen, depth+1)
		},
		Generic: func(g *Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && containsAnyDynamic(param.Constraint, seen, depth+1) {
					return true
				}
			}
			return containsAnyDynamic(g.Body, seen, depth+1)
		},
		Instantiated: func(i *Instantiated) bool {
			if i.Generic != nil && containsAnyDynamic(i.Generic, seen, depth+1) {
				return true
			}
			for _, arg := range i.TypeArgs {
				if containsAnyDynamic(arg, seen, depth+1) {
					return true
				}
			}
			return false
		},
		TypeParam: func(p *TypeParam) bool {
			return containsAnyDynamic(p.Constraint, seen, depth+1)
		},
		Interface: func(i *Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && containsAnyDynamic(method.Type, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *Recursive) bool {
			return containsAnyDynamic(r.Body, seen, depth+1)
		},
		Default: func(Type) bool {
			return false
		},
	})
}
