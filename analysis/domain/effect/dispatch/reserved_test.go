package dispatch

import "testing"

func TestReservedLabels(t *testing.T) {
	assertDispatchLabel(t, "variadic transform", VariadicTransform{}, "variadic_transform", VariadicTransform{})
	assertDispatchLabel(t, "type predicate", TypePredicate{}, "type_predicate", TypePredicate{})
}
