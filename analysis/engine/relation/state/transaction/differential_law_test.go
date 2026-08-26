package transaction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func differentialLawCell(t *testing.T, fixture lawFixture, scope witness.Scope, row model.RowID, column int) binding.CellToken {
	t.Helper()
	witnessValue, ok := fixture.mounted.Denominator(fixture.denominator)
	if !ok {
		t.Fatal("differential denominator")
	}
	cell, ok := fixture.mounted.IssueCell(witnessValue, scope, fixture.columns[column], row)
	if !ok {
		t.Fatal("differential cell")
	}
	return cell
}

func differentialLawProposal(t *testing.T, fixture lawFixture, scope witness.Scope, row model.RowID, column, value int) binding.Proposal {
	t.Helper()
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("differential presence")
	}
	proposal, ok := binding.NewProposal(differentialLawCell(t, fixture, scope, row, column), fixture.values[value], presence)
	if !ok {
		t.Fatal("differential proposal")
	}
	return proposal
}

func differentialLawInputSlots(t *testing.T, fixture lawFixture, scope witness.Scope, row model.RowID) []binding.Slot {
	t.Helper()
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("differential input presence")
	}
	slots := make([]binding.Slot, 0, len(fixture.columns))
	for column := range fixture.columns {
		cell, cellOK := binding.NewCell(differentialLawCell(t, fixture, scope, row, column), fixture.typeID, fixture.values[0], presence)
		if !cellOK {
			t.Fatal("differential input cell")
		}
		slot, slotOK := binding.NewScalarSlot(cell)
		if !slotOK {
			t.Fatal("differential input slot")
		}
		slots = append(slots, slot)
	}
	return slots
}

func differentialLawApplication(t *testing.T, fixture *lawFixture, scope witness.Scope, inputRow model.RowID, lineage model.LineageRef, proposals ...binding.Proposal) apply.Application {
	t.Helper()
	fixture.applyWorker.result = outcome.Result{Code: outcome.Produced}
	fixture.applyWorker.proposals = append([]binding.Proposal(nil), proposals...)
	application, ok := apply.Apply(
		fixture.mounted,
		fixture.signature.Identity(),
		scope,
		lineage,
		binding.NewOwnerNamedDestination(fixture.relation),
		differentialLawInputSlots(t, *fixture, scope, inputRow)...,
	)
	if !ok {
		t.Fatal("differential apply")
	}
	return application
}

func differentialLawTransition(t *testing.T, fixture lawFixture, address invocation.InvocationAddress, destination binding.CellToken, before, after binding.ContributionSide) invocation.ContributionTransition {
	t.Helper()
	transition, ok := invocation.NewContributionTransition(fixture.contribution, address, destination, fixture.base.Fence(), before, after)
	if !ok {
		t.Fatal("differential transition")
	}
	return transition
}

func differentialLawAddress(t *testing.T, fixture lawFixture, scope witness.Scope, source model.RowID) invocation.InvocationAddress {
	t.Helper()
	scopeToken, ok := fixture.mounted.ScopeToken(scope)
	if !ok {
		t.Fatal("differential address scope")
	}
	tuple, ok := invocation.NewTupleSources([]model.RowID{source})
	if !ok {
		t.Fatal("differential address tuple")
	}
	vector, ok := invocation.NewSourceVector([]invocation.TupleSources{tuple})
	if !ok {
		t.Fatal("differential address vector")
	}
	address, ok := invocation.New(scopeToken, []invocation.SourceVector{vector})
	if !ok {
		t.Fatal("differential address")
	}
	return address
}

func differentialLawCommit(t *testing.T, fixture lawFixture, base database.Version, batch SubmissionBatch) (database.Version, database.Delta) {
	t.Helper()
	prepared, ok := Prepare(base, fixture.geometry, store.NewReadScratch(fixture.manager), batch)
	if !ok || !prepared.Available() {
		t.Fatal("differential prepare")
	}
	next, delta, ok := database.Commit(prepared)
	if !ok || !delta.Available() || !next.SuccessorOf(base) || next.Revision() != base.Revision()+1 || !delta.Base().Same(base) || !delta.Next().Same(next) {
		t.Fatal("differential atomic commit")
	}
	return next, delta
}

func differentialLawPartsAt(t *testing.T, version database.Version, fixture lawFixture, scope witness.Scope, row model.RowID, column int) []store.ReadPart {
	t.Helper()
	token := differentialLawCell(t, fixture, scope, row, column)
	coordinate, ok := fixture.geometry.Resolve(token)
	if !ok {
		t.Fatal("differential coordinate")
	}
	scratch := store.NewReadScratch(fixture.manager)
	if scratch == nil {
		t.Fatal("differential scratch")
	}
	parts := make([]store.ReadPart, 0, 4)
	completed, ok := version.Store().Read(fixture.columns[column], coordinate.Dense(), coordinate.Mask(), scratch, func(part store.ReadPart) bool {
		parts = append(parts, part)
		return true
	})
	if !completed || !ok {
		t.Fatal("differential store read")
	}
	return parts
}

