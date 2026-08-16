package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// TestTransformerInputForwardsOnlyExistingProgramViews proves that the
// transformer facade joins sealed Source/Flow handles without retaining a
// second projection or accepting a foreign Causal Site.
func TestTransformerInputForwardsOnlyExistingProgramViews(t *testing.T) {
	left, err := Publish(rootAssembly(t, "transformer-input-left.lua"))
	if err != nil {
		t.Fatalf("Publish(left): %v", err)
	}
	right, err := Publish(rootAssembly(t, "transformer-input-right.lua"))
	if err != nil {
		t.Fatalf("Publish(right): %v", err)
	}
	input := left.TransformerInput()
	if !input.Available() || input.ContentID() != left.ContentID() {
		t.Fatal("TransformerInput did not forward Program availability/content identity")
	}

	entry, ok := left.Source().Index().Entry()
	if !ok {
		t.Fatal("left Program has no Source entry")
	}
	span, ok := input.Span(entry)
	spanAuthored, authoredOK := span.Authored()
	spanEntry, spanEntryOK := span.Entry()
	spanFinish, spanFinishOK := span.Finish()
	if !ok || !span.Available() || !authoredOK || spanAuthored != entry ||
		!spanEntryOK || !spanFinishOK || !spanEntry.Available() || !spanFinish.Available() {
		t.Fatalf("Span(%v) = %#v/%v", entry, span, ok)
	}
	ports, sites := left.Flow().Ports(), left.Flow().Causal().Sites()
	wantEntry, entryOK := ports.Entry(entry)
	wantFinish, finishOK := ports.Finish(entry)
	wantEntrySite, entrySiteOK := sites.ForTerm(wantEntry)
	wantFinishSite, finishSiteOK := sites.ForTerm(wantFinish)
	if !entryOK || !finishOK || !entrySiteOK || !finishSiteOK ||
		!spanEntry.Equal(wantEntrySite) || !spanFinish.Equal(wantFinishSite) {
		t.Fatal("Span did not join exact published Ports and Causal Sites")
	}
	finish, finishSiteOK := input.FinishSite(entry)
	if !finishSiteOK || !finish.Equal(spanFinish) {
		t.Fatal("FinishSite disagreed with Span")
	}
	rootTerm, rootExists := left.Source().Index().Root(entry)
	root, rootOK := input.RootSpan(entry)
	if !rootExists {
		if rootOK {
			t.Fatal("RootSpan succeeded for a Source term without a root")
		}
	} else {
		rootSpan, spanOK := input.Span(rootTerm)
		rootAuthored, rootAuthoredOK := root.Authored()
		wantRootAuthored, wantRootAuthoredOK := rootSpan.Authored()
		rootEntry, rootEntryOK := root.Entry()
		wantRootEntry, wantRootEntryOK := rootSpan.Entry()
		rootFinish, rootFinishOK := root.Finish()
		wantRootFinish, wantRootFinishOK := rootSpan.Finish()
		if !rootOK || !spanOK || !rootAuthoredOK || !wantRootAuthoredOK || rootAuthored != wantRootAuthored ||
			!rootEntryOK || !wantRootEntryOK || !rootEntry.Equal(wantRootEntry) ||
			!rootFinishOK || !wantRootFinishOK || !rootFinish.Equal(wantRootFinish) {
			t.Fatal("RootSpan disagreed with Source.Root followed by Span")
		}
	}

	body, bodyOK := input.Body(entry)
	wantBoundary, wantBodyOK := left.Flow().FunctionBoundaries().ForBody(entry)
	if !bodyOK || !body.Available() || !wantBodyOK {
		t.Fatal("Body did not forward the exact root BodyBoundary and absent Function")
	}
	if _, functionOK := body.Function(); functionOK {
		t.Fatal("root Body fabricated a Function boundary")
	}
	bodyEntry, bodyEntryOK := body.EntrySite()
	if !bodyEntryOK || !bodyEntry.Equal(wantEntrySite) {
		t.Fatal("Body.EntrySite did not return the existing Causal entry Site")
	}
	if body.OutcomeCount() != wantBoundary.OutcomeCount() {
		t.Fatalf("Body OutcomeCount = %d, want %d", body.OutcomeCount(), wantBoundary.OutcomeCount())
	}
	var firstOutcomeTerm keyspace.Term
	for index := 0; index < wantBoundary.OutcomeCount(); index++ {
		exit, exitOK := wantBoundary.OutcomeAt(index)
		outcome, outcomeOK := body.OutcomeAt(index)
		direct, directOK := input.Outcome(exit.Outcome)
		site, siteOK := outcome.Site()
		kind, kindOK := outcome.Kind()
		target, targetOK := outcome.Target()
		wantSite, wantSiteOK := left.Flow().Causal().Sites().ForTerm(exit.Outcome)
		if !exitOK || !outcomeOK || !directOK || !direct.Equal(outcome) || !direct.BelongsTo(body) || !outcome.Available() || !siteOK || !kindOK || !targetOK || !wantSiteOK ||
			!site.Equal(wantSite) || kind != exit.Kind || target != exit.Target {
			t.Fatalf("Body OutcomeAt(%d) did not forward exact Outcome/Causal Site", index)
		}
		if firstOutcomeTerm == 0 {
			firstOutcomeTerm = exit.Outcome
		}
		compact, compactOK := body.OutcomeSiteAt(index)
		if !compactOK || !compact.Equal(wantSite) {
			t.Fatalf("Body OutcomeSiteAt(%d) did not forward exact Causal Site", index)
		}
	}
	if _, ok := body.OutcomeAt(body.OutcomeCount()); ok {
		t.Fatal("Body OutcomeAt accepted an out-of-range index")
	}
	if input.BodyCount() != left.Source().Identity().FamilyCount(keyspace.FamilyBody) {
		t.Fatal("BodyCount did not forward Source's Body denominator")
	}
	bodyAt, bodyAtOK := input.BodyAt(0)
	bodyAtEntry, bodyAtEntryOK := bodyAt.EntrySite()
	if !bodyAtOK || !bodyAt.Available() || !bodyAtEntryOK || !bodyAtEntry.Equal(wantEntrySite) || wantBoundary.Available() != body.Available() {
		t.Fatal("BodyAt did not return the canonical root Body view")
	}
	got, gotOK := input.GuardCount(spanEntry)
	want, wantOK := left.Flow().Continuation().GuardCount(wantEntry)
	if !gotOK || !wantOK || got != 0 || got != want {
		t.Fatalf("empty root GuardCount = %d/%v, want zero/true (%d/%v)", got, gotOK, want, wantOK)
	}
	if _, ok := input.GuardAt(spanEntry, 0); ok {
		t.Fatal("GuardAt accepted an out-of-range zero-count guard index")
	}
	if input.Regions().Count() != left.Flow().Local().Regions().Count() {
		t.Fatal("Regions did not forward Flow.Local")
	}

	var zero TransformerInput
	if zero.Available() || zero.ContentID().Available() || zero.Regions().Count() != 0 {
		t.Fatal("zero TransformerInput did not fail closed")
	}
	if _, ok := zero.Span(entry); ok {
		t.Fatal("zero TransformerInput exposed a Span")
	}
	rightEntry, rightEntryOK := right.Source().Index().Entry()
	rightSpan, rightSpanOK := right.TransformerInput().Span(rightEntry)
	if !rightEntryOK || !rightSpanOK {
		t.Fatal("right Program has no transformer Span")
	}
	rightSite, rightSiteOK := rightSpan.Entry()
	if !rightSiteOK {
		t.Fatal("right Span has no entry Site")
	}
	if _, ok := input.GuardCount(rightSite); ok {
		t.Fatal("left TransformerInput accepted a foreign Site")
	}
	replayed, err := Publish(rootAssembly(t, "transformer-input-left.lua"))
	if err != nil {
		t.Fatalf("Publish(replay): %v", err)
	}
	replaySpan, replayOK := replayed.TransformerInput().Span(entry)
	replaySite, replaySiteOK := replaySpan.Entry()
	if !replayOK || !replaySiteOK || !span.Equal(replaySpan) {
		t.Fatal("equivalent replay did not retain the published Span identity")
	}
	if _, replayGuardsOK := input.GuardCount(replaySite); replayGuardsOK {
		t.Fatal("exact TransformerInput accepted an equivalent replay Site")
	}

	originalFlow := left.flow
	left.flow = right.flow
	if input.Available() || input.ContentID().Available() || input.Regions().Count() != 0 {
		t.Fatal("issued TransformerInput accepted left root identity with right Flow")
	}
	left.flow = originalFlow
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = left.TransformerInput()
		_, _ = input.Span(entry)
		_, _ = input.Body(entry)
		_, _ = input.Outcome(firstOutcomeTerm)
		_, _ = input.GuardCount(spanEntry)
		_ = input.Regions().Count()
	}); allocations != 0 {
		t.Fatalf("TransformerInput queries allocate %v times", allocations)
	}
}

