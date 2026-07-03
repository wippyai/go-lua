package lift

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func mustMapSample() []MustMapLane[string, sign] {
	return []MustMapLane[string, sign]{
		MustMapBottom[string, sign](),
		MustMapValues[string, sign](nil),
		MustMapValues(map[string]sign{"x": sNeg}),
		MustMapValues(map[string]sign{"x": sPos}),
		MustMapValues(map[string]sign{"y": sZero}),
		MustMapValues(map[string]sign{"x": sNeg, "y": sPos}),
		MustMapValues(map[string]sign{"x": sPos, "y": sZero}),
		MustMapValues(map[string]sign{"y": sPos, "z": sNeg}),
		MustMapValues(map[string]sign{"x": sNeg, "y": sPos, "z": sZero}),
	}
}

func formatMustMap(l MustMapLane[string, sign]) string {
	if l.Bottom() {
		return "bottom"
	}
	values := l.Values()
	if len(values) == 0 {
		return "top"
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%s:%d", k, values[k])
	}
	return out + "}"
}

func TestMustMap_Laws(t *testing.T) {
	d := MustMap[string, sign](signLattice())
	suite := latticelaws.LawSuite[MustMapLane[string, sign]]{
		Name:   "lift.MustMap(string,sign)",
		Domain: d,
		Sample: mustMapSample(),
		Format: formatMustMap,
	}
	suite.Run(t)
}

func TestMustMap_CommonKeysCombineValues(t *testing.T) {
	d := MustMap[string, sign](signLattice())
	a := MustMapValues(map[string]sign{
		"common":   sNeg,
		"leftOnly": sPos,
	})
	b := MustMapValues(map[string]sign{
		"common":    sPos,
		"rightOnly": sZero,
	})

	joined := d.Join(a, b)
	wantJoin := MustMapValues(map[string]sign{"common": sTop})
	if !d.Equal(joined, wantJoin) {
		t.Fatalf("Join = %s, want %s", formatMustMap(joined), formatMustMap(wantJoin))
	}

	widened := d.Widen(a, b)
	wantWiden := MustMapValues(map[string]sign{"common": sTop})
	if !d.Equal(widened, wantWiden) {
		t.Fatalf("Widen = %s, want %s", formatMustMap(widened), formatMustMap(wantWiden))
	}
}

func TestMustMap_WidenPreservesOperandOrderWhenIteratingSmallerSide(t *testing.T) {
	d := MustMap[string, int](lattice.Lattice[int]{
		Bottom:   func() int { return 0 },
		Top:      func() int { return 999 },
		Equal:    func(a, b int) bool { return a == b },
		LessOrEq: func(a, b int) bool { return a <= b },
		Join: func(a, b int) int {
			if a > b {
				return a
			}
			return b
		},
		Widen: func(prev, next int) int {
			return prev*10 + next
		},
	})
	prev := MustMapValues(map[string]int{"common": 2, "prevOnly": 8})
	next := MustMapValues(map[string]int{"common": 3})

	got := d.Widen(prev, next)
	if got.Values()["common"] != 23 {
		t.Fatalf("Widen common = %d, want directional prev,next result 23", got.Values()["common"])
	}
}

func TestMustMap_BottomTopOrderEquality(t *testing.T) {
	d := MustMap[string, sign](signLattice())
	bottom := d.Bottom()
	top := d.Top()
	finite := MustMapValues(map[string]sign{"x": sNeg})
	sameFinite := MustMapValues(map[string]sign{"x": sNeg})
	stricter := MustMapValues(map[string]sign{"x": sNeg, "y": sPos})

	if !bottom.Bottom() || top.Bottom() {
		t.Fatalf("Bottom/Top bottom flags not preserved")
	}
	if !d.Equal(finite, sameFinite) || d.Equal(finite, top) || d.Equal(bottom, top) {
		t.Fatalf("unexpected equality behavior")
	}
	if !d.LessOrEq(bottom, finite) || !d.LessOrEq(finite, top) {
		t.Fatalf("bottom/top order failed")
	}
	if !d.LessOrEq(stricter, finite) || d.LessOrEq(finite, stricter) {
		t.Fatalf("must-map reverse key order failed")
	}
}

func TestMustMap_CloneIsolation(t *testing.T) {
	original := MustMapValues(map[string]sign{"x": sNeg})
	clone := original.Clone()

	clone.Values()["x"] = sPos
	if original.Values()["x"] != sNeg {
		t.Fatalf("Clone shared map with original")
	}
}

