package subjectflow_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
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
