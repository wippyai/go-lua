package axis

import "reflect"

// Key identifies one typed axis in a value product.
type Key[T any] struct {
	id     string
	typeID reflect.Type
}

// NewKey creates a typed axis key. The id must be stable across runs because it
// participates in product hashing and canonicalization.
func NewKey[T any](id string) Key[T] {
	if id == "" {
		panic("axis: empty key id")
	}
	return Key[T]{id: id, typeID: reflect.TypeFor[T]()}
}

// ID returns the stable erased key id.
func (k Key[T]) ID() string {
	return k.id
}

// Type returns the typed key's cached runtime type identity. It is populated
// once when NewKey constructs the key, so product operations can validate a
// key without rebuilding a reflect.Type on every access.
func (k Key[T]) Type() reflect.Type {
	return k.typeID
}

func (k Key[T]) String() string {
	return k.id
}
