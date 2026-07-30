// Package caseset owns immutable, canonical finite variant-case sets.
package caseset

import "slices"

// Set is an immutable sorted set of variant case indexes.
//
// The backing storage is private and no API exposes it. Copying a Set is safe:
// every operation that constructs a Set takes its own copy and there are no
// mutating operations.
type Set struct {
	values []int
}

// New copies values, sorts them, and removes duplicates.
func New(values []int) Set {
	if len(values) == 0 {
		return Set{}
	}
	owned := append([]int(nil), values...)
	slices.Sort(owned)
	owned = slices.Compact(owned)
	return Set{values: owned}
}

// Len reports the number of cases.
func (s Set) Len() int { return len(s.values) }

// At returns the i-th case in canonical ascending order.
func (s Set) At(i int) int { return s.values[i] }

// View returns an allocation-free immutable view of s.
func (s Set) View() View { return View{values: s.values} }

// View is an allocation-free immutable view over a Set.
//
// It deliberately exposes only indexed reads: callers cannot mutate or retain
// the private backing slice through the type system.
type View struct {
	values []int
}

// Len reports the number of cases.
func (v View) Len() int { return len(v.values) }

// At returns the i-th case in canonical ascending order.
func (v View) At(i int) int { return v.values[i] }
