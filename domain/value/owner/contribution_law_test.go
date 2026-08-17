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
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

const valueContributionSource = `
local child = { value = 1 }
local record = { child = child, name = child }
return record
`

const valueContributionWiderSource = `
local first = { value = 1 }
local second = { first = first, count = 2 }
local third = { second = second, first = first, label = "x" }
return third
`

// sealedValueSchema is the smallest constructible value authority: one lowered
// module, sealed over its own heap family through the artifact-native mount
// seam this axis's own Mount hook uses. The contributor is read against the
// very authority the mount produces, so the two halves of the axis are
// exercised over one seal.
func sealedValueSchema(t testing.TB) *valuedomain.Schema {
	t.Helper()
	return sealedValueSchemaFrom(t, valueContributionSource)
}

func sealedValueSchemaFrom(t testing.TB, source string) *valuedomain.Schema {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: "value_contribution_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "value_contribution_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mountedPrograms := linked.Project().Mounts()
	heapMounts := make([]heapdomain.ArtifactMount, mountedPrograms.Count())
	valueMounts := make([]valuedomain.ArtifactMount, mountedPrograms.Count())
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
		heapMounts[index], mountOK = heapdomain.NewArtifactMount(artifact, module, programID)
		if !mountOK {
			t.Fatalf("heap mount %d is not admitted", index)
		}
		valueMounts[index], mountOK = valuedomain.NewArtifactMount(artifact, module, programID)
		if !mountOK {
			t.Fatalf("value mount %d is not admitted", index)
		}
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, heapMounts)
	if heapFailure != heapdomain.SealFailureNone || !heapSchema.Valid() {
		t.Fatalf("seal the heap authority the value universe is sealed over: %v", heapFailure)
	}
	schema, failure := valuedomain.SealWithFailure(linked, heapSchema, valueMounts)
	if failure != valuedomain.SealFailureNone || schema == nil || !schema.Valid() {
		t.Fatalf("seal the value authority: %v", failure)
	}
	return schema
}

// valueAlternatingLane stands for one completed solve's value lane: it holds a
// fact at every second coordinate and none at the rest. A contributor must
// publish the first as rows and the second as nothing at all, which is what
// makes the two absences a published column distinguishes visible to the laws
// below.
func valueAlternatingLane(schema *valuedomain.Schema) valueowner.Lane {
	return func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
		index, indexed := schema.CoordinateIndex(coordinate)
		if !indexed || index%2 == 1 {
			return valuedomain.Value{}, false
		}
		return schema.Top(), true
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

// publishValueColumn seals one lane into the column this axis declares and
// returns the publication and the address a consumer reads it at.
func publishValueColumn(t testing.TB, schema *valuedomain.Schema, lane valueowner.Lane) (snapshot.Snapshot, snapshot.Axis[valuedomain.Coordinate, valuedomain.Value]) {
	t.Helper()
	denominator, members, sealed := valueowner.Denominator(schema)
	if !sealed || !denominator.Available() || len(members) != schema.CoordinateCount() {
		t.Fatalf("the sealed value authority publishes no key universe: sealed=%t members=%d coordinates=%d", sealed, len(members), schema.CoordinateCount())
	}
	schemaID, schemaOK := composite.PublicationSchema()
	column, projected := composite.ProjectAxis[valuedomain.Coordinate, valuedomain.Value]("value/facts")
	if !schemaOK || !projected || !column.Available() {
		t.Fatal("the declared output value/facts projects no address")
	}
	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	fillForeignColumns(t, &builder, schemaID, column.Slot)
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[valuedomain.Coordinate, valuedomain.Value]{
		Denominator: denominator,
		Members:     members,
	}); err != nil {
		t.Fatalf("seal the value column: %v", err)
	}
	published := 0
	if !valueowner.Contribute(schema, lane, func(coordinate valuedomain.Coordinate, fact valuedomain.Value) bool {
		published++
		return snapshot.SetRow(&builder, column, coordinate, fact) == nil
	}) {
		t.Fatal("the value contributor refused a lane of its own sealed authority")
	}
	if published == 0 {
		t.Fatal("the value contributor published no row for a lane that holds facts")
	}
	publication, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}
	return publication, column
}

