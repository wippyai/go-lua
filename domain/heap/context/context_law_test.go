package context_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

type contextLawFixture struct {
	heap      heapdomain.Schema
	schema    contextdomain.Schema
	link      *link.Link
	left      executioncontext.Context
	right     executioncontext.Context
	leftRight executioncontext.Transition
	rightLeft executioncontext.Transition
}

func TestContextualSealIsDeterministicButOwnerFenced(t *testing.T) {
	fixture := contextFixture(t)
	first, firstOK := contextdomain.Seal(fixture.heap, fixture.schema.Directory())
	second, secondOK := contextdomain.Seal(fixture.heap, fixture.schema.Directory())
	if !firstOK || !secondOK || first.ContentID() != second.ContentID() {
		t.Fatal("equivalent typed Heap+Directory seals were not deterministic")
	}
	if first.OwnsSchema(second) || second.OwnsSchema(first) {
		t.Fatal("independent contextual seals shared an issuer fence")
	}
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	local, localOK := first.Local(key, fixture.left, materialization.Recent)
	foreignLocal, foreignLocalOK := second.Local(key, fixture.left, materialization.Recent)
	if !keyOK || !localOK || !foreignLocalOK || local.Result().FencedTo(second) || foreignLocal.Result().FencedTo(first) {
		t.Fatal("foreign contextual schema crossed the current-reference fence")
	}
}

func TestAllocationIdentityIsSeparateFromReferenceBindingIdentity(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	left, leftOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	shared, sharedOK := fixture.schema.Share(left.Result(), fixture.leftRight)
	summary, summaryOK := fixture.schema.Local(key, fixture.left, materialization.Summary)
	right, rightOK := fixture.schema.Local(key, fixture.right, materialization.Recent)
	movedRight, movedRightOK := fixture.schema.Move(right.Result(), fixture.rightLeft)
	if !keyOK || !leftOK || !sharedOK || !summaryOK || !rightOK || !movedRightOK {
		t.Fatal("local contextual references")
	}
	if !left.Result().Allocation().Equal(shared.Result().Allocation()) {
		t.Fatal("Share did not retain one Allocation identity")
	}
	if left.Result().Equal(shared.Result()) {
		t.Fatal("same Allocation under different holders collapsed Reference identity")
	}
	if left.Result().Equal(movedRight.Result()) {
		t.Fatal("B1:H and B2:H collapsed despite distinct immutable origins")
	}
	if left.Result().Origin().Equal(movedRight.Result().Origin()) {
		t.Fatal("distinct local origins collapsed after holder projection")
	}
	if !left.Result().Allocation().Equal(summary.Result().Allocation()) || left.Result().Equal(summary.Result()) {
		t.Fatal("materialization role was omitted from Reference identity")
	}
}

func TestLocalEqualityAndForeignContextRowsRefuse(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	first, firstOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	second, secondOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	if !keyOK || !firstOK || !secondOK || !first.Equal(second) || !first.Result().Equal(second.Result()) {
		t.Fatal("equivalent local capabilities were not equal")
	}
	foreignContext, foreignContextOK := executioncontext.NewContext(fixture.link.ContentID(), fixture.left.ModuleKey(), lawID("foreign-actor"), fixture.left.RepresentativeCacheInstanceID())
	if !foreignContextOK {
		t.Fatal("foreign context fixture")
	}
	if _, ok := fixture.schema.Local(key, foreignContext, materialization.Recent); ok {
		t.Fatal("Local accepted a context outside the sealed Directory")
	}
	// Heap remains the sole authority over which materialization role a key
	// carries: an allocation root has Recent and Summary alternatives, and the
	// contextual issuer never mints an Exact row the Heap refuses.
	if _, ok := fixture.schema.Local(key, fixture.left, materialization.Exact); ok {
		t.Fatal("Local minted a role the Heap does not admit at an allocation key")
	}
}

