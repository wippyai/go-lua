// Package evalscratch owns the bounded re-entrant depth accounting shared by
// compiled evaluators and runtime projection encoders.
package evalscratch

// Depth is worker-owned, bounded nesting state. Push never grows capacity:
// exhaustion is metered and reported to the caller.
type Depth struct {
	depth     uint32
	capacity  uint32
	overflows uint64
}

// NewDepth provisions a fixed nesting capacity. Negative capacities are
// treated as zero.
func NewDepth(capacity int) Depth {
	if capacity < 0 {
		capacity = 0
	}
	return Depth{capacity: uint32(capacity)}
}

// Push reserves the next frame and returns its zero-based index.
func (d *Depth) Push() (uint32, bool) {
	if d == nil || d.depth >= d.capacity {
		if d != nil {
			d.overflows++
		}
		return 0, false
	}
	index := d.depth
	d.depth++
	return index, true
}

// Pop releases the innermost frame.
func (d *Depth) Pop() bool {
	if d == nil || d.depth == 0 {
		return false
	}
	d.depth--
	return true
}

// Reset releases all frames without changing the fixed capacity.
func (d *Depth) Reset() {
	if d != nil {
		d.depth = 0
	}
}

// Len reports the number of currently reserved frames.
func (d *Depth) Len() int {
	if d == nil {
		return 0
	}
	return int(d.depth)
}

// Reject meters a capacity failure discovered by the storage owner.
func (d *Depth) Reject() {
	if d != nil {
		d.overflows++
	}
}

// OverflowCount reports rejected frame reservations.
func (d *Depth) OverflowCount() uint64 {
	if d == nil {
		return 0
	}
	return d.overflows
}
