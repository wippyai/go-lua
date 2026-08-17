package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	"github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

const effectContributionSource = `
local function first(value) return value end
local function second(value) return first(value) end
local function third(value) return second(first(value)) end
return third
`

// sealedEffectAlgebra is the smallest constructible effect authority: one
// lowered module, sealed through the artifact-native mount seam this axis's own
// Mount hook uses, over the pack authority the algebra declares as its
// dependency. The contributor is read against the very authority the mount
// produces, so the two halves of the axis are exercised over one seal.
func sealedEffectAlgebra(t testing.TB) *effectfactor.Algebra {
	t.Helper()
	return sealedEffectAlgebraFrom(t, effectContributionSource)
}

func sealedEffectAlgebraFrom(t testing.TB, source string) *effectfactor.Algebra {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: "effect_contribution_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_contribution_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mountedPrograms := linked.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, mountedPrograms.Count())
	packMounts := make([]pack.ArtifactMount, mountedPrograms.Count())
	staticMounts := make([]staticdomain.MountedArtifact, mountedPrograms.Count())
	effectMounts := make([]effectfactor.MountedArtifact, mountedPrograms.Count())
	for index := 0; index < mountedPrograms.Count(); index++ {
		shard, shardOK := mountedPrograms.At(index)
		mounted, mountedOK := mountedPrograms.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mountedPrograms.ProgramID(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK || !programIDOK {
			t.Fatalf("mount %d has no artifact source", index)
		}
		artifact, failure := composite.CompileArtifactDetailed(mounted, receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile artifact %d: %v", index, failure)
		}
		artifacts[index] = artifact
		var mountOK bool
		packMounts[index], mountOK = pack.NewArtifactMount(artifact, module, programID)
		if !mountOK {
			t.Fatalf("mount %d is not admitted by the pack authority", index)
		}
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
		effectMounts[index] = effectfactor.MountedArtifact{ModuleKey: module, Artifact: artifact}
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if err != nil || types == nil {
		t.Fatalf("seal the type authority: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
	if err != nil || statics == nil {
		t.Fatalf("seal the static authority: %v", err)
	}
	packs, packsOK := pack.SealMountedArtifacts(linked, statics, packMounts)
	if !packsOK || packs == nil {
		t.Fatal("seal the pack authority")
	}
	algebra, sealed := effectfactor.NewWithMountedArtifacts(linked, packs, contract, effectMounts)
	if !sealed || algebra == nil || !algebra.Valid() {
		t.Fatal("seal the effect authority")
	}
	return algebra
}

// effectAlternatingLane stands for one completed solve's effect lane: it holds
// a fact at every second root and none at the rest. A contributor must publish
// the first as rows and the second as nothing at all, which is what makes the
// two absences a published column distinguishes visible to the laws below. The
// facts it holds are the two endpoints of the lattice, so a row read back is the
// fact the lane wrote at that root rather than any admitted value.
func effectAlternatingLane(algebra *effectfactor.Algebra) effectowner.Lane {
	return func(root effectfactor.Root) (effectfactor.Value, bool) {
		return effectLaneFact(algebra, root)
	}
}

// effectLaneFact is the lane's own statement at one root: the fact it holds
// there and whether it holds one at all, read by the laws to state what a row
// must carry.
func effectLaneFact(algebra *effectfactor.Algebra, root effectfactor.Root) (effectfactor.Value, bool) {
	index, indexed := algebra.RootIndex(root)
	if !indexed || index%2 == 1 {
		return effectfactor.Value{}, false
	}
	if index%4 == 0 {
		return algebra.Bottom(), true
	}
	return algebra.Top(), true
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

// effectPublication is one sealed publication of this domain's own column,
// held beside the authority and the lane it was filled from, so a law can state
// what the column serves against what the lane held.
type effectPublication struct {
	algebra   *effectfactor.Algebra
	lane      effectowner.Lane
	members   []effectfactor.Root
	column    snapshot.Axis[effectfactor.Root, effectfactor.Value]
	schemaID  identity.ContentID
	published int
	snapshot  snapshot.Snapshot
}

// publishEffectColumn drives this domain's contributor over one sealed
// authority and seals the publication its rows land in.
func publishEffectColumn(t testing.TB, algebra *effectfactor.Algebra) effectPublication {
	t.Helper()
	denominator, members, sealed := effectowner.Denominator(algebra)
	if !sealed || !denominator.Available() || len(members) != algebra.RootCount() {
		t.Fatalf("the sealed effect authority publishes no root universe: sealed=%t members=%d roots=%d", sealed, len(members), algebra.RootCount())
	}
	schemaID, schemaOK := composite.PublicationSchema()
	column, projected := composite.ProjectAxis[effectfactor.Root, effectfactor.Value]("effect/facts")
	if !schemaOK || !projected || !column.Available() {
		t.Fatal("the declared output effect/facts projects no address")
	}
	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	fillForeignColumns(t, &builder, schemaID, column.Slot)
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[effectfactor.Root, effectfactor.Value]{
		Denominator: denominator,
		Members:     members,
	}); err != nil {
		t.Fatalf("seal the effect column: %v", err)
	}
	lane := effectAlternatingLane(algebra)
	published := 0
	if !effectowner.Contribute(algebra, lane, func(root effectfactor.Root, fact effectfactor.Value) bool {
		published++
		return snapshot.SetRow(&builder, column, root, fact) == nil
	}) {
		t.Fatal("the effect contributor refused a lane of its own sealed authority")
	}
	if published == 0 {
		t.Fatal("the effect contributor published no row for a lane that holds facts")
	}
	sealedSnapshot, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}
	return effectPublication{
		algebra:   algebra,
		lane:      lane,
		members:   members,
		column:    column,
		schemaID:  schemaID,
		published: published,
		snapshot:  sealedSnapshot,
	}
}

// TestEffectContributionPublishesTheDeclaredColumn is the stitch on the effect
// side: the axis's declared output is projected into the published value's
// addressing, filled by this domain's own contributor, and read back through
// every outcome the read contract distinguishes. A root the seal covers and the
// lane held no fact for reads as a proven absence, which is the whole reason
// the contributor publishes a denominator rather than rows alone.
func TestEffectContributionPublishesTheDeclaredColumn(t *testing.T) {
	algebra := sealedEffectAlgebra(t)
	coverage, coverageOK := composite.PublicationCoverage("effect/facts")
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("effect/facts publishes coverage %d, not the total coverage its dense axis declares", coverage)
	}

	publication := publishEffectColumn(t, algebra)
	for index, root := range publication.members {
		fact, status := snapshot.Read(&publication.snapshot, publication.column, root)
		held, holds := effectLaneFact(algebra, root)
		if holds {
			if status != snapshot.ReadHit || !algebra.Equal(fact, held) {
				t.Fatalf("root %d read back as %s, not the fact the lane held", index, status)
			}
			continue
		}
		if status != snapshot.ReadProvenAbsent {
			t.Fatalf("root %d read back as %s, not the proven absence its sealed universe covers", index, status)
		}
	}
	foreign := sealedEffectAlgebra(t)
	uncovered, uncoveredOK := foreign.RootAt(0)
	if !uncoveredOK {
		t.Fatal("the second sealed authority issues no root")
	}
	if _, status := snapshot.Read(&publication.snapshot, publication.column, uncovered); status != snapshot.ReadMiss {
		t.Fatalf("a root of another authority read back as %s, not a miss", status)
	}
	mistyped := snapshot.Axis[effectfactor.Root, uint64]{SchemaID: publication.schemaID, Slot: publication.column.Slot}
	if _, status := snapshot.Read(&publication.snapshot, mistyped, publication.members[0]); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}

// TestEffectContributionIsDeterministic states that a contributor is a function
// of its authority and its lane. Two runs publish one root universe under one
// identity and one row sequence, so a publication is reproducible and a
// snapshot derived from a re-run is a snapshot of the same content.
func TestEffectContributionIsDeterministic(t *testing.T) {
	algebra := sealedEffectAlgebra(t)
	lane := effectAlternatingLane(algebra)

	firstDenominator, firstMembers, firstOK := effectowner.Denominator(algebra)
	secondDenominator, secondMembers, secondOK := effectowner.Denominator(algebra)
	if !firstOK || !secondOK || firstDenominator != secondDenominator {
		t.Fatal("two readings of one sealed authority name two root universes")
	}
	if len(firstMembers) != len(secondMembers) {
		t.Fatalf("two readings of one sealed authority cover %d and %d members", len(firstMembers), len(secondMembers))
	}
	for index := range firstMembers {
		if firstMembers[index] != secondMembers[index] {
			t.Fatalf("member %d differs between two readings of one sealed authority", index)
		}
	}

	var first, second []effectfactor.Root
	collect := func(rows *[]effectfactor.Root) func(effectfactor.Root, effectfactor.Value) bool {
		return func(root effectfactor.Root, _ effectfactor.Value) bool {
			*rows = append(*rows, root)
			return true
		}
	}
	if !effectowner.Contribute(algebra, lane, collect(&first)) || !effectowner.Contribute(algebra, lane, collect(&second)) {
		t.Fatal("the effect contributor refused a lane of its own sealed authority")
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

// TestEffectRootUniverseIsIdentifiedByItsMembers states what a denominator
// identity means. It is the identity of a root set, so two authorities that
// cover the same roots are total over one universe and share one membership
// authority, while two that cover different roots never do. A column sealed
// against it therefore proves an absence only against the very set it covers.
func TestEffectRootUniverseIsIdentifiedByItsMembers(t *testing.T) {
	same, _, sameOK := effectowner.Denominator(sealedEffectAlgebra(t))
	repeated, _, repeatedOK := effectowner.Denominator(sealedEffectAlgebra(t))
	if !sameOK || !repeatedOK || same != repeated {
		t.Fatal("two authorities sealed from one source name two root universes")
	}
	wider, widerMembers, widerOK := effectowner.Denominator(sealedEffectAlgebraFrom(t, `
local function alpha(value) return value end
local function beta(value) return alpha(value) end
local function gamma(value) return beta(alpha(value)) end
local function delta(value) return gamma(beta(value)) end
local function epsilon(value) return delta(gamma(value)) end
return epsilon
`))
	if !widerOK {
		t.Fatal("the wider authority publishes no root universe")
	}
	if _, members, _ := effectowner.Denominator(sealedEffectAlgebra(t)); len(members) == len(widerMembers) {
		t.Fatal("the two sources seal one root count, so this law compares nothing")
	}
	if wider == same {
		t.Fatal("two authorities covering different roots name one root universe")
	}
}

// TestEffectContributionRefusesWhatItsAuthorityDoesNotOwn states the fence. A
// contributor publishes this authority's facts at this authority's roots; a
// fact of another seal is refused rather than written, so a consumer of the
// column can read a fact as owned by the authority the column is addressed
// under.
func TestEffectContributionRefusesWhatItsAuthorityDoesNotOwn(t *testing.T) {
	algebra := sealedEffectAlgebra(t)
	foreign := sealedEffectAlgebra(t)
	if effectowner.Contribute(algebra, func(effectfactor.Root) (effectfactor.Value, bool) {
		return foreign.Bottom(), true
	}, func(effectfactor.Root, effectfactor.Value) bool { return true }) {
		t.Fatal("the effect contributor published a fact of another sealed authority")
	}
	if effectowner.Contribute(algebra, effectAlternatingLane(algebra), nil) {
		t.Fatal("the effect contributor published rows with no writer to publish them to")
	}
	if effectowner.Contribute(algebra, nil, func(effectfactor.Root, effectfactor.Value) bool { return true }) {
		t.Fatal("the effect contributor published rows with no lane to read them from")
	}
	if _, _, sealed := effectowner.Denominator(nil); sealed {
		t.Fatal("an unsealed effect authority publishes a root universe")
	}
	if effectowner.Contribute(nil, effectAlternatingLane(algebra), func(effectfactor.Root, effectfactor.Value) bool { return true }) {
		t.Fatal("an unsealed effect authority published rows")
	}
}

// TestEffectExactAnswerFoldsThePublishedRow states that the family's answer is
// a fold over the published column and not a second reading of the solve. The
// row is read back out of the sealed publication and folded by this domain's
// own fold; it must reach the answer the lane the column was filled from
// folds to, so serving the family from the column serves the same answer.
func TestEffectExactAnswerFoldsThePublishedRow(t *testing.T) {
	algebra := sealedEffectAlgebra(t)
	publication := publishEffectColumn(t, algebra)
	publishedLane := func(root effectfactor.Root) (effectfactor.Value, bool) {
		fact, status := snapshot.Read(&publication.snapshot, publication.column, root)
		if status != snapshot.ReadHit {
			return effectfactor.Value{}, false
		}
		return fact, true
	}
	for index, root := range publication.members {
		served, servedOK := effectowner.FoldExact(algebra, root, publishedLane)
		solved, solvedOK := effectowner.FoldExact(algebra, root, publication.lane)
		if !servedOK || !solvedOK {
			t.Fatalf("root %d folds to no answer: published=%t solved=%t", index, servedOK, solvedOK)
		}
		if !effectfactor.EqualEffect(served, solved) {
			t.Fatalf("root %d folds to one answer from the column and another from the lane", index)
		}
	}
}

// TestEffectExactAnswerDistinguishesAbsenceFromIgnorance states the four states
// the result column carries. A root the lane held a fact for folds to one
// present row; a root of the same universe the lane held none for folds to one
// row that is absent, which is an exact query that observed an empty cell; and a
// root the algebra never issued folds to no answer at all, which is the
// ignorance the universe never covered.
func TestEffectExactAnswerDistinguishesAbsenceFromIgnorance(t *testing.T) {
	algebra := sealedEffectAlgebra(t)
	lane := effectAlternatingLane(algebra)
	_, members, sealed := effectowner.Denominator(algebra)
	if !sealed || len(members) < 2 {
		t.Fatalf("the sealed effect authority covers %d roots, so this law compares nothing", len(members))
	}
	present, absent := 0, 0
	for index, root := range members {
		answer, folded := effectowner.FoldExact(algebra, root, lane)
		if !folded || !answer.Valid || answer.Rows != 1 {
			t.Fatalf("root %d folds to %d rows, not the one row an exact query observes", index, answer.Rows)
		}
		_, holds := effectLaneFact(algebra, root)
		if answer.Present != holds {
			t.Fatalf("root %d folds to presence %t against a lane that holds %t", index, answer.Present, holds)
		}
		if holds {
			present++
			continue
		}
		absent++
	}
	if present == 0 || absent == 0 {
		t.Fatalf("the lane holds facts at %d roots and none at %d, so this law states only one state", present, absent)
	}
	foreign := sealedEffectAlgebra(t)
	unissued, unissuedOK := foreign.RootAt(0)
	if !unissuedOK {
		t.Fatal("the second sealed authority issues no root")
	}
	answer, folded := effectowner.FoldExact(algebra, unissued, lane)
	if folded || answer.Valid || answer.Rows != 0 {
		t.Fatalf("a root of another authority folds to an answer of %d rows", answer.Rows)
	}
}

// TestEffectExactContributionPublishesOneAnswerPerSubject states the shape of
// the result column: the subject key is the materializer's, the answer is this
// domain's fold, and one contribution writes one answer at the subject it was
// asked at. A contribution with nowhere to publish is refused rather than
// folded into nothing.
func TestEffectExactContributionPublishesOneAnswerPerSubject(t *testing.T) {
	algebra := sealedEffectAlgebra(t)
	lane := effectAlternatingLane(algebra)
	root, issued := algebra.RootAt(0)
	if !issued {
		t.Fatal("the sealed effect authority issues no root")
	}
	expected, folded := effectowner.FoldExact(algebra, root, lane)
	if !folded {
		t.Fatal("the effect fold answers nothing at a root of its own sealed authority")
	}

	subjects := make([]uint64, 0, 1)
	answers := make([]effectfactor.EffectObservation, 0, 1)
	if !effectowner.ContributeExact(algebra, uint64(7), root, lane, func(subject uint64, answer effectfactor.EffectObservation) bool {
		subjects = append(subjects, subject)
		answers = append(answers, answer)
		return true
	}) {
		t.Fatal("the effect contributor refused a subject of its own sealed authority")
	}
	if len(subjects) != 1 || subjects[0] != 7 {
		t.Fatalf("one contribution published %d answers, and the first under subject %v", len(subjects), subjects)
	}
	if !effectfactor.EqualEffect(answers[0], expected) {
		t.Fatal("the published answer is not the answer the fold produced")
	}
	if effectowner.ContributeExact(algebra, uint64(7), root, lane, nil) {
		t.Fatal("the effect contributor published an answer with no writer to publish it to")
	}
}
