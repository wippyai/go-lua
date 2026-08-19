package typ

import "github.com/wippyai/go-lua/domain/type/kind"

// IsGraphClosed proves that every reachable Recursive node has a body. A
// productive backedge to an already visited Recursive node is closed; a
// dangling placeholder anywhere in the finite graph is not. This is the
// owner-side admission predicate for callers that must not retain an open
// mutable graph as a closed value.
func IsGraphClosed(t Type) bool {
	return recursiveGraphClosureForType(t)
}

func recursiveContainsGraphClosed(t Type, seen map[*Recursive]bool) bool {
	return recursiveGraphClosedWalk(t, seen, make(map[recursiveTraversalMemoKey]bool))
}

func recursiveGraphClosureForRecursive(r *Recursive) bool {
	if r == nil {
		return true
	}
	return recursiveGraphClosedWalk(r, make(map[*Recursive]bool), make(map[recursiveTraversalMemoKey]bool))
}

// recursiveGraphClosureForType proves closure for a composite node: every
// reachable Recursive placeholder already has a body.
func recursiveGraphClosureForType(t Type) bool {
	return recursiveGraphClosedWalk(t, make(map[*Recursive]bool), make(map[recursiveTraversalMemoKey]bool))
}

// recursiveGraphClosedWalk is an exact finite graph walk. A visited recursive
// declaration closes a productive backedge; every distinct reachable node is
// still inspected for an unresolved recursive body.
func recursiveGraphClosedWalk(root Type, seen map[*Recursive]bool, memo map[recursiveTraversalMemoKey]bool) bool {
	if seen == nil {
		seen = make(map[*Recursive]bool)
	}
	if memo == nil {
		memo = make(map[recursiveTraversalMemoKey]bool)
	}
	work := []Type{root}
	active := make(map[recursiveTraversalMemoKey]bool)
	visited := make([]recursiveTraversalMemoKey, 0, 8)
	for len(work) != 0 {
		last := len(work) - 1
		current := unwrapAnnotatedOrNil(work[last])
		work = work[:last]
		if current == nil {
			continue
		}
		key, keyed := recursiveTraversalMemo(current)
		if keyed {
			if closed, found := memo[key]; found {
				if !closed {
					return false
				}
				continue
			}
			if active[key] {
				continue
			}
			active[key] = true
			visited = append(visited, key)
		}
		if recursive, ok := current.(*Recursive); ok {
			if recursive.Body == nil {
				seen[recursive] = true
				if keyed {
					memo[key] = false
				}
				return false
			}
			if seen[recursive] {
				continue
			}
			seen[recursive] = true
			work = append(work, recursive.Body)
			continue
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
	for _, key := range visited {
		memo[key] = true
	}
	return true
}

type recursiveTraversalMemoKey struct {
	kind kind.Kind
	ptr  uintptr
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
