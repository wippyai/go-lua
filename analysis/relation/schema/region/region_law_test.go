package region_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func testID(first byte) identity.ContentID {
	var id identity.ContentID
	id[0] = first
	return id
}

func testAtom(t *testing.T, first byte) region.Atom {
	t.Helper()
	atom, ok := region.NewAtom(testID(first))
	if !ok {
		t.Fatalf("NewAtom(%d) failed", first)
	}
	return atom
}

func TestAtomAndTerminalsAreExplicitAndStable(t *testing.T) {
	if atom, ok := region.NewAtom(identity.ContentID{}); ok || atom.Available() || atom.ID().Available() {
		t.Fatal("zero owner-issued identity became an atom")
	}
	atom := testAtom(t, 1)
	if !atom.Available() || atom.ID() != testID(1) {
		t.Fatal("atom did not retain the explicit issued identity")
	}
	falseOne, trueOne := region.False(), region.True()
	falseTwo, trueTwo := region.False(), region.True()
	if !falseOne.Available() || !trueOne.Available() || !falseOne.IsFalse() || !trueOne.IsTrue() {
		t.Fatal("terminals were not sealed")
	}
	if falseOne.Identity() == (identity.ContentID{}) || trueOne.Identity() == (identity.ContentID{}) || falseOne.Identity() == trueOne.Identity() {
		t.Fatal("terminals do not have distinct canonical identities")
	}
	if falseOne.Identity() != falseTwo.Identity() || trueOne.Identity() != trueTwo.Identity() {
		t.Fatal("terminal identity depends on construction call")
	}
}

func TestNewRegionCanonicalizesTransportOrder(t *testing.T) {
	a, b := testAtom(t, 1), testAtom(t, 2)
	forward := []region.Node{
		{Atom: a, Low: 0, High: 3}, // a, rooted at 2
		{Atom: b, Low: 0, High: 1}, // b
	}
	reverse := []region.Node{
		{Atom: b, Low: 0, High: 1}, // b
		{Atom: a, Low: 0, High: 2}, // a, rooted at 3
	}
	first, firstOK := region.NewRegion(forward, 2)
	second, secondOK := region.NewRegion(reverse, 3)
	if !firstOK || !secondOK || !first.Available() || !second.Available() {
		t.Fatal("valid reordered DAG did not seal")
	}
	if first.Identity() != second.Identity() {
		t.Fatal("canonical identity depends on input row order")
	}
	rows := first.Nodes()
	if len(rows) != 2 || rows[0].Atom.ID() != b.ID() || rows[1].Atom.ID() != a.ID() {
		t.Fatalf("canonical postorder = %#v, want b then a", rows)
	}
	rows[0].Low, rows[0].High = 1, 1
	if first.Identity() == (identity.ContentID{}) || first.Nodes()[0].Low != 0 {
		t.Fatal("Nodes returned mutable sealed storage")
	}
}

func TestNewRegionRejectsMalformedGraph(t *testing.T) {
	a, b := testAtom(t, 1), testAtom(t, 2)
	cases := []struct {
		name string
		rows []region.Node
		root uint32
	}{
		{"out-of-range-root", nil, 4},
		{"out-of-range-child", []region.Node{{Atom: a, Low: 0, High: 4}}, 2},
		{"unreduced", []region.Node{{Atom: a, Low: 0, High: 0}}, 2},
		{"cycle", []region.Node{{Atom: a, Low: 2, High: 1}}, 2},
		{"unreachable", []region.Node{{Atom: a, Low: 0, High: 1}, {Atom: b, Low: 0, High: 1}}, 2},
		{"unavailable-atom", []region.Node{{Low: 0, High: 1}}, 2},
		{"wrong-order", []region.Node{{Atom: b, Low: 0, High: 2}, {Atom: a, Low: 0, High: 1}}, 2},
		{"duplicate-node", []region.Node{{Atom: a, Low: 2, High: 3}, {Atom: b, Low: 0, High: 1}, {Atom: b, Low: 0, High: 1}}, 2},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := region.NewRegion(test.rows, test.root); ok || got.Available() {
				t.Fatalf("malformed graph sealed: %#v/%t", got, ok)
			}
		})
	}
}

func TestConjoinAndEntailsAreExact(t *testing.T) {
	a, b := testAtom(t, 1), testAtom(t, 2)
	left, leftOK := region.FromAtom(a)
	right, rightOK := region.FromAtom(b)
	if !leftOK || !rightOK {
		t.Fatal("atom formulas did not seal")
	}
	joined, joinedOK := region.Conjoin(left, right)
	reversed, reversedOK := region.Conjoin(right, left)
	if !joinedOK || !reversedOK || joined.Identity() != reversed.Identity() {
		t.Fatal("conjunction is not canonical and commutative")
	}
	if !region.Entails(joined, left) || !region.Entails(joined, right) || region.Entails(left, joined) {
		t.Fatal("conjunction entailment is not exact")
	}
	if !region.Entails(region.False(), left) || !region.Entails(left, region.True()) {
		t.Fatal("terminal entailment laws failed")
	}
	if region.Entails(region.True(), left) {
		t.Fatal("true incorrectly entails a non-true atom")
	}
	if _, ok := region.Conjoin(left, region.Region{}); ok || region.Entails(left, region.Region{}) {
		t.Fatal("unavailable formula participated in algebra")
	}
	identity, identityOK := region.Conjoin(region.True(), left)
	if !identityOK || !identity.Available() || identity.Identity() != left.Identity() {
		t.Fatal("true conjunction was not an identity")
	}
	idempotent, idempotentOK := region.Conjoin(left, left)
	if !idempotentOK || idempotent.Identity() != left.Identity() {
		t.Fatal("conjunction was not idempotent")
	}
	falseJoin, falseJoinOK := region.Conjoin(region.False(), left)
	if !falseJoinOK || !falseJoin.IsFalse() {
		t.Fatal("false conjunction was not false")
	}
}
