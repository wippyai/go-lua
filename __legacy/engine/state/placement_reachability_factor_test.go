package state

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPlacementReachabilityFactorComputesExactFiniteClosure(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	outer := identity.ID{Kind: "test.object", Site: "placement", Index: 1}
	inner := identity.ID{Kind: "test.object", Site: "placement", Index: 2}
	unrelated := identity.ID{Kind: "test.object", Site: "placement", Index: 3}
	member, ok := heapidentity.StaticMemberSuffixKey(keys, []segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("member key")
	}
	outerValue, innerValue := identityvalue.Present(reg, outer), identityvalue.Present(reg, inner)
	input := Reachable(State{}).
		WriteHeapTableObject(reg, outer, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: outerValue, StaticMembers: map[keyspace.Key]product.Value{member: innerValue}})).
		WriteHeapTableObject(reg, inner, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: innerValue})).
		WriteHeapTableObject(reg, unrelated, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, unrelated)})).
		WritePlacement(unrelated, placement.Stack)
	domain := RegisteredProductDomain(reg)
	plan, err := domain.PreparePlacementReachabilityPlan(keys, []product.Value{outerValue}, placement.SharedHeap)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyPlacementReachability(context.Background(), plan, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadPlacement(outer) != placement.SharedHeap || got.ReadPlacement(inner) != placement.SharedHeap || got.ReadPlacement(unrelated) != placement.Stack {
		t.Fatalf("placements = outer:%v inner:%v unrelated:%v", got.ReadPlacement(outer), got.ReadPlacement(inner), got.ReadPlacement(unrelated))
	}
}

func TestPlacementReachabilityCancellationPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	id := identity.ID{Kind: "test.object", Site: "cancel", Index: 1}
	value := identityvalue.Present(reg, id)
	input := Reachable(State{}).WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value}))
	domain := RegisteredProductDomain(reg)
	plan, err := domain.PreparePlacementReachabilityPlan(keys, []product.Value{value}, placement.SharedHeap)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := domain.ApplyPlacementReachability(ctx, plan, input)
	if !errors.Is(err, context.Canceled) || !domain.Lattice().Equal(got, input) {
		t.Fatalf("canceled placement = err:%v equal:%t", err, domain.Lattice().Equal(got, input))
	}
}
