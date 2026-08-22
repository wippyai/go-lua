package contextfiber

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func layoutDirectory(t *testing.T, sameModule bool, contextCount int) fiberDirectoryFixture {
	t.Helper()
	if contextCount <= 0 {
		t.Fatal("context count")
	}
	link := fiberLawID(t, "layout-link")
	contexts := make([]executioncontext.Context, 0, contextCount)
	roots := make([]executioncontext.RootContext, 0, contextCount)
	for index := 0; index < contextCount; index++ {
		module := fiberLawID(t, "layout-module/shared")
		if !sameModule {
			module = fiberLawID(t, "layout-module/"+string(rune('a'+index)))
		}
		row, rowOK := executioncontext.NewContext(link, module, fiberLawID(t, "layout-actor/"+string(rune('a'+index))), fiberLawID(t, "layout-representative"))
		if !rowOK {
			t.Fatal("layout context")
		}
		root, rootOK := executioncontext.NewRootContext(link, fiberLawID(t, "layout-root/"+string(rune('a'+index))), row.ID())
		if !rootOK {
			t.Fatal("layout root")
		}
		contexts = append(contexts, row)
		roots = append(roots, root)
	}
	directory, directoryOK := executioncontext.Seal(link, contexts, roots, nil)
	if !directoryOK {
		t.Fatal("layout directory")
	}
	return fiberDirectoryFixture{directory: directory, contexts: contexts, link: link}
}

func mountedOwnersForDirectory(t *testing.T, directory executioncontext.Directory, includeGlobal bool) []PointOwner {
	t.Helper()
	owners := make([]PointOwner, 0, directory.ContextCount()+1)
	seen := make(map[identity.ContentID]struct{})
	for index := 0; index < directory.ContextCount(); index++ {
		context, contextOK := directory.ContextAt(index)
		if !contextOK {
			t.Fatal("layout owner context")
		}
		module := context.ModuleKey()
		if _, duplicate := seen[module]; duplicate {
			continue
		}
		owner, ownerOK := Mounted(module)
		if !ownerOK {
			t.Fatal("mounted owner")
		}
		owners = append(owners, owner)
		seen[module] = struct{}{}
	}
	if includeGlobal {
		owner, ownerOK := LinkGlobal(directory.LinkID())
		if !ownerOK {
			t.Fatal("global owner")
		}
		owners = append(owners, owner)
	}
	return owners
}

func TestPointOwnersAuthenticateMountedAndGlobalLanes(t *testing.T) {
	module := fiberLawID(t, "owner-module")
	link := fiberLawID(t, "owner-link")
	mounted, mountedOK := Mounted(module)
	global, globalOK := LinkGlobal(link)
	if !mountedOK || !globalOK || !mounted.Available() || !global.Available() || !mounted.Mounted() || mounted.LinkGlobal() || !global.LinkGlobal() || global.Mounted() {
		t.Fatal("owner constructors")
	}
	if mounted.ModuleKey() != module || mounted.LinkID().Available() || global.LinkID() != link || global.ModuleKey().Available() {
		t.Fatal("owner accessors")
	}
	if mounted.Kind() != PointOwnerMounted || global.Kind() != PointOwnerLinkGlobal || !mounted.ID().Available() || !global.ID().Available() {
		t.Fatal("owner identities")
	}
	if forged := (PointOwner{kind: PointOwnerMounted, key: module, id: link}); forged.Available() {
		t.Fatal("forged mounted owner authenticated")
	}
	if _, ok := Mounted(identity.ContentID{}); ok {
		t.Fatal("zero module owner admitted")
	}
	if _, ok := LinkGlobal(identity.ContentID{}); ok {
		t.Fatal("zero global owner admitted")
	}
}

