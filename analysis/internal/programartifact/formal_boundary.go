package programartifact

import (
	"github.com/wippyai/go-lua/program/keyspace"
)

// FunctionFormalPort is one ordered fixed input of a reusable Program
// transformer. ID and CellID belong to Program's formal namespace;
// StorageCellID is the exact bridge used by Link-local value substitution.
type FunctionFormalPort struct {
	id, cell, storage, declared keyspace.ContentID
	position                    uint32
}

func (port FunctionFormalPort) Available() bool {
	return port.id.Available() && port.cell.Available() && port.storage.Available()
}
func (port FunctionFormalPort) ID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.id
}
func (port FunctionFormalPort) CellID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.cell
}
func (port FunctionFormalPort) StorageCellID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.storage
}
func (port FunctionFormalPort) DeclaredStaticTypeID() (keyspace.ContentID, bool) {
	return port.declared, port.Available() && port.declared.Available()
}
func (port FunctionFormalPort) Position() (int, bool) {
	return int(port.position), port.Available()
}

// FunctionVarargPort is the optional open input of one Function boundary.
// It remains distinct from fixed formal storage: no storage coordinate is
// invented when Program did not issue one.
type FunctionVarargPort struct{ id, cell keyspace.ContentID }

func (port FunctionVarargPort) Available() bool {
	return port.id.Available() && port.cell.Available()
}
func (port FunctionVarargPort) ID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.id
}
func (port FunctionVarargPort) CellID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.cell
}

// FunctionCapturePort is one ordered lexical interface edge. Inner/Outer are
// role-specific Cell identities; the Body identities retain the direction of
// the edge without exposing authored Terms or live Flow handles.
type FunctionCapturePort struct {
	id, inner, outer     keyspace.ContentID
	innerBody, outerBody keyspace.ContentID
	position             uint32
}

func (port FunctionCapturePort) Available() bool {
	return port.id.Available() && port.inner.Available() && port.outer.Available() &&
		port.innerBody.Available() && port.outerBody.Available() &&
		port.inner != port.outer && port.innerBody != port.outerBody
}
func (port FunctionCapturePort) ID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.id
}
func (port FunctionCapturePort) InnerCellID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.inner
}
func (port FunctionCapturePort) OuterCellID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.outer
}
func (port FunctionCapturePort) InnerBodyID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.innerBody
}
func (port FunctionCapturePort) OuterBodyID() keyspace.ContentID {
	if !port.Available() {
		return keyspace.ContentID{}
	}
	return port.outerBody
}
func (port FunctionCapturePort) Position() (int, bool) {
	return int(port.position), port.Available()
}

// FunctionBoundaryRow is ProgramArtifact's domain-neutral callable interface.
// It references the already-sealed Body and Outcome planes and owns the sole
// reusable formal/capture relation. Pack may refine these rows, but cannot be
// their authority.
type FunctionBoundaryRow struct {
	id, body, bodyContext, entry, callFormal keyspace.ContentID
	formals                                  []FunctionFormalPort
	vararg                                   FunctionVarargPort
	hasVararg                                bool
	captures                                 []FunctionCapturePort
	outcomes                                 []keyspace.ContentID
	sealed                                   bool
}

