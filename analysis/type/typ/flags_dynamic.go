package typ

func containsInstantiatedDynamic(t Type, seen map[Type]bool, depth int) bool {
	if t == nil || DepthExceeded(depth) {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if knownContainsInstantiated(t) {
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
			return containsInstantiatedDynamic(o.Inner, seen, depth+1)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if containsInstantiatedDynamic(member, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if containsInstantiatedDynamic(member, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return containsInstantiatedDynamic(a.Element, seen, depth+1)
		},
		Map: func(m *Map) bool {
			return containsInstantiatedDynamic(m.Key, seen, depth+1) ||
				containsInstantiatedDynamic(m.Value, seen, depth+1)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return containsInstantiatedDynamic(m.Key, seen, depth+1) ||
				containsInstantiatedDynamic(m.Value, seen, depth+1)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if containsInstantiatedDynamic(elem, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Function: func(fn *Function) bool {
			for _, param := range fn.Params {
				if containsInstantiatedDynamic(param.Type, seen, depth+1) {
					return true
				}
			}
			if containsInstantiatedDynamic(fn.Variadic, seen, depth+1) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsInstantiatedDynamic(ret, seen, depth+1) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && containsInstantiatedDynamic(param.Constraint, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
			if containsInstantiatedDynamic(r.MapKey, seen, depth+1) ||
				containsInstantiatedDynamic(r.MapValue, seen, depth+1) ||
				containsInstantiatedDynamic(r.Metatable, seen, depth+1) {
				return true
			}
			for _, field := range r.Fields {
				if containsInstantiatedDynamic(field.Type, seen, depth+1) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if containsInstantiatedDynamic(member.Type, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Alias: func(a *Alias) bool {
			return containsInstantiatedDynamic(a.Target, seen, depth+1)
		},
		Meta: func(m *Meta) bool {
			return containsInstantiatedDynamic(m.Of, seen, depth+1)
		},
		Generic: func(g *Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && containsInstantiatedDynamic(param.Constraint, seen, depth+1) {
					return true
				}
			}
			return containsInstantiatedDynamic(g.Body, seen, depth+1)
		},
		TypeParam: func(p *TypeParam) bool {
			return containsInstantiatedDynamic(p.Constraint, seen, depth+1)
		},
		FieldAccess: func(f *FieldAccess) bool {
			return containsInstantiatedDynamic(f.Base, seen, depth+1)
		},
		IndexAccess: func(i *IndexAccess) bool {
			return containsInstantiatedDynamic(i.Base, seen, depth+1) ||
				containsInstantiatedDynamic(i.Index, seen, depth+1)
		},
		Sum: func(s *Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if containsInstantiatedDynamic(t, seen, depth+1) {
						return true
					}
				}
			}
			return false
		},
		Interface: func(i *Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && containsInstantiatedDynamic(method.Type, seen, depth+1) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *Recursive) bool {
			return containsInstantiatedDynamic(r.Body, seen, depth+1)
		},
		Default: func(Type) bool {
			return false
		},
	})
}

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
		FieldAccess: func(f *FieldAccess) bool {
			return containsNeverDynamic(f.Base, seen)
		},
		IndexAccess: func(i *IndexAccess) bool {
			return containsNeverDynamic(i.Base, seen) ||
				containsNeverDynamic(i.Index, seen)
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
		FieldAccess: func(f *FieldAccess) bool {
			return containsTypeParamDynamic(f.Base, seen, depth+1)
		},
		IndexAccess: func(i *IndexAccess) bool {
			return containsTypeParamDynamic(i.Base, seen, depth+1) ||
				containsTypeParamDynamic(i.Index, seen, depth+1)
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
		FieldAccess: func(f *FieldAccess) bool {
			return containsAnyDynamic(f.Base, seen, depth+1)
		},
		IndexAccess: func(i *IndexAccess) bool {
			return containsAnyDynamic(i.Base, seen, depth+1) ||
				containsAnyDynamic(i.Index, seen, depth+1)
		},
		Sum: func(s *Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if containsAnyDynamic(t, seen, depth+1) {
						return true
					}
				}
			}
			return false
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
