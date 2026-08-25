package memberdefinition

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestContributionsKeepClassAndEvidenceWritersDistinct(t *testing.T) {
	class := Contribution()
	evidence := EvidenceContribution()
	if !class.Available() || !evidence.Available() {
		t.Fatalf("suspension contributions unavailable: class=%t evidence=%t", class.Available(), evidence.Available())
	}
	if class.Axis == evidence.Axis || class.Rule == evidence.Rule {
		t.Fatalf("class/evidence contributions share identity: class=%q/%q evidence=%q/%q", class.Axis, class.Rule, evidence.Axis, evidence.Rule)
	}
	classReducer := class.Reducers[0]
	evidenceReducer := evidence.Reducers[0]
	if classReducer.Implementation == evidenceReducer.Implementation {
		t.Fatal("class and evidence contributions share a reducer symbol")
	}
	if classReducer.Outputs[0].Carrier != "PlacementFactCarrier" {
		t.Fatalf("class output carrier=%q, want PlacementFactCarrier", classReducer.Outputs[0].Carrier)
	}
	if evidenceReducer.Outputs[0].Carrier != "EvidenceFactCarrier" {
		t.Fatalf("evidence output carrier=%q, want EvidenceFactCarrier", evidenceReducer.Outputs[0].Carrier)
	}
	if evidenceReducer.Outputs[0].Carrier == classReducer.Outputs[0].Carrier {
		t.Fatal("evidence contribution writes the Placement class carrier")
	}
	// Two inputs, not three: the anchor read states the denominator the source
	// vector is complete against, and a denominator is not a fold input. What
	// each fold takes is the vector and the routed cell it publishes into.
	if len(classReducer.Inputs) != 2 || len(evidenceReducer.Inputs) != 2 {
		t.Fatalf("class/evidence input counts=%d/%d, want the vector and the routed cell", len(classReducer.Inputs), len(evidenceReducer.Inputs))
	}
	if classReducer.Inputs[0] != evidenceReducer.Inputs[0] {
		t.Fatalf("neutral source vector differs between class and evidence: %#v != %#v", classReducer.Inputs[0], evidenceReducer.Inputs[0])
	}
	if classReducer.Inputs[0].Form != member.ReadFormSummary || classReducer.Inputs[0].Multiplicity != member.MultiplicityMany ||
		classReducer.Inputs[0].Tag == "" {
		t.Fatalf("source vector input=%#v, want a tagged whole-vector delivery", classReducer.Inputs[0])
	}
	if classReducer.Inputs[1].Carrier != "PlacementFactCarrier" || evidenceReducer.Inputs[1].Carrier != "EvidenceFactCarrier" {
		t.Fatalf("route input carriers=%q/%q, want distinct owner cells", classReducer.Inputs[1].Carrier, evidenceReducer.Inputs[1].Carrier)
	}
	if classReducer.Inputs[1].Tag == evidenceReducer.Inputs[1].Tag {
		t.Fatalf("route tag vocabulary was duplicated: class=%q evidence=%q", classReducer.Inputs[1].Tag, evidenceReducer.Inputs[1].Tag)
	}
	if classReducer.Inputs[1].Route != "PlacementKeyCarrier" || evidenceReducer.Inputs[1].Route != "PlacementKeyCarrier" {
		t.Fatalf("route coordinate carriers=%q/%q, want shared canonical PlacementKeyCarrier", classReducer.Inputs[1].Route, evidenceReducer.Inputs[1].Route)
	}
}

func TestContributionsDoNotDeclareUnknownFallbacks(t *testing.T) {
	for name, contribution := range map[string]struct {
		axis string
		key  string
	}{
		"class":    {axis: string(Contribution().Axis), key: string(Contribution().Reducers[0].Key)},
		"evidence": {axis: string(EvidenceContribution().Axis), key: string(EvidenceContribution().Reducers[0].Key)},
	} {
		if name == "" || contribution.axis == "" || contribution.key == "" || strings.Contains(contribution.key, "unknown") {
			t.Fatalf("%s contribution has an invalid reducer identity: axis=%q key=%q", name, contribution.axis, contribution.key)
		}
	}
}
