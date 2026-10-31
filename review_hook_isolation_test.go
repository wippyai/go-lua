package lua

import (
	"strings"
	"testing"
)

func TestReviewDebugHookObservesDistinctChunksWithTheSameLineNumber(t *testing.T) {
	L := NewState()
	defer L.Close()
	hits := 0
	L.SetGlobal("reviewRecordLine", L.NewFunction(func(L *LState) int {
		if L.CheckString(1) == "line" {
			hits++
		}
		return 0
	}))
	if err := L.DoString(`debug.sethook(reviewRecordLine, "l")`); err != nil {
		t.Fatal(err)
	}
	hits = 0
	for _, source := range []string{"@review-first.lua", "@review-second.lua"} {
		fn, err := L.Load(strings.NewReader("return 42"), source)
		if err != nil {
			t.Fatal(err)
		}
		L.Push(fn)
		if err := L.PCall(0, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	if hits < 2 {
		t.Fatalf("both executed chunks need line events, got %d", hits)
	}
}

func TestReviewPoolResetClearsDebugHookAuthority(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`debug.sethook(function() end, "l", 7)`); err != nil {
		t.Fatal(err)
	}
	resetLState(L)
	if (L.hook != nil && L.hook != LNil) || L.hookMask != 0 || L.hookCount != 0 || L.hookCounter != 0 || L.hookLastLine != 0 || L.hookLastProto != nil || L.hookLastPC != 0 || L.inHook {
		t.Fatal("pooled state retains the prior owner's hook or hook accounting")
	}
}

func TestReviewDebugHookObservesSameLineLoopBackEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	hits := 0
	L.SetGlobal("reviewRecordLoop", L.NewFunction(func(L *LState) int {
		if L.CheckString(1) == "line" {
			hits++
		}
		return 0
	}))
	if err := L.DoString(`debug.sethook(reviewRecordLoop, "l")`); err != nil {
		t.Fatal(err)
	}
	hits = 0
	if err := L.DoString(`local n = 0; for i=1,5 do n=n+i end; assert(n==15)`); err != nil {
		t.Fatal(err)
	}
	if hits < 5 {
		t.Fatalf("same-line loop needs repeated line events, got %d", hits)
	}
}

func TestReviewChangingHookDoesNotReenterActiveCallback(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
        local inside, armed, reentered = false, false, false
        local function second() if inside then reentered = true end end
        local function first()
            if armed then return end
            armed = true
            inside = true
            debug.sethook(nil)
            debug.sethook(second, "l")
            local value = 42
            inside = false
        end
        debug.sethook(first, "l")
        local value = 42
        debug.sethook()
        assert(not reentered, "changing hooks reentered the active callback")
    `)
}
