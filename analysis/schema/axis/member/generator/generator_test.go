package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	heapmemberdefinition "github.com/wippyai/go-lua/domain/heap/memberdefinition"
	packmemberdefinition "github.com/wippyai/go-lua/domain/pack/memberdefinition"
	staticmemberdefinition "github.com/wippyai/go-lua/domain/static/memberdefinition"
	"github.com/wippyai/go-lua/domain/value/memberdefinition"
)

func externalProviderDefinition() definition.Definition {
	owner := definition.GoType{PackagePath: "example/placement", Name: "Schema"}
	candidate := definition.GoType{PackagePath: "example/value", Name: "StorageTransfer"}
	fact := definition.GoType{PackagePath: "example/placement", Name: "Fact"}
	key := definition.GoType{PackagePath: "example/placement", Name: "Key"}
	axis := func(name string) schema.EntryReference {
		return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(name)}
	}
	provider := member.RelationRef{Axis: axis("value"), Member: "value/storage-transfer/candidates"}
	return definition.Definition{
		Name: "PlacementStore", Axis: "placement",
		Binding:     definition.Binding{Key: definition.KeyNormalization{Carrier: "Key", Dense: definition.GoType{Name: "uint32"}, Normalizer: definition.GoSymbol{PackagePath: "example/placement", Name: "Normalize", Receiver: owner, ResultIndex: 0}}},
		Signature:   definition.Signature{Key: "Key", Fact: "Fact"},
		Carriers:    []definition.Carrier{{Name: "Candidate", Key: "carrier/value/storage-transfer", Type: candidate}, {Name: "Fact", Key: "carrier/placement/fact", Type: fact}, {Name: "Key", Key: "carrier/placement/key", Type: key}},
		Relations:   []definition.Relation{{Name: "Route", Key: "placement/store/route", Subject: "Fact", Inputs: []string{"Candidate", "Fact"}, CandidateProvider: provider}},
		Projections: []definition.Projection{{Name: "Destination", Key: "placement/store/destination", Relation: "Route", Role: member.Destination, Result: "Key", CandidateProvider: provider, Accessor: definition.GoSymbol{PackagePath: "example/placement", Name: "Destination", Receiver: fact, ResultIndex: 0}}},
		Reducers:    []definition.Reducer{{Name: "Store", Key: "placement/reducer/store", Inputs: []definition.ReducerInput{{Axis: axis("placement"), Carrier: "Fact", Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne}}, Outputs: []definition.ReducerOutput{{Axis: axis("placement"), Carrier: "Fact"}}, Implementation: definition.GoSymbol{PackagePath: "example/placement", Name: "Store", ResultIndex: 0}}},
	}
}

func selfProviderDefinition() definition.Definition {
	owner := definition.GoType{PackagePath: "example/self", Name: "Schema"}
	candidate := definition.GoType{PackagePath: "example/self", Name: "Candidate"}
	key := definition.GoType{PackagePath: "example/self", Name: "Key"}
	fact := definition.GoType{PackagePath: "example/self", Name: "Fact"}
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "self"}
	provider := member.RelationRef{Axis: axis, Member: "self/candidates"}
	method := func(name string, receiver definition.GoType) definition.GoSymbol {
		return definition.GoSymbol{PackagePath: owner.PackagePath, Name: name, Receiver: receiver, ResultIndex: 0}
	}
	return definition.Definition{
		Name: "Self", Axis: "self",
		Binding:   definition.Binding{Key: definition.KeyNormalization{Carrier: "Key", Dense: definition.GoType{Name: "uint32"}, Normalizer: method("KeyIndex", owner)}},
		Signature: definition.Signature{Key: "Key", Fact: "Fact"},
		Carriers:  []definition.Carrier{{Name: "Candidate", Key: "carrier/self/candidate", Type: candidate}, {Name: "Key", Key: "carrier/self/key", Type: key}, {Name: "Fact", Key: "carrier/self/fact", Type: fact}},
		Relations: []definition.Relation{{
			Name: "Candidates", Key: "self/candidates", Subject: "Candidate", CandidateProvider: provider,
			CandidateResolver: method("CandidateForOccurrence", owner), CandidateOrdinal: method("CandidateOrdinal", owner), CandidateAt: method("CandidateAt", owner),
		}},
		Projections: []definition.Projection{{
			Name: "CandidateKey", Key: "self/candidate/key", Relation: "Candidates", Role: member.Key, Result: "Key", CandidateProvider: provider,
			Accessor: method("Key", candidate),
		}},
	}
}

