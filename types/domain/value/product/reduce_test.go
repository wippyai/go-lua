package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/typ"
)

// build constructs an AbstractValue varying only the shape, presence, and numeric
// axes; the remaining axes carry their Top identity so a test isolates the axes it
// reduces over.
func build(shape shapevalue.Value, pres presence.Value, num numeric.Value) AbstractValue {
	return New(
		shape, pres, num,
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)
}

// rawNode builds a node directly so a test can observe the pre-reduction content a
// reducer then refines. It bypasses New (and therefore reduce and interning).
func rawNode(shape shapevalue.Value, pres presence.Value, num numeric.Value) *node {
	return &node{
		shape:    shape,
		presence: pres,
		numeric:  num,
		effects:  effectrows.Top(),
		owner:    ownership.Top(),
		escape:   escape.Top(),
		identity: identityrecursion.Top(),
		evidence: evidence.Top(),
	}
}

// reductionSamples spans the corners the reducers act on: contradictions between
// presence and shape, empty numeric content, and unconstrained values.
func reductionSamples() []*node {
	return []*node{
		rawNode(shapevalue.Of(typ.Number), presence.Present(), numeric.Top()),
		rawNode(shapevalue.Of(typ.Number), presence.Absent(), numeric.Top()),
		rawNode(shapevalue.Of(typ.Number), presence.Maybe(), numeric.Top()),
		rawNode(shapevalue.Of(typ.Number), presence.Bottom(), numeric.Top()),
		rawNode(shapevalue.Bottom(), presence.Present(), numeric.Top()),
		rawNode(shapevalue.Bottom(), presence.Maybe(), numeric.Top()),
		rawNode(shapevalue.Bottom(), presence.Absent(), numeric.Top()),
		rawNode(shapevalue.Top(), presence.Maybe(), numeric.Top()),
		rawNode(shapevalue.Of(typ.Number), presence.Present(), numeric.Bottom()),
		rawNode(shapevalue.Of(typ.Number), presence.Maybe(), numeric.Range(0, 4)),
		rawNode(shapevalue.Of(typ.NewUnion(typ.Number, typ.Nil)), presence.Maybe(), numeric.Top()),
		rawNode(shapevalue.Of(typ.NewUnion(typ.Number, typ.Nil)), presence.Absent(), numeric.Top()),
		// A pure-nil shape (not Bottom, but no non-nil content) under Present is a
		// two-rule contradiction: presence -> Bottom drags shape -> Bottom. The
		// reducer must close both directions in one pass to stay idempotent.
		rawNode(shapevalue.Of(typ.Nil), presence.Present(), numeric.Top()),
		rawNode(shapevalue.Of(typ.Nil), presence.Maybe(), numeric.Top()),
	}
}

// nodeCovers reports whether a covers b on every axis: the product order used to
// prove that reduction only refines downward.
func nodeCovers(a, b *node) bool {
	return a.shape.Covers(b.shape) &&
		a.presence.Covers(b.presence) &&
		a.numeric.Covers(b.numeric) &&
		a.effects.Covers(b.effects) &&
		a.owner.Covers(b.owner) &&
		a.escape.Covers(b.escape) &&
		a.identity.Covers(b.identity) &&
		a.evidence.Covers(b.evidence)
}

// TestReductionMonotone is the go_lua.reduction_monotone_idempotent law: reduction
// only refines downward, never raising an axis above the component join. The
// reduced node is covered by its pre-reduction input on every axis.
func TestReductionMonotone(t *testing.T) {
	for i, in := range reductionSamples() {
		out := reduce(in)
		if !nodeCovers(in, out) {
			t.Fatalf("sample %d: reduction raised an axis above the input (not downward)", i)
		}
	}
}

// TestReductionIdempotent is the second half of the law: reduce(reduce(n)) is
// node-equal to reduce(n), so the driver reaches a true local fixed point.
func TestReductionIdempotent(t *testing.T) {
	for i, in := range reductionSamples() {
		once := reduce(in)
		twice := reduce(once)
		if !nodeEqual(once, twice) {
			t.Fatalf("sample %d: reduce is not idempotent", i)
		}
	}
}

