// Package invocation owns the immutable structural address of one semantic
// invocation.
//
// An address is provenance, not a generated digest or an evaluation ordinal.
// The caller supplies the already-authenticated scope and source vectors;
// this package only copies and validates them.  Keeping this vocabulary above
// the engine lets state consumers retain the exact address without importing
// the Apply operator (or inventing a second invocation identity).
package invocation

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// TupleSources is the immutable source-row vector of one selected tuple.
// Source order is authored tuple order; it is provenance, not an evaluation
// ordinal and never a query key.
type TupleSources struct {
	rows []model.RowID
}

// NewTupleSources adopts an already-issued source-row vector.  It copies the
// vector and never issues or derives a row identity.
func NewTupleSources(rows []model.RowID) (TupleSources, bool) {
	if rows == nil {
		return TupleSources{}, false
	}
	copyOf := make([]model.RowID, len(rows))
	copy(copyOf, rows)
	result := TupleSources{rows: copyOf}
	if !result.Available() {
		return TupleSources{}, false
	}
	return result, true
}

func (sources TupleSources) available() bool {
	if sources.rows == nil {
		return false
	}
	for _, row := range sources.rows {
		if !row.Available() {
			return false
		}
	}
	return true
}

// Available reports whether the vector was sealed.  An empty vector is a
// valid extent only when it is explicitly sealed by its caller.
func (sources TupleSources) Available() bool { return sources.available() }

// Len reports the number of source rows in the tuple.
func (sources TupleSources) Len() int {
	if !sources.Available() {
		return 0
	}
	return len(sources.rows)
}

// At returns one source row in authored tuple order.
func (sources TupleSources) At(index int) (model.RowID, bool) {
	if !sources.Available() || index < 0 || index >= len(sources.rows) {
		return model.RowID{}, false
	}
	return sources.rows[index], true
}

// Rows returns a defensive copy of the sealed source vector.
func (sources TupleSources) Rows() []model.RowID {
	if !sources.Available() {
		return nil
	}
	return append([]model.RowID(nil), sources.rows...)
}

// SourceVector is one ordered Apply-child selection.  Scalar children have
// one tuple vector; a span child has one vector per selected tuple.
type SourceVector struct {
	tuples []TupleSources
}

// NewSourceVector adopts already-sealed tuple vectors and copies their
// containers.  It does not assign a child or tuple ordinal.
func NewSourceVector(tuples []TupleSources) (SourceVector, bool) {
	if tuples == nil {
		return SourceVector{}, false
	}
	copyOf := make([]TupleSources, len(tuples))
	for index, tuple := range tuples {
		if !tuple.Available() {
			return SourceVector{}, false
		}
		copyRows, ok := NewTupleSources(tuple.rows)
		if !ok {
			return SourceVector{}, false
		}
		copyOf[index] = copyRows
	}
	result := SourceVector{tuples: copyOf}
	if !result.Available() {
		return SourceVector{}, false
	}
	return result, true
}

func (vector SourceVector) available() bool {
	if vector.tuples == nil {
		return false
	}
	for _, tuple := range vector.tuples {
		if !tuple.Available() {
			return false
		}
	}
	return true
}

// Available reports whether the vector was sealed.
func (vector SourceVector) Available() bool { return vector.available() }

// Len reports the number of selected tuple vectors for this child.
func (vector SourceVector) Len() int {
	if !vector.Available() {
		return 0
	}
	return len(vector.tuples)
}

// At returns one selected tuple source vector in physical child order.
func (vector SourceVector) At(index int) (TupleSources, bool) {
	if !vector.Available() || index < 0 || index >= len(vector.tuples) {
		return TupleSources{}, false
	}
	return vector.tuples[index], true
}

// Tuples returns defensive copies of all selected tuple source vectors.
func (vector SourceVector) Tuples() []TupleSources {
	if !vector.Available() {
		return nil
	}
	result := make([]TupleSources, len(vector.tuples))
	for index, tuple := range vector.tuples {
		copyRows, _ := NewTupleSources(tuple.rows)
		result[index] = copyRows
	}
	return result
}

// InvocationAddress is the immutable provenance address of one invocation.
// It is intentionally structural: no digest, ordinal, or fallback identity
// is manufactured from the address.
type InvocationAddress struct {
	scope    binding.ScopeToken
	children []SourceVector
}

// New adopts one authenticated invocation scope and sealed child vectors.
// The returned address copies all containers and never retains a caller write
// path into its source rows.
func New(scope binding.ScopeToken, children []SourceVector) (InvocationAddress, bool) {
	if !scope.Available() || children == nil {
		return InvocationAddress{}, false
	}
	copyChildren := make([]SourceVector, len(children))
	for index, child := range children {
		if !child.Available() {
			return InvocationAddress{}, false
		}
		copyTuples, ok := NewSourceVector(child.tuples)
		if !ok {
			return InvocationAddress{}, false
		}
		copyChildren[index] = copyTuples
	}
	result := InvocationAddress{scope: scope, children: copyChildren}
	if !result.Available() {
		return InvocationAddress{}, false
	}
	return result, true
}

