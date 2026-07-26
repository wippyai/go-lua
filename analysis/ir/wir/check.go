package wir

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CheckKind tags a normalized condition descriptor stored on branch,
// assertion-call, and boolean-producing expression instructions.
// It is the in-IR projection of a condition already normalized by lowering; the
// IR concludes nothing from it, it only carries the descriptor for the transfer
// interpreter and the textual printer.
//
// wir is the sole owner of this enum: it is upstream IR and analysis/lua's
// syntax-facing packages (branchcond in particular) consume it, never the
// reverse, so branchcond.CheckKind is a type alias of this type rather than a
// second definition. Add new variants here first; CheckKindExhaustivenessTest
// fails the build if the two packages' names or count ever diverge again.
type CheckKind uint8

const (
	CheckNone CheckKind = iota
	CheckTruthy
	CheckFalsy
	CheckNil
	CheckNotNil
	CheckTypeEqual
	CheckTypeNot
	CheckLiteralEqual
	CheckLiteralNot
	CheckPathEqual
	CheckPathNot
	CheckLenGe
	CheckIndexInRange
	CheckNumGe
	CheckNumLe
	CheckFrozenTable
	CheckModResidue
)

// Check is the neutral condition descriptor interned by a Body. Lowering
// produces it from syntax; the IR stores and prints it and never derives values
// from it. Fields carry only path and type identities, both of which live below
// the IR layer, so the descriptor stays free of any syntax dependency.
type Check struct {
	Kind          CheckKind
	Path          path.Path
	OtherPath     path.Path
	TypeName      string
	Literal       typ.Type
	LiteralString string
	LenFloor      int64
	// NumFloor carries the numeric lower bound for CheckNumGe.
	NumFloor       int64
	NumCeil        int64
	HasNumCeil     bool
	NumCeilNegated bool
	// Modulus and Residue carry the two integers of CheckModResidue: the
	// comparison `path % Modulus == Residue`. Modulus is always positive and
	// Residue is already reduced into [0, Modulus-1], which is the range Lua's
	// floor modulo produces for a positive modulus whatever the dividend's sign.
	Modulus int64
	Residue int64
	// Negated is true when the bound holds on the FALSE edge of the comparison
	// rather than the true edge. Only the bound checks (CheckIndexInRange,
	// CheckNumGe, CheckNumLe, CheckLenGe) use it.
	Negated bool
}

// ImpliedCheck records one normalized leaf check proven on a particular outer
// branch edge of a compound condition. Edge is the CFG edge carrying the proof;
// Polarity is the truth value of Check on that edge. It is WIR syntax metadata:
// transfer decides what factflow relations/refinements the proof implies.
type ImpliedCheck struct {
	Check    Check
	Edge     bool
	Polarity bool
}
