package freshresult_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	targetvocabulary "github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// freshSubject is the source whose sole mounted host call declares a Target
// fresh result: coroutine.create acquires a FreshThread in its normal outcome.
const freshSubject = "local co = coroutine.create(function() end)\nreturn co\n"

type sealedFixture struct {
	linked *link.Link
	heaps  heapdomain.Schema
	values *valuedomain.Schema
	module identity.ContentID
	calls  []identity.ContentID
}

// sealFixture reaches the same sealed altitude the composed binding reads:
// one canonical artifact, Heap's fresh catalogue, and Value's detached
// fresh-result directory. It stops before construction, which is where the
// mounted solve begins.
func sealFixture(t *testing.T, name, source string) sealedFixture {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(contract, name+".lua", []byte(source))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !mountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []programmount.MountedArtifact{mount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal schemas heap=%s value=%s", heapFailure, valueFailure)
	}
	canonical := snapshot.Program()
	count, published := canonical.OccurrenceCount()
	if !published {
		t.Fatal("occurrence family is unpublished")
	}
	fixture := sealedFixture{linked: linked, heaps: heaps, values: values, module: module}
	for index := 0; index < count; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK {
			t.Fatalf("occurrence %d", index)
		}
		if occurrence.Kind() == programschema.OccurrenceCall {
			fixture.calls = append(fixture.calls, occurrence.ID())
		}
	}
	return fixture
}

// TestFreshResultOperandNamesTheHeapRootAndAnExistingResultCoordinate is the
// identity half of the rule's contract. Every admitted operand is redeemed
// from Heap's own fresh catalogue - the occurrence identity is the Heap key
// content ID, never a second minted identity - and the coordinate it writes is
// one the mounted CallResultSlot directory already owns. A row that named a
// coordinate outside that directory would be a fabricated Value address for a
// result the artifact never published.
func TestFreshResultOperandNamesTheHeapRootAndAnExistingResultCoordinate(t *testing.T) {
	fixture := sealFixture(t, "fresh_result_root", freshSubject)
	admitted := fixture.values.FreshResultCallCount()
	if admitted == 0 {
		t.Fatal("the fresh-result directory is empty; the subject call declares a Target fresh result")
	}

	freshKeys := make(map[heapdomain.Key]identity.ContentID, fixture.heaps.FreshCount())
	for index := 0; index < fixture.heaps.FreshCount(); index++ {
		content, key, ok := fixture.heaps.FreshAt(index)
		if !ok {
			t.Fatalf("heap fresh root %d", index)
		}
		freshKeys[key] = content
	}

	resultCoordinates := make(map[valuedomain.Coordinate]identity.ContentID, len(fixture.calls))
	for _, call := range fixture.calls {
		slot, slotOK := fixture.values.MountedCallResultSlotFor(fixture.module, call, 0)
		if !slotOK {
			continue
		}
		coordinate, coordinateOK := slot.Coordinate()
		if !coordinateOK {
			t.Fatalf("mounted result slot for call %v has no coordinate", call)
		}
		resultCoordinates[coordinate] = call
	}
	if len(resultCoordinates) == 0 {
		t.Fatal("no mounted call publishes a fixed result coordinate")
	}

	for index := 0; index < admitted; index++ {
		row, rowOK := fixture.values.FreshResultCallAt(index)
		key, keyOK := row.Key()
		content, contentOK := row.KeyID()
		coordinate, coordinateOK := row.Coordinate()
		if !rowOK || !keyOK || !contentOK || !coordinateOK {
			t.Fatalf("admitted fresh-result row %d is incomplete", index)
		}
		if !fixture.values.OwnsFreshResultCall(row) {
			t.Fatalf("row %d is not owned by the schema that sealed it", index)
		}
		if key.Kind() != heapdomain.RootAllocation {
			t.Fatalf("row %d key kind = %v, want a Heap root allocation", index, key.Kind())
		}
		application, outcomeResult, _, fresh := key.FreshResultID()
		if !fresh || !application.Available() || !outcomeResult.Available() {
			t.Fatalf("row %d key carries no fresh-result identity", index)
		}
		if got, found := freshKeys[key]; !found || got != content {
			t.Fatalf("row %d occurrence identity %v is not Heap's fresh catalogue content for its key", index, content)
		}
		if redeemed, redeemedOK := fixture.heaps.KeyForID(content); !redeemedOK || redeemed != key {
			t.Fatalf("row %d occurrence identity does not redeem to its own Heap key", index)
		}
		if resolved, resolvedOK := fixture.values.FreshResultCallFor(key); !resolvedOK || resolved != row {
			t.Fatalf("row %d does not round-trip through its Heap key", index)
		}
		if _, owned := resultCoordinates[coordinate]; !owned {
			t.Fatalf("row %d writes coordinate %v, which no mounted CallResultSlot owns", index, coordinate)
		}
	}
}

