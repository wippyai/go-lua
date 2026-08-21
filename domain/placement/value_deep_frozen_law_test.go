package placement_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	targetcompiler "github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	typecontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestDecodeSummaryResultRequiresExpectedSchemaIdentity keeps the publication
// boundary strict in both directions: an unavailable expected authority is
// never allowed to open a result, and a valid authority rejects a payload
// whose self-described schema identity is not the exact expected identity.
// The payload comes from the existing DeepFrozen fixture; this law does not
// construct another composite schema merely to exercise the rejection path.
func TestDecodeSummaryResultRequiresExpectedSchemaIdentity(t *testing.T) {
	fixture := newDeepFrozenValueFixture(t)
	foreign := newDeepFrozenValueFixture(t)
	present, rows, payload := fixture.encodedSummary(t, nil)
	encoded := string(payload)
	if result, ok := placementdomain.DecodeSummaryResult(fixture.placement, present, rows, encoded); !ok || !result.Available() {
		t.Fatal("valid expected Placement schema rejected its own result")
	}
	foreignPayload := append([]byte(nil), payload...)
	if foreign.placement.ContentID() == fixture.placement.ContentID() {
		// The existing fixture is intentionally deterministic. Equal-content
		// schemas are not a wrong authority identity, so make the transport
		// identity wrong while still exercising the foreign expected authority.
		foreignPayload[8] ^= 0x01
	}
	if result, ok := placementdomain.DecodeSummaryResult(foreign.placement, present, rows, string(foreignPayload)); ok || result.Available() {
		t.Fatal("foreign expected Placement schema opened a result owned by another schema")
	}
	if result, ok := placementdomain.DecodeSummaryResult(placementdomain.Schema{}, present, rows, encoded); ok || result.Available() {
		t.Fatal("unavailable expected Placement schema opened a result")
	}

	wrongIdentity := append([]byte(nil), payload...)
	wrongIdentity[8] ^= 0x01
	if !placementResultHeaderIdentityAvailable(wrongIdentity) {
		t.Fatal("wrong-schema fixture accidentally became unavailable")
	}
	if result, ok := placementdomain.DecodeSummaryResult(fixture.placement, present, rows, string(wrongIdentity)); ok || result.Available() {
		t.Fatal("expected Placement schema opened a result carrying another schema identity")
	}
}

func placementResultHeaderIdentityAvailable(payload []byte) bool {
	if len(payload) < 40 {
		return false
	}
	for _, value := range payload[8:40] {
		if value != 0 {
			return true
		}
	}
	return false
}

// TestDeepFrozenValueTreatsScalarAndBottomAsVacuouslyProven pins the
// consumer boundary: scalar alternatives have no mutable Heap graph, and
// Bottom contains no concrete alternative at all. Neither case needs an
// allocation receipt to claim transitive immutability.
func TestDeepFrozenValueTreatsScalarAndBottomAsVacuouslyProven(t *testing.T) {
	fixture := newDeepFrozenValueFixture(t)
	summary := fixture.summary(t, map[heapdomain.Key]placementdomain.EvidenceState{})

	if got := placementdomain.DeepFrozenValue(fixture.placement, fixture.values, summary, fixture.values.Bottom()); got != placementdomain.EvidenceProven {
		t.Fatalf("Bottom deep-frozen state = %v, want proven", got)
	}
	atom, atomOK := fixture.values.OpaqueKind(runtimekind.Number)
	if !atomOK {
		t.Fatal("numeric scalar atom")
	}
	scalar, scalarOK := fixture.values.Singleton(atom)
	if !scalarOK {
		t.Fatal("numeric scalar value")
	}
	if got := placementdomain.DeepFrozenValue(fixture.placement, fixture.values, summary, scalar); got != placementdomain.EvidenceProven {
		t.Fatalf("scalar deep-frozen state = %v, want proven", got)
	}
}

// TestDeepFrozenValueRequiresTransitiveProofForRecentAndSummary proves that
// both live materialization ages are valid publication alternatives when the
// owner-issued Placement evidence proves the allocation graph frozen.
func TestDeepFrozenValueRequiresTransitiveProofForRecentAndSummary(t *testing.T) {
	fixture := newDeepFrozenValueFixture(t)
	key := fixture.allocations[0]
	summary := fixture.summary(t, map[heapdomain.Key]placementdomain.EvidenceState{key: placementdomain.EvidenceProven})
	for _, role := range []materialization.Role{materialization.Recent, materialization.Summary} {
		atom, atomOK := fixture.values.Allocation(key, role)
		if !atomOK {
			t.Fatalf("allocation atom role %v", role)
		}
		fact, factOK := fixture.values.Singleton(atom)
		if !factOK {
			t.Fatalf("allocation value role %v", role)
		}
		if got := placementdomain.DeepFrozenValue(fixture.placement, fixture.values, summary, fact); got != placementdomain.EvidenceProven {
			t.Fatalf("allocation role %v deep-frozen state = %v, want proven", role, got)
		}
	}
}

