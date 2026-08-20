package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	callableCellFormal       = uint64(1)
	callableCellCaptureInner = uint64(3)
	callableCellCaptureOuter = uint64(4)
)

// copyFunctionBoundariesFailure emits the callable-interface families
// directly into the compiler publication.  The compiler keeps these rows only
// until Freeze; the sealed Artifact retains no boundary slice or inverse.
func (compiler *compiler) copyFunctionBoundariesFailure() CompileFailure {
	if compiler == nil || len(compiler.bodies) == 0 || len(compiler.outcomes) == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	flowView := compiler.input.Flow()
	programID := compiler.key.ProgramID()
	staticView := compiler.input.Static()
	for bodyIndex := 0; bodyIndex < compiler.input.BodyCount(); bodyIndex++ {
		body, bodyOK := compiler.input.BodyAt(bodyIndex)
		if !bodyOK || !compiler.input.OwnsBody(body) || bodyIndex >= len(compiler.bodies) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		function, callable := body.Function()
		if !callable {
			continue
		}
		copiedBody := compiler.bodies[bodyIndex]
		functionID, functionOK := copiedBody.FunctionContextID()
		callFormal, callFormalOK := flowView.CallBodyTarget(function)
		callFormalID, callFormalIDOK := callFormal.ID()
		copiedFormalID, copiedFormalIDOK := copiedBody.CallFormalID()
		if !functionOK || !copiedBody.Callable() || copiedBody.OutcomeCount() == 0 || !callFormalOK || !callFormalIDOK || !copiedFormalIDOK || copiedFormalID != callFormalID {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}

		if !fitsUint32(len(compiler.functionFormals)) || !fitsUint32(len(compiler.functionCaptures)) || !fitsUint32(len(compiler.functionVarargs)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		formalOffset := uint32(len(compiler.functionFormals))
		for position := 0; position < function.FormalCount(); position++ {
			formalID, cellID, storageID, declared, formalOK := artifactFunctionFormalAt(programID, flowView, staticView, function, position)
			if !formalOK || uint64(position) > uint64(^uint32(0)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			formal, formalSealed := programschema.NewFunctionFormal(formalID, cellID, storageID, declared, uint32(position))
			if !formalSealed {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			compiler.functionFormals = append(compiler.functionFormals, formal)
		}

		varargOffset := uint32(len(compiler.functionVarargs))
		varargCount := uint32(0)
		if varargID, cellID, varargOK := flowView.FunctionVarargIDs(function); varargOK {
			vararg, varargSealed := programschema.NewFunctionVararg(varargID, cellID)
			if !varargSealed {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			compiler.functionVarargs = append(compiler.functionVarargs, vararg)
			varargCount = 1
		}

		captureOffset := uint32(len(compiler.functionCaptures))
		for position := 0; position < function.CaptureCount(); position++ {
			captureID, innerID, outerID, innerBodyID, outerBodyID, captureOK := artifactFunctionCaptureAt(flowView, function, position)
			if !captureOK || uint64(position) > uint64(^uint32(0)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			capture, captureSealed := programschema.NewFunctionCapture(captureID, innerID, outerID, innerBodyID, outerBodyID, uint32(position))
			if !captureSealed {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, position, CompileReasonBodyUnavailable)
			}
			compiler.functionCaptures = append(compiler.functionCaptures, capture)
		}

		boundary, boundarySealed := programschema.NewFunctionBoundary(
			functionID, copiedBody.ID(), copiedBody.ContextID(), copiedBody.EntryID(), callFormalID,
			formalOffset, uint32(function.FormalCount()), varargOffset, varargCount,
			captureOffset, uint32(function.CaptureCount()),
		)
		if !boundarySealed {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		compiler.functionBoundaries = append(compiler.functionBoundaries, boundary)
	}
	return CompileFailure{}
}

// artifactFunctionFormalAt joins one Flow formal with its lexical Cell,
// storage Cell, and optional Static declaration. An absent declaration is a
// valid unannotated formal and therefore does not invalidate the row.
func artifactFunctionFormalAt(programID identity.ContentID, view flow.View, staticView staticquery.View, boundary functionboundary.Boundary, index int) (formalID, cellID, storageID, declaredTypeID identity.ContentID, ok bool) {
	if !programID.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	term, termOK := boundary.FormalAt(index)
	bodyTerm, bodyTermOK := boundary.Body()
	boundaries := view.FunctionBoundaries()
	body, bodyOK := boundaries.ForBody(bodyTerm)
	pathID, pathOK := view.BodyPath(bodyTerm)
	if !termOK || term == 0 || !bodyTermOK || !bodyOK || !boundaries.OwnsBody(body) || !pathOK || !pathID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	cellID = artifactCallableCellID(pathID, callableCellFormal, uint64(index))
	formalID = artifactCallableSemanticID("program/transformer/formal", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Uint(uint64(index)) == nil && writer.Bytes(cellID[:]) == nil
	})
	storageID, storageOK := artifactStorageCellID(programID, view, term)
	if !cellID.Available() || !formalID.Available() || !storageOK || !storageID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	declaredTypeID, _ = artifactDeclaredStaticTypeID(programID, staticView, term)
	return formalID, cellID, storageID, declaredTypeID, true
}

// artifactStorageCellID is the explicit Artifact handoff for the shared
// storage-role identity. The Cell relation remains Flow-owned; only the root
// fence is supplied by the Artifact compiler.
func artifactStorageCellID(programID identity.ContentID, view flow.View, term keyspace.Term) (identity.ContentID, bool) {
	if !programID.Available() || term == 0 {
		return identity.ContentID{}, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(term); !cellOK {
		return identity.ContentID{}, false
	}
	return programschema.StorageCellIdentity(programID, term)
}

// artifactFunctionCaptureAt joins one ordered Flow capture pair. Both Body
// paths are already sealed by Flow; the scalar equations remain byte-for-byte
// identical to the former Program projection.
func artifactFunctionCaptureAt(view flow.View, boundary functionboundary.Boundary, index int) (id, innerID, outerID, innerBodyID, outerBodyID identity.ContentID, ok bool) {
	if index < 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	pair, pairOK := boundary.CaptureAt(index)
	bodyTerm, bodyTermOK := boundary.Body()
	boundaries := view.FunctionBoundaries()
	body, bodyOK := boundaries.ForBody(bodyTerm)
	innerBody, innerBodyOK := boundaries.ForBody(pair.InnerBody)
	outerBody, outerBodyOK := boundaries.ForBody(pair.OuterBody)
	if !pairOK || pair.Inner == 0 || pair.Outer == 0 || !bodyTermOK || !bodyOK || !innerBodyOK || !outerBodyOK ||
		!boundaries.OwnsBody(body) || !boundaries.OwnsBody(innerBody) || !boundaries.OwnsBody(outerBody) || !innerBody.Equal(body) || outerBody.Equal(innerBody) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	innerBodyID, innerBodyOK = view.BodyPath(pair.InnerBody)
	outerBodyID, outerBodyOK = view.BodyPath(pair.OuterBody)
	if !innerBodyOK || !outerBodyOK || !innerBodyID.Available() || !outerBodyID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	innerID = artifactCallableCellID(innerBodyID, callableCellCaptureInner, uint64(index))
	outerID = artifactCallableCellID(outerBodyID, callableCellCaptureOuter, uint64(index))
	id = artifactCallableSemanticID("program/transformer/capture", func(writer *framing.Writer) bool {
		return writer.Bytes(innerBodyID[:]) == nil && writer.Bytes(outerBodyID[:]) == nil &&
			writer.Uint(uint64(index)) == nil && writer.Bytes(innerID[:]) == nil && writer.Bytes(outerID[:]) == nil
	})
	return id, innerID, outerID, innerBodyID, outerBodyID, id.Available() && innerID.Available() && outerID.Available()
}

func artifactCallableCellID(bodyPath identity.ContentID, role, index uint64) identity.ContentID {
	return artifactCallableSemanticID("program/transformer/cell-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Uint(role) == nil && writer.Uint(index) == nil
	})
}

func artifactDeclaredStaticTypeID(programID identity.ContentID, view staticquery.View, cell keyspace.Term) (identity.ContentID, bool) {
	if !programID.Available() || !view.Available() || cell == 0 {
		return identity.ContentID{}, false
	}
	declarations := view.Declarations().DeclaredTypes()
	declaration, declarationOK := declarations.ForCell(cell)
	declaredCell, target, rowOK := declarations.Get(declaration)
	ref, refOK := view.StaticTypes().Ref(target)
	id, idOK := staticquery.TypeReferenceID(programID, ref)
	if !declarationOK || !rowOK || declaredCell != cell || !refOK || ref.Term() != target || !idOK {
		return identity.ContentID{}, false
	}
	return id, true
}

func writeCallableTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0 &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}

func artifactCallableSemanticID(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// functionBoundaryIDs is used only by compiler-side summary and validation
// passes while the canonical rows are still being assembled.
func functionBoundaryIDs(row programschema.FunctionBoundary) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	if !row.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	return row.ID(), row.BodyID(), row.CallFormalID(), true
}

func (compiler *compiler) functionFormalAt(boundary programschema.FunctionBoundary, index int) (programschema.FunctionFormal, bool) {
	if compiler == nil || index < 0 || index >= boundary.FormalCount() {
		return programschema.FunctionFormal{}, false
	}
	offset, _, ok := boundary.FormalSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(compiler.functionFormals)) {
		return programschema.FunctionFormal{}, false
	}
	formal := compiler.functionFormals[int(offset)+index]
	return formal, formal.Available()
}

func (compiler *compiler) functionVararg(boundary programschema.FunctionBoundary) (programschema.FunctionVararg, bool) {
	if compiler == nil || !boundary.HasVararg() {
		return programschema.FunctionVararg{}, false
	}
	offset, count, ok := boundary.VarargSpan()
	if !ok || count != 1 || uint64(offset) >= uint64(len(compiler.functionVarargs)) {
		return programschema.FunctionVararg{}, false
	}
	vararg := compiler.functionVarargs[offset]
	return vararg, vararg.Available()
}

func (compiler *compiler) functionCaptureAt(boundary programschema.FunctionBoundary, index int) (programschema.FunctionCapture, bool) {
	if compiler == nil || index < 0 || index >= boundary.CaptureCount() {
		return programschema.FunctionCapture{}, false
	}
	offset, _, ok := boundary.CaptureSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(compiler.functionCaptures)) {
		return programschema.FunctionCapture{}, false
	}
	capture := compiler.functionCaptures[int(offset)+index]
	return capture, capture.Available() && capture.InnerBodyID() == boundary.BodyID()
}

func (compiler *compiler) functionBoundaryForBody(bodyID identity.ContentID) (programschema.FunctionBoundary, bool) {
	if compiler == nil || !bodyID.Available() {
		return programschema.FunctionBoundary{}, false
	}
	var found programschema.FunctionBoundary
	for _, boundary := range compiler.functionBoundaries {
		if !boundary.Available() || boundary.BodyID() != bodyID {
			continue
		}
		if found.Available() {
			return programschema.FunctionBoundary{}, false
		}
		found = boundary
	}
	return found, found.Available()
}
