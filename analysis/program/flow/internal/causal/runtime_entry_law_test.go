package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/runtimeentry"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func gotoBranchResumeSpec(name string, probe func(*runtimeEntryFixture)) causalSpec {
	parent := causalTerm(keyspace.FamilyBody, 1)
	loopBody := causalTerm(keyspace.FamilyBody, 2)
	trueBody := causalTerm(keyspace.FamilyBody, 3)
	falseBody := causalTerm(keyspace.FamilyBody, 4)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	loopCondition := causalTerm(keyspace.FamilyNil, 1)
	branchCondition := causalTerm(keyspace.FamilyNil, 2)
	return causalSpec{
		name: name,
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 4},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
			causalFamilyCount{keyspace.FamilyBranch, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyNil, 2},
		),
		rows:      [][]keyspace.Term{{loop, label, branch}, {gotoTerm}, nil, nil},
		nilOwners: []keyspace.Term{parent, parent},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}},
			Gotos:  []authored.Goto{{Owner: loopBody, Target: label}},
			Branches: []authored.Branch{{Owner: parent, Condition: branchCondition,
				WhenTrue: trueBody, WhenFalse: falseBody}},
			Loops: []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: loopCondition}},
		}},
		runtimeEntryProbe: probe,
	}
}

func directOutcome(t *testing.T, fixture *runtimeEntryFixture, root keyspace.Term) keyspace.Term {
	t.Helper()
	out, ok := fixture.outcomes.GotoExit(root)
	if !ok {
		t.Fatal("Goto Outcome is unavailable")
	}
	for {
		parent, propagated := fixture.outcomes.Propagation(out)
		if !propagated {
			return out
		}
		out = parent
	}
}

func TestRuntimeEntryBindsOutcomeResumeToExactBranchConditionDespiteSharedCSRPhase(t *testing.T) {
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	condition := causalTerm(keyspace.FamilyNil, 2)
	var resumeOutcome keyspace.Term
	f := openCausalFixture(t, gotoBranchResumeSpec("runtime-entry-exact.lua", func(input *runtimeEntryFixture) {
		resumeOutcome = directOutcome(t, input, gotoTerm)
		raw, ok := input.control.Resume(label)
		if !ok || raw != branch {
			t.Fatalf("SourceControl Resume = %v/%v, want Branch %v", raw, ok, branch)
		}
		entry, ok := input.entries.Entry(raw)
		if !ok || entry != condition {
			t.Fatalf("runtime Entry = %v/%v, want exact condition %v", entry, ok, condition)
		}
		branchPhase, branchOK := input.control.CoordinatePhase(input.sourceView, branch)
		conditionPhase, conditionOK := input.control.CoordinatePhase(input.sourceView, condition)
		branchPath, branchPathOK := input.control.ResolvePhaseRef(branchPhase)
		conditionPath, conditionPathOK := input.control.ResolvePhaseRef(conditionPhase)
		if !branchOK || !conditionOK || !branchPathOK || !conditionPathOK || branchPath != conditionPath {
			t.Fatal("fixture does not exercise two exact endpoints sharing one CSR phase")
		}
		anchor, err := input.control.IssueOutcomeResumeAnchor(input.sourceView, input.outcomes, resumeOutcome)
		if err != nil {
			t.Fatalf("IssueOutcomeResumeAnchor: %v", err)
		}
		projection, err := input.entries.IssueOutcomeResumeProjection(input.sourceView, input.control, anchor)
		if err != nil {
			t.Fatalf("IssueOutcomeResumeProjection: %v", err)
		}
		projectionCopy := *projection
		segment, ok := runtimeentry.ConsumeOutcomeResumeProjection(input.entries, input.control, projection)
		if !ok {
			t.Fatal("exact Outcome resume projection did not consume")
		}
		from, to, ok := segment.RouteTerms(input.entries, input.control)
		if !ok || from != resumeOutcome || to != condition || segment.MatchesRoute(resumeOutcome, branch) {
			t.Fatalf("normalized route = %v -> %v/%v", from, to, ok)
		}
		if _, ok := runtimeentry.ConsumeOutcomeResumeProjection(input.entries, input.control, &projectionCopy); ok {
			t.Fatal("copied Outcome resume projection consumed twice")
		}
		anchor, err = input.control.IssueOutcomeResumeAnchor(input.sourceView, input.outcomes, resumeOutcome)
		if err != nil {
			t.Fatalf("second IssueOutcomeResumeAnchor: %v", err)
		}
		anchorCopy := *anchor
		if _, err := input.entries.IssueOutcomeResumeProjection(input.sourceView, input.control, anchor); err != nil {
			t.Fatalf("exact copied-anchor setup failed: %v", err)
		}
		if _, err := input.entries.IssueOutcomeResumeProjection(input.sourceView, input.control, &anchorCopy); err == nil {
			t.Fatal("copied Outcome resume anchor consumed twice")
		}
		if entry, ok := input.entries.Entry(label); ok || entry != 0 {
			t.Fatalf("non-executable Label acquired runtime Entry %v/%v", entry, ok)
		}
	}))
	if resumeOutcome == 0 {
		t.Fatal("direct resume Outcome was not captured")
	}
	found := false
	for index := 0; index < f.result.Successors().Count(resumeOutcome); index++ {
		successor, ok := f.result.Successors().At(resumeOutcome, index)
		found = found || ok && !successor.IsBoundary() && successor.To == condition
		if ok && successor.To == branch {
			t.Fatalf("Outcome resume retained same-phase Branch sibling: %#v", successor)
		}
	}
	if !found {
		t.Fatalf("Outcome %v did not resume at exact condition %v", resumeOutcome, condition)
	}
}

