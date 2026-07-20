package transformer

import (
	"errors"
	"testing"
)

func TestFormalFiberDirectoryZeroFiberAndOwnershipValidation(t *testing.T) {
	empty, err := newFormalFiberDirectoryArena(0)
	if err != nil {
		t.Fatal(err)
	}
	root := empty.defaultRoot()
	if root.owner != empty || root.ref != 0 || empty.fiberCount() != 0 || empty.updateDepth() != 0 || empty.nodeCount() != 0 {
		t.Fatalf("empty directory = owner %p ref %d fibers %d depth %d nodes %d", root.owner, root.ref, empty.fiberCount(), empty.updateDepth(), empty.nodeCount())
	}
	if _, _, err := empty.update(root, 0, 1); err == nil {
		t.Fatal("zero-fiber directory accepted a point update")
	}
	delta, err := empty.sealDelta(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, stats, err := empty.applyDelta(root, delta)
	if err != nil || got != root || stats.NodesAdded != 0 {
		t.Fatalf("empty delta = root %+v stats %+v err %v", got, stats, err)
	}
	if _, _, err := empty.update(formalFiberDirectoryRoot{}, 0, 1); err == nil {
		t.Fatal("directory accepted an unowned root")
	}
	if _, err := newFormalFiberDirectoryArena(-1); err == nil {
		t.Fatal("negative fiber inventory was accepted")
	}
}

func TestFormalFiberDirectoryPointUpdateReusesIdentityAndDepth(t *testing.T) {
	arena, err := newFormalFiberDirectoryArena(9)
	if err != nil {
		t.Fatal(err)
	}
	root := arena.defaultRoot()
	updated, stats, err := arena.update(root, 7, 41)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NodesAdded <= 0 || stats.NodesAdded > arena.updateDepth() {
		t.Fatalf("one update added %d nodes, depth %d", stats.NodesAdded, arena.updateDepth())
	}
	if value, err := arena.valueAt(updated, 7); err != nil || value != 41 {
		t.Fatalf("updated value = %d, err %v", value, err)
	}
	before := arena.nodeCount()
	again, sameStats, err := arena.update(updated, 7, 41)
	if err != nil {
		t.Fatal(err)
	}
	if again != updated || sameStats.NodesAdded != 0 || arena.nodeCount() != before {
		t.Fatalf("unchanged update lost identity: roots %+v/%+v stats %+v nodes %d/%d", updated, again, sameStats, before, arena.nodeCount())
	}
	cleared, clearStats, err := arena.update(updated, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cleared != root || clearStats.NodesAdded != 0 {
		t.Fatalf("clear did not recover canonical default root: got %+v want %+v stats %+v", cleared, root, clearStats)
	}
	t.Logf("point update: depth=%d first_nodes=%d retained_nodes=%d", arena.updateDepth(), stats.NodesAdded, arena.nodeCount())
}

func TestFormalFiberDirectoryDeltaIsSortedCanonicalAndExact(t *testing.T) {
	arena, err := newFormalFiberDirectoryArena(17)
	if err != nil {
		t.Fatal(err)
	}
	writesA := []formalFiberWrite{{ordinal: 16, value: 160}, {ordinal: 2, value: 20}, {ordinal: 9, value: 90}, {ordinal: 0, value: 10}}
	writesB := []formalFiberWrite{{ordinal: 9, value: 90}, {ordinal: 0, value: 10}, {ordinal: 16, value: 160}, {ordinal: 2, value: 20}}
	deltaA, err := arena.sealDelta(writesA)
	if err != nil {
		t.Fatal(err)
	}
	deltaB, err := arena.sealDelta(writesB)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltaA.writes) != len(deltaB.writes) {
		t.Fatalf("canonical delta widths = %d/%d", len(deltaA.writes), len(deltaB.writes))
	}
	for index := range deltaA.writes {
		if deltaA.writes[index] != deltaB.writes[index] || index != 0 && deltaA.writes[index-1].ordinal >= deltaA.writes[index].ordinal {
			t.Fatalf("canonical delta mismatch at %d: %+v / %+v", index, deltaA.writes, deltaB.writes)
		}
	}
	rootA, statsA, err := arena.applyDelta(arena.defaultRoot(), deltaA)
	if err != nil {
		t.Fatal(err)
	}
	rootB, statsB, err := arena.applyDelta(arena.defaultRoot(), deltaB)
	if err != nil {
		t.Fatal(err)
	}
	if rootA != rootB || statsB.NodesAdded != 0 {
		t.Fatalf("permuted delta roots = %+v/%+v stats A=%+v B=%+v", rootA, rootB, statsA, statsB)
	}
	for _, write := range deltaA.writes {
		value, valueErr := arena.valueAt(rootA, write.ordinal)
		if valueErr != nil || value != write.value {
			t.Fatalf("fiber %d = %d, want %d, err %v", write.ordinal, value, write.value, valueErr)
		}
	}
	if value, err := arena.valueAt(rootA, 8); err != nil || value != 0 {
		t.Fatalf("untouched fiber = %d, err %v", value, err)
	}
	if _, err := arena.sealDelta([]formalFiberWrite{{ordinal: 3, value: 1}, {ordinal: 3, value: 2}}); err == nil {
		t.Fatal("duplicate delta ordinal was accepted")
	}
	forged := formalFiberDelta{owner: arena, writes: []formalFiberWrite{{ordinal: 9, value: 1}, {ordinal: 2, value: 2}}}
	if _, _, err := arena.applyDelta(rootA, forged); err == nil {
		t.Fatal("non-canonical forged delta was accepted")
	}
	t.Logf("four-write delta: depth=%d nodes=%d retained_nodes=%d", arena.updateDepth(), statsA.NodesAdded, arena.nodeCount())
}

func TestFormalFiberDirectoryZipSkipsEqualSubtrees(t *testing.T) {
	arena, err := newFormalFiberDirectoryArena(16)
	if err != nil {
		t.Fatal(err)
	}
	common, _, err := arena.update(arena.defaultRoot(), 1, 11)
	if err != nil {
		t.Fatal(err)
	}
	left, _, err := arena.update(common, 13, 20)
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := arena.update(common, 13, 22)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	combined, stats, err := arena.zip(left, right, func(ordinal formalFiberOrdinal, left, right formalFiberValue) (formalFiberValue, error) {
		calls++
		if ordinal != 13 {
			return 0, errors.New("zip visited an equal leaf")
		}
		return left + right, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || stats.LeafCalls != 1 || stats.EqualSubtrees == 0 {
		t.Fatalf("zip calls=%d stats=%+v", calls, stats)
	}
	if value, err := arena.valueAt(combined, 1); err != nil || value != 11 {
		t.Fatalf("shared subtree value = %d, err %v", value, err)
	}
	if value, err := arena.valueAt(combined, 13); err != nil || value != 42 {
		t.Fatalf("combined value = %d, err %v", value, err)
	}
	identical, identicalStats, err := arena.zip(combined, combined, func(formalFiberOrdinal, formalFiberValue, formalFiberValue) (formalFiberValue, error) {
		t.Fatal("identical zip invoked leaf callback")
		return 0, nil
	})
	if err != nil || identical != combined || identicalStats.LeafCalls != 0 || identicalStats.NodesAdded != 0 {
		t.Fatalf("identical zip = root %+v stats %+v err %v", identical, identicalStats, err)
	}
	t.Logf("zip: leaves=%d equal_subtrees=%d new_nodes=%d", stats.LeafCalls, stats.EqualSubtrees, stats.NodesAdded)
}

func TestFormalFiberDirectoryRejectsCrossArenaRootsAndDeltas(t *testing.T) {
	leftArena, err := newFormalFiberDirectoryArena(4)
	if err != nil {
		t.Fatal(err)
	}
	rightArena, err := newFormalFiberDirectoryArena(4)
	if err != nil {
		t.Fatal(err)
	}
	leftRoot, _, err := leftArena.update(leftArena.defaultRoot(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, _, err := rightArena.update(rightArena.defaultRoot(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := leftArena.update(rightRoot, 1, 8); err == nil {
		t.Fatal("point update accepted a foreign root")
	}
	if _, _, err := leftArena.zip(leftRoot, rightRoot, func(_ formalFiberOrdinal, left, _ formalFiberValue) (formalFiberValue, error) { return left, nil }); err == nil {
		t.Fatal("zip accepted roots from independent arenas")
	}
	delta, err := rightArena.sealDelta([]formalFiberWrite{{ordinal: 2, value: 9}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := leftArena.applyDelta(leftRoot, delta); err == nil {
		t.Fatal("directory accepted a foreign delta")
	}
}
