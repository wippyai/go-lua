package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSegmentRouteBuildersRejectUnavailableInputs(t *testing.T) {
	graph := &Result{}
	from := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	to := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if segment, err := graph.RootOutcomeSegment(zeroSourceControlSourceView(), nil, from, to, keyspace.MakeTerm(keyspace.FamilyBody, 1), NoSegmentCarrier()); err == nil || segment.MatchesRoute(from, to) {
		t.Fatalf("RootOutcomeSegment accepted unavailable inputs: segment=%v err=%v", segment, err)
	}
	if segment, err := graph.OutcomePropagationSegment(zeroSourceControlSourceView(), nil, to, to); err == nil || segment.MatchesRoute(to, to) {
		t.Fatalf("OutcomePropagationSegment accepted unavailable inputs: segment=%v err=%v", segment, err)
	}
}