func TestMustMap_IdentityCasesReusePersistentValue(t *testing.T) {
	d := MustMap[string, sign](signLattice())
	original := MustMapValues(map[string]sign{"x": sNeg})

	joined := d.Join(original, original)
	if reflect.ValueOf(joined.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Join(finite, same finite) cloned persistent map")
	}
	widened := d.Widen(original, original)
	if reflect.ValueOf(widened.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Widen(finite, same finite) cloned persistent map")
	}
	bottomJoined := d.Join(d.Bottom(), original)
	if reflect.ValueOf(bottomJoined.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Join(bottom, finite) cloned persistent map")
	}
	bottomWidened := d.Widen(d.Bottom(), original)
	if reflect.ValueOf(bottomWidened.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Widen(bottom, finite) cloned persistent map")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = d.Join(d.Bottom(), original)
		_ = d.Join(original, d.Bottom())
		_ = d.Widen(d.Bottom(), original)
		_ = d.Widen(original, d.Bottom())
	}); allocs != 0 {
		t.Fatalf("must-map bottom identity allocated %.1f times, want operand reuse", allocs)
	}
}

func mustSetSample() []MustSetLane[string] {
	return []MustSetLane[string]{
		MustSetBottom[string](),
		MustSetValues[string](nil),
		MustSetValues(map[string]struct{}{"x": {}}),
		MustSetValues(map[string]struct{}{"y": {}}),
		MustSetValues(map[string]struct{}{"x": {}, "y": {}}),
		MustSetValues(map[string]struct{}{"y": {}, "z": {}}),
		MustSetValues(map[string]struct{}{"x": {}, "y": {}, "z": {}}),
	}
}

func formatMustSet(l MustSetLane[string]) string {
	if l.Bottom() {
		return "bottom"
	}
	values := l.Values()
	if len(values) == 0 {
		return "top"
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprintf("%v", keys)
}

func TestMustSet_Laws(t *testing.T) {
	d := MustSet[string]()
	suite := latticelaws.LawSuite[MustSetLane[string]]{
		Name:   "lift.MustSet(string)",
		Domain: d,
		Sample: mustSetSample(),
		Format: formatMustSet,
	}
	suite.Run(t)
}

func TestMustSet_Intersection(t *testing.T) {
	d := MustSet[string]()
	a := MustSetValues(map[string]struct{}{"common": {}, "leftOnly": {}})
	b := MustSetValues(map[string]struct{}{"common": {}, "rightOnly": {}})
	want := MustSetValues(map[string]struct{}{"common": {}})

	joined := d.Join(a, b)
	if !d.Equal(joined, want) {
		t.Fatalf("Join = %s, want %s", formatMustSet(joined), formatMustSet(want))
	}
	widened := d.Widen(a, b)
	if !d.Equal(widened, want) {
		t.Fatalf("Widen = %s, want %s", formatMustSet(widened), formatMustSet(want))
	}
}

func TestMustSet_BottomTopOrderEquality(t *testing.T) {
	d := MustSet[string]()
	bottom := d.Bottom()
	top := d.Top()
	finite := MustSetValues(map[string]struct{}{"x": {}})
	sameFinite := MustSetValues(map[string]struct{}{"x": {}})
	stricter := MustSetValues(map[string]struct{}{"x": {}, "y": {}})

	if !bottom.Bottom() || top.Bottom() {
		t.Fatalf("Bottom/Top bottom flags not preserved")
	}
	if !d.Equal(finite, sameFinite) || d.Equal(finite, top) || d.Equal(bottom, top) {
		t.Fatalf("unexpected equality behavior")
	}
	if !d.LessOrEq(bottom, finite) || !d.LessOrEq(finite, top) {
		t.Fatalf("bottom/top order failed")
	}
	if !d.LessOrEq(stricter, finite) || d.LessOrEq(finite, stricter) {
		t.Fatalf("must-set reverse subset order failed")
	}
}

func TestMustSet_CloneIsolation(t *testing.T) {
	original := MustSetValues(map[string]struct{}{"x": {}})
	clone := original.Clone()

	clone.Values()["y"] = struct{}{}
	if _, ok := original.Values()["y"]; ok {
		t.Fatalf("Clone shared set with original")
	}
}

func TestMustSet_IdentityCasesReusePersistentValue(t *testing.T) {
	d := MustSet[string]()
	original := MustSetValues(map[string]struct{}{"x": {}})

	joined := d.Join(original, original)
	if reflect.ValueOf(joined.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Join(finite, same finite) cloned persistent set")
	}
	widened := d.Widen(original, original)
	if reflect.ValueOf(widened.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Widen(finite, same finite) cloned persistent set")
	}
	bottomJoined := d.Join(d.Bottom(), original)
	if reflect.ValueOf(bottomJoined.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Join(bottom, finite) cloned persistent set")
	}
	bottomWidened := d.Widen(d.Bottom(), original)
	if reflect.ValueOf(bottomWidened.Values()).Pointer() != reflect.ValueOf(original.Values()).Pointer() {
		t.Fatalf("Widen(bottom, finite) cloned persistent set")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = d.Join(d.Bottom(), original)
		_ = d.Join(original, d.Bottom())
		_ = d.Widen(d.Bottom(), original)
		_ = d.Widen(original, d.Bottom())
	}); allocs != 0 {
		t.Fatalf("must-set bottom identity allocated %.1f times, want operand reuse", allocs)
	}
}
