// Package scalar defines the one ordered key representation accepted by the
// facts plane. Keys remain their declared named scalar type in every map,
// tree, change row, and reverse closure; this constraint only permits the
// algorithms to compare them directly.
package scalar

// Key admits compact named key identities without erasing their type at the
// facts boundary. Both underlying representations are totally ordered and
// zero-extend exactly when an external uint64 bound or opaque identity needs
// it.
type Key interface {
	~uint32 | ~uint64
}