func TestLayoutRejectsCrossModulePairsAndSharesGlobalRows(t *testing.T) {
	fixture := layoutDirectory(t, false, 2)
	owners := mountedOwnersForDirectory(t, fixture.directory, true)
	index, indexOK := New(fixture.directory, len(owners), identity.Generation(21))
	if !indexOK {
		t.Fatal("layout index")
	}
	layout, layoutOK := NewLayout(index, fixture.directory, owners, identity.Generation(21))
	if !layoutOK || !layout.Available() {
		t.Fatal("layout seal")
	}
	if layout.StateCount() != 3 || layout.StateCount() >= StateOrdinal(index.FiberCount()) {
		t.Fatalf("compact state count=%d cartesian=%d", layout.StateCount(), index.FiberCount())
	}
	first, firstOK := index.ContextID(0)
	second, secondOK := index.ContextID(1)
	if !firstOK || !secondOK {
		t.Fatal("layout contexts")
	}
	firstContext, firstContextOK := fixture.directory.Context(first)
	secondContext, secondContextOK := fixture.directory.Context(second)
	if !firstContextOK || !secondContextOK {
		t.Fatal("layout context rows")
	}
	firstModule, secondModule := firstContext.ModuleKey(), secondContext.ModuleKey()
	var firstPoint, secondPoint PointOrdinal
	for point := 0; point < layout.PointCount(); point++ {
		owner, ownerOK := layout.PointOwnerAt(PointOrdinal(point))
		if !ownerOK {
			t.Fatal("layout point owner")
		}
		switch {
		case owner.Mounted() && owner.ModuleKey() == firstModule:
			firstPoint = PointOrdinal(point)
		case owner.Mounted() && owner.ModuleKey() == secondModule:
			secondPoint = PointOrdinal(point)
		}
	}
	firstState, firstStateOK := layout.Lookup(0, firstPoint)
	secondState, secondStateOK := layout.Lookup(1, secondPoint)
	if !firstStateOK || !secondStateOK || firstState == secondState {
		t.Fatalf("mounted rows did not remain context-distinct: %d/%v and %d/%v", firstState, firstStateOK, secondState, secondStateOK)
	}
	if _, ok := layout.Lookup(0, secondPoint); ok {
		t.Fatal("cross-module mounted point admitted")
	}
	var globalPoint PointOrdinal
	for point := 0; point < layout.PointCount(); point++ {
		owner, ownerOK := layout.PointOwnerAt(PointOrdinal(point))
		if !ownerOK {
			t.Fatal("global point owner")
		}
		if owner.LinkGlobal() {
			globalPoint = PointOrdinal(point)
		}
	}
	globalFirst, globalFirstOK := layout.Lookup(0, globalPoint)
	globalSecond, globalSecondOK := layout.Lookup(1, globalPoint)
	if !globalFirstOK || !globalSecondOK || globalFirst != globalSecond {
		t.Fatal("global point was duplicated per context")
	}
	cell, cellOK := layout.StateAt(globalFirst)
	expectedGlobalOwner, expectedGlobalOwnerOK := layout.PointOwnerAt(globalPoint)
	if !cellOK || !cell.Available() || !expectedGlobalOwnerOK || cell.Owner() != expectedGlobalOwner {
		t.Fatal("global inverse row was not singular")
	}
	if _, contextOK := cell.ContextOrdinal(); contextOK {
		t.Fatal("global inverse row fabricated a context")
	}
	mountedCell, mountedCellOK := layout.StateAt(firstState)
	if !mountedCellOK || !mountedCell.Available() {
		t.Fatal("mounted inverse row")
	}
	if context, ok := mountedCell.ContextOrdinal(); !ok || context != 0 {
		t.Fatalf("mounted inverse context=%d/%v", context, ok)
	}
}

func TestLayoutKeepsSameModuleContextsDistinct(t *testing.T) {
	fixture := layoutDirectory(t, true, 2)
	owners := mountedOwnersForDirectory(t, fixture.directory, true)
	index, indexOK := New(fixture.directory, len(owners), identity.Generation(22))
	if !indexOK {
		t.Fatal("same-module index")
	}
	layout, layoutOK := NewLayout(index, fixture.directory, owners, identity.Generation(22))
	if !layoutOK {
		t.Fatal("same-module layout")
	}
	var mounted PointOrdinal
	for point := 0; point < layout.PointCount(); point++ {
		owner, ownerOK := layout.PointOwnerAt(PointOrdinal(point))
		if !ownerOK {
			t.Fatal("same-module owner")
		}
		if owner.Mounted() {
			mounted = PointOrdinal(point)
		}
	}
	left, leftOK := layout.Lookup(0, mounted)
	right, rightOK := layout.Lookup(1, mounted)
	if !leftOK || !rightOK || left == right {
		t.Fatalf("same-module contexts shared state row: %d/%v %d/%v", left, leftOK, right, rightOK)
	}
	if layout.StateCount() != 3 {
		t.Fatalf("same-module compact count=%d want=3", layout.StateCount())
	}
}