func TestShareMovePreserveAllocationOriginAndReferenceHistoryIsIrrelevant(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	local, localOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	shared, sharedOK := fixture.schema.Share(local.Result(), fixture.leftRight)
	movedBack, movedBackOK := fixture.schema.Move(shared.Result(), fixture.rightLeft)
	directShare, directShareOK := fixture.schema.Share(local.Result(), fixture.leftRight)
	directMove, directMoveOK := fixture.schema.Move(local.Result(), fixture.leftRight)
	if !keyOK || !localOK || !sharedOK || !movedBackOK || !directShareOK || !directMoveOK {
		t.Fatal("share/move contextual capabilities")
	}
	if !local.Result().Allocation().Equal(shared.Result().Allocation()) || !local.Result().Allocation().Equal(movedBack.Result().Allocation()) {
		t.Fatal("Share/Move changed Allocation identity")
	}
	if !local.Result().Origin().Equal(shared.Result().Origin()) || !local.Result().Origin().Equal(movedBack.Result().Origin()) {
		t.Fatal("Share/Move changed immutable origin")
	}
	if !movedBack.Result().Equal(local.Result()) || !shared.Result().Equal(directShare.Result()) || !directMove.Result().Equal(directShare.Result()) {
		t.Fatal("current Reference equality retained transfer history or event kind")
	}
	if shared.Result().Holder().ID() != fixture.right.ID() || movedBack.Result().Holder().ID() != fixture.left.ID() {
		t.Fatal("Share/Move holder projection")
	}
}

func TestForeignTransitionsAndLinksRefuse(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	local, localOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	foreignContext, foreignContextOK := executioncontext.NewContext(lawID("foreign-link"), fixture.left.ModuleKey(), lawID("actor"), lawID("representative"))
	if !keyOK || !localOK || !foreignContextOK {
		t.Fatal("foreign transition fixture")
	}
	foreignTransition, transitionOK := executioncontext.NewTransition(fixture.link.ContentID(), fixture.left.ID(), foreignContext.ID())
	if !transitionOK {
		t.Fatal("foreign transition row")
	}
	if _, ok := fixture.schema.Share(local.Result(), foreignTransition); ok {
		t.Fatal("Share accepted a transition whose target Context is outside Directory")
	}
	foreignDirectory, directoryOK := executioncontext.Seal(lawID("foreign-link"), []executioncontext.Context{foreignContext}, []executioncontext.RootContext{mustRoot(t, foreignContext, "foreign-root")}, nil)
	if !directoryOK {
		t.Fatal("foreign directory fixture")
	}
	if _, ok := contextdomain.Seal(fixture.heap, foreignDirectory); ok {
		t.Fatal("contextual Seal accepted a directory from a foreign Link")
	}
}

func TestCopyRequiresFreshTargetAndRecordsBoundedSourceProvenance(t *testing.T) {
	fixture := contextFixture(t)
	sourceKey, sourceKeyOK := fixture.heap.AllocationKeyAt(0)
	targetKey, targetKeyOK := fixture.schema.FreshAt(0)
	local, localOK := fixture.schema.Local(sourceKey, fixture.left, materialization.Recent)
	copyEvent, copyOK := fixture.schema.Copy(local.Result(), targetKey, fixture.leftRight)
	targetLocal, targetLocalOK := fixture.schema.Local(targetKey, fixture.right, materialization.Recent)
	if !sourceKeyOK || !targetKeyOK || !localOK || !copyOK || !copyEvent.Valid() || !targetLocalOK {
		t.Fatal("copy fixture did not produce a fresh contextual target")
	}
	copied := copyEvent.Result()
	provenance, provenanceOK := copyEvent.CopiedFrom()
	if !provenanceOK || !provenance.Equal(local.Result()) {
		t.Fatal("Copy did not retain its bounded source Reference")
	}
	if copied.Key() == local.Result().Key() || copied.Allocation().Equal(local.Result().Allocation()) {
		t.Fatal("Copy reused source Allocation identity")
	}
	if copied.Origin().Equal(local.Result().Origin()) || copied.Origin().Context().ID() != fixture.right.ID() {
		t.Fatal("Copy did not create a new target origin")
	}
	if copied.Holder().ID() != fixture.right.ID() || copied.Role() != local.Result().Role() {
		t.Fatal("Copy changed holder/role unexpectedly")
	}
	if !copied.Equal(targetLocal.Result()) {
		t.Fatal("current Reference identity retained Copy event history")
	}
	if _, ok := fixture.schema.Copy(local.Result(), sourceKey, fixture.leftRight); ok {
		t.Fatal("Copy accepted a non-fresh Program allocation")
	}
	if _, ok := fixture.schema.Copy(local.Result(), targetKey, fixture.rightLeft); ok {
		t.Fatal("Copy accepted a transition with the wrong source holder")
	}
}

