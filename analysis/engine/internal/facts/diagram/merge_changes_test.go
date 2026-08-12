package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type soleChangeRow struct {
	key testKey
}

func newSoleBatchFixture(t testing.TB) diagramFixture {
	t.Helper()
	fixture := newDiagramFixture(t)
	facts, ok := New(Config[testFactor, testKey, uint8]{
		Factors:   []testFactor{factorFirst},
		Terminals: fixture.diagram.Terminals(),
		Guards:    fixture.manager,
	})
	if !ok {
		t.Fatal("sole-factor diagram")
	}
	fixture.diagram = facts
	return fixture
}

func soleAll(t testing.TB, fixture diagramFixture) support.Mask {
	t.Helper()
	all, ok := support.FromGuard(fixture.manager, fixture.manager.True())
	if !ok {
		t.Fatal("whole support")
	}
	return all
}

func soleRoot(t testing.TB, fixture diagramFixture, entries map[testKey]terminal.ID[uint8]) Root[testFactor, testKey, uint8] {
	t.Helper()
	builder := fixture.diagram.Begin()
	if builder == nil {
		t.Fatal("builder")
	}
	root := fixture.diagram.Empty()
	all := soleAll(t, fixture)
	for key := testKey(1); key <= 31; key++ {
		value, present := entries[key]
		if !present {
			continue
		}
		var ok bool
		root, ok = builder.Set(root, factorFirst, key, all, value)
		if !ok {
			t.Fatalf("set key %d", key)
		}
	}
	root, ok := builder.Seal(root)
	if !ok {
		t.Fatal("seal")
	}
	return root
}

func assertKeyAVL(t testing.TB, node *keyNode[testKey, uint8]) (height int8, count int, low, high testKey) {
	t.Helper()
	if node == nil {
		return 0, 0, 0, 0
	}
	leftHeight, leftCount, leftLow, leftHigh := assertKeyAVL(t, node.left)
	rightHeight, rightCount, rightLow, rightHigh := assertKeyAVL(t, node.right)
	if leftCount != 0 && leftHigh >= node.key {
		t.Fatalf("left key %d is not below %d", leftHigh, node.key)
	}
	if rightCount != 0 && rightLow <= node.key {
		t.Fatalf("right key %d is not above %d", rightLow, node.key)
	}
	delta := int(leftHeight) - int(rightHeight)
	if delta < -1 || delta > 1 {
		t.Fatalf("key %d has AVL balance %d", node.key, delta)
	}
	wantHeight := leftHeight
	if rightHeight > wantHeight {
		wantHeight = rightHeight
	}
	wantHeight++
	if node.height != wantHeight {
		t.Fatalf("key %d height = %d, want %d", node.key, node.height, wantHeight)
	}
	low, high = node.key, node.key
	if leftCount != 0 {
		low = leftLow
	}
	if rightCount != 0 {
		high = rightHigh
	}
	return wantHeight, leftCount + rightCount + 1, low, high
}

func TestMergeSoleFactorChangesMaintainsAVLAfterDeletingCompleteHalf(t *testing.T) {
	fixture := newSoleBatchFixture(t)
	entries := make(map[testKey]terminal.ID[uint8], 15)
	for key := testKey(1); key <= 15; key++ {
		entries[key] = fixture.values[0]
	}
	left := soleRoot(t, fixture, entries)
	rows := make([]soleChangeRow, 7)
	for index := range rows {
		rows[index].key = testKey(index + 1)
	}
	all := soleAll(t, fixture)
	regions := support.New(fixture.manager)
	builder := fixture.diagram.Begin()
	result, ok := builder.MergeSoleFactorChanges(left, fixture.diagram.Empty(), len(rows), NewSoleScratch[testKey, uint8](), regions,
		func(_ testKey, _, _ terminal.ID[uint8]) (terminal.ID[uint8], bool) {
			return terminal.ID[uint8]{}, true
		}, func(left, right terminal.ID[uint8]) bool {
			return left == right
		}, func(_ testKey, changed support.Mask) bool {
			return !support.Empty(changed)
		}, func(index int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
			return rows[index].key, all, all, all, true
		})
	if !ok || result.count != 8 || result.lease != builder.lease {
		t.Fatalf("batch result = ok:%t count:%d leased:%t", ok, result.count, result.lease == builder.lease)
	}
	factor := findFactor(result.root, fixture.diagram.ranks[factorFirst])
	if factor == nil {
		t.Fatal("batch deleted the surviving factor")
	}
	_, count, low, high := assertKeyAVL(t, factor.keys)
	if count != 8 || low != 8 || high != 15 {
		t.Fatalf("survivors = count:%d [%d,%d], want 8 [8,15]", count, low, high)
	}
	result, ok = builder.Seal(result)
	if !ok || !fixture.diagram.Valid(result) {
		t.Fatal("batch result did not publish")
	}
}

