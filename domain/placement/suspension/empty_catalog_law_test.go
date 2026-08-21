package suspension_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	suspension "github.com/wippyai/go-lua/domain/placement/suspension"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

var suspensionCatalogFenceBenchmarkSink bool

// TestSuspensionCatalogAdmitsAProgramWithoutSuspensionSubjects proves that a
// scalar-only artifact has a valid empty denominator. The consumer must not
// require a handwritten yield or allocation merely to bind into a Link.
func TestSuspensionCatalogAdmitsAProgramWithoutSuspensionSubjects(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "placement-suspension-empty.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "placement-suspension-empty", Program: program}}})
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
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural, structuralOK := composite.StructureVocabulary(receipt)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := heap.NewArtifactMount(snapshot, module, programID)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []heap.ArtifactMount{mount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
	if !receiptOK || !grammarOK || !issuanceOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !programIDOK || !structuralOK || !mountOK || !valueMountOK || heapFailure != heap.SealFailureNone || !placementOK || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("suspension fixture grammar=%t failure=%v artifact=%v ingress=%t shard=%t module=%t program=%t structural=%t mount=%t valueMount=%t heap=%v placement=%t value=%v", grammarOK, failure, artifact, lowered, shardOK, moduleOK, programIDOK, structuralOK, mountOK, valueMountOK, heapFailure, placementOK, valueFailure)
	}
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heap.RootAllocation {
			t.Fatalf("scalar-only fixture unexpectedly contains allocation root at %d", index)
		}
	}
	catalog, catalogOK := suspension.SealCatalog(placementSchema, values)
	if !catalogOK || catalog == nil || !catalog.FencedTo(placementSchema, values) || catalog.Count() != 0 {
		t.Fatalf("empty suspension catalog = %#v/%t count=%d", catalog, catalogOK, catalog.Count())
	}
	if _, idOK := catalog.IDAt(0); idOK {
		t.Fatal("empty suspension catalog issued an operand identity")
	}
	if legacy, legacyOK := suspension.SealCatalog(placementSchema, nil); legacyOK || legacy != nil {
		t.Fatal("suspension catalog admitted the removed direct/optional-Value compatibility lane")
	}
	if catalog.FencedTo(placementSchema, nil) {
		t.Fatal("suspension catalog accepted an unowned nil Value fence")
	}
	foreign := *values
	if catalog.FencedTo(placementSchema, &foreign) {
		t.Fatal("suspension catalog accepted a copied Value schema instead of its owner fence")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		suspensionCatalogFenceBenchmarkSink = catalog.FencedTo(placementSchema, values) && catalog.Count() == 0
	}); allocations != 0 {
		t.Fatalf("suspension catalog hot fence allocations = %v, want zero", allocations)
	}
}
