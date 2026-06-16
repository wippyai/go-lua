package numeric

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// Numeric constraints encode arithmetic relationships between paths.
//
// This package is a reusable theory leaf. Higher analysis layers and solver
// packages consume these constraints when wired, but the surface stays
// independent of any single engine or pass.
// Unlike type constraints, numeric constraints operate on values rather than types.
//
// # Supported Constraints
//
//   - Comparisons: [Le] (x-y≤c), [Lt] (x<y), [Ge] (x≥y), [Gt] (x>y), [Eq] (x==y)
//   - Constants: [EqConst] (x==c), [LeConst] (x≤c), [GeConst] (x≥c)
//   - Modular: [ModEq] (x%m==r)
//   - Symbolic: [LeLenOf] (x≤len(arr)+c)
//   - Length bounds: [LenLeConst], [LenGeConst]
//
// # Usage with Theory Solvers
//
// Numeric constraints are typically consumed by:
//   - Difference Logic solver: handles Le, Lt, Ge, Gt constraints
//   - Modular Arithmetic solver: handles ModEq constraints
//
// All numeric constraints are AST-free and can be serialized for caching.

// NumKind classifies numeric constraint terms.
type NumKind uint8

const (
	NumInvalid    NumKind = iota
	NumLe                 // x - y <= c
	NumLt                 // x < y
	NumGe                 // x >= y
	NumGt                 // x > y
	NumEq                 // x == y
	NumEqConst            // x == c
	NumLeConst            // x <= c
	NumGeConst            // x >= c
	NumModEq              // x % m == r
	NumLeLenOf            // x <= len(arr) + offset
	NumLenLeConst         // len(arr) <= c
	NumLenGeConst         // len(arr) >= c
)

// NumericConstraint is a marker interface for numeric constraints.
type NumericConstraint interface {
	NumKind() NumKind
	Paths() []pathdom.Path
	Hash() uint64
	Equals(other NumericConstraint) bool
}

// Le represents x - y <= c.
type Le struct {
	X pathdom.Path
	Y pathdom.Path
	C int64
}

func (c Le) NumKind() NumKind      { return NumLe }
func (c Le) Paths() []pathdom.Path { return []pathdom.Path{c.X, c.Y} }
func (c Le) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, c.Y, c.C) }
func (c Le) Equals(o NumericConstraint) bool {
	other, ok := o.(Le)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y) && c.C == other.C
}

// Lt represents x < y.
type Lt struct {
	X pathdom.Path
	Y pathdom.Path
}

func (c Lt) NumKind() NumKind      { return NumLt }
func (c Lt) Paths() []pathdom.Path { return []pathdom.Path{c.X, c.Y} }
func (c Lt) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Lt) Equals(o NumericConstraint) bool {
	other, ok := o.(Lt)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// Ge represents x >= y.
type Ge struct {
	X pathdom.Path
	Y pathdom.Path
}

func (c Ge) NumKind() NumKind      { return NumGe }
func (c Ge) Paths() []pathdom.Path { return []pathdom.Path{c.X, c.Y} }
func (c Ge) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Ge) Equals(o NumericConstraint) bool {
	other, ok := o.(Ge)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// Gt represents x > y.
type Gt struct {
	X pathdom.Path
	Y pathdom.Path
}

func (c Gt) NumKind() NumKind      { return NumGt }
func (c Gt) Paths() []pathdom.Path { return []pathdom.Path{c.X, c.Y} }
func (c Gt) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Gt) Equals(o NumericConstraint) bool {
	other, ok := o.(Gt)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// Eq represents x == y.
type Eq struct {
	X pathdom.Path
	Y pathdom.Path
}

func (c Eq) NumKind() NumKind      { return NumEq }
func (c Eq) Paths() []pathdom.Path { return []pathdom.Path{c.X, c.Y} }
func (c Eq) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Eq) Equals(o NumericConstraint) bool {
	other, ok := o.(Eq)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// EqConst represents x == c.
type EqConst struct {
	X pathdom.Path
	C int64
}

func (c EqConst) NumKind() NumKind      { return NumEqConst }
func (c EqConst) Paths() []pathdom.Path { return []pathdom.Path{c.X} }
func (c EqConst) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, pathdom.Path{}, c.C) }
func (c EqConst) Equals(o NumericConstraint) bool {
	other, ok := o.(EqConst)
	return ok && c.X.Equal(other.X) && c.C == other.C
}

