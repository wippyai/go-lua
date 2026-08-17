package routeplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
)

func TestNewRequiresAnAvailableSourceControlOwner(t *testing.T) {
	if builder, err := New(sourcecontrol.Owner{}); err == nil || builder != nil {
		t.Fatalf("New accepted an unavailable owner: builder=%v err=%v", builder, err)
	}
}

func TestTypedOriginsFailClosedUntilEndpointsAreIssued(t *testing.T) {
	origin := CSRPhasePair(sourcecontrol.PhaseRef{}, sourcecontrol.PhaseRef{})
	if _, _, ok := origin.Endpoints(); ok {
		t.Fatal("zero CSR origin exposed endpoints")
	}
	if _, ok := origin.RecurrenceCarrier(); ok {
		t.Fatal("zero CSR origin exposed a recurrence carrier")
	}
	if _, _, ok := OutcomeSubdivision(nil, sourcecontrol.Segment{}); ok {
		t.Fatal("nil SourceControl graph admitted an Outcome subdivision")
	}
	if _, _, ok := OutcomeResumeSubdivision(nil, nil, runtimeResumeRowForTest()); ok {
		t.Fatal("nil owners admitted an Outcome resume subdivision")
	}
}

func runtimeResumeRowForTest() (row sourcecontrol.OutcomeResumeRow) { return row }

func TestZeroRouteCannotBeEmittedWithoutAnOrigin(t *testing.T) {
	var builder *Builder
	if err := builder.Emit(Route{}, Origin{}); err == nil {
		t.Fatal("nil Builder accepted a route declaration")
	}
	if plan, err := builder.Seal(); err == nil || plan != nil {
		t.Fatalf("nil Builder sealed: plan=%v err=%v", plan, err)
	}
}
