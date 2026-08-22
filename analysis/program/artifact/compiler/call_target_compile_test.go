package compiler

import (
	"testing"

	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

func TestCallTargetCompilerRequiresCopiedBodies(t *testing.T) {
	failure := (&compiler{}).copyCallTargetsFailure()
	if !failure.Available() || !failure.Construction().Available() || failure.Construction().Family() != programcatalog.CallTarget() || failure.Construction().Issue() != programconstruction.IssueCallTargetUnavailable {
		t.Fatal("call-target compiler admitted an empty body projection")
	}
}
