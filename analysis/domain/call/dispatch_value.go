package call

import "sort"

// DispatchValue is Call's narrow completion boundary. Call consumers may
// provide only Target capabilities issued by this Algebra; raw selector
// ordinals and cross-domain facts cannot enter the core carrier. The
// cross-domain dispatch Rule proves whether opaque must be retained before
// invoking this constructor.
func (algebra *Algebra) DispatchValue(key Key, targets []Target, opaque bool) (Value, bool) {
	if algebra == nil || !algebra.validKey(key) {
		return Value{}, false
	}
	selectors := make([]selector, len(targets))
	for index, target := range targets {
		if !target.Valid() || target.owner != algebra || !algebra.contains(key, target.selector) {
			return Value{}, false
		}
		selectors[index] = target.selector
	}
	sort.Slice(selectors, func(left, right int) bool { return selectors[left] < selectors[right] })
	unique := selectors[:0]
	for _, selector := range selectors {
		if len(unique) == 0 || unique[len(unique)-1] != selector {
			unique = append(unique, selector)
		}
	}
	selectors = unique
	if opaque {
		return algebra.open(key, selectors)
	}
	return algebra.value(key, false, selectors)
}
