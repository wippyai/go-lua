package control

import (
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestControlFlowReportsCollectorFailureAsTypedError(t *testing.T) {
	var writer Writer
	if _, err := writer.Return(programsource.Span{File: "control.lua"}, 1, 0); err == nil {
		t.Fatal("Return accepted an unavailable Flow collector")
	}
}
