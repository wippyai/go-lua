package typ

import "github.com/wippyai/go-lua/analysis/type/recursivefamily"

// RecursiveFamilyKey returns the optional family identity metadata for this node.
func (r *Recursive) RecursiveFamilyKey() recursivefamily.Key {
	if r == nil {
		return recursivefamily.Key{}
	}
	return r.familyKey
}
