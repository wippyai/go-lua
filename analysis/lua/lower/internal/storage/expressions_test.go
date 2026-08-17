package storage

import (
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestStorageExpressionDispatchRejectsForeignSyntax(t *testing.T) {
	var writer Writer
	if err := writer.ScheduleExpression(nil, 1, programsource.Span{File: "storage.lua"}); err == nil {
		t.Fatal("ScheduleExpression accepted foreign syntax")
	}
}
