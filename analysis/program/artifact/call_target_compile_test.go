package artifact

import "testing"

func TestCallTargetCompilerRequiresCopiedBodies(t *testing.T) {
	failure := (&compiler{}).copyCallTargetsFailure()
	if !failure.Available() || failure.Stage() != CompileStageBodyOutcomes || failure.RowKind() != CompileRowBody {
		t.Fatal("call-target compiler admitted an empty body projection")
	}
}
