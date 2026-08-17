package call

import (
	"testing"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestCallWriterRequiresAllTypedOwnersBeforeScheduling(t *testing.T) {
	writer := New(nil, nil, nil, nil, nil, nil, nil, nil, "calls.lua")
	if writer == nil || !writer.Clean() {
		t.Fatal("New did not return a clean Call writer")
	}
	call := &ast.FuncCallExpr{}
	if err := writer.Schedule(call, 1, programsource.Span{File: "calls.lua"}); err == nil {
		t.Fatal("Call writer accepted scheduling without its typed owners")
	}
}
