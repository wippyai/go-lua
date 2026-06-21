package typ

// walkChildren visits the direct child type slots of t in stable canonical order.
// It returns true when visit reports a match and traversal should stop.
func walkChildren(t Type, visit func(Type) bool) bool {
	return WalkChildren(t, visit)
}

// WalkChildren visits the canonical child type slots of t in stable order.
// It returns true when visit reports a match and traversal should stop.
func WalkChildren(t Type, visit func(Type) bool) bool {
	if visit == nil || t == nil {
		return false
	}
	visitType := func(child Type) bool {
		return child != nil && visit(child)
	}
	t = UnwrapTransparentWrappers(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Optional:
		return visitType(n.Inner)
	case *Union:
		return walkEachType(n.Members, visitType)
	case *Intersection:
		return walkEachType(n.Members, visitType)
	case *Array:
		return visitType(n.Element)
	case *Map:
		return visitType(n.Key) || visitType(n.Value)
	case *ReadonlyMap:
		return visitType(n.Key) || visitType(n.Value)
	case *Tuple:
		return walkEachType(n.Elements, visitType)
	case *Function:
		for _, param := range n.TypeParams {
			if param != nil && visitType(param.Constraint) {
				return true
			}
		}
		for _, param := range n.Params {
			if visitType(param.Type) {
				return true
			}
		}
		if visitType(n.Variadic) {
			return true
		}
		for _, ret := range n.Returns {
			if visitType(ret) {
				return true
			}
		}
	case *Record:
		for _, field := range n.Fields {
			if visitType(field.Type) {
				return true
			}
		}
		for _, member := range n.StaticMembers {
			if visitType(member.Type) {
				return true
			}
		}
		if visitType(n.Metatable) {
			return true
		}
		if n.HasMapComponent() {
			if visitType(n.MapKey) || visitType(n.MapValue) {
				return true
			}
		}
	case *Alias:
		return visitType(n.Target)
	case *Meta:
		return visitType(n.Of)
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && visitType(param.Constraint) {
				return true
			}
		}
		return visitType(n.Body)
	case *Instantiated:
		if visitType(n.Generic) {
			return true
		}
		return walkEachType(n.TypeArgs, visitType)
	case *TypeParam:
		return visitType(n.Constraint)
	case *Interface:
		for _, method := range n.Methods {
			if visitType(method.Type) {
				return true
			}
		}
	case *Recursive:
		return visitType(n.Body)
	}
	return false
}

func walkEachType(types []Type, visit func(Type) bool) bool {
	for _, t := range types {
		if visit(t) {
			return true
		}
	}
	return false
}
