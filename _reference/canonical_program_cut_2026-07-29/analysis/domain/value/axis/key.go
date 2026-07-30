package axis

// Key identifies one typed axis in a value product.
type Key[T any] struct {
	id string
}

// NewKey creates a typed axis key. The id must be stable across runs because it
// participates in product hashing and canonicalization.
func NewKey[T any](id string) Key[T] {
	if id == "" {
		panic("axis: empty key id")
	}
	return Key[T]{id: id}
}

// ID returns the stable erased key id.
func (k Key[T]) ID() string {
	return k.id
}

func (k Key[T]) String() string {
	return k.id
}
