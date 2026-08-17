package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	denominatorcounts "github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	executionowner "github.com/wippyai/go-lua/domain/execution/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// The materializer laws are stated over the real path: a real Link, mounted
// through the phase every axis seals its own authority in, published by the
// real driver over lanes the law supplies. Nothing is stood in for, so what
// these laws hold is the composition a consumer of the analyzer receives.

const materializerSource = `
local child = { value = 1 }
local record = { child = child, name = child }
local function identity(value) return value end
local first = identity(record)
local second = identity(first)
return second
`

const materializerForeignSource = `
local first = { value = 1 }
local second = { first = first, count = 2 }
local function pair(value) return value, value end
local third = { second = second, first = first, label = "x" }
local left, right = pair(third)
return left, right
`

var (
	materializerStore      = identity.StoreID(7)
	materializerGeneration = identity.Generation(3)
	materializerUniverse   = identity.ContentID{0x5e, 0x11}
	materializerSubjects   = [2]identity.ContentID{{0xa1}, {0xa2}}
	materializerUnasked    = identity.ContentID{0xa9}
	materializerReached    = mountedExecutionPoint{Mount: identity.MountID{0x21}, Point: identity.ContentID{0x22}}
	materializerUnreached  = mountedExecutionPoint{Mount: identity.MountID{0x21}, Point: identity.ContentID{0x23}}
	materializerUnmounted  = mountedExecutionPoint{Mount: identity.MountID{0x31}, Point: identity.ContentID{0x32}}
)

// mountedRecord seals one Link and runs the mount phase over it. The record it
// returns is the phase's own output: every declared mount has sealed its
// authority into it and both post-mount derivations have run, which is the only
// record the materializer admits.
func mountedRecord(t testing.TB, name, source string) LinkInputs {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mounts := linked.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, mounts.Count())
	rows := make([]axis.MountedArtifact, mounts.Count())
	statics := make([]staticdomain.MountedArtifact, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mounts.ProgramID(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK || !programIDOK {
			t.Fatalf("mount %d has no artifact source", index)
		}
		artifact, failure := CompileArtifactDetailed(mounted, receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile artifact %d: %v", index, failure)
		}
		artifacts[index] = artifact
		rows[index] = axis.MountedArtifact{Artifact: artifact, ModuleKey: module, ProgramID: programID}
		statics[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if err != nil || types == nil {
		t.Fatalf("seal the type authority: %v", err)
	}
	inventory, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, statics)
	if err != nil || inventory == nil {
		t.Fatalf("seal the static authority: %v", err)
	}
	record, failure := MountLink(LinkInputs{Source: linked, Artifacts: rows, StaticAuthority: inventory})
	if failure.Available() {
		t.Fatalf("mount the Link: %v", failure)
	}
	return record
}

// materializerLanes is one completed solve as the laws state it: every factor
// lane holds a fact at every second member of its own authority and none at the
// rest, the reachability lane reaches one of its two mounted points, and each
// query family is asked at two subjects of which one carries a lane. The
// alternation is what makes the two absences a published column distinguishes
// visible on every column at once.
func materializerLanes(t testing.TB, record LinkInputs) LaneSet[mountedExecutionPoint] {
	t.Helper()
	valueSchema, packSchema, heapSchema := record.ValueSchema, record.PackSchema, record.HeapSchema
	callAlgebra, effectAlgebra := record.CallAlgebra, record.EffectAlgebra
	summaryLane := valueowner.Lane(func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
		index, indexed := valueSchema.CoordinateIndex(coordinate)
		if !indexed || index%2 == 1 {
			return valuedomain.Value{}, false
		}
		return valueSchema.Top(), true
	})
	root, rootOK := effectAlgebra.RootAt(0)
	if !rootOK {
		t.Fatal("the sealed effect authority issues no root")
	}
	return LaneSet[mountedExecutionPoint]{
		Value: summaryLane,
		Pack: func(root packdomain.Root) (packdomain.Value, bool) {
			order, ordered := packSchema.RootOrder(root)
			if !ordered || order%2 == 1 {
				return packdomain.Value{}, false
			}
			return packSchema.Bottom(), true
		},
		Heap: func(key heapdomain.Key) (heapdomain.Value, bool) {
			index, indexed := heapSchema.KeyIndex(key)
			if !indexed || index%2 == 1 {
				return heapdomain.Value{}, false
			}
			return heapSchema.Bottom(), true
		},
		Call: func(key calldomain.Key) (calldomain.Value, bool) {
			index, indexed := callAlgebra.KeyIndex(key)
			if !indexed || index%2 == 1 {
				return calldomain.Value{}, false
			}
			return callAlgebra.Bottom(), true
		},
		Effect: func(root effectfactor.Root) (effectfactor.Value, bool) {
			index, indexed := effectAlgebra.RootIndex(root)
			if !indexed || index%2 == 1 {
				return effectfactor.Value{}, false
			}
			return effectAlgebra.Bottom(), true
		},
		Execution: ExecutionLane[mountedExecutionPoint]{
			Universe: materializerUniverse,
			Points:   []mountedExecutionPoint{materializerReached, materializerUnreached},
			Reached:  func(point mountedExecutionPoint) bool { return point == materializerReached },
		},
		Counts: materializerCounts(t),
		ValueSummary: []SummarySubject{
			{Subject: materializerSubjects[0], Lane: summaryLane},
			{Subject: materializerSubjects[1]},
		},
		EffectExact: []ExactSubject{
			{Subject: materializerSubjects[0], Root: root, Lane: func(asked effectfactor.Root) (effectfactor.Value, bool) {
				if asked != root {
					return effectfactor.Value{}, false
				}
				return effectAlgebra.Bottom(), true
			}},
			{Subject: materializerSubjects[1], Root: root},
		},
	}
}

