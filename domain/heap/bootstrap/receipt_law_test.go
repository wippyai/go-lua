package bootstrap_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	bootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestHotBootstrapUsesSealedRootReceiptAndRejectsForeignBinding(t *testing.T) {
	schema, _ := bootstrapFixture(t)
	bootID, bootIDOK := schema.BootIDAt(0)
	key, keyOK := schema.KeyForBootID(bootID)
	rootID, rootIDOK := schema.BootRootID(key)
	rootValue, rootValueOK := schema.BootValue(key)
	if !bootIDOK || !keyOK || !rootIDOK || !rootValueOK || !rootValue.Valid() {
		t.Fatal("bootstrap root row")
	}
	if !rootID.Available() {
		t.Fatal("bootstrap root row ID")
	}

	builder := engine.NewSchema()
	ownerFragment, ownerOK := heapowner.DeclareSchema(builder, bootstrapKey(1), bootstrapKey(101))
	fragment, fragmentOK := bootstrap.DeclareSchema(builder, bootstrapKey(2), bootstrapKey(3), ownerFragment)
	cold, coldOK := builder.Seal()
	if !ownerOK || !fragmentOK || !coldOK || cold == nil {
		t.Fatal("bootstrap receipt cold schema")
	}
	bind := func(domain heapdomain.Schema) (*heapowner.HotOwner, *bootstrap.HotRule, *heapowner.RuleImplementation[heapdomain.Key], *engine.SchemaBinding) {
		binding := engine.NewSchemaBinding(cold)
		owner, ownerHotOK := heapowner.BindHot(binding, ownerFragment, domain)
		rule, ruleOK := bootstrap.BindHot(fragment, owner)
		if !ownerHotOK || !ruleOK || rule == nil || !binding.Seal() {
			return nil, nil, nil, binding
		}
		issuer, issued := rule.Implementation()
		if !issued {
			return owner, rule, nil, binding
		}
		return owner, rule, issuer, binding
	}
	owner, rule, issuer, binding := bind(schema)
	if owner == nil || rule == nil || issuer == nil || binding == nil {
		t.Fatal("bootstrap hot receipt bind")
	}
	if implementation, issued := heapowner.ResolveRuleImplementationFor(owner, issuer); !issued || implementation == nil {
		t.Fatal("bootstrap hot receipt issue")
	}
	localKey, localOK := schema.KeyForBootID(bootID)
	localID, localIssued := schema.BootRootID(localKey)
	if !localOK || !localIssued || !localID.Available() || localKey != key || rule.Count() != schema.BootCount() {
		t.Fatal("local bootstrap occurrence row")
	}

	foreignSchema, _ := bootstrapFixture(t)
	foreignOwner, _, foreignIssuer, foreignBinding := bind(foreignSchema)
	if foreignOwner == nil || foreignIssuer == nil || foreignBinding == nil {
		t.Fatal("foreign bootstrap hot receipt bind")
	}
	if implementation, accepted := heapowner.ResolveRuleImplementationFor(foreignOwner, issuer); accepted || implementation != nil {
		t.Fatal("foreign equal binding accepted local bootstrap receipt")
	}
	if implementation, accepted := heapowner.ResolveRuleImplementationFor(foreignOwner, foreignIssuer); !accepted || implementation == nil {
		t.Fatal("foreign equal binding rejected own bootstrap receipt")
	}
	var zero heapdomain.Key
	if _, issued := schema.BootRootID(zero); issued {
		t.Fatal("zero bootstrap key acquired a row")
	}
}

