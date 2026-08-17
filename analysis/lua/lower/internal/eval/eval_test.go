package eval

import (
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestValuesRejectsSchedulingWithoutTypedEvaluationOwners(t *testing.T) {
	values := New(nil, nil, nil, nil)
	if err := values.ScheduleExpression(nil, 1, programsource.Span{File: "eval.lua"}); err == nil {
		t.Fatal("eval Values accepted a schedule without its typed owners")
	}
	if err := values.Run(); err == nil {
		t.Fatal("eval Values ran without a pending continuation")
	}
}
