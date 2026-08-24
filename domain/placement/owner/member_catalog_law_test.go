package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// placementCatalogInputs is intentionally only the declaration shape. This
// law proves AxisEntry publishes its member vocabulary before any mounted
// authority exists.
type placementCatalogInputs struct{}

func (placementCatalogInputs) HeapInput() heap.Schema           { return heap.Schema{} }
func (placementCatalogInputs) PlacementInput() placement.Schema { return placement.Schema{} }

func TestAxisEntryPublishesPlacementMemberCatalog(t *testing.T) {
	spec := AxisEntry[placementCatalogInputs]()
	if !spec.Catalog.Available() || spec.Signature.Key != placement.PlacementKeyCarrier || spec.Signature.Fact != placement.PlacementFactCarrier {
		t.Fatalf("placement member catalog/signature missing: catalog=%+v signature=%+v", spec.Catalog, spec.Signature)
	}
	if relation, ok := spec.Catalog.Relation(placement.StorageRoutes); !ok || relation.Subject != placement.StorageRouteCarrier {
		t.Fatalf("placement Store relation missing from axis catalog: relation=%+v/%t", relation, ok)
	}
	if _, ok := axis.New(spec); !ok {
		t.Fatal("placement axis declaration rejected with its member catalog")
	}
}