func (row FunctionBoundaryRow) Available() bool {
	if !row.sealed || !row.id.Available() || !row.body.Available() || !row.bodyContext.Available() ||
		!row.entry.Available() || !row.callFormal.Available() || row.hasVararg != row.vararg.Available() || len(row.outcomes) == 0 {
		return false
	}
	for index, port := range row.formals {
		if !port.Available() || uint64(index) != uint64(port.position) {
			return false
		}
	}
	for index, port := range row.captures {
		if !port.Available() || uint64(index) != uint64(port.position) || port.innerBody != row.body {
			return false
		}
	}
	for _, outcome := range row.outcomes {
		if !outcome.Available() {
			return false
		}
	}
	return true
}
func (row FunctionBoundaryRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row FunctionBoundaryRow) BodyID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body
}
func (row FunctionBoundaryRow) BodyContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.bodyContext
}
func (row FunctionBoundaryRow) EntryID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.entry
}
func (row FunctionBoundaryRow) CallFormalID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.callFormal
}
func (row FunctionBoundaryRow) FormalCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.formals)
}
func (row FunctionBoundaryRow) FormalAt(index int) (FunctionFormalPort, bool) {
	if !row.Available() || index < 0 || index >= len(row.formals) {
		return FunctionFormalPort{}, false
	}
	return row.formals[index], true
}
func (row FunctionBoundaryRow) Vararg() (FunctionVarargPort, bool) {
	return row.vararg, row.Available() && row.hasVararg
}
func (row FunctionBoundaryRow) CaptureCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.captures)
}
func (row FunctionBoundaryRow) CaptureAt(index int) (FunctionCapturePort, bool) {
	if !row.Available() || index < 0 || index >= len(row.captures) {
		return FunctionCapturePort{}, false
	}
	return row.captures[index], true
}
func (row FunctionBoundaryRow) OutcomeCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.outcomes)
}
func (row FunctionBoundaryRow) OutcomeAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.outcomes) {
		return keyspace.ContentID{}, false
	}
	return row.outcomes[index], true
}
func (compiler *compiler) copyFunctionBoundariesFailure() CompileFailure {
	if compiler == nil || len(compiler.bodies) == 0 || len(compiler.outcomes) == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	rows := make([]FunctionBoundaryRow, 0)
	for bodyIndex := 0; bodyIndex < compiler.input.BodyCount(); bodyIndex++ {
		body, bodyOK := compiler.input.BodyAt(bodyIndex)
		if !bodyOK || !compiler.input.OwnsBody(body) || bodyIndex >= len(compiler.bodies) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		function, callable := body.TransformerFunction()
		if !callable {
			continue
		}
		copiedBody := compiler.bodies[bodyIndex]
		callFormal, callFormalOK := body.CallTarget()
		callFormalID, callFormalIDOK := callFormal.ID()
		if !compiler.input.OwnsFunction(function) || !copiedBody.Callable() || !callFormalOK || !callFormalIDOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		row := FunctionBoundaryRow{
			id: function.ContextID(), body: copiedBody.ID(), bodyContext: copiedBody.ContextID(),
			entry: copiedBody.EntryID(), callFormal: callFormalID, sealed: true,
		}
		for position := 0; position < function.FormalCount(); position++ {
			formal, formalOK := function.FormalAt(position)
			cell, cellOK := formal.Cell()
			storage, storageOK := formal.StorageCell()
			declared, _ := formal.DeclaredStaticTypeReferenceID()
			formalPosition, positionOK := formal.Position()
			if !formalOK || !cellOK || !storageOK || !positionOK || formalPosition != position ||
				!compiler.input.OwnsFormal(formal) || !compiler.input.OwnsCell(cell) || !compiler.input.OwnsCell(storage) ||
				uint64(position) > uint64(^uint32(0)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			row.formals = append(row.formals, FunctionFormalPort{
				id: formal.ContextID(), cell: cell.ContextID(), storage: storage.ContextID(), declared: declared, position: uint32(position),
			})
		}
		if vararg, varargOK := function.Vararg(); varargOK {
			cell, cellOK := vararg.Cell()
			if !cellOK || !compiler.input.OwnsVararg(vararg) || !compiler.input.OwnsCell(cell) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			row.vararg = FunctionVarargPort{id: vararg.ContextID(), cell: cell.ContextID()}
			row.hasVararg = true
		}
		for position := 0; position < function.CaptureCount(); position++ {
			capture, captureOK := function.CaptureAt(position)
			inner, innerOK := capture.Inner()
			outer, outerOK := capture.Outer()
			capturePosition, positionOK := capture.Position()
			if !captureOK || !innerOK || !outerOK || !positionOK || capturePosition != position ||
				!compiler.input.OwnsCapture(capture) || !compiler.input.OwnsCell(inner) || !compiler.input.OwnsCell(outer) ||
				uint64(position) > uint64(^uint32(0)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			row.captures = append(row.captures, FunctionCapturePort{
				id: capture.ContextID(), inner: inner.ContextID(), outer: outer.ContextID(),
				innerBody: capture.InnerBodyPathID(), outerBody: capture.OuterBodyPathID(), position: uint32(position),
			})
		}
		for outcomeIndex := copiedBody.outcomeStart; outcomeIndex < copiedBody.outcomeEnd; outcomeIndex++ {
			if uint64(outcomeIndex) >= uint64(len(compiler.outcomes)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, int(outcomeIndex-copiedBody.outcomeStart), CompileReasonOutcomeRange)
			}
			row.outcomes = append(row.outcomes, compiler.outcomes[outcomeIndex].ID())
		}
		if !row.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		rows = append(rows, row)
	}
	compiler.functionBoundaries = rows
	return CompileFailure{}
}
