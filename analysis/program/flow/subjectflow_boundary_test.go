package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
)

func TestSubjectFlowPairsYieldWithExactCallReentry(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{Name: "subject-flow-call.lua", Text: []byte(`
local function run(value)
  return value
end
return run(1)
`)})
	if err != nil {
		t.Fatal(err)
	}
	projection := program.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("SubjectFlow was unavailable for a dynamic-call fixture")
	}
	if projection.BoundaryCount() == 0 {
		t.Fatal("dynamic call yielded no suspension boundary")
	}
	for index := 0; index < projection.BoundaryCount(); index++ {
		boundary, ok := projection.BoundaryAt(index)
		if !ok || !boundary.ID.Available() || boundary.YieldArm != causal.BoundaryYield || !boundary.CallPath.Available() || !boundary.YieldRoute.Available() {
			t.Fatalf("boundary %d = %#v/%v, want issued Yield route", index, boundary, ok)
		}
		callPath, callPathOK := program.Flow().CallPath(boundary.Call)
		if !callPathOK || callPath != boundary.CallPath {
			t.Fatalf("boundary %d CallPath = %v/%v, want Flow CallPath %v", index, boundary.CallPath, callPathOK, callPath)
		}
		if boundary.State == subjectflow.BoundaryPaired {
			if !boundary.ReentryRoute.Available() || !boundary.ReentryFromPath.Available() || !boundary.ReentryToPath.Available() {
				t.Fatalf("paired boundary %d lost re-entry path: %#v", index, boundary)
			}
		}
	}
	if projection.LivenessCount() == 0 {
		t.Fatal("paired dynamic call yielded no per-subject liveness rows")
	}
	for index := 0; index < projection.LivenessCount(); index++ {
		row, ok := projection.LivenessAt(index)
		if !ok || !row.ID.Available() || !row.YieldRoute.Available() || !row.Subject.ID.Available() {
			t.Fatalf("liveness %d = %#v/%v, want issued route and subject", index, row, ok)
		}
		if row.State != subjectflow.LivenessUnknown && row.State != subjectflow.LivenessLive && row.State != subjectflow.LivenessDiesBefore {
			t.Fatalf("liveness %d state = %d, want closed tri-state", index, row.State)
		}
	}
}

func TestSubjectFlowLivenessIsPartitionedByYieldRoute(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{Name: "subject-flow-two-calls.lua", Text: []byte(`
local function run(value)
  return value
end
local first = run(1)
local second = run(first)
return second
`)})
	if err != nil {
		t.Fatal(err)
	}
	projection := program.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("SubjectFlow was unavailable for a two-call fixture")
	}
	routes := make(map[[32]byte]int)
	for index := 0; index < projection.LivenessCount(); index++ {
		row, ok := projection.LivenessAt(index)
		if !ok {
			t.Fatalf("LivenessAt(%d) failed", index)
		}
		routes[row.YieldRoute]++
	}
	if len(routes) < 2 {
		t.Fatalf("liveness routes = %d, want one partition per non-tail call", len(routes))
	}
	for route, count := range routes {
		if count == 0 || route == ([32]byte{}) {
			t.Fatalf("liveness route %x has no rows", route[:4])
		}
	}
}
