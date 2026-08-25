package address

import "github.com/wippyai/go-lua/analysis/identity"

// Address is a typed logical identity paired with one private runtime
// coordinate.  The coordinate is intentionally not exposed: callers carry
// addresses and validate their fence, while only the owning physical layer
// interprets the local uint64 during future mount integration.
type Address[T comparable] struct {
	id      T
	locator identity.Locator[uint64]
	fence   Fence
}

func newAddress[T comparable](id T, slot uint64, fence Fence) Address[T] {
	return Address[T]{
		id:      id,
		locator: identity.NewLocator(fence.StoreID(), fence.Generation(), slot),
		fence:   fence,
	}
}

// ID returns the stable logical identity.  It never returns the local slot.
func (address Address[T]) ID() T { return address.id }

// Available reports whether the address carries a complete fence and a
// generation-qualified locator.
func (address Address[T]) Available() bool {
	return address.fence.Available() && address.locator.Available()
}

// ValidFor reports whether the address belongs to exactly fence.  A stale
// generation, a different store, mount, schema, or certificate all fail.
func (address Address[T]) ValidFor(fence Fence) bool {
	return address.Available() && address.fence.Same(fence) && address.locator.Valid(fence.StoreID(), fence.Generation())
}

// Fence returns the address's immutable authentication fence.  It does not
// reveal the local coordinate.
func (address Address[T]) Fence() Fence { return address.fence }
