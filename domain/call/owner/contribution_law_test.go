package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/composite"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

const callContributionSource = `
local function identity(value) return value end
local first = identity(1)
local second = identity(first)
return second
`

// sealedCallAlgebra is the smallest constructible call authority: one lowered
// module carrying more than one call site, sealed through the artifact-native
// mount seam this axis's own Mount hook uses. The contributor is read against
// the very authority the mount produces, so the two halves of the axis are
// exercised over one seal.
func sealedCallAlgebra(t testing.TB) *calldomain.Algebra {
	t.Helper()
	return sealedCallAlgebraFrom(t, callContributionSource)
}

func sealedCallAlgebraFrom(t testing.TB, source string) *calldomain.Algebra {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: "call_contribution_law.lua", Text: []byte(source)})
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
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "call_contribution_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mountedPrograms := linked.Project().Mounts()
	mounts := make([]calldomain.MountedArtifact, mountedPrograms.Count())
	for index := 0; index < mountedPrograms.Count(); index++ {
		shard, shardOK := mountedPrograms.At(index)
		mounted, mountedOK := mountedPrograms.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK {
			t.Fatalf("mount %d has no artifact source", index)
		}
		artifact, failure := composite.CompileArtifactDetailed(mounted, receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("compile artifact %d: %v", index, failure)
		}
		mounts[index] = calldomain.MountedArtifact{ModuleKey: module, Artifact: artifact}
	}
	algebra, sealed := calldomain.NewWithMountedArtifacts(linked, mounts)
	if !sealed || algebra == nil || !algebra.Valid() {
		t.Fatal("seal the call authority")
	}
	return algebra
}

// callAlternatingLane stands for one completed solve's call lane: it holds a
// fact at every second key and none at the rest. A contributor must publish the
// first as rows and the second as nothing at all, which is what makes the two
// absences a published column distinguishes visible to the laws below.
func callAlternatingLane(algebra *calldomain.Algebra) callowner.Lane {
	return func(key calldomain.Key) (calldomain.Value, bool) {
		index, indexed := algebra.KeyIndex(key)
		if !indexed || index%2 == 1 {
			return calldomain.Value{}, false
		}
		return algebra.Bottom(), true
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

// TestCallContributionPublishesTheDeclaredColumn is the stitch on the call
// side: the axis's declared output is projected into the published value's
// addressing, filled by this domain's own contributor, and read back through
// every outcome the read contract distinguishes. A key the seal covers and the
// lane held no fact for reads as a proven absence, which is the whole reason
// the contributor publishes a denominator rather than rows alone.
func TestCallContributionPublishesTheDeclaredColumn(t *testing.T) {
	algebra := sealedCallAlgebra(t)
	denominator, members, sealed := callowner.Denominator(algebra)
	if !sealed || !denominator.Available() || len(members) != algebra.KeyCount() {
		t.Fatalf("the sealed call authority publishes no key universe: sealed=%t members=%d keys=%d", sealed, len(members), algebra.KeyCount())
	}
	if len(members) < 2 {
		t.Fatalf("the fixture seals %d keys, so an alternating lane distinguishes nothing", len(members))
	}

	coverage, coverageOK := composite.PublicationCoverage("call/facts")
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("call/facts publishes coverage %d, not the total coverage its dense axis declares", coverage)
	}
	schemaID, schemaOK := composite.PublicationSchema()
	column, projected := composite.ProjectAxis[calldomain.Key, calldomain.Value]("call/facts")
	if !schemaOK || !projected || !column.Available() {
		t.Fatal("the declared output call/facts projects no address")
	}

	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	fillForeignColumns(t, &builder, schemaID, column.Slot)
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[calldomain.Key, calldomain.Value]{
		Denominator: denominator,
		Members:     members,
	}); err != nil {
		t.Fatalf("seal the call column: %v", err)
	}
	published := 0
	if !callowner.Contribute(algebra, callAlternatingLane(algebra), func(key calldomain.Key, fact calldomain.Value) bool {
		published++
		return snapshot.SetRow(&builder, column, key, fact) == nil
	}) {
		t.Fatal("the call contributor refused a lane of its own sealed authority")
	}
	if published == 0 {
		t.Fatal("the call contributor published no row for a lane that holds facts")
	}
	publication, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	for index, key := range members {
		fact, status := snapshot.Read(&publication, column, key)
		if index%2 == 0 {
			if status != snapshot.ReadHit || !algebra.Equal(fact, algebra.Bottom()) {
				t.Fatalf("key %d read back as %s, not the fact the lane held", index, status)
			}
			continue
		}
		if status != snapshot.ReadProvenAbsent {
			t.Fatalf("key %d read back as %s, not the proven absence its sealed universe covers", index, status)
		}
	}
	foreignKey := sealedCallAlgebra(t)
	uncovered, uncoveredOK := foreignKey.KeyAt(0)
	if !uncoveredOK {
		t.Fatal("the second sealed authority issues no key")
	}
	if _, status := snapshot.Read(&publication, column, uncovered); status != snapshot.ReadMiss {
		t.Fatalf("a key of another authority read back as %s, not a miss", status)
	}
	mistyped := snapshot.Axis[calldomain.Key, uint64]{SchemaID: schemaID, Slot: column.Slot}
	if _, status := snapshot.Read(&publication, mistyped, members[0]); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}