// materializerCounts is one complete set of owner-local relation counts. The
// neutral denominator column is total over the generated relation catalog, so
// every declaration carries a row here, including the ones a count of zero
// covers.
func materializerCounts(t testing.TB) []denominatorcounts.CountRows {
	t.Helper()
	entries := denominatorcounts.GeneratedRelationEntries()
	rows := make([]denominatorcounts.CountRow, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || !entry.EntryAvailable() {
			t.Fatal("the generated relation catalog holds an unavailable declaration")
		}
		row, ok := denominatorcounts.NewCountRow(entry.ID(), 0)
		if !ok {
			t.Fatalf("relation %q admits no count row", entry.Key())
		}
		rows = append(rows, row)
	}
	set, ok := denominatorcounts.NewCountRows(rows)
	if !ok || !denominatorcounts.GeneratedCountRowsComplete(set) {
		t.Fatal("the law supplies an incomplete relation count set")
	}
	return []denominatorcounts.CountRows{set}
}

// stateColumn holds one published column to the read contract and, when a
// second publication is supplied, to determinism. A member the lane held a fact
// for is a hit, every other member of the sealed universe is a proven absence,
// a key outside that universe is a miss, and a wrong value claim is invalid. The
// second publication answers every member exactly as the first does, which is
// what makes a publication a function of its authority and its lanes.
func stateColumn[K comparable, V any](t *testing.T, first, second *snapshot.Snapshot, output schema.Key, members []K, uncovered K, held func(index int) bool, equal func(V, V) bool) {
	t.Helper()
	if len(members) == 0 {
		t.Fatalf("column %q publishes an empty key universe", output)
	}
	column, projected := ProjectAxis[K, V](output)
	if !projected || !column.Available() {
		t.Fatalf("the declared output %q projects no address", output)
	}
	rows := 0
	for index, member := range members {
		fact, status := snapshot.Read(first, column, member)
		if held(index) {
			if status != snapshot.ReadHit {
				t.Fatalf("column %q member %d read back as %s, not the fact its lane held", output, index, status)
			}
			rows++
		} else if status != snapshot.ReadProvenAbsent {
			t.Fatalf("column %q member %d read back as %s, not the proven absence its sealed universe covers", output, index, status)
		}
		if second == nil {
			continue
		}
		repeated, repeatedStatus := snapshot.Read(second, column, member)
		if repeatedStatus != status {
			t.Fatalf("column %q member %d read back as %s and as %s in two publications of one lane set", output, index, status, repeatedStatus)
		}
		if status == snapshot.ReadHit && !equal(fact, repeated) {
			t.Fatalf("column %q member %d carries two facts in two publications of one lane set", output, index)
		}
	}
	if rows == 0 {
		t.Fatalf("column %q published no row for a lane that holds facts", output)
	}
	if _, status := snapshot.Read(first, column, uncovered); status != snapshot.ReadMiss {
		t.Fatalf("a key outside the universe of %q read back as %s, not a miss", output, status)
	}
	mistyped := snapshot.Axis[K, uint8]{SchemaID: column.SchemaID, Slot: column.Slot}
	if _, status := snapshot.Read(first, mistyped, members[0]); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim on %q read back as %s", output, status)
	}
}

