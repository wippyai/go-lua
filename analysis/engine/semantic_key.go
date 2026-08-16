// Package engine assembles canonical Program semantics into one symbolic
// analyzer. Domains declare typed Factor schemas; the engine owns no domain
// algebra and assigns no executable storage until template compilation.
package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// SemanticKey is the complete, versioned identity of one authored schema.
// The digest is canonical content supplied by the composition root; it is not
// a package name, registration ordinal, pointer, carrier slot, or Program
// import. Its fields stay private so callers cannot manufacture a partially
// available identity or depend on the internal composition representation.
type SemanticKey struct {
	digest  [32]byte
	version uint64
}

// NewSemanticKey admits one already-canonical semantic digest. The engine
// deliberately does not hash source text or Program objects here: that would
// create a second identity authority. Callers derive the digest from their
// canonical content and pass it by value.
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

// compositionKey is the one-way private conversion to the cold canonical
// representation. No public engine API exposes composition.ID.
func (key SemanticKey) compositionKey() composition.Key {
	return composition.Key{ID: composition.ID(key.digest), Version: key.version}
}

// semanticKeyFromComposition returns the same already-sealed canonical
// identity in the public opaque wrapper. It does not derive a second digest or
// admit caller content; runtime derivations use it only for a compiler-issued
// Rule-instance identity.
func semanticKeyFromComposition(key composition.Key) SemanticKey {
	return SemanticKey{digest: [32]byte(key.ID), version: key.Version}
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

// compareSemanticKey is the sole canonical ordering for authored schema
// identities. Physical layout, if any, is derived later by the E compiler.
func compareSemanticKey(left, right SemanticKey) int {
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
