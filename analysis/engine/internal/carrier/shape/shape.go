// Package shape owns the dense physical layout for one sealed carrier
// composition. A Slot is a vector position; semantic ordering is supplied by
// the solver before composition reaches carrier.
package shape

// Slot is one dense host-natural physical position in a sealed composition.
// Persistence encodes this index canonically; the in-memory carrier imposes
// no arbitrary 16-bit composition-width limit.
type Slot int

// Shape freezes a finite physical width.
type Shape struct{ count int }

// Seal freezes one finite physical width.
func Seal(count int) (*Shape, bool) {
	if count < 0 {
		return nil, false
	}
	return &Shape{count: count}, true
}

// Count returns the composition's fixed physical width.
func (shape *Shape) Count() int {
	if shape == nil {
		return 0
	}
	return shape.count
}

// ValidSlot proves that slot belongs to this exact sealed composition.
func (shape *Shape) ValidSlot(slot Slot) bool {
	return shape != nil && slot >= 0 && int(slot) < shape.count
}
