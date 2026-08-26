package suspension_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	suspension "github.com/wippyai/go-lua/domain/placement/suspension"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestSuspensionBindRejectsEqualSchemaForeignBinding proves that schema and
// heap/value equality are not sufficient authority for a hot suspension rule.
// The two owners below share the exact same cold Schema and concrete domain
// schemas, but their private engine bindings are distinct transactions.
func TestSuspensionBindRejectsEqualSchemaForeignBinding(t *testing.T) {
	placementSchema, values, calls := newSuspensionBindingLawSchemas(t)
	builder := engine.NewSchema()
	placementFragment, placementOK := placementowner.DeclareSchema(builder, suspensionBindingLawSemantic(1), suspensionBindingLawSemantic(2))
	evidenceFragment, evidenceFactorOK := suspension.DeclareEvidenceFactorSchema(builder, suspensionBindingLawSemantic(3), suspensionBindingLawSemantic(4))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, suspensionBindingLawSemantic(5), suspensionBindingLawSemantic(6), suspensionBindingLawSemantic(7))
	callFragment, callOK := callowner.DeclareSchema(builder, suspensionBindingLawSemantic(12))
	fragment, fragmentOK := suspension.DeclareSchema(builder, suspensionBindingLawSemantic(8), suspensionBindingLawSemantic(9), valueFragment, callFragment, placementFragment)
	evidenceRuleFragment, evidenceRuleOK := suspension.DeclareEvidenceSchema(builder, suspensionBindingLawSemantic(10), suspensionBindingLawSemantic(11), valueFragment, callFragment, evidenceFragment)
	cold, coldOK := builder.Seal()
	if !placementOK || !evidenceFactorOK || !valueOK || !callOK || !fragmentOK || !evidenceRuleOK || !coldOK || cold == nil {
		t.Fatalf("suspension binding law declaration placement=%t evidenceFactor=%t value=%t call=%t class=%t evidence=%t cold=%t", placementOK, evidenceFactorOK, valueOK, callOK, fragmentOK, evidenceRuleOK, coldOK)
	}
	localBinding := engine.NewSchemaBinding(cold)
	foreignBinding := engine.NewSchemaBinding(cold)
	localPlacement, localPlacementOK := placementowner.BindHot(localBinding, placementFragment, placementSchema)
	foreignPlacement, foreignPlacementOK := placementowner.BindHot(foreignBinding, placementFragment, placementSchema)
	localEvidence, localEvidenceOK := suspension.BindEvidenceFactorHot(localBinding, evidenceFragment, placementSchema)
	foreignEvidence, foreignEvidenceOK := suspension.BindEvidenceFactorHot(foreignBinding, evidenceFragment, placementSchema)
	localValues, localValuesOK := valueowner.BindHot(localBinding, valueFragment, values)
	foreignValues, foreignValuesOK := valueowner.BindHot(foreignBinding, valueFragment, values)
	localCalls, localCallsOK := callowner.BindHot(localBinding, callFragment, calls)
	foreignCalls, foreignCallsOK := callowner.BindHot(foreignBinding, callFragment, calls)
	if localBinding == nil || foreignBinding == nil || !localPlacementOK || !foreignPlacementOK || !localEvidenceOK || !foreignEvidenceOK || !localValuesOK || !foreignValuesOK || !localCallsOK || !foreignCallsOK || localPlacement == nil || foreignPlacement == nil || localEvidence == nil || foreignEvidence == nil || localValues == nil || foreignValues == nil || localCalls == nil || foreignCalls == nil {
		t.Fatalf("suspension binding law owner setup localPlacement=%t foreignPlacement=%t localEvidence=%t foreignEvidence=%t localValues=%t foreignValues=%t localCalls=%t foreignCalls=%t", localPlacementOK, foreignPlacementOK, localEvidenceOK, foreignEvidenceOK, localValuesOK, foreignValuesOK, localCallsOK, foreignCallsOK)
	}
	classCases := []struct {
		name      string
		placement *placementowner.HotOwner
		values    *valueowner.HotOwner
		calls     *callowner.HotOwner
	}{
		{name: "foreign placement", placement: foreignPlacement, values: localValues, calls: localCalls},
		{name: "foreign value", placement: localPlacement, values: foreignValues, calls: localCalls},
		{name: "foreign call", placement: localPlacement, values: localValues, calls: foreignCalls},
		{name: "both foreign", placement: foreignPlacement, values: foreignValues, calls: foreignCalls},
	}
	for _, item := range classCases {
		if rule, ok := suspension.BindHot(localBinding, fragment, item.placement, item.values, item.calls, values, placementSchema); ok || rule != nil {
			t.Fatalf("class bind accepted %s owner from an equal-schema foreign binding", item.name)
		}
	}
	evidenceCases := []struct {
		name     string
		evidence *suspension.EvidenceOwner
		values   *valueowner.HotOwner
		calls    *callowner.HotOwner
	}{
		{name: "foreign evidence", evidence: foreignEvidence, values: localValues, calls: localCalls},
		{name: "foreign value", evidence: localEvidence, values: foreignValues, calls: localCalls},
		{name: "foreign call", evidence: localEvidence, values: localValues, calls: foreignCalls},
		{name: "both foreign", evidence: foreignEvidence, values: foreignValues, calls: foreignCalls},
	}
	for _, item := range evidenceCases {
		if rule, ok := suspension.BindEvidenceHot(localBinding, evidenceRuleFragment, item.evidence, item.values, item.calls, values, placementSchema); ok || rule != nil {
			t.Fatalf("evidence bind accepted %s owner from an equal-schema foreign binding", item.name)
		}
	}
	classRule, classRuleOK := suspension.BindHot(localBinding, fragment, localPlacement, localValues, localCalls, values, placementSchema)
	evidenceRule, evidenceRuleOK := suspension.BindEvidenceHot(localBinding, evidenceRuleFragment, localEvidence, localValues, localCalls, values, placementSchema)
	sealed := localBinding.Seal()
	if !classRuleOK || classRule == nil || !evidenceRuleOK || evidenceRule == nil || !sealed {
		t.Fatalf("local suspension bind class=%t evidence=%t sealed=%t", classRuleOK, evidenceRuleOK, sealed)
	}
	if catalog := classRule.Catalog(); catalog == nil || catalog.Count() != 0 {
		count := -1
		if catalog != nil {
			count = catalog.Count()
		}
		t.Fatalf("local class suspension catalog = %v count=%d, want empty", catalog, count)
	}
	if catalog := evidenceRule.Catalog(); catalog == nil || catalog.Count() != 0 {
		count := -1
		if catalog != nil {
			count = catalog.Count()
		}
		t.Fatalf("local evidence suspension catalog = %v count=%d, want empty", catalog, count)
	}
}