// TestEachLiveReducerIdempotent pins that every live reducer is a fixed point on
// its own output, the per-reducer obligation the driver relies on.
func TestEachLiveReducerIdempotent(t *testing.T) {
	live := []struct {
		name string
		r    reducer
	}{
		{"presence<->shape", reducePresenceShape},
		{"numeric->presence", reduceNumericPresence},
	}
	for _, lr := range live {
		for i, in := range reductionSamples() {
			once := lr.r(in)
			twice := lr.r(once)
			if !nodeEqual(once, twice) {
				t.Fatalf("%s sample %d: reducer not idempotent", lr.name, i)
			}
			if !nodeCovers(in, once) {
				t.Fatalf("%s sample %d: reducer raised an axis above the input", lr.name, i)
			}
		}
	}
}

// TestPlaceholderReducersAreIdentity pins that the deferred reducers are exact
// no-ops: they return their input unchanged so they contribute no fake behavior.
func TestPlaceholderReducersAreIdentity(t *testing.T) {
	placeholders := []reducer{
		reduceOwnershipUpdate,
		reduceEscapeAllocation,
		reduceEvidenceOccurrence,
	}
	for _, r := range placeholders {
		for i, in := range reductionSamples() {
			if r(in) != in {
				t.Fatalf("placeholder reducer changed sample %d; placeholders must be exact identities", i)
			}
		}
	}
}

// TestPresenceShapeContradictionRefinesToBottom pins the presence<->shape reducer
// resolving a contradiction: a definitely-present value over an empty non-nil
// shape cannot exist, so the presence refines to Bottom.
func TestPresenceShapeContradictionRefinesToBottom(t *testing.T) {
	v := build(shapevalue.Bottom(), presence.Present(), numeric.Top())
	if !presence.Equal(v.Presence(), presence.Bottom()) {
		t.Fatalf("present over an empty non-nil shape must refine presence to Bottom, got %s", v.Presence())
	}
}

// TestPresenceAbsentNarrowsShape pins that a definitely-absent slot narrows its
// shape to Bottom: it holds no non-nil structural content.
func TestPresenceAbsentNarrowsShape(t *testing.T) {
	v := build(shapevalue.Of(typ.Number), presence.Absent(), numeric.Top())
	if !v.Shape().IsBottom() {
		t.Fatalf("an absent slot must narrow its shape to Bottom, got %s", v.Shape())
	}
}

// TestEmptyShapeMaybeRefinesToAbsent pins mutual refinement: a Maybe over an empty
// non-nil shape can only be the absent case, so presence sharpens to Absent.
func TestEmptyShapeMaybeRefinesToAbsent(t *testing.T) {
	v := build(shapevalue.Bottom(), presence.Maybe(), numeric.Top())
	if !presence.Equal(v.Presence(), presence.Absent()) {
		t.Fatalf("maybe over an empty non-nil shape must refine to Absent, got %s", v.Presence())
	}
}

// TestPresenceBottomDragsShape pins that an unreachable presence drags the shape
// to Bottom: nothing inhabits the value.
func TestPresenceBottomDragsShape(t *testing.T) {
	v := build(shapevalue.Of(typ.Number), presence.Bottom(), numeric.Top())
	if !v.Shape().IsBottom() {
		t.Fatalf("unreachable presence must drag shape to Bottom, got %s", v.Shape())
	}
}

// TestPresentNonNilShapeUnchanged pins that a consistent value is not perturbed:
// a present non-nil value keeps its shape and presence.
func TestPresentNonNilShapeUnchanged(t *testing.T) {
	v := build(shapevalue.Of(typ.Number), presence.Present(), numeric.Top())
	if !presence.Equal(v.Presence(), presence.Present()) || v.Shape().IsBottom() {
		t.Fatal("a consistent present non-nil value must be unperturbed by reduction")
	}
}

// TestNumericBottomRefinesPresenceToBottom pins the single-node numeric->presence
// reduction: an empty integer set means no integer inhabits the value, so its
// presence refines to Bottom.
func TestNumericBottomRefinesPresenceToBottom(t *testing.T) {
	v := build(shapevalue.Of(typ.Number), presence.Present(), numeric.Bottom())
	if !presence.Equal(v.Presence(), presence.Bottom()) {
		t.Fatalf("empty numeric content must refine presence to Bottom, got %s", v.Presence())
	}
}

