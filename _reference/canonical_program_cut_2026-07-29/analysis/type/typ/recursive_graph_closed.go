package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

func recursiveContainsGraphClosed(t Type, seen map[*Recursive]bool) bool {
	return recursiveContainsGraphClosedMemo(t, seen, make(map[recursiveTraversalMemoKey]bool))
}

func recursiveGraphClosureForRecursive(r *Recursive) (bool, []recursiveHashDep) {
	if r == nil {
		return true, nil
	}
	seen := make(map[*Recursive]bool)
	closed := collectRecursiveGraphClosureDeps(r, seen, make(map[recursiveTraversalMemoKey]bool))
	return closed, recursiveGraphClosureDeps(seen)
}

// recursiveGraphClosureForType proves closure for a composite node and records
// every recursive node that proof depends on. Callers can reuse the proof until
// one of those placeholders receives a new body.
func recursiveGraphClosureForType(t Type) (bool, []recursiveHashDep) {
	seen := make(map[*Recursive]bool)
	closed := collectRecursiveGraphClosureDepsInType(t, seen, make(map[recursiveTraversalMemoKey]bool))
	return closed, recursiveGraphClosureDeps(seen)
}

func recursiveGraphClosureDeps(seen map[*Recursive]bool) []recursiveHashDep {
	deps := make([]recursiveHashDep, 0, len(seen))
	for rec := range seen {
		deps = append(deps, recursiveHashDep{rec: rec, rev: rec.rev})
	}
	return deps
}

func collectRecursiveGraphClosureDeps(r *Recursive, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	if r == nil {
		return true
	}
	if seen[r] {
		return true
	}
	seen[r] = true
	if r.Body == nil {
		return false
	}
	return collectRecursiveGraphClosureDepsInType(r.Body, seen, memo)
}

func collectRecursiveGraphClosureDepsInType(t Type, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	if t == nil {
		return true
	}
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return true
	}
	if key, ok := recursiveTraversalMemo(t); ok {
		if closed, found := memo[key]; found {
			return closed
		}
	}

	var result bool
	switch n := t.(type) {
	case nil:
		result = true
	case *Recursive:
		result = collectRecursiveGraphClosureDeps(n, seen, memo)
	default:
		result = recursiveTypeChildrenAll(t, func(child Type) bool {
			return collectRecursiveGraphClosureDepsInType(child, seen, memo)
		})
	}
	if key, ok := recursiveTraversalMemo(t); ok {
		memo[key] = result
	}
	return result
}

type recursiveTraversalMemoKey struct {
	kind kind.Kind
	ptr  uintptr
}

func recursiveContainsGraphClosedMemo(t Type, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	if t == nil {
		return true
	}
	if seen == nil {
		seen = make(map[*Recursive]bool)
	}
	t = unwrapAnnotatedOrNil(t)
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
		if n.Body == nil {
			result = false
			break
		}
		if seen[n] {
			result = true
			break
		}
		seen[n] = true
		result = recursiveContainsGraphClosedMemo(n.Body, seen, memo)
	default:
		result = recursiveTypeChildrenAll(t, func(child Type) bool {
			return recursiveContainsGraphClosedMemo(child, seen, memo)
		})
	}
	if key, ok := recursiveTraversalMemo(t); ok {
		memo[key] = result
	}
	return result
}

func recursiveTraversalMemo(t Type) (recursiveTraversalMemoKey, bool) {
	if t == nil {
		return recursiveTraversalMemoKey{}, false
	}
	ptr := typePointer(t)
	if ptr == 0 {
		ptr = uintptr(t.Kind())
	}
	return recursiveTraversalMemoKey{kind: t.Kind(), ptr: ptr}, true
}
