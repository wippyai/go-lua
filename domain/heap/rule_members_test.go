package heap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// TestAxisMemberCatalogPublishesTheFreezeRouteSelection states the end of the
// selection lane on a real axis: the freeze routes are computed from the
// actuals the reads before them delivered, so Heap publishes them through a
// declared operation and stamps them with the tag a reading rule joins on.
func TestAxisMemberCatalogPublishesTheFreezeRouteSelection(t *testing.T) {
	catalog := AxisMemberCatalog()
	selection, found := catalog.Selection(FormalFreezeRouteSelection)
	if !found {
		t.Fatal("the heap catalog publishes no selection for the freeze routes")
	}
	if selection.Relation != FormalFreezeRoutes {
		t.Fatalf("selection relation = %q, want the freeze route rows", selection.Relation)
	}
	if selection.Tag != FormalFreezeRouteTag {
		t.Fatalf("selection tag = %q, want the tag a reading rule correlates by", selection.Tag)
	}
	relation, relationOK := catalog.Relation(selection.Relation)
	if !relationOK {
		t.Fatal("the selection publishes into a relation the catalog does not declare")
	}
	tag, tagOK := catalog.Projection(selection.Tag)
	if !tagOK || tag.Relation != relation.Key || tag.Role != member.Predicate {
		t.Fatal("the tag is not the selection projection of the rows it stamps")
	}
}
