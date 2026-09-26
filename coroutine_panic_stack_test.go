package lua

import (
	"context"
	"strings"
	"testing"
)

func TestResumePanicIncludesGoStack(t *testing.T) {
	var logged string
	L := NewState(Options{
		IncludeGoStackTrace: true,
		PanicHandler: func(_ *LState, message string) {
			logged = message
		},
	})
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
	if logged != err.Error() {
		t.Fatalf("panic handler message = %q, want returned error", logged)
	}
}

func TestResumePanicHandlerPanicKeepsOriginalError(t *testing.T) {
	L := NewState(Options{
		PanicHandler: func(*LState, string) {
			panic("panic handler sentinel")
		},
	})
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
	if err == nil || !strings.Contains(err.Error(), "coroutine panic sentinel") {
		t.Fatalf("resume error = %v, want original panic", err)
	}
	if strings.Contains(err.Error(), "panic handler sentinel") {
		t.Fatalf("resume error = %q, want no handler panic", err)
	}
	if !co.Dead {
		t.Fatal("coroutine is not dead")
	}
	if status := L.Status(co); status != "dead" {
		t.Fatalf("coroutine status = %q, want dead", status)
	}
}