func localDependentDefinition() definition.Definition {
	source := selfProviderDefinition()
	provider := source.Relations[0].CandidateProvider
	fact := definition.GoType{PackagePath: "example/self", Name: "Fact"}
	source.Relations = append(source.Relations, definition.Relation{
		Name: "UsesCandidate", Key: "self/uses-candidate", Subject: "Fact", Inputs: []string{"Candidate", "Fact"},
		CandidateProvider: provider,
	})
	source.Projections = append(source.Projections, definition.Projection{
		Name: "FactKey", Key: "self/fact/key", Relation: "UsesCandidate", Role: member.Key, Result: "Key",
		CandidateProvider: provider, Accessor: definition.GoSymbol{PackagePath: "example/self", Name: "Key", Receiver: fact, ResultIndex: 0},
	})
	return source
}

func TestResolveUsesExplicitCrossAxisCandidateProvider(t *testing.T) {
	metadata, err := Resolve(externalProviderDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if got := metadata.Relations[0].CandidateProvider; got.Axis.Key != "value" || got.Member != "value/storage-transfer/candidates" {
		t.Fatalf("provider=%+v", got)
	}
	if got := metadata.Projections[0].CandidateProvider; got != metadata.Relations[0].CandidateProvider {
		t.Fatalf("projection provider=%+v relation provider=%+v", got, metadata.Relations[0].CandidateProvider)
	}
	if metadata.Relations[0].CandidateProviderLocal || metadata.Projections[0].CandidateProviderLocal {
		t.Fatal("foreign provider was laundered into a local candidate ordinal")
	}
}

func TestResolveUsesExplicitSelfCandidateProvider(t *testing.T) {
	metadata, err := Resolve(selfProviderDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Relations) != 1 || !metadata.Relations[0].CandidateProviderLocal || metadata.Relations[0].CandidateRelation != 0 {
		t.Fatalf("self provider metadata=%+v", metadata.Relations)
	}
	if len(metadata.Projections) != 1 || !metadata.Projections[0].CandidateProviderLocal || metadata.Projections[0].CandidateRelation != 0 {
		t.Fatalf("self projection metadata=%+v", metadata.Projections)
	}
}

func TestResolveRequiresDeclaredProviderCarrierForLocalDependentRelation(t *testing.T) {
	accepted := localDependentDefinition()
	metadata, err := Resolve(accepted)
	if err != nil {
		t.Fatalf("explicit local dependent provider rejected: %v", err)
	}
	if len(metadata.Relations) != 2 || !metadata.Relations[1].CandidateProviderLocal || metadata.Relations[1].CandidateRelation != 0 {
		t.Fatalf("dependent provider metadata=%+v", metadata.Relations)
	}
	accepted.Relations[1].Inputs = []string{"Fact"}
	if _, err := Resolve(accepted); err == nil {
		t.Fatal("dependent relation admitted without the provider subject carrier")
	}
}

func TestRenderForeignProviderDoesNotEmitOwnerDirectoryMirror(t *testing.T) {
	artifact, err := Render("placement", externalProviderDefinition())
	if err != nil {
		t.Fatal(err)
	}
	relations := string(artifact.Relations)
	if strings.Contains(relations, "StorageTransferAt") || strings.Contains(relations, "StorageTransferOrdinal") {
		t.Fatalf("foreign candidate directory leaked into consumer artifact:\n%s", relations)
	}
	if !strings.Contains(relations, "func (owner *RelationOwner) Project") {
		t.Fatalf("consumer bind artifact lost its stable owner surface:\n%s", relations)
	}
}

func TestResolveRejectsProviderDriftAndAbsence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*definition.Definition)
	}{
		{name: "missing", mutate: func(source *definition.Definition) { source.Relations[0].CandidateProvider = member.RelationRef{} }},
		{name: "wrong-provider", mutate: func(source *definition.Definition) {
			source.Projections[0].CandidateProvider.Member = "value/wrong-directory"
		}},
		{name: "foreign-directory-symbols", mutate: func(source *definition.Definition) {
			source.Relations[0].CandidateResolver = definition.GoSymbol{PackagePath: "example/value", Name: "CandidateAt"}
		}},
		{name: "ambiguous-provider-without-member", mutate: func(source *definition.Definition) {
			source.Relations[0].CandidateProvider = member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source := externalProviderDefinition()
			test.mutate(&source)
			if _, err := Resolve(source); err == nil {
				t.Fatal("provider drift admitted")
			}
		})
	}
}

func TestColdRendererUsesStableSchemaAPIAlias(t *testing.T) {
	artifact, err := Render("pack", packmemberdefinition.Source())
	if err != nil {
		t.Fatal(err)
	}
	cold := string(artifact.Cold)
	if !strings.Contains(cold, "schemaapi.Key") || strings.Contains(cold, "schema.Key ") || strings.Contains(cold, "schema.EntryReference") || strings.Contains(cold, "\n\t\"github.com/wippyai/go-lua/analysis/schema\"") {
		t.Fatalf("cold output schema import alias drifted:\n%s", cold)
	}
}

