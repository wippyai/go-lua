package flow

import (
	"github.com/wippyai/go-lua/analysis/program/flow/internal/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binaryprimitive"
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
	return view.result.TableFields()
}

func (view AccessGeometry) ExactLenses() AccessExactLenses {
	if !view.Available() {
		return AccessExactLenses{}
	}
	return view.result.ExactLenses()
}

func (view AccessGeometry) DynamicLenses() AccessDynamicLenses {
	if !view.Available() {
		return AccessDynamicLenses{}
	}
	return view.result.DynamicLenses()
}

func (view AccessGeometry) IndexAccesses() AccessIndexAccesses {
	if !view.Available() {
		return AccessIndexAccesses{}
	}
	return view.result.IndexAccesses()
}

func (view AccessGeometry) ExactRead(read keyspace.Term) (keyspace.Term, int, bool) {
	if !view.Available() {
		return 0, 0, false
	}
	return view.result.ExactReads().Get(read)
}

// ExactReadPath opens one immutable leaf-to-root cursor over a sealed exact
// selector chain. Each Segment advances one existing parent-chain edge in O(1)
// without materializing or restarting the path.
func (view AccessGeometry) ExactReadPath(read keyspace.Term) (ExactReadPath, bool) {
	if !view.Available() {
		return ExactReadPath{}, false
	}
	return view.result.ExactReads().PathCursor(read)
}

// ExactReadPath is one immutable leaf-to-root cursor over a sealed exact
// selector chain.
type ExactReadPath = accessgeometry.ExactReadPath

func (view AccessGeometry) TypePublication(publication keyspace.Term) (root, owner keyspace.Term, depth int, ok bool) {
	if !view.Available() {
		return 0, 0, 0, false
	}
	return view.result.TypePublications().Get(publication)
}

// PublicationPath opens one immutable leaf-to-root cursor over the sealed
// exact Static publication path. Segment is O(1) and allocation-free.
func (view AccessGeometry) TypePublicationPath(publication keyspace.Term) (PublicationPath, bool) {
	if !view.Available() {
		return PublicationPath{}, false
	}
	return view.result.TypePublications().PathCursor(publication)
}

// PublicationPath is one immutable leaf-to-root cursor over the sealed exact
// Static publication path.
type PublicationPath = accessgeometry.PublicationPath

func (view AccessGeometry) DirectCall(call keyspace.Term) (keyspace.Term, CallForm, bool) {
	if !view.Available() {
		return 0, 0, false
	}
	read, form, ok := view.result.DirectCalls().Get(call)
	if !ok {
		return 0, 0, false
	}
	return read, publicCallForm(form), true
}

// CallForm is the closed direct-call syntax disposition.
type CallForm uint8

const (
	CallFormPlain  CallForm = 1
	CallFormMethod CallForm = 2
)

func publicCallForm(form uint8) CallForm {
	switch form {
	case 1:
		return CallFormPlain
	case 2:
		return CallFormMethod
	default:
		return 0
	}
}

// Access geometry planes.  Each name below is the single accessgeometry view
// published under its public name; the private package keeps every
// constructor unexported, so the capability fence is unchanged.
type (
	// AccessTableFields is the normalized-key view over authored TableFields.
	AccessTableFields = accessgeometry.TableFields
	// AccessExactLenses is the normalized-key view over authored exact Lenses.
	AccessExactLenses = accessgeometry.ExactLenses
	// AccessDynamicLenses is the dense zero-key view over authored dynamic Lenses.
	AccessDynamicLenses = accessgeometry.DynamicLenses
	// AccessIndexAccesses splits candidate indexed reads and writes into
	// their two typed planes.
	AccessIndexAccesses = accessgeometry.IndexAccesses
	// AccessIndexReads is the candidate IndexGet Read view.
	AccessIndexReads = accessgeometry.IndexReads
	// AccessIndexWrites is the candidate IndexSet Write view.
	AccessIndexWrites = accessgeometry.IndexWrites
)

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
	return view.result.Arithmetic()
}

func (view BinaryPrimitives) Bitwise() BinaryBitwise {
	if !view.Available() {
		return BinaryBitwise{}
	}
	return view.result.Bitwise()
}

func (view BinaryPrimitives) Equality() BinaryEquality {
	if !view.Available() {
		return BinaryEquality{}
	}
	return view.result.Equality()
}

func (view BinaryPrimitives) Order() BinaryOrder {
	if !view.Available() {
		return BinaryOrder{}
	}
	return view.result.Order()
}

func (view BinaryPrimitives) Primitive(binary keyspace.Term) (BinaryPrimitive, bool) {
	if !view.Available() {
		return BinaryPrimitive{}, false
	}
	return view.result.Primitive(binary)
}

// Binary primitive planes.  Each name below is the single binaryprimitive
// row or bucket published under its public name; the private package keeps
// every constructor unexported, so the capability fence is unchanged.
type (
	// BinaryArithmetic is the typed arithmetic primitive bucket.
	BinaryArithmetic = binaryprimitive.Arithmetic
	// BinaryBitwise is the typed bitwise primitive bucket.
	BinaryBitwise = binaryprimitive.Bitwise
	// BinaryEquality is the typed equality-comparison primitive bucket.
	BinaryEquality = binaryprimitive.Equality
	// BinaryOrder is the typed relational-order primitive bucket.
	BinaryOrder = binaryprimitive.Order
	// BinaryPrimitive is an opaque occurrence handle for one retained Binary.
	BinaryPrimitive = binaryprimitive.Primitive
	// BinaryOperation is the arithmetic/bitwise interpretation of a Primitive.
	BinaryOperation = binaryprimitive.Operation
	// BinaryComparison is the branch interpretation of a Primitive.
	BinaryComparison = binaryprimitive.Comparison
)
