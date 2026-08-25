package engine

import "testing"

// The laws of the one projection from a bound Factor onto the algebra the axis
// surface publishes it as. There is one projection because a restatement of
// this spec's fields made once per axis is a restatement that drifts, and the
// measures are the pair a copy silently drops.

// TestAnAxisAlgebraCarriesEveryMeasureItsFactorDeclares states that both
// measures cross the projection. A widening rank that arrived as the absent
// rank would let an axis iterate without a bound, and a narrowing rank that did
// would silently discard a declared descent.
func TestAnAxisAlgebraCarriesEveryMeasureItsFactorDeclares(t *testing.T) {
	spec := hotUintFactorSpec()
	spec.WidenRank = Measure[uint64, uint64]{Width: 2, At: func(_ uint64, value uint64, component int) uint64 {
		return value + uint64(component)
	}}
	spec.NarrowRank = Measure[uint64, uint64]{Width: 3, At: func(_ uint64, value uint64, component int) uint64 {
		return value*10 + uint64(component)
	}}
	algebra, ok := spec.AxisAlgebra()
	if !ok {
		t.Fatal("a complete factor spec projects no axis algebra")
	}
	if algebra.KeyEnd != spec.KeyEnd {
		t.Fatalf("projected key end = %d, declared %d", algebra.KeyEnd, spec.KeyEnd)
	}
	if algebra.Widen.Width != 2 || algebra.Narrow.Width != 3 {
		t.Fatalf("projected measure widths = widen %d narrow %d, declared 2 and 3", algebra.Widen.Width, algebra.Narrow.Width)
	}
	if got := algebra.Widen.At(1, 7, 1); got != 8 {
		t.Fatalf("projected widen measure = %d, the declared measure answers 8", got)
	}
	if got := algebra.Narrow.At(1, 7, 1); got != 71 {
		t.Fatalf("projected narrow measure = %d, the declared measure answers 71", got)
	}
}

// TestAFactorWithNoNarrowingProjectsTheAbsentRank is the nearest negative, and
// it is the reason an owner need not spell the measure out to leave it off: an
// axis that declares no narrowing carries the absent rank, which is the same
// value whether the projection writes it or the owner omits it.
func TestAFactorWithNoNarrowingProjectsTheAbsentRank(t *testing.T) {
	spec := hotUintFactorSpec()
	spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	algebra, ok := spec.AxisAlgebra()
	if !ok {
		t.Fatal("a factor spec declaring no narrowing projects no axis algebra")
	}
	if !algebra.Narrow.Absent() {
		t.Fatalf("a factor spec declaring no narrowing projects narrow width %d", algebra.Narrow.Width)
	}
	if !algebra.Available() {
		t.Fatal("an algebra with the absent narrow rank is unavailable")
	}
}

// TestAnAxisAlgebraRefusesAFactorItCannotAnswerFor states the projection is a
// projection and not a repair: a spec missing the membership it is admitted
// under produces no algebra rather than one that admits everything.
func TestAnAxisAlgebraRefusesAFactorItCannotAnswerFor(t *testing.T) {
	spec := hotUintFactorSpec()
	spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	spec.AdmitAt = nil
	if _, ok := spec.AxisAlgebra(); ok {
		t.Fatal("a factor spec with no admission projected an axis algebra")
	}
}
