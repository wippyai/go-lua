package engine

// FrozenResult is the typed persistence contract for one receipt-native Query
// projection. Its callbacks are behavior owned by the sealed query cell; no
// declaration carrier or cold execution root is retained here.
type FrozenResult[R any] struct {
	Semantic    SemanticKey
	Freeze      func(R) R
	Clone       func(R) R
	Equal       func(R, R) bool
	Fingerprint func(R) uint64
}

func validFrozenResult[R any](result FrozenResult[R]) bool {
	return result.Semantic.Available() && result.Freeze != nil && result.Clone != nil && result.Equal != nil && result.Fingerprint != nil
}
