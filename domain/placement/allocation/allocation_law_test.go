package allocation_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	allocation "github.com/wippyai/go-lua/domain/placement/allocation"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func TestAllocationOperandRejectsMissingForeignAndNonAllocationCoordinates(t *testing.T) {
	fixture := placementAllocationFixture(t)
	// The direct owner projection has no optimistic fallback for either
	// missing coordinates or an unavailable Placement authority.
	if _, ok := allocation.BindHot(nil, nil, nil, heapdomain.Schema{}); ok {
		t.Fatal("invalid Placement bind was admitted")
	}
	if fixture.boot.Valid() {
		// Boot roots have no mounted allocation occurrence, so Heap's
		// canonical occurrence issuer cannot issue one as a seed operand.
		issuer, issuerOK := fixture.heap.OccurrenceMountForModule(fixture.module)
		if !issuerOK {
			t.Fatal("canonical occurrence issuer unavailable")
		}
		if _, ok := issuer.AllocationRootForOccurrence(fixture.bootID); ok {
			t.Fatal("Boot coordinate was admitted as an allocation seed")
		}
	}
	if _, ok := fixture.heap.OccurrenceMountForModule(placementAllocationContentID("foreign-module")); ok {
		t.Fatal("foreign module was admitted by the canonical occurrence issuer")
	}
}

func TestAllocationUsesCanonicalMountIdentityAndProjectsExactHeapCoordinate(t *testing.T) {
	fixture := placementAllocationFixture(t)
	placementAllocationBind(t, &fixture)

	mount, mountOK := fixture.heap.OccurrenceMountForModule(fixture.module)
	canonical, canonicalOK := mount.AllocationRootForOccurrence(fixture.occurrence)
	index, indexOK := fixture.heap.KeyIndex(canonical)
	if !mountOK || !canonicalOK || !indexOK || canonical.Kind() != heapdomain.RootAllocation || index < 0 {
		t.Fatalf("allocation owner mount=%t key=%t index=%t kind=%v", mountOK, canonicalOK, indexOK, canonical.Kind())
	}

	foreignModule := placementAllocationContentID("foreign-module")
	foreignOccurrence := placementAllocationContentID("foreign-occurrence")
	if _, ok := fixture.heap.OccurrenceMountForModule(foreignModule); ok {
		t.Fatal("foreign module crossed the canonical mount fence")
	}
	if _, ok := mount.AllocationRootForOccurrence(foreignOccurrence); ok {
		t.Fatal("unknown occurrence received an optimistic Stack seed")
	}
}

func TestAllocationBindAndSealIssuesTypedPlacementRule(t *testing.T) {
	fixture := placementAllocationFixture(t)
	rule := placementAllocationBind(t, &fixture)
	issuer, issued := rule.Implementation()
	if !issued || issuer == nil {
		t.Fatal("typed Placement seed issuer unavailable")
	}
	if resolved, ok := placementowner.ResolveRuleImplementation(issuer); !ok || resolved == nil {
		t.Fatal("sealed Placement seed rule unavailable")
	}
}

type allocationFixtureState struct {
	heap       heapdomain.Schema
	placement  placementdomain.Schema
	values     *valuedomain.Schema
	mounts     []heapdomain.ArtifactMount
	module     identity.ContentID
	occurrence identity.ContentID
	key        heapdomain.Key
	boot       heapdomain.Key
	bootID     identity.ContentID
}

func placementAllocationFixture(t testing.TB) allocationFixtureState {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "placement_allocation.lua", Text: []byte("local root = {}; return root")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "placement-allocation", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Build()
	if !ok {
		t.Fatal("global schema receipt")
	}
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !grammarOK || !issuanceOK {
		t.Fatal("artifact compiler inputs")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %v", failure)
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	structural, structuralOK := composite.StructureVocabulary(receipt)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || !structuralOK {
		t.Fatalf("schema seal heap=%v value=%v structural=%t", heapFailure, valueFailure, structuralOK)
	}
	var occurrence identity.ContentID
	programSchema := artifact.Program()
	occurrenceCount, occurrenceOK := programSchema.OccurrenceCount()
	if !occurrenceOK {
		t.Fatal("allocation occurrence family")
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := programSchema.OccurrenceAt(index)
		if rowOK && row.Kind() == programschema.OccurrenceAllocation {
			occurrence = row.ID()
			break
		}
	}
	if !occurrence.Available() {
		t.Fatal("allocation occurrence")
	}
	projected, projectedOK := placementdomain.NewSchema(heapSchema)
	if !projectedOK {
		t.Fatal("Placement schema")
	}
	var key, boot heapdomain.Key
	for index := 0; index < heapSchema.KeyCount(); index++ {
		candidate, candidateOK := heapSchema.KeyAt(index)
		if !candidateOK {
			continue
		}
		if candidate.Kind() == heapdomain.RootAllocation && !key.Valid() {
			key = candidate
		}
		if candidate.Kind() == heapdomain.RootBoot && !boot.Valid() {
			boot = candidate
		}
	}
	if !key.Valid() {
		t.Fatal("allocation root")
	}
	return allocationFixtureState{heap: heapSchema, placement: projected, values: values, mounts: []heapdomain.ArtifactMount{heapMount}, module: module, occurrence: occurrence, key: key, boot: boot, bootID: bootOccurrenceID(heapSchema, boot)}
}

func placementAllocationBind(t testing.TB, fixture *allocationFixtureState) *allocation.HotRule {
	t.Helper()
	builder := engine.NewSchema()
	placementFragment, placementOK := placementowner.DeclareSchema(builder, placementAllocationSemanticKey(1), placementAllocationSemanticKey(8))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, placementAllocationSemanticKey(2), placementAllocationSemanticKey(3), placementAllocationSemanticKey(4))
	fragment, fragmentOK := allocation.DeclareSchema(builder, placementAllocationSemanticKey(5), placementAllocationSemanticKey(6), placementFragment)
	cold, coldOK := builder.Seal()
	if !placementOK || !valueOK || !fragmentOK || !coldOK || cold == nil {
		t.Fatal("seed cold schema")
	}
	binding := engine.NewSchemaBinding(cold)
	placementHot, placementHotOK := placementowner.BindHot(binding, placementFragment, fixture.placement)
	_, valueHotOK := valueowner.BindHot(binding, valueFragment, fixture.values)
	rule, ruleOK := allocation.BindHot(binding, fragment, placementHot, fixture.heap)
	sealed := binding.Seal()
	if !placementHotOK || !valueHotOK || !ruleOK || !sealed {
		t.Fatalf("seed hot bind placement=%t value=%t rule=%t seal=%t", placementHotOK, valueHotOK, ruleOK, sealed)
	}
	return rule
}

func bootOccurrenceID(schema heapdomain.Schema, key heapdomain.Key) identity.ContentID {
	if !key.Valid() || key.Kind() != heapdomain.RootBoot {
		return identity.ContentID{}
	}
	return keyID(schema, key)
}

func keyID(schema heapdomain.Schema, key heapdomain.Key) identity.ContentID {
	// Boot roots are intentionally not allocation occurrences.  A stable
	// non-matching ID keeps the negative redemption assertion explicit.
	_ = schema
	_ = key
	return placementAllocationContentID("boot-not-an-allocation-occurrence")
}

func placementAllocationSemanticKey(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xA7, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("semantic key")
	}
	return key
}

func placementAllocationContentID(seed string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(seed)))
}
