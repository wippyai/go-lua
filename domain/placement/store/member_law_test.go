package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

var (
	_ func(Route) (heap.Key, heap.Key, bool)                                                                                    = Route.Coordinates
	_ func(Route) (uint64, bool)                                                                                                = Route.Predicate
	_ func(valuedomain.StorageTransfer, valuedomain.Value, uint64, placement.Fact) (placement.Fact, structure.ReductionOutcome) = StorageFold
)

func TestRouteAccessorsRefuseUnissuedRows(t *testing.T) {
	var route Route
	if _, _, ok := route.Coordinates(); ok {
		t.Fatal("zero route coordinates admitted")
	}
	if _, ok := route.Predicate(); ok {
		t.Fatal("zero route predicate admitted")
	}
}

func TestStorageFoldRefusesUnauthenticatedEvidence(t *testing.T) {
	var candidate valuedomain.StorageTransfer
	if _, outcome := StorageFold(candidate, valuedomain.Value{}, 1, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("invalid candidate or source admitted")
	}
	if _, outcome := StorageFold(candidate, valuedomain.Value{}, 1, placement.BottomFact()); outcome != structure.Refuse {
		t.Fatal("invalid selected Placement fact admitted")
	}
}