// TestFreshResultValueIsTheHeapAllocationRootValue states what the rule may
// stage. The fresh result is the Heap root's own presealed reference under the
// Recent role, not a Value the rule mints for the call: the two atoms are one
// atom. Exact remains the raw allocation alternative and is refused here, and
// an authored allocation root that is not a Target fresh result is refused
// outright, so a valid Heap root alone never enters this authority.
func TestFreshResultValueIsTheHeapAllocationRootValue(t *testing.T) {
	fixture := sealFixture(t, "fresh_result_value", freshSubject)
	admitted := fixture.values.FreshResultCallCount()
	if admitted == 0 {
		t.Fatal("the fresh-result directory is empty; the subject call declares a Target fresh result")
	}
	for index := 0; index < admitted; index++ {
		row, rowOK := fixture.values.FreshResultCallAt(index)
		key, keyOK := row.Key()
		if !rowOK || !keyOK {
			t.Fatalf("admitted fresh-result row %d is incomplete", index)
		}
		for _, role := range []materialization.Role{materialization.Recent, materialization.Summary} {
			atom, atomOK := fixture.values.FreshResultAtom(key, role)
			allocation, allocationOK := fixture.values.Allocation(key, role)
			if !atomOK || !allocationOK || atom != allocation {
				t.Fatalf("row %d %v atom = %v/%t, Heap root allocation atom = %v/%t; want one atom", index, role, atom, atomOK, allocation, allocationOK)
			}
			fact, factOK := fixture.values.FreshResultFact(key, role)
			singleton, singletonOK := fixture.values.Singleton(atom)
			if !factOK || !singletonOK || !fixture.values.Equal(fact, singleton) {
				t.Fatalf("row %d %v fact is not the singleton of the Heap root atom", index, role)
			}
			if fixture.values.ValueAtomCount(fact) != 1 {
				t.Fatalf("row %d %v fact carries %d atoms, want the one Heap root", index, role, fixture.values.ValueAtomCount(fact))
			}
		}
		if _, exact := fixture.values.FreshResultAtom(key, materialization.Exact); exact {
			t.Fatalf("row %d admits the Exact role; the raw allocation alternative is not a fresh result", index)
		}
	}

	authored := 0
	for index := 0; index < fixture.heaps.AllocationKeyCount(); index++ {
		key, keyOK := fixture.heaps.AllocationKeyAt(index)
		if !keyOK {
			t.Fatalf("allocation key %d", index)
		}
		if _, _, _, fresh := key.FreshResultID(); fresh {
			continue
		}
		authored++
		if _, atomOK := fixture.values.FreshResultAtom(key, materialization.Recent); atomOK {
			t.Fatalf("authored allocation root %d entered the fresh-result authority", index)
		}
		if _, rowOK := fixture.values.FreshResultCallFor(key); rowOK {
			t.Fatalf("authored allocation root %d resolved a fresh-result operand", index)
		}
	}
	if authored == 0 {
		t.Fatal("the subject sealed no authored allocation root to hold the negative half against")
	}
}