// TestReduceAfterJoinConsistency pins that constructing a value through New (which
// reduces) and joining values (which reduces after the component join) agree with
// reducing once: Join already returns a reduced, interned value, so re-admitting
// its components yields the same node.
func TestReduceAfterJoinConsistency(t *testing.T) {
	a := build(shapevalue.Of(typ.Number), presence.Present(), numeric.Top())
	b := build(shapevalue.Of(typ.String), presence.Present(), numeric.Top())

	j := Join(a, b)

	// Re-admit the joined value's own components: the reduced product is closed,
	// so reduction is already at a fixed point and the rebuild interns identically.
	rebuilt := New(
		j.Shape(), j.Presence(), j.Numeric(),
		j.Effects(), j.Ownership(), j.Escape(), j.Identity(), j.Evidence(),
	)
	if rebuilt.n != j.n {
		t.Fatal("re-admitting a joined value's reduced components must intern to the same node")
	}
	if !Equal(rebuilt, j) {
		t.Fatal("re-admitting a joined value must be Equal to it")
	}
}

// TestReducedValuesInternCanonically pins that reduction preserves interning:
// values whose pre-reduction content differs but reduces to the same node share
// one canonical node, and Equal stays consistent with Hash. A present value over
// an empty non-nil shape and an unreachable value both reduce to the same Bottom
// presence over an empty shape.
func TestReducedValuesInternCanonically(t *testing.T) {
	a := build(shapevalue.Bottom(), presence.Present(), numeric.Top())
	b := build(shapevalue.Bottom(), presence.Bottom(), numeric.Top())

	if !Equal(a, b) {
		t.Fatal("values that reduce to the same node must be Equal")
	}
	if a.n != b.n {
		t.Fatal("values that reduce to the same node must intern to one canonical node")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("reduced Equal values must hash identically")
	}
}

// TestReductionPreservesEqualToTop pins that reduction does not perturb Equal
// beyond the lattice order: an already-consistent Top value is unchanged, so it
// stays Equal to a freshly built Top.
func TestReductionPreservesEqualToTop(t *testing.T) {
	if !Equal(Top(), Top()) || Top().n != Top().n {
		t.Fatal("reduction must leave a consistent Top value canonical")
	}
}

// --- index-presence + array-length-bound Phi/Join stress test ---

// arrayLen is the proven minimum length of the container the stress test indexes:
// a literal of three elements installed on the dominating path.
const arrayLen = 3

// indexAt builds an indexed-lookup result whose numeric axis carries an exact
// index value, with Maybe presence (unrefined until ReduceIndexPresence runs).
func indexAt(i int64) AbstractValue {
	return build(shapevalue.Of(typ.Number), presence.Maybe(), numeric.Exact(i))
}

// TestIndexPresenceDominatingInstallIsDefinite pins the dominating-literal-install
// case: a literal three-element array bounds the length to >= 3, so an in-bounds
// index lookup is definite (Present).
func TestIndexPresenceDominatingInstallIsDefinite(t *testing.T) {
	for i := int64(0); i < arrayLen; i++ {
		got := ReduceIndexPresence(indexAt(i), arrayLen)
		if !presence.Equal(got.Presence(), presence.Present()) {
			t.Fatalf("index %d under length %d must be a definite lookup, got %s", i, arrayLen, got.Presence())
		}
	}
}

// TestIndexPresenceOutOfBoundsIsAbsent pins that an index proven outside the
// length bound can never address an element, so the lookup is Absent.
func TestIndexPresenceOutOfBoundsIsAbsent(t *testing.T) {
	got := ReduceIndexPresence(indexAt(arrayLen+2), arrayLen)
	if !presence.Equal(got.Presence(), presence.Absent()) {
		t.Fatalf("an out-of-bounds index must be Absent, got %s", got.Presence())
	}
	neg := build(shapevalue.Of(typ.Number), presence.Maybe(), numeric.Range(-4, -1))
	if got := ReduceIndexPresence(neg, arrayLen); !presence.Equal(got.Presence(), presence.Absent()) {
		t.Fatalf("a negative index must be Absent, got %s", got.Presence())
	}
}

