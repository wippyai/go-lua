package compiler

import "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"

// copyBodyBoundaryFailure is the single parent handoff for the complete body,
// outcome, and callable-boundary plane. Construction and its indexes belong to
// bodyboundary; the compiler retains only the child Bundle until publication.
func (compiler *compiler) copyBodyBoundaryFailure() CompileFailure {
	if compiler == nil {
		_, fault := bodyboundary.Build(bodyboundary.Input{})
		return CompileFailure{construction: fault}
	}
	bundle, fault := bodyboundary.Build(bodyboundary.Input{
		Program:        compiler.input,
		ProgramID:      compiler.key.ProgramID(),
		Values:         compiler.publication.Values,
		PointIDsBySite: compiler.pointIDsBySite,
	})
	if fault.Available() {
		return CompileFailure{construction: fault}
	}
	compiler.bodyBoundary = bundle
	return CompileFailure{}
}
