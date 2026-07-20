package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCallOutcomeAxisFactorsMatchConcreteLaneLaws(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	id := identity.ID{Kind: "table", Site: "call-outcome-factor", Index: 1}

	initialObject := heapidentity.TopObject().WithRoot(product.Absent(reg))
	returnedObject := heapidentity.TopObject().WithRoot(product.Top())
	heapLane, _ := domain.ProductLane(LaneHeapTableIdentity)
	heapFactors, err := domain.DecomposeLanes(
		domain.Lattice().Bottom().WriteHeapTableObject(reg, id, initialObject),
		[]ProductLane{heapLane},
	)
	if err != nil {
		t.Fatal(err)
	}
	heapFactor, err := domain.ApplyCallOutcomeHeapObjectFactor(heapFactors[0], id, returnedObject, true)
	if err != nil {
		t.Fatal(err)
	}
	wantHeap := domain.Lattice().Bottom().WriteHeapTableObject(
		reg, id, heapidentity.ObjectDomain(reg).Join(initialObject, returnedObject),
	)
	wantHeapFactors, _ := domain.DecomposeLanes(wantHeap, []ProductLane{heapLane})
	if equal, equalErr := domain.LaneEqual(heapFactor, wantHeapFactors[0]); equalErr != nil || !equal {
		t.Fatalf("heap factor equal=%t err=%v", equal, equalErr)
	}

	placementLane, _ := domain.ProductLane(LanePlacement)
	placementFactors, err := domain.DecomposeLanes(
		domain.Lattice().Bottom().WritePlacement(id, placement.Stack),
		[]ProductLane{placementLane},
	)
	if err != nil {
		t.Fatal(err)
	}
	placementFactor, err := domain.ApplyCallOutcomePlacementFactor(placementFactors[0], id, placement.SharedHeap)
	if err != nil {
		t.Fatal(err)
	}
	wantPlacement, _ := domain.DecomposeLanes(
		domain.Lattice().Bottom().WritePlacement(id, placement.Join(placement.Stack, placement.SharedHeap)),
		[]ProductLane{placementLane},
	)
	if equal, equalErr := domain.LaneEqual(placementFactor, wantPlacement[0]); equalErr != nil || !equal {
		t.Fatalf("placement factor equal=%t err=%v", equal, equalErr)
	}

	resource := typestate.Resource{ID: "call-outcome-resource", Protocol: "file"}
	caller := typestate.Store{}.Acquire(resource, "open", typestate.Obligation{Final: "closed"})
	normal := typestate.Store{}.Acquire(resource, "closed", typestate.Obligation{Final: "closed"})
	exceptional := typestate.Store{}.Acquire(resource, "failed", typestate.Obligation{Final: "closed"})
	typestateLane, _ := domain.ProductLane(LaneTypestates)
	typestateFactors, err := domain.DecomposeLanes(
		domain.Lattice().Bottom().WithTypestateSnapshot(caller),
		[]ProductLane{typestateLane},
	)
	if err != nil {
		t.Fatal(err)
	}
	typestateFactor, err := domain.ApplyProtectedCallTypestateFactor(typestateFactors[0], normal, true, exceptional, true)
	if err != nil {
		t.Fatal(err)
	}
	wantTypestate := typestate.Join(caller.Overlay(normal), caller.Overlay(exceptional))
	wantTypestateFactors, _ := domain.DecomposeLanes(
		domain.Lattice().Bottom().WithTypestateSnapshot(wantTypestate),
		[]ProductLane{typestateLane},
	)
	if equal, equalErr := domain.LaneEqual(typestateFactor, wantTypestateFactors[0]); equalErr != nil || !equal {
		t.Fatalf("typestate factor equal=%t err=%v", equal, equalErr)
	}
}

func TestCallOutcomeDynamicIndexBatchAndCandidateObservationMatchConcrete(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	container := keys.FromPath(pathdom.NewPath(symbol.ID(91), "table"))
	if container.Kind == keyspace.KindInvalid {
		t.Fatal("container key is invalid")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	first := dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})
	second := dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: product.Top(), HasValue: true, Admission: dynamicindex.AdmissionAdmitted})
	mutations := []CallOutcomeDynamicIndexMutation{
		{Key: dynamicindex.Key{Table: container, Site: "z-second"}, Fact: second},
		{Key: dynamicindex.Key{Table: container, Site: "a-first"}, Fact: first},
	}
	lane, _ := domain.ProductLane(LaneDynamicIndex)
	input := domain.Lattice().Bottom()
	factors, err := domain.DecomposeLanes(input, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyCallOutcomeDynamicIndexFactors(factors[0], mutations)
	if err != nil {
		t.Fatal(err)
	}
	wantState := input
	for _, mutation := range mutations {
		wantState = wantState.WriteDynamicIndexFact(reg, mutation.Key, mutation.Fact)
	}
	want, _ := domain.DecomposeLanes(wantState, []ProductLane{lane})
	if equal, equalErr := domain.LaneEqual(got, want[0]); equalErr != nil || !equal {
		t.Fatalf("dynamic batch differs from concrete: equal=%t err=%v", equal, equalErr)
	}
	observed, err := domain.ObserveCallOutcomeDynamicIndexFactors(got, container)
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 || observed[0].Key.Site != "a-first" || observed[1].Key.Site != "z-second" {
		t.Fatalf("candidate observation = %#v, want stable site order", observed)
	}
}