func TestMergeSoleFactorChangesMatchesScalarMixedPatchesAndCanonicalEmpty(t *testing.T) {
	fixture := newSoleBatchFixture(t)
	entries := make(map[testKey]terminal.ID[uint8], 15)
	for key := testKey(1); key <= 15; key++ {
		entries[key] = fixture.values[0]
	}
	left := soleRoot(t, fixture, entries)
	right := soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{3: fixture.values[2], 16: fixture.values[1]})
	rows := []soleChangeRow{{key: 2}, {key: 3}, {key: 4}, {key: 16}}
	all := soleAll(t, fixture)
	combine := func(key testKey, left, right terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		switch key {
		case 2:
			return terminal.ID[uint8]{}, true
		case 3, 16:
			return right, true
		case 4:
			return left, true
		default:
			return terminal.ID[uint8]{}, false
		}
	}
	equal := func(left, right terminal.ID[uint8]) bool { return left == right }

	scalarBuilder := fixture.diagram.Begin()
	scalar := left
	var scalarReported []testKey
	var scalarRegions []support.Mask
	for _, row := range rows {
		var changed support.Mask
		var ok bool
		scalar, changed, ok = scalarBuilder.MergeSoleFactorKey(scalar, right, row.key, all, all, all, NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal)
		if !ok {
			t.Fatalf("scalar key %d", row.key)
		}
		if !support.Empty(changed) {
			scalarReported = append(scalarReported, row.key)
			scalarRegions = append(scalarRegions, changed)
		}
	}
	scalar, ok := scalarBuilder.Seal(scalar)
	if !ok {
		t.Fatal("scalar seal")
	}

	batchBuilder := fixture.diagram.Begin()
	var batchReported []testKey
	var batchRegions []support.Mask
	batch, ok := batchBuilder.MergeSoleFactorChanges(left, right, len(rows), NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal,
		func(key testKey, changed support.Mask) bool {
			batchReported = append(batchReported, key)
			batchRegions = append(batchRegions, changed)
			return true
		}, func(index int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
			return rows[index].key, all, all, all, true
		})
	if !ok || batch.count != 15 || len(batchReported) != len(scalarReported) || len(batchRegions) != len(scalarRegions) {
		t.Fatalf("batch = ok:%t count:%d reports:%v, scalar reports:%v", ok, batch.count, batchReported, scalarReported)
	}
	for index := range scalarReported {
		if batchReported[index] != scalarReported[index] {
			t.Fatalf("report[%d] = %d, want %d", index, batchReported[index], scalarReported[index])
		}
		if batchRegions[index] != scalarRegions[index] {
			t.Fatalf("report region[%d] differs from scalar", index)
		}
	}
	batch, ok = batchBuilder.Seal(batch)
	if !ok || !fixture.diagram.Equal(batch, scalar) {
		t.Fatal("batch meaning differs from scalar key updates")
	}

	noopBuilder := fixture.diagram.Begin()
	noop, ok := noopBuilder.MergeSoleFactorChanges(left, right, 1, NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal,
		func(testKey, support.Mask) bool { return false },
		func(int) (testKey, support.Mask, support.Mask, support.Mask, bool) { return 4, all, all, all, true })
	if !ok || noop.root != left.root || noop.count != left.count || noop.lease != noopBuilder.lease {
		t.Fatal("no-op batch did not retain exact lhs root with candidate lease")
	}
	if _, ok = noopBuilder.Seal(noop); !ok {
		t.Fatal("no-op candidate did not seal")
	}

	deleteRows := make([]soleChangeRow, 15)
	for index := range deleteRows {
		deleteRows[index].key = testKey(index + 1)
	}
	emptyBuilder := fixture.diagram.Begin()
	empty, ok := emptyBuilder.MergeSoleFactorChanges(left, fixture.diagram.Empty(), len(deleteRows), NewSoleScratch[testKey, uint8](), support.New(fixture.manager),
		func(testKey, terminal.ID[uint8], terminal.ID[uint8]) (terminal.ID[uint8], bool) {
			return terminal.ID[uint8]{}, true
		}, equal,
		func(testKey, support.Mask) bool { return true },
		func(index int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
			return deleteRows[index].key, all, all, all, true
		})
	if !ok || empty.root != nil || empty.count != 0 {
		t.Fatalf("complete deletion = ok:%t root:%p count:%d", ok, empty.root, empty.count)
	}
	empty, ok = emptyBuilder.Seal(empty)
	if !ok || !fixture.diagram.Valid(empty) {
		t.Fatal("canonical empty result did not publish")
	}
}

