package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

func recursiveContainsGraphClosed(t Type, seen map[*Recursive]bool) bool {
	return recursiveContainsGraphClosedMemo(t, seen, make(map[recursiveTraversalMemoKey]bool))
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