func TestResolveKeepsTypedRowsAlignedWithColdKinds(t *testing.T) {
	metadata, err := Resolve(memberdefinition.StorageTransfer())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Key.Carrier != "carrier/value/coordinate" || metadata.Key.Input.Name != "Coordinate" || metadata.Key.Dense.Name != "uint32" || metadata.Key.Normalizer.Name != "CoordinateIndex" {
		t.Fatalf("key metadata = %#v", metadata.Key)
	}
	if len(metadata.Relations) != 3 || metadata.Relations[0].Key != "value/storage-transfer/candidates" || metadata.Relations[0].Subject.Name != "StorageTransfer" || metadata.Relations[0].CandidateResolver.Name != "StorageTransferForArtifactOccurrence" || metadata.Relations[0].CandidateOrdinal.Name != "StorageTransferOrdinal" || metadata.Relations[0].CandidateAt.Name != "StorageTransferAt" {
		t.Fatalf("relation metadata = %#v", metadata.Relations)
	}
	if len(metadata.Projections) != 3 || metadata.Projections[0].Accessor.ResultIndex != 0 || metadata.Projections[1].Accessor.ResultIndex != 1 || metadata.Projections[0].Result.Name != "Coordinate" {
		t.Fatalf("projection metadata = %#v", metadata.Projections)
	}
	if len(metadata.Reducers) != 2 || len(metadata.Reducers[0].Inputs) != 1 || metadata.Reducers[0].Inputs[0].Type.Name != "Value" || metadata.Reducers[0].Implementation.Name != "IdentityValue" {
		t.Fatalf("reducer metadata = %#v", metadata.Reducers)
	}
	if metadata.Reducers[0].CandidatePresent || metadata.Reducers[0].Candidate.Available() || !metadata.Reducers[0].CandidateConstant {
		t.Fatalf("joined reducer unexpectedly declares a candidate: %#v", metadata.Reducers[0])
	}
	if !metadata.Reducers[1].CandidatePresent || metadata.Reducers[1].CandidateConstant || metadata.Reducers[1].Candidate.Name != "SourceSeed" {
		t.Fatalf("source reducer candidate metadata = %#v", metadata.Reducers[1])
	}
}

func TestResolveKeepsCandidateIndexedCarryTransformTyped(t *testing.T) {
	source := memberdefinition.StorageTransfer().Clone()
	source.CarryTransforms = []definition.CarryTransform{{
		Name:           "StorageTransferTransform",
		Key:            "value/storage-transfer/transform",
		Candidate:      "StorageTransferCarrier",
		Input:          "ValueFactCarrier",
		Output:         "ValueFactCarrier",
		Implementation: definition.GoSymbol{PackagePath: "github.com/wippyai/go-lua/domain/value", Name: "TransformValue", ResultIndex: 0},
	}}
	metadata, err := Resolve(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.CarryTransforms) != 1 {
		t.Fatalf("carry transforms=%d, want 1", len(metadata.CarryTransforms))
	}
	transform := metadata.CarryTransforms[0]
	if transform.Key != "value/storage-transfer/transform" || transform.Candidate.Name != "StorageTransfer" || transform.Input.Name != "Value" || transform.Output.Name != "Value" || transform.Implementation.Name != "TransformValue" {
		t.Fatalf("typed carry transform=%#v", transform)
	}
}

func TestResolveRetainsExactlyTheFourInventoriedOwnerCarryMembers(t *testing.T) {
	valueMetadata, err := Resolve(memberdefinition.StorageTransfer())
	if err != nil {
		t.Fatal(err)
	}
	heapMetadata, err := Resolve(heapmemberdefinition.AllocationCarry())
	if err != nil {
		t.Fatal(err)
	}
	rows := append(append([]CarryTransformBinding(nil), valueMetadata.CarryTransforms...), heapMetadata.CarryTransforms...)
	if len(rows) != 4 {
		t.Fatalf("owner carry transform count=%d, want 4", len(rows))
	}
	want := []struct {
		key, candidate, implementation string
	}{
		{"transform/value/allocation", "AllocationResult", "Age"},
		{"transform/value/callresult-freshresult", "FreshResultCall", "Age"},
		{"transform/heap/allocation-empty", "Root", "Age"},
		{"transform/heap/allocation-closed", "Closed", "Age"},
	}
	seen := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		if string(row.Key) != want[index].key || row.Candidate.Name != want[index].candidate || row.Input.Name != "Value" || row.Output != row.Input || row.Implementation.Name != want[index].implementation || !row.Implementation.Available() {
			t.Fatalf("owner carry transform[%d]=%#v", index, row)
		}
		if _, duplicate := seen[string(row.Key)]; duplicate {
			t.Fatalf("duplicate owner carry transform key %q", row.Key)
		}
		seen[string(row.Key)] = struct{}{}
	}
}