// TestIndexPresenceBranchJoinKeepsDefinite pins the branch join: two in-bounds
// indices from two branches join to an in-bounds interval, so the joined lookup is
// still definite. The presence is refined on each branch before the join.
func TestIndexPresenceBranchJoinKeepsDefinite(t *testing.T) {
	left := ReduceIndexPresence(indexAt(0), arrayLen)
	right := ReduceIndexPresence(indexAt(arrayLen-1), arrayLen)

	if !presence.Equal(left.Presence(), presence.Present()) || !presence.Equal(right.Presence(), presence.Present()) {
		t.Fatal("both in-bounds branches must be definite before the join")
	}

	joined := Join(left, right)
	if !presence.Equal(joined.Presence(), presence.Present()) {
		t.Fatalf("a join of two definite in-bounds lookups must stay definite, got %s", joined.Presence())
	}
	// The joined index interval still lies inside the length bound, so reducing the
	// joined value against the bound agrees with the branch presence.
	reJoined := ReduceIndexPresence(joined, arrayLen)
	if !presence.Equal(reJoined.Presence(), presence.Present()) {
		t.Fatalf("re-reducing the joined lookup must stay definite, got %s", reJoined.Presence())
	}
}

// TestIndexPresenceBranchJoinWithOutOfBoundsIsMaybe pins that joining a definite
// in-bounds branch with an out-of-bounds branch yields an interval that straddles
// the bound, so the merged lookup is no longer definite: presence is Maybe.
func TestIndexPresenceBranchJoinWithOutOfBoundsIsMaybe(t *testing.T) {
	inBounds := indexAt(1)
	outOfBounds := indexAt(arrayLen + 5)

	joined := Join(inBounds, outOfBounds)
	reduced := ReduceIndexPresence(joined, arrayLen)
	if !presence.Equal(reduced.Presence(), presence.Maybe()) {
		t.Fatalf("a join straddling the length bound must be Maybe, got %s", reduced.Presence())
	}
}

// TestNilOverwriteMakesDirectLookupFail pins the nil-overwrite case: once the
// slot is overwritten with nil, the lookup result is definitely absent regardless
// of the index bound, and the presence<->shape reducer drives its shape to Bottom.
func TestNilOverwriteMakesDirectLookupFail(t *testing.T) {
	// The slot holds nil: shape carries no non-nil content (Bottom), presence is
	// Absent. The direct lookup must fail (Absent), not be refined back to Present.
	overwritten := build(shapevalue.Bottom(), presence.Absent(), numeric.Exact(1))
	if !presence.Equal(overwritten.Presence(), presence.Absent()) {
		t.Fatalf("a nil-overwritten slot must stay Absent, got %s", overwritten.Presence())
	}
	if !overwritten.Shape().IsBottom() {
		t.Fatal("a nil-overwritten slot must carry an empty non-nil shape")
	}
	// Even with an in-bounds index, the definitely-absent slot stays Absent: an
	// Absent presence is already the sharpest, and the index reduction only
	// refines a Maybe.
	got := ReduceIndexPresence(overwritten, arrayLen)
	if !presence.Equal(got.Presence(), presence.Absent()) {
		t.Fatalf("a nil-overwritten in-bounds lookup must remain Absent, got %s", got.Presence())
	}
}

// TestIndexPresenceReducedAndInterned pins that ReduceIndexPresence returns a
// reduced, interned value: two independently reduced equal lookups share a node.
func TestIndexPresenceReducedAndInterned(t *testing.T) {
	a := ReduceIndexPresence(indexAt(0), arrayLen)
	b := ReduceIndexPresence(indexAt(0), arrayLen)
	if a.n != b.n {
		t.Fatal("equal index-presence reductions must intern to one node")
	}
	if !presence.Equal(a.Presence(), presence.Present()) {
		t.Fatal("the reduced lookup must be definite")
	}
}

// TestIndexPresenceNoBoundLeavesUnrefined pins that without a proven length bound
// (a non-positive lower bound proves nothing) the presence is left unrefined.
func TestIndexPresenceNoBoundLeavesUnrefined(t *testing.T) {
	got := ReduceIndexPresence(indexAt(1), 0)
	if !presence.Equal(got.Presence(), presence.Maybe()) {
		t.Fatalf("no proven length bound must leave presence unrefined (Maybe), got %s", got.Presence())
	}
}
