package call

import internal "github.com/wippyai/go-lua/internal/hash"

// Fingerprint returns the deterministic in-memory fingerprint of one Value in
// this exact Call algebra. It is a hot State operation; persistent algebra
// identity remains ContentID.
func (algebra *Algebra) Fingerprint(value Value) uint64 {
	if !algebra.owns(value) {
		return 0
	}
	hash := internal.MixHash(0x63616c6c2d666163, 1) // "call-fac"
	if value.top {
		return internal.MixHash(hash, 1)
	}
	if value.open {
		hash = internal.MixHash(hash, 2)
	}
	for _, selector := range value.selectors {
		hash = internal.MixHash(hash, uint64(selector)+3)
	}
	return hash
}
