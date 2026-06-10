package recursivefamily

import "github.com/wippyai/go-lua/analysis/internal/hash"

// Key is stable identity metadata for recursive families minted outside typ.
type Key struct {
	Namespace string
	Owner     string
}

// String renders the key for a recursion-variable name.
func (k Key) String() string {
	if k.Namespace == "" {
		return k.Owner
	}
	return k.Namespace + ":" + k.Owner
}

// IsZero reports whether no family key is present.
func (k Key) IsZero() bool {
	return k == Key{}
}

// Hash folds the key into a stable bucket value.
func (k Key) Hash() uint64 {
	h := hash.FnvString(k.Namespace)
	return hash.HashCombine(h, hash.FnvString(k.Owner))
}
