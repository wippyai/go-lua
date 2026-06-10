package mutation

type TypeTransformVisitor[R any] struct {
	Unchanged             func(Unchanged) R
	ElementUnion          func(ElementUnion) R
	ContainerElementUnion func(ContainerElementUnion) R
	ToArray               func(ToArray) R
	Default               func(TypeTransform) R
}

func VisitTransform[R any](t TypeTransform, v TypeTransformVisitor[R]) R {
	switch tt := t.(type) {
	case Unchanged:
		if v.Unchanged != nil {
			return v.Unchanged(tt)
		}
	case *Unchanged:
		if v.Unchanged != nil {
			return v.Unchanged(*tt)
		}
	case ElementUnion:
		if v.ElementUnion != nil {
			return v.ElementUnion(tt)
		}
	case *ElementUnion:
		if v.ElementUnion != nil {
			return v.ElementUnion(*tt)
		}
	case ContainerElementUnion:
		if v.ContainerElementUnion != nil {
			return v.ContainerElementUnion(tt)
		}
	case *ContainerElementUnion:
		if v.ContainerElementUnion != nil {
			return v.ContainerElementUnion(*tt)
		}
	case ToArray:
		if v.ToArray != nil {
			return v.ToArray(tt)
		}
	case *ToArray:
		if v.ToArray != nil {
			return v.ToArray(*tt)
		}
	}
	if v.Default != nil {
		return v.Default(t)
	}
	var zero R
	return zero
}
