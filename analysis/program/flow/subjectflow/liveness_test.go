package subjectflow_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAggregateLivenessRequiresAnAllPathProof(t *testing.T) {
	cases := []struct {
		name   string
		states []subjectflow.LivenessState
		want   subjectflow.LivenessState
	}{
		{name: "all dies before", states: []subjectflow.LivenessState{subjectflow.LivenessDiesBefore, subjectflow.LivenessDiesBefore}, want: subjectflow.LivenessDiesBefore},
		{name: "all live", states: []subjectflow.LivenessState{subjectflow.LivenessLive, subjectflow.LivenessLive}, want: subjectflow.LivenessLive},
		{name: "mixed", states: []subjectflow.LivenessState{subjectflow.LivenessDiesBefore, subjectflow.LivenessLive}, want: subjectflow.LivenessUnknown},
		{name: "missing", states: nil, want: subjectflow.LivenessUnknown},
		{name: "unknown arm", states: []subjectflow.LivenessState{subjectflow.LivenessDiesBefore, subjectflow.LivenessUnknown}, want: subjectflow.LivenessUnknown},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := subjectflow.AggregateLiveness(test.states); got != test.want {
				t.Fatalf("AggregateLiveness(%v) = %d, want %d", test.states, got, test.want)
			}
		})
	}
}

// An opaque call/return event after re-entry cannot erase a witnessed use of
// a local that crosses the yield. Unknown prevents a DiesBefore proof; it is
// not evidence against an exact positive liveness proof.
func TestCoroutineLocalUsedAfterYieldRemainsLiveAcrossOpaquePostEvents(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "subject-liveness-yield.lua", Text: []byte(`
local function run()
    local captured = { value = 1 }
    coroutine.yield()
    return captured.value
end
local wrapped = coroutine.wrap(run)
wrapped()
return wrapped
`)})
	if err != nil {
		t.Fatal(err)
	}
	projection := program.Flow().SubjectFlow()
	if projection == nil || !projection.Available() || projection.LivenessCount() == 0 {
		t.Fatal("coroutine fixture published no subject-liveness rows")
	}
	liveTables := 0
	for index := 0; index < projection.LivenessCount(); index++ {
		row, rowOK := projection.LivenessAt(index)
		if !rowOK {
			t.Fatalf("subject-liveness row %d is unavailable", index)
		}
		if row.Subject.Kind == subjectflow.SubjectValue && keyspace.TermFamily(row.Subject.Term) == keyspace.FamilyTable && row.State == subjectflow.LivenessLive {
			liveTables++
		}
	}
	if liveTables == 0 {
		t.Fatal("local used after coroutine.yield has no exact Live table-allocation judgment")
	}
}
