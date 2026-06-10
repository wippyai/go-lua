package typ

func containsNeverDynamic(t Type, seen map[Type]bool) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if knownContainsNever(t) {
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
			return containsNeverDynamic(o.Inner, seen)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if containsNeverDynamic(member, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if containsNeverDynamic(member, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return containsNeverDynamic(a.Element, seen)
		},
		Map: func(m *Map) bool {
			return containsNeverDynamic(m.Key, seen) ||
				containsNeverDynamic(m.Value, seen)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return containsNeverDynamic(m.Key, seen) ||
				containsNeverDynamic(m.Value, seen)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if containsNeverDynamic(elem, seen) {
					return true
				}
			}
			return false
		},
		Function: func(fn *Function) bool {
			for _, param := range fn.Params {
				if containsNeverDynamic(param.Type, seen) {
					return true
				}
			}
			if containsNeverDynamic(fn.Variadic, seen) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsNeverDynamic(ret, seen) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && containsNeverDynamic(param.Constraint, seen) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
			if containsNeverDynamic(r.MapKey, seen) ||
				containsNeverDynamic(r.MapValue, seen) ||
				containsNeverDynamic(r.Metatable, seen) {
				return true
			}
			for _, field := range r.Fields {
				if containsNeverDynamic(field.Type, seen) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if containsNeverDynamic(member.Type, seen) {
					return true
				}
			}
			return false
		},
		Alias: func(a *Alias) bool {
			return containsNeverDynamic(a.Target, seen)
		},
		Meta: func(m *Meta) bool {
			return containsNeverDynamic(m.Of, seen)
		},
		Generic: func(g *Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && containsNeverDynamic(param.Constraint, seen) {
					return true
				}
			}
			return containsNeverDynamic(g.Body, seen)
		},
		Instantiated: func(i *Instantiated) bool {
			if i.Generic != nil && containsNeverDynamic(i.Generic, seen) {
				return true
			}
			for _, arg := range i.TypeArgs {
				if containsNeverDynamic(arg, seen) {
					return true
				}
			}
			return false
		},
		TypeParam: func(p *TypeParam) bool {
			return containsNeverDynamic(p.Constraint, seen)
		},
		Sum: func(s *Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if containsNeverDynamic(t, seen) {
						return true
					}
				}
			}
			return false
		},
		Interface: func(i *Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && containsNeverDynamic(method.Type, seen) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *Recursive) bool {
			return containsNeverDynamic(r.Body, seen)
		},
		Default: func(Type) bool {
			return false
		},
	})
}