// TestSuspensionSummaryRefusesAbsentEvidence keeps a sparse/default Factor
// cell from becoming an implicit "no suspension" answer. A non-empty Heap
// summary must receive one authenticated Evidence cell per dense coordinate;
// a delivery that answers no coordinate is a missing predecessor, not a
// result to skip.
func TestSuspensionSummaryRefusesAbsentEvidence(t *testing.T) {
	placementSchema, _, _ := newSuspensionBindingLawSchemas(t)
	if placementSchema.DenseKeyCount() == 0 {
		t.Skip("fixture has no dense Heap coordinates")
	}
	observation := placementdomain.BeginPlacementSummary(placementSchema)
	if _, ok := suspension.AccumulatePlacementSummarySuspensionRows(placementSchema, observation, 0, absentEvidenceAt); ok {
		t.Fatal("suspension summary accepted absent Evidence predecessors")
	}
}

// TestSuspensionSummaryPreservesSparseEvidenceOwnerBottom proves that the
// engine's sparse cell transport is distinct from an unavailable predecessor.
// Placement first projects its owner-issued sparse Stack baseline as a
// complete public allocation row. The exact Evidence owner Bottom then carries
// no verdict and leaves that row's proof column absent; a sparse non-Bottom
// Evidence payload is malformed.
func TestSuspensionSummaryPreservesSparseEvidenceOwnerBottom(t *testing.T) {
	placementSchema, _, _ := newSuspensionAllocationLawSchemas(t)
	denseCount := placementSchema.DenseKeyCount()
	observation := placementdomain.BeginPlacementSummary(placementSchema)
	var placementOK bool
	observation, placementOK = placementdomain.AccumulatePlacementSummaryRows(placementSchema, observation, denseCount,
		func(int) (placementdomain.Fact, bool, bool) { return placementdomain.DefaultFact(), false, true })
	if !placementOK {
		t.Fatal("Placement owner sparse Stack baseline was not authenticated")
	}
	folded, foldedOK := suspension.AccumulatePlacementSummarySuspensionRows(placementSchema, observation, denseCount,
		func(int) (suspension.Evidence, bool, bool) { return suspension.EvidenceMissing, false, true })
	if !foldedOK || !placementdomain.EqualPlacementSummary(placementSchema, folded, observation) {
		t.Fatal("suspension summary did not preserve exact sparse owner Bottom")
	}
	if _, ok := suspension.AccumulatePlacementSummarySuspensionRows(placementSchema, observation, denseCount,
		func(int) (suspension.Evidence, bool, bool) { return suspension.EvidenceUnknown, false, true }); ok {
		t.Fatal("suspension summary accepted sparse non-Bottom evidence")
	}
}

