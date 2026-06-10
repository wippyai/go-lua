package product

import (
	"fmt"
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

// product2Sample builds the full Cartesian sample of sign x sign so the law
// suite exercises every component combination.
func product2Sample() []Pair[sign, sign] {
	var out []Pair[sign, sign]
	for _, a := range signSample() {
		for _, b := range signSample() {
			out = append(out, Pair[sign, sign]{A: a, B: b})
		}
	}
	return out
}

func formatPair2(p Pair[sign, sign]) string {
	return fmt.Sprintf("(%d,%d)", p.A, p.B)
}

// TestProduct2_Laws drives the full lattice law suite over sign x sign. Both
// components provide a Meet, so the product's Meet is present and the
// meet-side laws (including absorption) run.
func TestProduct2_Laws(t *testing.T) {
	d := Product2(signLattice(), signLattice())
	if d.Meet == nil {
		t.Fatalf("Product2 of two meet-bearing lattices must provide Meet")
	}
	suite := latticelaws.LawSuite[Pair[sign, sign]]{
		Name:   "product.Product2(sign,sign)",
		Domain: d,
		Sample: product2Sample(),
		Format: formatPair2,
	}
	suite.Run(t)
}

// TestProduct2_MeetNilPropagation pins that a forward-only component (Meet ==
// nil) makes the product forward-only, and that the law harness tolerates it.
func TestProduct2_MeetNilPropagation(t *testing.T) {
	forward := signLattice()
	forward.Meet = nil

	// Either component forward-only -> product Meet nil.
	if d := Product2(forward, signLattice()); d.Meet != nil {
		t.Errorf("Product2(forward, full).Meet should be nil")
	}
	if d := Product2(signLattice(), forward); d.Meet != nil {
		t.Errorf("Product2(full, forward).Meet should be nil")
	}

	d := Product2(forward, forward)
	if d.Meet != nil {
		t.Fatalf("Product2(forward, forward).Meet should be nil")
	}
	suite := latticelaws.LawSuite[Pair[sign, sign]]{
		Name:   "product.Product2(forward,forward)",
		Domain: d,
		Sample: product2Sample(),
		Format: formatPair2,
	}
	suite.Run(t)
}

// product3Sample builds a representative sample of the nested triple product.
func product3Sample() []Pair[sign, Pair[sign, sign]] {
	var out []Pair[sign, Pair[sign, sign]]
	for _, a := range signSample() {
		for _, b := range signSample() {
			for _, c := range signSample() {
				out = append(out, Pair[sign, Pair[sign, sign]]{
					A: a,
					B: Pair[sign, sign]{A: b, B: c},
				})
			}
		}
	}
	return out
}

// TestProduct3_Laws drives the law suite over the nested triple product to
// confirm Product2 nesting composes soundly.
func TestProduct3_Laws(t *testing.T) {
	d := Product3(signLattice(), signLattice(), signLattice())
	suite := latticelaws.LawSuite[Pair[sign, Pair[sign, sign]]]{
		Name:   "product.Product3(sign,sign,sign)",
		Domain: d,
		Sample: product3Sample(),
		Format: func(p Pair[sign, Pair[sign, sign]]) string {
			return fmt.Sprintf("(%d,(%d,%d))", p.A, p.B.A, p.B.B)
		},
	}
	suite.Run(t)
}

// TestProduct2_Componentwise spot-checks the componentwise definitions
// directly, independent of the law suite.
func TestProduct2_Componentwise(t *testing.T) {
	d := Product2(signLattice(), signLattice())

	bot := d.Bottom()
	if bot.A != sBottom || bot.B != sBottom {
		t.Errorf("Bottom = %v, want (sBottom,sBottom)", bot)
	}
	top := d.Top()
	if top.A != sTop || top.B != sTop {
		t.Errorf("Top = %v, want (sTop,sTop)", top)
	}

	x := Pair[sign, sign]{A: sNeg, B: sPos}
	y := Pair[sign, sign]{A: sPos, B: sPos}
	j := d.Join(x, y)
	// A: Join(sNeg,sPos)=sTop; B: Join(sPos,sPos)=sPos.
	if j.A != sTop || j.B != sPos {
		t.Errorf("Join((sNeg,sPos),(sPos,sPos)) = %v, want (sTop,sPos)", j)
	}
	m := d.Meet(x, y)
	// A: Meet(sNeg,sPos)=sBottom; B: Meet(sPos,sPos)=sPos.
	if m.A != sBottom || m.B != sPos {
		t.Errorf("Meet((sNeg,sPos),(sPos,sPos)) = %v, want (sBottom,sPos)", m)
	}
}
