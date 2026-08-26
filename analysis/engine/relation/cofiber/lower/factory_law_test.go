package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber/lower"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// mountAtoms reads back every neutral atom the mount's own scopes stand on.
// A production caller declares an extent for exactly this set.
func mountAtoms(t *testing.T, mounted witness.Mounted) []identity.ContentID {
	t.Helper()
	atoms := make([]identity.ContentID, 0, 4)
	seen := make(map[identity.ContentID]struct{}, 4)
	for _, id := range mounted.Scopes() {
		scope, ok := mounted.Scope(id)
		if !ok {
			t.Fatal("the mount does not resolve its own scope")
		}
		value, ok := mounted.RegionForScope(scope)
		if !ok || !value.Available() {
			t.Fatal("the mount does not carry the region its scope stands on")
		}
		for _, row := range value.Nodes() {
			if !row.Atom.Available() {
				t.Fatal("a mounted region carries an unavailable atom")
			}
			if _, duplicate := seen[row.Atom.ID()]; duplicate {
				continue
			}
			seen[row.Atom.ID()] = struct{}{}
			atoms = append(atoms, row.Atom.ID())
		}
	}
	return atoms
}

// TestAMountSolvesThroughTheGeometryItsOwnRegionsFoldTo carries one committed
// mount through the whole physical half of the span: the scopes the mount
// declares are read back, each of their atoms is given an extent, the fold
// becomes a scope authority, and the runtime bootstraps and solves over the
// geometry that authority yields.
//
// This is the production shape of the arithmetic fixture's own geometry: the
// fixture enumerates a mask for its one region identity, and here nothing is
// enumerated. The folded view drives the same schedule the fixture's declared
// view does, which is the property that belongs to the geometry.
//
// The root is bootstrapped fresh and therefore carries no seeded rows, so this
// solve publishes nothing. That is a property of the root, not of the view: the
// seeded comparison lives with the constructor, which owns the seeding step.
// The two views cannot share one root, because a stored column's masks belong
// to the guard manager its geometry was sealed with.
func TestAMountSolvesThroughTheGeometryItsOwnRegionsFoldTo(t *testing.T) {
	fixture := arithmetic.New(t)
	mounted := fixture.Mounted()
	if !mounted.Available() {
		t.Fatal("the arithmetic fixture did not mount")
	}
	atoms := mountAtoms(t, mounted)
	if len(atoms) == 0 {
		t.Fatal("the mount declares no scope atom to give an extent to")
	}

	order := make([]guard.Atom, len(atoms))
	for index := range atoms {
		order[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(order)
	if err != nil || manager == nil {
		t.Fatalf("seal the physical universe: %v", err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("open physical work")
	}
	extents := make(map[identity.ContentID]support.Mask, len(atoms))
	for index, atom := range atoms {
		mask, ok := work.Literal(guard.Atom(index+1), true)
		if !ok {
			t.Fatalf("extent for atom %d", index)
		}
		extents[atom] = mask
	}
	if !work.Seal() {
		work.Discard()
		t.Fatal("seal the declared extents")
	}

	lowering, ok := lower.New(manager, extents)
	if !ok {
		t.Fatal("seal the lowering over the mount's own atoms")
	}
	factory, ok := lower.NewFactory(lowering)
	if !ok {
		t.Fatal("adopt the lowering as a mount capability")
	}
	view, ok := factory.Bind(mounted)
	if !ok || !view.Available() || !view.ValidFor(mounted) {
		t.Fatal("bind the geometry the mount's regions fold to")
	}

	base, ok := database.Bootstrap(mounted, view)
	if !ok || !base.Available() {
		t.Fatal("bootstrap the root over the folded geometry")
	}
	result, ok := runtime.Solve(mounted, base, view)
	if !ok || !result.Available() {
		t.Fatal("solve over the folded geometry")
	}

	reference, ok := runtime.Solve(mounted, fixture.Base(), fixture.View())
	if !ok || !reference.Available() {
		t.Fatal("solve over the fixture's declared geometry")
	}
	if result.Evaluations() != reference.Evaluations() {
		t.Fatalf("the folded view drove a different schedule: folded=%d declared=%d",
			result.Evaluations(), reference.Evaluations())
	}
	if result.Publications() != 0 {
		t.Fatalf("an unseeded root published %d times", result.Publications())
	}
	if reference.Publications() == 0 {
		t.Fatal("the seeded reference root published nothing, so the counts above prove nothing")
	}
}

// TestAMountWhoseAtomsWereNotDeclaredRefuses states that a geometry never
// covers a scope whose extent nobody declared: binding refuses rather than
// producing a view whose scopes silently mean nothing.
func TestAMountWhoseAtomsWereNotDeclaredRefuses(t *testing.T) {
	fixture := arithmetic.New(t)
	manager, err := guard.New([]guard.Atom{1})
	if err != nil || manager == nil {
		t.Fatalf("seal the physical universe: %v", err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("open physical work")
	}
	stranger, ok := identity.DeriveContentID("relation-lower-law/v1", []byte("declared-by-nobody"))
	if !ok {
		t.Fatal("derive a foreign atom")
	}
	mask, ok := work.Literal(1, true)
	if !ok || !work.Seal() {
		work.Discard()
		t.Fatal("seal the unrelated extent")
	}
	lowering, ok := lower.New(manager, map[identity.ContentID]support.Mask{stranger: mask})
	if !ok {
		t.Fatal("seal the lowering")
	}
	factory, ok := lower.NewFactory(lowering)
	if !ok {
		t.Fatal("adopt the lowering")
	}
	if view, ok := factory.Bind(fixture.Mounted()); ok || view.Available() {
		t.Fatal("a mount bound a geometry over atoms nobody declared")
	}
	var absent lower.Factory
	if absent.Available() {
		t.Fatal("the zero factory is available")
	}
	if _, ok := absent.Bind(fixture.Mounted()); ok {
		t.Fatal("the zero factory bound a geometry")
	}
}
