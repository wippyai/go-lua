package generator

import (
	"github.com/wippyai/go-lua/domain/memberroster"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// composedSource resolves one axis's member definition the way the generator
// command does: from the composition roster, as the sealed fold of the axis
// base and the reducer contributions its rules declare. A law that reached for
// a base directly would be testing half a definition.
func composedSource(t *testing.T, name string) definition.Definition {
	t.Helper()
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	_, composed, composedOK := roster.Definition(name)
	if !composedOK {
		t.Fatalf("member definition source %q does not compose", name)
	}
	return composed
}

func externalProviderDefinition() definition.Definition {
	owner := definition.GoType{PackagePath: "example/placement", Name: "Schema"}
	candidate := definition.GoType{PackagePath: "example/value", Name: "StorageTransfer"}
	fact := definition.GoType{PackagePath: "example/placement", Name: "Fact"}
	key := definition.GoType{PackagePath: "example/placement", Name: "Key"}
	axis := func(name string) schema.EntryReference {
		return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(name)}
	}
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: axis("value"), Member: "value/storage-transfer/candidates"})
	return definition.Definition{
		Name: "PlacementStore", Axis: "placement",
		Binding:     definition.Binding{Key: definition.KeyNormalization{Carrier: "Key", Dense: definition.GoType{Name: "uint32"}, Normalizer: definition.GoSymbol{PackagePath: "example/placement", Name: "Normalize", Receiver: owner, ResultIndex: 0}}},
		Signature:   definition.Signature{Key: "Key", Fact: "Fact"},
		Carriers:    []definition.Carrier{{Name: "Candidate", Key: "carrier/value/storage-transfer", Type: candidate}, {Name: "Fact", Key: "carrier/placement/fact", Type: fact}, {Name: "Key", Key: "carrier/placement/key", Type: key}},
		Relations:   []definition.Relation{{Name: "Route", Key: "placement/store/route", Subject: "Fact", Inputs: []definition.RelationInput{{Carrier: "Candidate"}, {Carrier: "Fact"}}, CandidateProvider: provider}},
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
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: axis, Member: "self/candidates"})
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
		Name: "UsesCandidate", Key: "self/uses-candidate", Subject: "Fact", Inputs: []definition.RelationInput{{Carrier: "Candidate"}, {Carrier: "Fact"}},
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
	if got := metadata.Relations[0].CandidateProvider; got.AxisRelation.Axis.Key != "value" || got.AxisRelation.Member != "value/storage-transfer/candidates" {
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
	accepted.Relations[1].Inputs = []definition.RelationInput{{Carrier: "Fact"}}
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
		{name: "missing", mutate: func(source *definition.Definition) { source.Relations[0].CandidateProvider = member.CandidateRef{} }},
		{name: "wrong-provider", mutate: func(source *definition.Definition) {
			source.Projections[0].CandidateProvider.AxisRelation.Member = "value/wrong-directory"
		}},
		{name: "foreign-directory-symbols", mutate: func(source *definition.Definition) {
			source.Relations[0].CandidateResolver = definition.GoSymbol{PackagePath: "example/value", Name: "CandidateAt"}
		}},
		{name: "ambiguous-provider-without-member", mutate: func(source *definition.Definition) {
			source.Relations[0].CandidateProvider = member.AxisRelationCandidate(member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}})
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
	artifact, err := Render("pack", composedSource(t, "pack"))
	if err != nil {
		t.Fatal(err)
	}
	cold := string(artifact.Cold)
	if !strings.Contains(cold, "schemaapi.Key") || strings.Contains(cold, "schema.Key ") || strings.Contains(cold, "schema.EntryReference") || strings.Contains(cold, "\n\t\"github.com/wippyai/go-lua/analysis/schema\"") {
		t.Fatalf("cold output schema import alias drifted:\n%s", cold)
	}
}