// stateColumns holds every declared column of one publication to the read
// contract. Each column is read at its own domain's coordinates and its own
// domain's equality, which is what a consumer holds when it reads one.
func stateColumns(t *testing.T, record, foreign LinkInputs, first, second *snapshot.Snapshot) {
	t.Helper()
	even := func(index int) bool { return index%2 == 0 }
	all := func(int) bool { return true }

	stateColumn(t, first, second, "value/facts", valueMembersOf(t, record), valueMembersOf(t, foreign)[0], even, record.ValueSchema.Equal)
	stateColumn(t, first, second, "pack/facts", packMembersOf(t, record), packMembersOf(t, foreign)[0], even, record.PackSchema.Lattice().Equal)
	stateColumn(t, first, second, "heap/facts", heapMembersOf(t, record), heapMembersOf(t, foreign)[0], even, heapdomain.Equal)
	stateColumn(t, first, second, "call/facts", callMembersOf(t, record), callMembersOf(t, foreign)[0], even, record.CallAlgebra.Equal)
	stateColumn(t, first, second, "effect/facts", effectMembersOf(t, record), effectMembersOf(t, foreign)[0], even, record.EffectAlgebra.Equal)

	stateColumn(t, first, second, executionowner.OutputKey,
		[]mountedExecutionPoint{materializerReached, materializerUnreached}, materializerUnmounted,
		func(index int) bool { return index == 0 },
		func(left, right executionowner.Reachable) bool { return left == right })

	counts := materializerCounts(t)[0]
	relations := make([]schema.EntryID, 0, counts.Count())
	for index := 0; index < counts.Count(); index++ {
		row, ok := counts.At(index)
		if !ok {
			t.Fatalf("the relation count set holds no row at %d", index)
		}
		relations = append(relations, row.ID())
	}
	stateColumn(t, first, second, "denominator/counts", relations, schema.EntryID{0xff},
		all, func(left, right uint64) bool { return left == right })
}