func TestBootstrapReceiptNativeHeaderAndRawAbsence(t *testing.T) {
	schema, _ := bootstrapFixture(t)
	seenMutable, seenFrozen := false, false
	seenAbsent, seenPresent := false, false
	for rootIndex := 0; rootIndex < schema.BootCount(); rootIndex++ {
		bootID, bootOK := schema.BootIDAt(rootIndex)
		key, keyOK := schema.KeyForBootID(bootID)
		value, resultOK := schema.BootValue(key)
		world, worldOK := value.WorldAt(0)
		object, objectOK := world.Exact()
		shape, frozen, headerOK := object.Header()
		wantFrozen, wantFrozenOK := schema.BootFrozen(key)
		if !bootOK || !keyOK || !resultOK || !worldOK || !objectOK || !headerOK || !wantFrozenOK || world.Kind() != heapdomain.WorldExact || shape != heapdomain.ShapeEligible || frozen != wantFrozen {
			t.Fatalf("bootstrap header root=%d row", rootIndex)
		}
		if frozen == heapdomain.FrozenMutable {
			seenMutable = true
		}
		if frozen == heapdomain.FrozenFrozen {
			seenFrozen = true
		}
		foundAbsent, foundPresent, foundEntry := false, false, false
		for index := 0; index < schema.BootEntryCount(); index++ {
			entry, entryOK := schema.BootEntryAt(index)
			entryKey, entryKeyOK := entry.Key()
			slot, slotOK := entry.Slot()
			selector, selectorOK := schema.SelectorForSlot(slot)
			if !entryOK || !entryKeyOK || !slotOK || !selectorOK || entryKey != key {
				continue
			}
			foundEntry = true
			heapdomainRaw := heapdomain.RawInvalid
			if !schema.VisitRawAccess(key, value, materialization.Exact, selector, func(access heapdomain.RawAccess) bool {
				cell, cellOK := access.Cell()
				raw, rawOK := cell.Raw()
				if cellOK && rawOK {
					heapdomainRaw = raw
				}
				return cellOK && rawOK
			}) {
				t.Fatalf("bootstrap raw visit root=%d entry=%d", rootIndex, index)
			}
			switch heapdomainRaw {
			case heapdomain.RawAbsent:
				foundAbsent = true
			case heapdomain.RawPresent:
				foundPresent = true
			}
		}
		seenAbsent = seenAbsent || foundAbsent
		seenPresent = seenPresent || foundPresent
		if !foundEntry || (!foundAbsent && !foundPresent) {
			t.Fatalf("bootstrap fixture root=%d raw entry=%t absent=%t present=%t", rootIndex, foundEntry, foundAbsent, foundPresent)
		}
	}
	if !seenMutable || !seenFrozen || !seenAbsent || !seenPresent {
		t.Fatalf("bootstrap fixture headers mutable=%t frozen=%t raw absent=%t present=%t", seenMutable, seenFrozen, seenAbsent, seenPresent)
	}
	foreignSchema, _ := bootstrapFixture(t)
	foreignBootID, foreignBootOK := foreignSchema.BootIDAt(0)
	foreignKey, foreignKeyOK := foreignSchema.KeyForBootID(foreignBootID)
	_, foreignValueOK := foreignSchema.BootValue(foreignKey)
	if !foreignBootOK || !foreignKeyOK || !foreignValueOK {
		t.Fatal("foreign bootstrap evaluator fixture")
	}
	if _, foreignAccepted := schema.BootValue(foreignKey); foreignAccepted {
		t.Fatal("bootstrap evaluator accepted a foreign key/schema pair")
	}
}

type bootstrapFixtureMounts struct {
	linked *link.Link
	heap   []heapdomain.ArtifactMount
}

func bootstrapFixture(t testing.TB) (heapdomain.Schema, bootstrapFixtureMounts) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "bootstrap_receipt.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics: domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{
			{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}},
			{Identity: "StringMetatableRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateMetatable, Immutable: true, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "StringMetatableRoot"}}},
		},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "missing"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
			{Root: "StringMetatableRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__index"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "StringMetatableRoot"}, Mutability: vocabulary.InitialMutable},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("bootstrap artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := bootstrapFixtureMounts{linked: linked, heap: make([]heapdomain.ArtifactMount, projectMounts.Count())}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("bootstrap artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("bootstrap artifact compile: %v", failure)
		}
		var mountOK bool
		mounts.heap[index], mountOK = heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		if !mountOK {
			t.Fatal("bootstrap artifact mount receipt")
		}
	}
	schema, failure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	if failure != heapdomain.SealFailureNone {
		t.Fatalf("bootstrap Heap schema: %v", failure)
	}
	return schema, mounts
}

func bootstrapKey(value byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xB1, value})
	key, _ := identity.NewSemanticKey(digest, 1)
	return key
}
