package compiler

import (
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

// copyAllocationRowsFailure delegates the complete allocation construction
// join to its internal owner. The child returns the schema-owned refusal
// directly; the compiler only carries it through its public failure value.
func (compiler *compiler) copyAllocationRowsFailure() CompileFailure {
	if compiler == nil {
		return CompileFailure{construction: programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, -1, -1)}
	}
	bundle, fault := allocation.Build(allocation.Input{
		Program: compiler.input,
		Values:  compiler.publication.Values,
	})
	if fault.Available() {
		return CompileFailure{construction: fault}
	}
	if bundle == nil {
		return CompileFailure{construction: programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, -1, -1)}
	}
	compiler.allocations = bundle
	return CompileFailure{}
}