// TestValueContributionPublishesTheDeclaredColumn is the stitch on the value
// side: the axis's declared output is projected into the published value's
// addressing, filled by this domain's own contributor, and read back through
// every outcome the read contract distinguishes. A coordinate the seal covers
// and the lane held no fact for reads as a proven absence, which is the whole
// reason the contributor publishes a denominator rather than rows alone.
func TestValueContributionPublishesTheDeclaredColumn(t *testing.T) {
	schema := sealedValueSchema(t)
	coverage, coverageOK := composite.PublicationCoverage("value/facts")
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("value/facts publishes coverage %d, not the total coverage its dense axis declares", coverage)
	}
	publication, column := publishValueColumn(t, schema, valueAlternatingLane(schema))

	for index := 0; index < schema.CoordinateCount(); index++ {
		coordinate, issued := schema.CoordinateAt(index)
		if !issued {
			t.Fatalf("coordinate %d is not issued by its own sealed authority", index)
		}
		fact, status := snapshot.Read(&publication, column, coordinate)
		if index%2 == 0 {
			if status != snapshot.ReadHit || !schema.Equal(fact, schema.Top()) {
				t.Fatalf("coordinate %d read back as %s, not the fact the lane held", index, status)
			}
			continue
		}
		if status != snapshot.ReadProvenAbsent {
			t.Fatalf("coordinate %d read back as %s, not the proven absence its sealed universe covers", index, status)
		}
	}
	uncovered, uncoveredOK := sealedValueSchema(t).CoordinateAt(0)
	if !uncoveredOK {
		t.Fatal("the second sealed authority issues no coordinate")
	}
	if _, status := snapshot.Read(&publication, column, uncovered); status != snapshot.ReadMiss {
		t.Fatalf("a coordinate of another authority read back as %s, not a miss", status)
	}
	schemaID, _ := composite.PublicationSchema()
	mistyped := snapshot.Axis[valuedomain.Coordinate, uint64]{SchemaID: schemaID, Slot: column.Slot}
	first, firstOK := schema.CoordinateAt(0)
	if !firstOK {
		t.Fatal("the sealed authority issues no coordinate")
	}
	if _, status := snapshot.Read(&publication, mistyped, first); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}

