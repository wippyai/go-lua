package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

func recursiveContainsGraphClosed(t Type, seen map[*Recursive]bool, depth int) bool {
	return recursiveContainsGraphClosedMemo(t, seen, make(map[graphClosedKey]bool), depth)
}

type graphClosedKey struct {
	kind kind.Kind
	ptr  uintptr
}

func recursiveContainsGraphClosedMemo(t Type, seen map[*Recursive]bool, memo map[graphClosedKey]bool, depth int) bool {
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return true
	}
	if key, ok := graphClosedMemoKey(t); ok {
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
		result = recursiveContainsGraphClosedMemo(n.Body, seen, memo, depth+1)
	case *Alias:
		result = recursiveContainsGraphClosedMemo(n.Target, seen, memo, depth+1)
	case *Optional:
		result = recursiveContainsGraphClosedMemo(n.Inner, seen, memo, depth+1)
	case *Union:
		for _, member := range n.Members {
			if !recursiveContainsGraphClosedMemo(member, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *Intersection:
		for _, member := range n.Members {
			if !recursiveContainsGraphClosedMemo(member, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *Array:
		result = recursiveContainsGraphClosedMemo(n.Element, seen, memo, depth+1)
	case *Map:
		result = recursiveContainsGraphClosedMemo(n.Key, seen, memo, depth+1) &&
			recursiveContainsGraphClosedMemo(n.Value, seen, memo, depth+1)
	case *ReadonlyMap:
		result = recursiveContainsGraphClosedMemo(n.Key, seen, memo, depth+1) &&
			recursiveContainsGraphClosedMemo(n.Value, seen, memo, depth+1)
	case *Tuple:
		for _, elem := range n.Elements {
			if !recursiveContainsGraphClosedMemo(elem, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *Function:
		for _, param := range n.Params {
			if !recursiveContainsGraphClosedMemo(param.Type, seen, memo, depth+1) {
				result = false
				break
			}
		}
		if result {
			for _, ret := range n.Returns {
				if !recursiveContainsGraphClosedMemo(ret, seen, memo, depth+1) {
					result = false
					break
				}
			}
		}
		if result && n.Variadic != nil && !recursiveContainsGraphClosedMemo(n.Variadic, seen, memo, depth+1) {
			result = false
		}
	case *Record:
		for _, field := range n.Fields {
			if !recursiveContainsGraphClosedMemo(field.Type, seen, memo, depth+1) {
				result = false
				break
			}
		}
		if result {
			for _, member := range n.StaticMembers {
				if !recursiveContainsGraphClosedMemo(member.Type, seen, memo, depth+1) {
					result = false
					break
				}
			}
		}
		if result && n.Metatable != nil && !recursiveContainsGraphClosedMemo(n.Metatable, seen, memo, depth+1) {
			result = false
		}
		if result && n.HasMapComponent() {
			result = recursiveContainsGraphClosedMemo(n.MapKey, seen, memo, depth+1) &&
				recursiveContainsGraphClosedMemo(n.MapValue, seen, memo, depth+1)
		}
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && !recursiveContainsGraphClosedMemo(param.Constraint, seen, memo, depth+1) {
				result = false
				break
			}
		}
		if result {
			result = recursiveContainsGraphClosedMemo(n.Body, seen, memo, depth+1)
		}
	case *Instantiated:
		if !recursiveContainsGraphClosedMemo(n.Generic, seen, memo, depth+1) {
			result = false
			break
		}
		for _, arg := range n.TypeArgs {
			if !recursiveContainsGraphClosedMemo(arg, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *TypeParam:
		result = recursiveContainsGraphClosedMemo(n.Constraint, seen, memo, depth+1)
	case *Sum:
		for _, variant := range n.Variants {
			for _, t := range variant.Types {
				if !recursiveContainsGraphClosedMemo(t, seen, memo, depth+1) {
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
			if method.Type != nil && !recursiveContainsGraphClosedMemo(method.Type, seen, memo, depth+1) {
				result = false
				break
			}
		}
	}
	if key, ok := graphClosedMemoKey(t); ok {
		memo[key] = result
	}
	return result
}

func graphClosedMemoKey(t Type) (graphClosedKey, bool) {
	if t == nil {
		return graphClosedKey{}, false
	}
	ptr := typePointer(t)
	if ptr == 0 {
		ptr = uintptr(t.Kind())
	}
	return graphClosedKey{kind: t.Kind(), ptr: ptr}, true
}