// stateQueries holds every sealed query family of one publication to the read
// contract on its result column: the family opens under the identity the table
// sealed it as, a subject that carried a lane is answered, a subject asked
// without one is a proven absence, a subject the family was never asked at is a
// miss, and an answer claimed at another type opens nothing.
func stateQueries(t *testing.T, record LinkInputs, first, second *snapshot.Snapshot) {
	t.Helper()
	requests, ok := QueryRequests()
	if !ok || len(requests) == 0 {
		t.Fatal("the sealed table requests no query result columns")
	}
	answered := 0
	for _, request := range requests {
		switch request.Family {
		case QueryFamilyValueSummary:
			plan, opens := snapshot.OpenQuery[identity.ContentID, valuedomain.ValueSummaryObservation](first, request.ID)
			if !opens || plan.Slot != request.Slot {
				t.Fatalf("family %q opens no result column at the slot its projection requested", request.Family)
			}
			expected, folded := valueowner.FoldSummary(record.ValueSchema, materializerLanes(t, record).Value)
			if !folded {
				t.Fatal("the value summary fold refused a lane of its own sealed authority")
			}
			answer, status := snapshot.Query(first, plan, materializerSubjects[0])
			if status != snapshot.ReadHit || !valuedomain.EqualValueSummary(record.ValueSchema, answer, expected) {
				t.Fatalf("family %q answered its asked subject as %s, not with the answer its lane folds to", request.Family, status)
			}
			if second != nil {
				repeated, repeatedStatus := snapshot.Query(second, plan, materializerSubjects[0])
				if repeatedStatus != status || !valuedomain.EqualValueSummary(record.ValueSchema, answer, repeated) {
					t.Fatalf("family %q answers one subject twice over in two publications of one lane set", request.Family)
				}
			}
			if _, status := snapshot.Query(first, plan, materializerSubjects[1]); status != snapshot.ReadProvenAbsent {
				t.Fatalf("a subject of %q asked without a lane read back as %s, not a proven absence", request.Family, status)
			}
			if _, status := snapshot.Query(first, plan, materializerUnasked); status != snapshot.ReadMiss {
				t.Fatalf("a subject %q was never asked at read back as %s, not a miss", request.Family, status)
			}
			if _, opens := snapshot.OpenQuery[identity.ContentID, uint8](first, request.ID); opens {
				t.Fatalf("a wrong answer claim opened family %q", request.Family)
			}
			answered++
		case QueryFamilyEffectExact:
			plan, opens := snapshot.OpenQuery[identity.ContentID, effectfactor.EffectObservation](first, request.ID)
			if !opens || plan.Slot != request.Slot {
				t.Fatalf("family %q opens no result column at the slot its projection requested", request.Family)
			}
			lanes := materializerLanes(t, record)
			expected, folded := effectowner.FoldExact(record.EffectAlgebra, lanes.EffectExact[0].Root, lanes.EffectExact[0].Lane)
			if !folded {
				t.Fatal("the effect exact fold refused a lane of its own sealed authority")
			}
			answer, status := snapshot.Query(first, plan, materializerSubjects[0])
			if status != snapshot.ReadHit || !effectfactor.EqualEffect(answer, expected) {
				t.Fatalf("family %q answered its asked subject as %s, not with the answer its lane folds to", request.Family, status)
			}
			if second != nil {
				repeated, repeatedStatus := snapshot.Query(second, plan, materializerSubjects[0])
				if repeatedStatus != status || !effectfactor.EqualEffect(answer, repeated) {
					t.Fatalf("family %q answers one subject twice over in two publications of one lane set", request.Family)
				}
			}
			if _, status := snapshot.Query(first, plan, materializerSubjects[1]); status != snapshot.ReadProvenAbsent {
				t.Fatalf("a subject of %q asked without a lane read back as %s, not a proven absence", request.Family, status)
			}
			if _, status := snapshot.Query(first, plan, materializerUnasked); status != snapshot.ReadMiss {
				t.Fatalf("a subject %q was never asked at read back as %s, not a miss", request.Family, status)
			}
			answered++
		default:
			t.Fatalf("the sealed table answers family %q, which no law states", request.Family)
		}
	}
	if answered != len(requests) {
		t.Fatalf("%d of the %d sealed families were read back", answered, len(requests))
	}
}

// TestMaterializedPublicationAnswersTheWholeSealedCatalog is the end-to-end
// stitch: a real Link mounted through the phase that seals every axis's
// authority, published by the driver over one lane set, read back through every
// outcome the read contract distinguishes on every declared column and every
// sealed query family at once.
func TestMaterializedPublicationAnswersTheWholeSealedCatalog(t *testing.T) {
	record := mountedRecord(t, "materializer_law", materializerSource)
	foreign := mountedRecord(t, "materializer_foreign_law", materializerForeignSource)
	published, failure := Materialize(Materialization[mountedExecutionPoint]{
		Link:       record,
		Store:      materializerStore,
		Generation: materializerGeneration,
		Lanes:      materializerLanes(t, record),
	})
	if failure.Available() {
		t.Fatalf("the materializer refused a mounted record and a complete lane set: %v", failure)
	}
	if !published.Published() || published.Schema() != publicationSchemaOf(t) {
		t.Fatal("the publication is addressed under a schema other than the sealed table")
	}
	stateColumns(t, record, foreign, &published, nil)
	stateQueries(t, record, &published, nil)
}

// TestMaterializedPublicationFillsExactlyTheRequestedSlots is the completeness
// law. The published slot range is the two issuance sets and nothing else: every
// declared column and every sealed family's result column is filled, and no slot
// beside them is addressed, so a column silently skipped is a publication that
// does not seal rather than a snapshot with a hole in it.
func TestMaterializedPublicationFillsExactlyTheRequestedSlots(t *testing.T) {
	record := mountedRecord(t, "materializer_law", materializerSource)
	published, failure := Materialize(Materialization[mountedExecutionPoint]{
		Link:       record,
		Store:      materializerStore,
		Generation: materializerGeneration,
		Lanes:      materializerLanes(t, record),
	})
	if failure.Available() {
		t.Fatalf("the materializer refused a mounted record and a complete lane set: %v", failure)
	}
	columns, columnsOK := WriteRequests()
	queries, queriesOK := QueryRequests()
	if !columnsOK || !queriesOK {
		t.Fatal("the sealed table publishes no column plan")
	}
	if published.Columns() != len(columns)+len(queries) || published.Columns() != PublicationColumns() {
		t.Fatalf("%d slots were published for %d issued columns and %d result columns", published.Columns(), len(columns), len(queries))
	}
	for _, request := range columns {
		if _, addressed := PublicationCoverage(request.Output); !addressed {
			t.Fatalf("the published column %q states no coverage", request.Output)
		}
		if int(request.Slot) >= published.Columns() {
			t.Fatalf("column %q was issued at slot %d outside the %d published slots", request.Output, request.Slot, published.Columns())
		}
	}
	if published.Queries().Len() != len(queries) {
		t.Fatalf("%d families are answerable against a publication that materialized %d", published.Queries().Len(), len(queries))
	}
	for _, request := range queries {
		if !published.Queries().Published(request.ID) {
			t.Fatalf("family %q is not answerable against the publication that materialized it", request.Family)
		}
	}
}