func TestResolveKeepsTypedRowsAlignedWithColdKinds(t *testing.T) {
	metadata, err := Resolve(composedSource(t, "value"))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Key.Carrier != "carrier/value/coordinate" || metadata.Key.Input.Name != "Coordinate" || metadata.Key.Dense.Name != "uint32" || metadata.Key.Normalizer.Name != "CoordinateIndex" {
		t.Fatalf("key metadata = %#v", metadata.Key)
	}
	// The inventory is pinned by ordered key, not by a bare count: a row added
	// to the value axis has to name itself here, and a row that moves is a
	// different ordinal in every generated ladder.
	wantRelations := []schema.Key{
		"value/storage-transfer/candidates",
		"value/binary-arithmetic/candidates",
		"value/binary-equality/candidates",
		"value/binary-order/candidates",
		"value/presence-refinement/candidates",
		"value/storage-transfer/sources",
		"value/source/candidates",
		"value/global-bootstrap/candidates",
		"value/mounted-call/argument-candidates",
		"value/mounted-call/arguments",
		// The two allocation-form candidate directories. They are appended, not
		// interleaved, because a relation that moves is a different ordinal in
		// every generated ladder.
		"value/allocation/candidates",
		"value/fresh-result/candidates",
		// The return-boundary candidate directory, the root join that hangs off
		// it, and the nested ordered member set it parents. Appended for the
		// same reason.
		"value/return-boundary/candidates",
		"value/return-boundary/roots",
		"value/return-boundary/members",
		// The mounted-call actuals parent and the nested ordered member set it
		// parents. The mounted-call owner states them on the candidate
		// directories, ahead of the fact relations that consume them.
		"value/mounted-call/parents",
		"value/mounted-call/actual-members",
		"value/binary-arithmetic/sources",
		"value/binary-equality/sources",
		"value/presence-refinement/sources",
		"value/binary-order/sources",
	}
	if len(metadata.Relations) != len(wantRelations) {
		t.Fatalf("relation inventory = %d, want %d", len(metadata.Relations), len(wantRelations))
	}
	for index, key := range wantRelations {
		if metadata.Relations[index].Key != key {
			t.Fatalf("relation %d = %q, want %q", index, metadata.Relations[index].Key, key)
		}
	}
	if metadata.Relations[0].Subject.Name != "StorageTransfer" || metadata.Relations[0].CandidateResolver.Name != "StorageTransferForArtifactOccurrence" || metadata.Relations[0].CandidateOrdinal.Name != "StorageTransferOrdinal" || metadata.Relations[0].CandidateAt.Name != "StorageTransferAt" {
		t.Fatalf("relation metadata = %#v", metadata.Relations[0])
	}
	wantProjections := []schema.Key{
		"value/storage-transfer/source-key",
		"value/storage-transfer/target",
		"value/source/coordinate",
		"value/global-bootstrap/coordinate",
		"value/mounted-call/argument-key",
		"value/allocation/coordinate",
		"value/fresh-result/coordinate",
		"value/return-boundary/root-key",
		"value/return-boundary/member-key",
		"value/mounted-call/callee-key",
		"value/mounted-call/actual-key",
		"value/mounted-call/actual-tag",
		"value/binary-arithmetic/left",
		"value/binary-arithmetic/right",
		"value/binary-arithmetic/write",
		"value/binary-equality/left",
		"value/binary-equality/right",
		"value/binary-equality/write",
		"value/presence-refinement/source-key",
		"value/presence-refinement/write",
		"value/binary-order/left",
		"value/binary-order/right",
		"value/binary-order/write",
	}
	if len(metadata.Projections) != len(wantProjections) {
		t.Fatalf("projection inventory = %d, want %d", len(metadata.Projections), len(wantProjections))
	}
	for index, key := range wantProjections {
		if metadata.Projections[index].Key != key {
			t.Fatalf("projection %d = %q, want %q", index, metadata.Projections[index].Key, key)
		}
	}
	// The three accessor arities the emitter binds: result 0 of a pair, result
	// 1 of a pair, and the sole result of an unpaired accessor.
	if metadata.Projections[0].Accessor.ResultIndex != 0 || metadata.Projections[1].Accessor.ResultIndex != 1 || metadata.Projections[4].Accessor.ResultIndex != -1 || metadata.Projections[0].Result.Name != "Coordinate" {
		t.Fatalf("projection metadata = %#v", metadata.Projections)
	}
	wantReducers := []schema.Key{
		"value/reducer/identity",
		"value/reducer/source",
		"value/reducer/global-bootstrap",
		"value/binary-arithmetic/reducer",
		"value/binary-equality/reducer",
		"value/presence-refinement/reducer",
		"value/binary-order/reducer",
	}
	if len(metadata.Reducers) != len(wantReducers) {
		t.Fatalf("reducer inventory = %d, want %d", len(metadata.Reducers), len(wantReducers))
	}
	for index, key := range wantReducers {
		if metadata.Reducers[index].Key != key {
			t.Fatalf("reducer %d = %q, want %q", index, metadata.Reducers[index].Key, key)
		}
	}
	if len(metadata.Reducers[0].Inputs) != 1 || metadata.Reducers[0].Inputs[0].Type.Name != "Value" || metadata.Reducers[0].Implementation.Name != "IdentityValue" {
		t.Fatalf("reducer metadata = %#v", metadata.Reducers[0])
	}
	if metadata.Reducers[0].CandidatePresent || metadata.Reducers[0].Candidate.Available() || !metadata.Reducers[0].CandidateConstant {
		t.Fatalf("joined reducer unexpectedly declares a candidate: %#v", metadata.Reducers[0])
	}
	if !metadata.Reducers[1].CandidatePresent || metadata.Reducers[1].CandidateConstant || metadata.Reducers[1].Candidate.Name != "SourceSeed" {
		t.Fatalf("source reducer candidate metadata = %#v", metadata.Reducers[1])
	}
	arithmetic := metadata.Reducers[3]
	if !arithmetic.CandidatePresent || arithmetic.Candidate.Name != "BinaryArithmetic" || len(arithmetic.Inputs) != 2 || arithmetic.Inputs[0].Type.Name != "Value" || arithmetic.Inputs[1].Type.Name != "Value" || arithmetic.Implementation.Name != "ArithmeticValue" {
		t.Fatalf("arithmetic reducer metadata = %#v", arithmetic)
	}
}

