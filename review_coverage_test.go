package lua

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewCoverageRecordsSeparateChunksWithEqualLineNumbers(t *testing.T) {
	if !CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	L := NewState()
	defer L.Close()
	for _, source := range []string{"@review-cov-first.lua", "@review-cov-second.lua"} {
		fn, err := L.Load(strings.NewReader("return 42"), source)
		if err != nil {
			t.Fatal(err)
		}
		L.Push(fn)
		if err := L.PCall(0, 0, nil); err != nil {
			t.Fatal(err)
		}
		found, hit := CoverageSummary(func(s string) bool { return s == source })
		if found < 1 || hit < 1 {
			t.Fatalf("executed source %s has coverage %d/%d", source, hit, found)
		}
	}
}

func TestReviewCoverageSurvivesClearingUserDebugHooks(t *testing.T) {
	if !CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	L := NewState()
	defer L.Close()
	if err := L.DoString(`debug.sethook(nil)`); err != nil {
		t.Fatal(err)
	}
	source := "@review-cov-after-clear.lua"
	fn, err := L.Load(strings.NewReader("return 42"), source)
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	found, hit := CoverageSummary(func(s string) bool { return s == source })
	if found < 1 || hit < 1 {
		t.Fatalf("clearing debug hook disabled coverage: %d/%d", hit, found)
	}
}

func TestReviewCoverageIncludesDebugHookCallbackCode(t *testing.T) {
	if !CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	L := NewState()
	defer L.Close()
	source := "@review-cov-callback.lua"
	fn, err := L.Load(strings.NewReader("return function()\n local n = 42\n return n\nend"), source)
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		t.Fatal(err)
	}
	L.SetGlobal("reviewCoverageCallback", L.Get(-1))
	L.Pop(1)
	if err := L.DoString(`debug.sethook(reviewCoverageCallback, "l"); local n=42; debug.sethook()`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "callback.info")
	if err := WriteCoverageLCOV(path, func(s string) bool { return s == source }); err != nil {
		t.Fatal(err)
	}
	trace, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bodyLine := range []string{"DA:2,1\n", "DA:3,1\n"} {
		if !strings.Contains(string(trace), bodyLine) {
			t.Fatalf("executed callback body line missing: %s", trace)
		}
	}
}

func TestReviewCoverageRetainsUnexecutedFunctionLines(t *testing.T) {
	if !CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	L := NewState()
	defer L.Close()
	source := "@review-cov-unexecuted.lua"
	fn, err := L.Load(strings.NewReader("local function unused()\n return 99\nend\nreturn 42"), source)
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "unexecuted.info")
	if err := WriteCoverageLCOV(path, func(s string) bool { return s == source }); err != nil {
		t.Fatal(err)
	}
	trace, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(trace), "DA:2,0\n") {
		t.Fatalf("unexecuted function body needs zero credit: %s", trace)
	}
}

func TestReviewCoveragePreservesUserHookMask(t *testing.T) {
	if !CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
        local callback = function() end
        debug.sethook(callback, "", 7)
        local hook, mask, count = debug.gethook()
        assert(hook == callback and mask == "" and count == 7)
        debug.sethook()
        assert(debug.gethook() == nil)
    `)
}
