package lua

import (
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
