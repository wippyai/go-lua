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
	t = unwrapAnnotated(t)
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
	default:
		result = recursiveTypeChildrenAll(t, func(child Type) bool {
			return collectRecursiveHashDepsInTypeMemo(child, seen, memo)
		})
	}
	if key, ok := recursiveTraversalMemo(t); ok {
		memo[key] = result
	}
	return result
}
