package identity

// SemanticKey is the complete, versioned identity of one authored schema.
// The digest is canonical content supplied by the composition root; it is not
// a package name, registration ordinal, pointer, carrier slot, or Program
// import. Its fields stay private so callers cannot manufacture a partially
// available identity.
type SemanticKey struct {
	digest  [32]byte
	version uint64
}

// NewSemanticKey admits one already-canonical semantic digest. The identity
// tree deliberately does not hash source text or Program objects here: that
// would create a second identity authority. Callers derive the digest from
// their canonical content and pass it by value.
func NewSemanticKey(digest [32]byte, version uint64) (SemanticKey, bool) {
	key := SemanticKey{digest: digest, version: version}
	return key, key.Available()
}

// Digest returns the immutable canonical content digest.
func (key SemanticKey) Digest() [32]byte { return key.digest }

// Version returns the semantic interpretation version.
func (key SemanticKey) Version() uint64 { return key.version }

// Available reports whether key can name canonical composition input.
func (key SemanticKey) Available() bool {
	return key.digest != [32]byte{} && key.version != 0
}

// DistinctKeys reports whether every key is usable and no two name the same
// semantic role. It is the one admission law for a rule's identity tuple.
func DistinctKeys(keys ...SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for _, prior := range keys[:index] {
			if prior == key {
				return false
			}
		}
	}
	return true
}

// CompareSemanticKey is the sole canonical ordering for authored schema
// identities. Physical layout, if any, is derived later by the E compiler.
func CompareSemanticKey(left, right SemanticKey) int {
	for index := range left.digest {
		if left.digest[index] < right.digest[index] {
			return -1
		}
		if left.digest[index] > right.digest[index] {
			return 1
		}
	}
	if left.version < right.version {
		return -1
	}
	if left.version > right.version {
		return 1
	}
	return 0
}
