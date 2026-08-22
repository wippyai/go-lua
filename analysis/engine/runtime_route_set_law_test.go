package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

type routeSetLawRef uint64

func (ref routeSetLawRef) factorRow() schemaFactorBinding { return nil }
func (ref routeSetLawRef) rawAddress() uint64             { return uint64(ref) }

type routeSetLawFactor struct {
	unit carrier.Unit
	row  schemaFactorBinding
	zero uint64
	top  uint64
}

func (factor routeSetLawFactor) stagedUnit(ref exactRef) (carrier.Unit, bool) {
	return factor.unit, ref != nil && ref.rawAddress() == 1
}

func (routeSetLawFactor) stagedObserve(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[uint64], support.Mask) bool) bool {
	return false
}

func (routeSetLawFactor) stagedSlot() (shape.Slot, bool) { return 0, false }

func (factor routeSetLawFactor) stagedRow() schemaFactorBinding { return factor.row }

func (factor routeSetLawFactor) stagedDefault() (uint64, bool) { return factor.zero, true }

func (factor routeSetLawFactor) stagedTop() (uint64, bool) { return factor.top, true }

func routeSetLawUnit(t testing.TB) carrier.Unit {
	t.Helper()
	issuer, issued := carrier.NewIssuer()
	if !issued {
		t.Fatal("carrier issuer")
	}
	first, firstOK := issuer.IssueUnit(carrier.ExactUnit, 1, 1)
	if !firstOK {
		t.Fatal("carrier units")
	}
	return first
}

func TestRouteSetCanonicalizationIsIdempotentAndSequenceRemainsStrict(t *testing.T) {
	first := routeSetLawUnit(t)
	routes := []stagedRoute[uint64]{
		{unit: first, tag: 7},
		{unit: first, tag: 7},
		{unit: first, tag: 8},
	}

	strict := &typedStagedSelectionSession[uint64, uint64, uint64]{}
	if _, _, _, ok := strict.indexRoutes(append([]stagedRoute[uint64](nil), routes...), false); ok {
		t.Fatal("sequence route admission accepted a duplicate semantic route")
	}

	set := &typedStagedSelectionSession[uint64, uint64, uint64]{}
	canonical, units, indices, ok := set.indexRoutes(routes, true)
	if !ok || len(canonical) != 2 || len(units) != 1 || len(indices) != len(canonical) {
		t.Fatalf("set canonicalization: routes=%d units=%d indices=%d ok=%t", len(canonical), len(units), len(indices), ok)
	}
	if !canonical[0].unit.Same(first) || canonical[0].tag != 7 ||
		!canonical[1].unit.Same(first) || canonical[1].tag != 8 {
		t.Fatalf("noncanonical route-set order: %#v", canonical)
	}
	if indices[0] != 0 || indices[1] != 0 {
		t.Fatalf("route-set unit partition = %v", indices)
	}
}

func TestRouteSetSinkRejectsMixedModesAndForeignRefs(t *testing.T) {
	unit := routeSetLawUnit(t)
	target := routeSetLawFactor{unit: unit}
	set := stagedRouteSink[uint64, uint64]{target: target}
	if !set.accept(emittedRoute[uint64]{ref: routeSetLawRef(1), tag: 7, set: true}) {
		t.Fatal("route-set member refused")
	}
	if set.accept(emittedRoute[uint64]{ref: routeSetLawRef(1), tag: 8}) {
		t.Fatal("route-set selector accepted sequence emission")
	}
	if set.accept(emittedRoute[uint64]{ref: routeSetLawRef(2), tag: 9, set: true}) {
		t.Fatal("route-set selector accepted foreign exact ref")
	}

	sequence := stagedRouteSink[uint64, uint64]{target: target}
	if !sequence.accept(emittedRoute[uint64]{ref: routeSetLawRef(1), tag: 7}) {
		t.Fatal("sequence member refused")
	}
	if sequence.accept(emittedRoute[uint64]{ref: routeSetLawRef(1), tag: 7, set: true}) {
		t.Fatal("sequence selector accepted route-set emission")
	}
}

func TestRouteSetSessionRejectsModeChangesAcrossSourceRows(t *testing.T) {
	sequence := &typedStagedSelectionSession[uint64, uint64, uint64]{}
	if !sequence.acceptRouteMode(stagedRouteSequence) || !sequence.acceptRouteMode(stagedRouteSequence) || sequence.acceptRouteMode(stagedRouteSet) {
		t.Fatal("selection session did not retain sequence mode across rows")
	}
	set := &typedStagedSelectionSession[uint64, uint64, uint64]{}
	if !set.acceptRouteMode(stagedRouteSet) || !set.acceptRouteMode(stagedRouteSet) || set.acceptRouteMode(stagedRouteSequence) {
		t.Fatal("selection session did not retain route-set mode across rows")
	}
}

func BenchmarkRouteSetCanonicalization1024(b *testing.B) {
	unit := routeSetLawUnit(b)
	template := make([]stagedRoute[uint64], 1024)
	for index := range template {
		template[index] = stagedRoute[uint64]{unit: unit, tag: uint64(index/2) + 1}
	}
	scratch := make([]stagedRoute[uint64], len(template))
	session := &typedStagedSelectionSession[uint64, uint64, uint64]{
		routeIndices: make([]int, len(template)),
		routeUnits:   make([]carrier.Unit, 0, 1),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		copy(scratch, template)
		routes, units, indices, ok := session.indexRoutes(scratch, true)
		if !ok || len(routes) != len(template)/2 || len(units) != 1 || len(indices) != len(routes) {
			b.Fatal("route-set canonicalization")
		}
	}
}
