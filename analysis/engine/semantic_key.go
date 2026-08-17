// Package engine assembles canonical Program semantics into one symbolic
// analyzer. Domains declare typed Factor schemas; the engine owns no domain
// algebra and assigns no executable storage until template compilation.
package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// compositionKeyOf is the one-way conversion to the cold canonical
// representation. No public engine API exposes composition.ID. It reads the
// identity through its published accessors, so the engine never becomes a
// second authority over the digest bytes.
func compositionKeyOf(key identity.SemanticKey) composition.Key {
	return composition.Key{ID: composition.ID(key.Digest()), Version: key.Version()}
}

// semanticKeyFromComposition returns the same already-sealed canonical
// identity in the public opaque wrapper. It does not derive a second digest or
// admit caller content; runtime derivations use it only for a compiler-issued
// Rule-instance identity.
func semanticKeyFromComposition(key composition.Key) identity.SemanticKey {
	semantic, _ := identity.NewSemanticKey([32]byte(key.ID), key.Version)
	return semantic
}