// TestSoleResultProjectionAccessorEmitsOneBoundName states the accessor-arity
// law: an owner that publishes exactly one projection and no fact beside it
// declares result -1, and the emitted direct call binds one name plus its
// validity. Without it every such owner has to add a paired accessor that
// returns a second value it does not have, which is a wrapper written around
// the generator rather than a member declaration.
func TestSoleResultProjectionAccessorEmitsOneBoundName(t *testing.T) {
	artifact, err := Render("call", composedSource(t, "call"))
	if err != nil {
		t.Fatal(err)
	}
	relations := string(artifact.Relations)
	if !strings.Contains(relations, "first, projectionOK := candidate.Key()") {
		t.Fatalf("sole-result accessor was not emitted as one bound name:\n%s", relations)
	}
	if strings.Contains(relations, "first, second, projectionOK := candidate.Key()") || strings.Contains(relations, "_ = second") {
		t.Fatalf("sole-result accessor still binds a discarded second result:\n%s", relations)
	}
	paired, err := Render("heap", composedSource(t, "heap"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(paired.Relations), "_ = second") {
		t.Fatalf("a paired accessor stopped discarding its unused result:\n%s", string(paired.Relations))
	}
}

// TestResolveRefusesAProjectionResultTheCallCannotBind states the other half:
// -1, 0 and 1 are the whole accessor-arity vocabulary, and any other index
// names a result the emitted call has no name for.
func TestResolveRefusesAProjectionResultTheCallCannotBind(t *testing.T) {
	for _, index := range []int8{2, 3, 127} {
		source := composedSource(t, "value").Clone()
		source.Projections[0].Accessor.ResultIndex = index
		if _, err := Resolve(source); err == nil {
			t.Fatalf("projection accessor result %d admitted", index)
		}
	}
}

func TestResolveKeepsCandidateIndexedCarryTransformTyped(t *testing.T) {
	source := composedSource(t, "value").Clone()
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

// TestResolveRetainsExactlyTheFourInventoriedOwnerCarryMembers pins the whole
// owner carry inventory: four members, each carrying one fact type through one
// owner-issued transition named on the candidate the rule draws from its
// relation.
//
// The two axes satisfy that one law differently, and the difference is which
// package declares the descriptor. Heap's constructor descriptors live beside
// the fold in a package the heap axis cannot import, so no heap relation could
// ever publish rows of them; heap therefore carries on the allocation
// coordinate, which its candidate directories already subject. Value declares
// AllocationResult and FreshResultCall itself, so it publishes those two
// directories and carries on the receipts directly. Either way the candidate
// is the subject of a relation of its own axis, which is what the plan's carry
// admission binds against.
func TestResolveRetainsExactlyTheFourInventoriedOwnerCarryMembers(t *testing.T) {
	valueMetadata, err := Resolve(composedSource(t, "value"))
	if err != nil {
		t.Fatal(err)
	}
	heapMetadata, err := Resolve(composedSource(t, "heap"))
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
		{"transform/heap/allocation-empty", "Key", "Age"},
		{"transform/heap/allocation-closed", "Key", "Age"},
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
	source := composedSource(t, "value").Clone()
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
	source = composedSource(t, "value").Clone()
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
	source := composedSource(t, "value").Clone()
	source.Projections[0].Accessor = definition.GoSymbol{}
	if _, err := Resolve(source); err == nil {
		t.Fatal("missing projection accessor admitted")
	}

	source = composedSource(t, "value").Clone()
	source.Binding.Key.Normalizer = definition.GoSymbol{}
	if _, err := Resolve(source); err == nil {
		t.Fatal("missing key normalizer admitted")
	}

	source = composedSource(t, "value").Clone()
	source.Binding.Key.Carrier = "StorageTransferCarrier"
	if _, err := Resolve(source); err == nil {
		t.Fatal("key normalization detached from axis signature admitted")
	}
}

func TestResolveRejectsUnknownReducerCandidateCarrier(t *testing.T) {
	source := composedSource(t, "value").Clone()
	source.Reducers[0].Candidate = "MissingCandidateCarrier"
	if _, err := Resolve(source); err == nil {
		t.Fatal("reducer candidate carrier was inferred or admitted")
	}
}

// TestCheckedInGeneratedOutputIsFreshAndCompiles is the freshness law over
// every registered axis, not a hand-picked subset of them: it walks the
// roster generically so a new axis is checked the moment it registers,
// rather than by remembering to add it here. Before this walked the roster,
// domain/call's checked-in rule_members.go/generated_relation_owner.go
// carried a MountedCallFacts relation and MountedCallFactKey projection its
// own authored domain/call/memberdefinition/source.go did not declare -
// folded in instead from a foreign contribution in
// domain/heap/formalfreeze/memberdefinition - and this law never ran for
// "call" at all, so the divergence between generated and authored was
// invisible to it (c6ee4bc2d7 moved the relation to Call's own source; this
// law is what should have caught the split before that).
func TestCheckedInGeneratedOutputIsFreshAndCompiles(t *testing.T) {
	root := repositoryRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		t.Run(source.Name, func(t *testing.T) {
			composed := composedSource(t, source.Name)
			coldPath := filepath.Join(root, "domain", source.Package, "rule_members.go")
			if err := Generate(source.Package, composed, coldPath, true); err != nil {
				t.Fatal(err)
			}

			relationsPackage := composed.RelationsPackage
			if relationsPackage == "" {
				relationsPackage = source.Package
			}
			relationsRelative := composed.RelationsPath
			if relationsRelative == "" {
				relationsRelative = "generated_relation_owner.go"
			}
			relationPath := filepath.Join(root, "domain", source.Package, relationsRelative)
			if err := GenerateRelations(relationsPackage, composed, relationPath, true); err != nil {
				t.Fatal(err)
			}
		})
	}

	command := exec.Command("go", "test", "./domain/value", "-run", "^TestAxisMemberCatalogOwnsStorageTransferGeometry$", "-count=1")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cold package failed to compile: %v\n%s", err, output)
	}
}

