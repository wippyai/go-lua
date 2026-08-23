package bootstrap_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	bootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// TestRuleEntryDeclaresCanonicalBootProgram states the shape of the
// declaration this package now is: a zero-join exact write whose candidate is
// the Link-global directory of sealed bootstrap roots.
func TestRuleEntryDeclaresCanonicalBootProgram(t *testing.T) {
	spec := bootstrap.RuleEntry()
	problem, ok := spec.Program.Check()
	if !ok {
		t.Fatalf("heap-bootstrap Program rejected: %+v", problem)
	}
	if spec.Lane != rule.LaneLink || len(spec.Issues) != 0 {
		t.Fatalf("heap-bootstrap lane = %v with %d issuances", spec.Lane, len(spec.Issues))
	}
	if spec.Program.JoinCount() != 0 || len(spec.Program.Fold.Inputs) != 0 || len(spec.Program.Fold.Outputs) != 1 {
		t.Fatalf("heap-bootstrap Program shape = joins:%d inputs:%d outputs:%d", spec.Program.JoinCount(), len(spec.Program.Fold.Inputs), len(spec.Program.Fold.Outputs))
	}
	if spec.Program.Carry != nil {
		t.Fatalf("heap-bootstrap carry = %#v", spec.Program.Carry)
	}
	if spec.Program.Candidate.Member != heapdomain.BootRoots {
		t.Fatalf("heap-bootstrap candidate = %s", spec.Program.Candidate.Member)
	}
	if spec.Program.Fold.Outputs[0].Mode != program.ModeExact {
		t.Fatalf("heap-bootstrap output mode = %v", spec.Program.Fold.Outputs[0].Mode)
	}
}

// TestRoutedBootOutputIsRefused is the nearest negative: a zero-read Link rule
// derives no relation to route a write through.
func TestRoutedBootOutputIsRefused(t *testing.T) {
	declaration := bootstrap.RuleEntry().Program
	declaration.Fold.Outputs[0].Mode = program.ModeRoute
	problem, ok := declaration.Check()
	if ok || problem.Kind != program.ProblemOutput {
		t.Fatalf("routed heap-bootstrap output admitted: problem=%+v ok=%t", problem, ok)
	}
}

// TestSealedRootReceiptDirectoryIsTheOccurrenceInventory is the sealed-receipt
// law, stated where the occurrences now come from. Every bootstrap root Heap
// sealed carries its image and its identity, that directory is fenced to the
// schema that issued it, and it is exactly the occurrence directory the axis
// publishes for this rule's candidate relation - so the inventory a Link rule
// admits is derived from the declaration rather than issued by a callback.
func TestSealedRootReceiptDirectoryIsTheOccurrenceInventory(t *testing.T) {
	schema, _ := bootstrapFixture(t)
	owner := heapdomain.NewRelationOwner(schema)
	directory, directoryOK := any(owner).(memberrelation.OccurrenceDirectory)
	relation, relationOK := heapdomain.AxisMemberCatalog().RelationOrdinal(heapdomain.BootRoots)
	if owner == nil || !directoryOK || !relationOK {
		t.Fatal("heap boot occurrence directory")
	}
	count, countOK := directory.OccurrenceCount(relation)
	if !countOK || count != schema.BootCount() || count == 0 {
		t.Fatalf("occurrence census = %d/%t, want %d", count, countOK, schema.BootCount())
	}
	for index := 0; index < count; index++ {
		id, idOK := directory.OccurrenceIDAt(relation, index)
		want, wantOK := schema.BootIDAt(index)
		key, keyOK := schema.KeyForBootID(id)
		rootID, rootIDOK := schema.BootRootID(key)
		value, valueOK := schema.BootValue(key)
		if !idOK || !wantOK || id != want || !keyOK || !rootIDOK || !rootID.Available() || !valueOK || !value.Valid() {
			t.Fatalf("occurrence row %d = %v/%t", index, id, idOK)
		}
		// The candidate is addressed by the occurrence alone: this relation is
		// Link-global, so a mount is refused rather than ignored.
		candidate, candidateOK := owner.CandidateAt(relation, identity.ContentID{}, id, 0)
		if !candidateOK || candidate != uint32(index) {
			t.Fatalf("occurrence row %d candidate = %d/%t", index, candidate, candidateOK)
		}
		if _, mounted := owner.CandidateAt(relation, id, id, 0); mounted {
			t.Fatalf("occurrence row %d admitted a mount", index)
		}
		if ordinal, ordinalOK := schema.BootRootOrdinal(key); !ordinalOK || ordinal != uint32(index) {
			t.Fatalf("occurrence row %d ordinal = %d/%t", index, ordinal, ordinalOK)
		}
		if fact, outcome := heapdomain.BootFact(key); outcome != structure.Concrete || !fact.Valid() {
			t.Fatalf("occurrence row %d folded to %v", index, outcome)
		}
	}
	if _, ok := directory.OccurrenceIDAt(relation, count); ok {
		t.Fatal("the directory admitted a row past its census")
	}
	var zero heapdomain.Key
	if _, issued := schema.BootRootID(zero); issued {
		t.Fatal("zero bootstrap key acquired a row")
	}
	if _, admitted := owner.CandidateAt(relation, identity.ContentID{}, identity.ContentID{}, 0); admitted {
		t.Fatal("an unavailable occurrence resolved a bootstrap candidate")
	}
	ingress, ingressOK := heapdomain.AxisMemberCatalog().RelationOrdinal(heapdomain.IngressSeeds)
	if !ingressOK {
		t.Fatal("heap ingress relation ordinal")
	}
	if _, published := directory.OccurrenceCount(ingress); published {
		t.Fatal("a mounted relation published an occurrence directory")
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
	heap   []programmount.MountedArtifact
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
	mounts := bootstrapFixtureMounts{linked: linked, heap: make([]programmount.MountedArtifact, projectMounts.Count())}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		_, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("bootstrap artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("bootstrap artifact compile: %v", failure)
		}
		var mountOK bool
		mounts.heap[index], mountOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
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
