package flow

import (
	"github.com/wippyai/go-lua/analysis/program/flow/internal/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binaryprimitive"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Activation is the retained Body activation projection.  It is deliberately
// separate from Source's Body parent/root index.
type Activation struct{ projection *activationProjection }

func (view Activation) Count() int {
	if view.projection == nil {
		return 0
	}
	return len(view.projection.terms)
}
func (view Activation) At(index int) (keyspace.Term, bool) {
	if view.projection == nil || index < 0 || index >= len(view.projection.terms) {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), true
}
func (view Activation) For(body keyspace.Term) (keyspace.Term, bool) {
	if view.projection == nil || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) > uint64(len(view.projection.terms)) {
		return 0, false
	}
	activation := view.projection.terms[ordinal-1]
	return activation, true
}

// Containment retains every canonical term, including Body roots, together
// with its parent edge and static classification. Body rows are needed by
// consumers that distinguish a statically owned function body from an
// executable body; omitting them would erase that owner-level judgment.
type Containment struct{ projection *containmentProjection }

func (view Containment) Count() int {
	if view.projection == nil {
		return 0
	}
	return len(view.projection.terms)
}
func (view Containment) At(index int) (keyspace.Term, bool) {
	if view.projection == nil || index < 0 || index >= len(view.projection.terms) {
		return 0, false
	}
	return view.projection.terms[index], true
}
func (view Containment) Parent(term keyspace.Term) (keyspace.Term, bool) {
	index, ok := view.index(term)
	if !ok {
		return 0, false
	}
	parent := view.projection.parents[index]
	return parent, parent != 0
}
func (view Containment) Static(term keyspace.Term) bool {
	index, ok := view.index(term)
	return ok && view.projection.static[index]
}
func (view Containment) index(term keyspace.Term) (int, bool) {
	if view.projection == nil {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return 0, false
	}
	plane := view.projection.index[family]
	if uint64(ordinal) >= uint64(len(plane)) || plane[ordinal] == 0 {
		return 0, false
	}
	return int(plane[ordinal] - 1), true
}

// AccessGeometry is Flow's retained normalized table/access projection. Its
// child views borrow the sealed internal planes directly; no authored owner or
// row collection is copied into the public surface.
type AccessGeometry struct {
	result    *accessgeometry.Result
	available bool
}

// Available reports whether this projection crossed the exact committed
// Source/Flow/Static/Module fence. It remains true for a valid sealed result
// with zero rows, so callers never infer availability from a count.
func (view AccessGeometry) Available() bool { return view.available && view.result != nil }

func (view AccessGeometry) TableFields() AccessTableFields {
	if !view.Available() {
		return AccessTableFields{}
	}
	return AccessTableFields{view: view.result.TableFields()}
}

func (view AccessGeometry) ExactLenses() AccessExactLenses {
	if !view.Available() {
		return AccessExactLenses{}
	}
	return AccessExactLenses{view: view.result.ExactLenses()}
}

func (view AccessGeometry) DynamicLenses() AccessDynamicLenses {
	if !view.Available() {
		return AccessDynamicLenses{}
	}
	return AccessDynamicLenses{view: view.result.DynamicLenses()}
}

func (view AccessGeometry) IndexAccesses() AccessIndexAccesses {
	if !view.Available() {
		return AccessIndexAccesses{}
	}
	return AccessIndexAccesses{view: view.result.IndexAccesses()}
}

// AccessTableFields is the normalized-key view over authored TableFields.
type AccessTableFields struct{ view accessgeometry.TableFields }

func (view AccessTableFields) Count() int                         { return view.view.Count() }
func (view AccessTableFields) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view AccessTableFields) Get(field keyspace.Term) (keyspace.Key, bool) {
	return view.view.Get(field)
}

// AccessExactLenses is the normalized-key view over authored exact Lenses.
type AccessExactLenses struct{ view accessgeometry.ExactLenses }

func (view AccessExactLenses) Count() int                         { return view.view.Count() }
func (view AccessExactLenses) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view AccessExactLenses) Get(lens keyspace.Term) (keyspace.Key, bool) {
	return view.view.Get(lens)
}

// AccessDynamicLenses is the dense zero-key view over authored dynamic Lenses.
type AccessDynamicLenses struct{ view accessgeometry.DynamicLenses }

func (view AccessDynamicLenses) Count() int                         { return view.view.Count() }
func (view AccessDynamicLenses) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view AccessDynamicLenses) Get(lens keyspace.Term) (keyspace.Key, bool) {
	return view.view.Get(lens)
}

// AccessIndexAccesses splits candidate indexed reads and writes into their
// two typed planes.
type AccessIndexAccesses struct{ view accessgeometry.IndexAccesses }

func (view AccessIndexAccesses) Reads() AccessIndexReads {
	return AccessIndexReads{view: view.view.Reads()}
}

func (view AccessIndexAccesses) Writes() AccessIndexWrites {
	return AccessIndexWrites{view: view.view.Writes()}
}

// AccessIndexReads is the candidate IndexGet Read view.
type AccessIndexReads struct{ view accessgeometry.IndexReads }

