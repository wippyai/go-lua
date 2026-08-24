package oracle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

// TestMountedLibraryAllocationRetainsPointSpecificPlacement proves the claim
// made by the divergent-consumer fixture at its exact identity boundary. The
// reusable lib Program and its allocation occurrence remain one mounted Heap
// root, while distinct selected points may publish different Placement states
// for that root. Aggregate class counts from unrelated module allocations are
// deliberately insufficient evidence for this law.
func TestMountedLibraryAllocationRetainsPointSpecificPlacement(t *testing.T) {
	run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, "transitive-libs/shared-lib-divergent-consumers"), corpusHarnessDiagnosticMode())
	if err != nil {
		t.Fatalf("divergent mounted library failed at %s: %v", class, err)
	}
	if run == nil || run.linked == nil || !run.placementSchema.Valid() {
		t.Fatal("divergent mounted library produced no Link-bound Placement authority")
	}

	mounts := run.linked.Project().Mounts()
	var libMount identity.ContentID
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		name, nameOK := mounts.Name(shard)
		module, moduleOK := run.linked.Project().ModuleKey(shard)
		if !shardOK || !nameOK || !moduleOK {
			t.Fatalf("mounted project row %d is unreadable", index)
		}
		if name != "lib" {
			continue
		}
		if libMount.Available() {
			t.Fatal("divergent fixture mounted lib more than once")
		}
		libMount = module
	}
	if !libMount.Available() {
		t.Fatal("divergent fixture has no canonical lib mount")
	}

	observation := corpusPlacementProjection(run.result, run.placementSchema)
	if defects := corpusPlacementObservationOperationalDefects(observation); len(defects) != 0 {
		t.Fatalf("divergent Placement publication defects: %v", defects)
	}
	family, familyOK := placementpublication.Open(run.result)
	if !familyOK {
		t.Fatal("divergent Placement family is unavailable")
	}

	heapSchema := run.placementSchema.Heap()
	for allocationID, positions := range observation.positions {
		key, keyOK := heapSchema.KeyForID(allocationID)
		module, _, _, kind, _, originOK := heapSchema.AllocationOriginForKey(key)
		if !keyOK || !originOK || module != libMount || kind != heapdomain.AllocationTable {
			continue
		}
		facts := make(map[placementdomain.Fact]struct{})
		points := make(map[struct {
			mount identity.ContentID
			point identity.ContentID
		}]struct{})
		for _, position := range positions {
			if !position.present {
				continue
			}
			query, queryOK := family.QueryAt(position.query)
			point, pointOK := query.PointID()
			mount, mountOK := query.MountID()
			if !queryOK || !pointOK || !mountOK {
				continue
			}
			facts[position.fact] = struct{}{}
			points[struct {
				mount identity.ContentID
				point identity.ContentID
			}{mount: mount, point: point}] = struct{}{}
		}
		_, owned := facts[placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}]
		_, shared := facts[placementdomain.Fact{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven}]
		if owned && shared && len(points) >= 2 {
			return
		}
	}
	t.Fatal("no exact lib table allocation published OwnedHeap and SharedHeap at distinct selected points")
}
