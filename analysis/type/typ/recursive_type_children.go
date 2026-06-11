package typ

func recursiveTypeChildrenAll(t Type, visit func(Type) bool) bool {
	switch n := t.(type) {
	case *Alias:
		return visit(n.Target)
	case *Optional:
		return visit(n.Inner)
	case *Union:
		for _, member := range n.Members {
			if !visit(member) {
				return false
			}
		}
	case *Intersection:
		for _, member := range n.Members {
			if !visit(member) {
				return false
			}
		}
	case *Array:
		return visit(n.Element)
	case *Map:
		return visit(n.Key) && visit(n.Value)
	case *ReadonlyMap:
		return visit(n.Key) && visit(n.Value)
	case *Tuple:
		for _, elem := range n.Elements {
			if !visit(elem) {
				return false
			}
		}
	case *Function:
		for _, param := range n.TypeParams {
			if param != nil && !visit(param.Constraint) {
				return false
			}
		}
		for _, param := range n.Params {
			if !visit(param.Type) {
				return false
			}
		}
		if n.Variadic != nil && !visit(n.Variadic) {
			return false
		}
		for _, ret := range n.Returns {
			if !visit(ret) {
				return false
			}
		}
	case *Record:
		for _, field := range n.Fields {
			if !visit(field.Type) {
				return false
			}
		}
		for _, member := range n.StaticMembers {
			if !visit(member.Type) {
				return false
			}
		}
		if n.Metatable != nil && !visit(n.Metatable) {
			return false
		}
		if n.HasMapComponent() && (!visit(n.MapKey) || !visit(n.MapValue)) {
			return false
		}
	case *Meta:
		return visit(n.Of)
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && !visit(param.Constraint) {
				return false
			}
		}
		return visit(n.Body)
	case *Instantiated:
		if !visit(n.Generic) {
			return false
		}
		for _, arg := range n.TypeArgs {
			if !visit(arg) {
				return false
			}
		}
	case *TypeParam:
		return visit(n.Constraint)
	case *Interface:
		for _, method := range n.Methods {
			if method.Type != nil && !visit(method.Type) {
				return false
			}
		}
	}
	return true
}
