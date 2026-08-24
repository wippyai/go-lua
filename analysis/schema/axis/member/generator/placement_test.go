package generator

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestResolvePlacementStorageKeepsForeignCandidateOwner(t *testing.T) {
	metadata, err := Resolve(composedSource(t, "placement"))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Axis != "placement" || len(metadata.Relations) != 1 || len(metadata.Projections) != 3 || len(metadata.Reducers) != 1 {
		t.Fatalf("placement metadata shape=%+v", metadata)
	}

	relation := metadata.Relations[0]
	if relation.CandidateProvider.AxisRelation.Axis.Key != "value" || relation.CandidateProvider.AxisRelation.Member != "value/storage-transfer/candidates" || relation.CandidateProviderLocal || relation.HasCandidateRelation {
		t.Fatalf("foreign route provider was localized: %+v", relation)
	}
	if relation.Derivation.State.Name != "RoutePlan" || relation.Derivation.Build.Name != "DeriveRoutes" || relation.Derivation.Count.Name != "RouteCount" || relation.Derivation.At.Name != "RouteAt" ||
		len(relation.Derivation.StaticAxes) != 2 || relation.Derivation.StaticAxes[0].Key != "placement" || relation.Derivation.StaticAxes[1].Key != "value" {
		t.Fatalf("Store relation derivation was not preserved: %+v", relation.Derivation)
	}
	for index, projection := range metadata.Projections {
		if projection.CandidateProvider != relation.CandidateProvider || projection.CandidateProviderLocal || projection.CandidateRelation != 0 {
			t.Fatalf("projection[%d] provider=%+v, want foreign provider without local ordinal", index, projection)
		}
	}

	reducer := metadata.Reducers[0]
	if !reducer.CandidatePresent || reducer.Candidate.Name != "StorageTransfer" || reducer.CandidateConstant || len(reducer.Inputs) != 2 || len(reducer.Outputs) != 1 || reducer.Implementation.Name != "StorageFold" {
		t.Fatalf("placement storage reducer=%+v", reducer)
	}
	if reducer.Inputs[0].Axis.Key != "value" || reducer.Inputs[0].Type.Name != "Value" || reducer.Inputs[0].Form != member.ReadFormExact ||
		reducer.Inputs[1].Axis.Key != "placement" || reducer.Inputs[1].Type.Name != "Fact" || reducer.Inputs[1].Form != member.ReadFormSelected || reducer.Inputs[1].Tag.Name != "uint64" ||
		reducer.Outputs[0].Axis.Key != "placement" || reducer.Outputs[0].Type.Name != "Fact" {
		t.Fatalf("placement reducer signature=%+v", reducer)
	}
}

func TestRenderPlacementStorageDoesNotEmitForeignDirectory(t *testing.T) {
	artifact, err := Render("placement", composedSource(t, "placement"))
	if err != nil {
		t.Fatal(err)
	}
	cold := string(artifact.Cold)
	if !strings.Contains(cold, `Member: "value/storage-transfer/candidates"`) {
		t.Fatalf("cold catalog lost explicit foreign provider:\n%s", cold)
	}
	relations := string(artifact.Relations)
	for _, leaked := range []string{"StorageTransferAt", "StorageTransferOrdinal", "StorageTransferForArtifactOccurrence"} {
		if strings.Contains(relations, leaked) {
			t.Fatalf("foreign candidate directory leaked into Placement owner: %s\n%s", leaked, relations)
		}
	}
}
