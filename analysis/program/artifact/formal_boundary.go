package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// copyFunctionBoundariesFailure emits the callable-interface families
// directly into the compiler publication.  The compiler keeps these rows only
// until Freeze; the sealed Artifact retains no boundary slice or inverse.
func (compiler *compiler) copyFunctionBoundariesFailure() CompileFailure {
	if compiler == nil || len(compiler.bodies) == 0 || len(compiler.outcomes) == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
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
		callFormal, callFormalOK := flowView.CallBodyTarget(function)
		callFormalID, callFormalIDOK := callFormal.ID()
		if !functionOK || !copiedBody.Callable() || copiedBody.OutcomeCount() == 0 || !callFormalOK || !callFormalIDOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}

		if !fitsUint32(len(compiler.functionFormals)) || !fitsUint32(len(compiler.functionCaptures)) || !fitsUint32(len(compiler.functionVarargs)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		formalOffset := uint32(len(compiler.functionFormals))
		for position := 0; position < function.FormalCount(); position++ {
			formalID, cellID, storageID, declared, formalOK := compiler.input.FunctionFormalAt(function, position)
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
			captureID, innerID, outerID, innerBodyID, outerBodyID, captureOK := compiler.input.FunctionCaptureAt(function, position)
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