func TestMergeSoleFactorChangesRejectsBadOrderAndReportFailure(t *testing.T) {
	fixture := newSoleBatchFixture(t)
	left := soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{2: fixture.values[0]})
	right := soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{2: fixture.values[1]})
	all := soleAll(t, fixture)
	combine := func(_ testKey, _, right terminal.ID[uint8]) (terminal.ID[uint8], bool) { return right, true }
	equal := func(left, right terminal.ID[uint8]) bool { return left == right }
	for _, rows := range [][]soleChangeRow{{{key: 2}, {key: 2}}, {{key: 3}, {key: 2}}} {
		builder := fixture.diagram.Begin()
		if _, ok := builder.MergeSoleFactorChanges(left, right, len(rows), NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal,
			func(testKey, support.Mask) bool { return true }, func(index int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
				return rows[index].key, all, all, all, true
			}); ok {
			t.Fatalf("accepted invalid row order: %v", rows)
		}
	}
	builder := fixture.diagram.Begin()
	if _, ok := builder.MergeSoleFactorChanges(left, right, 1, NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal,
		func(testKey, support.Mask) bool { return false }, func(int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
			return 2, all, all, all, true
		}); ok {
		t.Fatal("accepted a rejecting report callback")
	}
}

func TestTransformSoleFactorSharesShapeAndMatchesCanonicalDeletion(t *testing.T) {
	fixture := newSoleBatchFixture(t)
	entries := make(map[testKey]terminal.ID[uint8], 15)
	for key := testKey(1); key <= 15; key++ {
		entries[key] = fixture.values[0]
	}
	input := soleRoot(t, fixture, entries)

	noopBuilder := fixture.diagram.Begin()
	noop, ok := noopBuilder.TransformSoleFactor(input, func(_ testKey, value Value[uint8]) (Value[uint8], bool) {
		return value, true
	})
	if !ok || noop.root != input.root || noop.count != input.count || noop.lease != noopBuilder.lease {
		t.Fatal("identity transform did not retain the exact immutable root")
	}
	if _, ok = noopBuilder.Seal(noop); !ok {
		t.Fatal("identity transform candidate did not seal")
	}

	builder := fixture.diagram.Begin()
	visited := make([]testKey, 0, 15)
	result, ok := builder.TransformSoleFactor(input, func(key testKey, value Value[uint8]) (Value[uint8], bool) {
		visited = append(visited, key)
		switch {
		case key < 8:
			return builder.Constant(terminal.ID[uint8]{})
		case key == 15:
			return builder.Constant(fixture.values[1])
		default:
			return value, true
		}
	})
	if !ok || result.count != 8 || len(visited) != 15 {
		t.Fatalf("transform = ok:%t count:%d visited:%v", ok, result.count, visited)
	}
	for index, key := range visited {
		if key != testKey(index+1) {
			t.Fatalf("visit[%d] = %d, want ascending %d", index, key, index+1)
		}
	}
	factor := findFactor(result.root, fixture.diagram.ranks[factorFirst])
	if factor == nil {
		t.Fatal("transform deleted the surviving factor")
	}
	_, count, low, high := assertKeyAVL(t, factor.keys)
	if count != 8 || low != 8 || high != 15 {
		t.Fatalf("survivors = count:%d [%d,%d], want 8 [8,15]", count, low, high)
	}
	result, ok = builder.Seal(result)
	if !ok {
		t.Fatal("transform seal")
	}
	wantEntries := make(map[testKey]terminal.ID[uint8], 8)
	for key := testKey(8); key <= 15; key++ {
		wantEntries[key] = fixture.values[0]
	}
	wantEntries[15] = fixture.values[1]
	want := soleRoot(t, fixture, wantEntries)
	if !fixture.diagram.Equal(result, want) {
		t.Fatal("shape-preserving transform differs from canonical sparse root")
	}
}

