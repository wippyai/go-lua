package callpayload

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func TestCallOutcomeRekeyHeapTableObjectsRejectsForeignKeyTransactionally(t *testing.T) {
	from, foreign, to := keyspace.New(), keyspace.New(), keyspace.New()
	foreignKey, ok := foreign.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("foreign suffix failed")
	}
	id := identity.ID{Kind: "table", Site: "foreign", Index: 1}
	original := CallOutcome{HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{foreignKey: product.Top()},
		}),
	}}

	got, err := original.RekeyHeapTableObjects(from, to)
	if err == nil {
		t.Fatal("foreign nested key was accepted")
	}
	if members := got.HeapTableObjects[id].StaticMembers(); len(members) != 1 {
		t.Fatalf("failed import partially erased members: %#v", members)
	}
}

func TestCallOutcomeRekeyHeapTableObjectsAllowsNilForKeyFreeObject(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "key-free", Index: 1}
	original := CallOutcome{HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
	}}
	got, err := original.RekeyHeapTableObjects(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.HeapTableObjects) != 1 {
		t.Fatalf("key-free object count = %d, want 1", len(got.HeapTableObjects))
	}
}
