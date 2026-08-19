package engine

import "github.com/wippyai/go-lua/analysis/identity"

// FrozenResult is the typed persistence contract for one receipt-native Query
// projection. Its callbacks are behavior owned by the sealed query cell; no
// declaration carrier or cold execution root is retained here.
type FrozenResult[R any] struct {
	Semantic    identity.SemanticKey
	Freeze      func(R) R
	Clone       func(R) R
	Equal       func(R, R) bool
	Fingerprint func(R) uint64
	// Present is the owning domain's row-presence predicate. A result value
	// may be a valid typed fold state while still representing a covered
	// absence; the publication layer uses this predicate to withdraw that
	// member instead of inventing a hit.
	Present func(R) bool
}

func validFrozenResult[R any](result FrozenResult[R]) bool {
	return result.Semantic.Available() && result.Freeze != nil && result.Clone != nil && result.Equal != nil && result.Fingerprint != nil && result.Present != nil
}