// TestDeepFrozenValueDoesNotTreatExactAsAnAllocationProof keeps structural
// Exact references conservative. Exact has not selected a materialized Heap
// version, so even a Proven Recent/Summary row cannot certify it.
func TestDeepFrozenValueDoesNotTreatExactAsAnAllocationProof(t *testing.T) {
	fixture := newDeepFrozenValueFixture(t)
	key := fixture.allocations[0]
	summary := fixture.summary(t, map[heapdomain.Key]placementdomain.EvidenceState{key: placementdomain.EvidenceProven})
	atom, atomOK := fixture.values.Allocation(key, materialization.Exact)
	if !atomOK {
		t.Fatal("exact allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("exact allocation value")
	}
	if got := placementdomain.DeepFrozenValue(fixture.placement, fixture.values, summary, fact); got != placementdomain.EvidenceUnknown {
		t.Fatalf("exact allocation deep-frozen state = %v, want unknown", got)
	}
}

// TestDeepFrozenValueMutableWitnessDominatesUnknown proves the three-valued
// consumer join: one exact Refuted allocation alternative is a sound negative
// witness even when another possible alternative has no proof at all.
func TestDeepFrozenValueMutableWitnessDominatesUnknown(t *testing.T) {
	fixture := newDeepFrozenValueFixture(t)
	if len(fixture.allocations) < 2 {
		t.Skip("fixture has fewer than two allocation roots")
	}
	first, firstOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	second, secondOK := fixture.values.Allocation(fixture.allocations[1], materialization.Recent)
	if !firstOK || !secondOK {
		t.Fatal("allocation alternatives")
	}
	fact, factOK := fixture.values.Alternatives(first, second)
	if !factOK {
		t.Fatal("mixed allocation value")
	}
	summary := fixture.summary(t, map[heapdomain.Key]placementdomain.EvidenceState{
		fixture.allocations[0]: placementdomain.EvidenceRefuted,
		fixture.allocations[1]: placementdomain.EvidenceUnknown,
	})
	if got := placementdomain.DeepFrozenValue(fixture.placement, fixture.values, summary, fact); got != placementdomain.EvidenceRefuted {
		t.Fatalf("mixed mutable/unknown deep-frozen state = %v, want refuted", got)
	}
}

// TestDeepFrozenValueRejectsForeignHeapAuthority proves that a foreign Value
// schema cannot borrow an equal-content Placement summary. The Heap schema
// pointer is the authority fence, not a replayable ContentID.
func TestDeepFrozenValueRejectsForeignHeapAuthority(t *testing.T) {
	local := newDeepFrozenValueFixture(t)
	foreign := newDeepFrozenValueFixture(t)
	key := foreign.allocations[0]
	atom, atomOK := foreign.values.Allocation(key, materialization.Recent)
	if !atomOK {
		t.Fatal("foreign allocation atom")
	}
	fact, factOK := foreign.values.Singleton(atom)
	if !factOK {
		t.Fatal("foreign allocation value")
	}
	foreignSummary := foreign.summary(t, map[heapdomain.Key]placementdomain.EvidenceState{key: placementdomain.EvidenceProven})
	if got := placementdomain.DeepFrozenValue(local.placement, foreign.values, foreignSummary, fact); got != placementdomain.EvidenceUnknown {
		t.Fatalf("foreign authority deep-frozen state = %v, want unknown", got)
	}
}

type deepFrozenValueFixture struct {
	placement   placementdomain.Schema
	values      *valuedomain.Schema
	allocations []heapdomain.Key
}

func newDeepFrozenValueFixture(t testing.TB) deepFrozenValueFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{
		Name: "placement-value-deep-frozen.lua",
		Text: []byte("local first = {}; local second = {}; return first"),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := targetcompiler.Seal(&declaration.Spec{Semantics: typecontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "placement-value-deep-frozen", Program: program}},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Build()
	if !receiptOK {
		t.Fatal("global schema receipt")
	}
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !grammarOK || !issuanceOK {
		t.Fatal("artifact compiler grammar")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %v", failure)
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mounted program identity")
	}
	snapshot := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshot, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	structural, structuralOK := composite.StructureVocabulary(receipt)
	if !heapMountOK || !valueMountOK || !structuralOK {
		t.Fatal("artifact mount or structural vocabulary")
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("schema seal heap=%v value=%v", heapFailure, valueFailure)
	}
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	if !placementOK {
		t.Fatal("Placement schema")
	}
	allocations := make([]heapdomain.Key, 0, heapSchema.KeyCount())
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			allocations = append(allocations, key)
		}
	}
	if len(allocations) == 0 {
		t.Fatal("allocation roots")
	}
	return deepFrozenValueFixture{placement: placementSchema, values: values, allocations: allocations}
}

func (fixture deepFrozenValueFixture) summary(t testing.TB, states map[heapdomain.Key]placementdomain.EvidenceState) placementdomain.SummaryResult {
	present, rows, payload := fixture.encodedSummary(t, states)
	result, decodedOK := placementdomain.DecodeSummaryResult(fixture.placement, present, rows, string(payload))
	if !decodedOK || !result.Available() {
		t.Fatal("decode Placement summary")
	}
	return result
}

func (fixture deepFrozenValueFixture) encodedSummary(t testing.TB, states map[heapdomain.Key]placementdomain.EvidenceState) (bool, uint64, []byte) {
	t.Helper()
	observation := placementdomain.BeginPlacementSummary(fixture.placement)
	for _, key := range fixture.allocations {
		index, indexOK := fixture.placement.Heap().KeyIndex(key)
		id, idOK := key.ContentID()
		if !indexOK || !idOK {
			t.Fatal("allocation summary coordinate")
		}
		observation.Values[index] = placementdomain.OwnedHeap
		observation.Present[index] = true
		observation.Rows = 1
		evidence := placementdomain.AllocationEvidence{OwnerIdentity: id, HasOwnerIdentity: true, DeepFrozen: states[key]}
		updated, updatedOK := placementdomain.WithPlacementSummaryEvidence(fixture.placement, observation, key, evidence)
		if !updatedOK {
			t.Fatal("owner-issued DeepFrozen evidence")
		}
		observation = updated
	}
	present, rows, payload, encodedOK := placementdomain.EncodeSummaryResult(observation)
	if !encodedOK {
		t.Fatal("encode Placement summary")
	}
	return present, rows, payload
}
