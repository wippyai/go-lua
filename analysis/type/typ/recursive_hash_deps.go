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
	if !recursiveGraphClosedWalk(r, seen, make(map[recursiveTraversalMemoKey]bool)) {
		return nil, false
	}
	deps := make([]recursiveHashDep, 0, len(seen))
	for rec := range seen {
		deps = append(deps, recursiveHashDep{rec: rec, rev: rec.rev})
	}
	return deps, true
}

func collectRecursiveHashDepsMemo(r *Recursive, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	return recursiveGraphClosedWalk(r, seen, memo)
}

func collectRecursiveHashDepsInTypeMemo(t Type, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	return recursiveGraphClosedWalk(t, seen, memo)
}
