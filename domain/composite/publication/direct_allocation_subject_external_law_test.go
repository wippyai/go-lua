package publication_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite"
	publication "github.com/wippyai/go-lua/domain/composite/publication"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func portableAnyType() schematype.Type {
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		panic("portable any type")
	}
	return value
}

type directAllocationSubjectFixture struct {
	values      *valuedomain.Schema
	packs       *packdomain.Schema
	heaps       heapdomain.Schema
	allocation  *valuedomain.AllocationResult
	other       *valuedomain.AllocationResult
	source      packdomain.SemanticSource
	otherSource packdomain.SemanticSource
}

func directAllocationSubjectContract(t testing.TB) (*contract.Contract, vocabulary.Operation) {
	t.Helper()
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil || contract == nil {
		t.Fatalf("seal direct allocation Target: %v", err)
	}
	operation, operationOK := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}})
	if !operationOK {
		t.Fatal("direct allocation operation")
	}
	return contract, operation
}

func directAllocationSubjectFixtureFor(t testing.TB, label string) directAllocationSubjectFixture {
	t.Helper()
	contract, operation := directAllocationSubjectContract(t)
	// Both literal allocations reach the method call directly. The receiver is
	// formal 0 and therefore names its allocation root itself; the argument is
	// a distinct literal at formal 1 for the negative allocation/source cases.
	// A named local would be a storage-alias coordinate and must not satisfy the
	// direct-identity receipt.
	published, err := lower.Lower(lower.Source{Name: "direct_allocation_subject_" + label + ".lua", Text: []byte("({}):send({})\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "direct_allocation_subject_" + label, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := composite.Global()
	artifactGrammar, artifactGrammarOK := composite.ArtifactGrammar(grammar)
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory()
	if !grammarOK || !artifactGrammarOK || !issuanceOK {
		t.Fatal("direct allocation program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("direct allocation mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, artifactGrammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile direct allocation artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()})
	if err != nil || types == nil {
		t.Fatalf("seal direct allocation types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedProgram{{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal direct allocation static: %v", err)
	}
	packMount, packMountOK := packdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	if !packMountOK || !heapMountOK || !valueMountOK {
		t.Fatal("direct allocation artifact mounts")
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []packdomain.ArtifactMount{packMount})
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if !packsOK || packs == nil || heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal direct allocation schemas pack=%t heap=%s value=%s", packsOK, heapFailure, valueFailure)
	}
	selector, selectorOK := packs.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	otherSelector, otherSelectorOK := packs.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !selectorOK || !otherSelectorOK {
		t.Fatal("direct allocation fixed selectors")
	}
	frozen, catalog, coldPublished := artifact.ColdPublication()
	coldProgram := programschema.Program{
		Frozen: frozen, ArtifactID: artifact.ID(),
		ProgramID: artifact.CompileKey().ProgramID(), SchemaID: artifact.CompileKey().SchemaDigest(),
	}
	if !coldPublished || !catalog.Available() || !coldProgram.Available() {
		t.Fatal("direct allocation cold program")
	}
	callCount, callsOK := coldProgram.CallCount()
	if !callsOK {
		t.Fatal("direct allocation call family")
	}
	var callID identity.ContentID
	for index := 0; index < callCount; index++ {
		call, callOK := coldProgram.CallAt(index)
		if callOK && call.Form() == programschema.CallFormMethod {
			callID = call.ID()
			break
		}
	}
	if !callID.Available() {
		t.Fatal("direct allocation call")
	}
	source, sourceOK := packs.MountedInputSemanticSource(module, callID, selector)
	otherSource, otherSourceOK := packs.MountedInputSemanticSource(module, callID, otherSelector)
	if !sourceOK || !otherSourceOK || !packs.OwnsSemanticSource(source) || !packs.OwnsSemanticSource(otherSource) || source.Same(otherSource) {
		t.Fatal("direct allocation source")
	}
	fixture := directAllocationSubjectFixture{values: values, packs: packs, heaps: heaps, source: source, otherSource: otherSource}
	rootCount, resultCount, directCount := 0, 0, 0
	for index := 0; index < heaps.KeyCount(); index++ {
		key, keyOK := heaps.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		rootCount++
		allocation, allocationOK := values.AllocationResultFor(key)
		if !allocationOK {
			continue
		}
		resultCount++
		if _, directOK := publication.NewDirectAllocationSubject(values, packs, source, allocation); directOK {
			fixture.allocation = allocation
			directCount++
		} else if fixture.other == nil {
			fixture.other = allocation
		}
	}
	if fixture.allocation == nil || fixture.other == nil {
		t.Fatalf("direct and distinct allocation results roots=%d results=%d direct=%d", rootCount, resultCount, directCount)
	}
	return fixture
}

func TestDirectAllocationSubjectFencesValuePackAndAllocation(t *testing.T) {
	left := directAllocationSubjectFixtureFor(t, "equal")
	right := directAllocationSubjectFixtureFor(t, "equal")
	foreign := directAllocationSubjectFixtureFor(t, "foreign")
	if left.heaps.ContentID() != right.heaps.ContentID() {
		t.Fatal("equal-content Heap schemas")
	}
	direct, directOK := publication.NewDirectAllocationSubject(left.values, left.packs, left.source, left.allocation)
	rightDirect, rightDirectOK := publication.NewDirectAllocationSubject(right.values, right.packs, right.source, right.allocation)
	leftID, leftIDOK := direct.ContentID()
	rightID, rightIDOK := rightDirect.ContentID()
	if !directOK || !rightDirectOK || !leftIDOK || !rightIDOK || !direct.Valid() || !rightDirect.Valid() || leftID != rightID {
		t.Fatal("equal-content direct identity receipt")
	}
	if _, accepted := publication.NewDirectAllocationSubject(left.values, right.packs, right.source, left.allocation); accepted {
		t.Fatal("foreign equal-content Pack/Value entered direct identity")
	}
	if _, accepted := publication.NewDirectAllocationSubject(left.values, left.packs, foreign.source, left.allocation); accepted {
		t.Fatal("nonmember Pack source entered direct identity")
	}
	if _, accepted := publication.NewDirectAllocationSubject(left.values, left.packs, left.otherSource, left.allocation); accepted {
		t.Fatal("different valid local Pack source entered direct identity")
	}
	if _, accepted := publication.NewDirectAllocationSubject(left.values, left.packs, left.source, right.allocation); accepted {
		t.Fatal("foreign Value AllocationResult entered direct identity")
	}
	if _, accepted := publication.NewDirectAllocationSubject(left.values, left.packs, left.source, left.other); accepted {
		t.Fatal("different allocation entered direct identity")
	}
	if _, accepted := publication.NewDirectAllocationSubject(nil, nil, packdomain.SemanticSource{}, nil); accepted {
		t.Fatal("missing Value, Pack, source, and allocation evidence issued direct identity")
	}
}

// The receipt classifies exactly the one Value coordinate it was issued at.
// The summary vector is read by index, so a receipt that answered at any other
// index would let one allocation's membership stand for another's cell.
func TestDirectAllocationSubjectClassifiesOnlyItsOwnCoordinate(t *testing.T) {
	fixture := directAllocationSubjectFixtureFor(t, "coordinate")
	direct, directOK := publication.NewDirectAllocationSubject(fixture.values, fixture.packs, fixture.source, fixture.allocation)
	recent, recentOK := fixture.allocation.Fresh()
	if !directOK || !direct.Valid() || !recentOK {
		t.Fatal("direct coordinate fixture")
	}
	matchedIndex := -1
	for index := 0; index < fixture.values.CoordinateCount(); index++ {
		if got, matched := direct.ClassifySummaryCell(index, recent); matched {
			if got != valuedomain.MembershipRecent {
				t.Fatalf("direct coordinate class=%d", got)
			}
			matchedIndex = index
			break
		}
	}
	if matchedIndex < 0 || fixture.values.CoordinateCount() < 2 {
		t.Fatal("direct coordinate match")
	}
	mismatchIndex := 0
	if mismatchIndex == matchedIndex {
		mismatchIndex = 1
	}
	if got, matched := direct.ClassifySummaryCell(mismatchIndex, recent); matched || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("direct receipt classified a different Value coordinate")
	}
	if got, matched := direct.ClassifySummaryCell(-1, recent); matched || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("direct receipt classified a negative Value coordinate")
	}
	if got, matched := direct.ClassifySummaryCell(fixture.values.CoordinateCount(), recent); matched || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("direct receipt classified an out-of-range Value coordinate")
	}
}
