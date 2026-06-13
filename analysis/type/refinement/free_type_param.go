package refinement

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func freeTypeParamSeenKey(t typ.Type) uint64 {
	if inspect.ContainsRecursive(t) {
		if ptr := nodeid.Pointer(t); ptr != 0 {
			return uint64(ptr)
		}
	}
	return typ.EqualityHash(t)
}

// ContainsFreeTypeParam reports whether t contains an unbound symbolic type
// parameter/reference. Unlike ContainsTypeParam, a closed instantiated generic
// such as Box<string> is treated as closed: only its concrete type arguments are
// inspected, not the generic declaration template.
func ContainsFreeTypeParam(t typ.Type) bool {
	scan := inspect.NewScanner(inspect.ScanOptions{
		Seen: inspect.NewEqualitySeenWithKey(freeTypeParamSeenKey),
	})
	return containsFreeTypeParam(t, scan, nil)
}

func containsFreeTypeParam(t typ.Type, scan *inspect.Scanner, owned map[*typ.TypeParam]int) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}

	switch v := t.(type) {
	case *typ.TypeParam:
		return owned == nil || owned[v] == 0
	}

	switch t.Kind() {
	case kind.Ref, kind.Generic:
		return true
	}

	if freeTypeParamUseSeen(owned) {
		if !scan.Enter(t) {
			return false
		}
	}

	switch v := t.(type) {
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if containsFreeTypeParam(arg, scan, owned) {
				return true
			}
		}
		return false
	case *typ.Function:
		nextOwned := owned
		if len(v.TypeParams) > 0 {
			nextOwned = make(map[*typ.TypeParam]int, len(owned)+len(v.TypeParams))
			for tp, count := range owned {
				nextOwned[tp] = count
			}
			for _, tp := range v.TypeParams {
				if tp != nil {
					nextOwned[tp]++
				}
			}
		}
		for _, param := range v.Params {
			if containsFreeTypeParam(param.Type, scan, nextOwned) {
				return true
			}
		}
		if containsFreeTypeParam(v.Variadic, scan, nextOwned) {
			return true
		}
		for _, ret := range v.Returns {
			if containsFreeTypeParam(ret, scan, nextOwned) {
				return true
			}
		}
		return false
	case *typ.Interface:
		return false
	}

	return scan.WalkChildren(t, func(child typ.Type) bool {
		return containsFreeTypeParam(child, scan, owned)
	})
}

func freeTypeParamUseSeen(owned map[*typ.TypeParam]int) bool {
	return len(owned) == 0
}
