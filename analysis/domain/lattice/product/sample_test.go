package product

import "github.com/wippyai/go-lua/analysis/domain/lattice"

// sign is a tiny finite element lattice used to drive the combinator law
// suites. It is the standard sign lattice:
//
//	     sTop
//	   /   |   \
//	sNeg  sZero sPos
//	   \   |   /
//	    sBottom
//
// The three middle elements are pairwise incomparable; Join of two distinct
// middle elements is sTop, Meet is sBottom. Finite height, so Widen = Join is
// a valid widening. It mirrors the in-package presence lattice used by the
// law harness's own tests, kept local so these tests stand alone.
type sign int

const (
	sBottom sign = iota
	sNeg
	sZero
	sPos
	sTop
)

func signEqual(a, b sign) bool { return a == b }

func signLessOrEq(a, b sign) bool {
	if a == sBottom || b == sTop {
		return true
	}
	return a == b
}

func signJoin(a, b sign) sign {
	if a == b {
		return a
	}
	if a == sBottom {
		return b
	}
	if b == sBottom {
		return a
	}
	return sTop
}

func signMeet(a, b sign) sign {
	if a == b {
		return a
	}
	if a == sTop {
		return b
	}
	if b == sTop {
		return a
	}
	return sBottom
}

// signLattice is the canonical Lattice for the sign domain. Finite height →
// Widen = Join.
func signLattice() lattice.Lattice[sign] {
	return lattice.Lattice[sign]{
		Bottom:   func() sign { return sBottom },
		Top:      func() sign { return sTop },
		Equal:    signEqual,
		LessOrEq: signLessOrEq,
		Join:     signJoin,
		Meet:     signMeet,
		Widen:    signJoin,
	}
}

// signSample is a representative cross-section of the sign lattice.
func signSample() []sign {
	return []sign{sBottom, sNeg, sZero, sPos, sTop}
}