func (view AccessIndexReads) Count() int                          { return view.view.Count() }
func (view AccessIndexReads) At(index int) (keyspace.Term, bool)  { return view.view.At(index) }
func (view AccessIndexReads) Contains(read keyspace.Term) bool    { return view.view.Contains(read) }
func (view AccessIndexReads) Slot(read keyspace.Term) (int, bool) { return view.view.Slot(read) }
func (view AccessIndexReads) Get(read keyspace.Term) (base, keyTerm, lens keyspace.Term, ok bool) {
	return view.view.Get(read)
}

// AccessIndexWrites is the candidate IndexSet Write view.
type AccessIndexWrites struct{ view accessgeometry.IndexWrites }

func (view AccessIndexWrites) Count() int                           { return view.view.Count() }
func (view AccessIndexWrites) At(index int) (keyspace.Term, bool)   { return view.view.At(index) }
func (view AccessIndexWrites) Contains(write keyspace.Term) bool    { return view.view.Contains(write) }
func (view AccessIndexWrites) Slot(write keyspace.Term) (int, bool) { return view.view.Slot(write) }
func (view AccessIndexWrites) Get(write keyspace.Term) (base, keyTerm, values keyspace.Term, position int, lens keyspace.Term, ok bool) {
	return view.view.Get(write)
}

// BinaryPrimitives is Flow's retained executable primitive-Binary projection.
// Each bucket remains a distinct typed view and Primitive returns an opaque
// handle for the operation/comparison facts of one retained Binary.
type BinaryPrimitives struct {
	result    *binaryprimitive.Result
	available bool
}

// Available reports whether this projection crossed the exact committed
// Source/Flow/Static/Module fence. A valid zero-row result is available.
func (view BinaryPrimitives) Available() bool { return view.available && view.result != nil }

func (view BinaryPrimitives) Arithmetic() BinaryArithmetic {
	if !view.Available() {
		return BinaryArithmetic{}
	}
	return BinaryArithmetic{view: view.result.Arithmetic()}
}

func (view BinaryPrimitives) Bitwise() BinaryBitwise {
	if !view.Available() {
		return BinaryBitwise{}
	}
	return BinaryBitwise{view: view.result.Bitwise()}
}

func (view BinaryPrimitives) Equality() BinaryEquality {
	if !view.Available() {
		return BinaryEquality{}
	}
	return BinaryEquality{view: view.result.Equality()}
}

func (view BinaryPrimitives) Order() BinaryOrder {
	if !view.Available() {
		return BinaryOrder{}
	}
	return BinaryOrder{view: view.result.Order()}
}

func (view BinaryPrimitives) Primitive(binary keyspace.Term) (BinaryPrimitive, bool) {
	if !view.Available() {
		return BinaryPrimitive{}, false
	}
	primitive, ok := view.result.Primitive(binary)
	return BinaryPrimitive{primitive: primitive}, ok
}

// BinaryArithmetic is the typed arithmetic primitive bucket.
type BinaryArithmetic struct{ view binaryprimitive.Arithmetic }

func (view BinaryArithmetic) Count() int                         { return view.view.Count() }
func (view BinaryArithmetic) At(index int) (keyspace.Term, bool) { return view.view.At(index) }

// BinaryBitwise is the typed bitwise primitive bucket.
type BinaryBitwise struct{ view binaryprimitive.Bitwise }

func (view BinaryBitwise) Count() int                         { return view.view.Count() }
func (view BinaryBitwise) At(index int) (keyspace.Term, bool) { return view.view.At(index) }

// BinaryEquality is the typed equality-comparison primitive bucket.
type BinaryEquality struct{ view binaryprimitive.Equality }

func (view BinaryEquality) Count() int                         { return view.view.Count() }
func (view BinaryEquality) At(index int) (keyspace.Term, bool) { return view.view.At(index) }

// BinaryOrder is the typed relational-order primitive bucket.
type BinaryOrder struct{ view binaryprimitive.Order }

func (view BinaryOrder) Count() int                         { return view.view.Count() }
func (view BinaryOrder) At(index int) (keyspace.Term, bool) { return view.view.At(index) }

// BinaryPrimitive is an opaque occurrence handle for one retained Binary.
// Operation and Comparison return public value copies, never internal rows.
type BinaryPrimitive struct{ primitive binaryprimitive.Primitive }

func (primitive BinaryPrimitive) Source() (keyspace.Term, bool) {
	return primitive.primitive.Source()
}

type BinaryOperation struct {
	Owner       keyspace.Term
	Op          kind.BinaryOp
	Left, Right keyspace.Term
}

func (primitive BinaryPrimitive) Operation() (BinaryOperation, bool) {
	operation, ok := primitive.primitive.Operation()
	return BinaryOperation{Owner: operation.Owner, Op: operation.Op, Left: operation.Left, Right: operation.Right}, ok
}

type BinaryComparison struct {
	Branch              keyspace.Term
	TrueBody, FalseBody keyspace.Term
	Left, Right         keyspace.Term
	Invert              bool
}

func (primitive BinaryPrimitive) Comparison() (BinaryComparison, bool) {
	comparison, ok := primitive.primitive.Comparison()
	return BinaryComparison{
		Branch: comparison.Branch, TrueBody: comparison.TrueBody, FalseBody: comparison.FalseBody,
		Left: comparison.Left, Right: comparison.Right, Invert: comparison.Invert,
	}, ok
}
