package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
)

func TestPreparedBodyOwnsReusableWTOPlan(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, "function f(x) local y = x while y do y = nil end return y end")
	prepared, err := PrepareBoundFunction(fn, bind.BindFunction(fn, bind.Options{}), Config{Registry: reg, Schedule: transfer.ScheduleWTODual})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	if prepared.wtoPlan == nil {
		t.Fatal("prepared body has nil WTO plan")
	}
	owned := prepared.wtoPlan
	var comparisons []transfer.WTOComparison
	first := solvePreparedForTest(t, prepared, SolveConfig{
		Schedule:   transfer.ScheduleWTODual,
		CompareWTO: func(report transfer.WTOComparison) { comparisons = append(comparisons, report) },
	})
	second := solvePreparedForTest(t, prepared, SolveConfig{
		Schedule:   transfer.ScheduleWTODual,
		CompareWTO: func(report transfer.WTOComparison) { comparisons = append(comparisons, report) },
	})
	if prepared.wtoPlan != owned {
		t.Fatal("prepared WTO plan changed across solves")
	}
	if len(comparisons) != 2 || comparisons[0].Fallback || comparisons[1].Fallback {
		t.Fatalf("comparisons = %#v, want two eligible runs", comparisons)
	}
	domain := state.Domain(reg)
	firstExit, firstOK := first.ExitState()
	secondExit, secondOK := second.ExitState()
	if !firstOK || !secondOK || !domain.Equal(firstExit, secondExit) {
		t.Fatalf("repeated dual exit states differ: first=%v/%v second=%v/%v", firstExit, firstOK, secondExit, secondOK)
	}
}

func TestPreparedBodyDefaultFIFODoesNotBuildWTOPlan(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, "function f(x) return x end")
	prepared, err := PrepareBoundFunction(fn, bind.BindFunction(fn, bind.Options{}), Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	if prepared.wtoPlan != nil {
		t.Fatal("default FIFO preparation built a WTO plan")
	}
	result := solvePreparedForTest(t, prepared, SolveConfig{})
	if _, ok := result.ExitState(); !ok {
		t.Fatal("default FIFO solve has no exit")
	}
}
