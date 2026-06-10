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

// RecursiveName returns the declared name of the recursion variable.
func (r *Recursive) RecursiveName() string {
	if r == nil {
		return ""
	}
	return r.Name
}

// RecursiveBody returns the body type, which may be nil for an unsealed placeholder.
func (r *Recursive) RecursiveBody() Type {
	if r == nil {
		return nil
	}
	return r.Body
}

// SetRecursiveBody is an alias for SetBody exposed for the identity axis package.
func (r *Recursive) SetRecursiveBody(body Type) {
	r.SetBody(body)
}

// RecursiveFamilyKey returns the optional family identity metadata for this node.
func (r *Recursive) RecursiveFamilyKey() RecursiveFamilyKey {
	if r == nil {
		return RecursiveFamilyKey{}
	}
	return r.familyKey
}
