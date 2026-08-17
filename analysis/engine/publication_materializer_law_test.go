// publication_materializer_law_test.go states the publication driver's laws
// against builder-constructed snapshots. The driver is inert: no production
// solve publishes through it yet, so every publication these laws read is one
// this file built.

package engine

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// The materializer law fixture publishes one query family over four columns:
// the execution-reachability gate its subjects are admitted by, the two
// declared inputs its fold reads, and the result column its answers are
// written into. The result column follows the axis columns in the dense slot
// range, exactly as the sealed table orders them.
const (
	lawReachOutput  schema.Key = "law/execution-reachability"
	lawInputAOutput schema.Key = "law/input-a"
	lawInputBOutput schema.Key = "law/input-b"
	lawResultOutput schema.Key = "law/result"
	lawEngineWriter schema.Key = "law/engine"
)

var (
	lawFoldTable         = identity.ContentID{0x22, 0x02}
	lawFoldStore         = identity.StoreID(13)
	lawFoldGeneration    = identity.Generation(1)
	lawFamily            = identity.ContentID{0x23, 0x03}
	lawReachDenominator  = identity.ContentID{0x24, 0x04}
	lawResultDenominator = identity.ContentID{0x25, 0x05}

	lawReachAxis  = snapshot.Axis[uint64, lawReachable]{SchemaID: lawFoldTable, Slot: 0}
	lawInputAAxis = snapshot.Axis[uint64, uint64]{SchemaID: lawFoldTable, Slot: 1}
	lawInputBAxis = snapshot.Axis[uint64, uint64]{SchemaID: lawFoldTable, Slot: 2}
)

// lawReachable is the published fact of one reached subject. It carries
// presence alone, exactly as the execution-reachability column's fact does.
type lawReachable struct{}

// lawFoldAdmissions is the sealed table's issuance request for the fixture's
// four columns, in slot order.
func lawFoldAdmissions() []ColumnAdmission {
	return []ColumnAdmission{
		{Schema: lawFoldTable, Output: lawReachOutput, Writer: lawEngineWriter, Slot: 0},
		{Schema: lawFoldTable, Output: lawInputAOutput, Writer: lawEngineWriter, Slot: 1},
		{Schema: lawFoldTable, Output: lawInputBOutput, Writer: lawEngineWriter, Slot: 2},
		{Schema: lawFoldTable, Output: lawResultOutput, Writer: lawEngineWriter, Slot: 3},
	}
}

// lawSummingFold is the fixture family's fold: it sums the present cells of
// the ordered observation its declared inputs produce. The fold has no Finish
// hook because the contributor surface declares none; Begin and Accumulate are
// the whole of what a family states about how its answer is produced.
func lawSummingFold() HotExactQuerySpec[uint64, uint64] {
	return HotExactQuerySpec[uint64, uint64]{
		Fold: QueryFold[OrderedCells[uint64], uint64]{
			Begin: func() uint64 { return 0 },
			Accumulate: func(result uint64, cells OrderedCells[uint64]) (uint64, bool) {
				for index := 0; index < cells.Count(); index++ {
					value, present, ok := cells.At(index)
					if !ok {
						return 0, false
					}
					if present {
						result += value
					}
				}
				return result, true
			},
		},
		Result: lawFrozenSum(),
	}
}

// lawFoldRefusingAfterOne is a family whose fold answers its first subject and
// then refuses. It is what a contract violation looks like partway through a
// delta: by the time it refuses, the generation being built already holds an
// answer, so the abort has something to discard.
func lawFoldRefusingAfterOne() HotExactQuerySpec[uint64, uint64] {
	folded := 0
	return HotExactQuerySpec[uint64, uint64]{
		Fold: QueryFold[OrderedCells[uint64], uint64]{
			Begin: func() uint64 { return 0 },
			Accumulate: func(uint64, OrderedCells[uint64]) (uint64, bool) {
				folded++
				if folded > 1 {
					return 0, false
				}
				return 777, true
			},
		},
		Result: lawFrozenSum(),
	}
}

func lawFrozenSum() FrozenResult[uint64] {
	return FrozenResult[uint64]{
		Semantic:    coldKey(948_003),
		Freeze:      func(value uint64) uint64 { return value },
		Clone:       func(value uint64) uint64 { return value },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
	}
}

// lawFoldFixture is one sealed family and the write capabilities its four
// columns were minted for.
type lawFoldFixture struct {
	implementation *ExactQueryImplementation[uint64, uint64]
	reach          ColumnWrite[uint64, lawReachable]
	inputA         ColumnWrite[uint64, uint64]
	inputB         ColumnWrite[uint64, uint64]
	result         ColumnWrite[uint64, uint64]
	universe       []uint64
}

