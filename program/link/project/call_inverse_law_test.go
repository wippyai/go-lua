package project

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

// TestCallsForOccurrenceReissuesOnlyExecutableProgramProofs exercises the
// exact mounted inverse at both ends of the Program Call denominator. A dead
// Call remains a valid Program occurrence but is absent from Project.
func TestCallsForOccurrenceReissuesOnlyExecutableProgramProofs(t *testing.T) {
	p := projectProgram(t, `
first()
local value = 1 + 2
last()
do return end
dead()
`)
	input := p.TransformerInput()
	authored := p.Flow().Authored().Calls()
	var live []program.CallOccurrence
	dead := false
	for index := 0; index < input.CallCount(); index++ {
		term, termOK := authored.At(index)
		if !termOK {
			t.Fatalf("authored Call %d unavailable", index)
		}
		if !p.Flow().Executable().Contains(term) {
			dead = true
			continue
		}
		occurrence, occurrenceOK := input.CallAt(index)
		if !occurrenceOK {
			t.Fatalf("executable Program CallOccurrence %d unavailable", index)
		}
		live = append(live, occurrence)
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
	first, firstOK := calls.ForOccurrence(shard, live[0])
	last, lastOK := calls.ForOccurrence(shard, live[len(live)-1])
	if !firstOK || !lastOK || !calls.Owns(first) || !calls.Owns(last) {
		t.Fatalf("first/last inverse = %#v/%v %#v/%v", first, firstOK, last, lastOK)
	}
	firstApplication, firstApplicationOK := first.Application()
	lastApplication, lastApplicationOK := last.Application()
	if !firstApplicationOK || !lastApplicationOK || firstApplication == lastApplication {
		t.Fatal("distinct Program Call proofs collapsed to one Application")
	}
	for _, proof := range []CallApplication{first, last} {
		application, applicationOK := proof.Application()
		occurrence, occurrenceOK := proof.Occurrence()
		gotShard, gotTerm, rowOK := draft.Applications().Call(application)
		if !applicationOK || !occurrenceOK || !rowOK || gotShard != shard || gotTerm == 0 || !p.Flow().Executable().Contains(gotTerm) || !occurrence.Equal(callOccurrenceForTerm(t, p, gotTerm)) {
			t.Fatalf("CallApplication round-trip = %v/%v/%v/%v", gotShard, gotTerm, applicationOK, occurrenceOK)
		}
	}
	if _, ok := calls.ForOccurrence(shard, program.CallOccurrence{}); ok {
		t.Fatal("zero CallOccurrence accepted")
	}
}

// TestCallApplicationFencesMountProjectAndProgramOwners proves that equal
// scalar occurrence identities never substitute for exact hot ownership.
// Duplicate mounts can issue distinct Applications for the same exact Program
// proof without putting the mount coordinate into the reusable ContextID.
func TestCallApplicationFencesMountProjectAndProgramOwners(t *testing.T) {
	p := projectProgram(t, `run()`)
	replayedProgram := projectProgram(t, `run()`)
	first := projectDraft(t, []Module{{Name: "left", Program: p}, {Name: "right", Program: p}})
	second := projectDraft(t, []Module{{Name: "left", Program: p}, {Name: "right", Program: p}})
	occurrence := onlyExecutableCallOccurrence(t, p)
	replayedOccurrence := onlyExecutableCallOccurrence(t, replayedProgram)
	if occurrence.ContextID() != replayedOccurrence.ContextID() {
		t.Fatal("equivalent Program replay renamed CallOccurrence")
	}
	leftShard, leftOK := first.Mounts().At(0)
	rightShard, rightOK := first.Mounts().At(1)
	foreignShard, foreignOK := second.Mounts().At(0)
	if !leftOK || !rightOK || !foreignOK {
		t.Fatal("duplicate Project mounts unavailable")
	}
	left, leftOK := first.Applications().Calls().ForOccurrence(leftShard, occurrence)
	right, rightOK := first.Applications().Calls().ForOccurrence(rightShard, occurrence)
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
	if allocations := testing.AllocsPerRun(10_000, func() {
		_, applicationOK := left.Application()
		_, occurrenceOK := left.Occurrence()
		_, mountOK := left.Mount()
		if !first.Applications().Calls().Owns(left) || !left.ContextID().Available() || !applicationOK || !occurrenceOK || !mountOK {
			panic("sealed CallApplication became unavailable")
		}
	}); allocations != 0 {
		t.Fatalf("sealed CallApplication hot projections allocations = %g, want 0", allocations)
	}
	if _, ok := first.Applications().Calls().ForOccurrence(foreignShard, occurrence); ok {
		t.Fatal("equivalent foreign Project Shard crossed owner fence")
	}
	if _, ok := first.Applications().Calls().ForOccurrence(leftShard, replayedOccurrence); ok {
		t.Fatal("equivalent replay Program proof crossed mounted Program fence")
	}
	if second.Applications().Calls().Owns(left) {
		t.Fatal("foreign Project Calls owner accepted CallApplication")
	}
	hostileReplay := CallApplication{application: leftApplication, occurrence: replayedOccurrence, formal: left.formal}
	if hostileReplay.Available() || first.Applications().Calls().Owns(hostileReplay) || hostileReplay.ContextID().Available() {
		t.Fatal("equal-ID replay occurrence was spliced into exact Project proof")
	}
}

// TestCallApplicationIdentityIgnoresUnrelatedApplicationsButRejectsTheir
// proofs covers the dependency-local identity law and hostile sibling splice.
func TestCallApplicationIdentityIgnoresUnrelatedApplicationsButRejectsTheirProofs(t *testing.T) {
	mainProgram := projectProgram(t, `main_call()`)
	siblingProgram := projectProgram(t, `sibling_call(); local value = 1 + 2`)
	target := projectTarget(t, "GlobalEnvRoot")
	base := finalizedProject(t, target, []Module{{Name: "main", Program: mainProgram}})
	expanded := finalizedProject(t, target, []Module{{Name: "main", Program: mainProgram}, {Name: "sibling", Program: siblingProgram}})
	mainOccurrence := onlyExecutableCallOccurrence(t, mainProgram)
	siblingOccurrence := onlyExecutableCallOccurrence(t, siblingProgram)
	baseShard := shardForProgram(t, base, mainProgram)
	expandedMainShard := shardForProgram(t, expanded, mainProgram)
	expandedSiblingShard := shardForProgram(t, expanded, siblingProgram)
	baseProof, baseOK := base.Applications().Calls().ForOccurrence(baseShard, mainOccurrence)
	expandedProof, expandedOK := expanded.Applications().Calls().ForOccurrence(expandedMainShard, mainOccurrence)
	siblingProof, siblingOK := expanded.Applications().Calls().ForOccurrence(expandedSiblingShard, siblingOccurrence)
	if !baseOK || !expandedOK || !siblingOK {
		t.Fatal("fixture CallApplication proof unavailable")
	}
	if baseProof.ContextID() != expandedProof.ContextID() {
		t.Fatal("unrelated sibling applications renamed CallApplication")
	}
	if baseProof.ContextID() == siblingProof.ContextID() {
		t.Fatal("unrelated Program occurrences collapsed CallApplication identity")
	}
	if _, ok := expanded.Applications().Calls().ForOccurrence(expandedMainShard, siblingOccurrence); ok {
		t.Fatal("sibling Program occurrence crossed exact mount fence")
	}
	siblingApplication, _ := siblingProof.Application()
	hostileSibling := CallApplication{application: siblingApplication, occurrence: mainOccurrence, formal: siblingProof.formal}
	if hostileSibling.Available() || expanded.Applications().Calls().Owns(hostileSibling) {
		t.Fatal("unrelated Application accepted a spliced occurrence proof")
	}
}

func callOccurrenceForTerm(t testing.TB, p *program.Program, term keyspace.Term) program.CallOccurrence {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	input := p.TransformerInput()
	for index := 0; index < calls.Count(); index++ {
		candidate, ok := calls.At(index)
		if ok && candidate == term {
			occurrence, occurrenceOK := input.CallAt(index)
			if !occurrenceOK {
				t.Fatal("Program CallOccurrence unavailable")
			}
			return occurrence
		}
	}
	t.Fatal("Program Call term absent")
	return program.CallOccurrence{}
}

func onlyExecutableCallOccurrence(t testing.TB, p *program.Program) program.CallOccurrence {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	input := p.TransformerInput()
	var result program.CallOccurrence
	for index := 0; index < calls.Count(); index++ {
		term, termOK := calls.At(index)
		occurrence, occurrenceOK := input.CallAt(index)
		if !termOK || !occurrenceOK || !p.Flow().Executable().Contains(term) {
			continue
		}
		if result.Available() {
			t.Fatal("fixture has multiple executable Calls")
		}
		result = occurrence
	}
	if !result.Available() {
		t.Fatal("fixture has no executable Call")
	}
	return result
}

func finalizedProject(t testing.TB, target *target.Contract, modules []Module) *Component {
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

func shardForProgram(t testing.TB, component *Component, owner *program.Program) Shard {
	t.Helper()
	for index := 0; index < component.Mounts().Count(); index++ {
		shard, shardOK := component.Mounts().At(index)
		mounted, mountedOK := component.Mounts().Program(shard)
		if shardOK && mountedOK && mounted == owner {
			return shard
		}
	}
	t.Fatal("mounted Program absent")
	return Shard{}
}
