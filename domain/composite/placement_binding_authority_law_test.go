package composite_test

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
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	capture "github.com/wippyai/go-lua/domain/placement/capture"
	containment "github.com/wippyai/go-lua/domain/placement/containment"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestPlacementRuleBindersRejectEqualSchemaForeignAuthorities proves the
// shared binding is part of every Placement consumer's authority contract.
// The local and foreign owners intentionally use the same cold schema and
// concrete domain schemas; only their private SchemaBinding identities differ.
//
// ReturnEscape, Transfer and Formal carry no case here: each cut to a
// generated RuleFamily, whose authority is the family-claim fence
// (ac0408205b) - engine.BindRuleFamily types the claim against the Factor
// named at the family's own declaration, so a family typed at a foreign
// coordinate is refused where it is bound, not admitted and later caught by a
// schema-equality check on a separately declared/bound Rule the way capture
// and containment - still on that older protocol - are proven here. Each
// generated family states that fence in its own package, beside the install
// arm that answers it.
func TestPlacementRuleBindersRejectEqualSchemaForeignAuthorities(t *testing.T) {
	placementSchema, values, heapSchema, calls := placementBindingLawSchemas(t)
	builder := engine.NewSchema()
	placementFragment, placementOK := placementowner.DeclareSchema(builder, placementBindingLawSemantic(1), placementBindingLawSemantic(2))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, placementBindingLawSemantic(3), placementBindingLawSemantic(4), placementBindingLawSemantic(5))
	heapFragment, heapOK := heapowner.DeclareSchema(builder, placementBindingLawSemantic(6), placementBindingLawSemantic(7))
	callFragment, callOK := callowner.DeclareSchema(builder, placementBindingLawSemantic(8))
	captureFragment, captureOK := capture.DeclareSchema(builder, placementBindingLawSemantic(11), placementBindingLawSemantic(12), valueFragment, placementFragment)
	containmentFragment, containmentOK := containment.DeclareSchema(builder, placementBindingLawSemantic(13), placementBindingLawSemantic(14), placementFragment, heapFragment)
	cold, coldOK := builder.Seal()
	if !placementOK || !valueOK || !heapOK || !callOK || !captureOK || !containmentOK || !coldOK || cold == nil {
		t.Fatalf("Placement binding-law declaration placement=%t value=%t heap=%t call=%t capture=%t containment=%t cold=%t", placementOK, valueOK, heapOK, callOK, captureOK, containmentOK, coldOK)
	}
	localBinding := engine.NewSchemaBinding(cold)
	foreignBinding := engine.NewSchemaBinding(cold)
	localPlacement, localPlacementOK := placementowner.BindHot(localBinding, placementFragment, placementSchema)
	foreignPlacement, foreignPlacementOK := placementowner.BindHot(foreignBinding, placementFragment, placementSchema)
	localValues, localValuesOK := valueowner.BindHot(localBinding, valueFragment, values)
	foreignValues, foreignValuesOK := valueowner.BindHot(foreignBinding, valueFragment, values)
	localHeap, localHeapOK := heapowner.BindHot(localBinding, heapFragment, heapSchema)
	foreignHeap, foreignHeapOK := heapowner.BindHot(foreignBinding, heapFragment, heapSchema)
	localCalls, localCallsOK := callowner.BindHot(localBinding, callFragment, calls)
	foreignCalls, foreignCallsOK := callowner.BindHot(foreignBinding, callFragment, calls)
	if !localPlacementOK || !foreignPlacementOK || !localValuesOK || !foreignValuesOK || !localHeapOK || !foreignHeapOK || !localCallsOK || !foreignCallsOK || localPlacement == nil || foreignPlacement == nil || localValues == nil || foreignValues == nil || localHeap == nil || foreignHeap == nil || localCalls == nil || foreignCalls == nil {
		t.Fatalf("Placement binding-law owner setup placement=%t/%t value=%t/%t heap=%t/%t call=%t/%t", localPlacementOK, foreignPlacementOK, localValuesOK, foreignValuesOK, localHeapOK, foreignHeapOK, localCallsOK, foreignCallsOK)
	}

	tests := []struct {
		name string
		bind func() bool
	}{
		{name: "capture placement", bind: func() bool {
			_, ok := capture.BindHot(localBinding, captureFragment, foreignPlacement, localValues, values, placementSchema)
			return ok
		}},
		{name: "containment placement", bind: func() bool {
			_, ok := containment.BindHot(localBinding, containmentFragment, foreignPlacement, localHeap, placementSchema)
			return ok
		}},
		{name: "capture value", bind: func() bool {
			_, ok := capture.BindHot(localBinding, captureFragment, localPlacement, foreignValues, values, placementSchema)
			return ok
		}},
		{name: "containment heap", bind: func() bool {
			_, ok := containment.BindHot(localBinding, containmentFragment, localPlacement, foreignHeap, placementSchema)
			return ok
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.bind() {
				t.Fatalf("accepted equal-schema authority from a foreign binding")
			}
		})
	}
}

func placementBindingLawSchemas(t testing.TB) (placementdomain.Schema, *valuedomain.Schema, heapdomain.Schema, *calldomain.Algebra) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "placement-binding-law.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "placement-binding-law", Program: program}}})
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
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	mountedProgram := snapshottest.MustMount(t, artifact, module)
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{{Program: mountedProgram, Snapshot: snapshot}})
	if !receiptOK || !grammarOK || !issuanceOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !programIDOK || !structuralOK || !heapMountOK || heapFailure != heapdomain.SealFailureNone || !placementOK || !valueMountOK || valueFailure != valuedomain.SealFailureNone || values == nil || !callsOK || calls == nil {
		t.Fatalf("Placement binding-law schemas grammar=%t failure=%v artifact=%v ingress=%t shard=%t module=%t program=%t structural=%t mount=%t heap=%v placement=%t valueMount=%t value=%v calls=%t", grammarOK, failure, artifact, lowered, shardOK, moduleOK, programIDOK, structuralOK, heapMountOK, heapFailure, placementOK, valueMountOK, valueFailure, callsOK)
	}
	return placementSchema, values, heapSchema, calls
}

func placementBindingLawSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xE7, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("Placement binding-law semantic key")
	}
	return key
}
