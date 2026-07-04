package cir

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CheckKind tags the normalized branch-condition descriptor stored on OpBranch.
// It is the in-IR projection of a condition already normalized by lowering; the
// IR concludes nothing from it, it only carries the descriptor for the transfer
// interpreter and the textual printer.
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
)

// Check is the neutral branch-condition descriptor interned by a Body. Lowering
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
	NumFloor int64
	// Negated is true when the bound holds on the FALSE edge of the comparison
	// rather than the true edge. Only the bound checks (CheckIndexInRange,
	// CheckNumGe, CheckLenGe) use it.
	Negated bool
}

// LiteralValue returns the literal type a literal-equality check compares
// against, materializing a string literal from its raw spelling when needed.
func (c Check) LiteralValue() (typ.Type, bool) {
	if c.Literal != nil {
		return c.Literal, true
	}
	if c.Kind == CheckLiteralEqual || c.Kind == CheckLiteralNot {
		return typ.LiteralString(c.LiteralString), true
	}
	return nil, false
}
