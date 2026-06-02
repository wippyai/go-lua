package typ

// containsAnyType is the canonical exact dynamic-any predicate for the type
// algebra. Immutable non-recursive types answer from construction-time flags;
// recursive graphs are traversed with cycle protection because placeholder
// bodies can be assigned after wrapper nodes have been constructed.
func containsAnyType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsAny(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsAny(t)
	}
	return containsAnyDynamic(t, make(map[Type]bool), 0)
}

func containsNeverType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsNever(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsNever(t)
	}
	return containsNeverDynamic(t, make(map[Type]bool))
}

func containsTypeParamType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsTypeParam(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsTypeParam(t)
	}
	return containsTypeParamDynamic(t, make(map[Type]bool), 0)
}

func containsInstantiatedType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsInstantiated(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsInstantiated(t)
	}
	return containsInstantiatedDynamic(t, make(map[Type]bool), 0)
}

// HasCallableSurface reports whether t is, or is transparently wrapped around,
// a function at the callable surface. It deliberately does not descend through
// data containers such as records, arrays, maps, or tuples.
func HasCallableSurface(t Type) bool {
	if result, ok := hasCallableSurfaceFast(t); ok {
		return result
	}
	return hasCallableSurface(t, make(map[Type]bool))
}

// RecordHasCallableSurface reports whether a record exposes a callable value
// directly through a field, metatable, or map value. It does not descend
// through data containers.
func RecordHasCallableSurface(r *Record) bool {
	return r != nil && r.containsCallableSurf
}

func hasCallableSurfaceFast(t Type) (bool, bool) {
	t = UnwrapAnnotated(t)
	if t == nil {
		return false, true
	}
	switch n := t.(type) {
	case *Function:
		return true, true
	case *Optional:
		return hasCallableSurfaceFast(n.Inner)
	case *Union:
		if n.memberHashes != nil || n.hash != 0 {
			return n.containsCallableSurf, true
		}
		return false, false
	case *Alias, *Intersection:
		return false, false
	default:
		return false, true
	}
}

func hasCallableSurface(t Type, seen map[Type]bool) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch n := t.(type) {
	case *Function:
		return true
	case *Optional:
		return hasCallableSurface(n.Inner, seen)
	case *Union:
		if n.memberHashes != nil || n.hash != 0 {
			return n.containsCallableSurf
		}
		for _, member := range n.Members {
			if hasCallableSurface(member, seen) {
				return true
			}
		}
		return false
	case *Intersection:
		for _, member := range n.Members {
			if hasCallableSurface(member, seen) {
				return true
			}
		}
		return false
	case *Alias:
		return hasCallableSurface(n.Target, seen)
	default:
		return false
	}
}

func knownContainsAny(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if IsAny(t) {
		return true
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsAny
	case *Optional:
		return n.containsAny
	case *Union:
		return n.containsAny
	case *Intersection:
		return n.containsAny
	case *Array:
		return n.containsAny
	case *Map:
		return n.containsAny
	case *ReadonlyMap:
		return n.containsAny
	case *Tuple:
		return n.containsAny
	case *Function:
		return n.containsAny
	case *Record:
		return n.containsAny
	case *Alias:
		return n.containsAny
	case *Meta:
		return n.containsAny
	case *Generic:
		return n.containsAny
	case *Instantiated:
		return n.containsAny
	case *TypeParam:
		return n.containsAny
	case *FieldAccess:
		return n.containsAny
	case *IndexAccess:
		return n.containsAny
	case *Sum:
		return n.containsAny
	case *Interface:
		return n.containsAny
	default:
		return false
	}
}

func knownContainsNever(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if IsNever(t) {
		return true
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsNever
	case *Optional:
		return n.containsNever
	case *Union:
		return n.containsNever
	case *Intersection:
		return n.containsNever
	case *Array:
		return n.containsNever
	case *Map:
		return n.containsNever
	case *ReadonlyMap:
		return n.containsNever
	case *Tuple:
		return n.containsNever
	case *Function:
		return n.containsNever
	case *Record:
		return n.containsNever
	case *Alias:
		return n.containsNever
	case *Meta:
		return n.containsNever
	case *Generic:
		return n.containsNever
	case *Instantiated:
		return n.containsNever
	case *TypeParam:
		return n.containsNever
	case *FieldAccess:
		return n.containsNever
	case *IndexAccess:
		return n.containsNever
	case *Sum:
		return n.containsNever
	case *Interface:
		return n.containsNever
	default:
		return false
	}
}

func knownContainsTypeParam(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *TypeParam:
		return true
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsTypeParam
	case *Optional:
		return n.containsTypeParam
	case *Union:
		return n.containsTypeParam
	case *Intersection:
		return n.containsTypeParam
	case *Array:
		return n.containsTypeParam
	case *Map:
		return n.containsTypeParam
	case *ReadonlyMap:
		return n.containsTypeParam
	case *Tuple:
		return n.containsTypeParam
	case *Function:
		return n.containsTypeParam
	case *Record:
		return n.containsTypeParam
	case *Alias:
		return n.containsTypeParam
	case *Meta:
		return n.containsTypeParam
	case *Generic:
		return n.containsTypeParam
	case *Instantiated:
		return n.containsTypeParam
	case *FieldAccess:
		return n.containsTypeParam
	case *IndexAccess:
		return n.containsTypeParam
	case *Sum:
		return n.containsTypeParam
	case *Interface:
		return n.containsTypeParam
	default:
		return false
	}
}