// TestValueContributionIsDeterministic states that a contributor is a function
// of its authority and its lane. Two runs publish one key universe under one
// identity and one row sequence, so a publication is reproducible and a
// snapshot derived from a re-run is a snapshot of the same content.
func TestValueContributionIsDeterministic(t *testing.T) {
	schema := sealedValueSchema(t)
	lane := valueAlternatingLane(schema)

	firstDenominator, firstMembers, firstOK := valueowner.Denominator(schema)
	secondDenominator, secondMembers, secondOK := valueowner.Denominator(schema)
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

	var first, second []valuedomain.Coordinate
	collect := func(rows *[]valuedomain.Coordinate) func(valuedomain.Coordinate, valuedomain.Value) bool {
		return func(coordinate valuedomain.Coordinate, _ valuedomain.Value) bool {
			*rows = append(*rows, coordinate)
			return true
		}
	}
	if !valueowner.Contribute(schema, lane, collect(&first)) || !valueowner.Contribute(schema, lane, collect(&second)) {
		t.Fatal("the value contributor refused a lane of its own sealed authority")
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

// TestValueCoordinateUniverseIsIdentifiedByItsLink states what this
// denominator identity means. The value coordinate range is exactly the
// boundary value range of the Link the schema sealed it from, so two schemas of
// one Link are total over one universe and share one membership authority,
// while schemas of two Links never are.
func TestValueCoordinateUniverseIsIdentifiedByItsLink(t *testing.T) {
	same, _, sameOK := valueowner.Denominator(sealedValueSchema(t))
	repeated, _, repeatedOK := valueowner.Denominator(sealedValueSchema(t))
	if !sameOK || !repeatedOK || same != repeated {
		t.Fatal("two authorities sealed from one source name two key universes")
	}
	wider := sealedValueSchemaFrom(t, valueContributionWiderSource)
	widerDenominator, widerMembers, widerOK := valueowner.Denominator(wider)
	if !widerOK {
		t.Fatal("the wider authority publishes no key universe")
	}
	if len(widerMembers) == sealedValueSchema(t).CoordinateCount() {
		t.Fatal("the two sources seal one coordinate count, so this law compares nothing")
	}
	if widerDenominator == same {
		t.Fatal("two authorities covering different coordinates name one key universe")
	}
}

// TestValueContributionRefusesWhatItsAuthorityDoesNotOwn states the fence. A
// contributor publishes this authority's facts at this authority's
// coordinates; a fact of another seal is refused rather than written, so a
// consumer of the column can read a fact as owned by the authority the column
// is addressed under.
func TestValueContributionRefusesWhatItsAuthorityDoesNotOwn(t *testing.T) {
	schema := sealedValueSchema(t)
	foreign := sealedValueSchema(t)
	if valueowner.Contribute(schema, func(valuedomain.Coordinate) (valuedomain.Value, bool) {
		return foreign.Top(), true
	}, func(valuedomain.Coordinate, valuedomain.Value) bool { return true }) {
		t.Fatal("the value contributor published a fact of another sealed authority")
	}
	if valueowner.Contribute(schema, valueAlternatingLane(schema), nil) {
		t.Fatal("the value contributor published rows with no writer to publish them to")
	}
	if valueowner.Contribute(schema, nil, func(valuedomain.Coordinate, valuedomain.Value) bool { return true }) {
		t.Fatal("the value contributor published rows with no lane to read them from")
	}
	if _, _, sealed := valueowner.Denominator(nil); sealed {
		t.Fatal("an unsealed value authority publishes a key universe")
	}
	if valueowner.Contribute(nil, valueAlternatingLane(schema), func(valuedomain.Coordinate, valuedomain.Value) bool { return true }) {
		t.Fatal("an unsealed value authority published rows")
	}
}

// TestValueSummaryAnswerFoldsThePublishedRows is the flash-cut equivalence in
// the half that needs no engine. The value-summary family's answer is a fold
// over the value factor's facts; a published column holds those facts, so
// folding the column reproduces the answer folding the lane produces. A
// consumer moved from the query receipt to the published column therefore
// reads the same answer.
func TestValueSummaryAnswerFoldsThePublishedRows(t *testing.T) {
	schema := sealedValueSchema(t)
	lane := valueAlternatingLane(schema)
	publication, column := publishValueColumn(t, schema, lane)

	published := func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
		fact, status := snapshot.Read(&publication, column, coordinate)
		return fact, status == snapshot.ReadHit
	}
	fromColumn, columnFolded := valueowner.FoldSummary(schema, published)
	fromLane, laneFolded := valueowner.FoldSummary(schema, lane)
	if !columnFolded || !laneFolded {
		t.Fatal("the value summary fold refused a lane of its own sealed authority")
	}
	if !valuedomain.EqualValueSummary(schema, fromColumn, fromLane) {
		t.Fatal("the published column folds to an answer other than the one its lane folds to")
	}
}

// TestValueSummaryAnswerDistinguishesAbsenceFromIgnorance states the four-state
// contract on the result column. The answer opens at the schema's coordinate
// width and marks present exactly the coordinates the lane held a fact for, so
// a coordinate the solve proved nothing about stays distinguishable from one it
// proved a fact at, on the way out of the analyzer as well as inside it.
func TestValueSummaryAnswerDistinguishesAbsenceFromIgnorance(t *testing.T) {
	schema := sealedValueSchema(t)
	answer, folded := valueowner.FoldSummary(schema, valueAlternatingLane(schema))
	if !folded || !answer.Valid || answer.Rows != 1 {
		t.Fatalf("the folded answer is valid=%t over %d rows", answer.Valid, answer.Rows)
	}
	if len(answer.Values) != schema.CoordinateCount() || len(answer.Present) != schema.CoordinateCount() {
		t.Fatalf("the folded answer is %d values wide over %d coordinates", len(answer.Values), schema.CoordinateCount())
	}
	for index := range answer.Present {
		if answer.Present[index] != (index%2 == 0) {
			t.Fatalf("coordinate %d is marked present=%t against a lane that holds a fact there=%t", index, answer.Present[index], index%2 == 0)
		}
		if answer.Present[index] && !schema.Equal(answer.Values[index], schema.Top()) {
			t.Fatalf("coordinate %d folds to a value other than the fact the lane held", index)
		}
	}
	if _, folded := valueowner.FoldSummary(schema, nil); folded {
		t.Fatal("the value summary fold answered with no lane to read")
	}
	if _, folded := valueowner.FoldSummary(nil, valueAlternatingLane(schema)); folded {
		t.Fatal("the value summary fold answered on an unsealed authority")
	}
}

// TestValueSummaryContributionPublishesOneAnswerPerSubject states the shape of
// the result column's row. The subject a family is asked at belongs to the
// materializer and the answer belongs to this domain, so the contributor hands
// the writer exactly the pair and refuses when there is no writer to hand it
// to.
func TestValueSummaryContributionPublishesOneAnswerPerSubject(t *testing.T) {
	schema := sealedValueSchema(t)
	lane := valueAlternatingLane(schema)
	expected, folded := valueowner.FoldSummary(schema, lane)
	if !folded {
		t.Fatal("the value summary fold refused a lane of its own sealed authority")
	}
	subject := identity.ContentID{0x51}
	answers := make(map[identity.ContentID]valuedomain.ValueSummaryObservation)
	if !valueowner.ContributeSummary(schema, subject, lane, func(key identity.ContentID, answer valuedomain.ValueSummaryObservation) bool {
		answers[key] = answer
		return true
	}) {
		t.Fatal("the value summary contributor published no answer for its subject")
	}
	if len(answers) != 1 || !valuedomain.EqualValueSummary(schema, answers[subject], expected) {
		t.Fatalf("%d answers were published, and the subject's is not the one its lane folds to", len(answers))
	}
	if valueowner.ContributeSummary(schema, subject, lane, nil) {
		t.Fatal("the value summary contributor published an answer with no writer to publish it to")
	}
	if valueowner.ContributeSummary(schema, subject, nil, func(identity.ContentID, valuedomain.ValueSummaryObservation) bool { return true }) {
		t.Fatal("the value summary contributor published an answer with no lane to fold")
	}
}
