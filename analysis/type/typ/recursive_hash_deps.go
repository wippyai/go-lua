package typ

type recursiveHashDep struct {
	rec *Recursive
	rev uint64
}

func recursiveHashDepsValid(deps []recursiveHashDep) bool {
	for _, dep := range deps {
		if dep.rec == nil || dep.rec.rev != dep.rev {
			return false
		}
	}
	return true
}

func recursiveHashDeps(r *Recursive) ([]recursiveHashDep, bool) {
	if r == nil {
		return nil, true
	}
	seen := make(map[*Recursive]bool)
	if !collectRecursiveHashDepsMemo(r, seen, make(map[recursiveTraversalMemoKey]bool)) {
		return nil, false
	}
	deps := make([]recursiveHashDep, 0, len(seen))
	for rec := range seen {
		deps = append(deps, recursiveHashDep{rec: rec, rev: rec.rev})
	}
	return deps, true
}

func collectRecursiveHashDepsMemo(r *Recursive, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	if r == nil {
		return true
	}
	if r.Body == nil {
		return false
	}
	if seen[r] {
		return true
	}
	seen[r] = true
	return collectRecursiveHashDepsInTypeMemo(r.Body, seen, memo)
}

func collectRecursiveHashDepsInTypeMemo(t Type, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return true
	}
	if key, ok := recursiveTraversalMemo(t); ok {
		if closed, found := memo[key]; found {
			return closed
		}
	}

	result := true
	switch n := t.(type) {
	case nil:
		result = true
	case *Recursive:
		result = collectRecursiveHashDepsMemo(n, seen, memo)
	case *Alias:
		result = collectRecursiveHashDepsInTypeMemo(n.Target, seen, memo)
	case *Optional:
		result = collectRecursiveHashDepsInTypeMemo(n.Inner, seen, memo)
	case *Union:
		for _, member := range n.Members {
			if !collectRecursiveHashDepsInTypeMemo(member, seen, memo) {
				result = false
				break
			}
		}
	case *Intersection:
		for _, member := range n.Members {
			if !collectRecursiveHashDepsInTypeMemo(member, seen, memo) {
				result = false
				break
			}
		}
	case *Array:
		result = collectRecursiveHashDepsInTypeMemo(n.Element, seen, memo)
	case *Map:
		result = collectRecursiveHashDepsInTypeMemo(n.Key, seen, memo) &&
			collectRecursiveHashDepsInTypeMemo(n.Value, seen, memo)
	case *ReadonlyMap:
		result = collectRecursiveHashDepsInTypeMemo(n.Key, seen, memo) &&
			collectRecursiveHashDepsInTypeMemo(n.Value, seen, memo)
	case *Tuple:
		for _, elem := range n.Elements {
			if !collectRecursiveHashDepsInTypeMemo(elem, seen, memo) {
				result = false
				break
			}
		}
	case *Function:
		for _, param := range n.Params {
			if !collectRecursiveHashDepsInTypeMemo(param.Type, seen, memo) {
				result = false
				break
			}
		}
		if result {
			for _, ret := range n.Returns {
				if !collectRecursiveHashDepsInTypeMemo(ret, seen, memo) {
					result = false
					break
				}
			}
		}
		if result && n.Variadic != nil && !collectRecursiveHashDepsInTypeMemo(n.Variadic, seen, memo) {
			result = false
		}
	case *Record:
		for _, field := range n.Fields {
			if !collectRecursiveHashDepsInTypeMemo(field.Type, seen, memo) {
				result = false
				break
			}
		}
		if result {
			for _, member := range n.StaticMembers {
				if !collectRecursiveHashDepsInTypeMemo(member.Type, seen, memo) {
					result = false
					break
				}
			}
		}
		if result && n.Metatable != nil && !collectRecursiveHashDepsInTypeMemo(n.Metatable, seen, memo) {
			result = false
		}
		if result && n.HasMapComponent() {
			result = collectRecursiveHashDepsInTypeMemo(n.MapKey, seen, memo) &&
				collectRecursiveHashDepsInTypeMemo(n.MapValue, seen, memo)
		}
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && !collectRecursiveHashDepsInTypeMemo(param.Constraint, seen, memo) {
				result = false
				break
			}
		}
		if result {
			result = collectRecursiveHashDepsInTypeMemo(n.Body, seen, memo)
		}
	case *Instantiated:
		if !collectRecursiveHashDepsInTypeMemo(n.Generic, seen, memo) {
			result = false
			break
		}
		for _, arg := range n.TypeArgs {
			if !collectRecursiveHashDepsInTypeMemo(arg, seen, memo) {
				result = false
				break
			}
		}
	case *TypeParam:
		result = collectRecursiveHashDepsInTypeMemo(n.Constraint, seen, memo)
	case *Sum:
		for _, variant := range n.Variants {
			for _, t := range variant.Types {
				if !collectRecursiveHashDepsInTypeMemo(t, seen, memo) {
					result = false
					break
				}
			}
			if !result {
				break
			}
		}
	case *Interface:
		for _, method := range n.Methods {
			if method.Type != nil && !collectRecursiveHashDepsInTypeMemo(method.Type, seen, memo) {
				result = false
				break
			}
		}
	}
	if key, ok := recursiveTraversalMemo(t); ok {
		memo[key] = result
	}
	return result
}