func TestLayoutRefusesOrphansForeignOwnersAndShapes(t *testing.T) {
	fixture := layoutDirectory(t, false, 2)
	global, globalOK := LinkGlobal(fixture.directory.LinkID())
	if !globalOK {
		t.Fatal("global owner")
	}
	index, indexOK := New(fixture.directory, 1, identity.Generation(23))
	if !indexOK {
		t.Fatal("orphan index")
	}
	if _, ok := NewLayout(index, fixture.directory, []PointOwner{global}, identity.Generation(23)); ok {
		t.Fatal("context module without mounted point admitted")
	}
	foreignModule, foreignModuleOK := Mounted(fiberLawID(t, "orphan-module"))
	if !foreignModuleOK {
		t.Fatal("foreign mounted owner")
	}
	if _, ok := NewLayout(index, fixture.directory, []PointOwner{foreignModule}, identity.Generation(23)); ok {
		t.Fatal("mounted module without context admitted")
	}
	foreignGlobal, foreignGlobalOK := LinkGlobal(fiberLawID(t, "foreign-link"))
	if !foreignGlobalOK {
		t.Fatal("foreign global owner")
	}
	if _, ok := NewLayout(index, fixture.directory, []PointOwner{foreignGlobal}, identity.Generation(23)); ok {
		t.Fatal("foreign global owner admitted")
	}
	owners := mountedOwnersForDirectory(t, fixture.directory, true)
	index, indexOK = New(fixture.directory, len(owners), identity.Generation(24))
	if !indexOK {
		t.Fatal("valid fence index")
	}
	layout, layoutOK := NewLayout(index, fixture.directory, owners, identity.Generation(24))
	if !layoutOK || !layout.OwnedBy(index, fixture.directory, owners, identity.Generation(24)) {
		t.Fatal("valid layout fence")
	}
	if layout.OwnedBy(index, fixture.directory, owners, identity.Generation(25)) || layout.OwnedBy(index, fixture.directory, owners[:len(owners)-1], identity.Generation(24)) {
		t.Fatal("layout crossed generation or point-shape fence")
	}
	foreignDirectory := newFiberDirectory(t, "left", "right")
	if layout.OwnedBy(index, foreignDirectory.directory, owners, identity.Generation(24)) {
		t.Fatal("layout crossed directory fence")
	}
	foreignIndex, foreignIndexOK := New(foreignDirectory.directory, len(owners), identity.Generation(24))
	if !foreignIndexOK {
		t.Fatal("foreign index")
	}
	if layout.OwnedBy(foreignIndex, fixture.directory, owners, identity.Generation(24)) || layout.OwnedBy(index, fixture.directory, owners, 0) {
		t.Fatal("layout crossed index/generation owner fence")
	}
	var zero Layout
	if zero.Available() || zero.OwnedBy(index, fixture.directory, owners, identity.Generation(24)) || zero.StateCount() != 0 {
		t.Fatal("zero layout available")
	}
}

