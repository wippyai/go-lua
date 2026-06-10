package typ

func containsTypeParamDynamic(t Type, seen map[Type]bool, depth int) bool {
	if t == nil || DepthExceeded(depth) {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if knownContainsTypeParam(t) {
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
			return containsTypeParamDynamic(o.Inner, seen, depth+1)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if containsTypeParamDynamic(member, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if containsTypeParamDynamic(member, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return containsTypeParamDynamic(a.Element, seen, depth+1)
		},
		Map: func(m *Map) bool {
			return containsTypeParamDynamic(m.Key, seen, depth+1) ||
				containsTypeParamDynamic(m.Value, seen, depth+1)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return containsTypeParamDynamic(m.Key, seen, depth+1) ||
				containsTypeParamDynamic(m.Value, seen, depth+1)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if containsTypeParamDynamic(elem, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Function: func(fn *Function) bool {
			for _, param := range fn.Params {
				if containsTypeParamDynamic(param.Type, seen, depth+1) {
					return true
				}
			}
			if containsTypeParamDynamic(fn.Variadic, seen, depth+1) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsTypeParamDynamic(ret, seen, depth+1) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && containsTypeParamDynamic(param.Constraint, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
			if containsTypeParamDynamic(r.MapKey, seen, depth+1) ||
				containsTypeParamDynamic(r.MapValue, seen, depth+1) ||
				containsTypeParamDynamic(r.Metatable, seen, depth+1) {
				return true
			}
			for _, field := range r.Fields {
				if containsTypeParamDynamic(field.Type, seen, depth+1) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if containsTypeParamDynamic(member.Type, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Alias: func(a *Alias) bool {
			return containsTypeParamDynamic(a.Target, seen, depth+1)
		},
		Meta: func(m *Meta) bool {
			return containsTypeParamDynamic(m.Of, seen, depth+1)
		},
		Generic: func(g *Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && containsTypeParamDynamic(param.Constraint, seen, depth+1) {
					return true
				}
			}
			return containsTypeParamDynamic(g.Body, seen, depth+1)
		},
		Instantiated: func(i *Instantiated) bool {
			if i.Generic != nil && containsTypeParamDynamic(i.Generic, seen, depth+1) {
				return true
			}
			for _, arg := range i.TypeArgs {
				if containsTypeParamDynamic(arg, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Sum: func(s *Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if containsTypeParamDynamic(t, seen, depth+1) {
						return true
					}
				}
			}
			return false
		},
		Interface: func(i *Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && containsTypeParamDynamic(method.Type, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *Recursive) bool {
			return containsTypeParamDynamic(r.Body, seen, depth+1)
		},
		Default: func(Type) bool {
			return false
		},
	})
}
