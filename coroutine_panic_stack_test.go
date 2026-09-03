package lua

import (
	"context"
	"strings"
	"testing"
)

func TestResumePanicIncludesGoStack(t *testing.T) {
	L := NewState(Options{IncludeGoStackTrace: true})
	defer L.Close()

	L.SetGlobal("panic_from_go", L.NewFunction(func(*LState) int {
		panic("coroutine panic sentinel")
	}))
	if err := L.DoString(`function run_panic() panic_from_go() end`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.Background())
	fn := L.GetGlobal("run_panic").(*LFunction)
	state, _, err := L.Resume(co, fn)
	if state != ResumeError {
		t.Fatalf("resume state = %v, want ResumeError", state)
	}
	if err == nil {
		t.Fatal("resume error is nil")
	}
	if !strings.Contains(err.Error(), "coroutine panic sentinel") {
		t.Fatalf("resume error = %q, want panic message", err)
	}
	if !strings.Contains(err.Error(), "TestResumePanicIncludesGoStack") {
		t.Fatalf("resume error = %q, want Go stack", err)
	}
}
