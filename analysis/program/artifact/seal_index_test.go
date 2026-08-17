package artifact

import "testing"

func TestSealIndexRejectsNilValidationState(t *testing.T) {
	artifact := &Artifact{}
	if failure := artifact.validateSealIndexes(nil); !failure.Available() || failure.Stage() != CompileStageSeal || failure.RowKind() != CompileRowAuthority || failure.Reason() != CompileReasonArtifactIdentity {
		t.Fatal("seal index phase admitted missing state")
	}
}