// TestCallContributionIsDeterministic states that a contributor is a function
// of its authority and its lane. Two runs publish one key universe under one
// identity and one row sequence, so a publication is reproducible and a
// snapshot derived from a re-run is a snapshot of the same content.
func TestCallContributionIsDeterministic(t *testing.T) {
	algebra := sealedCallAlgebra(t)
	lane := callAlternatingLane(algebra)

	firstDenominator, firstMembers, firstOK := callowner.Denominator(algebra)
	secondDenominator, secondMembers, secondOK := callowner.Denominator(algebra)
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

	var first, second []calldomain.Key
	collect := func(rows *[]calldomain.Key) func(calldomain.Key, calldomain.Value) bool {
		return func(key calldomain.Key, _ calldomain.Value) bool {
			*rows = append(*rows, key)
			return true
		}
	}
	if !callowner.Contribute(algebra, lane, collect(&first)) || !callowner.Contribute(algebra, lane, collect(&second)) {
		t.Fatal("the call contributor refused a lane of its own sealed authority")
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

// TestCallKeyUniverseIsIdentifiedByItsMembers states what a denominator
// identity means. It is the identity of a key set, so two authorities that
// cover the same keys are total over one universe and share one membership
// authority, while two that cover different keys never do. A column sealed
// against it therefore proves an absence only against the very set it covers.
func TestCallKeyUniverseIsIdentifiedByItsMembers(t *testing.T) {
	same, _, sameOK := callowner.Denominator(sealedCallAlgebra(t))
	repeated, _, repeatedOK := callowner.Denominator(sealedCallAlgebra(t))
	if !sameOK || !repeatedOK || same != repeated {
		t.Fatal("two authorities sealed from one source name two key universes")
	}
	wider, widerMembers, widerOK := callowner.Denominator(sealedCallAlgebraFrom(t, `
local function identity(value) return value end
local function twice(value) return identity(identity(value)) end
local first = twice(1)
local second = identity(first)
return second
`))
	if !widerOK {
		t.Fatal("the wider authority publishes no key universe")
	}
	if _, members, _ := callowner.Denominator(sealedCallAlgebra(t)); len(members) == len(widerMembers) {
		t.Fatal("the two sources seal one key count, so this law compares nothing")
	}
	if wider == same {
		t.Fatal("two authorities covering different keys name one key universe")
	}
}

// TestCallContributionRefusesWhatItsAuthorityDoesNotOwn states the fence. A
// contributor publishes this authority's facts at this authority's keys; a fact
// of another seal is refused rather than written, so a consumer of the column
// can read a fact as owned by the authority the column is addressed under.
func TestCallContributionRefusesWhatItsAuthorityDoesNotOwn(t *testing.T) {
	algebra := sealedCallAlgebra(t)
	foreign := sealedCallAlgebra(t)
	if callowner.Contribute(algebra, func(calldomain.Key) (calldomain.Value, bool) {
		return foreign.Bottom(), true
	}, func(calldomain.Key, calldomain.Value) bool { return true }) {
		t.Fatal("the call contributor published a fact of another sealed authority")
	}
	if callowner.Contribute(algebra, callAlternatingLane(algebra), nil) {
		t.Fatal("the call contributor published rows with no writer to publish them to")
	}
	if callowner.Contribute(algebra, nil, func(calldomain.Key, calldomain.Value) bool { return true }) {
		t.Fatal("the call contributor published rows with no lane to read them from")
	}
	if _, _, sealed := callowner.Denominator(nil); sealed {
		t.Fatal("an unsealed call authority publishes a key universe")
	}
	if callowner.Contribute(nil, callAlternatingLane(algebra), func(calldomain.Key, calldomain.Value) bool { return true }) {
		t.Fatal("an unsealed call authority published rows")
	}
}
