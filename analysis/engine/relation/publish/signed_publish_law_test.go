package publish_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// signedApplication keeps the exact application-owned proposal lease. The
// helper changes only the fixture worker's authored output; it does not copy
// or reconstruct a ProposalBatch at the publication boundary.
func signedApplication(t *testing.T, value fixture, proposals ...binding.Proposal) apply.Application {
	t.Helper()
	if len(proposals) == 0 {
		t.Fatal("signed application needs a proposal")
	}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	value.worker.proposal = proposals[0]
	value.worker.proposals = append([]binding.Proposal(nil), proposals...)
	return value.application(t, outcome.Produced)
}

func signedDifferential(t *testing.T, before, after apply.Application) applydifferential.Differential {
	t.Helper()
	value, ok := applydifferential.New(before, after)
	if !ok || !value.Available() {
		t.Fatal("signed differential")
	}
	return value
}

func seededContribution(t *testing.T, value fixture) (fixture, database.Version) {
	t.Helper()
	seed := value.application(t, outcome.Produced)
	settlement := value.door.Publish(value.aggregate, value.readScratch, seed, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() {
		t.Fatal("seed contribution")
	}
	if settlement.Next().ContributionState().Len() == 0 {
		t.Fatal("seed did not retain producer contribution")
	}
	return value, settlement.Next()
}

func assertSignedAscent(t *testing.T, base database.Version, settlement publish.Settlement) {
	t.Helper()
	if !settlement.Available() || !settlement.Changed() || !settlement.Next().SuccessorOf(base) {
		t.Fatal("signed publication did not commit one ascent")
	}
	if settlement.Next().Revision() != base.Revision()+1 {
		t.Fatalf("signed publication revisions base=%d next=%d", base.Revision(), settlement.Next().Revision())
	}
	delta, ok := settlement.Delta()
	if !ok || !delta.Available() || !delta.Base().Same(base) || !delta.Next().Same(settlement.Next()) {
		t.Fatal("signed publication lost its exact one-commit delta")
	}
}

func TestPublishDifferentialBeforeOnlyRemovalCommitsOnce(t *testing.T) {
	value, base := seededContribution(t, newContributionFixture(t))
	before := signedApplication(t, value, value.proposals[1])
	differential := signedDifferential(t, before, apply.Application{})

	settlement := value.door.PublishDifferential(base, value.readScratch, differential, value.permit)
	if !settlement.Available() {
		t.Fatalf("before-only removal settlement unavailable: changed=%v outcome=%+v", settlement.Changed(), settlement.Outcome())
	}
	if !settlement.Changed() {
		t.Fatalf("before-only removal settlement did not commit: outcome=%+v", settlement.Outcome())
	}
	if got := settlement.Outcome().Code; got != outcome.Produced {
		t.Fatalf("before-only removal settlement outcome=%v, want Produced", got)
	}
	if settlement.Next().ContributionState().Len() != 0 {
		t.Fatal("before-only removal retained producer contribution")
	}
	assertSignedAscent(t, base, settlement)
}

func TestPublishDifferentialReplacementCommitsOneDelta(t *testing.T) {
	value, base := seededContribution(t, newContributionFixture(t))
	before := signedApplication(t, value, value.proposals[1])

	replacementValue, ok := value.mounted.IssueValue(value.typeID, content("signed-replacement"))
	if !ok {
		t.Fatal("replacement value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("replacement presence")
	}
	replacement, ok := binding.NewProposal(value.token, replacementValue, presence)
	if !ok {
		t.Fatal("replacement proposal")
	}
	after := signedApplication(t, value, replacement)
	differential := signedDifferential(t, before, after)

	settlement := value.door.PublishDifferential(base, value.readScratch, differential, value.permit)
	if !settlement.Available() || !settlement.Changed() || settlement.Outcome().Code != outcome.Produced {
		t.Fatal("replacement did not retain Produced settlement")
	}
	rows := settlement.Next().ContributionState().Rows()
	if len(rows) != 1 || !rows[0].Value.Same(replacementValue) {
		t.Fatal("replacement did not replace the producer payload")
	}
	assertSignedAscent(t, base, settlement)
}

func TestPublishDifferentialMovedDestinationDeletesAndInsertsAtomically(t *testing.T) {
	cardinality, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("cardinality")
	}
	value, base := seededContribution(t, newFixtureWithContribution(t, 0x9a, 2, cardinality, true))
	before := signedApplication(t, value, value.proposals[1])

	movedValue, ok := value.mounted.IssueValue(value.typeID, content("signed-move"))
	if !ok {
		t.Fatal("moved value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("moved presence")
	}
	moved, ok := binding.NewProposal(value.tokens[1], movedValue, presence)
	if !ok {
		t.Fatal("moved proposal")
	}
	// Keep the same input frame/invocation address while changing only the
	// authenticated output destination.
	after := signedApplication(t, value, moved)
	differential := signedDifferential(t, before, after)

	settlement := value.door.PublishDifferential(base, value.readScratch, differential, value.permit)
	if !settlement.Available() || !settlement.Changed() {
		t.Fatal("moved destination did not commit")
	}
	rows := settlement.Next().ContributionState().Rows()
	if len(rows) != 1 || rows[0].Cell().Row() != value.rows[1] || !rows[0].Value.Same(movedValue) {
		t.Fatal("moved destination was not an atomic delete/insert")
	}
	assertSignedAscent(t, base, settlement)
}

func TestPublishDifferentialForeignRefusesUnchanged(t *testing.T) {
	local := newContributionFixture(t)
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("foreign cardinality")
	}
	foreign := newFixtureWithContribution(t, 0x76, 1, cardinality, true)
	foreignApplication := foreign.application(t, outcome.Produced)
	differential := signedDifferential(t, apply.Application{}, foreignApplication)

	settlement := local.door.PublishDifferential(local.aggregate, local.readScratch, differential, witness.WideningPermit{})
	if settlement.Available() {
		t.Fatal("foreign differential crossed the publication Door")
	}
	if local.aggregate.Revision() != 1 || local.aggregate.ContributionState().Len() != 0 {
		t.Fatal("foreign differential changed the predecessor root")
	}
}

func TestPublishDifferentialOrdinaryAfterUsesPositivePublication(t *testing.T) {
	value := newContributionFixture(t)
	ordinary := signedApplication(t, value, value.proposals[0], value.proposals[2])
	differential := signedDifferential(t, apply.Application{}, ordinary)

	settlement := value.door.PublishDifferential(value.aggregate, value.readScratch, differential, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() || !settlement.Outcome().Available() || settlement.Outcome().Code != outcome.Produced {
		t.Fatal("ordinary After did not use positive publication")
	}
	if !settlement.Next().ContributionState().Same(value.aggregate.ContributionState()) {
		t.Fatal("ordinary After entered signed contribution state")
	}
	assertSignedAscent(t, value.aggregate, settlement)
}
