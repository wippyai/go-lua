package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestContributionDeclaresEqualityJoinsAndDirectFold(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() {
		t.Fatal("equality contribution is not available")
	}
	if contribution.Axis != "value" || contribution.Rule != "value-binary-equality" {
		t.Fatalf("contribution identity = %q/%q", contribution.Axis, contribution.Rule)
	}
	if len(contribution.Relations) != 1 || len(contribution.Projections) != 3 || len(contribution.Reducers) != 1 {
		t.Fatalf("contribution rows relations=%d projections=%d reducers=%d, want 1/3/1", len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
	}

	relation := contribution.Relations[0]
	if relation.Name != "BinaryEqualitySources" || relation.Subject != "ValueFactCarrier" || len(relation.Inputs) != 1 || relation.Inputs[0].Carrier != "BinaryEqualityCarrier" {
		t.Fatalf("source relation = %#v", relation)
	}
	provider := relation.CandidateProvider
	if provider.Member != "value/binary-equality/candidates" {
		t.Fatalf("source provider = %#v", provider)
	}
	wantProjections := []struct {
		name, key, relation, accessor string
		role                          member.Role
	}{
		{"BinaryEqualityLeft", "value/binary-equality/left", "BinaryEqualitySources", "Left", member.Key},
		{"BinaryEqualityRight", "value/binary-equality/right", "BinaryEqualitySources", "Right", member.Key},
		{"BinaryEqualityWrite", "value/binary-equality/write", "BinaryEqualityCandidates", "Write", member.Destination},
	}
	for index, want := range wantProjections {
		projection := contribution.Projections[index]
		if projection.Name != want.name || string(projection.Key) != want.key || projection.Relation != want.relation || projection.Accessor.Name != want.accessor || projection.Role != want.role || projection.CandidateProvider != provider {
			t.Fatalf("projection[%d] = %#v, want %s.%s", index, projection, want.relation, want.accessor)
		}
	}

	reducer := contribution.Reducers[0]
	if reducer.Name != "BinaryEqualityReducer" || reducer.Key != "value/binary-equality/reducer" || reducer.Candidate != "BinaryEqualityCarrier" || len(reducer.Inputs) != 2 || len(reducer.Outputs) != 1 {
		t.Fatalf("reducer = %#v", reducer)
	}
	for index, input := range reducer.Inputs {
		if input.Axis.Key != "value" || input.Carrier != "ValueFactCarrier" || input.Form != member.ReadFormExact || input.Multiplicity != member.MultiplicityOne {
			t.Fatalf("reducer input[%d] = %#v", index, input)
		}
	}
	if reducer.Outputs[0].Axis.Key != "value" || reducer.Outputs[0].Carrier != "ValueFactCarrier" || reducer.Implementation.Name != "EqualityValue" || reducer.Implementation.PackagePath != valuePackagePath {
		t.Fatalf("reducer output/implementation = %#v/%#v", reducer.Outputs[0], reducer.Implementation)
	}
}
