package bootstrap_test

import (
	"crypto/sha256"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	bootstrap "github.com/wippyai/go-lua/analysis/domain/heap/bootstrap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestHotBootstrapUsesSealedRootReceiptAndRejectsForeignBinding(t *testing.T) {
	schema, _ := bootstrapFixture(t)
	bootID, bootIDOK := schema.BootIDAt(0)
	key, keyOK := schema.KeyForBootID(bootID)
	root, rootOK := bootstrap.NewRoot(schema, key)
	if !bootIDOK || !keyOK || !rootOK {
		t.Fatal("bootstrap root receipt")
	}
	if id, issued := root.ID(); !issued || !id.Available() {
		t.Fatal("bootstrap root ID receipt")
	}

	builder := engine.NewSchema()
	ownerFragment, ownerOK := heapowner.DeclareSchema(builder, bootstrapKey(1))
	fragment, fragmentOK := bootstrap.DeclareSchema(builder, bootstrapKey(2), bootstrapKey(3), bootstrapKey(4), ownerFragment)
	cold, coldOK := builder.Seal()
	if !ownerOK || !fragmentOK || !coldOK || cold == nil {
		t.Fatal("bootstrap receipt cold schema")
	}
	bind := func(domain heapdomain.Schema) (*heapowner.HotOwner, *bootstrap.HotRule, *heapowner.RuleImplementation[bootstrap.Root], *engine.SchemaBinding) {
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
	localRoot, localKey, localOK := rule.Catalog().ReceiptForID(bootID)
	localID, localIssued := localRoot.ID()
	if !localOK || !localIssued || !localID.Available() || localKey != key || !rule.Catalog().FencedTo(schema) {
		t.Fatal("local bootstrap occurrence receipt")
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
	var zero bootstrap.Root
	if _, issued := zero.ID(); issued {
		t.Fatal("zero bootstrap root acquired a receipt")
	}
}

func TestBootstrapReceiptNativeHeaderAndRawAbsence(t *testing.T) {
	schema, _ := bootstrapFixture(t)
	seenMutable, seenFrozen := false, false
	seenAbsent, seenPresent := false, false
	for rootIndex := 0; rootIndex < schema.BootCount(); rootIndex++ {
		bootID, bootOK := schema.BootIDAt(rootIndex)
		key, keyOK := schema.KeyForBootID(bootID)
		root, rootOK := bootstrap.NewRoot(schema, key)
		_, value, resultOK := bootstrap.ResultForSchemaTest(schema, root)
		world, worldOK := value.WorldAt(0)
		object, objectOK := world.Exact()
		shape, frozen, headerOK := object.Header()
		wantFrozen, wantFrozenOK := schema.BootFrozen(key)
		if !bootOK || !keyOK || !rootOK || !resultOK || !worldOK || !objectOK || !headerOK || !wantFrozenOK || world.Kind() != heapdomain.WorldExact || shape != heapdomain.ShapeEligible || frozen != wantFrozen {
			t.Fatalf("bootstrap header root=%d receipt", rootIndex)
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
	foreignRoot, foreignRootOK := bootstrap.NewRoot(foreignSchema, foreignKey)
	if !foreignBootOK || !foreignKeyOK || !foreignRootOK {
		t.Fatal("foreign bootstrap evaluator fixture")
	}
	if _, _, foreignAccepted := bootstrap.ResultForSchemaTest(schema, foreignRoot); foreignAccepted {
		t.Fatal("bootstrap evaluator accepted a foreign root/schema pair")
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
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{
			{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}},
			{Identity: "StringMetatableRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateMetatable, Immutable: true, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "StringMetatableRoot"}}},
		},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "missing"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "StringMetatableRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__index"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "StringMetatableRoot"}, Mutability: target.InitialMutable},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := programschema.Global()
	if !receiptOK {
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
		artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("bootstrap artifact compile: %v", failure)
		}
		var mountOK bool
		mounts.heap[index], mountOK = heapdomain.NewArtifactMount(artifact, module, programID)
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

func bootstrapKey(value byte) engine.SemanticKey {
	digest := sha256.Sum256([]byte{0xB1, value})
	key, _ := engine.NewSemanticKey(digest, 1)
	return key
}
