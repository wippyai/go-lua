package project

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
)

// TestCallsIssueOnlyExecutableProgramApplications exercises the exact mounted
// inverse at both ends of the Program Call denominator. A dead Call remains a
// valid Program occurrence but is absent from Project.
func TestCallsIssueOnlyExecutableProgramApplications(t *testing.T) {
	p := projectProgram(t, `
first()
local value = 1 + 2
last()
do return end
dead()
`)
	authored := p.Flow().Authored().Calls()
	var live []identity.ContentID
	dead := false
	for index := 0; index < authored.Count(); index++ {
		term, termOK := authored.At(index)
		if !termOK {
			t.Fatalf("authored Call %d unavailable", index)
		}
		if !p.Flow().Executable().Contains(term) {
			dead = true
			continue
		}
		callID, callOK := callIdentityAt(p, index)
		if !callOK {
			t.Fatalf("executable Program Call identity %d unavailable", index)
		}
		live = append(live, callID)
	}
	if len(live) < 2 || !dead {
		t.Fatalf("fixture Calls live=%d dead=%v, want first/last live and one dead", len(live), dead)
	}
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	shard, ok := draft.Mounts().At(0)
	if !ok {
		t.Fatal("missing Project Shard")
	}
	calls := draft.Applications().Calls()
	if calls.Count() != len(live) {
		t.Fatalf("Project Calls count = %d, want %d", calls.Count(), len(live))
	}
	for index := 0; index < calls.Count(); index++ {
		application, applicationOK := calls.At(index)
		proof, proofOK := calls.ForApplication(application)
		gotShard, gotTerm, rowOK := draft.Applications().Call(application)
		if !applicationOK || !proofOK || !calls.Owns(proof) || !rowOK || gotShard != shard || gotTerm == 0 || !p.Flow().Executable().Contains(gotTerm) {
			t.Fatalf("CallApplication round-trip = %v/%v/%v/%v", gotShard, gotTerm, applicationOK, proofOK)
		}
		wantID := callIDForTerm(t, p, gotTerm)
		if proof.CallID() != wantID || !proof.CallID().Available() {
			t.Fatalf("CallApplication call ID = %x, want %x", proof.CallID(), wantID)
		}
	}
	if _, ok := calls.ForApplication(Application{}); ok {
		t.Fatal("zero Application accepted")
	}
}

// TestCallApplicationFencesMountProjectOwners proves that equal scalar call
// identities remain fenced by exact Project ownership. Duplicate mounts can
// issue distinct Applications for the same reusable Program call ID without
// putting the mount coordinate into the reusable ContextID.
func TestCallApplicationFencesMountProjectAndProgramOwners(t *testing.T) {
	p := projectProgram(t, `run()`)
	replayedProgram := projectProgram(t, `run()`)
	first := projectDraft(t, []Module{{Name: "left", Program: p}, {Name: "right", Program: p}})
	second := projectDraft(t, []Module{{Name: "left", Program: p}, {Name: "right", Program: p}})
	callID := onlyExecutableCallID(t, p)
	replayedCallID := onlyExecutableCallID(t, replayedProgram)
	if callID != replayedCallID {
		t.Fatal("equivalent Program replay renamed scalar call ID")
	}
	leftShard, leftOK := first.Mounts().At(0)
	rightShard, rightOK := first.Mounts().At(1)
	if !leftOK || !rightOK {
		t.Fatal("duplicate Project mounts unavailable")
	}
	left, leftOK := callApplicationForMount(t, first.Applications(), leftShard)
	right, rightOK := callApplicationForMount(t, first.Applications(), rightShard)
	if !leftOK || !rightOK {
		t.Fatal("duplicate mount CallApplication unavailable")
	}
	leftApplication, _ := left.Application()
	rightApplication, _ := right.Application()
	if leftApplication == rightApplication {
		t.Fatal("duplicate mount CallApplications collapsed exact Applications")
	}
	if left.ContextID() != right.ContextID() {
		t.Fatal("mount coordinate leaked into reusable CallApplication ContextID")
	}
	if left.CallID() != callID || right.CallID() != callID {
		t.Fatal("mount coordinate leaked into scalar call ID")
	}
	if allocations := testing.AllocsPerRun(10_000, func() {
		_, applicationOK := left.Application()
		_, mountOK := left.Mount()
		if !first.Applications().Calls().Owns(left) || !left.ContextID().Available() || !left.CallID().Available() || !applicationOK || !mountOK {
			panic("sealed CallApplication became unavailable")
		}
	}); allocations != 0 {
		t.Fatalf("sealed CallApplication hot projections allocations = %g, want 0", allocations)
	}
	if second.Applications().Calls().Owns(left) {
		t.Fatal("foreign Project Calls owner accepted CallApplication")
	}
	if _, ok := first.Applications().Calls().ForApplication(rightApplication); !ok {
		t.Fatal("right Project Application unavailable")
	}
}

