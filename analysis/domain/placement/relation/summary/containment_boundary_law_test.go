package summary_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

type heapRootRows struct {
	values  []heapdomain.Value
	present []bool
}

func (rows heapRootRows) Len() int { return len(rows.values) }

func (rows heapRootRows) At(index int) (heapdomain.Value, bool, bool) {
	if index < 0 || index >= len(rows.values) || len(rows.present) != len(rows.values) {
		return heapdomain.Value{}, false, false
	}
	return rows.values[index], rows.present[index], true
}

func TestContainmentEvidenceConsumesOneCompleteHeapSpanWithoutMetadataReconstruction(t *testing.T) {
	fixture := relationfixture.New(t)
	placement, ok := placementdomain.NewSchema(fixture.Heap)
	if !ok {
		t.Fatal("Placement schema")
	}
	rows := heapRootRows{
		values:  make([]heapdomain.Value, fixture.Heap.KeyCount()),
		present: make([]bool, fixture.Heap.KeyCount()),
	}
	for index := range rows.values {
		rows.values[index] = fixture.Heap.Bottom()
	}

	projection, ok := placementdomain.DeriveContainmentEvidence(placement, rows)
	if !ok {
		t.Fatal("complete sparse Heap projection")
	}
	for index := 0; index < placement.KeyCount(); index++ {
		key, keyOK := placement.KeyAt(index)
		evidence, evidenceOK := projection.Evidence(key)
		if !keyOK || !evidenceOK || !evidence.Valid() {
			t.Fatalf("allocation evidence %d = %#v/%v", index, evidence, evidenceOK)
		}
		if evidence.HasOwnerIdentity || evidence.HasKind || evidence.HasClass {
			t.Fatalf("containment projection reconstructed foreign metadata: %#v", evidence)
		}
		if !evidence.HasDepth || evidence.Depth != 0 || evidence.DeepFrozen != placementdomain.EvidenceProven {
			t.Fatalf("sparse allocation evidence %d = %#v, want depth zero and deep-frozen", index, evidence)
		}
	}
	if fixture.Heap.BootCount() > 0 {
		boot, bootOK := fixture.Heap.BootRootAt(0)
		if !bootOK {
			t.Fatal("Boot root")
		}
		if evidence, accepted := projection.Evidence(boot); accepted || evidence.Valid() && evidence != (placementdomain.AllocationEvidence{}) {
			t.Fatalf("Boot root became public allocation evidence: %#v/%v", evidence, accepted)
		}
	}
}