func TestRuntimeEntryOutcomeResumeRejectsForeignExactOwner(t *testing.T) {
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	openCausalFixture(t, gotoBranchResumeSpec("runtime-entry-owner-a.lua", func(first *runtimeEntryFixture) {
		out := directOutcome(t, first, gotoTerm)
		foreignAnchor, err := first.control.IssueOutcomeResumeAnchor(first.sourceView, first.outcomes, out)
		if err != nil {
			t.Fatalf("IssueOutcomeResumeAnchor: %v", err)
		}
		projectionAnchor, err := first.control.IssueOutcomeResumeAnchor(first.sourceView, first.outcomes, out)
		if err != nil {
			t.Fatalf("IssueOutcomeResumeAnchor for projection: %v", err)
		}
		foreignProjection, err := first.entries.IssueOutcomeResumeProjection(first.sourceView, first.control, projectionAnchor)
		if err != nil {
			t.Fatalf("IssueOutcomeResumeProjection: %v", err)
		}
		openCausalFixture(t, gotoBranchResumeSpec("runtime-entry-owner-b.lua", func(second *runtimeEntryFixture) {
			if _, err := second.entries.IssueOutcomeResumeProjection(first.sourceView, first.control, foreignAnchor); err == nil {
				t.Fatal("foreign runtime-entry owner accepted exact first-owner anchor")
			}
			if _, ok := runtimeentry.ConsumeOutcomeResumeProjection(second.entries, second.control, foreignProjection); ok {
				t.Fatal("foreign runtime-entry owner consumed exact first-owner projection")
			}
		}))
		if _, err := first.entries.IssueOutcomeResumeProjection(first.sourceView, first.control, foreignAnchor); err == nil {
			t.Fatal("foreign anchor probe did not consume and clear receipt terminally")
		}
		if _, ok := runtimeentry.ConsumeOutcomeResumeProjection(first.entries, first.control, foreignProjection); ok {
			t.Fatal("foreign projection probe did not consume and clear receipt terminally")
		}
	}))
}

func TestRuntimeEntryBodyResumeKeepsNormalBodyTail(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	child := causalTerm(keyspace.FamilyBody, 2)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	breakTerm := causalTerm(keyspace.FamilyBreak, 1)
	condition := causalTerm(keyspace.FamilyNil, 1)
	openCausalFixture(t, causalSpec{
		name: "runtime-entry-body-tail.lua",
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2}, causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyBreak, 1}, causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows: [][]keyspace.Term{{loop}, {breakTerm}}, nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Breaks: []authored.Break{{Owner: child}},
			Loops:  []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: condition}},
		}},
		runtimeEntryProbe: func(input *runtimeEntryFixture) {
			out, ok := input.outcomes.BreakExit(breakTerm)
			if !ok {
				t.Fatal("Break Outcome is unavailable")
			}
			for {
				next, propagated := input.outcomes.Propagation(out)
				if !propagated {
					break
				}
				out = next
			}
			anchor, err := input.control.IssueOutcomeResumeAnchor(input.sourceView, input.outcomes, out)
			if err != nil {
				t.Fatalf("IssueOutcomeResumeAnchor: %v", err)
			}
			projection, err := input.entries.IssueOutcomeResumeProjection(input.sourceView, input.control, anchor)
			if err != nil {
				t.Fatalf("IssueOutcomeResumeProjection: %v", err)
			}
			segment, ok := runtimeentry.ConsumeOutcomeResumeProjection(input.entries, input.control, projection)
			if !ok {
				t.Fatal("Body resume projection did not consume")
			}
			_, to, ok := segment.RouteTerms(input.entries, input.control)
			normal, normalOK := input.outcomes.BodyExit(parent, kind.OutcomeNormal)
			_, tailOK := input.control.BodyTailPhase(parent)
			if !ok || !normalOK || !tailOK || to != normal {
				t.Fatalf("Body resume = %v/%v, want Normal %v/%v with tail %v", to, ok, normal, normalOK, tailOK)
			}
		},
	})
}