// LeConst represents x <= c.
type LeConst struct {
	X pathdom.Path
	C int64
}

func (c LeConst) NumKind() NumKind      { return NumLeConst }
func (c LeConst) Paths() []pathdom.Path { return []pathdom.Path{c.X} }
func (c LeConst) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, pathdom.Path{}, c.C) }
func (c LeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LeConst)
	return ok && c.X.Equal(other.X) && c.C == other.C
}

// GeConst represents x >= c.
type GeConst struct {
	X pathdom.Path
	C int64
}

func (c GeConst) NumKind() NumKind      { return NumGeConst }
func (c GeConst) Paths() []pathdom.Path { return []pathdom.Path{c.X} }
func (c GeConst) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, pathdom.Path{}, c.C) }
func (c GeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(GeConst)
	return ok && c.X.Equal(other.X) && c.C == other.C
}

// ModEq represents x % m == r.
type ModEq struct {
	X pathdom.Path
	M int64
	R int64
}

func (c ModEq) NumKind() NumKind      { return NumModEq }
func (c ModEq) Paths() []pathdom.Path { return []pathdom.Path{c.X} }
func (c ModEq) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, pathdom.Path{}, c.M, c.R) }
func (c ModEq) Equals(o NumericConstraint) bool {
	other, ok := o.(ModEq)
	return ok && c.X.Equal(other.X) && c.M == other.M && c.R == other.R
}

// LeLenOf represents x <= len(arr)+offset, a symbolic upper bound.
type LeLenOf struct {
	X      pathdom.Path // variable being bounded
	Array  pathdom.Path // array whose length is the upper bound
	Offset int64        // additive offset (can be negative)
}

func (c LeLenOf) NumKind() NumKind      { return NumLeLenOf }
func (c LeLenOf) Paths() []pathdom.Path { return []pathdom.Path{c.X, c.Array} }
func (c LeLenOf) Hash() uint64          { return hashNumConstraint(c.NumKind(), c.X, c.Array, c.Offset) }
func (c LeLenOf) Equals(o NumericConstraint) bool {
	other, ok := o.(LeLenOf)
	return ok && c.X.Equal(other.X) && c.Array.Equal(other.Array) && c.Offset == other.Offset
}

// LenLeConst represents len(arr) <= c.
type LenLeConst struct {
	Array pathdom.Path
	C     int64
}

func (c LenLeConst) NumKind() NumKind      { return NumLenLeConst }
func (c LenLeConst) Paths() []pathdom.Path { return []pathdom.Path{c.Array} }
func (c LenLeConst) Hash() uint64 {
	return hashNumConstraint(c.NumKind(), c.Array, pathdom.Path{}, c.C)
}
func (c LenLeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LenLeConst)
	return ok && c.Array.Equal(other.Array) && c.C == other.C
}

// LenGeConst represents len(arr) >= c.
type LenGeConst struct {
	Array pathdom.Path
	C     int64
}

func (c LenGeConst) NumKind() NumKind      { return NumLenGeConst }
func (c LenGeConst) Paths() []pathdom.Path { return []pathdom.Path{c.Array} }
func (c LenGeConst) Hash() uint64 {
	return hashNumConstraint(c.NumKind(), c.Array, pathdom.Path{}, c.C)
}
func (c LenGeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LenGeConst)
	return ok && c.Array.Equal(other.Array) && c.C == other.C
}

func hashNumConstraint(kind NumKind, a, b pathdom.Path, extra ...int64) uint64 {
	h := internal.MixHash(uint64(kind), a.Hash())
	if !b.IsEmpty() {
		h = internal.MixHash(h, b.Hash())
	}

	for _, v := range extra {
		h = internal.MixHash(h, uint64(v))
	}

	return h
}
