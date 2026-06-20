package numeric

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// Numeric constraints encode arithmetic relationships between path keys.
//
// This package is a reusable theory leaf. Higher analysis layers and solver
// packages consume these constraints when wired, but the surface stays
// independent of any single engine or pass.
// Unlike type constraints, numeric constraints operate on values rather than types.
//
// Variables are pathdom.PathKey, the stable key the engine has already resolved
// at the factapply boundary. The IR is key-based, not Path-based: a solver treats
// every variable as an opaque key. A length operand is just a PathKey that happens
// to be a state.LengthRelKey sentinel; constraints carry no special length kind.
//
// # Supported Constraints
//
//   - Comparisons: [Le] (x-y≤c), [Lt] (x<y), [Ge] (x≥y), [Gt] (x>y), [Eq] (x==y)
//   - Bounded affine: [SumLe] (coX·x+coY·y-z≤c), a relation between two
//     positive operands (with positive coefficients) and one negative
//     unit-coefficient operand; a unit [NewSumLe] or scaled [NewScaledLe]
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
	NumSumLe              // x + y - z <= c
)

// NumericConstraint is a marker interface for numeric constraints.
type NumericConstraint interface {
	NumKind() NumKind
	Keys() []pathdom.PathKey
	Hash() uint64
	Equals(other NumericConstraint) bool
}

// Le represents x - y <= c.
type Le struct {
	X pathdom.PathKey
	Y pathdom.PathKey
	C int64
}

func (c Le) NumKind() NumKind        { return NumLe }
func (c Le) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X, c.Y} }
func (c Le) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, c.Y, c.C) }
func (c Le) Equals(o NumericConstraint) bool {
	other, ok := o.(Le)
	return ok && c.X == other.X && c.Y == other.Y && c.C == other.C
}

// SumLe represents CoX*value(X) + CoY*value(Y) - value(Z) <= C, a bounded affine
// relation between two positive operands (with positive coefficients CoX, CoY)
// and one negative unit-coefficient operand. The two (Co,Key) positive pairs are
// canonicalized into a deterministic order by the constructors so commutative
// sums dedup. An empty Y means the second positive term is absent (CoY ignored).
type SumLe struct {
	CoX int64
	X   pathdom.PathKey
	CoY int64
	Y   pathdom.PathKey
	Z   pathdom.PathKey
	C   int64
}

// NewSumLe builds value(x) + value(y) - value(z) <= c with unit coefficients,
// canonicalizing the two positive operands x and y into a deterministic order so
// x+y and y+x dedup.
func NewSumLe(x, y, z pathdom.PathKey, c int64) SumLe {
	return NewScaledLe(1, x, 1, y, z, c)
}

// NewScaledLe builds coX*value(x) + coY*value(y) - value(z) <= c, canonicalizing
// the two positive (coefficient, key) pairs into a deterministic order so
// commutative sums dedup. An empty y drops the second positive term.
func NewScaledLe(coX int64, x pathdom.PathKey, coY int64, y pathdom.PathKey, z pathdom.PathKey, c int64) SumLe {
	if y != "" && positiveTermLess(coY, y, coX, x) {
		coX, x, coY, y = coY, y, coX, x
	}
	return SumLe{CoX: coX, X: x, CoY: coY, Y: y, Z: z, C: c}
}

// positiveTermLess orders (co1,k1) before (co2,k2) by key first, then coefficient,
// so the two positive operands canonicalize deterministically.
func positiveTermLess(co1 int64, k1 pathdom.PathKey, co2 int64, k2 pathdom.PathKey) bool {
	if k1 != k2 {
		return k1 < k2
	}
	return co1 < co2
}

func (c SumLe) NumKind() NumKind { return NumSumLe }
func (c SumLe) Keys() []pathdom.PathKey {
	if c.Y == "" {
		return []pathdom.PathKey{c.X, c.Z}
	}
	return []pathdom.PathKey{c.X, c.Y, c.Z}
}
func (c SumLe) Hash() uint64 {
	h := hashNumConstraint(c.NumKind(), c.X, c.Y, c.C, c.CoX, c.CoY)
	return internal.MixHash(h, internal.FnvString(string(c.Z)))
}
func (c SumLe) Equals(o NumericConstraint) bool {
	other, ok := o.(SumLe)
	return ok && c.CoX == other.CoX && c.X == other.X && c.CoY == other.CoY &&
		c.Y == other.Y && c.Z == other.Z && c.C == other.C
}

// Lt represents x < y.
type Lt struct {
	X pathdom.PathKey
	Y pathdom.PathKey
}

func (c Lt) NumKind() NumKind        { return NumLt }
func (c Lt) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X, c.Y} }
func (c Lt) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Lt) Equals(o NumericConstraint) bool {
	other, ok := o.(Lt)
	return ok && c.X == other.X && c.Y == other.Y
}

