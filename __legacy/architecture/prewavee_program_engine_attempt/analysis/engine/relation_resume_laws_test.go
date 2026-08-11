package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// resumeRelationFixture connects Link's typed target Resume projection with
// two distinct, already-existing Program activations. The suspended body and
// the resumer deliberately live in different module roots, so no source
// coordinate can stand in for both operands.
type resumeRelationFixture struct {
	link *link.Link

	suspendedShard     link.Shard
	suspendedCall      program.Term
	suspendedTerm      program.Term
	suspendedApp       link.Application
	suspendedCandidate link.Candidate

	resumerShard     link.Shard
	resumerCall      program.Term
	resumerCandidate link.Candidate
	resumeApp        link.Application
}

func resumeRelationFixtureFor(t testing.TB) resumeRelationFixture {
	t.Helper()
	suspended, err := programlower.Lower(programlower.Source{
		Name: "suspended.lua",
		Text: []byte(`
local function suspended() return 0 end
suspended()
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	resumer, err := programlower.Lower(programlower.Source{
		Name: "resumer.lua",
		Text: []byte(`resume_op(nil)`),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"resume_op"}}},
		ValuesVars: 1,
		Input: target.ValuesSpec{
			Fixed: []typ.Type{typ.Any}, Tail: target.ValuesVariable, Var: 0,
		},
		Outcomes: []target.OutcomeSpec{{
			Kind: program.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesVariable, Var: 0},
		}},
		Resumes: []target.ResumeSpec{{
			Source: target.ResumeSourceValueFormal, Carrier: 0, Arguments: 0,
			Outcomes: []target.ResumeOutcomeSpec{
				{Kind: program.OutcomeNormal, Outcome: 0},
				{Kind: program.OutcomeReturn, Outcome: 0},
				{Kind: program.OutcomeThrow, Outcome: 0},
				{Kind: program.OutcomeYield, Outcome: 0},
				{Kind: program.OutcomeCancel, Outcome: 0},
			},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{
		{Name: "suspended", Program: suspended},
		{Name: "resumer", Program: resumer},
	}})
	if err != nil {
		t.Fatal(err)
	}
	suspendedShard := resumeRelationShard(t, project, suspended)
	resumerShard := resumeRelationShard(t, project, resumer)
	suspendedCall, ok := suspended.CallAt(0)
	if !ok {
		t.Fatal("missing suspended Call")
	}
	resumerCall, ok := resumer.CallAt(0)
	if !ok {
		t.Fatal("missing resumer Call")
	}
	suspendedApp := resumeRelationCallApplication(t, project, suspendedShard, suspendedCall)
	resumerApp := resumeRelationCallApplication(t, project, resumerShard, resumerCall)
	function, ok := suspended.FunctionAt(0)
	if !ok {
		t.Fatal("missing suspended Function")
	}
	suspendedCandidate, ok := project.CandidateForFunction(suspendedApp, suspendedShard, function)
	if !ok {
		t.Fatal("missing suspended Body Candidate")
	}
	_, suspendedBody, ok := project.CandidateBody(suspendedCandidate)
	if !ok {
		t.Fatal("missing suspended Body activation")
	}
	suspendedTerm := resumeFunctionReturn(t, suspended, suspendedBody)
	operation, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"resume_op"}})
	if !ok {
		t.Fatal("missing resume operation")
	}
	seed, ok := project.SeedForOperation(operation)
	if !ok {
		t.Fatal("missing resume operation Seed")
	}
	resumerCandidate, ok := project.CandidateForSeed(resumerApp, seed)
	if !ok {
		t.Fatal("missing resumer Candidate")
	}
	resume, ok := contract.ResumeIDAt(operation, 0)
	if !ok {
		t.Fatal("missing target ResumeID")
	}
	owner, source, carrier, arguments, ok := contract.Resume(resume)
	if !ok || owner != operation || source != target.ResumeSourceValueFormal || carrier != 0 || arguments != 0 {
		t.Fatal("invalid typed target resume projection")
	}
	resumeApp, ok := project.CandidateResume(resumerCandidate, resume)
	if !ok {
		t.Fatal("missing Link resume projection")
	}
	shard, term, selectedOperation, selectedResume, ok := project.ResumeApplication(resumeApp)
	if !ok || shard != resumerShard || term != resumerCall || selectedOperation != operation || selectedResume != resume {
		t.Fatal("invalid typed Link resume projection")
	}
	return resumeRelationFixture{
		link: project,

		suspendedShard: suspendedShard, suspendedCall: suspendedCall,
		suspendedTerm: suspendedTerm, suspendedApp: suspendedApp, suspendedCandidate: suspendedCandidate,

		resumerShard: resumerShard, resumerCall: resumerCall,
		resumerCandidate: resumerCandidate, resumeApp: resumeApp,
	}
}

func resumeRelationShard(t testing.TB, project *link.Link, wanted *program.Program) link.Shard {
	t.Helper()
	for index := 0; index < project.ShardCount(); index++ {
		shard, ok := project.ShardAt(index)
		if !ok {
			continue
		}
		value, ok := project.Program(shard)
		if ok && value == wanted {
			return shard
		}
	}
	t.Fatal("missing Program Shard")
	return 0
}

func resumeRelationCallApplication(t testing.TB, project *link.Link, shard link.Shard, term program.Term) link.Application {
	t.Helper()
	for index := 0; index < project.ApplicationCount(); index++ {
		application, ok := project.ApplicationAt(index)
		if !ok {
			continue
		}
		applicationShard, applicationTerm, ok := project.CallApplication(application)
		if ok && applicationShard == shard && applicationTerm == term {
			return application
		}
	}
	t.Fatal("missing Call Application")
	return link.Application{}
}

func resumeFunctionReturn(t testing.TB, value *program.Program, body program.Term) program.Term {
	t.Helper()
	for index := 0; index < value.ReturnCount(); index++ {
		term, ok := value.ReturnAt(index)
		if !ok {
			continue
		}
		activation, active := value.Activation(term)
		if active && activation == body {
			return term
		}
	}
	t.Fatal("missing existing suspended Return occurrence")
	return 0
}

// TestRelationResumeNonPrimaryReentryIsRejected proves the current boundary:
// a resume relation that re-enters a non-primary suspended body is not a
// proved Program/Link recurrence. It must publish no State until Program/Link
// carries a presealed continuation correspondence for that re-entry.
func TestRelationResumeNonPrimaryReentryIsRejected(t *testing.T) {
	fixture := resumeRelationFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	inputs := relationFactor(t, solver, "resume-inputs")
	forward := relationFactor(t, solver, "resume-forward")
	reversed := relationFactor(t, solver, "resume-reversed")
	trigger := relationFactor(t, solver, "resume-trigger")

	suspendRule := resumeSuspensionRelation(t, solver, inputs, fixture.suspendedApp)
	forwardRule := resumeOrderedRelation(t, solver, forward, inputs, fixture.resumeApp, "resume-forward")
	reversedRule := resumeOrderedRelation(t, solver, reversed, inputs, fixture.resumeApp, "resume-reversed")

	declareAt(t, solver, inputs, "rule/resume-suspended-input", fixture.suspendedShard, fixture.suspendedCall, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x03)
	})
	declareAt(t, solver, inputs, "rule/resume-resumer-input", fixture.resumerShard, fixture.resumerCall, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x0c)
	})
	declareAt(t, solver, trigger, "rule/resume-suspend-select", fixture.suspendedShard, fixture.suspendedCall, func(access engine.Access[uint64, uint8]) bool {
		return engine.Activate(access, suspendRule, fixture.suspendedCandidate, func(relation engine.Relation) bool {
			caller, callerOK := relation.Caller(fixture.suspendedCall)
			output, outputOK := relation.Selected(fixture.suspendedTerm)
			return callerOK && outputOK && relation.Bind(output, caller)
		})
	})
	declareAt(t, solver, trigger, "rule/resume-select", fixture.resumerShard, fixture.resumerCall, func(access engine.Access[uint64, uint8]) bool {
		forwardOK := engine.Activate(access, forwardRule, fixture.resumerCandidate, func(relation engine.Relation) bool {
			return bindResumeOperands(relation, fixture.suspendedCandidate, fixture.suspendedTerm, fixture.resumerCall, false)
		})
		reversedOK := engine.Activate(access, reversedRule, fixture.resumerCandidate, func(relation engine.Relation) bool {
			return bindResumeOperands(relation, fixture.suspendedCandidate, fixture.suspendedTerm, fixture.resumerCall, true)
		})
		return forwardOK && reversedOK
	})
	if _, ok := engine.DeclareQuery(solver, trigger, fixture.suspendedShard, fixture.suspendedCall, 0); !ok {
		t.Fatal("declare suspended selector engine.Query")
	}
	if _, ok := engine.DeclareQuery(solver, trigger, fixture.resumerShard, fixture.resumerCall, 0); !ok {
		t.Fatal("declare resumer selector engine.Query")
	}
	if _, ok := engine.DeclareCandidateQuery(solver, inputs, fixture.suspendedCandidate, fixture.suspendedShard, fixture.suspendedTerm, 0); !ok {
		t.Fatal("declare suspended Body engine.Query")
	}
	if _, ok := engine.DeclareQuery(solver, inputs, fixture.resumerShard, fixture.resumerCall, 0); !ok {
		t.Fatal("declare resumer input engine.Query")
	}
	if _, ok := engine.DeclareCandidateQuery(solver, forward, fixture.suspendedCandidate, fixture.suspendedShard, fixture.suspendedTerm, 0); !ok {
		t.Fatal("declare forward resume engine.Query")
	}
	if _, ok := engine.DeclareCandidateQuery(solver, reversed, fixture.suspendedCandidate, fixture.suspendedShard, fixture.suspendedTerm, 0); !ok {
		t.Fatal("declare reversed resume engine.Query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	if state, solved := solver.Solve(context.Background(), nil); solved || state != nil {
		t.Fatal("unproved non-primary resume re-entry published a State")
	}
}

func resumeOrderedRelation(t testing.TB, solver *engine.Solver, output, input *engine.Factor[uint64, uint8], application link.Application, label string) *engine.Rule[uint64, uint8] {
	t.Helper()
	var first, second engine.ReadRef[uint64, uint8]
	rule, ok := engine.DeclareRule(solver, output, relationSemantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.Relation(application, 2)
	}, func(access engine.Access[uint64, uint8]) bool {
		left, leftPresent, leftOK := engine.ReadAt(access, first, 0)
		right, rightPresent, rightOK := engine.ReadAt(access, second, 0)
		if !leftOK || !rightOK || !leftPresent || !rightPresent {
			return false
		}
		return access.Set(0, left<<4|right)
	})
	if !ok {
		t.Fatalf("engine.DeclareRule(%s)", label)
	}
	first, ok = engine.Read(rule, 0, input)
	if !ok {
		t.Fatalf("declare first engine.ReadRef(%s)", label)
	}
	second, ok = engine.Read(rule, 1, input)
	if !ok {
		t.Fatalf("declare second engine.ReadRef(%s)", label)
	}
	return rule
}

func resumeSuspensionRelation(t testing.TB, solver *engine.Solver, value *engine.Factor[uint64, uint8], application link.Application) *engine.Rule[uint64, uint8] {
	t.Helper()
	var caller engine.ReadRef[uint64, uint8]
	rule, ok := engine.DeclareRule(solver, value, relationSemantic("rule/resume-suspension"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(application, 1)
	}, func(access engine.Access[uint64, uint8]) bool {
		return engine.Carry(access, caller)
	})
	if !ok {
		t.Fatal("engine.DeclareRule(resume-suspension)")
	}
	caller, ok = engine.Read(rule, 0, value)
	if !ok {
		t.Fatal("declare suspension engine.ReadRef")
	}
	return rule
}

// bindResumeOperands uses only existing Program activation references. The
// output remains at the suspended body's activation; operand order is the
// behavior under test rather than a synthesized resume endpoint.
func bindResumeOperands(relation engine.Relation, candidate link.Candidate, body, caller program.Term, reverse bool) bool {
	suspended, suspendedOK := relation.Body(candidate, body)
	resumer, resumerOK := relation.Caller(caller)
	if !suspendedOK || !resumerOK {
		return false
	}
	if reverse {
		return relation.Bind(suspended, resumer, suspended)
	}
	return relation.Bind(suspended, suspended, resumer)
}
