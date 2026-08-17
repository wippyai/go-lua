package assembly

import (
	"testing"

	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func TestAssemblyControlFaultRequiresAValidOwner(t *testing.T) {
	c := newAssemblyCollector()
	if got := c.ControlFault(assemblyTestSpan(), 0, programsource.ControlFaultUndefinedGoto, 0, 0); got != 0 || c.err == nil {
		t.Fatalf("invalid control fault = %d with error %v", got, c.err)
	}
}