func contextFixture(t testing.TB) contextLawFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "context-reference-law.lua", Text: []byte("return fresh()")})
	if err != nil {
		t.Fatal(err)
	}
	freshBinding := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh"}}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		Operations: []vocabulary.OperationSpec{
			{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			{Bindings: []vocabulary.BindingSpec{freshBinding}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: portableAnyTypes(1), Tail: vocabulary.ValuesClosed}, FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"other"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: portableAnyTypes(1), Tail: vocabulary.ValuesClosed}, FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: freshBinding}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "fresh", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "context-reference-law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	shard, shardOK := linked.Project().Mounts().At(0)
	mountedProgram, mountedProgramOK := linked.Project().Mounts().Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	artifact, failure := artifactcompiler.CompileDetailed(mountedProgram, compilation.ExecutionSchemaID(), issuance)
	if !compilationOK || !issuanceOK || !shardOK || !mountedProgramOK || mountedProgram == nil || failure.Available() || artifact == nil {
		t.Fatalf("artifact seal compilation=%t issuance=%t shard=%t program=%t failure=%v artifact=%v", compilationOK, issuanceOK, shardOK, mountedProgramOK, failure, artifact != nil)
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	if !shardOK || !moduleOK || !programIDOK || !mountOK || heapFailure != heapdomain.SealFailureNone || !heapSchema.Valid() || heapSchema.FreshCount() == 0 {
		t.Fatalf("heap seal shard=%t module=%t program=%t mount=%t failure=%v fresh=%d", shardOK, moduleOK, programIDOK, mountOK, heapFailure, heapSchema.FreshCount())
	}
	linkID := linked.ContentID()
	// The two contexts are sibling cache instances of one module inside one
	// actor. A module cache is actor-local, so a transition between two
	// actors is not an edge any Directory carries.
	actor := lawID("actor")
	left, leftOK := executioncontext.NewContext(linkID, module, actor, lawID("left-representative"))
	right, rightOK := executioncontext.NewContext(linkID, module, actor, lawID("right-representative"))
	leftRoot := mustRoot(t, left, "left-root")
	rightRoot := mustRoot(t, right, "right-root")
	leftRight, leftRightOK := executioncontext.NewTransition(linkID, left.ID(), right.ID())
	rightLeft, rightLeftOK := executioncontext.NewTransition(linkID, right.ID(), left.ID())
	directory, directoryOK := executioncontext.Seal(linkID, []executioncontext.Context{right, left}, []executioncontext.RootContext{rightRoot, leftRoot}, []executioncontext.Transition{rightLeft, leftRight})
	contextSchema, schemaOK := contextdomain.Seal(heapSchema, directory)
	if !leftOK || !rightOK || !leftRightOK || !rightLeftOK || !directoryOK || !schemaOK {
		t.Fatalf("context seal contexts=%t/%t transitions=%t/%t directory=%t schema=%t", leftOK, rightOK, leftRightOK, rightLeftOK, directoryOK, schemaOK)
	}
	return contextLawFixture{heap: heapSchema, schema: contextSchema, link: linked, left: left, right: right, leftRight: leftRight, rightLeft: rightLeft}
}

func mustRoot(t testing.TB, row executioncontext.Context, label string) executioncontext.RootContext {
	t.Helper()
	root, ok := executioncontext.NewRootContext(row.LinkID(), lawID(label), row.ID())
	if !ok {
		t.Fatalf("root %s", label)
	}
	return root
}

func lawID(label string) identity.ContentID {
	id, _ := identity.DeriveContentID("domain/heap/context/law/" + fmt.Sprint(label))
	return id
}

func portableAnyTypes(count int) []schematype.Type {
	values := make([]schematype.Type, count)
	for index := range values {
		value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
		if !ok {
			panic("portable any type")
		}
		values[index] = value
	}
	return values
}
