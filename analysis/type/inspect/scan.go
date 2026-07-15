package inspect

import (
	"github.com/wippyai/go-lua/analysis/type/internal/graph"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func containsMatching(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	scan := NewScanner(ScanOptions{Seen: newEqualitySeen()})
	return contains(t, pred, scan)
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
	if !active.Enter(t) {
		return false
	}
	defer active.Leave(t)
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

func contains(t typ.Type, pred func(typ.Type) bool, scan *Scanner) bool {
	if t == nil {
		return false
	}
	t = unwrapTransparent(t)
	if t == nil {
		return false
	}
	if !scan.Enter(t) {
		return false
	}
	if pred(t) {
		return true
	}

	return typ.WalkChildren(t, func(child typ.Type) bool {
		return contains(child, pred, scan)
	})
}
