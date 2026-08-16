package inspect

import (
	graph "github.com/wippyai/go-lua/analysis/domain/type/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func containsMatching(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	return contains(t, pred, newEqualitySeen())
}

// ContainsUnknown reports whether t contains the unresolved unknown type.
func ContainsUnknown(t typ.Type) bool {
	return containsMatching(t, typ.IsUnknown)
}

// IsMultiArmUnion reports whether t resolves through transparent wrappers to a
// union with at least two members.
func IsMultiArmUnion(t typ.Type) bool {
	return isMultiArmUnion(t, &graph.Path{})
}

func isMultiArmUnion(t typ.Type, active *graph.Path) bool {
	if t == nil {
		return false
	}
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return len(v.Members) >= 2
	case *typ.Alias:
		return isMultiArmUnion(v.UnaliasedTarget(), active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return isMultiArmUnion(v.Body, active)
	default:
		return false
	}
}

// contains walks every node reachable from t. Its only termination measure is
// seen: a node already remembered closes the branch, so a finite type graph is
// traversed exhaustively however deep it is, and a cyclic one still stops.
func contains(t typ.Type, pred func(typ.Type) bool, seen ScanSeen) bool {
	if t == nil {
		return false
	}
	t = unwrapTransparent(t)
	if t == nil {
		return false
	}
	if seen.Contains(t) {
		return false
	}
	seen.Remember(t)
	if pred(t) {
		return true
	}

	return typ.WalkChildren(t, func(child typ.Type) bool {
		return contains(child, pred, seen)
	})
}
