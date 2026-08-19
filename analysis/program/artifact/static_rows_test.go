package artifact

import "testing"

func TestStaticRowsRejectOutOfRangeChildQueries(t *testing.T) {
	artifact := &Artifact{}
	if artifact.StaticTypeValueCount() != 0 || artifact.StaticTypeNodeCount() != 0 {
		t.Fatal("unavailable artifact exposed static rows")
	}
}
