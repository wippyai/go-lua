package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

// missingTypePair reports the only intrinsically unprovable relation: a
// missing endpoint. Recursive type relations terminate through exact pair
// coinduction in checker, not a traversal budget: a finite acyclic type graph
// must be checked to its leaf no matter how deeply it is nested.
func missingTypePair(sub, super typ.Type) bool {
	return sub == nil || super == nil
}
