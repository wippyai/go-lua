package lua

import (
	"context"
	"strings"
	"testing"
)

func TestResumePreCanceledContextRestoresThreadState(t *testing.T) {
	for _, useResumeInto := range []bool{false, true} {
		api := "Resume"
		if useResumeInto {
			api = "ResumeInto"
		}
		for _, cancelAfterYield := range []bool{false, true} {
			phase := "before_start"
			if cancelAfterYield {
				phase = "after_yield"
			}
			t.Run(api+"/"+phase, func(t *testing.T) {
				L := NewState()
				defer L.Close()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				co := L.NewThreadWithContext(ctx)
				fn := L.NewFunction(func(L *LState) int {
					return L.Yield(LString("token"))
				})

				if cancelAfterYield {
					state, values, err := resumeForContextTest(L, co, fn, useResumeInto)
					if err != nil || state != ResumeYield || len(values) != 1 || values[0] != LString("token") {
						t.Fatalf("first resume = (%v, %v, %v), want (ResumeYield, [token], nil)", state, values, err)
					}
				}
				cancel()

				state, _, err := resumeForContextTest(L, co, fn, useResumeInto)
				if state != ResumeError || err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
					t.Fatalf("canceled resume = (%v, %v), want ResumeError with context cancellation", state, err)
				}
				if !co.Dead {
					t.Fatal("canceled coroutine remained resumable")
				}
				if co.Parent != nil {
					t.Fatalf("canceled coroutine retained parent %p", co.Parent)
				}
				if L.G.CurrentThread != L {
					t.Fatalf("current thread = %p, want parent %p", L.G.CurrentThread, L)
				}
				if status := L.Status(co); status != "dead" {
					t.Fatalf("canceled coroutine status = %q, want dead", status)
				}

				state, _, err = resumeForContextTest(L, co, fn, useResumeInto)
				if state != ResumeError || err == nil || !strings.Contains(err.Error(), "can not resume a dead thread") {
					t.Fatalf("resume after cancellation = (%v, %v), want dead-thread error", state, err)
				}
			})
		}
	}
}

func resumeForContextTest(L, co *LState, fn *LFunction, useResumeInto bool) (ResumeState, []LValue, error) {
	if useResumeInto {
		return L.ResumeInto(co, fn, make([]LValue, 0, 2))
	}
	return L.Resume(co, fn)
}
