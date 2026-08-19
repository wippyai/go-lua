package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// FunctionFormalPort is one ordered fixed input of a reusable Program
// transformer. ID and CellID belong to Program's formal namespace;
// StorageCellID is the exact bridge used by Link-local value substitution.
type FunctionFormalPort struct {
	id, cell, storage, declared identity.ContentID
	position                    uint32
}

func (port FunctionFormalPort) Available() bool {
	return port.id.Available() && port.cell.Available() && port.storage.Available()
}
func (port FunctionFormalPort) ID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.id
}
func (port FunctionFormalPort) CellID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.cell
}
func (port FunctionFormalPort) StorageCellID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.storage
}
func (port FunctionFormalPort) DeclaredStaticTypeID() (identity.ContentID, bool) {
	return port.declared, port.Available() && port.declared.Available()
}
func (port FunctionFormalPort) Position() (int, bool) {
	return int(port.position), port.Available()
}

// FunctionVarargPort is the optional open input of one Function boundary.
// It remains distinct from fixed formal storage: no storage coordinate is
// invented when Program did not issue one.
type FunctionVarargPort struct{ id, cell identity.ContentID }

func (port FunctionVarargPort) Available() bool {
	return port.id.Available() && port.cell.Available()
}
func (port FunctionVarargPort) ID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.id
}
func (port FunctionVarargPort) CellID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.cell
}

// FunctionCapturePort is one ordered lexical interface edge. Inner/Outer are
// role-specific Cell identities; the Body identities retain the direction of
// the edge without exposing authored Terms or live Flow handles.
type FunctionCapturePort struct {
	id, inner, outer     identity.ContentID
	innerBody, outerBody identity.ContentID
	position             uint32
}

func (port FunctionCapturePort) Available() bool {
	return port.id.Available() && port.inner.Available() && port.outer.Available() &&
		port.innerBody.Available() && port.outerBody.Available() &&
		port.inner != port.outer && port.innerBody != port.outerBody
}
func (port FunctionCapturePort) ID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.id
}
func (port FunctionCapturePort) InnerCellID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.inner
}
func (port FunctionCapturePort) OuterCellID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.outer
}
func (port FunctionCapturePort) InnerBodyID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.innerBody
}
func (port FunctionCapturePort) OuterBodyID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
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
	id, body, bodyContext, entry, callFormal identity.ContentID
	formals                                  []FunctionFormalPort
	vararg                                   FunctionVarargPort
	hasVararg                                bool
	captures                                 []FunctionCapturePort
	sealed                                   bool
}

func (row FunctionBoundaryRow) Available() bool {
	if !row.sealed || !row.id.Available() || !row.body.Available() || !row.bodyContext.Available() ||
		!row.entry.Available() || !row.callFormal.Available() || row.hasVararg != row.vararg.Available() {
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
	return true
}
func (row FunctionBoundaryRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row FunctionBoundaryRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row FunctionBoundaryRow) BodyContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.bodyContext
}
func (row FunctionBoundaryRow) EntryID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.entry
}
func (row FunctionBoundaryRow) CallFormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
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
func (compiler *compiler) copyFunctionBoundariesFailure() CompileFailure {
	if compiler == nil || len(compiler.bodies) == 0 || len(compiler.outcomes) == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	rows := make([]FunctionBoundaryRow, 0)
	flowView := compiler.input.Flow()
	for bodyIndex := 0; bodyIndex < compiler.input.BodyCount(); bodyIndex++ {
		body, bodyOK := compiler.input.BodyAt(bodyIndex)
		if !bodyOK || !compiler.input.OwnsBody(body) || bodyIndex >= len(compiler.bodies) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		function, callable := body.Function()
		functionID, functionOK := compiler.input.FunctionID(function)
		if !callable {
			continue
		}
		copiedBody := compiler.bodies[bodyIndex]
		callFormal, callFormalOK := body.CallTarget()
		callFormalID, callFormalIDOK := callFormal.ID()
		if !functionOK || !copiedBody.Callable() || copiedBody.OutcomeCount() == 0 || !callFormalOK || !callFormalIDOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		row := FunctionBoundaryRow{
			id: functionID, body: copiedBody.ID(), bodyContext: copiedBody.ContextID(),
			entry: copiedBody.EntryID(), callFormal: callFormalID, sealed: true,
		}
		for position := 0; position < function.FormalCount(); position++ {
			formalID, cellID, storageID, declared, formalOK := compiler.input.FunctionFormalAt(function, position)
			if !formalOK || uint64(position) > uint64(^uint32(0)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			row.formals = append(row.formals, FunctionFormalPort{
				id: formalID, cell: cellID, storage: storageID, declared: declared, position: uint32(position),
			})
		}
		if varargID, cellID, varargOK := flowView.FunctionVarargIDs(function); varargOK {
			if !varargID.Available() || !cellID.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			row.vararg = FunctionVarargPort{id: varargID, cell: cellID}
			row.hasVararg = true
		}
		for position := 0; position < function.CaptureCount(); position++ {
			captureID, innerID, outerID, innerBodyID, outerBodyID, captureOK := compiler.input.FunctionCaptureAt(function, position)
			if !captureOK || uint64(position) > uint64(^uint32(0)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			row.captures = append(row.captures, FunctionCapturePort{
				id: captureID, inner: innerID, outer: outerID,
				innerBody: innerBodyID, outerBody: outerBodyID, position: uint32(position),
			})
		}
		if !row.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		rows = append(rows, row)
	}
	compiler.functionBoundaries = rows
	return CompileFailure{}
}

// FunctionBoundaryCount and FunctionBoundaryAt expose the sole neutral
// callable-interface denominator. The rows are content-addressed Program
// artifact data; Link supplies only mounted actual/callback substitutions.
func (artifact *Artifact) FunctionBoundaryCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.functionBoundaries)
}
func (artifact *Artifact) FunctionBoundaryAt(index int) (FunctionBoundaryRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.functionBoundaries) {
		return FunctionBoundaryRow{}, false
	}
	return artifact.functionBoundaries[index], true
}

// FunctionBoundaryForBody resolves the sole callable boundary owned by a
// Body. The inverse is sealed with the Artifact and keeps consumers from
// rebuilding a body-to-function map beside the canonical column.
func (artifact *Artifact) FunctionBoundaryForBody(bodyID identity.ContentID) (FunctionBoundaryRow, bool) {
	if artifact == nil || !artifact.Available() || !bodyID.Available() || artifact.functionBoundaryByBody == nil {
		return FunctionBoundaryRow{}, false
	}
	index, ok := artifact.functionBoundaryByBody[bodyID]
	if !ok || uint64(index) >= uint64(len(artifact.functionBoundaries)) {
		return FunctionBoundaryRow{}, false
	}
	row := artifact.functionBoundaries[index]
	return row, row.Available() && row.BodyID() == bodyID
}
