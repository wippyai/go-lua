package lattice

import "testing"

type tinyChain int

const (
	tinyBottom tinyChain = iota
	tinyMid
	tinyTop
)

func tinyLattice() Lattice[tinyChain] {
	return Lattice[tinyChain]{
		Bottom: func() tinyChain { return tinyBottom },
		Top:    func() tinyChain { return tinyTop },
		Equal: func(a, b tinyChain) bool {
			return a == b
		},
		LessOrEq: func(a, b tinyChain) bool {
			return a == tinyBottom || b == tinyTop || a == b
		},
		Join: func(a, b tinyChain) tinyChain {
			if a == b {
				return a
			}
			if a == tinyBottom {
				return b
			}
			if b == tinyBottom {
				return a
			}
			return tinyTop
		},
		Widen: func(prev, next tinyChain) tinyChain {
			if prev == next {
				return prev
			}
			return tinyTop
		},
	}
}

func TestLatticeShape_AllowsOptionalMeet(t *testing.T) {
	d := tinyLattice()

	if d.Bottom == nil || d.Top == nil || d.Equal == nil || d.LessOrEq == nil || d.Join == nil || d.Widen == nil {
		t.Fatalf("lattice shape is missing required hooks: %#v", d)
	}
	if d.Meet != nil {
		t.Fatalf("expected Meet to be optional for forward-only domains")
	}

	bottom := d.Bottom()
	top := d.Top()
	mid := tinyMid

	if !d.Equal(bottom, bottom) || !d.Equal(top, top) || d.Equal(bottom, top) {
		t.Fatalf("equality contract not preserved")
	}
	if !d.LessOrEq(bottom, mid) || !d.LessOrEq(mid, top) || d.LessOrEq(top, mid) {
		t.Fatalf("order contract not preserved")
	}
	if got := d.Join(bottom, mid); got != mid {
		t.Fatalf("Join(bottom, mid) = %v, want %v", got, mid)
	}
	if got := d.Widen(bottom, mid); got != tinyTop {
		t.Fatalf("Widen(bottom, mid) = %v, want %v", got, tinyTop)
	}
}
