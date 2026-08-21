package flow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
)

func TestSubjectFlowPublishesNeutralFactsAndUnknownReturnBoundary(t *testing.T) {
	assembly := openProjectionAssembly(t, "subject-flow.lua")
	_, flowComponent, _, _, err := assembly.Take()
	if err != nil {
		t.Fatalf("Assembly.Take: %v", err)
	}
	projection := flowComponent.View().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("valid assembled SubjectFlow projection was unavailable")
	}
	if projection.EventCount() == 0 {
		t.Fatal("SubjectFlow published no local facts for a non-empty executable fixture")
	}
	hasDefine, hasUse, hasUnknown := false, false, false
	ids := make(map[[32]byte]struct{}, projection.EventCount())
	for index := 0; index < projection.EventCount(); index++ {
		event, ok := projection.EventAt(index)
		if !ok || !event.ID.Available() || !event.Path.Available() {
			t.Fatalf("event %d = %#v/%v, want issued path fact", index, event, ok)
		}
		if _, duplicate := ids[event.ID]; duplicate {
			t.Fatalf("event identity %v repeated", event.ID)
		}
		ids[event.ID] = struct{}{}
		switch event.Kind {
		case subjectflow.EventDefine:
			hasDefine = true
		case subjectflow.EventUse:
			hasUse = true
		case subjectflow.EventUnknown:
			hasUnknown = true
		}
	}
	if !hasDefine || !hasUse || !hasUnknown {
		t.Fatalf("event vocabulary = define=%v use=%v unknown=%v", hasDefine, hasUse, hasUnknown)
	}
	if projection.BoundaryCount() != 0 {
		// This fixture has no dynamic Call; a route-less program must not gain
		// a synthetic suspension boundary.
		t.Fatalf("non-call fixture published %d suspension boundaries", projection.BoundaryCount())
	}
}