func TestLayoutPermutationAndAliasRowsAreCanonical(t *testing.T) {
	fixture := newFiberDirectory(t, "left", "right")
	owners := mountedOwnersForDirectory(t, fixture.directory, true)
	index, indexOK := New(fixture.directory, len(owners), identity.Generation(25))
	if !indexOK {
		t.Fatal("permutation index")
	}
	first, firstOK := NewLayout(index, fixture.directory, owners, identity.Generation(25))
	if !firstOK {
		t.Fatal("first layout")
	}
	contexts := append([]executioncontext.Context(nil), fixture.contexts...)
	roots := make([]executioncontext.RootContext, 0, len(contexts))
	for index := range contexts {
		root, rootOK := executioncontext.NewRootContext(fixture.link, fiberLawID(t, "layout-permuted-root/"+string(rune('a'+index))), contexts[index].ID())
		if !rootOK {
			t.Fatal("permuted root")
		}
		roots = append(roots, root)
	}
	permutedDirectory, permutedOK := executioncontext.Seal(fixture.link, []executioncontext.Context{contexts[1], contexts[0]}, []executioncontext.RootContext{roots[1], roots[0]}, nil)
	if !permutedOK {
		t.Fatal("permuted directory")
	}
	secondIndex, secondIndexOK := New(permutedDirectory, len(owners), identity.Generation(25))
	second, secondOK := NewLayout(secondIndex, permutedDirectory, owners, identity.Generation(25))
	if !secondIndexOK || !secondOK || first.StateCount() != second.StateCount() {
		t.Fatal("permuted layout")
	}
	for context := 0; context < first.ContextCount(); context++ {
		for point := 0; point < first.PointCount(); point++ {
			left, leftOK := first.Lookup(ContextOrdinal(context), PointOrdinal(point))
			right, rightOK := second.Lookup(ContextOrdinal(context), PointOrdinal(point))
			if leftOK != rightOK || left != right {
				t.Fatalf("permutation changed row (%d,%d): %d/%v %d/%v", context, point, left, leftOK, right, rightOK)
			}
		}
	}

	aliasLink := fiberLawID(t, "layout-alias-link")
	module, actor, representative := fiberLawID(t, "layout-alias-module"), fiberLawID(t, "layout-alias-actor"), fiberLawID(t, "layout-alias-representative")
	context, contextOK := executioncontext.NewContext(aliasLink, module, actor, representative)
	if !contextOK {
		t.Fatal("alias context")
	}
	rootA, rootAOK := executioncontext.NewRootContext(aliasLink, fiberLawID(t, "layout-alias-root-a"), context.ID())
	rootB, rootBOK := executioncontext.NewRootContext(aliasLink, fiberLawID(t, "layout-alias-root-b"), context.ID())
	aliasDirectory, aliasDirectoryOK := executioncontext.Seal(aliasLink, []executioncontext.Context{context}, []executioncontext.RootContext{rootA, rootB}, nil)
	if !rootAOK || !rootBOK || !aliasDirectoryOK {
		t.Fatal("alias directory")
	}
	mounted, mountedOK := Mounted(module)
	global, globalOK := LinkGlobal(aliasLink)
	if !mountedOK || !globalOK {
		t.Fatal("alias owners")
	}
	aliasIndex, aliasIndexOK := New(aliasDirectory, 2, identity.Generation(26))
	aliasLayout, aliasLayoutOK := NewLayout(aliasIndex, aliasDirectory, []PointOwner{mounted, global}, identity.Generation(26))
	if !aliasIndexOK || !aliasLayoutOK || aliasLayout.ContextCount() != 1 || aliasLayout.StateCount() != 2 {
		t.Fatalf("alias quotient expanded: contexts=%d states=%d", aliasLayout.ContextCount(), aliasLayout.StateCount())
	}
}

func TestLayoutBoundsAndAdditionOverflowRefuse(t *testing.T) {
	fixture := layoutDirectory(t, false, 2)
	owners := mountedOwnersForDirectory(t, fixture.directory, true)
	index, indexOK := New(fixture.directory, len(owners), identity.Generation(27))
	if !indexOK {
		t.Fatal("bounds index")
	}
	layout, layoutOK := NewLayout(index, fixture.directory, owners, identity.Generation(27))
	if !layoutOK {
		t.Fatal("bounds layout")
	}
	if _, ok := layout.Lookup(ContextOrdinal(layout.ContextCount()), 0); ok {
		t.Fatal("context upper bound admitted")
	}
	if _, ok := layout.Lookup(0, PointOrdinal(layout.PointCount())); ok {
		t.Fatal("layout bounds admitted")
	}
	if _, ok := layout.StateAt(layout.StateCount()); ok || func() bool {
		_, ok := layout.StateAt(StateOrdinal(math.MaxUint64))
		return ok
	}() {
		t.Fatal("layout state bounds admitted")
	}
	if sum, ok := checkedAdd(math.MaxUint64, 1); ok || sum != 0 {
		t.Fatal("addition overflow wrapped")
	}
	if sum, ok := checkedAdd(0, math.MaxUint64); !ok || sum != math.MaxUint64 {
		t.Fatal("maximum addition refused")
	}
}