func newLawFoldFixture(t testing.TB, spec HotExactQuerySpec[uint64, uint64], universe []uint64) lawFoldFixture {
	t.Helper()
	_, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, spec) {
		t.Fatal("law fold binding")
	}
	if !AdmitColumns(binding, lawFoldAdmissions()) || !binding.Seal() {
		t.Fatal("law fold admission")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil {
		t.Fatal("law fold implementation")
	}
	fixture := lawFoldFixture{implementation: implementation, universe: universe}
	var minted bool
	if fixture.reach, minted = MintColumnWrite[uint64, lawReachable](binding, lawReachOutput, lawEngineWriter); !minted {
		t.Fatal("law reachability capability")
	}
	if fixture.inputA, minted = MintColumnWrite[uint64, uint64](binding, lawInputAOutput, lawEngineWriter); !minted {
		t.Fatal("law input capability")
	}
	if fixture.inputB, minted = MintColumnWrite[uint64, uint64](binding, lawInputBOutput, lawEngineWriter); !minted {
		t.Fatal("law input capability")
	}
	if fixture.result, minted = MintColumnWrite[uint64, uint64](binding, lawResultOutput, lawEngineWriter); !minted {
		t.Fatal("law result capability")
	}
	return fixture
}

// base seals the fixture's three axis columns. The result column is not filled
// here: the driver declares it, which is what makes the publication a
// materialization rather than a hand-written column.
func (fixture lawFoldFixture) base(t testing.TB, reached []uint64, rowsA, rowsB map[uint64]uint64) snapshot.Snapshot {
	t.Helper()
	builder := snapshot.NewBuilder(lawFoldTable, lawFoldStore, lawFoldGeneration)
	rows := make(map[uint64]lawReachable, len(reached))
	for _, key := range reached {
		rows[key] = lawReachable{}
	}
	if err := PublishColumn(fixture.reach, &builder, snapshot.Content[uint64, lawReachable]{
		Rows: rows, Denominator: lawReachDenominator, Members: fixture.universe,
	}); err != nil {
		t.Fatalf("seal the reachability column: %v", err)
	}
	if err := PublishColumn(fixture.inputA, &builder, snapshot.Content[uint64, uint64]{Rows: rowsA}); err != nil {
		t.Fatalf("seal input a: %v", err)
	}
	if err := PublishColumn(fixture.inputB, &builder, snapshot.Content[uint64, uint64]{Rows: rowsB}); err != nil {
		t.Fatalf("seal input b: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the law base publication: %v", err)
	}
	return sealed
}

func (fixture lawFoldFixture) publisher(t testing.TB) *QueryPublisher[uint64, lawReachable, uint64, uint64] {
	t.Helper()
	publisher, ok := NewQueryPublisher(QueryPublisherSpec[uint64, lawReachable, uint64, uint64]{
		Family:      lawFamily,
		Write:       fixture.result,
		Reach:       lawReachAxis,
		Inputs:      []snapshot.Axis[uint64, uint64]{lawInputAAxis, lawInputBAxis},
		Denominator: Denominator[uint64]{ID: lawResultDenominator, Members: fixture.universe},
		Fold:        fixture.implementation,
	})
	if !ok || publisher == nil {
		t.Fatal("law query publisher")
	}
	return publisher
}

// lawPublish runs one delta of the driver over base.
func lawPublish(t testing.TB, publisher *QueryPublisher[uint64, lawReachable, uint64, uint64], base snapshot.Snapshot, generation identity.Generation, dirty ...uint64) (snapshot.Snapshot, error) {
	t.Helper()
	publisher.Reconsider(dirty)
	publication := NewPublication()
	if !AddQueryPublisher(publication, publisher) {
		t.Fatal("the driver admits no publisher")
	}
	return publication.PublishDelta(base, generation)
}

