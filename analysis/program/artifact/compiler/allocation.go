package compiler

import "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"

// copyAllocationRowsFailure delegates the complete allocation construction
// join to its internal owner. The compiler remains the sole authority that
// maps a child-local construction coordinate onto public CompileFailure.
func (compiler *compiler) copyAllocationRowsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	bundle, fault := allocation.Build(allocation.Input{
		Program: compiler.input,
		Values:  compiler.publication.Values,
	})
	if fault.Failed() || bundle == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, fault.Row(), fault.Field(), CompileReasonOccurrenceAllocation)
	}
	compiler.allocations = bundle
	return CompileFailure{}
}