// TestTransformerInputForwardsCausalProofQueries keeps the retained
// TransformerInput surface honest: every query is a direct projection of the
// one sealed Flow owner, with no new row or graph authority in Program.
func TestTransformerInputForwardsCausalProofQueries(t *testing.T) {
	left, err := Publish(rootAssembly(t, "transformer-input-causal-left.lua"))
	if err != nil {
		t.Fatalf("Publish(left): %v", err)
	}
	right, err := Publish(rootAssembly(t, "transformer-input-causal-right.lua"))
	if err != nil {
		t.Fatalf("Publish(right): %v", err)
	}
	input := left.TransformerInput()
	if !input.Available() {
		t.Fatal("left TransformerInput is unavailable")
	}

	wantProvenance := left.Flow().Provenance()
	provenance, provenanceOK := input.Provenance()
	if !provenanceOK || provenance != wantProvenance {
		t.Fatalf("TransformerInput.Provenance = %#v/%v, want %#v/true", provenance, provenanceOK, wantProvenance)
	}

	wantSites := left.Flow().Causal().Sites()
	if got := input.CausalSiteCount(); got != wantSites.Count() {
		t.Fatalf("CausalSiteCount = %d, want %d", got, wantSites.Count())
	}
	for index := 0; index < wantSites.Count(); index++ {
		got, gotOK := input.CausalSiteAt(index)
		want, wantOK := wantSites.At(index)
		if !gotOK || !wantOK || !got.Equal(want) {
			t.Fatalf("CausalSiteAt(%d) = %#v/%v, want %#v/true", index, got, gotOK, want)
		}
	}
	if _, ok := input.CausalSiteAt(-1); ok {
		t.Fatal("CausalSiteAt accepted a negative index")
	}
	if _, ok := input.CausalSiteAt(input.CausalSiteCount()); ok {
		t.Fatal("CausalSiteAt accepted an out-of-bounds index")
	}

	entry, entryOK := left.Source().Index().Entry()
	if !entryOK {
		t.Fatal("left Program has no Source entry")
	}
	wantSuccessors := left.Flow().Causal().Successors()
	for _, from := range []keyspace.Term{entry, 0, keyspace.MakeTerm(keyspace.FamilyWrite, 1)} {
		gotCount := input.CausalSuccessorCount(from)
		wantCount := wantSuccessors.Count(from)
		if gotCount != wantCount {
			t.Fatalf("CausalSuccessorCount(%v) = %d, want %d", from, gotCount, wantCount)
		}
		for index := 0; index < wantCount; index++ {
			got, gotOK := input.CausalSuccessorAt(from, index)
			want, wantOK := wantSuccessors.At(from, index)
			gotID, gotIDOK := got.Identity()
			wantID, wantIDOK := want.Identity()
			if !gotOK || !wantOK || !gotIDOK || !wantIDOK || gotID != wantID ||
				got.From != want.From || got.To != want.To || got.Decision != want.Decision ||
				got.Truth != want.Truth || got.Mu != want.Mu || got.Arm != want.Arm {
				t.Fatalf("CausalSuccessorAt(%v,%d) = %#v/%v, want %#v/true", from, index, got, gotOK, want)
			}
		}
		if _, ok := input.CausalSuccessorAt(from, -1); ok {
			t.Fatalf("CausalSuccessorAt(%v,-1) accepted a negative index", from)
		}
		if _, ok := input.CausalSuccessorAt(from, wantCount); ok {
			t.Fatalf("CausalSuccessorAt(%v,%d) accepted an out-of-bounds index", from, wantCount)
		}
	}
	if input.CausalSuccessorCount(0) != 0 {
		t.Fatal("CausalSuccessorCount accepted a zero/foreign source term")
	}

	wantBoundaries := left.Flow().Causal().Boundaries()
	if got := input.CausalBoundaryCount(); got != wantBoundaries.Count() {
		t.Fatalf("CausalBoundaryCount = %d, want %d", got, wantBoundaries.Count())
	}
	for index := 0; index < wantBoundaries.Count(); index++ {
		got, gotOK := input.CausalBoundaryAt(index)
		want, wantOK := wantBoundaries.At(index)
		if !gotOK || !wantOK || got != want {
			t.Fatalf("CausalBoundaryAt(%d) = %#v/%v, want %#v/true", index, got, gotOK, want)
		}
	}
	if _, ok := input.CausalBoundaryAt(-1); ok {
		t.Fatal("CausalBoundaryAt accepted a negative index")
	}
	if _, ok := input.CausalBoundaryAt(input.CausalBoundaryCount()); ok {
		t.Fatal("CausalBoundaryAt accepted an out-of-bounds index")
	}

	wantWrites := left.Source().Identity().FamilyCount(keyspace.FamilyWrite)
	for ordinal := 1; ordinal <= wantWrites; ordinal++ {
		write := keyspace.MakeTerm(keyspace.FamilyWrite, uint32(ordinal))
		got, gotOK := input.AssignmentPredecessor(write)
		want, wantOK := wantSuccessors.AssignmentPredecessor(write)
		gotID, gotIDOK := got.Identity()
		wantID, wantIDOK := want.Identity()
		if gotOK != wantOK || gotIDOK != wantIDOK || (gotOK && (gotID != wantID || got.From != want.From || got.To != want.To)) {
			t.Fatalf("AssignmentPredecessor(%v) = %#v/%v, want %#v/%v", write, got, gotOK, want, wantOK)
		}
	}
	if _, ok := input.AssignmentPredecessor(0); ok {
		t.Fatal("AssignmentPredecessor accepted a zero/foreign term")
	}
	if _, ok := input.AssignmentPredecessor(keyspace.MakeTerm(keyspace.FamilyValues, 1)); ok {
		t.Fatal("AssignmentPredecessor accepted a non-Write term")
	}
	if _, ok := input.AssignmentPredecessor(keyspace.MakeTerm(keyspace.FamilyWrite, uint32(wantWrites+1))); ok {
		t.Fatal("AssignmentPredecessor accepted an out-of-bounds Write")
	}

	var zero TransformerInput
	if _, ok := zero.Provenance(); ok || zero.CausalSiteCount() != 0 || zero.CausalSuccessorCount(entry) != 0 || zero.CausalBoundaryCount() != 0 {
		t.Fatal("zero TransformerInput exposed causal proof queries")
	}
	if _, ok := zero.CausalSiteAt(0); ok {
		t.Fatal("zero TransformerInput exposed a Causal Site")
	}
	if _, ok := zero.CausalSuccessorAt(entry, 0); ok {
		t.Fatal("zero TransformerInput exposed a Causal Successor")
	}
	if _, ok := zero.CausalBoundaryAt(0); ok {
		t.Fatal("zero TransformerInput exposed a Causal Boundary")
	}
	if _, ok := zero.AssignmentPredecessor(keyspace.MakeTerm(keyspace.FamilyWrite, 1)); ok {
		t.Fatal("zero TransformerInput exposed an assignment predecessor")
	}

	foreignEntry, foreignEntryOK := right.Source().Index().Entry()
	foreignSite, foreignSiteOK := right.Flow().Causal().Sites().ForTerm(foreignEntry)
	if !foreignEntryOK || !foreignSiteOK || input.CausalSiteCount() == 0 {
		t.Fatal("foreign causal fixture has no entry Site")
	}
	localSite, localSiteOK := input.CausalSiteAt(0)
	if !localSiteOK || localSite.Equal(foreignSite) {
		t.Fatal("foreign Causal Site crossed the Program owner fence")
	}

	write := keyspace.MakeTerm(keyspace.FamilyWrite, 1)
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = input.Provenance()
		_ = input.CausalSiteCount()
		_, _ = input.CausalSiteAt(0)
		_ = input.CausalSuccessorCount(entry)
		_, _ = input.CausalSuccessorAt(entry, 0)
		_ = input.CausalBoundaryCount()
		_, _ = input.CausalBoundaryAt(0)
		_, _ = input.AssignmentPredecessor(write)
	}); allocations != 0 {
		t.Fatalf("TransformerInput causal proof queries allocate %v times", allocations)
	}
}
