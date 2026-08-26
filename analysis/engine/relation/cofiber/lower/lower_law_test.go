package lower

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func atomID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation-lower-law/v1", []byte(name))
	if !ok {
		t.Fatalf("derive atom %s", name)
	}
	return value
}

func atomRegion(t *testing.T, id identity.ContentID) region.Region {
	t.Helper()
	atom, ok := region.NewAtom(id)
	if !ok {
		t.Fatal("adopt atom")
	}
	value, ok := region.FromAtom(atom)
	if !ok || !value.Available() {
		t.Fatal("build single atom region")
	}
	return value
}

// universe seals one physical manager and gives each named neutral atom the
// extent of its own physical variable. That is the arrangement the committed
// arithmetic fixture declares for its single scope, so the laws below are
// stated against the same shape production mounts hand this package.
func universe(t *testing.T, names ...string) (*guard.Manager, map[identity.ContentID]support.Mask) {
	t.Helper()
	order := make([]guard.Atom, len(names))
	for index := range names {
		order[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(order)
	if err != nil || manager == nil {
		t.Fatalf("seal physical universe: %v", err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("open physical work")
	}
	extents := make(map[identity.ContentID]support.Mask, len(names))
	for index, name := range names {
		mask, ok := work.Literal(guard.Atom(index+1), true)
		if !ok {
			t.Fatalf("literal for %s", name)
		}
		extents[atomID(t, name)] = mask
	}
	if !work.Seal() {
		work.Discard()
		t.Fatal("seal extents")
	}
	return manager, extents
}

func maskIdentity(t *testing.T, mask support.Mask) [32]byte {
	t.Helper()
	if !mask.Valid() {
		t.Fatal("mask is invalid")
	}
	value, ok := mask.Identity()
	if !ok {
		t.Fatal("mask has no canonical identity")
	}
	return value
}

// TestAConjunctionIsAnsweredByEvaluationNotByEnumeration is the reason this
// fold exists. The conjunction of two declared regions is never handed to the
// lowering, and it still answers with exactly the conjunction of their two
// masks. cofiber's table form refuses here unless its caller predicted this
// conjunction in advance, which is the obligation this package removes.
func TestAConjunctionIsAnsweredByEvaluationNotByEnumeration(t *testing.T) {
	manager, extents := universe(t, "first", "second")
	lowering, ok := New(manager, extents)
	if !ok || !lowering.Available() {
		t.Fatal("seal the lowering")
	}
	left, right := atomRegion(t, atomID(t, "first")), atomRegion(t, atomID(t, "second"))
	conjoined, ok := region.Conjoin(left, right)
	if !ok || !conjoined.Available() {
		t.Fatal("conjoin the two regions")
	}
	lowered, ok := lowering.Translate(conjoined)
	if !ok {
		t.Fatal("translate the conjunction")
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("open physical work")
	}
	expected, ok := work.And(extents[atomID(t, "first")], extents[atomID(t, "second")])
	if !ok || !work.Seal() {
		work.Discard()
		t.Fatal("conjoin the two extents")
	}
	if maskIdentity(t, lowered) != maskIdentity(t, expected) {
		t.Fatal("the lowered conjunction is not the conjunction of the declared extents")
	}
}

// TestASingleAtomLowersToItsDeclaredExtent states the base case the whole fold
// rests on: a region that is one atom denotes exactly what its owner declared
// that atom covers, neither widened nor narrowed.
func TestASingleAtomLowersToItsDeclaredExtent(t *testing.T) {
	manager, extents := universe(t, "only")
	lowering, ok := New(manager, extents)
	if !ok {
		t.Fatal("seal the lowering")
	}
	lowered, ok := lowering.Translate(atomRegion(t, atomID(t, "only")))
	if !ok {
		t.Fatal("translate the atom")
	}
	if maskIdentity(t, lowered) != maskIdentity(t, extents[atomID(t, "only")]) {
		t.Fatal("a single atom did not lower to its declared extent")
	}
}

// TestAnAtomExtentMayBeACompoundRegion states what expanding on the extent
// rather than on a physical variable buys: an owner may declare that one
// neutral atom covers a compound region of the universe, and every region over
// that atom still lowers correctly.
func TestAnAtomExtentMayBeACompoundRegion(t *testing.T) {
	manager, extents := universe(t, "first", "second")
	work := support.New(manager)
	if work == nil {
		t.Fatal("open physical work")
	}
	compound, ok := work.Or(extents[atomID(t, "first")], extents[atomID(t, "second")])
	if !ok || !work.Seal() {
		work.Discard()
		t.Fatal("build a compound extent")
	}
	wide := atomID(t, "wide")
	lowering, ok := New(manager, map[identity.ContentID]support.Mask{wide: compound})
	if !ok {
		t.Fatal("seal the lowering over a compound extent")
	}
	lowered, ok := lowering.Translate(atomRegion(t, wide))
	if !ok {
		t.Fatal("translate the compound atom")
	}
	if maskIdentity(t, lowered) != maskIdentity(t, compound) {
		t.Fatal("a compound extent was not preserved")
	}
}

// TestTerminalsLowerToTheUnconstrainedAndEmptyMasks states that the explicit
// terminals keep their meaning across the boundary and need no declaration.
func TestTerminalsLowerToTheUnconstrainedAndEmptyMasks(t *testing.T) {
	manager, extents := universe(t, "only")
	lowering, ok := New(manager, extents)
	if !ok {
		t.Fatal("seal the lowering")
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("open physical work")
	}
	wantTrue, wantFalse := work.True(), work.False()
	if !work.Seal() {
		work.Discard()
		t.Fatal("seal the reference masks")
	}
	loweredTrue, trueOK := lowering.Translate(region.True())
	loweredFalse, falseOK := lowering.Translate(region.False())
	if !trueOK || !falseOK {
		t.Fatal("translate the terminals")
	}
	if maskIdentity(t, loweredTrue) != maskIdentity(t, wantTrue) {
		t.Fatal("the true terminal did not lower to the unconstrained mask")
	}
	if maskIdentity(t, loweredFalse) != maskIdentity(t, wantFalse) {
		t.Fatal("the false terminal did not lower to the empty mask")
	}
}

// TestAnUndeclaredExtentRefuses is the no-invention law. This package never
// decides what an atom covers, so a region over an atom it was not given has no
// answer here and is never approximated by one that was.
func TestAnUndeclaredExtentRefuses(t *testing.T) {
	manager, extents := universe(t, "declared")
	lowering, ok := New(manager, extents)
	if !ok {
		t.Fatal("seal the lowering")
	}
	if mask, ok := lowering.Translate(atomRegion(t, atomID(t, "undeclared"))); ok || mask.Valid() {
		t.Fatal("a region over an undeclared atom was translated")
	}
	if _, ok := New(nil, extents); ok {
		t.Fatal("a lowering without a universe was sealed")
	}
	foreign, foreignExtents := universe(t, "declared")
	if _, ok := New(manager, foreignExtents); ok {
		t.Fatal("an extent from a foreign universe was adopted")
	}
	if foreign == nil {
		t.Fatal("foreign universe")
	}
	if _, ok := New(manager, map[identity.ContentID]support.Mask{{}: extents[atomID(t, "declared")]}); ok {
		t.Fatal("an unavailable atom identity was declared")
	}
	var absent Lowering
	if absent.Available() || absent.Manager() != nil {
		t.Fatal("the zero lowering holds a universe")
	}
	if _, ok := absent.Translate(region.True()); ok {
		t.Fatal("the zero lowering translated a terminal")
	}
}
