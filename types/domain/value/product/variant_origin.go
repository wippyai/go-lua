package product

import "github.com/wippyai/go-lua/types/domain/value/axis/variantorigin"

// WithVariantOrigin attaches finite variant-origin evidence to a product value.
// It refines only the origin axis; structural projection remains unchanged.
func WithVariantOrigin(av AbstractValue, family uint64, cases []int) AbstractValue {
	if av.IsZero() {
		return av
	}
	n := *av.n
	n.origin = variantorigin.Of(family, cases)
	return AbstractValue{n: intern(reduce(&n)), prov: av.prov}
}

// NarrowVariantOriginCase applies an equality/inequality proof over the origin
// axis. A contradictory proof returns product Bottom.
func NarrowVariantOriginCase(av AbstractValue, family uint64, caseIndex int, equal bool) (AbstractValue, bool) {
	if av.IsZero() || family == 0 {
		return AbstractValue{}, false
	}
	nextOrigin := av.n.origin.NarrowCase(family, caseIndex, equal)
	if variantorigin.Equal(nextOrigin, av.n.origin) {
		return av, false
	}
	n := *av.n
	n.origin = nextOrigin
	return AbstractValue{n: intern(reduce(&n)), prov: av.prov}, true
}
