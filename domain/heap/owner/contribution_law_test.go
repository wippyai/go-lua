package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

const heapContributionSource = `
local child = { value = 1 }
local record = { child = child, name = child }
return record
`

// sealedHeapSchema is the smallest constructible heap authority: one lowered
// module, sealed through the artifact-native mount seam this axis's own Mount
// hook uses. The contributor is read against the very authority the mount
// produces, so the two halves of the axis are exercised over one seal.
func sealedHeapSchema(t testing.TB) heapdomain.Schema {
	t.Helper()
	return sealedHeapSchemaFrom(t, heapContributionSource)
}

func sealedHeapSchemaFrom(t testing.TB, source string) heapdomain.Schema {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: "heap_contribution_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "heap_contribution_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mountedPrograms := linked.Project().Mounts()
	mounts := make([]heapdomain.ArtifactMount, mountedPrograms.Count())
	for index := 0; index < mountedPrograms.Count(); index++ {
		shard, shardOK := mountedPrograms.At(index)
		mounted, mountedOK := mountedPrograms.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mountedPrograms.ProgramID(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK || !programIDOK {
			t.Fatalf("mount %d has no artifact source", index)
		}
		artifact, failure := composite.CompileArtifactDetailed(mounted, receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("compile artifact %d: %v", index, failure)
		}
		var mountOK bool
		mounts[index], mountOK = heapdomain.NewArtifactMount(artifact, module, programID)
		if !mountOK {
			t.Fatalf("mount %d is not admitted", index)
		}
	}
	schema, failure := heapdomain.SealWithArtifacts(linked, mounts)
	if failure != heapdomain.SealFailureNone || !schema.Valid() {
		t.Fatalf("seal the heap authority: %v", failure)
	}
	return schema
}

// heapAlternatingLane stands for one completed solve's heap lane: it holds a
// fact at every second key and none at the rest. A contributor must publish the
// first as rows and the second as nothing at all, which is what makes the two
// absences a published column distinguishes visible to the laws below.
func heapAlternatingLane(schema heapdomain.Schema) heapowner.Lane {
	return func(key heapdomain.Key) (heapdomain.Value, bool) {
		index, indexed := schema.KeyIndex(key)
		if !indexed || index%2 == 1 {
			return heapdomain.Value{}, false
		}
		return schema.Bottom(), true
	}
}

// fillForeignColumns fills the slots this domain does not own. A publication's
// slot range is dense because every declared column has a writer, and this law
// drives one of them, so the columns the other four factors and the
// reachability axis fill are stood in for here rather than left as holes the
// seal would reject.
func fillForeignColumns(t testing.TB, builder *snapshot.Builder, schemaID identity.ContentID, owned uint32) {
	t.Helper()
	for slot := uint32(0); slot < uint32(composite.PublicationColumns()); slot++ {
		if slot == owned {
			continue
		}
		if err := snapshot.PutColumn(builder, snapshot.Axis[uint64, uint64]{SchemaID: schemaID, Slot: slot}, snapshot.Content[uint64, uint64]{}); err != nil {
			t.Fatalf("stand in for the column at slot %d: %v", slot, err)
		}
	}
}

// TestHeapContributionPublishesTheDeclaredColumn is the stitch on the heap
// side: the axis's declared output is projected into the published value's
// addressing, filled by this domain's own contributor, and read back through
// every outcome the read contract distinguishes. A key the seal covers and the
// lane held no fact for reads as a proven absence, which is the whole reason
// the contributor publishes a denominator rather than rows alone.
func TestHeapContributionPublishesTheDeclaredColumn(t *testing.T) {
	schema := sealedHeapSchema(t)
	denominator, members, sealed := heapowner.Denominator(schema)
	if !sealed || !denominator.Available() || len(members) != schema.KeyCount() {
		t.Fatalf("the sealed heap authority publishes no key universe: sealed=%t members=%d keys=%d", sealed, len(members), schema.KeyCount())
	}

	coverage, coverageOK := composite.PublicationCoverage("heap/facts")
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("heap/facts publishes coverage %d, not the total coverage its dense axis declares", coverage)
	}
	schemaID, schemaOK := composite.PublicationSchema()
	column, projected := composite.ProjectAxis[heapdomain.Key, heapdomain.Value]("heap/facts")
	if !schemaOK || !projected || !column.Available() {
		t.Fatal("the declared output heap/facts projects no address")
	}

	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	fillForeignColumns(t, &builder, schemaID, column.Slot)
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[heapdomain.Key, heapdomain.Value]{
		Denominator: denominator,
		Members:     members,
	}); err != nil {
		t.Fatalf("seal the heap column: %v", err)
	}
	published := 0
	if !heapowner.Contribute(schema, heapAlternatingLane(schema), func(key heapdomain.Key, fact heapdomain.Value) bool {
		published++
		return snapshot.SetRow(&builder, column, key, fact) == nil
	}) {
		t.Fatal("the heap contributor refused a lane of its own sealed authority")
	}
	if published == 0 {
		t.Fatal("the heap contributor published no row for a lane that holds facts")
	}
	publication, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	for index, key := range members {
		fact, status := snapshot.Read(&publication, column, key)
		if index%2 == 0 {
			if status != snapshot.ReadHit || !heapdomain.Equal(fact, schema.Bottom()) {
				t.Fatalf("key %d read back as %s, not the fact the lane held", index, status)
			}
			continue
		}
		if status != snapshot.ReadProvenAbsent {
			t.Fatalf("key %d read back as %s, not the proven absence its sealed universe covers", index, status)
		}
	}
	foreignKey := sealedHeapSchema(t)
	uncovered, uncoveredOK := foreignKey.KeyAt(0)
	if !uncoveredOK {
		t.Fatal("the second sealed authority issues no key")
	}
	if _, status := snapshot.Read(&publication, column, uncovered); status != snapshot.ReadMiss {
		t.Fatalf("a key of another authority read back as %s, not a miss", status)
	}
	mistyped := snapshot.Axis[heapdomain.Key, uint64]{SchemaID: schemaID, Slot: column.Slot}
	if _, status := snapshot.Read(&publication, mistyped, members[0]); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}

