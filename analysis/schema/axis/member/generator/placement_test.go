package generator

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// TestResolvePlacementStorageKeepsForeignCandidateOwner is Store's law, so it
// names Store's rows. An axis is a growing catalog - every rule that folds
// into Placement adds relations, projections and a reducer to it - and a law
// that pinned the axis's census would measure the roster's size rather than
// the property it exists for: that a relation provided by another axis's
// candidate keeps that provider, gains no local ordinal, and hands its
// projections the same foreign authority.
func TestResolvePlacementStorageKeepsForeignCandidateOwner(t *testing.T) {
	metadata, err := Resolve(composedSource(t, "placement"))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Axis != "placement" {
		t.Fatalf("placement metadata names axis %q", metadata.Axis)
	}

	var relation RelationBinding
	var found bool
	for _, candidate := range metadata.Relations {
		if candidate.Key == "placement/store/storage-routes" {
			relation, found = candidate, true
		}
	}
	if !found {
		t.Fatalf("placement declares no Store route relation: %+v", metadata.Relations)
	}
	if relation.CandidateProvider.AxisRelation.Axis.Key != "value" || relation.CandidateProvider.AxisRelation.Member != "value/storage-transfer/candidates" || relation.CandidateProviderLocal || relation.HasCandidateRelation {
		t.Fatalf("foreign route provider was localized: %+v", relation)
	}
	// Store states the DECLARED derivation: the emitter writes the enumeration,
	// the union, the widening and the order, and the only authored symbols left
	// are the two judgments that say what one atom of a Value and one row of
	// Placement's own directory mean. Resolution carries that statement whole -
	// a metadata form that kept only part of it would answer an empty
	// derivation for every relation that has migrated off the authored quartet.
	if len(relation.Derivation.Source) != 1 || relation.Derivation.Source[0].Axis.Key != "value" || relation.Derivation.Source[0].Name != "Atoms" ||
		relation.Derivation.Resolve.Name != "ResolveRoute" || relation.Derivation.InlineWidth != 8 ||
		relation.Derivation.Widen.Predicate.Name != "BeyondAllocations" || relation.Derivation.Widen.Resolve.Name != "ResolveDirectoryRoute" ||
		len(relation.Derivation.Widen.Source) != 1 || relation.Derivation.Widen.Source[0].Axis.Key != "placement" || relation.Derivation.Widen.Source[0].Name != "AllocationDirectory" ||
		relation.Derivation.AuthoredDerivation() ||
		len(relation.Derivation.StaticAxes) != 2 || relation.Derivation.StaticAxes[0].Key != "placement" || relation.Derivation.StaticAxes[1].Key != "value" {
		t.Fatalf("Store relation derivation was not preserved: %+v", relation.Derivation)
	}
	projections := 0
	for index, projection := range metadata.Projections {
		if projection.Relation != relation.Key {
			continue
		}
		projections++
		if projection.CandidateProvider != relation.CandidateProvider || projection.CandidateProviderLocal || projection.CandidateRelation != 0 {
			t.Fatalf("projection[%d] provider=%+v, want foreign provider without local ordinal", index, projection)
		}
	}
	if projections != 3 {
		t.Fatalf("Store declares %d projections over its route relation, want its key, tag and destination", projections)
	}

	var reducer ReducerBinding
	found = false
	for _, candidate := range metadata.Reducers {
		if candidate.Key == "placement/store/reducer/storage" {
			reducer, found = candidate, true
		}
	}
	if !found {
		t.Fatalf("placement declares no Store reducer: %+v", metadata.Reducers)
	}
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