func TestResolveRejectsCarryTransformCarrierOrImplementationDrift(t *testing.T) {
	source := memberdefinition.StorageTransfer().Clone()
	source.CarryTransforms = []definition.CarryTransform{{
		Name:           "StorageTransferTransform",
		Key:            "value/storage-transfer/transform",
		Candidate:      "StorageTransferCarrier",
		Input:          "ValueFactCarrier",
		Output:         "ValueCoordinateCarrier",
		Implementation: definition.GoSymbol{PackagePath: "github.com/wippyai/go-lua/domain/value", Name: "TransformValue", ResultIndex: 0},
	}}
	if _, err := Resolve(source); err == nil {
		t.Fatal("fact carrier mismatch admitted")
	}
	source = memberdefinition.StorageTransfer().Clone()
	source.CarryTransforms = []definition.CarryTransform{{
		Name:      "StorageTransferTransform",
		Key:       "value/storage-transfer/transform",
		Candidate: "StorageTransferCarrier",
		Input:     "ValueFactCarrier",
		Output:    "ValueFactCarrier",
	}}
	if _, err := Resolve(source); err == nil {
		t.Fatal("missing typed transform implementation admitted")
	}
}

func TestResolveRejectsMissingMemberImplementation(t *testing.T) {
	source := memberdefinition.StorageTransfer().Clone()
	source.Projections[0].Accessor = definition.GoSymbol{}
	if _, err := Resolve(source); err == nil {
		t.Fatal("missing projection accessor admitted")
	}

	source = memberdefinition.StorageTransfer().Clone()
	source.Binding.Key.Normalizer = definition.GoSymbol{}
	if _, err := Resolve(source); err == nil {
		t.Fatal("missing key normalizer admitted")
	}

	source = memberdefinition.StorageTransfer().Clone()
	source.Binding.Key.Carrier = "StorageTransferCarrier"
	if _, err := Resolve(source); err == nil {
		t.Fatal("key normalization detached from axis signature admitted")
	}
}

func TestResolveRejectsUnknownReducerCandidateCarrier(t *testing.T) {
	source := memberdefinition.StorageTransfer().Clone()
	source.Reducers[0].Candidate = "MissingCandidateCarrier"
	if _, err := Resolve(source); err == nil {
		t.Fatal("reducer candidate carrier was inferred or admitted")
	}
}

func TestCheckedInGeneratedOutputIsFreshAndCompiles(t *testing.T) {
	root := repositoryRoot(t)
	coldPath := filepath.Join(root, "domain", "value", "rule_members.go")
	relationPath := filepath.Join(root, "domain", "value", "generated_relation_owner.go")
	if err := GenerateAll("value", memberdefinition.StorageTransfer(), coldPath, relationPath, true); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "./domain/value", "-run", "^TestAxisMemberCatalogOwnsStorageTransferGeometry$", "-count=1")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cold package failed to compile: %v\n%s", err, output)
	}

	staticCold := filepath.Join(root, "domain", "static", "rule_members.go")
	staticRelations := filepath.Join(root, "domain", "static", "generated_relation_owner.go")
	if err := GenerateAll("static", staticmemberdefinition.TypeFactTransfer(), staticCold, staticRelations, true); err != nil {
		t.Fatal(err)
	}

	packCold := filepath.Join(root, "domain", "pack", "rule_members.go")
	packRelations := filepath.Join(root, "domain", "pack", "generated_relation_owner.go")
	if err := GenerateAll("pack", packmemberdefinition.Source(), packCold, packRelations, true); err != nil {
		t.Fatal(err)
	}

	heapCold := filepath.Join(root, "domain", "heap", "rule_members.go")
	heapRelations := filepath.Join(root, "domain", "heap", "generated_relation_owner.go")
	if err := GenerateAll("heap", heapmemberdefinition.AllocationCarry(), heapCold, heapRelations, true); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRejectsPartialCandidateDirectory(t *testing.T) {
	source := memberdefinition.StorageTransfer().Clone()
	source.Relations[0].CandidateAt = definition.GoSymbol{}
	if _, err := Resolve(source); err == nil {
		t.Fatal("partial candidate directory admitted")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root not found")
		}
		directory = parent
	}
}