// TestHeapContributionIsDeterministic states that a contributor is a function
// of its authority and its lane. Two runs publish one key universe under one
// identity and one row sequence, so a publication is reproducible and a
// snapshot derived from a re-run is a snapshot of the same content.
func TestHeapContributionIsDeterministic(t *testing.T) {
	schema := sealedHeapSchema(t)
	lane := heapAlternatingLane(schema)

	firstDenominator, firstMembers, firstOK := heapowner.Denominator(schema)
	secondDenominator, secondMembers, secondOK := heapowner.Denominator(schema)
	if !firstOK || !secondOK || firstDenominator != secondDenominator {
		t.Fatal("two readings of one sealed authority name two key universes")
	}
	if len(firstMembers) != len(secondMembers) {
		t.Fatalf("two readings of one sealed authority cover %d and %d members", len(firstMembers), len(secondMembers))
	}
	for index := range firstMembers {
		if firstMembers[index] != secondMembers[index] {
			t.Fatalf("member %d differs between two readings of one sealed authority", index)
		}
	}

	var first, second []heapdomain.Key
	collect := func(rows *[]heapdomain.Key) func(heapdomain.Key, heapdomain.Value) bool {
		return func(key heapdomain.Key, _ heapdomain.Value) bool {
			*rows = append(*rows, key)
			return true
		}
	}
	if !heapowner.Contribute(schema, lane, collect(&first)) || !heapowner.Contribute(schema, lane, collect(&second)) {
		t.Fatal("the heap contributor refused a lane of its own sealed authority")
	}
	if len(first) != len(second) || len(first) == 0 {
		t.Fatalf("two contributions published %d and %d rows", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("row %d differs between two contributions of one lane", index)
		}
	}
}

// TestHeapKeyUniverseIsIdentifiedByItsMembers states what a denominator
// identity means. It is the identity of a key set, so two authorities that
// cover the same keys are total over one universe and share one membership
// authority, while two that cover different keys never do. A column sealed
// against it therefore proves an absence only against the very set it covers.
func TestHeapKeyUniverseIsIdentifiedByItsMembers(t *testing.T) {
	same, _, sameOK := heapowner.Denominator(sealedHeapSchema(t))
	repeated, _, repeatedOK := heapowner.Denominator(sealedHeapSchema(t))
	if !sameOK || !repeatedOK || same != repeated {
		t.Fatal("two authorities sealed from one source name two key universes")
	}
	wider, widerMembers, widerOK := heapowner.Denominator(sealedHeapSchemaFrom(t, `
local first = { value = 1 }
local second = { first = first }
local third = { second = second, first = first }
return third
`))
	if !widerOK {
		t.Fatal("the wider authority publishes no key universe")
	}
	if _, members, _ := heapowner.Denominator(sealedHeapSchema(t)); len(members) == len(widerMembers) {
		t.Fatal("the two sources seal one key count, so this law compares nothing")
	}
	if wider == same {
		t.Fatal("two authorities covering different keys name one key universe")
	}
}

// TestHeapContributionRefusesWhatItsAuthorityDoesNotOwn states the fence. A
// contributor publishes this authority's facts at this authority's keys; a fact
// of another seal is refused rather than written, so a consumer of the column
// can read a fact as owned by the authority the column is addressed under.
func TestHeapContributionRefusesWhatItsAuthorityDoesNotOwn(t *testing.T) {
	schema := sealedHeapSchema(t)
	foreign := sealedHeapSchema(t)
	if heapowner.Contribute(schema, func(heapdomain.Key) (heapdomain.Value, bool) {
		return foreign.Bottom(), true
	}, func(heapdomain.Key, heapdomain.Value) bool { return true }) {
		t.Fatal("the heap contributor published a fact of another sealed authority")
	}
	if heapowner.Contribute(schema, heapAlternatingLane(schema), nil) {
		t.Fatal("the heap contributor published rows with no writer to publish them to")
	}
	if heapowner.Contribute(schema, nil, func(heapdomain.Key, heapdomain.Value) bool { return true }) {
		t.Fatal("the heap contributor published rows with no lane to read them from")
	}
	if _, _, sealed := heapowner.Denominator(heapdomain.Schema{}); sealed {
		t.Fatal("an unsealed heap authority publishes a key universe")
	}
	if heapowner.Contribute(heapdomain.Schema{}, heapAlternatingLane(schema), func(heapdomain.Key, heapdomain.Value) bool { return true }) {
		t.Fatal("an unsealed heap authority published rows")
	}
}
