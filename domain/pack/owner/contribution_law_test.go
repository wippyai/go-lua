package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

const packContributionSource = `
local function identity(value) return value end
local first = identity(1)
local second = identity(first)
return second
`

// sealedPackSchema is the smallest constructible pack authority: one lowered
// module carrying more than one pack-valued occurrence, sealed through the
// artifact-native mount seam this axis's own Mount hook uses. The pack seal
// reads its mounted value substitutions from a static inventory, so the type
// and static authorities of the same Link are sealed first and the contributor
// is read against the very authority that chain produces.
func sealedPackSchema(t testing.TB) *packdomain.Schema {
	t.Helper()
	return sealedPackSchemaFrom(t, packContributionSource)
}

func sealedPackSchemaFrom(t testing.TB, source string) *packdomain.Schema {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: "pack_contribution_law.lua", Text: []byte(source)})
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
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_contribution_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mountedPrograms := linked.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, mountedPrograms.Count())
	statics := make([]staticdomain.MountedArtifact, mountedPrograms.Count())
	mounts := make([]packdomain.ArtifactMount, mountedPrograms.Count())
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
		artifacts[index] = artifact
		statics[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
		var mountOK bool
		mounts[index], mountOK = packdomain.NewArtifactMount(artifact, module, programID)
		if !mountOK {
			t.Fatalf("mount %d is not admitted", index)
		}
	}
	types, typesErr := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if typesErr != nil || types == nil {
		t.Fatalf("seal the type authority: %v", typesErr)
	}
	inventory, _, staticErr := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, statics)
	if staticErr != nil || inventory == nil {
		t.Fatalf("seal the static authority: %v", staticErr)
	}
	schema, sealed := packdomain.SealMountedArtifacts(linked, inventory, mounts)
	if !sealed || schema == nil {
		t.Fatal("seal the pack authority")
	}
	return schema
}

