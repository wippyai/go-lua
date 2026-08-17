package artifact

import "testing"

func TestStaticRowsRejectOutOfRangeChildQueries(t *testing.T) {
	artifact := &Artifact{}
	if artifact.StaticTypeArgumentCount() != 0 || artifact.StaticTypeValueCount() != 0 || artifact.StaticTypeNodeCount() != 0 {
		t.Fatal("unavailable artifact exposed static rows")
	}
	if _, ok := artifact.StaticTypeArgumentAt(0); ok {
		t.Fatal("static argument query exposed an unavailable row")
	}
}
