package bootstrap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/internal/testfixture"

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
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	bootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
)

// TestRuleEntryDeclaresCanonicalGlobalBootstrapProgram states the shape of the
// declaration this package now is: a zero-join exact write whose candidate is
// the Link-global directory of Host global binding receipts.
func TestRuleEntryDeclaresCanonicalGlobalBootstrapProgram(t *testing.T) {
	spec := bootstrap.RuleEntry()
	problem, ok := spec.Program.Check()
	if !ok {
		t.Fatalf("value-bootstrap Program rejected: %+v", problem)
	}
	if spec.Lane != rule.LaneLink || len(spec.Issues) != 0 {
		t.Fatalf("value-bootstrap lane = %v with %d issuances", spec.Lane, len(spec.Issues))
	}
	if spec.Program.JoinCount() != 0 || len(spec.Program.Fold.Inputs) != 0 || len(spec.Program.Fold.Outputs) != 1 {
		t.Fatalf("value-bootstrap Program shape = joins:%d inputs:%d outputs:%d", spec.Program.JoinCount(), len(spec.Program.Fold.Inputs), len(spec.Program.Fold.Outputs))
	}
	if spec.Program.Carry != nil {
		t.Fatalf("value-bootstrap carry = %#v", spec.Program.Carry)
	}
	if spec.Program.Candidate.AxisRelation.Member != valuedomain.GlobalBootstrapResults {
		t.Fatalf("value-bootstrap candidate = %s", spec.Program.Candidate.AxisRelation.Member)
	}
	if spec.Program.Fold.Outputs[0].Mode != program.ModeExact {
		t.Fatalf("value-bootstrap output mode = %v", spec.Program.Fold.Outputs[0].Mode)
	}
}

// TestRoutedGlobalBootstrapOutputIsRefused is the nearest negative: a zero-read
// Link rule derives no relation to route a write through, so the one degree of
// freedom that would let it write a coordinate it never derived must be refused
// by the program itself.
func TestRoutedGlobalBootstrapOutputIsRefused(t *testing.T) {
	declaration := bootstrap.RuleEntry().Program
	declaration.Fold.Outputs[0].Mode = program.ModeRoute
	problem, ok := declaration.Check()
	if ok || problem.Kind != program.ProblemOutput {
		t.Fatalf("routed value-bootstrap output admitted: problem=%+v ok=%t", problem, ok)
	}
}

// TestCanonicalGlobalDirectoryIsTheOccurrenceInventory is the canonical-global
// law, stated where the occurrences now come from. Value's sealed receipt
// directory is fenced to the schema that issued it, and it is exactly the
// occurrence directory the axis publishes for this rule's candidate relation -
// so the inventory a Link rule admits is derived from the declaration rather
// than issued by a callback.
//
// An absent initial value is part of that directory. It is a real Host global
// binding with a real target coordinate whose fold concludes no candidate, and
// dropping it would silently shrink the occurrences the program admits.
func TestCanonicalGlobalDirectoryIsTheOccurrenceInventory(t *testing.T) {
	local := bootstrapFixture(t, "local bootstrap")
	foreign := bootstrapFixture(t, "foreign bootstrap")

	localID, localOK := local.schema.GlobalBootstrapResultIDAt(0)
	foreignID, foreignOK := foreign.schema.GlobalBootstrapResultIDAt(0)
	if !localOK || !foreignOK || localID == foreignID {
		t.Fatal("bootstrap global identities")
	}
	if receipt, ok := local.schema.GlobalBootstrapResultForID(localID); !ok || receipt == nil {
		t.Fatal("local bootstrap result rejected")
	}
	if receipt, ok := local.schema.GlobalBootstrapResultForID(foreignID); ok || receipt != nil {
		t.Fatal("foreign bootstrap result crossed Value owner")
	}
	if receipt, ok := local.schema.GlobalBootstrapResultForID(identity.ContentID{}); ok || receipt != nil {
		t.Fatal("zero bootstrap result accepted")
	}

	owner := valuedomain.NewRelationOwner(local.schema)
	directory, directoryOK := any(owner).(memberrelation.OccurrenceDirectory)
	relation, relationOK := valuedomain.AxisMemberCatalog().RelationOrdinal(valuedomain.GlobalBootstrapResults)
	if owner == nil || !directoryOK || !relationOK {
		t.Fatal("value global bootstrap occurrence directory")
	}
	count, countOK := directory.OccurrenceCount(relation)
	if !countOK || count != local.schema.GlobalBootstrapResultCount() || count == 0 {
		t.Fatalf("occurrence census = %d/%t, want %d", count, countOK, local.schema.GlobalBootstrapResultCount())
	}
	absent, concrete := 0, 0
	for index := 0; index < count; index++ {
		id, idOK := directory.OccurrenceIDAt(relation, index)
		want, wantOK := local.schema.GlobalBootstrapResultIDAt(index)
		if !idOK || !wantOK || id != want {
			t.Fatalf("occurrence row %d = %v/%t, want %v", index, id, idOK, want)
		}
		// The candidate is addressed by the occurrence alone: this relation is
		// Link-global, so a mount is refused rather than ignored.
		candidate, candidateOK := owner.CandidateAt(relation, identity.ContentID{}, id, 0)
		if !candidateOK || candidate != uint32(index) {
			t.Fatalf("occurrence row %d candidate = %d/%t", index, candidate, candidateOK)
		}
		if _, mounted := owner.CandidateAt(relation, want, id, 0); mounted {
			t.Fatalf("occurrence row %d admitted a mount", index)
		}
		receipt, receiptOK := local.schema.GlobalBootstrapResultAt(index)
		if !receiptOK || receipt == nil {
			t.Fatalf("occurrence row %d receipt", index)
		}
		if ordinal, ordinalOK := local.schema.GlobalBootstrapResultOrdinal(receipt); !ordinalOK || ordinal != uint32(index) {
			t.Fatalf("occurrence row %d ordinal = %d/%t", index, ordinal, ordinalOK)
		}
		_, outcome := valuedomain.GlobalBootstrapFact(receipt)
		switch outcome {
		case structure.Concrete:
			concrete++
		case structure.NoCandidate:
			absent++
		default:
			t.Fatalf("occurrence row %d folded to %v", index, outcome)
		}
	}
	if absent == 0 || concrete == 0 {
		t.Fatalf("directory folds = %d concrete, %d absent; the fixture declares both", concrete, absent)
	}
	if _, ok := directory.OccurrenceIDAt(relation, count); ok {
		t.Fatal("the directory admitted a row past its census")
	}
	if _, ok := directory.OccurrenceCount(relation + 1); ok {
		t.Fatal("a mounted relation published an occurrence directory")
	}
}

type bootstrapFixtureState struct {
	schema *valuedomain.Schema
}

func bootstrapFixture(t testing.TB, name string) *bootstrapFixtureState {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local root = _G\nlocal absent = __link_absent\nreturn root, absent\n")})
	if err != nil {
		t.Fatal(err)
	}
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		Operations:   []vocabulary.OperationSpec{requireOperation},
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: bootstrapLiteral("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootstrapLiteral("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: bootstrapLiteral("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: bootstrapLiteral("__link_absent")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !ok || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}
	artifact, failure := artifactcompiler.CompileDetailed(programValue, grammar, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(receipt)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || schema.GlobalBootstrapResultCount() == 0 {
		t.Fatalf("schema seal heap=%s value=%s globals=%d", heapFailure, valueFailure, schema.GlobalBootstrapResultCount())
	}
	return &bootstrapFixtureState{schema: schema}
}

func bootstrapLiteral(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}