// packAlternatingLane stands for one completed solve's pack lane: it holds a
// fact at every second root and none at the rest. A contributor must publish
// the first as rows and the second as nothing at all, which is what makes the
// two absences a published column distinguishes visible to the laws below.
func packAlternatingLane(schema *packdomain.Schema) packowner.Lane {
	return func(root packdomain.Root) (packdomain.Value, bool) {
		order, ordered := schema.RootOrder(root)
		if !ordered || order%2 == 1 {
			return packdomain.Value{}, false
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

// TestPackContributionPublishesTheDeclaredColumn is the stitch on the pack
// side: the axis's declared output is projected into the published value's
// addressing, filled by this domain's own contributor, and read back through
// every outcome the read contract distinguishes. A root the seal covers and the
// lane held no fact for reads as a proven absence, which is the whole reason
// the contributor publishes a denominator rather than rows alone.
func TestPackContributionPublishesTheDeclaredColumn(t *testing.T) {
	schema := sealedPackSchema(t)
	denominator, members, sealed := packowner.Denominator(schema)
	if !sealed || !denominator.Available() || len(members) != schema.RootCount() {
		t.Fatalf("the sealed pack authority publishes no key universe: sealed=%t members=%d roots=%d", sealed, len(members), schema.RootCount())
	}
	if len(members) < 2 {
		t.Fatalf("the fixture seals %d roots, so an alternating lane distinguishes nothing", len(members))
	}

	coverage, coverageOK := composite.PublicationCoverage("pack/facts")
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("pack/facts publishes coverage %d, not the total coverage its dense axis declares", coverage)
	}
	schemaID, schemaOK := composite.PublicationSchema()
	column, projected := composite.ProjectAxis[packdomain.Root, packdomain.Value]("pack/facts")
	if !schemaOK || !projected || !column.Available() {
		t.Fatal("the declared output pack/facts projects no address")
	}

	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	fillForeignColumns(t, &builder, schemaID, column.Slot)
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[packdomain.Root, packdomain.Value]{
		Denominator: denominator,
		Members:     members,
	}); err != nil {
		t.Fatalf("seal the pack column: %v", err)
	}
	published := 0
	if !packowner.Contribute(schema, packAlternatingLane(schema), func(root packdomain.Root, fact packdomain.Value) bool {
		published++
		return snapshot.SetRow(&builder, column, root, fact) == nil
	}) {
		t.Fatal("the pack contributor refused a lane of its own sealed authority")
	}
	if published == 0 {
		t.Fatal("the pack contributor published no row for a lane that holds facts")
	}
	publication, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	equal := schema.Lattice().Equal
	for index, root := range members {
		fact, status := snapshot.Read(&publication, column, root)
		if index%2 == 0 {
			if status != snapshot.ReadHit || !equal(fact, schema.Bottom()) {
				t.Fatalf("root %d read back as %s, not the fact the lane held", index, status)
			}
			continue
		}
		if status != snapshot.ReadProvenAbsent {
			t.Fatalf("root %d read back as %s, not the proven absence its sealed universe covers", index, status)
		}
	}
	foreignRoot := sealedPackSchema(t)
	uncovered, uncoveredOK := foreignRoot.RootAt(0)
	if !uncoveredOK {
		t.Fatal("the second sealed authority issues no root")
	}
	if _, status := snapshot.Read(&publication, column, uncovered); status != snapshot.ReadMiss {
		t.Fatalf("a root of another authority read back as %s, not a miss", status)
	}
	mistyped := snapshot.Axis[packdomain.Root, uint64]{SchemaID: schemaID, Slot: column.Slot}
	if _, status := snapshot.Read(&publication, mistyped, members[0]); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}

// TestPackContributionIsDeterministic states that a contributor is a function
// of its authority and its lane. Two runs publish one key universe under one
// identity and one row sequence, so a publication is reproducible and a
// snapshot derived from a re-run is a snapshot of the same content.
func TestPackContributionIsDeterministic(t *testing.T) {
	schema := sealedPackSchema(t)
	lane := packAlternatingLane(schema)

	firstDenominator, firstMembers, firstOK := packowner.Denominator(schema)
	secondDenominator, secondMembers, secondOK := packowner.Denominator(schema)
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

	var first, second []packdomain.Root
	collect := func(rows *[]packdomain.Root) func(packdomain.Root, packdomain.Value) bool {
		return func(root packdomain.Root, _ packdomain.Value) bool {
			*rows = append(*rows, root)
			return true
		}
	}
	if !packowner.Contribute(schema, lane, collect(&first)) || !packowner.Contribute(schema, lane, collect(&second)) {
		t.Fatal("the pack contributor refused a lane of its own sealed authority")
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

// TestPackKeyUniverseIsIdentifiedByItsMembers states what a denominator
// identity means. It is the identity of a root set, so two authorities that
// cover the same roots are total over one universe and share one membership
// authority, while two that cover different roots never do. A column sealed
// against it therefore proves an absence only against the very set it covers.
func TestPackKeyUniverseIsIdentifiedByItsMembers(t *testing.T) {
	same, _, sameOK := packowner.Denominator(sealedPackSchema(t))
	repeated, _, repeatedOK := packowner.Denominator(sealedPackSchema(t))
	if !sameOK || !repeatedOK || same != repeated {
		t.Fatal("two authorities sealed from one source name two key universes")
	}
	wider, widerMembers, widerOK := packowner.Denominator(sealedPackSchemaFrom(t, `
local function identity(value) return value end
local function twice(value) return identity(identity(value)) end
local first = twice(1)
local second = identity(first)
return second
`))
	if !widerOK {
		t.Fatal("the wider authority publishes no key universe")
	}
	if _, members, _ := packowner.Denominator(sealedPackSchema(t)); len(members) == len(widerMembers) {
		t.Fatal("the two sources seal one root count, so this law compares nothing")
	}
	if wider == same {
		t.Fatal("two authorities covering different roots name one key universe")
	}
}

// TestPackContributionRefusesWhatItsAuthorityDoesNotOwn states the fence. A
// contributor publishes this authority's facts at this authority's roots; a
// fact of another seal is refused rather than written, so a consumer of the
// column can read a fact as owned by the authority the column is addressed
// under.
func TestPackContributionRefusesWhatItsAuthorityDoesNotOwn(t *testing.T) {
	schema := sealedPackSchema(t)
	foreign := sealedPackSchema(t)
	if packowner.Contribute(schema, func(packdomain.Root) (packdomain.Value, bool) {
		return foreign.Bottom(), true
	}, func(packdomain.Root, packdomain.Value) bool { return true }) {
		t.Fatal("the pack contributor published a fact of another sealed authority")
	}
	if packowner.Contribute(schema, packAlternatingLane(schema), nil) {
		t.Fatal("the pack contributor published rows with no writer to publish them to")
	}
	if packowner.Contribute(schema, nil, func(packdomain.Root, packdomain.Value) bool { return true }) {
		t.Fatal("the pack contributor published rows with no lane to read them from")
	}
	if _, _, sealed := packowner.Denominator(nil); sealed {
		t.Fatal("an unsealed pack authority publishes a key universe")
	}
	if packowner.Contribute(nil, packAlternatingLane(schema), func(packdomain.Root, packdomain.Value) bool { return true }) {
		t.Fatal("an unsealed pack authority published rows")
	}
}