// TestMaterializedColumnAnswersReachedSubjectsAndProvesEveryOtherAbsence is the
// driver's central law. A reached subject whose declared inputs hold facts is
// answered; a reached subject whose inputs hold nothing was derived nothing and
// so publishes no row; a subject outside the reached set is never folded at
// all. The result column is published with the key universe its coverage
// authority states, so each of the two silences reads as a proven absence and
// not as ignorance.
func TestMaterializedColumnAnswersReachedSubjectsAndProvesEveryOtherAbsence(t *testing.T) {
	fixture := newLawFoldFixture(t, lawSummingFold(), []uint64{1, 2, 3, 4})
	base := fixture.base(t, []uint64{1, 2}, map[uint64]uint64{1: 10}, map[uint64]uint64{1: 7})
	published, err := lawPublish(t, fixture.publisher(t), base, lawFoldGeneration+1, 1, 2, 3, 4)
	if err != nil {
		t.Fatalf("materialize the law family: %v", err)
	}
	plan, opened := snapshot.OpenQuery[uint64, uint64](&published, lawFamily)
	if !opened {
		t.Fatal("the materialized family opens no plan")
	}
	if answer, status := snapshot.Query(&published, plan, 1); status != snapshot.ReadHit || answer != 17 {
		t.Fatalf("the reached subject answers %d as %s, not 17 as a hit", answer, status)
	}
	if answer, status := snapshot.Query(&published, plan, 2); status != snapshot.ReadProvenAbsent {
		t.Fatalf("the factless subject answers %d as %s, not a proven absence", answer, status)
	}
	if answer, status := snapshot.Query(&published, plan, 3); status != snapshot.ReadProvenAbsent {
		t.Fatalf("the unreached subject answers %d as %s, not a proven absence", answer, status)
	}
	if _, status := snapshot.Query(&published, plan, 99); status != snapshot.ReadMiss {
		t.Fatalf("a subject outside the published universe answers as %s, not ignorance", status)
	}
}

// TestMaterializerWithdrawsTheRowOfASubjectThatLeavesTheReachedSet states the
// stale-row law. Reachability is read causally from the publication the delta
// derives from, so a subject the engine stopped reaching loses its answer in
// the very generation that reconsiders it, and the answer it loses becomes a
// proven absence rather than a stale hit.
func TestMaterializerWithdrawsTheRowOfASubjectThatLeavesTheReachedSet(t *testing.T) {
	fixture := newLawFoldFixture(t, lawSummingFold(), []uint64{1, 2})
	base := fixture.base(t, []uint64{1, 2}, map[uint64]uint64{1: 10, 2: 4}, map[uint64]uint64{1: 7})
	publisher := fixture.publisher(t)
	published, err := lawPublish(t, publisher, base, lawFoldGeneration+1, 1, 2)
	if err != nil {
		t.Fatalf("materialize the law family: %v", err)
	}

	// The engine's demand pass stops reaching subject 1. The reachability
	// column is written in its own generation, so the delta that reconsiders
	// the subject reads the withdrawal as an already published fact.
	narrowed := snapshot.NewDelta(published, lawFoldGeneration+2)
	if err := WithdrawRow(fixture.reach, &narrowed, 1); err != nil {
		t.Fatalf("withdraw reachability: %v", err)
	}
	unreached, err := narrowed.Seal()
	if err != nil {
		t.Fatalf("seal the narrowed reachability: %v", err)
	}

	following, err := lawPublish(t, publisher, unreached, lawFoldGeneration+3, 1)
	if err != nil {
		t.Fatalf("materialize the following generation: %v", err)
	}
	plan, opened := snapshot.OpenQuery[uint64, uint64](&following, lawFamily)
	if !opened {
		t.Fatal("the following generation opens no plan")
	}
	if answer, status := snapshot.Query(&following, plan, 1); status != snapshot.ReadProvenAbsent {
		t.Fatalf("the withdrawn subject answers %d as %s, not a proven absence", answer, status)
	}
	if answer, status := snapshot.Query(&following, plan, 2); status != snapshot.ReadHit || answer != 4 {
		t.Fatalf("an untouched subject answers %d as %s, not 4 as a hit", answer, status)
	}
	priorPlan, priorOpened := snapshot.OpenQuery[uint64, uint64](&published, lawFamily)
	if !priorOpened {
		t.Fatal("the prior generation opens no plan")
	}
	if answer, status := snapshot.Query(&published, priorPlan, 1); status != snapshot.ReadHit || answer != 17 {
		t.Fatalf("the prior generation moved to %d (%s) under the withdrawal", answer, status)
	}
}

