package result

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestDetachedPointIdentityRetainsContext proves that two canonical
// publications sharing a mounted point do not collapse into one detached
// geometry row when their execution contexts differ.
func TestDetachedPointIdentityRetainsContext(t *testing.T) {
	tables := artifactResultLawBaseTables(t)
	firstContext := artifactResultLawID(20)
	secondContext := artifactResultLawID(21)
	tables.points = []resultPoint{
		{context: firstContext, mount: tables.points[0].mount, point: tables.points[0].point, bodies: []uint32{1}},
		{context: secondContext, mount: tables.points[0].mount, point: tables.points[0].point, bodies: []uint32{1}},
	}
	first := tables.families[0].queries[0]
	first.point = 1
	second := first
	second.site = artifactResultLawID(14)
	second.key = artifactResultLawID(15)
	second.point = 2
	tables.families[0].queries = []resultQuery{first, second}

	result := artifactResultLawResult(t, tables)
	if len(result.points) != 2 {
		t.Fatalf("detached point rows = %d, want two context-qualified rows", len(result.points))
	}
	family, familyOK := result.FamilyAt(0)
	if !familyOK || family.QueryCount() != 2 {
		t.Fatalf("context-qualified family = %t/%d, want true/2", familyOK, family.QueryCount())
	}
	for index, want := range []struct {
		context, mount, point identity.ContentID
	}{{firstContext, tables.points[0].mount, tables.points[0].point}, {secondContext, tables.points[1].mount, tables.points[1].point}} {
		query, queryOK := family.QueryAt(index)
		context, contextOK := query.ContextID()
		mount, mountOK := query.MountID()
		point, pointOK := query.PointID()
		if !queryOK || !contextOK || !mountOK || !pointOK || context != want.context || mount != want.mount || point != want.point {
			t.Fatalf("query[%d] geometry = %v/%t %v/%t %v/%t, want context %v and shared mount/point", index, context, contextOK, mount, mountOK, point, pointOK, want.context)
		}
	}
}
