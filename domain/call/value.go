package call

import "sort"

// Value is an immutable keyless dispatch relation in one Link-scoped Algebra.
// It stores only dense owner-local target selectors and an opaque arm.
type Value struct {
	owner     *Algebra
	top       bool
	known     bool
	open      bool
	selectors []selector
}

func (value Value) valid() bool {
	if !value.hotValid() {
		return false
	}
	if value.top {
		return true
	}
	for index, selector := range value.selectors {
		if !selector.valid() || uint64(selector) > uint64(len(value.owner.targets)) || index > 0 && value.selectors[index-1] >= selector {
			return false
		}
	}
	return true
}

// hotValid checks only the immutable representation header. Selector support
// was proved once by the package-private constructors; rescanning it in every
// lattice operation would turn the Factor width into avoidable validation
// cost. The full valid check remains on cold construction and public
// projections.
func (value Value) hotValid() bool {
	if value.owner == nil || !value.owner.Valid() {
		return false
	}
	if value.top {
		return !value.known && !value.open && len(value.selectors) == 0
	}
	return value.known
}

// Initial returns Call's structural seed. Application keys have no
// unconditional opaque seed: selected dispatch is their sole producer, so
// the neutral Bottom value leaves room for a complete dispatch proof.
// Callback and resume keys remain open because their external re-entry
// boundary is not selected by an ordinary callee Value.
func (algebra *Algebra) Initial(key Key) (Value, bool) {
	if !algebra.validKey(key) {
		return Value{}, false
	}
	if key.IsApplication() {
		return algebra.Bottom(), true
	}
	return algebra.open(key, nil)
}

// open is the ordinary Call constructor. It can add only known target
// alternatives while retaining the mandatory opaque alternative; only Call's
// narrow DispatchValue boundary may create a relation outside this package.
func (algebra *Algebra) open(key Key, selectors []selector) (Value, bool) {
	return algebra.value(key, true, selectors)
}

func (algebra *Algebra) value(key Key, open bool, selectors []selector) (Value, bool) {
	if !algebra.validKey(key) || open && !algebra.dynamic(key) {
		return Value{}, false
	}
	selectors = append([]selector(nil), selectors...)
	sort.Slice(selectors, func(i, j int) bool { return selectors[i] < selectors[j] })
	for index, selector := range selectors {
		if !algebra.contains(key, selector) || index > 0 && selectors[index-1] == selector {
			return Value{}, false
		}
	}
	if len(selectors) == 0 && !open {
		return algebra.Bottom(), true
	}
	return Value{owner: algebra, known: true, open: open, selectors: selectors}, true
}

func (value Value) IsTop() bool { return value.valid() && value.top }
func (value Value) IsEmpty() bool {
	return value.valid() && !value.top && !value.open && len(value.selectors) == 0
}
func (value Value) IsOpen() bool     { return value.valid() && !value.top && value.open }
func (value Value) IsComplete() bool { return value.valid() && !value.top && !value.open }

// HasOpaqueAlternative is true for both Open and Top. Top is conservative
// Call uncertainty, so consumers must retain the opaque boundary for it.
func (value Value) HasOpaqueAlternative() bool { return value.valid() && (value.top || value.open) }

func (value Value) HasTarget(target Target) bool {
	return value.valid() && !value.top && target.Valid() && target.owner == value.owner && containsSelector(value.selectors, target.selector)
}
func (value Value) usedAlternatives() uint64 {
	used := uint64(len(value.selectors))
	if value.open {
		used++
	}
	return used
}

func containsSelector(values []selector, selector selector) bool {
	index := sort.Search(len(values), func(index int) bool { return values[index] >= selector })
	return index < len(values) && values[index] == selector
}
func selectorsSubset(left, right []selector) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		switch {
		case left[leftIndex] == right[rightIndex]:
			leftIndex++
			rightIndex++
		case left[leftIndex] > right[rightIndex]:
			rightIndex++
		default:
			return false
		}
	}
	return leftIndex == len(left)
}
func unionSelectors(left, right []selector) []selector {
	out := make([]selector, 0, len(left)+len(right))
	li, ri := 0, 0
	for li < len(left) || ri < len(right) {
		switch {
		case ri == len(right) || li < len(left) && left[li] < right[ri]:
			out = append(out, left[li])
			li++
		case li == len(left) || right[ri] < left[li]:
			out = append(out, right[ri])
			ri++
		default:
			out = append(out, left[li])
			li++
			ri++
		}
	}
	return out
}
func intersectSelectors(left, right []selector) []selector {
	out := make([]selector, 0)
	li, ri := 0, 0
	for li < len(left) && ri < len(right) {
		switch {
		case left[li] < right[ri]:
			li++
		case right[ri] < left[li]:
			ri++
		default:
			out = append(out, left[li])
			li++
			ri++
		}
	}
	return out
}