// TestFreshResultAdmitsOnlyTargetDeclaredFreshOperations is the widening half.
// Heap allocates one fresh root per mounted call and declared fresh operation,
// because the call's selected operation is decided by the Call fact and not by
// the artifact. Value therefore admits an operand for every such pair and for
// no other operation: a row whose operation declares no FreshResult would be a
// fresh Value fabricated for an operation that allocates nothing. The rows an
// unselected operation contributes are exactly these, and the transfer answers
// them with no candidate rather than a value.
func TestFreshResultAdmitsOnlyTargetDeclaredFreshOperations(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	fixture := sealFixture(t, "fresh_result_operations", freshSubject)
	admitted := fixture.values.FreshResultCallCount()
	if admitted == 0 {
		t.Fatal("the fresh-result directory is empty; the subject call declares a Target fresh result")
	}
	declared := make(map[targetvocabulary.Operation]bool)
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, operationOK := contract.Operations.OperationAt(index)
		if !operationOK {
			t.Fatalf("target operation %d", index)
		}
		for outcome := 0; outcome < contract.Operations.OutcomeCount(operation); outcome++ {
			if contract.Operations.FreshResultCount(operation, outcome) != 0 {
				declared[operation] = true
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("the sealed Target declares no fresh result at all")
	}

	subject, subjectOK := contract.Operations.Lookup(targetvocabulary.BindingSpec{
		Namespace: targetvocabulary.BindingModule, Owner: []string{"coroutine"}, Member: []string{"create"},
	})
	if !subjectOK || !declared[subject] {
		t.Fatalf("coroutine.create = %d/%t; the subject operation must declare a fresh result", subject, subjectOK)
	}

	perCoordinate := make(map[valuedomain.Coordinate]map[targetvocabulary.Operation]int)
	for index := 0; index < admitted; index++ {
		row, rowOK := fixture.values.FreshResultCallAt(index)
		operation, operationOK := row.Operation()
		coordinate, coordinateOK := row.Coordinate()
		if !rowOK || !operationOK || !coordinateOK {
			t.Fatalf("admitted fresh-result row %d is incomplete", index)
		}
		if !declared[operation] {
			t.Fatalf("row %d admits operation %d, which declares no Target fresh result", index, operation)
		}
		if perCoordinate[coordinate] == nil {
			perCoordinate[coordinate] = make(map[targetvocabulary.Operation]int, len(declared))
		}
		perCoordinate[coordinate][operation]++
	}
	for coordinate, operations := range perCoordinate {
		if len(operations) != len(declared) {
			t.Fatalf("coordinate %v carries %d fresh operations, want one per declared fresh operation (%d)", coordinate, len(operations), len(declared))
		}
		if operations[subject] != 1 {
			t.Fatalf("coordinate %v carries %d rows for the selected operation, want exactly one", coordinate, operations[subject])
		}
		for operation, count := range operations {
			if count != 1 {
				t.Fatalf("coordinate %v carries %d rows for operation %d, want one", coordinate, count, operation)
			}
		}
	}
}

// TestFreshResultRowsAreRefusedByAForeignValueSchema keeps the owner fence
// exact. Two independent seals of one source produce equal content; a row
// still belongs to the schema that admitted it, so neither the row nor its
// Heap key crosses.
func TestFreshResultRowsAreRefusedByAForeignValueSchema(t *testing.T) {
	first := sealFixture(t, "fresh_result_owner_first", freshSubject)
	second := sealFixture(t, "fresh_result_owner_second", freshSubject)
	if first.values.FreshResultCallCount() == 0 || first.values.FreshResultCallCount() != second.values.FreshResultCallCount() {
		t.Fatalf("fresh-result directories = %d/%d, want one equal non-empty pair", first.values.FreshResultCallCount(), second.values.FreshResultCallCount())
	}
	for index := 0; index < first.values.FreshResultCallCount(); index++ {
		row, rowOK := first.values.FreshResultCallAt(index)
		key, keyOK := row.Key()
		if !rowOK || !keyOK {
			t.Fatalf("admitted fresh-result row %d is incomplete", index)
		}
		if second.values.OwnsFreshResultCall(row) {
			t.Fatalf("a foreign Value schema owned row %d", index)
		}
		if _, resolved := second.values.FreshResultCallFor(key); resolved {
			t.Fatalf("a foreign Value schema resolved row %d through its Heap key", index)
		}
		if _, atomOK := second.values.FreshResultAtom(key, materialization.Recent); atomOK {
			t.Fatalf("a foreign Value schema issued the fresh atom of row %d", index)
		}
	}
}

// resourceSubject acquires two declared-lifecycle resources: the connection
// its first call creates and the transaction the second creates from it.
const resourceSubject = "local conn = resource.connect()\nlocal tx = resource.begin(conn)\nreturn conn, tx\n"

// TestAcquiredResourceResultsCarryDistinctHeapRoots is the typestate axis's
// root prerequisite stated as a law. A declared acquisition names the result
// slot that creates a resource; unless that result carries a fresh allocation
// identity of its own, two acquired resources are one address and no
// per-resource state can be keyed on either. The law therefore holds each
// acquiring call site to a Heap fresh root that no other call site and no
// other operation shares.
func TestAcquiredResourceResultsCarryDistinctHeapRoots(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	acquirers := map[string]targetvocabulary.Operation{}
	for _, member := range []string{"connect", "begin"} {
		operation, operationOK := contract.Operations.Lookup(targetvocabulary.BindingSpec{
			Namespace: targetvocabulary.BindingModule, Owner: []string{"resource"}, Member: []string{member},
		})
		if !operationOK {
			t.Fatalf("resource.%s is not a sealed operation", member)
		}
		fresh := 0
		for outcome := 0; outcome < contract.Operations.OutcomeCount(operation); outcome++ {
			fresh += contract.Operations.FreshResultCount(operation, outcome)
		}
		if fresh == 0 {
			t.Fatalf("resource.%s declares an acquisition and no fresh result; the acquired resource has no allocation identity", member)
		}
		acquirers[member] = operation
	}

	fixture := sealFixture(t, "resource_acquisition", resourceSubject)
	perOperation := make(map[targetvocabulary.Operation]map[heapdomain.Key]valuedomain.Coordinate)
	keys := make(map[heapdomain.Key]bool)
	for index := 0; index < fixture.values.FreshResultCallCount(); index++ {
		row, rowOK := fixture.values.FreshResultCallAt(index)
		operation, operationOK := row.Operation()
		key, keyOK := row.Key()
		coordinate, coordinateOK := row.Coordinate()
		if !rowOK || !operationOK || !keyOK || !coordinateOK {
			t.Fatalf("admitted fresh-result row %d is incomplete", index)
		}
		if keys[key] {
			t.Fatalf("row %d reuses a Heap fresh root another row already owns", index)
		}
		keys[key] = true
		if perOperation[operation] == nil {
			perOperation[operation] = make(map[heapdomain.Key]valuedomain.Coordinate)
		}
		perOperation[operation][key] = coordinate
	}

	for member, operation := range acquirers {
		rows := perOperation[operation]
		if len(rows) != 2 {
			t.Fatalf("resource.%s carries %d fresh roots, want one per call site in the subject", member, len(rows))
		}
		coordinates := make(map[valuedomain.Coordinate]bool, len(rows))
		for key, coordinate := range rows {
			atom, atomOK := fixture.values.FreshResultAtom(key, materialization.Recent)
			allocation, allocationOK := fixture.values.Allocation(key, materialization.Recent)
			if !atomOK || !allocationOK || atom != allocation {
				t.Fatalf("resource.%s fresh root %v has no Heap allocation atom", member, key)
			}
			coordinates[coordinate] = true
		}
		if len(coordinates) != 2 {
			t.Fatalf("resource.%s fresh roots write %d coordinates, want one per call site", member, len(coordinates))
		}
	}

	connection := perOperation[acquirers["connect"]]
	transaction := perOperation[acquirers["begin"]]
	for key := range connection {
		if _, shared := transaction[key]; shared {
			t.Fatalf("the connection and the transaction share Heap root %v; two acquired resources are one address", key)
		}
	}
}
