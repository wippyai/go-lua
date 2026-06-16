package typ

func recursiveTypeChildrenAll(t Type, visit func(Type) bool) bool {
	if visit == nil {
		return true
	}
	return !walkChildren(t, func(child Type) bool {
		return !visit(child)
	})
}
