package signature

import "crypto/sha256"

// ContentID is the full-width semantic identity of a function signature.
// It is derived from a versioned canonical encoding, never from process-local
// ordinals or by concatenating the domain's narrow equality hashes.
type ContentID [sha256.Size]byte

// Available reports whether the identity was successfully derived.
func (id ContentID) Available() bool { return id != ContentID{} }