// TestMaterializedPublicationIsAFunctionOfItsLanes is the determinism law. Two
// publications of one mounted record and one lane set answer every member of
// every column and every subject of every family identically, so a publication
// is reproducible and a consumer that re-runs the analyzer reads the same facts.
func TestMaterializedPublicationIsAFunctionOfItsLanes(t *testing.T) {
	record := mountedRecord(t, "materializer_law", materializerSource)
	foreign := mountedRecord(t, "materializer_foreign_law", materializerForeignSource)
	first, firstFailure := Materialize(Materialization[mountedExecutionPoint]{
		Link:       record,
		Store:      materializerStore,
		Generation: materializerGeneration,
		Lanes:      materializerLanes(t, record),
	})
	second, secondFailure := Materialize(Materialization[mountedExecutionPoint]{
		Link:       record,
		Store:      materializerStore,
		Generation: materializerGeneration,
		Lanes:      materializerLanes(t, record),
	})
	if firstFailure.Available() || secondFailure.Available() {
		t.Fatalf("the materializer refused one of two runs over one lane set: %v %v", firstFailure, secondFailure)
	}
	if first.Schema() != second.Schema() || first.Columns() != second.Columns() {
		t.Fatal("two publications of one lane set are addressed differently")
	}
	if first.Denominators().Len() != second.Denominators().Len() {
		t.Fatalf("two publications of one lane set seal %d and %d key universes", first.Denominators().Len(), second.Denominators().Len())
	}
	stateColumns(t, record, foreign, &first, &second)
	stateQueries(t, record, &first, &second)
}

