package control

import "testing"

func TestLoopSchedulingRejectsAbsentStatements(t *testing.T) {
	var writer Writer
	if err := writer.beginRepeat(nil, 1, writer.span(nil)); err == nil {
		t.Fatal("beginRepeat accepted an absent statement")
	}
	if err := writer.beginNumberFor(nil, 1, writer.span(nil)); err == nil {
		t.Fatal("beginNumberFor accepted an absent statement")
	}
}