func TestDifferentialBeforeOnlySelectiveRemovalWithZeroAfterProposals(t *testing.T) {
	fixture := newLawFixtureWithOutputPresenceAndPublishAndContributionRows(t, signature.ProducePresent, true, true, true)
	oldProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 0)
	oldApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[0], oldProposal)
	oldTransition := differentialLawTransition(t, fixture, oldApplication.Invocation(), fixture.tokens[1], binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	oldBatch, ok := NewSubmissionBatch(oldApplication, witness.WideningPermit{}, []invocation.ContributionTransition{oldTransition})
	if !ok {
		t.Fatal("legacy seed batch")
	}
	seed, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), oldBatch)
	if !ok {
		t.Fatal("differential selective seed")
	}

	siblingProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 1)
	siblingApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.otherRow, fixture.lineages[1], siblingProposal)
	siblingTransition := differentialLawTransition(t, fixture, siblingApplication.Invocation(), fixture.tokens[1], binding.NoContributionSide(), contributionSide(t, fixture, 1, 1))
	siblingBatch, ok := NewSubmissionBatch(siblingApplication, witness.WideningPermit{}, []invocation.ContributionTransition{siblingTransition})
	if !ok {
		t.Fatal("legacy sibling batch")
	}
	seed, _, ok = publish(seed, fixture.geometry, store.NewReadScratch(fixture.manager), siblingBatch)
	if !ok {
		t.Fatal("differential sibling seed")
	}

	differential, ok := applydifferential.New(oldApplication, apply.Application{})
	if !ok {
		t.Fatal("before-only differential")
	}
	removal := differentialLawTransition(t, fixture, oldApplication.Invocation(), fixture.tokens[1], contributionSide(t, fixture, 0, 0), binding.NoContributionSide())
	batch, ok := NewDifferentialSubmissionBatch(differential, witness.WideningPermit{}, []invocation.ContributionTransition{removal})
	if !ok || !batch.Available() || batch.Len() != 0 {
		t.Fatal("before-only differential batch")
	}
	next, delta := differentialLawCommit(t, fixture, seed, batch)
	if len(delta.AffectedContributionTargets()) != 1 || next.ContributionState().Len() != 1 {
		t.Fatal("selective differential removal")
	}
	rows := next.ContributionState().Rows()
	if len(rows) != 1 || rows[0].Destination.Row() != fixture.row || !rows[0].Value.Same(fixture.values[1]) {
		t.Fatal("differential removal discarded sibling")
	}
	parts := differentialLawPartsAt(t, next, fixture, fixture.scope, fixture.row, 1)
	if len(parts) != 1 || !parts[0].Presence().Is(model.Present) || !parts[0].Value().Same(fixture.values[1]) {
		t.Fatal("differential selective aggregate")
	}
}

func TestDifferentialExactReplacementMatchesBothProposalLeases(t *testing.T) {
	fixture := newContributionLawFixture(t)
	oldProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 0)
	oldApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[0], oldProposal)
	oldTransition := differentialLawTransition(t, fixture, oldApplication.Invocation(), fixture.tokens[1], binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	oldBatch, ok := NewSubmissionBatch(oldApplication, witness.WideningPermit{}, []invocation.ContributionTransition{oldTransition})
	if !ok {
		t.Fatal("replacement seed batch")
	}
	seed, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), oldBatch)
	if !ok {
		t.Fatal("replacement seed")
	}

	afterProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 1)
	afterApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[1], afterProposal)
	differential, ok := applydifferential.New(oldApplication, afterApplication)
	if !ok {
		t.Fatal("replacement differential")
	}
	replacement := differentialLawTransition(t, fixture, afterApplication.Invocation(), fixture.tokens[1], contributionSide(t, fixture, 0, 0), contributionSide(t, fixture, 1, 1))
	batch, ok := NewDifferentialSubmissionBatch(differential, witness.WideningPermit{}, []invocation.ContributionTransition{replacement})
	if !ok || !batch.Available() || batch.Len() != 1 {
		t.Fatal("replacement differential batch")
	}
	next, delta := differentialLawCommit(t, fixture, seed, batch)
	if len(delta.AffectedContributionTargets()) != 1 || next.ContributionState().Len() != 1 {
		t.Fatal("replacement contribution root")
	}
	parts := differentialLawPartsAt(t, next, fixture, fixture.scope, fixture.row, 1)
	if len(parts) != 1 || !parts[0].Presence().Is(model.Present) || !parts[0].Value().Same(fixture.values[1]) {
		t.Fatal("replacement aggregate")
	}
}

