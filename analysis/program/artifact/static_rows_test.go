package artifact

import "testing"

func TestStaticRowsRejectOutOfRangeChildQueries(t *testing.T) {
	artifact := &Artifact{}
	if artifact.StaticTypeValueCount() != 0 {
		t.Fatal("unavailable artifact exposed static rows")
	}
	program := artifact.Program()
	if program.Available() {
		t.Fatal("unavailable artifact exposed a Program")
	}
	if count, ok := program.StaticTypeNodeCount(); ok || count != 0 {
		t.Fatal("unavailable Program exposed static node rows")
	}
}
