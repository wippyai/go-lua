package contextfiber

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func fiberLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("contextfiber-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

type fiberDirectoryFixture struct {
	directory executioncontext.Directory
	contexts  []executioncontext.Context
	link      identity.ContentID
}

func newFiberDirectory(t *testing.T, labels ...string) fiberDirectoryFixture {
	t.Helper()
	if len(labels) == 0 {
		labels = []string{"left", "right"}
	}
	link := fiberLawID(t, "link")
	contexts := make([]executioncontext.Context, 0, len(labels))
	roots := make([]executioncontext.RootContext, 0, len(labels))
	for _, label := range labels {
		row, ok := executioncontext.NewContext(link, fiberLawID(t, "module/"+label), fiberLawID(t, "actor"), fiberLawID(t, "representative"))
		if !ok {
			t.Fatalf("context %s", label)
		}
		root, ok := executioncontext.NewRootContext(link, fiberLawID(t, "root/"+label), row.ID())
		if !ok {
			t.Fatalf("root %s", label)
		}
		contexts = append(contexts, row)
		roots = append(roots, root)
	}
	directory, ok := executioncontext.Seal(link, contexts, roots, nil)
	if !ok {
		t.Fatal("seal context directory")
	}
	return fiberDirectoryFixture{directory: directory, contexts: contexts, link: link}
}

func TestIndexUsesDirectoryContextOrderAndRoundTripsEveryTuple(t *testing.T) {
	fixture := newFiberDirectory(t, "left", "middle", "right")
	index, ok := New(fixture.directory, 5, identity.Generation(7))
	if !ok || !index.Available() {
		t.Fatal("valid context fiber index refused")
	}
	if index.ContextCount() != fixture.directory.ContextCount() || index.PointCount() != 5 || index.FiberCount() != 15 {
		t.Fatalf("shape = contexts=%d points=%d fibers=%d", index.ContextCount(), index.PointCount(), index.FiberCount())
	}
	for contextIndex := 0; contextIndex < fixture.directory.ContextCount(); contextIndex++ {
		row, rowOK := fixture.directory.ContextAt(contextIndex)
		if !rowOK {
			t.Fatalf("directory context %d", contextIndex)
		}
		contextOrdinal, ordinalOK := index.ContextOrdinal(row.ID())
		if !ordinalOK || contextOrdinal != ContextOrdinal(contextIndex) {
			t.Fatalf("context %d got ordinal %d/%v", contextIndex, contextOrdinal, ordinalOK)
		}
		resolved, resolvedOK := index.ContextID(contextOrdinal)
		if !resolvedOK || resolved != row.ID() {
			t.Fatalf("context %d did not round-trip", contextIndex)
		}
		for pointIndex := 0; pointIndex < index.PointCount(); pointIndex++ {
			flat, flatOK := index.Flatten(contextOrdinal, PointOrdinal(pointIndex))
			if !flatOK {
				t.Fatalf("flatten (%d,%d)", contextIndex, pointIndex)
			}
			gotContext, gotPoint, unflatOK := index.Unflatten(flat)
			if !unflatOK || gotContext != contextOrdinal || gotPoint != PointOrdinal(pointIndex) {
				t.Fatalf("round-trip (%d,%d) = (%d,%d)/%v", contextIndex, pointIndex, gotContext, gotPoint, unflatOK)
			}
		}
	}
	seen := make(map[FiberOrdinal]struct{})
	for contextIndex := 0; contextIndex < index.ContextCount(); contextIndex++ {
		for pointIndex := 0; pointIndex < index.PointCount(); pointIndex++ {
			flat, flatOK := index.Flatten(ContextOrdinal(contextIndex), PointOrdinal(pointIndex))
			if !flatOK {
				t.Fatal("valid tuple refused")
			}
			if _, duplicate := seen[flat]; duplicate {
				t.Fatalf("tuple collision at %d,%d -> %d", contextIndex, pointIndex, flat)
			}
			seen[flat] = struct{}{}
		}
	}
	if len(seen) != int(index.FiberCount()) {
		t.Fatalf("flatten image cardinality=%d want=%d", len(seen), index.FiberCount())
	}
}

func TestIndexRefusesUnavailableAndOutOfBoundsCoordinates(t *testing.T) {
	fixture := newFiberDirectory(t)
	var zero executioncontext.Directory
	if index, ok := New(zero, 1, identity.Generation(1)); ok || index.Available() {
		t.Fatal("zero directory admitted")
	}
	if index, ok := New(fixture.directory, 0, identity.Generation(1)); ok || index.Available() {
		t.Fatal("zero point shape admitted")
	}
	if index, ok := New(fixture.directory, -1, identity.Generation(1)); ok || index.Available() {
		t.Fatal("negative point shape admitted")
	}
	if index, ok := New(fixture.directory, 1, 0); ok || index.Available() {
		t.Fatal("zero generation admitted")
	}
	index, ok := New(fixture.directory, 2, identity.Generation(1))
	if !ok {
		t.Fatal("valid index")
	}
	if _, ok := index.Flatten(ContextOrdinal(index.ContextCount()), 0); ok {
		t.Fatal("context upper bound flattened")
	}
	if _, ok := index.Flatten(0, PointOrdinal(index.PointCount())); ok {
		t.Fatal("point upper bound flattened")
	}
	if _, ok := index.Flatten(ContextOrdinal(math.MaxUint64), 0); ok {
		t.Fatal("huge context flattened")
	}
	if _, _, ok := index.Unflatten(index.FiberCount()); ok {
		t.Fatal("fiber upper bound unflattened")
	}
	if _, _, ok := index.Unflatten(FiberOrdinal(math.MaxUint64)); ok {
		t.Fatal("huge fiber unflattened")
	}
	if _, ok := index.ContextOrdinal(identity.ContentID{}); ok {
		t.Fatal("zero context identity resolved")
	}
	if _, ok := index.ContextID(ContextOrdinal(index.ContextCount())); ok {
		t.Fatal("context upper bound resolved")
	}
}

func TestIndexOwnerGenerationAndShapeFence(t *testing.T) {
	fixture := newFiberDirectory(t)
	index, ok := New(fixture.directory, 3, identity.Generation(11))
	if !ok {
		t.Fatal("valid index")
	}
	if !index.OwnedBy(fixture.directory, 3, identity.Generation(11)) {
		t.Fatal("index did not recognize its owner")
	}
	if index.OwnedBy(fixture.directory, 4, identity.Generation(11)) || index.OwnedBy(fixture.directory, 3, identity.Generation(12)) {
		t.Fatal("index crossed shape or generation fence")
	}
	foreign := newFiberDirectory(t, "foreign")
	if index.OwnedBy(foreign.directory, 3, identity.Generation(11)) {
		t.Fatal("index crossed foreign directory fence")
	}
	if index.OwnedBy(executioncontext.Directory{}, 3, identity.Generation(11)) {
		t.Fatal("index crossed unavailable directory fence")
	}
	var zero Index
	if zero.Available() || zero.OwnedBy(fixture.directory, 3, identity.Generation(11)) || zero.Generation().Available() || zero.FiberCount() != 0 {
		t.Fatal("zero index crossed availability fence")
	}
}

func TestIndexOwnerIsStableAcrossDirectoryInputPermutation(t *testing.T) {
	fixture := newFiberDirectory(t, "left", "middle", "right")
	contexts := append([]executioncontext.Context(nil), fixture.contexts...)
	roots := make([]executioncontext.RootContext, 0, len(contexts))
	for _, row := range contexts {
		root, ok := executioncontext.NewRootContext(fixture.link, fiberLawID(t, "permuted-root/"+row.ModuleKey().String()), row.ID())
		if !ok {
			t.Fatal("permuted root")
		}
		roots = append(roots, root)
	}
	permuted, ok := executioncontext.Seal(fixture.link, []executioncontext.Context{contexts[2], contexts[0], contexts[1]}, []executioncontext.RootContext{roots[2], roots[0], roots[1]}, nil)
	if !ok {
		t.Fatal("permuted directory")
	}
	first, firstOK := New(fixture.directory, 4, identity.Generation(3))
	second, secondOK := New(permuted, 4, identity.Generation(3))
	if !firstOK || !secondOK {
		t.Fatal("permuted indexes")
	}
	for ordinal := 0; ordinal < first.ContextCount(); ordinal++ {
		firstID, firstIDOK := first.ContextID(ContextOrdinal(ordinal))
		secondID, secondIDOK := second.ContextID(ContextOrdinal(ordinal))
		if !firstIDOK || !secondIDOK || firstID != secondID {
			t.Fatalf("permutation changed context ordinal %d", ordinal)
		}
		for point := 0; point < first.PointCount(); point++ {
			left, leftOK := first.Flatten(ContextOrdinal(ordinal), PointOrdinal(point))
			right, rightOK := second.Flatten(ContextOrdinal(ordinal), PointOrdinal(point))
			if !leftOK || !rightOK || left != right {
				t.Fatalf("permutation changed fiber (%d,%d)", ordinal, point)
			}
		}
	}
}

func TestIndexReflectsDirectoryAliasQuotient(t *testing.T) {
	link := fiberLawID(t, "alias-link")
	module, actor, representative := fiberLawID(t, "module"), fiberLawID(t, "actor"), fiberLawID(t, "representative")
	first, firstOK := executioncontext.NewContext(link, module, actor, representative)
	second, secondOK := executioncontext.NewContext(link, module, actor, representative)
	if !firstOK || !secondOK || first.ID() != second.ID() {
		t.Fatal("context aliases did not share representative identity")
	}
	rootA, rootAOK := executioncontext.NewRootContext(link, fiberLawID(t, "alias-root-a"), first.ID())
	rootB, rootBOK := executioncontext.NewRootContext(link, fiberLawID(t, "alias-root-b"), second.ID())
	directory, directoryOK := executioncontext.Seal(link, []executioncontext.Context{first}, []executioncontext.RootContext{rootA, rootB}, nil)
	if !rootAOK || !rootBOK || !directoryOK || directory.ContextCount() != 1 {
		t.Fatal("aliased roots did not seal as one directory context")
	}
	index, indexOK := New(directory, 3, identity.Generation(1))
	if !indexOK || index.ContextCount() != 1 {
		t.Fatal("index multiplied aliased context")
	}
	if ordinal, ok := index.ContextOrdinal(first.ID()); !ok || ordinal != 0 {
		t.Fatal("aliased context did not occupy canonical ordinal zero")
	}
}

func TestIndexRefusesFlattenProductOverflow(t *testing.T) {
	fixture := newFiberDirectory(t, "one", "two", "three")
	if index, ok := New(fixture.directory, math.MaxInt, identity.Generation(1)); ok || index.Available() {
		t.Fatal("overflowing context-by-point shape admitted")
	}
	if product, ok := checkedProduct(math.MaxUint64, 2); ok || product != 0 {
		t.Fatal("checked product wrapped")
	}
	if product, ok := checkedProduct(0, math.MaxUint64); !ok || product != 0 {
		t.Fatal("zero product was not handled exactly")
	}
}