func TestMergeSoleFactorChangesPreservesUntouchedSubtreeAndCountDeltas(t *testing.T) {
	fixture := newSoleBatchFixture(t)
	all := soleAll(t, fixture)
	left := soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{1: fixture.values[0], 2: fixture.values[0], 3: fixture.values[0], 4: fixture.values[0], 5: fixture.values[0]})
	right := soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{2: fixture.values[1]})
	combine := func(_ testKey, _, right terminal.ID[uint8]) (terminal.ID[uint8], bool) { return right, true }
	equal := func(left, right terminal.ID[uint8]) bool { return left == right }
	leftFactor := findFactor(left.root, fixture.diagram.ranks[factorFirst])
	builder := fixture.diagram.Begin()
	replaced, ok := builder.MergeSoleFactorChanges(left, right, 1, NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal,
		func(testKey, support.Mask) bool { return true }, func(int) (testKey, support.Mask, support.Mask, support.Mask, bool) { return 2, all, all, all, true })
	if !ok || replaced.count != left.count {
		t.Fatalf("replace count = %d, want %d", replaced.count, left.count)
	}
	if findFactor(replaced.root, fixture.diagram.ranks[factorFirst]).right != leftFactor.right {
		t.Fatal("untouched opposite subtree was not shared")
	}
	for _, test := range []struct {
		name  string
		right Root[testFactor, testKey, uint8]
		key   testKey
		want  int
	}{
		{"insert", soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{9: fixture.values[1]}), 9, left.count + 1},
		{"delete", fixture.diagram.Empty(), 1, left.count - 1},
	} {
		builder := fixture.diagram.Begin()
		result, ok := builder.MergeSoleFactorChanges(left, test.right, 1, NewSoleScratch[testKey, uint8](), support.New(fixture.manager), combine, equal,
			func(testKey, support.Mask) bool { return true }, func(int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
				return test.key, all, all, all, true
			})
		if !ok || result.count != test.want {
			t.Fatalf("%s count = %d, want %d", test.name, result.count, test.want)
		}
	}
}

func TestMergeSoleFactorChangesRejectsForeignSupportManager(t *testing.T) {
	fixture := newSoleBatchFixture(t)
	foreign, err := guard.New([]guard.Atom{99})
	if err != nil {
		t.Fatal(err)
	}
	foreignMask, ok := support.True(foreign)
	if !ok {
		t.Fatal("foreign mask")
	}
	all := soleAll(t, fixture)
	left := soleRoot(t, fixture, map[testKey]terminal.ID[uint8]{2: fixture.values[0]})
	builder := fixture.diagram.Begin()
	if _, ok := builder.MergeSoleFactorChanges(left, fixture.diagram.Empty(), 1, NewSoleScratch[testKey, uint8](), support.New(fixture.manager),
		func(_ testKey, _, _ terminal.ID[uint8]) (terminal.ID[uint8], bool) { return terminal.ID[uint8]{}, true },
		func(left, right terminal.ID[uint8]) bool { return left == right }, func(testKey, support.Mask) bool { return true },
		func(int) (testKey, support.Mask, support.Mask, support.Mask, bool) {
			return 2, foreignMask, all, all, true
		}); ok {
		t.Fatal("accepted support mask from foreign manager")
	}
}