// TestSuspensionSummaryPublishesAuthenticatedUnknown is the publication half
// of the tri-state law. An all-routes verdict of Unknown is this producer's
// own authenticated result: every route was read and none of them decided.
// It must reach the public evidence plane as an explicit Unknown row. Leaving
// it unpublished would make the consumer read the column's absence default and
// so lose the distinction between "no route reported" and "every route
// reported and none decided".
func TestSuspensionSummaryPublishesAuthenticatedUnknown(t *testing.T) {
	placementSchema, _, _ := newSuspensionAllocationLawSchemas(t)
	denseCount := placementSchema.DenseKeyCount()
	if denseCount == 0 {
		t.Fatal("fixture has no dense Heap coordinates")
	}
	allocations := make([]heap.Key, 0, denseCount)
	observation := placementdomain.BeginPlacementSummary(placementSchema)
	for index := 0; index < denseCount; index++ {
		key, keyOK := placementSchema.KeyAt(index)
		if !keyOK {
			t.Fatal("dense Heap coordinate")
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		observation.Values[index] = placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceRefuted}
		observation.Present[index] = true
		observation.Rows = 1
		allocations = append(allocations, key)
	}
	if len(allocations) == 0 {
		t.Fatal("fixture has no allocation roots")
	}

	folded, foldedOK := suspension.AccumulatePlacementSummarySuspensionRows(placementSchema, observation, denseCount,
		func(int) (suspension.Evidence, bool, bool) { return suspension.EvidenceUnknown, true, true })
	if !foldedOK {
		t.Fatal("suspension summary refused an authenticated all-routes Unknown")
	}
	for _, key := range allocations {
		evidence, available := placementdomain.PlacementSummaryEvidence(placementSchema, folded, key)
		if !available {
			t.Fatal("authenticated Unknown suspension evidence was not published")
		}
		if evidence.DiesBeforeSuspension != placementdomain.EvidenceUnknown {
			t.Fatalf("published suspension column = %v, want unknown", evidence.DiesBeforeSuspension)
		}
	}
}

func newSuspensionBindingLawSchemas(t testing.TB) (placementdomain.Schema, *valuedomain.Schema, *calldomain.Algebra) {
	t.Helper()
	return newSuspensionLawSchemasForSource(t, "return 1")
}

// newSuspensionAllocationLawSchemas seals the same fixture over a source whose
// table constructors issue Heap allocation roots. The evidence plane is dense
// over allocations, so a law about published evidence rows needs a program that
// actually allocates.
func newSuspensionAllocationLawSchemas(t testing.TB) (placementdomain.Schema, *valuedomain.Schema, *calldomain.Algebra) {
	t.Helper()
	return newSuspensionLawSchemasForSource(t, "local first = {}; local second = {}; return first")
}

func newSuspensionLawSchemasForSource(t testing.TB, source string) (placementdomain.Schema, *valuedomain.Schema, *calldomain.Algebra) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "placement-suspension-binding-law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "placement-suspension-binding-law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural, structuralOK := composite.StructureVocabulary(receipt)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	calls := calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calls, []programmount.MountedArtifact{valueMount}, structural)
	if !receiptOK || !grammarOK || !issuanceOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !programIDOK || !structuralOK || !mountOK || !valueMountOK || heapFailure != heap.SealFailureNone || !placementOK || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("suspension binding law fixture grammar=%t failure=%v artifact=%v ingress=%t shard=%t module=%t program=%t structural=%t mount=%t valueMount=%t heap=%v placement=%t value=%v", grammarOK, failure, artifact, lowered, shardOK, moduleOK, programIDOK, structuralOK, mountOK, valueMountOK, heapFailure, placementOK, valueFailure)
	}
	return placementSchema, values, calls
}

func suspensionBindingLawSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xE5, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("suspension binding law semantic key")
	}
	return key
}