// TestCallApplicationIdentityIgnoresUnrelatedApplications covers the
// dependency-local identity law and scalar call-ID validation.
func TestCallApplicationIdentityIgnoresUnrelatedApplications(t *testing.T) {
	mainProgram := projectProgram(t, `main_call()`)
	siblingProgram := projectProgram(t, `sibling_call(); local value = 1 + 2`)
	target := projectTarget(t, "GlobalEnvRoot")
	base := finalizedProject(t, target, []Module{{Name: "main", Program: mainProgram}})
	expanded := finalizedProject(t, target, []Module{{Name: "main", Program: mainProgram}, {Name: "sibling", Program: siblingProgram}})
	mainTerm := callTermAt(t, mainProgram, 0)
	siblingTerm := callTermAt(t, siblingProgram, 0)
	mainCallID := callIDForTerm(t, mainProgram, mainTerm)
	siblingCallID := callIDForTerm(t, siblingProgram, siblingTerm)
	baseApplication, baseApplicationOK := applicationForCallID(t, base, mainCallID)
	expandedMainApplication, expandedMainApplicationOK := applicationForCallID(t, expanded, mainCallID)
	expandedSiblingApplication, expandedSiblingApplicationOK := applicationForCallID(t, expanded, siblingCallID)
	baseProof, baseOK := base.Applications().Calls().ForApplication(baseApplication)
	expandedProof, expandedOK := expanded.Applications().Calls().ForApplication(expandedMainApplication)
	siblingProof, siblingOK := expanded.Applications().Calls().ForApplication(expandedSiblingApplication)
	if !baseApplicationOK || !expandedMainApplicationOK || !expandedSiblingApplicationOK || !baseOK || !expandedOK || !siblingOK {
		t.Fatal("fixture CallApplication proof unavailable")
	}
	if baseProof.ContextID() != expandedProof.ContextID() {
		t.Fatal("unrelated sibling applications renamed CallApplication")
	}
	if baseProof.ContextID() == siblingProof.ContextID() {
		t.Fatal("unrelated Program occurrences collapsed CallApplication identity")
	}
	if baseProof.CallID() == siblingProof.CallID() {
		t.Fatal("unrelated Program calls collapsed scalar call identity")
	}
	siblingApplication, _ := siblingProof.Application()
	hostileSibling := CallApplication{application: siblingApplication, callID: baseProof.CallID(), formal: siblingProof.formal}
	if hostileSibling.Available() || expanded.Applications().Calls().Owns(hostileSibling) {
		t.Fatal("unrelated Application accepted a spliced scalar call ID")
	}
}

func callIDForTerm(t testing.TB, p *program.Program, term keyspace.Term) identity.ContentID {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		candidate, ok := calls.At(index)
		if ok && candidate == term {
			callID, callOK := callIdentityAt(p, index)
			if !callOK {
				t.Fatal("Program Call identity unavailable")
			}
			return callID
		}
	}
	t.Fatal("Program Call term absent")
	return identity.ContentID{}
}

func onlyExecutableCallID(t testing.TB, p *program.Program) identity.ContentID {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	var result identity.ContentID
	for index := 0; index < calls.Count(); index++ {
		term, termOK := calls.At(index)
		callID, callOK := callIdentityAt(p, index)
		if !termOK || !callOK || !p.Flow().Executable().Contains(term) {
			continue
		}
		if result.Available() {
			t.Fatal("fixture has multiple executable Calls")
		}
		result = callID
	}
	if !result.Available() {
		t.Fatal("fixture has no executable Call")
	}
	return result
}

func callTermAt(t testing.TB, p *program.Program, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Flow().Authored().Calls().At(index)
	if !ok {
		t.Fatalf("Program Call %d unavailable", index)
	}
	return term
}

func callApplicationForMount(t testing.TB, applications Applications, shard Shard) (CallApplication, bool) {
	t.Helper()
	calls := applications.Calls()
	for index := 0; index < calls.Count(); index++ {
		application, ok := calls.At(index)
		if !ok {
			continue
		}
		gotShard, _, callOK := applications.Call(application)
		if !callOK || gotShard != shard {
			continue
		}
		return calls.ForApplication(application)
	}
	return CallApplication{}, false
}

func applicationForCallID(t testing.TB, component *Component, callID identity.ContentID) (Application, bool) {
	t.Helper()
	applications := component.Applications()
	calls := applications.Calls()
	for index := 0; index < calls.Count(); index++ {
		application, ok := calls.At(index)
		if !ok {
			continue
		}
		_, _, gotCallID, callOK := calls.MountedIdentity(application)
		if callOK && gotCallID == callID {
			return application, true
		}
	}
	return Application{}, false
}

func finalizedProject(t testing.TB, target *contract.Contract, modules []Module) *Component {
	t.Helper()
	draft, err := Build(Input{Modules: modules, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	return component
}
