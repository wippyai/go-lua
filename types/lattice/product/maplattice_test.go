package product

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/types/lattice"
)

// mapSample builds a structural cross-section of the map[string]sign lattice:
// Bottom (empty), the Top sentinel, single-key cells at several element
// values, multi-key cells, overlapping and disjoint key sets, and a cell with
// an explicit bottom value (which must canonicalize to absence).
func mapSample(d lattice.Lattice[map[string]sign]) []map[string]sign {
	return []map[string]sign{
		d.Bottom(),
		d.Top(),
		{},                                 // explicit empty == Bottom
		{"x": sNeg},                        // single key
		{"x": sPos},                        // same key, different value
		{"y": sZero},                       // disjoint key
		{"x": sTop},                        // element top at a key
		{"x": sNeg, "y": sPos},             // two keys
		{"x": sPos, "y": sZero},            // overlapping keys, differ on x
		{"y": sPos, "z": sNeg},             // partial overlap with above
		{"x": sNeg, "y": sPos, "z": sZero}, // three keys
		{"x": sBottom},                     // explicit bottom -> canonical absence
		{"x": sNeg, "y": sBottom},          // mixed explicit bottom
	}
}

func formatMap(m map[string]sign) string {
	if m == nil {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%s:%d", k, m[k])
	}
	return out + "}"
}

// TestMapLattice_Laws drives the full law suite over map[string]sign. The
// element lattice provides a Meet, so the map lattice's Meet is present and
// the meet-side laws run. The sample includes the Top sentinel and an
// explicit-bottom cell to exercise the canonicalization.
func TestMapLattice_Laws(t *testing.T) {
	d := MapLattice[string, sign](signLattice())
	if d.Meet == nil {
		t.Fatalf("MapLattice over a meet-bearing element must provide Meet")
	}
	suite := lattice.LawSuite[map[string]sign]{
		Name:   "product.MapLattice(string,sign)",
		Domain: d,
		Sample: mapSample(d),
		Format: formatMap,
	}
	suite.Run(t)
}

// TestMapLattice_MeetNilPropagation pins that a forward-only element lattice
// (Meet == nil) makes the map lattice forward-only, and the law suite still
// runs (skipping meet-side laws).
func TestMapLattice_MeetNilPropagation(t *testing.T) {
	forward := signLattice()
	forward.Meet = nil
	d := MapLattice[string, sign](forward)
	if d.Meet != nil {
		t.Fatalf("MapLattice over a forward-only element must have nil Meet")
	}
	suite := lattice.LawSuite[map[string]sign]{
		Name:   "product.MapLattice(forward)",
		Domain: d,
		Sample: mapSample(d),
		Format: formatMap,
	}
	suite.Run(t)
}

// TestMapLattice_AbsentKeyIsBottom pins the central semantic invariant: an
// absent key denotes elem.Bottom(), so a map with an explicit bottom entry is
// Equal to the same map without that key, and Join/Widen drop bottom-valued
// keys (canonicalization).
func TestMapLattice_AbsentKeyIsBottom(t *testing.T) {
	d := MapLattice[string, sign](signLattice())

	withExplicit := map[string]sign{"x": sNeg, "y": sBottom}
	withoutKey := map[string]sign{"x": sNeg}
	if !d.Equal(withExplicit, withoutKey) {
		t.Errorf("explicit bottom entry must Equal absence: %s != %s",
			formatMap(withExplicit), formatMap(withoutKey))
	}

	// Bottom is the empty map; an all-bottom map Equals Bottom.
	allBottom := map[string]sign{"x": sBottom, "y": sBottom}
	if !d.Equal(allBottom, d.Bottom()) {
		t.Errorf("all-bottom map must Equal Bottom: %s", formatMap(allBottom))
	}

	// Join over a key whose values join to bottom drops the key.
	j := d.Join(map[string]sign{"x": sBottom}, map[string]sign{"x": sBottom})
	if len(j) != 0 {
		t.Errorf("Join of bottom-valued cells must canonicalize to empty, got %s", formatMap(j))
	}

	// Join is the pointwise least upper bound: union of keys, element join per key.
	a := map[string]sign{"x": sNeg, "y": sZero}
	b := map[string]sign{"x": sPos, "z": sPos}
	got := d.Join(a, b)
	want := map[string]sign{"x": sTop, "y": sZero, "z": sPos} // Join(sNeg,sPos)=sTop
	if !d.Equal(got, want) {
		t.Errorf("Join %s ⊔ %s = %s, want %s", formatMap(a), formatMap(b), formatMap(got), formatMap(want))
	}
}

// TestMapLattice_Order pins the pointwise partial order, especially that a
// finite map is strictly below the Top sentinel and that Bottom is below all.
func TestMapLattice_Order(t *testing.T) {
	d := MapLattice[string, sign](signLattice())
	top := d.Top()
	bot := d.Bottom()

	finite := map[string]sign{"x": sNeg}

	if !d.LessOrEq(bot, finite) {
		t.Errorf("Bottom ⊑ finite must hold")
	}
	if !d.LessOrEq(finite, top) {
		t.Errorf("finite ⊑ Top must hold")
	}
	if d.LessOrEq(top, finite) {
		t.Errorf("Top ⊑ finite must NOT hold")
	}
	if !d.LessOrEq(top, top) {
		t.Errorf("Top ⊑ Top must hold (reflexivity)")
	}
	if !d.Equal(top, top) {
		t.Errorf("Top Equal Top must hold")
	}
	if d.Equal(top, finite) {
		t.Errorf("Top must not Equal a finite map")
	}

	// Pointwise: {x:sNeg} ⊑ {x:sTop} but not ⊑ {x:sPos} (sNeg, sPos incomparable).
	if !d.LessOrEq(map[string]sign{"x": sNeg}, map[string]sign{"x": sTop}) {
		t.Errorf("{x:sNeg} ⊑ {x:sTop} must hold")
	}
	if d.LessOrEq(map[string]sign{"x": sNeg}, map[string]sign{"x": sPos}) {
		t.Errorf("{x:sNeg} ⊑ {x:sPos} must NOT hold")
	}
}

// TestMapLattice_TopAbsorbing pins the sentinel algebra: Top absorbs Join and
// Widen, and is the identity for Meet.
func TestMapLattice_TopAbsorbing(t *testing.T) {
	d := MapLattice[string, sign](signLattice())
	top := d.Top()
	x := map[string]sign{"x": sNeg, "y": sPos}

	if !d.Equal(d.Join(top, x), top) || !d.Equal(d.Join(x, top), top) {
		t.Errorf("Top must absorb Join")
	}
	if !d.Equal(d.Widen(top, x), top) || !d.Equal(d.Widen(x, top), top) {
		t.Errorf("Top must absorb Widen")
	}
	// Meet(Top, x) = x.
	if !d.Equal(d.Meet(top, x), x) || !d.Equal(d.Meet(x, top), x) {
		t.Errorf("Top must be identity for Meet")
	}
	// Meet(Top, Top) = Top.
	if !d.Equal(d.Meet(top, top), top) {
		t.Errorf("Meet(Top,Top) must be Top")
	}
}
