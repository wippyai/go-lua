package control

import (
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestControlAuthorityRequiresConcreteCrossings(t *testing.T) {
	writer := New(nil, nil, nil, nil, nil, nil, nil, "control.lua")
	if writer.Clean() != true {
		t.Fatal("new control writer retained pending state")
	}
	if err := writer.ready(); err == nil {
		t.Fatal("control writer accepted incomplete dependencies")
	}
}

func TestControlFlowReportsCollectorFailureAsTypedError(t *testing.T) {
	var writer Writer
	if _, err := writer.Return(programsource.Span{File: "control.lua"}, 1, 0); err == nil {
		t.Fatal("Return accepted an unavailable Flow collector")
	}
}

func TestLoopSchedulingRejectsAbsentStatements(t *testing.T) {
	var writer Writer
	if err := writer.beginRepeat(nil, 1, writer.span(nil)); err == nil {
		t.Fatal("beginRepeat accepted an absent statement")
	}
	if err := writer.beginNumberFor(nil, 1, writer.span(nil)); err == nil {
		t.Fatal("beginNumberFor accepted an absent statement")
	}
}
