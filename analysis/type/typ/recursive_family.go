package typ

// RecursiveFamilyKey is stable identity metadata for recursive families minted
// outside typ.
type RecursiveFamilyKey struct {
	Namespace string
	Owner     string
}

// String renders the key for a recursion-variable name.
func (k RecursiveFamilyKey) String() string {
	if k.Namespace == "" {
		return k.Owner
	}
	return k.Namespace + ":" + k.Owner
}

// IsZero reports whether no family key is present.
func (k RecursiveFamilyKey) IsZero() bool {
	return k == RecursiveFamilyKey{}
}

// RecursiveFamilyKey returns the optional family identity metadata for this node.
func (r *Recursive) RecursiveFamilyKey() RecursiveFamilyKey {
	if r == nil {
		return RecursiveFamilyKey{}
	}
	return r.familyKey
}