// TestMaterializationIsWholeOrNothing is the fail-closed law. One refusing
// contributor, one absent lane, one incomplete count set, one family asked at no
// subject, or a record no mount phase produced leaves no snapshot at all: a
// publication is the whole catalog or none of it, and the verdict names the
// stage and the column or family that refused.
func TestMaterializationIsWholeOrNothing(t *testing.T) {
	record := mountedRecord(t, "materializer_law", materializerSource)
	foreign := mountedRecord(t, "materializer_foreign_law", materializerForeignSource)

	refusals := []struct {
		name   string
		stage  PublishStage
		output schema.Key
		family schema.Key
		lanes  func(LaneSet[mountedExecutionPoint]) LaneSet[mountedExecutionPoint]
	}{
		{
			name: "a lane holding a fact of another authority", stage: PublishStageColumn, output: "value/facts",
			lanes: func(lanes LaneSet[mountedExecutionPoint]) LaneSet[mountedExecutionPoint] {
				lanes.Value = func(valuedomain.Coordinate) (valuedomain.Value, bool) { return foreign.ValueSchema.Top(), true }
				return lanes
			},
		},
		{
			name: "a writer with no lane at all", stage: PublishStageColumn, output: "heap/facts",
			lanes: func(lanes LaneSet[mountedExecutionPoint]) LaneSet[mountedExecutionPoint] {
				lanes.Heap = nil
				return lanes
			},
		},
		{
			name: "a reachability lane that covers nothing", stage: PublishStageColumn, output: executionowner.OutputKey,
			lanes: func(lanes LaneSet[mountedExecutionPoint]) LaneSet[mountedExecutionPoint] {
				lanes.Execution = ExecutionLane[mountedExecutionPoint]{}
				return lanes
			},
		},
		{
			name: "an incomplete relation count set", stage: PublishStageColumn, output: "denominator/counts",
			lanes: func(lanes LaneSet[mountedExecutionPoint]) LaneSet[mountedExecutionPoint] {
				lanes.Counts = nil
				return lanes
			},
		},
		{
			name: "a family asked at no subject", stage: PublishStageQuery, family: QueryFamilyValueSummary,
			lanes: func(lanes LaneSet[mountedExecutionPoint]) LaneSet[mountedExecutionPoint] {
				lanes.ValueSummary = nil
				return lanes
			},
		},
	}
	for _, refusal := range refusals {
		published, failure := Materialize(Materialization[mountedExecutionPoint]{
			Link:       record,
			Store:      materializerStore,
			Generation: materializerGeneration,
			Lanes:      refusal.lanes(materializerLanes(t, record)),
		})
		if !failure.Available() || failure.Stage != refusal.stage {
			t.Fatalf("%s published at stage %v", refusal.name, failure)
		}
		if failure.Output != refusal.output || failure.Family != refusal.family {
			t.Fatalf("%s was blamed on column %q and family %q", refusal.name, failure.Output, failure.Family)
		}
		if published.Published() {
			t.Fatalf("%s left a published snapshot behind", refusal.name)
		}
	}

	unmounted := LinkInputs{Source: record.Source, Artifacts: record.Artifacts, StaticAuthority: record.StaticAuthority}
	published, failure := Materialize(Materialization[mountedExecutionPoint]{
		Link:       unmounted,
		Store:      materializerStore,
		Generation: materializerGeneration,
		Lanes:      materializerLanes(t, record),
	})
	if !failure.Available() || failure.Stage != PublishStageInput || published.Published() {
		t.Fatalf("a record no mount phase produced published at stage %v", failure)
	}
	published, failure = Materialize(Materialization[mountedExecutionPoint]{
		Link:  record,
		Store: materializerStore,
		Lanes: materializerLanes(t, record),
	})
	if !failure.Available() || failure.Stage != PublishStageInput || published.Published() {
		t.Fatalf("a publication with no generation to advance published at stage %v", failure)
	}
}

// publicationSchemaOf is the identity every published address carries.
func publicationSchemaOf(t *testing.T) identity.ContentID {
	t.Helper()
	schemaID, ok := PublicationSchema()
	if !ok || !schemaID.Available() {
		t.Fatal("the sealed table publishes no schema identity")
	}
	return schemaID
}

// The member readers below unwrap each contributor's published key universe.
// The members are the column's own denominator, so the laws read a column at
// exactly the keys its coverage authority sealed it over.
func valueMembersOf(t *testing.T, record LinkInputs) []valuedomain.Coordinate {
	t.Helper()
	_, members, sealed := valueowner.Denominator(record.ValueSchema)
	if !sealed || len(members) < 2 {
		t.Fatalf("the sealed value authority covers %d coordinates, so an alternating lane distinguishes nothing", len(members))
	}
	return members
}

func packMembersOf(t *testing.T, record LinkInputs) []packdomain.Root {
	t.Helper()
	_, members, sealed := packowner.Denominator(record.PackSchema)
	if !sealed || len(members) < 2 {
		t.Fatalf("the sealed pack authority covers %d roots, so an alternating lane distinguishes nothing", len(members))
	}
	return members
}

func heapMembersOf(t *testing.T, record LinkInputs) []heapdomain.Key {
	t.Helper()
	_, members, sealed := heapowner.Denominator(record.HeapSchema)
	if !sealed || len(members) < 2 {
		t.Fatalf("the sealed heap authority covers %d keys, so an alternating lane distinguishes nothing", len(members))
	}
	return members
}

func callMembersOf(t *testing.T, record LinkInputs) []calldomain.Key {
	t.Helper()
	_, members, sealed := callowner.Denominator(record.CallAlgebra)
	if !sealed || len(members) < 2 {
		t.Fatalf("the sealed call authority covers %d keys, so an alternating lane distinguishes nothing", len(members))
	}
	return members
}

func effectMembersOf(t *testing.T, record LinkInputs) []effectfactor.Root {
	t.Helper()
	_, members, sealed := effectowner.Denominator(record.EffectAlgebra)
	if !sealed || len(members) < 2 {
		t.Fatalf("the sealed effect authority covers %d roots, so an alternating lane distinguishes nothing", len(members))
	}
	return members
}
