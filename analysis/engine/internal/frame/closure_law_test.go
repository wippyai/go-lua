package frame

import "testing"

func TestFrameAccessClosureIsLeastForwardEqualityClosure(t *testing.T) {
	closure, ok := Compile(Spec{
		Roots: 10,
		Equalities: []Equality{
			{Left: 2, Right: 3},
			{Left: 5, Right: 6},
			{Left: 8, Right: 9},
		},
		Follows: []Follow{
			{From: 1, To: 2},
			{From: 3, To: 4},
			{From: 4, To: 5},
			{From: 6, To: 7},
			{From: 1, To: 2}, // duplicate must not change the relation.
		},
		Projections: []Projection{
			{Known: true, MayRead: []Root{1}},
			{Known: true, MayRead: []Root{8}, MayWrite: []Root{4}},
		},
	})
	if !ok || !closure.Valid() || !closure.Known() {
		t.Fatal("known frame closure")
	}

	assertMembers(t, closure, true, []Root{1, 2, 3, 4, 5, 6, 7, 8, 9})
	assertMembers(t, closure, false, []Root{10})
	assertWrites(t, closure, true, []Root{4, 5, 6, 7})
	assertWrites(t, closure, false, []Root{1, 2, 3, 8, 9, 10})
	if closure.MayRead(0) || closure.MayRead(11) || closure.MayWrite(0) || closure.MayWrite(11) {
		t.Fatal("invalid Root authorized access")
	}
}

func TestFrameAccessClosureIsDeclarationPermutationInvariant(t *testing.T) {
	first, firstOK := Compile(Spec{
		Roots:      8,
		Equalities: []Equality{{Left: 2, Right: 3}, {Left: 6, Right: 7}, {Left: 1, Right: 1}},
		Follows: []Follow{
			{From: 1, To: 2}, {From: 3, To: 4}, {From: 4, To: 6}, {From: 5, To: 8}, {From: 1, To: 2},
		},
		Projections: []Projection{
			{Known: true, MayRead: []Root{1, 5}},
			{Known: true, MayWrite: []Root{4, 5}},
		},
	})
	second, secondOK := Compile(Spec{
		Roots:      8,
		Equalities: []Equality{{Left: 1, Right: 1}, {Left: 7, Right: 6}, {Left: 3, Right: 2}},
		Follows: []Follow{
			{From: 1, To: 2}, {From: 5, To: 8}, {From: 4, To: 6}, {From: 3, To: 4}, {From: 1, To: 2},
		},
		Projections: []Projection{
			{Known: true, MayWrite: []Root{5, 4}},
			{Known: true, MayRead: []Root{5, 1}},
		},
	})
	if !firstOK || !secondOK || !first.Known() || !second.Known() {
		t.Fatal("permuted closure compilation")
	}
	for root := Root(1); root <= 8; root++ {
		if first.MayRead(root) != second.MayRead(root) || first.MayWrite(root) != second.MayWrite(root) {
			t.Fatalf("Root %d changed under declaration permutation", root)
		}
	}
}

func TestFrameAccessClosureUnknownFailsClosedWithoutReadingItsLists(t *testing.T) {
	closure, ok := Compile(Spec{
		Roots:      3,
		Equalities: []Equality{{Left: 1, Right: 2}},
		Follows:    []Follow{{From: 2, To: 3}},
		Projections: []Projection{
			{Known: true, MayRead: []Root{1}, MayWrite: []Root{3}},
			// These roots are deliberately invalid. An unknown row has no
			// bounded root interpretation, so Compile must not read its lists.
			{Known: false, MayRead: []Root{0, 99}, MayWrite: []Root{88}},
		},
	})
	if !ok || !closure.Valid() || closure.Known() {
		t.Fatal("unknown closure disposition")
	}
	for root := Root(0); root <= 4; root++ {
		if closure.MayRead(root) || closure.MayWrite(root) {
			t.Fatalf("unknown closure authorized Root %d", root)
		}
	}

	// Unknown does not suppress validation of a known row.
	if closure, ok := Compile(Spec{Roots: 3, Projections: []Projection{
		{Known: false, MayRead: []Root{99}},
		{Known: true, MayRead: []Root{0}},
	}}); ok || closure != nil {
		t.Fatal("unknown row masked malformed known projection")
	}
}

func TestFrameAccessClosureRejectsMalformedDeclarations(t *testing.T) {
	for _, test := range []struct {
		name string
		spec Spec
	}{
		{name: "negative-root-count", spec: Spec{Roots: -1}},
		{name: "zero-equality-root", spec: Spec{Roots: 2, Equalities: []Equality{{Left: 0, Right: 1}}}},
		{name: "outside-equality-root", spec: Spec{Roots: 2, Equalities: []Equality{{Left: 1, Right: 3}}}},
		{name: "zero-follow-source", spec: Spec{Roots: 2, Follows: []Follow{{From: 0, To: 1}}}},
		{name: "outside-follow-target", spec: Spec{Roots: 2, Follows: []Follow{{From: 1, To: 3}}}},
		{name: "zero-known-read", spec: Spec{Roots: 2, Projections: []Projection{{Known: true, MayRead: []Root{0}}}}},
		{name: "outside-known-write", spec: Spec{Roots: 2, Projections: []Projection{{Known: true, MayWrite: []Root{3}}}}},
		{name: "invalid-global-edge-still-rejected-when-unknown", spec: Spec{Roots: 2, Follows: []Follow{{From: 1, To: 3}}, Projections: []Projection{{Known: false}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			closure, ok := Compile(test.spec)
			if ok || closure != nil {
				t.Fatal("malformed declaration accepted")
			}
		})
	}
}

func assertMembers(t testing.TB, closure *Closure, want bool, roots []Root) {
	t.Helper()
	for _, root := range roots {
		if closure.MayRead(root) != want {
			t.Fatalf("MayRead(%d) = %v, want %v", root, closure.MayRead(root), want)
		}
	}
}

func assertWrites(t testing.TB, closure *Closure, want bool, roots []Root) {
	t.Helper()
	for _, root := range roots {
		if closure.MayWrite(root) != want {
			t.Fatalf("MayWrite(%d) = %v, want %v", root, closure.MayWrite(root), want)
		}
	}
}
