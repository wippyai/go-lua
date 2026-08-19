package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// FunctionBoundary and its three ordered child planes are the canonical
// callable-interface publication.  A boundary owns each child through a
// half-open ordinal span; no child slice or nested row is retained in the
// sealed program.
const (
	slotFunctionBoundary = slotOutcomePoint + 1
	slotFunctionFormal   = slotFunctionBoundary + 1
	slotFunctionVararg   = slotFunctionFormal + 1
	slotFunctionCapture  = slotFunctionVararg + 1
)

func FunctionBoundaryFamily() Family[FunctionBoundary] {
	return Family[FunctionBoundary]{slot: slotFunctionBoundary, name: "function-boundary"}
}

func FunctionFormalFamily() Family[FunctionFormal] {
	return Family[FunctionFormal]{slot: slotFunctionFormal, name: "function-formal"}
}

func FunctionVarargFamily() Family[FunctionVararg] {
	return Family[FunctionVararg]{slot: slotFunctionVararg, name: "function-vararg"}
}

func FunctionCaptureFamily() Family[FunctionCapture] {
	return Family[FunctionCapture]{slot: slotFunctionCapture, name: "function-capture"}
}

// FunctionFormal is one ordered fixed input of a callable Function.  The
// declared static type is optional, exactly as it was in the compiler's
// source row; position is retained so the child plane authenticates order.
type FunctionFormal struct {
	id, cell, storage, declared identity.ContentID
	position                    uint32
}

func NewFunctionFormal(id, cell, storage, declared identity.ContentID, position uint32) (FunctionFormal, bool) {
	row := FunctionFormal{id: id, cell: cell, storage: storage, declared: declared, position: position}
	return row, row.Available()
}

func (row FunctionFormal) Available() bool {
	return row.id.Available() && row.cell.Available() && row.storage.Available()
}

func (row FunctionFormal) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row FunctionFormal) CellID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.cell
}

func (row FunctionFormal) StorageCellID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.storage
}

func (row FunctionFormal) DeclaredStaticTypeID() (identity.ContentID, bool) {
	return row.declared, row.Available() && row.declared.Available()
}

func (row FunctionFormal) Position() (int, bool) {
	return int(row.position), row.Available()
}

// FunctionVararg is the optional open input of one Function boundary.  A
// boundary with no vararg simply owns an empty span in this family.
type FunctionVararg struct{ id, cell identity.ContentID }

func NewFunctionVararg(id, cell identity.ContentID) (FunctionVararg, bool) {
	row := FunctionVararg{id: id, cell: cell}
	return row, row.Available()
}

func (row FunctionVararg) Available() bool { return row.id.Available() && row.cell.Available() }

func (row FunctionVararg) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row FunctionVararg) CellID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.cell
}

// FunctionCapture is one ordered lexical interface edge.  Inner/Outer are
// role-specific Cell identities and the Body identities retain the direction
// of the edge without importing authored Terms or live Flow handles.
type FunctionCapture struct {
	id, inner, outer, innerBody, outerBody identity.ContentID
	position                               uint32
}

func NewFunctionCapture(id, inner, outer, innerBody, outerBody identity.ContentID, position uint32) (FunctionCapture, bool) {
	row := FunctionCapture{id: id, inner: inner, outer: outer, innerBody: innerBody, outerBody: outerBody, position: position}
	return row, row.Available()
}

func (row FunctionCapture) Available() bool {
	return row.id.Available() && row.inner.Available() && row.outer.Available() &&
		row.innerBody.Available() && row.outerBody.Available() &&
		row.inner != row.outer && row.innerBody != row.outerBody
}

func (row FunctionCapture) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row FunctionCapture) InnerCellID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.inner
}

func (row FunctionCapture) OuterCellID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.outer
}

func (row FunctionCapture) InnerBodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.innerBody
}

func (row FunctionCapture) OuterBodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.outerBody
}

func (row FunctionCapture) Position() (int, bool) {
	return int(row.position), row.Available()
}

// FunctionBoundary is one immutable callable interface.  Its formal,
// vararg, and capture spans address the three flat child families.
type FunctionBoundary struct {
	id, body, bodyContext, entry, formal identity.ContentID
	formalOffset, formalCount            uint32
	varargOffset, varargCount            uint32
	captureOffset, captureCount          uint32
}

func NewFunctionBoundary(
	id, body, bodyContext, entry, formal identity.ContentID,
	formalOffset, formalCount, varargOffset, varargCount, captureOffset, captureCount uint32,
) (FunctionBoundary, bool) {
	row := FunctionBoundary{
		id: id, body: body, bodyContext: bodyContext, entry: entry, formal: formal,
		formalOffset: formalOffset, formalCount: formalCount,
		varargOffset: varargOffset, varargCount: varargCount,
		captureOffset: captureOffset, captureCount: captureCount,
	}
	return row, row.Available()
}

func (row FunctionBoundary) Available() bool {
	return row.id.Available() && row.body.Available() && row.bodyContext.Available() &&
		row.entry.Available() && row.formal.Available() && row.varargCount <= 1 &&
		uint64(row.formalOffset)+uint64(row.formalCount) <= uint64(^uint32(0)) &&
		uint64(row.varargOffset)+uint64(row.varargCount) <= uint64(^uint32(0)) &&
		uint64(row.captureOffset)+uint64(row.captureCount) <= uint64(^uint32(0))
}

func (row FunctionBoundary) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row FunctionBoundary) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row FunctionBoundary) BodyContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.bodyContext
}

func (row FunctionBoundary) EntryID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.entry
}

func (row FunctionBoundary) CallFormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

func (row FunctionBoundary) FormalSpan() (uint32, uint32, bool) {
	return row.formalOffset, row.formalCount, row.Available()
}

func (row FunctionBoundary) VarargSpan() (uint32, uint32, bool) {
	return row.varargOffset, row.varargCount, row.Available()
}

func (row FunctionBoundary) CaptureSpan() (uint32, uint32, bool) {
	return row.captureOffset, row.captureCount, row.Available()
}

func (row FunctionBoundary) FormalCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.formalCount)
}

func (row FunctionBoundary) CaptureCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.captureCount)
}

func (row FunctionBoundary) HasVararg() bool {
	return row.Available() && row.varargCount == 1
}