// TestRejectedFoldAbortsThePublicationWholesale states the atomicity law. A
// fold that refuses its observation is a contract violation, so the whole
// delta is abandoned even though an earlier subject of the same delta was
// already answered: no snapshot is published, no column is left holding part of
// a generation, and the publication the delta derived from answers exactly what
// it answered before.
func TestRejectedFoldAbortsThePublicationWholesale(t *testing.T) {
	accepted := newLawFoldFixture(t, lawSummingFold(), []uint64{1, 2})
	base := accepted.base(t, []uint64{1, 2}, map[uint64]uint64{1: 10, 2: 4}, nil)
	published, err := lawPublish(t, accepted.publisher(t), base, lawFoldGeneration+1, 1, 2)
	if err != nil {
		t.Fatalf("materialize the law family: %v", err)
	}

	refusing := newLawFoldFixture(t, lawFoldRefusingAfterOne(), []uint64{1, 2})
	aborted, err := lawPublish(t, refusing.publisher(t), published, lawFoldGeneration+2, 1, 2)
	if !errors.Is(err, ErrFoldRejected) {
		t.Fatalf("a refused fold published anyway: %v", err)
	}
	if aborted.Published() {
		t.Fatal("an aborted delta published a snapshot")
	}
	plan, opened := snapshot.OpenQuery[uint64, uint64](&published, lawFamily)
	if !opened {
		t.Fatal("the publication opens no plan after the abort")
	}
	if answer, status := snapshot.Query(&published, plan, 1); status != snapshot.ReadHit || answer != 10 {
		t.Fatalf("the publication answers %d as %s after an aborted delta", answer, status)
	}
	if answer, status := snapshot.Query(&published, plan, 2); status != snapshot.ReadHit || answer != 4 {
		t.Fatalf("the publication answers %d as %s after an aborted delta", answer, status)
	}
}

// TestMaterializerPublishesOnlyThroughAMintedCapability joins the two halves.
// The driver reaches storage through the capability the engine minted for the
// family's result column and through no other path, so a publisher declared
// without one answers nothing rather than writing a column it was not admitted
// to.
func TestMaterializerPublishesOnlyThroughAMintedCapability(t *testing.T) {
	fixture := newLawFoldFixture(t, lawSummingFold(), []uint64{1})
	if _, ok := NewQueryPublisher(QueryPublisherSpec[uint64, lawReachable, uint64, uint64]{
		Family:      lawFamily,
		Reach:       lawReachAxis,
		Inputs:      []snapshot.Axis[uint64, uint64]{lawInputAAxis},
		Denominator: Denominator[uint64]{ID: lawResultDenominator, Members: fixture.universe},
		Fold:        fixture.implementation,
	}); ok {
		t.Fatal("a publisher with no minted capability was declared")
	}
	if _, ok := NewQueryPublisher(QueryPublisherSpec[uint64, lawReachable, uint64, uint64]{
		Family: lawFamily,
		Write:  fixture.result,
		Reach:  lawReachAxis,
		Fold:   fixture.implementation,
	}); ok {
		t.Fatal("a publisher that declares no input was declared")
	}
}

// TestDeltaCostFollowsTheChangeSetAndNotTheColumn states the delta bound. A
// generation that reconsiders one subject copies that subject's path and shares
// every other node with the publication it derives from, so growing the column
// by two orders of magnitude does not grow what a one-subject delta allocates.
func TestDeltaCostFollowsTheChangeSetAndNotTheColumn(t *testing.T) {
	small := lawDeltaAllocations(t, 8)
	large := lawDeltaAllocations(t, 512)
	if large > small*2 {
		t.Fatalf("a one-subject delta allocates %.0f over 8 subjects and %.0f over 512, so its cost follows the column", small, large)
	}
}

// lawDeltaAllocations measures what one delta reconsidering a single subject
// allocates over a column of width subjects.
func lawDeltaAllocations(t testing.TB, width uint64) float64 {
	t.Helper()
	universe := make([]uint64, 0, width)
	rows := make(map[uint64]uint64, width)
	for key := uint64(1); key <= width; key++ {
		universe = append(universe, key)
		rows[key] = key
	}
	fixture := newLawFoldFixture(t, lawSummingFold(), universe)
	base := fixture.base(t, universe, rows, nil)
	publisher := fixture.publisher(t)
	published, err := lawPublish(t, publisher, base, lawFoldGeneration+1, universe...)
	if err != nil {
		t.Fatalf("materialize the law family: %v", err)
	}
	publisher.Reconsider([]uint64{1})
	publication := NewPublication()
	if !AddQueryPublisher(publication, publisher) {
		t.Fatal("the driver admits no publisher")
	}
	return testing.AllocsPerRun(64, func() {
		if _, err := publication.PublishDelta(published, lawFoldGeneration+2); err != nil {
			t.Fatalf("publish a one-subject delta: %v", err)
		}
	})
}
