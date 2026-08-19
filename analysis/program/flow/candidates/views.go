package candidates

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// Unary, Binary, and Access are the three narrow candidate planes. Each is a
// borrowed projection over the one sealed Result; none owns storage and none
// republishes the full candidate catalog.
type Unary struct{ result *Result }

type Binary struct{ result *Result }

type Access struct{ result *Result }

// Unary opens the unary-operator candidate plane.
func (r *Result) Unary() Unary { return Unary{result: r} }

// Binary opens the binary-operator candidate plane.
func (r *Result) Binary() Binary { return Binary{result: r} }

// Access opens the indexed access candidate plane.
func (r *Result) Access() Access { return Access{result: r} }

func (v Unary) NumericCount() int { return v.result.UnaryNumeric().Count() }

func (v Unary) NumericAt(index int) (keyspace.Term, bool) { return v.result.UnaryNumeric().At(index) }

func (v Unary) LengthCount() int { return v.result.Length().Count() }

func (v Unary) LengthAt(index int) (keyspace.Term, bool) { return v.result.Length().At(index) }

func (v Binary) ArithmeticCount() int { return v.result.Arithmetic().Count() }

func (v Binary) ArithmeticAt(index int) (keyspace.Term, bool) { return v.result.Arithmetic().At(index) }

func (v Binary) BitwiseCount() int { return v.result.Bitwise().Count() }

func (v Binary) BitwiseAt(index int) (keyspace.Term, bool) { return v.result.Bitwise().At(index) }

func (v Binary) ConcatCount() int { return v.result.Concat().Count() }

func (v Binary) ConcatAt(index int) (keyspace.Term, bool) { return v.result.Concat().At(index) }

func (v Binary) EqualityCount() int { return v.result.Equality().Count() }

func (v Binary) EqualityAt(index int) (keyspace.Term, bool) { return v.result.Equality().At(index) }

func (v Binary) OrderCount() int { return v.result.Order().Count() }

func (v Binary) OrderAt(index int) (keyspace.Term, bool) { return v.result.Order().At(index) }

func (v Access) GetCount() int { return v.result.IndexGet().Count() }

func (v Access) GetAt(index int) (keyspace.Term, bool) { return v.result.IndexGet().At(index) }

func (v Access) SetCount() int { return v.result.IndexSet().Count() }

func (v Access) SetAt(index int) (keyspace.Term, bool) { return v.result.IndexSet().At(index) }
