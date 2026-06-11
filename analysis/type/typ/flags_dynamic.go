package typ

func containsAnyDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsAny)
}

func containsNeverDynamic(t Type, seen map[Type]bool) bool {
	return containsDynamicFlag(t, seen, 0, -1, knownContainsNever)
}

func containsTypeParamDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsTypeParam)
}

func containsInstantiatedDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsInstantiated)
}

func containsDynamicFlag(
	t Type,
	seen map[Type]bool,
	depth int,
	maxDepth int,
	known func(Type) bool,
) bool {
	if t == nil || known == nil || (maxDepth >= 0 && depth > maxDepth) {
		return false
	}
	t = unwrapAnnotated(t)
	if t == nil {
		return false
	}
	if known(t) {
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

	next := func(child Type) bool {
		return containsDynamicFlag(child, seen, depth+1, maxDepth, known)
	}

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return next(o.Inner)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if next(member) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if next(member) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return next(a.Element)
		},
		Map: func(m *Map) bool {
			return next(m.Key) || next(m.Value)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return next(m.Key) || next(m.Value)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if next(elem) {
					return true
				}
			}
			return false
		},
		Function: func(fn *Function) bool {
			for _, param := range fn.Params {
				if next(param.Type) {
					return true
				}
			}
			if next(fn.Variadic) {
				return true
			}
			for _, ret := range fn.Returns {
				if next(ret) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && next(param.Constraint) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
			if next(r.MapKey) || next(r.MapValue) || next(r.Metatable) {
				return true
			}
			for _, field := range r.Fields {
				if next(field.Type) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if next(member.Type) {
					return true
				}
			}
			return false
		},
		Alias: func(a *Alias) bool {
			return next(a.Target)
		},
		Meta: func(m *Meta) bool {
			return next(m.Of)
		},
		Generic: func(g *Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && next(param.Constraint) {
					return true
				}
			}
			return next(g.Body)
		},
		Instantiated: func(i *Instantiated) bool {
			if i.Generic != nil && next(i.Generic) {
				return true
			}
			for _, arg := range i.TypeArgs {
				if next(arg) {
					return true
				}
			}
			return false
		},
		TypeParam: func(p *TypeParam) bool {
			return next(p.Constraint)
		},
		Interface: func(i *Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && next(method.Type) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *Recursive) bool {
			return next(r.Body)
		},
		Default: func(Type) bool {
			return false
		},
	})
}
