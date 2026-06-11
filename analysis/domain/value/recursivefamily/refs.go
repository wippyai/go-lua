package recursivefamily

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RebindRecursiveRef rewrites every reference to the recursion variable from
// inside body to to, so a body sealed under one recursion variable is re-rooted
// onto another (a fresh fold placeholder onto its family handle). The
// from node itself is left untouched when it appears as a non-reference value.
func RebindRecursiveRef(body typ.Type, from, to *typ.Recursive) typ.Type {
	if body == nil || from == nil || to == nil || from == to {
		return body
	}
	return typ.Rewrite(body, func(node typ.Type) (typ.Type, bool) {
		if typ.IsRecursiveRef(node, from) {
			return to, true
		}
		return nil, false
	})
}

// ContainsRecursiveRef reports whether body contains any reference to the
// recursion variable rec (the same node, the same ID, or a reference with
// rec by structural recursive identity.
func ContainsRecursiveRef(body typ.Type, rec *typ.Recursive) bool {
	if body == nil || rec == nil {
		return false
	}
	return inspect.Contains(body, func(t typ.Type) bool {
		other, ok := t.(*typ.Recursive)
		if !ok || other == nil {
			return false
		}
		if typ.IsRecursiveRef(t, rec) {
			return true
		}
		return false
	})
}