func knownContainsInstantiated(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Instantiated:
		return true
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsInstantiated
	case *Optional:
		return n.containsInstantiated
	case *Union:
		return n.containsInstantiated
	case *Intersection:
		return n.containsInstantiated
	case *Array:
		return n.containsInstantiated
	case *Map:
		return n.containsInstantiated
	case *ReadonlyMap:
		return n.containsInstantiated
	case *Tuple:
		return n.containsInstantiated
	case *Function:
		return n.containsInstantiated
	case *Record:
		return n.containsInstantiated
	case *Alias:
		return n.containsInstantiated
	case *Meta:
		return n.containsInstantiated
	case *Generic:
		return n.containsInstantiated
	case *TypeParam:
		return n.containsInstantiated
	case *FieldAccess:
		return n.containsInstantiated
	case *IndexAccess:
		return n.containsInstantiated
	case *Sum:
		return n.containsInstantiated
	case *Interface:
		return n.containsInstantiated
	default:
		return false
	}
}

func knownContainsRecursive(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		return true
	case *Optional:
		return n.containsRecursive
	case *Union:
		return n.containsRecursive
	case *Intersection:
		return n.containsRecursive
	case *Array:
		return n.containsRecursive
	case *Map:
		return n.containsRecursive
	case *ReadonlyMap:
		return n.containsRecursive
	case *Tuple:
		return n.containsRecursive
	case *Function:
		return n.containsRecursive
	case *Record:
		return n.containsRecursive
	case *Alias:
		return n.containsRecursive
	case *Meta:
		return n.containsRecursive
	case *Generic:
		return n.containsRecursive
	case *Instantiated:
		return n.containsRecursive
	case *TypeParam:
		return n.containsRecursive
	case *FieldAccess:
		return n.containsRecursive
	case *IndexAccess:
		return n.containsRecursive
	case *Sum:
		return n.containsRecursive
	case *Interface:
		return n.containsRecursive
	default:
		return false
	}
}

func knownContainsOpenRecursive(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsClosedFlag()
		return !n.containsFlagsClosed
	case *Optional:
		return n.containsOpenRecursive
	case *Union:
		return n.containsOpenRecursive
	case *Intersection:
		return n.containsOpenRecursive
	case *Array:
		return n.containsOpenRecursive
	case *Map:
		return n.containsOpenRecursive
	case *ReadonlyMap:
		return n.containsOpenRecursive
	case *Tuple:
		return n.containsOpenRecursive
	case *Function:
		return n.containsOpenRecursive
	case *Record:
		return n.containsOpenRecursive
	case *Alias:
		return n.containsOpenRecursive
	case *Meta:
		return n.containsOpenRecursive
	case *Generic:
		return n.containsOpenRecursive
	case *Instantiated:
		return n.containsOpenRecursive
	case *TypeParam:
		return n.containsOpenRecursive
	case *FieldAccess:
		return n.containsOpenRecursive
	case *IndexAccess:
		return n.containsOpenRecursive
	case *Sum:
		return n.containsOpenRecursive
	case *Interface:
		return n.containsOpenRecursive
	default:
		return false
	}
}

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

func knownAny(types ...Type) bool {
	for _, t := range types {
		if knownContainsAny(t) {
			return true
		}
	}
	return false
}

func knownNever(types ...Type) bool {
	for _, t := range types {
		if knownContainsNever(t) {
			return true
		}
	}
	return false
}

func knownTypeParam(types ...Type) bool {
	for _, t := range types {
		if knownContainsTypeParam(t) {
			return true
		}
	}
	return false
}

func knownInstantiated(types ...Type) bool {
	for _, t := range types {
		if knownContainsInstantiated(t) {
			return true
		}
	}
	return false
}

func knownRecursive(types ...Type) bool {
	for _, t := range types {
		if knownContainsRecursive(t) {
			return true
		}
	}
	return false
}

func knownOpenRecursive(types ...Type) bool {
	for _, t := range types {
		if knownContainsOpenRecursive(t) {
			return true
		}
	}
	return false
}

func knownAnyParams(params []Param) bool {
	for _, p := range params {
		if knownContainsAny(p.Type) {
			return true
		}
	}
	return false
}

func knownNeverParams(params []Param) bool {
	for _, p := range params {
		if knownContainsNever(p.Type) {
			return true
		}
	}
	return false
}

func knownTypeParamParams(params []Param) bool {
	for _, p := range params {
		if knownContainsTypeParam(p.Type) {
			return true
		}
	}
	return false
}

func knownInstantiatedParams(params []Param) bool {
	for _, p := range params {
		if knownContainsInstantiated(p.Type) {
			return true
		}
	}
	return false
}

func knownRecursiveParams(params []Param) bool {
	for _, p := range params {
		if knownContainsRecursive(p.Type) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveParams(params []Param) bool {
	for _, p := range params {
		if knownContainsOpenRecursive(p.Type) {
			return true
		}
	}
	return false
}

func knownAnyFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsAny(f.Type) {
			return true
		}
	}
	return false
}

func knownNeverFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsNever(f.Type) {
			return true
		}
	}
	return false
}

func knownTypeParamFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsTypeParam(f.Type) {
			return true
		}
	}
	return false
}

func knownInstantiatedFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsInstantiated(f.Type) {
			return true
		}
	}
	return false
}

func knownRecursiveFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsRecursive(f.Type) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsOpenRecursive(f.Type) {
			return true
		}
	}
	return false
}

func knownAnyTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsAny(p.Constraint) {
			return true
		}
	}
	return false
}

func knownNeverTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsNever(p.Constraint) {
			return true
		}
	}
	return false
}

func knownTypeParamTypeParams(params []*TypeParam) bool {
	return len(params) > 0
}

func knownInstantiatedTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsInstantiated(p.Constraint) {
			return true
		}
	}
	return false
}

func knownRecursiveTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsRecursive(p.Constraint) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsOpenRecursive(p.Constraint) {
			return true
		}
	}
	return false
}
