package typ

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

// RecursiveIsKeyed reports whether this node carries a stable family-key identity.
func (r *Recursive) RecursiveIsKeyed() bool {
	if r == nil {
		return false
	}
	return r.keyed
}

// RecursiveFamilyNamespace returns the namespace component of the family key for
// a keyed recursive node, or "" for an unkeyed node.
func (r *Recursive) RecursiveFamilyNamespace() string {
	if r == nil {
		return ""
	}
	return r.familyNS
}

// RecursiveFamilyOwner returns the owner component of the family key for a keyed
// recursive node, or "" for an unkeyed node.
func (r *Recursive) RecursiveFamilyOwner() string {
	if r == nil {
		return ""
	}
	return r.familyOwnerStr
}

// MarkKeyedFamily seals this placeholder as a keyed family node owned by token.
// It sets keyed=true, records the (ns, owner) key pair, and stores the opaque
// owner token that the identity package uses for ownership checks.
func (r *Recursive) MarkKeyedFamily(ns, owner string, token any) {
	if r == nil {
		return
	}
	r.keyed = true
	r.familyNS = ns
	r.familyOwnerStr = owner
	r.owner = token
}

// FamilyOwnerToken returns the opaque owner token stored by MarkKeyedFamily.
// The identity package compares it by pointer identity for ownership checks.
func (r *Recursive) FamilyOwnerToken() any {
	if r == nil {
		return nil
	}
	return r.owner
}
