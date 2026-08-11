package typ

type containmentFlag uint8

const (
	containmentAny containmentFlag = iota + 1
	containmentNever
	containmentTypeParam
	containmentInstantiated
	containmentGeneric
)

func containsAnyDynamic(t Type, seen map[Type]bool, _ int) bool {
	return containsDynamicFlag(t, seen, containmentAny)
}

func containsNeverDynamic(t Type, seen map[Type]bool) bool {
	return containsDynamicFlag(t, seen, containmentNever)
}

func containsTypeParamDynamic(t Type, seen map[Type]bool, _ int) bool {
	return containsDynamicFlag(t, seen, containmentTypeParam)
}

func containsInstantiatedDynamic(t Type, seen map[Type]bool, _ int) bool {
	return containsDynamicFlag(t, seen, containmentInstantiated)
}

func containsDynamicFlag(
	t Type,
	seen map[Type]bool,
	flag containmentFlag,
) bool {
	if t == nil || flag == 0 {
		return false
	}
	if seen == nil {
		seen = make(map[Type]bool)
	}
	work := []Type{t}
	for len(work) != 0 {
		last := len(work) - 1
		current := unwrapAnnotated(work[last])
		work = work[:last]
		if current == nil {
			continue
		}
		if recursive, ok := current.(*Recursive); ok {
			if seen[current] {
				continue
			}
			seen[current] = true
			work = append(work, recursive.Body)
			continue
		}
		// A node can intrinsically satisfy the query while also containing a
		// recursive back-edge. Preserve that local truth before bypassing stale
		// transitive construction flags.
		if flag.direct(current) {
			return true
		}
		// Instantiated's Generic is a binder, not a free-formal child.  For
		// type-parameter containment only its arguments are visible in the
		// application scope; walking the Generic would reintroduce the
		// declaration's already-substituted formals.
		if flag == containmentTypeParam {
			if instantiated, ok := current.(*Instantiated); ok {
				if seen[current] {
					continue
				}
				seen[current] = true
				work = append(work, instantiated.TypeArgs...)
				continue
			}
		}
		if !knownContainsRecursive(current) {
			if flag.known(current) {
				return true
			}
			continue
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
	return false
}

func (f containmentFlag) direct(t Type) bool {
	switch f {
	case containmentAny:
		return IsAny(t)
	case containmentNever:
		return IsNever(t)
	case containmentTypeParam:
		_, ok := t.(*TypeParam)
		return ok
	case containmentInstantiated:
		_, ok := t.(*Instantiated)
		return ok
	case containmentGeneric:
		switch t.(type) {
		case *Generic, *Instantiated:
			return true
		}
	}
	return false
}

func (f containmentFlag) known(t Type) bool {
	switch f {
	case containmentAny:
		return knownContainsAny(t)
	case containmentNever:
		return knownContainsNever(t)
	case containmentTypeParam:
		return knownContainsTypeParam(t)
	case containmentInstantiated:
		return knownContainsInstantiated(t)
	case containmentGeneric:
		return knownContainsGeneric(t)
	default:
		return false
	}
}
