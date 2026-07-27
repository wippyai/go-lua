package lenbound

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
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
	a := Floor{Lo: 1}
	b := Floor{Lo: 3}

	if !d.LessOrEq(b, a) {
		t.Fatalf("higher floor must be lower in lattice: %v <= %v expected", b, a)
	}
	if d.LessOrEq(a, b) {
		t.Fatalf("weaker floor must not be below stronger: %v <= %v unexpected", a, b)
	}
	if got := d.Join(a, b); got.Lo != 1 {
		t.Fatalf("Join must be min(Lo); got %d want 1", got.Lo)
	}
	if got := d.Join(b, a); got.Lo != 1 {
		t.Fatalf("Join must be commutative min; got %d want 1", got.Lo)
	}
	if got := d.Meet(a, b); got.Lo != 3 {
		t.Fatalf("Meet must be max(Lo); got %d want 3", got.Lo)
	}
	if got := d.Meet(b, a); got.Lo != 3 {
		t.Fatalf("Meet must be commutative max; got %d want 3", got.Lo)
	}
	if got := d.Meet(a, d.Join(a, b)); !d.Equal(got, a) {
		t.Fatalf("Meet absorption failed: got %v want %v", got, a)
	}
	if got := d.Join(a, d.Meet(a, b)); !d.Equal(got, a) {
		t.Fatalf("Join absorption failed: got %v want %v", got, a)
	}
	// Bottom is the strongest floor; Top is no floor.
	if got := d.Join(d.Bottom(), b); !d.Equal(got, b) {
		t.Fatalf("Join with Bottom must yield the other operand; got %v", got)
	}
	if got := d.Join(d.Top(), b); !d.Equal(got, d.Top()) {
		t.Fatalf("Join with Top must yield Top; got %v", got)
	}
	if !d.LessOrEq(d.Bottom(), b) {
		t.Fatalf("Bottom must be below every element")
	}
	if !d.LessOrEq(b, d.Top()) {
		t.Fatalf("every element must be below Top")
	}
	if got := d.Meet(d.Bottom(), b); !d.Equal(got, d.Bottom()) {
		t.Fatalf("Meet with Bottom must yield Bottom; got %v", got)
	}
	if got := d.Meet(d.Top(), b); !d.Equal(got, b) {
		t.Fatalf("Meet with Top must yield the other operand; got %v", got)
	}
}

// TestElemWidenAscendingChainTerminates verifies that repeated Widen of a
// strictly increasing sequence of floors stabilizes within two steps: the first
// concrete floor, then a collapse to Top on the next strict increase.
func TestElemWidenAscendingChainTerminates(t *testing.T) {
	d := elemDomain()
	// Drive a strictly increasing back-edge sequence and assert it terminates.
	prev := Floor{Lo: 1}
	for i := int64(2); i < 1000; i++ {
		next := d.Widen(prev, Floor{Lo: i})
		if next.Lo > i {
			t.Fatalf("Widen must not invent a higher floor: got %d", next.Lo)
		}
		if d.Equal(next, prev) {
			// Stabilized.
			if next.Lo != 0 {
				t.Fatalf("ascending chain must collapse to Top(0); got %d", next.Lo)
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
	xs := keyOf(ks, t, "xs")
	ys := keyOf(ks, t, "ys")
	a := lift_mustValues(map[keyspace.Key]Floor{
		xs: {Lo: 1},
		ys: {Lo: 2},
	})
	b := lift_mustValues(map[keyspace.Key]Floor{
		xs: {Lo: 3},
	})

	joined := d.Join(a, b)
	vals := joined.Values()
	// Must semantics: only keys on both edges survive; floor is the min.
	if len(vals) != 1 {
		t.Fatalf("join must keep only common keys; got %d", len(vals))
	}
	if f, ok := vals[xs]; !ok || f.Lo != 1 {
		t.Fatalf("common key floor must be min(1,3)=1; got %v ok=%v", f, ok)
	}
	if _, ok := vals[ys]; ok {
		t.Fatalf("non-common key must be dropped at the merge")
	}
}

func TestMapMeetConjoinsSupportAndFloors(t *testing.T) {
	d := MapDomain()
	ks := keyspace.New()
	xs := keyOf(ks, t, "xs")
	ys := keyOf(ks, t, "ys")
	zs := keyOf(ks, t, "zs")
	a := lift_mustValues(map[keyspace.Key]Floor{
		xs: {Lo: 1},
		ys: {Lo: 2},
	})
	b := lift_mustValues(map[keyspace.Key]Floor{
		xs: {Lo: 3},
		zs: {Lo: 4},
	})

	met := d.Meet(a, b)
	vals := met.Values()
	if len(vals) != 3 || vals[xs].Lo != 3 || vals[ys].Lo != 2 || vals[zs].Lo != 4 {
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
	xs := keyOf(ks, t, "xs")
	acc := lift_mustValues(map[keyspace.Key]Floor{xs: {Lo: 1}})
	for i := int64(2); i < 1000; i++ {
		next := lift_mustValues(map[keyspace.Key]Floor{xs: {Lo: i}})
		widened := d.Widen(acc, next)
		if d.Equal(widened, acc) {
			return
		}
		acc = widened
	}
	t.Fatalf("map Widen chain did not terminate")
}
