package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContinuationSealDirectCallAndExactSubjectPlane(t *testing.T) {
	fixture := openContinuationFixture(t, directContinuationSpec("continuation-direct-call.lua"))
	sourceID := fixture.sourceView.Identity().ContentID()
	flowID := fixture.flow.Cold().ContentID()
	if !Matches(fixture.result, sourceID, flowID, fixture.staticID, fixture.moduleID) {
		t.Fatal("sealed continuation provenance did not match exact quartet")
	}
	call := continuationTerm(keyspace.FamilyCall, 1)
	if count, ok := fixture.result.CellCount(call); !ok || count != 0 {
		t.Fatalf("direct Call Cells = %d/%v, want 0/true", count, ok)
	}
	if count, ok := fixture.result.GuardCount(call); !ok || count != 0 {
		t.Fatalf("direct Call Guards = %d/%v, want 0/true", count, ok)
	}
	if _, ok := fixture.result.CellAt(call, 0); ok {
		t.Fatal("empty direct Call Cell root exposed a Cell")
	}
	if _, ok := fixture.result.GuardAt(call, 0); ok {
		t.Fatal("empty direct Call Guard root exposed a Guard")
	}
	for _, foreign := range []identity.ContentID{sourceID, flowID, fixture.staticID, fixture.moduleID} {
		candidate := [4]identity.ContentID{sourceID, flowID, fixture.staticID, fixture.moduleID}
		index := 0
		for index := range candidate {
			if candidate[index] == foreign {
				break
			}
		}
		foreign[0]++
		candidate[index] = foreign
		if Matches(fixture.result, candidate[0], candidate[1], candidate[2], candidate[3]) {
			t.Fatalf("foreign quartet component %d matched", index)
		}
	}
	if _, ok := fixture.result.CellCount(keyspace.MakeTerm(keyspace.FamilyValues, 1)); ok {
		t.Fatal("Values Term entered continuation subject denominator")
	}
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if count, ok := fixture.result.GuardCount(outcome); !ok || count != 0 {
		t.Fatalf("Outcome endpoint Guards = %d/%v, want empty available scope", count, ok)
	}
	if _, ok := fixture.result.GuardAt(outcome, 0); ok {
		t.Fatal("empty Outcome endpoint Guard root exposed a Guard")
	}
	foreignOutcome := keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(fixture.sourceView.Identity().FamilyCount(keyspace.FamilyOutcome))+1)
	if _, ok := fixture.result.GuardCount(foreignOutcome); ok {
		t.Fatal("foreign/non-vertex Outcome Term entered continuation endpoint denominator")
	}
}

func TestContinuationSealAdmitsTerminalOnlyCausalSites(t *testing.T) {
	fixture := openContinuationFixture(t, continuationTerminalOnlySpec())
	sites := fixture.causal
	successors := sites.Successors()
	terminalSites := 0
	for index := 0; index < sites.SiteCount(); index++ {
		site, ok := sites.SiteAt(index)
		if !ok {
			t.Fatalf("Causal SiteAt(%d) failed", index)
		}
		term, ok := site.Term()
		if !ok || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
			continue
		}
		if successors.Count(term) != 0 {
			continue
		}
		terminalSites++
		if count, ok := fixture.result.GuardCount(term); !ok || count != 0 {
			t.Fatalf("terminal-only Outcome %08x GuardCount = %d/%v, want 0/true", uint32(term), count, ok)
		}
		if _, ok := fixture.result.GuardAt(term, 0); ok {
			t.Fatalf("terminal-only Outcome %08x exposed an empty Guard", uint32(term))
		}
	}
	if terminalSites == 0 {
		t.Fatal("terminal-only Causal fixture published no terminal Outcome Sites")
	}
}

