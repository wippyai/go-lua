package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	suspensionmember "github.com/wippyai/go-lua/domain/placement/suspension/memberdefinition"
)

// The evidence producer is an independent rule on its own axis. It reads the
// same two produced vectors under its OWN keys - one vector answering to two
// writers would be one row with two owners - and writes the evidence cell
// alone, never Placement class.
func TestEvidenceContributionStaysIndependentOfPlacementClass(t *testing.T) {
	evidence := Contribution()
	class := suspensionmember.Contribution()
	if !evidence.Available() || evidence.Axis != "placement-suspension-evidence" || evidence.Rule != "placement-suspension-evidence" {
		t.Fatalf("evidence contribution identity=%q/%q available=%t", evidence.Axis, evidence.Rule, evidence.Available())
	}
	if evidence.Axis == class.Axis || evidence.Rule == class.Rule {
		t.Fatalf("evidence and class contributions share identity: %q/%q", evidence.Axis, evidence.Rule)
	}
	if len(evidence.Relations) != 2 || len(evidence.Projections) != 5 || len(evidence.Selections) != 2 || len(evidence.Reducers) != 1 {
		t.Fatalf("evidence contribution relations=%d projections=%d selections=%d reducers=%d",
			len(evidence.Relations), len(evidence.Projections), len(evidence.Selections), len(evidence.Reducers))
	}
	for _, relation := range evidence.Relations {
		for _, mirrored := range class.Relations {
			if relation.Key == mirrored.Key {
				t.Fatalf("evidence relation %q reuses the consumer's key", relation.Key)
			}
		}
	}

	reducer := evidence.Reducers[0]
	classReducer := class.Reducers[0]
	if reducer.Implementation == classReducer.Implementation {
		t.Fatal("evidence and class contributions share a reducer symbol")
	}
	if reducer.Outputs[0].Carrier != "EvidenceFactCarrier" || reducer.Outputs[0].Carrier == classReducer.Outputs[0].Carrier {
		t.Fatalf("evidence output carrier=%q, want the evidence cell alone", reducer.Outputs[0].Carrier)
	}
	// The vector is the same shape read under this rule's own tag vocabulary:
	// sharing the consumer's tag would make one correlation answer to two
	// writers, which is the same defect the distinct route tags prevent.
	if len(reducer.Inputs) != 2 || reducer.Inputs[0].Carrier != classReducer.Inputs[0].Carrier ||
		reducer.Inputs[0].Form != member.ReadFormSummary || reducer.Inputs[0].Multiplicity != member.MultiplicityMany {
		t.Fatalf("evidence source vector input=%+v, want the whole-vector delivery", reducer.Inputs[0])
	}
	if reducer.Inputs[0].Tag == classReducer.Inputs[0].Tag {
		t.Fatalf("source tag vocabulary was duplicated: %q", reducer.Inputs[0].Tag)
	}
	if reducer.Inputs[1].Carrier != "EvidenceFactCarrier" || reducer.Inputs[1].Tag == classReducer.Inputs[1].Tag ||
		reducer.Inputs[1].Route != "PlacementKeyCarrier" {
		t.Fatalf("evidence routed input=%+v, want its own tag over the shared canonical route coordinate", reducer.Inputs[1])
	}
}
