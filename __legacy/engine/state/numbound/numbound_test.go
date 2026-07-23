package numbound

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func keyOf(ks *keyspace.KeySpace, t *testing.T, name string) keyspace.Key {
	t.Helper()
	k, ok := ks.FromStateKey(pathdom.PathKey(name))
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", name)
	}
	return k
}

func lift_mustValues(entries map[keyspace.Key]Floor) lift.MustMapLane[keyspace.Key, Floor] {
	return lift.MustMapValues(entries)
}

func TestElemLatticeLaws(t *testing.T) {
	d := elemDomain()
	neg := Floor{Lo: -7}
	a := Floor{Lo: 1}
	b := Floor{Lo: 3}

	if got := d.Top(); got.Lo != minFloor {
		t.Fatalf("Top must be the unbounded numeric floor; got %d want %d", got.Lo, minFloor)
	}
	if got := d.Bottom(); got.Lo != maxFloor {
		t.Fatalf("Bottom must be the unreachable max floor; got %d want %d", got.Lo, maxFloor)
	}
	if d.Equal(neg, d.Top()) {
		t.Fatalf("negative concrete floors must not collapse to Top")
	}
	if !d.LessOrEq(b, a) {
		t.Fatalf("higher floor must be lower in lattice: %v <= %v expected", b, a)
	}
	if d.LessOrEq(a, b) {
		t.Fatalf("weaker floor must not be below stronger: %v <= %v unexpected", a, b)
	}
	if !d.LessOrEq(a, neg) {
		t.Fatalf("positive floor must be below weaker negative floor: %v <= %v expected", a, neg)
	}
	if got := d.Join(a, b); got.Lo != 1 {
		t.Fatalf("Join must be min(Lo); got %d want 1", got.Lo)
	}
	if got := d.Join(b, a); got.Lo != 1 {
		t.Fatalf("Join must be commutative min; got %d want 1", got.Lo)
	}
	if got := d.Join(neg, a); got.Lo != -7 {
		t.Fatalf("Join must preserve weaker negative floors; got %d want -7", got.Lo)
	}
	if got := d.Meet(a, b); got.Lo != 3 {
		t.Fatalf("Meet must keep stronger floor max(1,3)=3; got %d", got.Lo)
	}
	if got := d.Meet(neg, a); got.Lo != 1 {
		t.Fatalf("Meet must preserve stronger floor max(-7,1)=1; got %d", got.Lo)
	}
	if got := d.Meet(a, d.Join(a, b)); !d.Equal(got, a) {
		t.Fatalf("Meet absorption failed: got %v want %v", got, a)
	}
	if got := d.Join(a, d.Meet(a, b)); !d.Equal(got, a) {
		t.Fatalf("Join absorption failed: got %v want %v", got, a)
	}
	if got := d.Join(d.Bottom(), b); !d.Equal(got, b) {
		t.Fatalf("Join with Bottom must yield the other operand; got %v", got)
	}
	if got := d.Join(d.Top(), b); !d.Equal(got, d.Top()) {
		t.Fatalf("Join with Top must yield Top; got %v", got)
	}
	if !d.LessOrEq(d.Bottom(), b) {
		t.Fatalf("Bottom must be below every element")
	}
	if !d.LessOrEq(neg, d.Top()) {
		t.Fatalf("every concrete floor must be below Top")
	}
	if got := d.Meet(d.Bottom(), b); !d.Equal(got, d.Bottom()) {
		t.Fatalf("Meet with Bottom must yield Bottom; got %v", got)
	}
	if got := d.Meet(d.Top(), b); !d.Equal(got, b) {
		t.Fatalf("Meet with Top must yield the other operand; got %v", got)
	}
}

func TestElemWidenAscendingChainTerminates(t *testing.T) {
	d := elemDomain()
	prev := Floor{Lo: 10}
	for i := int64(9); i > -1000; i-- {
		next := d.Widen(prev, Floor{Lo: i})
		if next.Lo > i {
			t.Fatalf("Widen must not invent a higher floor: got %d", next.Lo)
		}
		if d.Equal(next, prev) {
			if next.Lo != minFloor {
				t.Fatalf("ascending chain must collapse to Top(%d); got %d", minFloor, next.Lo)
			}
			return
		}
		prev = next
	}
	t.Fatalf("ascending Widen chain did not terminate; final %v", prev)
}

func TestMapMustSemantics(t *testing.T) {
	d := MapDomain()
	ks := keyspace.New()
	x := keyOf(ks, t, "x")
	y := keyOf(ks, t, "y")
	a := lift_mustValues(map[keyspace.Key]Floor{
		x: {Lo: -4},
		y: {Lo: 2},
	})
	b := lift_mustValues(map[keyspace.Key]Floor{
		x: {Lo: 7},
	})

	joined := d.Join(a, b)
	vals := joined.Values()
	if len(vals) != 1 {
		t.Fatalf("join must keep only common keys; got %d", len(vals))
	}
	if f, ok := vals[x]; !ok || f.Lo != -4 {
		t.Fatalf("common key floor must be min(-4,7)=-4; got %v ok=%v", f, ok)
	}
	if _, ok := vals[y]; ok {
		t.Fatalf("non-common key must be dropped at the merge")
	}
}

func TestMapMeetConjoinsSupportAndFloors(t *testing.T) {
	d := MapDomain()
	ks := keyspace.New()
	x := keyOf(ks, t, "x")
	y := keyOf(ks, t, "y")
	z := keyOf(ks, t, "z")
	a := lift_mustValues(map[keyspace.Key]Floor{
		x: {Lo: -4},
		y: {Lo: 2},
	})
	b := lift_mustValues(map[keyspace.Key]Floor{
		x: {Lo: 7},
		z: {Lo: -3},
	})

	met := d.Meet(a, b)
	vals := met.Values()
	if len(vals) != 3 || vals[x].Lo != 7 || vals[y].Lo != 2 || vals[z].Lo != -3 {
		t.Fatalf("meet = %#v, want union support with shared max floor", vals)
	}
	if got := d.Meet(d.Bottom(), a); !d.Equal(got, d.Bottom()) {
		t.Fatal("must-map Bottom did not absorb Meet")
	}
	if got := d.Meet(d.Top(), a); !d.Equal(got, a) {
		t.Fatal("must-map Top was not Meet identity")
	}
	if got := d.Meet(a, d.Join(a, b)); !d.Equal(got, a) {
		t.Fatal("must-map Meet absorption failed")
	}
	if got := d.Join(a, d.Meet(a, b)); !d.Equal(got, a) {
		t.Fatal("must-map Join absorption failed")
	}
}

func TestMapWidenTerminates(t *testing.T) {
	d := MapDomain()
	ks := keyspace.New()
	x := keyOf(ks, t, "x")
	acc := lift_mustValues(map[keyspace.Key]Floor{x: {Lo: 10}})
	for i := int64(9); i > -1000; i-- {
		next := lift_mustValues(map[keyspace.Key]Floor{x: {Lo: i}})
		widened := d.Widen(acc, next)
		if d.Equal(widened, acc) {
			floor, ok := widened.Values()[x]
			if !ok || floor.Lo != minFloor {
				t.Fatalf("map Widen chain must stabilize at Top floor; got %v ok=%v", floor, ok)
			}
			return
		}
		acc = widened
	}
	t.Fatalf("map Widen chain did not terminate")
}