func TestContinuationSealUnavailableAndForeignInputsFailClosed(t *testing.T) {
	left := openContinuationFixture(t, directContinuationSpec("continuation-foreign-left.lua"))
	rightSpec := directContinuationSpec("continuation-foreign-right.lua")
	rightSpec.counts[keyspace.FamilyCall] = 2
	rightSpec.counts[keyspace.FamilyValues] = 2
	rightSpec.counts[keyspace.FamilyNil] = 2
	rightSpec.rows = [][]keyspace.Term{{continuationTerm(keyspace.FamilyCall, 1), continuationTerm(keyspace.FamilyCall, 2)}}
	rightSpec.nilOwners = []keyspace.Term{continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 1)}
	rightSpec.flow.Values.Rows = []authored.Value{{Owner: continuationTerm(keyspace.FamilyBody, 1)}, {Owner: continuationTerm(keyspace.FamilyBody, 1)}}
	rightSpec.flow.Calls = []authored.Call{
		{Owner: continuationTerm(keyspace.FamilyBody, 1), Callee: continuationTerm(keyspace.FamilyNil, 1), Actuals: continuationTerm(keyspace.FamilyValues, 1)},
		{Owner: continuationTerm(keyspace.FamilyBody, 1), Callee: continuationTerm(keyspace.FamilyNil, 2), Actuals: continuationTerm(keyspace.FamilyValues, 2)},
	}
	right := openContinuationFixture(t, rightSpec)
	if Matches(left.result, right.sourceView.Identity().ContentID(), left.flow.Cold().ContentID(), left.staticID, left.moduleID) {
		t.Fatal("foreign Source identity matched continuation Result")
	}
	if Matches(left.result, left.sourceView.Identity().ContentID(), right.flow.Cold().ContentID(), left.staticID, left.moduleID) {
		t.Fatal("foreign authored Flow identity matched continuation Result")
	}
	if Matches(left.result, left.sourceView.Identity().ContentID(), left.flow.Cold().ContentID(), right.staticID, left.moduleID) {
		t.Fatal("foreign Static identity matched continuation Result")
	}
	foreignModule := left.moduleID
	foreignModule[0]++
	if Matches(left.result, left.sourceView.Identity().ContentID(), left.flow.Cold().ContentID(), left.staticID, foreignModule) {
		t.Fatal("foreign Module identity matched continuation Result")
	}
	if _, err := Seal(right.sourceView, left.flow, left.bodies, left.binding, left.executable, left.candidates, left.causal, left.staticID, left.moduleID); err == nil {
		t.Fatal("Seal accepted a foreign Source with typed prerequisites")
	}
	if _, err := Seal(left.sourceView, right.flow, left.bodies, left.binding, left.executable, left.candidates, left.causal, left.staticID, left.moduleID); err == nil {
		t.Fatal("Seal accepted a foreign authored Flow with typed prerequisites")
	}
	if _, err := Seal(left.sourceView, left.flow, left.bodies, binding.Result{}, left.executable, left.candidates, left.causal, left.staticID, left.moduleID); err == nil {
		t.Fatal("Seal accepted an unavailable Binding result")
	}
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.binding, nil, left.candidates, left.causal, left.staticID, left.moduleID); err == nil {
		t.Fatal("Seal accepted an unavailable Executable result")
	}
	foreignStatic := left.staticID
	foreignStatic[0]++
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.binding, left.executable, left.candidates, left.causal, foreignStatic, left.moduleID); err == nil {
		t.Fatal("Seal accepted a foreign Static identity")
	}
	foreignModule = left.moduleID
	foreignModule[0]++
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.binding, left.executable, left.candidates, left.causal, left.staticID, foreignModule); err == nil {
		t.Fatal("Seal accepted a foreign Module identity")
	}
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.binding, left.executable, left.candidates, left.causal, identity.ContentID{}, left.moduleID); err == nil {
		t.Fatal("Seal accepted an unavailable Static identity")
	}
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.binding, left.executable, left.candidates, left.causal, left.staticID, identity.ContentID{}); err == nil {
		t.Fatal("Seal accepted an unavailable Module identity")
	}
}