func TestDifferentialMovedDestinationUsesTwoTransitions(t *testing.T) {
	fixture := newLawFixtureWithOutputPresenceAndPublishAndContributionRows(t, signature.ProducePresent, true, true, true)
	oldProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 0)
	oldApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[0], oldProposal)
	oldTransition := differentialLawTransition(t, fixture, oldApplication.Invocation(), fixture.tokens[1], binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	oldBatch, ok := NewSubmissionBatch(oldApplication, witness.WideningPermit{}, []invocation.ContributionTransition{oldTransition})
	if !ok {
		t.Fatal("move seed batch")
	}
	seed, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), oldBatch)
	if !ok {
		t.Fatal("move seed")
	}

	newToken := differentialLawCell(t, fixture, fixture.scope, fixture.otherRow, 1)
	afterProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.otherRow, 1, 1)
	afterApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[1], afterProposal)
	differential, ok := applydifferential.New(oldApplication, afterApplication)
	if !ok {
		t.Fatal("move differential")
	}
	remove := differentialLawTransition(t, fixture, oldApplication.Invocation(), fixture.tokens[1], contributionSide(t, fixture, 0, 0), binding.NoContributionSide())
	insert := differentialLawTransition(t, fixture, afterApplication.Invocation(), newToken, binding.NoContributionSide(), contributionSide(t, fixture, 1, 1))
	batch, ok := NewDifferentialSubmissionBatch(differential, witness.WideningPermit{}, []invocation.ContributionTransition{remove, insert})
	if !ok || !batch.Available() || batch.Len() != 1 {
		t.Fatal("move differential batch")
	}
	next, delta := differentialLawCommit(t, fixture, seed, batch)
	if len(delta.AffectedContributionTargets()) != 2 || next.ContributionState().Len() != 1 {
		t.Fatal("move contribution root")
	}
	rows := next.ContributionState().Rows()
	if len(rows) != 1 || rows[0].Destination.Row() != fixture.otherRow || !rows[0].Value.Same(fixture.values[1]) {
		t.Fatal("move producer destination")
	}
	if parts := differentialLawPartsAt(t, next, fixture, fixture.scope, fixture.row, 1); len(parts) != 0 {
		t.Fatal("move left stale aggregate")
	}
	parts := differentialLawPartsAt(t, next, fixture, fixture.scope, fixture.otherRow, 1)
	if len(parts) != 1 || !parts[0].Value().Same(fixture.values[1]) {
		t.Fatal("move destination aggregate")
	}
}

func TestDifferentialRejectsForeignSideAndAddress(t *testing.T) {
	fixture := newContributionLawFixture(t)
	oldProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 0)
	oldApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[0], oldProposal)
	afterProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 1)
	afterApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[1], afterProposal)
	differential, ok := applydifferential.New(oldApplication, afterApplication)
	if !ok {
		t.Fatal("foreign envelope differential")
	}
	foreignSide := contributionSide(t, fixture, 0, 1)
	validAfter := contributionSide(t, fixture, 1, 1)
	foreignLineage := differentialLawTransition(t, fixture, afterApplication.Invocation(), fixture.tokens[1], foreignSide, validAfter)
	if _, ok := NewDifferentialSubmissionBatch(differential, witness.WideningPermit{}, []invocation.ContributionTransition{foreignLineage}); ok {
		t.Fatal("foreign lineage side crossed differential constructor")
	}

	foreignAddress := differentialLawAddress(t, fixture, fixture.otherScope, fixture.row)
	foreignAddressTransition := differentialLawTransition(t, fixture, foreignAddress, fixture.tokens[1], contributionSide(t, fixture, 0, 0), validAfter)
	if _, ok := NewDifferentialSubmissionBatch(differential, witness.WideningPermit{}, []invocation.ContributionTransition{foreignAddressTransition}); ok {
		t.Fatal("foreign invocation address crossed differential constructor")
	}
}

func TestDifferentialIgnoresOrdinaryBeforeProposal(t *testing.T) {
	fixture := newContributionLawFixture(t)
	beforeProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 0, 0)
	beforeApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[0], beforeProposal)
	afterProposal := differentialLawProposal(t, fixture, fixture.scope, fixture.row, 1, 1)
	afterApplication := differentialLawApplication(t, &fixture, fixture.scope, fixture.row, fixture.lineages[1], afterProposal)
	differential, ok := applydifferential.New(beforeApplication, afterApplication)
	if !ok {
		t.Fatal("ordinary-before differential")
	}
	transition := differentialLawTransition(t, fixture, afterApplication.Invocation(), fixture.tokens[1], binding.NoContributionSide(), contributionSide(t, fixture, 1, 1))
	batch, ok := NewDifferentialSubmissionBatch(differential, witness.WideningPermit{}, []invocation.ContributionTransition{transition})
	if !ok || !batch.Available() || batch.Len() != 1 {
		t.Fatal("ordinary-before differential batch")
	}
	next, delta := differentialLawCommit(t, fixture, fixture.base, batch)
	if len(delta.AffectedContributionTargets()) != 1 || next.ContributionState().Len() != 1 {
		t.Fatal("ordinary-before contribution")
	}
	if parts := differentialLawPartsAt(t, next, fixture, fixture.scope, fixture.row, 0); len(parts) != 0 {
		t.Fatal("ordinary Before proposal was admitted")
	}
	parts := differentialLawPartsAt(t, next, fixture, fixture.scope, fixture.row, 1)
	if len(parts) != 1 || !parts[0].Value().Same(fixture.values[1]) {
		t.Fatal("After contribution was not admitted")
	}
}