// TestCheckedInExactFoldTablesAreFresh holds every axis's exact-fold dispatch
// to the catalog it is derived from, by walking the roster.
//
// An exact-fold table captures MEMBER ordinals - the read relations, read keys
// and destination projections of each reducer it dispatches. Those are
// positions in the axis's sealed catalog, so a catalog that gains a row
// renumbers them and every captured ordinal in a table emitted before that
// addresses a catalog which no longer exists. The installer that holds such a
// table then refuses every rule it authors, and the refusal surfaces far from
// the file that caused it.
//
// The check was previously written for one axis by name, so a second axis
// declaring an exact fold - or a first one whose catalog moved - was not held
// to anything. It is a property of every rostered source, and an axis that
// declares no exact fold must emit no table rather than an empty one nothing
// checks.
func TestCheckedInExactFoldTablesAreFresh(t *testing.T) {
	root := repositoryRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	checked := 0
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		t.Run(source.Name, func(t *testing.T) {
			composed := composedSource(t, source.Name)
			metadata, err := Resolve(composed)
			if err != nil {
				t.Fatal(err)
			}
			reducers, err := exactFoldReducers(composed, metadata)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "domain", source.Package, "generated_exact_fold.go")
			if len(reducers) == 0 {
				if _, err := os.Stat(path); err == nil {
					t.Fatalf("%s declares no exact fold yet an emitted table is checked in at %s", source.Name, path)
				}
				return
			}
			checked++
			if err := GenerateExactFold(source.Package, composed, path, true); err != nil {
				t.Fatal(err)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no rostered axis declares an exact fold: this law proves nothing")
	}
}

// TestAStaleExactFoldTableIsRefused proves the freshness check itself sees the
// drift that actually shipped: one member ordinal moved, everything else
// identical. A check that compared anything coarser than the emitted bytes
// would pass a table whose every reducer row addresses the wrong rows.
func TestAStaleExactFoldTableIsRefused(t *testing.T) {
	composed := composedSource(t, "value")
	artifact, err := Render("value", composed)
	if err != nil {
		t.Fatal(err)
	}
	fresh := string(artifact.ExactFold)
	marker := "ReadRelationMember: [ExactFoldArity]uint32{"
	position := strings.Index(fresh, marker)
	if position < 0 {
		t.Fatalf("the emitted table declares no read relation member: %q", firstLine(fresh))
	}
	digit := position + len(marker)
	end := digit
	for end < len(fresh) && fresh[end] >= '0' && fresh[end] <= '9' {
		end++
	}
	if end == digit {
		t.Fatal("the emitted table's first read relation member is not a number")
	}
	stale := fresh[:digit] + "999" + fresh[end:]
	path := filepath.Join(t.TempDir(), "generated_exact_fold.go")
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateExactFold("value", composed, path, true); err == nil {
		t.Fatal("a table naming a member ordinal the catalog does not have passed the freshness check")
	}
	if err := os.WriteFile(path, []byte(fresh), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateExactFold("value", composed, path, true); err != nil {
		t.Fatalf("the freshly emitted table was refused: %v", err)
	}
}

func firstLine(text string) string {
	if index := strings.Index(text, "\n"); index >= 0 {
		return text[:index]
	}
	return text
}

func TestGeneratorCommandWritesEveryRequestedOutput(t *testing.T) {
	root := repositoryRoot(t)
	directory := t.TempDir()
	coldPath := filepath.Join(directory, "cold.go")
	relationsPath := filepath.Join(directory, "relations.go")
	exactFoldPath := filepath.Join(directory, "exact_fold.go")
	command := exec.Command("go", "run", "./analysis/schema/axis/member/generator/cmd",
		"-source", "value", "-cold", coldPath, "-relations", relationsPath, "-exact-fold", exactFoldPath)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generator command failed: %v\n%s", err, output)
	}
	for _, path := range []string{coldPath, relationsPath, exactFoldPath} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("generator did not write %s: %v", path, err)
		}
		if len(contents) == 0 {
			t.Fatalf("generator wrote empty output %s", path)
		}
	}
}

func TestResolveRejectsPartialCandidateDirectory(t *testing.T) {
	source := composedSource(t, "value").Clone()
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
