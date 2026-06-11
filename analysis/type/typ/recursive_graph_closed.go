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
	case *Alias:
		result = recursiveContainsGraphClosedMemo(n.Target, seen, memo)
	case *Optional:
		result = recursiveContainsGraphClosedMemo(n.Inner, seen, memo)
	case *Union:
		for _, member := range n.Members {
			if !recursiveContainsGraphClosedMemo(member, seen, memo) {
				result = false
				break
			}
		}
	case *Intersection:
		for _, member := range n.Members {
			if !recursiveContainsGraphClosedMemo(member, seen, memo) {
				result = false
				break
			}
		}
	case *Array:
		result = recursiveContainsGraphClosedMemo(n.Element, seen, memo)
	case *Map:
		result = recursiveContainsGraphClosedMemo(n.Key, seen, memo) &&
			recursiveContainsGraphClosedMemo(n.Value, seen, memo)
	case *ReadonlyMap:
		result = recursiveContainsGraphClosedMemo(n.Key, seen, memo) &&
			recursiveContainsGraphClosedMemo(n.Value, seen, memo)
	case *Tuple:
		for _, elem := range n.Elements {
			if !recursiveContainsGraphClosedMemo(elem, seen, memo) {
				result = false
				break
			}
		}
	case *Function:
		for _, param := range n.Params {
			if !recursiveContainsGraphClosedMemo(param.Type, seen, memo) {
				result = false
				break
			}
		}
		if result {
			for _, ret := range n.Returns {
				if !recursiveContainsGraphClosedMemo(ret, seen, memo) {
					result = false
					break
				}
			}
		}
		if result && n.Variadic != nil && !recursiveContainsGraphClosedMemo(n.Variadic, seen, memo) {
			result = false
		}
	case *Record:
		for _, field := range n.Fields {
			if !recursiveContainsGraphClosedMemo(field.Type, seen, memo) {
				result = false
				break
			}
		}
		if result {
			for _, member := range n.StaticMembers {
				if !recursiveContainsGraphClosedMemo(member.Type, seen, memo) {
					result = false
					break
				}
			}
		}
		if result && n.Metatable != nil && !recursiveContainsGraphClosedMemo(n.Metatable, seen, memo) {
			result = false
		}
		if result && n.HasMapComponent() {
			result = recursiveContainsGraphClosedMemo(n.MapKey, seen, memo) &&
				recursiveContainsGraphClosedMemo(n.MapValue, seen, memo)
		}
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && !recursiveContainsGraphClosedMemo(param.Constraint, seen, memo) {
				result = false
				break
			}
		}
		if result {
			result = recursiveContainsGraphClosedMemo(n.Body, seen, memo)
		}
	case *Instantiated:
		if !recursiveContainsGraphClosedMemo(n.Generic, seen, memo) {
			result = false
			break
		}
		for _, arg := range n.TypeArgs {
			if !recursiveContainsGraphClosedMemo(arg, seen, memo) {
				result = false
				break
			}
		}
	case *TypeParam:
		result = recursiveContainsGraphClosedMemo(n.Constraint, seen, memo)
	case *Interface:
		for _, method := range n.Methods {
			if method.Type != nil && !recursiveContainsGraphClosedMemo(method.Type, seen, memo) {
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
