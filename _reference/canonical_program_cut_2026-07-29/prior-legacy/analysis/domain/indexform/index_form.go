package indexform

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
)

// IndexFormKind identifies one normalized Lua array-index expression. The
// zero value is deliberately invalid: consumers can only obtain a usable form
// through the checked constructors below.
type IndexFormKind uint8

const (
	IndexFormInvalid IndexFormKind = iota
	IndexFormArrayLength
	IndexFormModuloLength
	IndexFormConstant
	IndexFormAffine
)

// AffineIndex is coeff*value(path)+offset. Coeff is always positive; this is
// the monotone fragment for which lower/upper numeric bounds can be
// transported without reversing their meaning.
type AffineIndex struct {
	path   pathdom.PathKey
	coeff  int64
	offset int64
}

// Path returns an isolated copy of the affine operand path.
func (a AffineIndex) Path() (pathdom.Path, bool) {
	stable, ok := pathaddr.StableFromKey(a.path)
	if !ok {
		return pathdom.Path{}, false
	}
	return stable.Path()
}

// PathKey returns the comparable stable identity of the affine operand.
func (a AffineIndex) PathKey() pathdom.PathKey { return a.path }

// Coeff returns the positive path coefficient.
func (a AffineIndex) Coeff() int64 { return a.coeff }

// Offset returns the additive integer offset.
func (a AffineIndex) Offset() int64 { return a.offset }

// Valid reports whether the term belongs to the supported monotone fragment.
func (a AffineIndex) Valid() bool { return a.path != "" && a.coeff > 0 }

// IndexForm is the sealed, syntax-independent description of a dynamic-read
// index. Array paths are retained for the length-derived forms so nested table
// paths remain structurally exact.
type IndexForm struct {
	kind     IndexFormKind
	array    pathdom.PathKey
	constant int64
	affine   AffineIndex
}

// IndexShape is the path-free operator retained after lexical paths have been
// lowered to engine coordinates. It preserves the exact normalized arithmetic
// while allowing symbolic path rebasing to remain owned by the transformer.
type IndexShape struct {
	kind     IndexFormKind
	constant int64
	coeff    int64
	offset   int64
}

// Shape lowers a valid lexical form to its path-free engine operator.
func (f IndexForm) Shape() (IndexShape, bool) {
	if !f.Valid() {
		return IndexShape{}, false
	}
	shape := IndexShape{kind: f.kind, constant: f.constant}
	if f.kind == IndexFormAffine {
		shape.coeff, shape.offset = f.affine.coeff, f.affine.offset
	}
	return shape, true
}

func (s IndexShape) Kind() IndexFormKind { return s.kind }
func (s IndexShape) Valid() bool {
	return s.kind == IndexFormArrayLength || s.kind == IndexFormModuloLength || s.kind == IndexFormConstant ||
		s.kind == IndexFormAffine && s.coeff > 0
}
func (s IndexShape) Constant() (int64, bool) { return s.constant, s.kind == IndexFormConstant }
func (s IndexShape) Affine() (coeff, offset int64, ok bool) {
	return s.coeff, s.offset, s.kind == IndexFormAffine && s.coeff > 0
}

// NewArrayLengthIndex constructs the exact #array form.
func NewArrayLengthIndex(array pathdom.Path) (IndexForm, bool) {
	stable, ok := pathaddr.StableOfPath(array)
	if !ok {
		return IndexForm{}, false
	}
	return IndexForm{kind: IndexFormArrayLength, array: stable.Key()}, true
}

// NewModuloLengthIndex constructs (integer % #array)+1. Whether the dividend
// is integer is a valuation certificate, not syntax, and is therefore supplied
// separately by the edge adapter/evidence binder.
func NewModuloLengthIndex(array pathdom.Path) (IndexForm, bool) {
	stable, ok := pathaddr.StableOfPath(array)
	if !ok {
		return IndexForm{}, false
	}
	return IndexForm{kind: IndexFormModuloLength, array: stable.Key()}, true
}

// NewConstantIndex constructs an exact integer index. Positivity is a proof
// obligation rather than a construction invariant, so negative constants are
// represented exactly and subsequently rejected by the range proof.
func NewConstantIndex(value int64) IndexForm {
	return IndexForm{kind: IndexFormConstant, constant: value}
}

// NewAffineIndex constructs coeff*value(path)+offset. Non-positive
// coefficients are outside the monotone bound-transport fragment.
func NewAffineIndex(path pathdom.Path, coeff, offset int64) (IndexForm, bool) {
	stable, ok := pathaddr.StableOfPath(path)
	if !ok {
		return IndexForm{}, false
	}
	affine := AffineIndex{path: stable.Key(), coeff: coeff, offset: offset}
	if !affine.Valid() {
		return IndexForm{}, false
	}
	return IndexForm{kind: IndexFormAffine, affine: affine}, true
}

// Kind returns the normalized form discriminator.
func (f IndexForm) Kind() IndexFormKind { return f.kind }

// Valid reports whether the form was produced by a checked constructor.
func (f IndexForm) Valid() bool {
	switch f.kind {
	case IndexFormArrayLength, IndexFormModuloLength:
		return f.array != ""
	case IndexFormConstant:
		return true
	case IndexFormAffine:
		return f.affine.Valid()
	default:
		return false
	}
}

// ArrayPath returns the exact container path for a length-derived form.
func (f IndexForm) ArrayPath() (pathdom.Path, bool) {
	if f.kind != IndexFormArrayLength && f.kind != IndexFormModuloLength {
		return pathdom.Path{}, false
	}
	stable, ok := pathaddr.StableFromKey(f.array)
	if !ok {
		return pathdom.Path{}, false
	}
	return stable.Path()
}

// ArrayPathKey returns the comparable stable identity of a length-derived
// container.
func (f IndexForm) ArrayPathKey() (pathdom.PathKey, bool) {
	return f.array, f.kind == IndexFormArrayLength || f.kind == IndexFormModuloLength
}

// Constant returns the exact integer constant form.
func (f IndexForm) Constant() (int64, bool) {
	return f.constant, f.kind == IndexFormConstant
}

// Affine returns the normalized monotone affine form.
func (f IndexForm) Affine() (AffineIndex, bool) {
	if f.kind != IndexFormAffine {
		return AffineIndex{}, false
	}
	out := f.affine
	return out, true
}

// CheckedAddInt64 adds two integers without overflow.
func CheckedAddInt64(left, right int64) (int64, bool) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, false
	}
	return left + right, true
}

// CheckedMulInt64 multiplies two integers without overflow.
func CheckedMulInt64(left, right int64) (int64, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	if left == -1 && right == math.MinInt64 || right == -1 && left == math.MinInt64 {
		return 0, false
	}
	product := left * right
	return product, product/right == left
}
