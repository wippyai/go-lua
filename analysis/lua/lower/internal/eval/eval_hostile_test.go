package eval

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	lowercollector "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestScheduleValuesRejectsMissingStaticsBeforeScheduling(t *testing.T) {
	phases := new(continuation.Stack)
	values := New(
		phases,
		lowercollector.New("eval-hostile.lua", 0, bind.GlobalCensus{}),
		continuation.NewExpressions(phases),
		nil,
	)

	err := values.ScheduleValues(nil, 1, source.Span{File: "eval-hostile.lua"})
	if err == nil {
		t.Fatal("ScheduleValues(nil statics) succeeded")
	}
	if len(values.steps) != 0 || len(values.terms) != 0 || len(values.open) != 0 {
		t.Fatal("ScheduleValues mutated continuation state after rejecting missing statics")
	}
}

func TestScheduleExpressionAndRunRejectMissingStaticsSafely(t *testing.T) {
	phases := new(continuation.Stack)
	values := New(
		phases,
		lowercollector.New("eval-hostile.lua", 0, bind.GlobalCensus{}),
		continuation.NewExpressions(phases),
		nil,
	)
	if err := values.ScheduleExpression(nil, 1, source.Span{File: "eval-hostile.lua"}); err == nil {
		t.Fatal("ScheduleExpression(nil statics) succeeded")
	}
	if err := values.Run(); err == nil {
		t.Fatal("Run on an unconfigured Values succeeded")
	}
}
