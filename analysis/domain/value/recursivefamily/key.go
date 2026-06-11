package recursivefamily

import "github.com/wippyai/go-lua/analysis/internal/hash"

// FamilyKey is the stable producer identity of a recursive family.
type FamilyKey struct {
	Namespace string
	Owner     string
}

// String renders the key for a recursion-variable name.
func (k FamilyKey) String() string {
	if k.Namespace == "" {
		return k.Owner
	}
	return k.Namespace + ":" + k.Owner
}

// IsZero reports whether no family key is present.
func (k FamilyKey) IsZero() bool {
	return k == FamilyKey{}
}

// Hash folds the key into a stable bucket value.
func (k FamilyKey) Hash() uint64 {
	h := hash.FnvString(k.Namespace)
	return hash.MixHash(h, hash.FnvString(k.Owner))
}
