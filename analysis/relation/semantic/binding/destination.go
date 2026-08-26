package binding

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// DestinationView is a borrowed, runtime-fenced row source resolved from a
// sealed Apply plan. It is intentionally smaller than a relation or a
// denominator: output encoders may consume a scalar row once or a complete
// span sequentially, but cannot choose a row except in the OwnerNamed form.
// The view borrows the sealed Cell/Span storage and carries no second row
// directory.
type DestinationView struct {
	kind   destinationKind
	scalar Cell
	span   Span
	owner  model.RelationID
	next   int
}

type destinationKind uint8

const (
	destinationInvalid destinationKind = iota
	destinationScalar
	destinationSpan
	destinationOwnerNamed
)

func NewScalarDestination(cell Cell) (DestinationView, bool) {
	if !cell.Available() {
		return DestinationView{}, false
	}
	return DestinationView{kind: destinationScalar, scalar: cell}, true
}

func NewSpanDestination(span Span) (DestinationView, bool) {
	if !span.Available() {
		return DestinationView{}, false
	}
	return DestinationView{kind: destinationSpan, span: span}, true
}

func NewOwnerNamedDestination(relation model.RelationID) DestinationView {
	return DestinationView{kind: destinationOwnerNamed, owner: relation}
}

func (view DestinationView) Available() bool {
	switch view.kind {
	case destinationScalar:
		return view.scalar.Available()
	case destinationSpan:
		return view.span.Available() && view.next >= 0 && view.next <= view.span.Len()
	case destinationOwnerNamed:
		return view.owner.Available()
	default:
		return false
	}
}

func (view DestinationView) IsScalar() bool {
	return view.Available() && view.kind == destinationScalar
}

func (view DestinationView) IsSpan() bool {
	return view.Available() && view.kind == destinationSpan
}

func (view DestinationView) IsOwnerNamed() bool {
	return view.Available() && view.kind == destinationOwnerNamed
}

// OwnerRelation returns the sealed relation whose owner-issued keys may be
// supplied by an OwnerNamed emitter.
func (view DestinationView) OwnerRelation() (model.RelationID, bool) {
	if !view.IsOwnerNamed() {
		return model.RelationID{}, false
	}
	return view.owner, true
}

// Next redeems one already-authenticated destination cell. It is the only
// sequential cursor in this view; the operation supplies no ordinal and no
// row identity. The receiver is intentionally mutable only inside the
// solve-local Emitter copy.
func (view *DestinationView) Next() (CellToken, bool) {
	if view == nil || !view.Available() {
		return CellToken{}, false
	}
	var cell Cell
	switch view.kind {
	case destinationScalar:
		if view.next != 0 {
			return CellToken{}, false
		}
		cell = view.scalar
	case destinationSpan:
		var ok bool
		cell, ok = view.span.At(view.next)
		if !ok {
			return CellToken{}, false
		}
	default:
		return CellToken{}, false
	}
	view.next++
	return cell.Address(), true
}