// Ge represents x >= y.
type Ge struct {
	X pathdom.PathKey
	Y pathdom.PathKey
}

func (c Ge) NumKind() NumKind        { return NumGe }
func (c Ge) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X, c.Y} }
func (c Ge) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Ge) Equals(o NumericConstraint) bool {
	other, ok := o.(Ge)
	return ok && c.X == other.X && c.Y == other.Y
}

// Gt represents x > y.
type Gt struct {
	X pathdom.PathKey
	Y pathdom.PathKey
}

func (c Gt) NumKind() NumKind        { return NumGt }
func (c Gt) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X, c.Y} }
func (c Gt) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Gt) Equals(o NumericConstraint) bool {
	other, ok := o.(Gt)
	return ok && c.X == other.X && c.Y == other.Y
}

// Eq represents x == y.
type Eq struct {
	X pathdom.PathKey
	Y pathdom.PathKey
}

func (c Eq) NumKind() NumKind        { return NumEq }
func (c Eq) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X, c.Y} }
func (c Eq) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Eq) Equals(o NumericConstraint) bool {
	other, ok := o.(Eq)
	return ok && c.X == other.X && c.Y == other.Y
}

// EqConst represents x == c.
type EqConst struct {
	X pathdom.PathKey
	C int64
}

func (c EqConst) NumKind() NumKind        { return NumEqConst }
func (c EqConst) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X} }
func (c EqConst) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, "", c.C) }
func (c EqConst) Equals(o NumericConstraint) bool {
	other, ok := o.(EqConst)
	return ok && c.X == other.X && c.C == other.C
}

// LeConst represents x <= c.
type LeConst struct {
	X pathdom.PathKey
	C int64
}

func (c LeConst) NumKind() NumKind        { return NumLeConst }
func (c LeConst) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X} }
func (c LeConst) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, "", c.C) }
func (c LeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LeConst)
	return ok && c.X == other.X && c.C == other.C
}

// GeConst represents x >= c.
type GeConst struct {
	X pathdom.PathKey
	C int64
}

func (c GeConst) NumKind() NumKind        { return NumGeConst }
func (c GeConst) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X} }
func (c GeConst) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, "", c.C) }
func (c GeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(GeConst)
	return ok && c.X == other.X && c.C == other.C
}

// ModEq represents x % m == r.
type ModEq struct {
	X pathdom.PathKey
	M int64
	R int64
}

func (c ModEq) NumKind() NumKind        { return NumModEq }
func (c ModEq) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X} }
func (c ModEq) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, "", c.M, c.R) }
func (c ModEq) Equals(o NumericConstraint) bool {
	other, ok := o.(ModEq)
	return ok && c.X == other.X && c.M == other.M && c.R == other.R
}

// LeLenOf represents x <= len(arr)+offset, a symbolic upper bound.
type LeLenOf struct {
	X      pathdom.PathKey // variable being bounded
	Array  pathdom.PathKey // array whose length is the upper bound
	Offset int64           // additive offset (can be negative)
}

func (c LeLenOf) NumKind() NumKind        { return NumLeLenOf }
func (c LeLenOf) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.X, c.Array} }
func (c LeLenOf) Hash() uint64            { return hashNumConstraint(c.NumKind(), c.X, c.Array, c.Offset) }
func (c LeLenOf) Equals(o NumericConstraint) bool {
	other, ok := o.(LeLenOf)
	return ok && c.X == other.X && c.Array == other.Array && c.Offset == other.Offset
}

// LenLeConst represents len(arr) <= c.
type LenLeConst struct {
	Array pathdom.PathKey
	C     int64
}

func (c LenLeConst) NumKind() NumKind        { return NumLenLeConst }
func (c LenLeConst) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.Array} }
func (c LenLeConst) Hash() uint64 {
	return hashNumConstraint(c.NumKind(), c.Array, "", c.C)
}
func (c LenLeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LenLeConst)
	return ok && c.Array == other.Array && c.C == other.C
}

// LenGeConst represents len(arr) >= c.
type LenGeConst struct {
	Array pathdom.PathKey
	C     int64
}

func (c LenGeConst) NumKind() NumKind        { return NumLenGeConst }
func (c LenGeConst) Keys() []pathdom.PathKey { return []pathdom.PathKey{c.Array} }
func (c LenGeConst) Hash() uint64 {
	return hashNumConstraint(c.NumKind(), c.Array, "", c.C)
}
func (c LenGeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LenGeConst)
	return ok && c.Array == other.Array && c.C == other.C
}

func hashNumConstraint(kind NumKind, a, b pathdom.PathKey, extra ...int64) uint64 {
	h := internal.MixHash(uint64(kind), internal.FnvString(string(a)))
	if b != "" {
		h = internal.MixHash(h, internal.FnvString(string(b)))
	}

	for _, v := range extra {
		h = internal.MixHash(h, uint64(v))
	}

	return h
}
