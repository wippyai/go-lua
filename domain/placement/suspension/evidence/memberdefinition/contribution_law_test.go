package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	suspensionmember "github.com/wippyai/go-lua/domain/placement/suspension/memberdefinition"
)

// Evidence writes its own Factor but consumes the canonical Placement route
// row. It therefore owns no second source/vector relation, projection, or
// selection authority.
func TestEvidenceContributionStaysIndependentOfPlacementClass(t *testing.T) {
	evidence := Contribution()
	class := suspensionmember.Contribution()
	if !evidence.Available() || evidence.Axis != "placement-suspension-evidence" || evidence.Rule != "placement-suspension-evidence" {
		t.Fatalf("evidence contribution identity=%q/%q available=%t", evidence.Axis, evidence.Rule, evidence.Available())
	}
	if evidence.Axis == class.Axis || evidence.Rule == class.Rule {
		t.Fatalf("evidence and class contributions share identity: %q/%q", evidence.Axis, evidence.Rule)
	}
	if len(evidence.Relations) != 0 || len(evidence.Projections) != 0 || len(evidence.Selections) != 0 || len(evidence.Reducers) != 1 {
		t.Fatalf("evidence contribution relations=%d projections=%d selections=%d reducers=%d",
			len(evidence.Relations), len(evidence.Projections), len(evidence.Selections), len(evidence.Reducers))
	}
	for _, carrier := range evidence.Carriers {
		if carrier.Name == "SuspensionSourceCarrier" || carrier.Name == "SuspensionRouteCarrier" || carrier.Name == "SuspensionEvidenceRouteTagCarrier" {
			t.Fatalf("evidence retains duplicate route authority carrier=%q", carrier.Name)
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
	// The complete Value span belongs to canonical SuspensionRoutes. Evidence
	// receives the four scalar columns of its already materialized route row,
	// so it cannot reopen, rebuild, or reinterpret that denominator.
	want := []string{"SourceSummaryCarrier", "PlacementKeyCarrier", "SuspensionRouteTagCarrier", "EvidenceFactCarrier"}
	if len(reducer.Inputs) != len(want) {
		t.Fatalf("evidence inputs=%d, want %d", len(reducer.Inputs), len(want))
	}
	for index, input := range reducer.Inputs {
		if input.Carrier != want[index] || input.Form != member.ReadFormSelected || input.Multiplicity != member.MultiplicityOne || input.Tag != "SuspensionRouteTagCarrier" {
			t.Fatalf("evidence scalar input %d=%+v, want canonical selected route column %q", index, input, want[index])
		}
	}
	if reducer.Inputs[3].Route != "PlacementKeyCarrier" {
		t.Fatalf("evidence routed input=%+v, want canonical route destination key", reducer.Inputs[3])
	}
}
