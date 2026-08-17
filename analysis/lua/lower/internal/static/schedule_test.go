package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticScheduleRequiresReadyAuthorities(t *testing.T) {
	var w Writer
	if err := w.ScheduleType(&ast.PrimitiveTypeExpr{}, 1, 1, spanForTest()); err == nil {
		t.Fatal("ScheduleType accepted an uninitialized static writer")
	}
	if err := w.scheduleType(nil, 1, 1, spanForTest()); err == nil {
		t.Fatal("scheduleType accepted a nil type continuation")
	}
}