// Available reports whether the address carries a complete scope and sealed
// child/source vectors.
func (address InvocationAddress) Available() bool {
	if !address.scope.Available() || address.children == nil {
		return false
	}
	for _, child := range address.children {
		if !child.Available() {
			return false
		}
	}
	return true
}

// ValidFor redeems the address against the exact runtime fence.
func (address InvocationAddress) ValidFor(fence binding.Fence) bool {
	return address.Available() && address.scope.ValidFor(fence)
}

// Scope returns the authenticated normalized invocation scope.
func (address InvocationAddress) Scope() binding.ScopeToken {
	if !address.Available() {
		return binding.ScopeToken{}
	}
	return address.scope
}

// ChildCount reports the number of positional child selections.
func (address InvocationAddress) ChildCount() int {
	if !address.Available() {
		return 0
	}
	return len(address.children)
}

// ChildAt returns one immutable child source vector.
func (address InvocationAddress) ChildAt(index int) (SourceVector, bool) {
	if !address.Available() || index < 0 || index >= len(address.children) {
		return SourceVector{}, false
	}
	return address.children[index], true
}

// Children returns defensive copies of all child source vectors.
func (address InvocationAddress) Children() []SourceVector {
	if !address.Available() {
		return nil
	}
	result := make([]SourceVector, len(address.children))
	for index, child := range address.children {
		copyTuples, _ := NewSourceVector(child.tuples)
		result[index] = copyTuples
	}
	return result
}

// Same compares exact structural provenance and fence-qualified scope.  It
// does not compare a generated digest because none exists.
func (address InvocationAddress) Same(other InvocationAddress) bool {
	if !address.Available() || !other.Available() || !address.scope.Same(other.scope) || len(address.children) != len(other.children) {
		return false
	}
	for childIndex := range address.children {
		left, right := address.children[childIndex], other.children[childIndex]
		if left.Len() != right.Len() {
			return false
		}
		for tupleIndex := 0; tupleIndex < left.Len(); tupleIndex++ {
			leftTuple, leftOK := left.At(tupleIndex)
			rightTuple, rightOK := right.At(tupleIndex)
			if !leftOK || !rightOK || leftTuple.Len() != rightTuple.Len() {
				return false
			}
			for rowIndex := 0; rowIndex < leftTuple.Len(); rowIndex++ {
				leftRow, leftOK := leftTuple.At(rowIndex)
				rightRow, rightOK := rightTuple.At(rowIndex)
				if !leftOK || !rightOK || leftRow != rightRow {
					return false
				}
			}
		}
	}
	return true
}

// Compare establishes the package's deterministic structural order.  It is
// a presentation order only: it does not turn provenance into a new identity
// or an evaluation ordinal.  The scope comparator is supplied by binding and
// compares opaque token bytes without decoding their meaning.
func (address InvocationAddress) Compare(other InvocationAddress) int {
	if address.Available() != other.Available() {
		if !address.Available() {
			return -1
		}
		return 1
	}
	if !address.Available() {
		return 0
	}
	if result := binding.CompareScope(address.scope, other.scope); result != 0 {
		return result
	}
	if len(address.children) != len(other.children) {
		if len(address.children) < len(other.children) {
			return -1
		}
		return 1
	}
	for childIndex := range address.children {
		left, right := address.children[childIndex], other.children[childIndex]
		if left.Len() != right.Len() {
			if left.Len() < right.Len() {
				return -1
			}
			return 1
		}
		for tupleIndex := 0; tupleIndex < left.Len(); tupleIndex++ {
			leftTuple, _ := left.At(tupleIndex)
			rightTuple, _ := right.At(tupleIndex)
			if leftTuple.Len() != rightTuple.Len() {
				if leftTuple.Len() < rightTuple.Len() {
					return -1
				}
				return 1
			}
			for rowIndex := 0; rowIndex < leftTuple.Len(); rowIndex++ {
				leftRow, _ := leftTuple.At(rowIndex)
				rightRow, _ := rightTuple.At(rowIndex)
				if result := compareRowID(leftRow, rightRow); result != 0 {
					return result
				}
			}
		}
	}
	return 0
}

func compareRowID(left, right model.RowID) int {
	leftRelation, rightRelation := left.Relation(), right.Relation()
	if result := compareOwner(leftRelation.Owner(), rightRelation.Owner()); result != 0 {
		return result
	}
	leftRelationContent, rightRelationContent := leftRelation.Content(), rightRelation.Content()
	if result := bytes.Compare(leftRelationContent[:], rightRelationContent[:]); result != 0 {
		return result
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:])
}

func compareOwner(left, right model.OwnerID) int {
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:])
}
