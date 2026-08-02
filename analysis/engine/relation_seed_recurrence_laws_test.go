package engine_test

import (
	"context"
	"crypto/sha256"
	"math/bits"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// seedRecurrenceBits is deliberately finite: these laws are about the
// semantic fixed point of public relations, not a scheduler iteration count.
type seedRecurrenceBits uint8

const (
	seedRecurrenceOne  seedRecurrenceBits = 1
	seedRecurrenceTwo  seedRecurrenceBits = 2
	seedRecurrenceBoth                    = seedRecurrenceOne | seedRecurrenceTwo
)

func seedRecurrenceLattice() lattice.Lattice[seedRecurrenceBits] {
	return lattice.Lattice[seedRecurrenceBits]{
		Bottom: func() seedRecurrenceBits { return 0 },
		Top:    func() seedRecurrenceBits { return seedRecurrenceBoth },
		Equal: func(left, right seedRecurrenceBits) bool {
			return left == right
		},
		LessOrEq: func(left, right seedRecurrenceBits) bool {
			return left&^right == 0
		},
		Join: func(left, right seedRecurrenceBits) seedRecurrenceBits {
			return left | right
		},
		Meet: func(left, right seedRecurrenceBits) seedRecurrenceBits {
			return left & right
		},
		Widen: func(left, right seedRecurrenceBits) seedRecurrenceBits {
			return left | right
		},
	}
}

func seedRecurrenceSemantic(label string) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID(sha256.Sum256([]byte("seed-recurrence-law/" + label))), Version: 1}
}

func seedRecurrenceFactor(t testing.TB, solver *engine.Solver, label string) *engine.Factor[uint64, seedRecurrenceBits] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, seedRecurrenceBits]{
		Keys:     engine.KeySpace{End: 1},
		Semantic: seedRecurrenceSemantic("factor/" + label),
		Lattice:  seedRecurrenceLattice(),
		Default:  0,
		Fingerprint: func(value seedRecurrenceBits) uint64 {
			return uint64(value)
		},
		WidenRank: engine.Measure[uint64, seedRecurrenceBits]{
			Width: 1,
			At: func(_ uint64, value seedRecurrenceBits, _ int) uint64 {
				return uint64(8 - bits.OnesCount8(uint8(value)))
			},
		},
	})
	if !ok {
		t.Fatalf("DeclareFactor(%s)", label)
	}
	return factor
}

func seedRecurrenceContract(t testing.TB, name string) *target.Contract {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{name}}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: program.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatalf("seal target Contract: %v", err)
	}
	return contract
}

func seedRecurrenceProgramAndLink(t testing.TB, source string, contract *target.Contract) (*program.Program, *link.Link, link.Shard) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "relation-seed-recurrence.lua", Text: []byte(source)})
	if err != nil {
		t.Fatalf("lower Program: %v", err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatalf("seal Link: %v", err)
	}
	for index := 0; index < project.ShardCount(); index++ {
		shard, ok := project.ShardAt(index)
		if !ok {
			continue
		}
		candidate, ok := project.Program(shard)
		if ok && candidate == p {
			return p, project, shard
		}
	}
	t.Fatal("sealed Link did not retain its Program")
	return nil, nil, 0
}

func seedRecurrenceCallApplication(t testing.TB, project *link.Link, shard link.Shard, call program.Term) link.Application {
	t.Helper()
	for index := 0; index < project.ApplicationCount(); index++ {
		application, ok := project.ApplicationAt(index)
		if !ok {
			continue
		}
		gotShard, gotCall, ok := project.CallApplication(application)
		if ok && gotShard == shard && gotCall == call {
			return application
		}
	}
	t.Fatal("missing canonical Call Application")
	return link.Application{}
}

func seedRecurrenceCandidate(t testing.TB, project *link.Link, contract *target.Contract, application link.Application, name string) link.Candidate {
	t.Helper()
	operation, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{name}})
	if !ok {
		t.Fatalf("missing target operation %q", name)
	}
	seed, ok := project.SeedForOperation(operation)
	if !ok {
		t.Fatalf("missing Link Seed for %q", name)
	}
	candidate, ok := project.CandidateForSeed(application, seed)
	if !ok {
		t.Fatalf("missing Seed Candidate for %q", name)
	}
	return candidate
}

func seedRecurrenceEntry(t testing.TB, p *program.Program) program.Term {
	t.Helper()
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	return entry
}

func seedRecurrenceDeclareAt(t testing.TB, solver *engine.Solver, output *engine.Factor[uint64, seedRecurrenceBits], label string, shard link.Shard, at program.Term, run func(engine.Access[uint64, seedRecurrenceBits]) bool) *engine.Rule[uint64, seedRecurrenceBits] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, output, seedRecurrenceSemantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, at)
	}, run)
	if !ok {
		t.Fatalf("DeclareRule(At, %s)", label)
	}
	return rule
}

func seedRecurrenceRead(t testing.TB, query *engine.Query[uint64, seedRecurrenceBits], state *engine.State) seedRecurrenceBits {
	t.Helper()
	value, present := query.Read(state)
	if !present {
		t.Fatal("Query has no semantic value")
	}
	return value
}

func seedRecurrenceSolve(t testing.TB, solver *engine.Solver) *engine.State {
	t.Helper()
	if !solver.Seal() {
		t.Fatal("Solver.Seal rejected the law")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solver.Solve did not publish a State")
	}
	return state
}

// Seed Candidates name a target Seed, not a synthetic Program body. They can
// nevertheless carry a forward relation from an existing caller to its
// canonical continuation; they do not create a synthetic body or a feedback
// edge to the activation root.
func TestRelationSeedCandidateHasNoFabricatedBody(t *testing.T) {
	contract := seedRecurrenceContract(t, "op")
	p, project, shard := seedRecurrenceProgramAndLink(t, "op(); local done = 1", contract)
	entry := seedRecurrenceEntry(t, p)
	exit, ok := p.BodyNormalExit(entry)
	if !ok {
		t.Fatal("Program has no normal continuation")
	}
	call, ok := p.CallAt(0)
	if !ok {
		t.Fatal("Program has no operation Call")
	}
	application := seedRecurrenceCallApplication(t, project, shard, call)
	candidate := seedRecurrenceCandidate(t, project, contract, application, "op")
	if _, _, ok := project.CandidateBody(candidate); ok {
		t.Fatal("CandidateSeed exposed a Program body")
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatalf("new Solver: %v", err)
	}
	trigger := seedRecurrenceFactor(t, solver, "seed-trigger")
	result := seedRecurrenceFactor(t, solver, "seed-result")
	relation, ok := engine.DeclareRule(solver, result, seedRecurrenceSemantic("rule/seed-target"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(application, 2)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return access.Set(0, seedRecurrenceOne)
	})
	if !ok {
		t.Fatal("DeclareRule(Relation seed target)")
	}
	seedRecurrenceDeclareAt(t, solver, trigger, "seed-selector", shard, call, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return engine.Activate(access, relation, candidate, func(resolver engine.Relation) bool {
			if _, ok := resolver.Selected(entry); ok {
				return false
			}
			if _, ok := resolver.Body(candidate, entry); ok {
				return false
			}
			caller, callerOK := resolver.Caller(call)
			continuation, continuationOK := resolver.Caller(exit)
			root, rootOK := resolver.Root(shard, entry)
			return callerOK && continuationOK && rootOK && resolver.Bind(continuation, caller, root)
		})
	})
	query, ok := engine.DeclareQuery(solver, result, shard, exit, 0)
	if !ok {
		t.Fatal("DeclareQuery(seed relation result)")
	}

	state := seedRecurrenceSolve(t, solver)
	if got := seedRecurrenceRead(t, query, state); got != seedRecurrenceOne {
		t.Fatalf("caller/continuation Seed relation result = %d, want %d", got, seedRecurrenceOne)
	}
}

// A dynamically selected Seed relation has no static Link recurrence witness,
// yet its finite compiled equation still converges after discovery and epoch
// rebuild. No synthetic Program or Link recurrence is required.
func TestRelationDynamicSeedCycleConvergesWithoutStaticRecurrence(t *testing.T) {
	contract := seedRecurrenceContract(t, "op")
	p, project, shard := seedRecurrenceProgramAndLink(t, "local f = missing; f()", contract)
	entry := seedRecurrenceEntry(t, p)
	call, ok := p.CallAt(0)
	if !ok {
		t.Fatal("Program has no dynamic Call")
	}
	application := seedRecurrenceCallApplication(t, project, shard, call)
	candidate := seedRecurrenceCandidate(t, project, contract, application, "op")
	if _, _, ok := project.CandidateBody(candidate); ok {
		t.Fatal("dynamic Seed Candidate exposed a Program body")
	}
	if _, ok := project.ApplicationRecurrence(application); ok {
		t.Fatal("dynamic Call Application exposed a static recurrence witness")
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatalf("new Solver: %v", err)
	}
	trigger := seedRecurrenceFactor(t, solver, "dynamic-trigger")
	cycle := seedRecurrenceFactor(t, solver, "dynamic-cycle")
	result := seedRecurrenceFactor(t, solver, "dynamic-result")
	var self engine.ReadRef[uint64, seedRecurrenceBits]
	cycleRelation, ok := engine.DeclareRule(solver, cycle, seedRecurrenceSemantic("rule/dynamic-cycle"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(application, 1)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		prior, present, valid := engine.ReadAt(access, self, 0)
		if !valid {
			return false
		}
		next := seedRecurrenceOne
		if present {
			next = prior | seedRecurrenceTwo
		}
		return access.Set(0, next)
	})
	if !ok {
		t.Fatal("DeclareRule(dynamic cyclic relation)")
	}
	self, ok = engine.Read(cycleRelation, 0, cycle)
	if !ok {
		t.Fatal("Read(dynamic cyclic relation)")
	}
	var carried engine.ReadRef[uint64, seedRecurrenceBits]
	resultRelation, ok := engine.DeclareRule(solver, result, seedRecurrenceSemantic("rule/dynamic-result"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(application, 1)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		value, present, valid := engine.ReadAt(access, carried, 0)
		if !valid {
			return false
		}
		return !present || access.Set(0, value)
	})
	if !ok {
		t.Fatal("DeclareRule(dynamic result relation)")
	}
	carried, ok = engine.Read(resultRelation, 0, cycle)
	if !ok {
		t.Fatal("Read(dynamic result relation)")
	}
	seedRecurrenceDeclareAt(t, solver, trigger, "dynamic-selector", shard, call, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		cycleOK := engine.Activate(access, cycleRelation, candidate, func(resolver engine.Relation) bool {
			caller, ok := resolver.Caller(call)
			return ok && resolver.Bind(caller, caller)
		})
		resultOK := engine.Activate(access, resultRelation, candidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(call)
			root, rootOK := resolver.Root(shard, entry)
			return callerOK && rootOK && resolver.Bind(root, caller)
		})
		return cycleOK && resultOK
	})
	query, ok := engine.DeclareQuery(solver, result, shard, entry, 0)
	if !ok {
		t.Fatal("DeclareQuery(dynamic cyclic result)")
	}

	if !solver.Seal() {
		t.Fatal("Solver.Seal rejected the unresolved dynamic relation topology")
	}
	state, solved := solver.Solve(context.Background(), nil)
	if !solved || state == nil {
		t.Fatal("Solve did not converge the dynamic Seed cycle")
	}
	if got := seedRecurrenceRead(t, query, state); got != seedRecurrenceBoth {
		t.Fatalf("dynamic Seed fixed point = %d, want %d", got, seedRecurrenceBoth)
	}
}

func seedRecurrenceFunction(t testing.TB, p *program.Program, index int) (program.Term, program.Term, program.Term) {
	t.Helper()
	function, ok := p.FunctionAt(index)
	if !ok {
		t.Fatalf("Program has no Function %d", index)
	}
	_, body, _, ok := p.Function(function)
	if !ok {
		t.Fatalf("Program Function %d has no Body", index)
	}
	entry, ok := p.BodyEntry(body)
	if !ok {
		t.Fatalf("Program Function %d Body has no entry", index)
	}
	return function, body, entry
}

func seedRecurrenceCallInActivation(t testing.TB, p *program.Program, activation program.Term, ordinal int) program.Term {
	t.Helper()
	found := 0
	for index := 0; index < p.CallCount(); index++ {
		call, ok := p.CallAt(index)
		if !ok {
			continue
		}
		owner, ok := p.Activation(call)
		if !ok || owner != activation {
			continue
		}
		if found == ordinal {
			return call
		}
		found++
	}
	t.Fatalf("activation %v has no Call %d", activation, ordinal)
	return 0
}

func seedRecurrenceBodyCandidate(t testing.TB, project *link.Link, application link.Application, shard link.Shard, function program.Term) link.Candidate {
	t.Helper()
	candidate, ok := project.CandidateForFunction(application, shard, function)
	if !ok {
		t.Fatal("missing static Body Candidate")
	}
	return candidate
}

// A selected f-body entry is a reset only for f's candidate-qualified
// activation decisions.  It must not erase g's atom merely because f and g
// participate in one ordinary Factor component; a g boundary is required to
// reset g. Otherwise f's tag could combine with g's false exit and publish
// the forbidden root result below.
func TestRelationSelectedEntryResetDoesNotEraseOtherCandidate(t *testing.T) {
	contract := seedRecurrenceContract(t, "op")
	p, project, shard := seedRecurrenceProgramAndLink(t, `
local function f()
  if unknown then return f() end
end
local function g()
  if unknown then return g() end
end
f()
g()
`, contract)
	root := seedRecurrenceEntry(t, p)
	fFunction, fBody, fEntry := seedRecurrenceFunction(t, p, 0)
	gFunction, gBody, gEntry := seedRecurrenceFunction(t, p, 1)
	fCall := seedRecurrenceCallInActivation(t, p, fBody, 0)
	gCall := seedRecurrenceCallInActivation(t, p, gBody, 0)
	rootFCall := seedRecurrenceCallInActivation(t, p, root, 0)
	rootGCall := seedRecurrenceCallInActivation(t, p, root, 1)
	fApplication := seedRecurrenceCallApplication(t, project, shard, fCall)
	gApplication := seedRecurrenceCallApplication(t, project, shard, gCall)
	rootFApplication := seedRecurrenceCallApplication(t, project, shard, rootFCall)
	rootGApplication := seedRecurrenceCallApplication(t, project, shard, rootGCall)
	fCandidate := seedRecurrenceBodyCandidate(t, project, fApplication, shard, fFunction)
	gCandidate := seedRecurrenceBodyCandidate(t, project, gApplication, shard, gFunction)
	rootFCandidate := seedRecurrenceBodyCandidate(t, project, rootFApplication, shard, fFunction)
	rootGCandidate := seedRecurrenceBodyCandidate(t, project, rootGApplication, shard, gFunction)
	gExit, ok := p.BodyNormalExit(gBody)
	if !ok {
		t.Fatal("g has no normal exit for its false branch")
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatalf("new Solver: %v", err)
	}
	trigger := seedRecurrenceFactor(t, solver, "static-trigger")
	tagTrigger := seedRecurrenceFactor(t, solver, "static-tag-trigger")
	tag := seedRecurrenceFactor(t, solver, "static-tag")
	leak := seedRecurrenceFactor(t, solver, "static-leak")

	startF, ok := engine.DeclareRule(solver, tag, seedRecurrenceSemantic("rule/static-start-f"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(rootFApplication, 1)
	}, func(engine.Access[uint64, seedRecurrenceBits]) bool { return true })
	if !ok {
		t.Fatal("DeclareRule(static f activation)")
	}
	startG, ok := engine.DeclareRule(solver, tag, seedRecurrenceSemantic("rule/static-start-g"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(rootGApplication, 1)
	}, func(engine.Access[uint64, seedRecurrenceBits]) bool { return true })
	if !ok {
		t.Fatal("DeclareRule(static g activation)")
	}
	var fCross, gCross engine.ReadRef[uint64, seedRecurrenceBits]
	fRecurrence, ok := engine.DeclareRule(solver, tag, seedRecurrenceSemantic("rule/static-recurrence-f"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fApplication, 2)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		_, _, valid := engine.ReadAt(access, fCross, 0)
		return valid && access.Set(0, seedRecurrenceOne)
	})
	if !ok {
		t.Fatal("DeclareRule(static f recurrence)")
	}
	fCross, ok = engine.Read(fRecurrence, 1, tag)
	if !ok {
		t.Fatal("Read(static f cross dependency)")
	}
	gRecurrence, ok := engine.DeclareRule(solver, tag, seedRecurrenceSemantic("rule/static-recurrence-g"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(gApplication, 2)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		_, _, valid := engine.ReadAt(access, gCross, 0)
		return valid && access.Set(0, seedRecurrenceTwo)
	})
	if !ok {
		t.Fatal("DeclareRule(static g recurrence)")
	}
	gCross, ok = engine.Read(gRecurrence, 1, tag)
	if !ok {
		t.Fatal("Read(static g cross dependency)")
	}
	leakRelation, ok := engine.DeclareRule(solver, leak, seedRecurrenceSemantic("rule/static-leak"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fApplication, 1)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return access.Set(0, seedRecurrenceOne)
	})
	if !ok {
		t.Fatal("DeclareRule(static discharge sentinel)")
	}

	seedRecurrenceDeclareAt(t, solver, trigger, "static-root-f-selector", shard, rootFCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return engine.Activate(access, startF, rootFCandidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(rootFCall)
			selected, selectedOK := resolver.Selected(fEntry)
			return callerOK && selectedOK && resolver.Bind(selected, caller)
		})
	})
	seedRecurrenceDeclareAt(t, solver, trigger, "static-root-g-selector", shard, rootGCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return engine.Activate(access, startG, rootGCandidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(rootGCall)
			selected, selectedOK := resolver.Selected(gEntry)
			return callerOK && selectedOK && resolver.Bind(selected, caller)
		})
	})
	seedRecurrenceDeclareAt(t, solver, trigger, "static-f-selector", shard, fCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return engine.Activate(access, fRecurrence, fCandidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(fCall)
			selected, selectedOK := resolver.Selected(fEntry)
			gCaller, gCallerOK := resolver.Body(gCandidate, gCall)
			return callerOK && selectedOK && gCallerOK && resolver.Bind(selected, caller, gCaller)
		})
	})
	seedRecurrenceDeclareAt(t, solver, trigger, "static-g-selector", shard, gCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return engine.Activate(access, gRecurrence, gCandidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(gCall)
			selected, selectedOK := resolver.Selected(gEntry)
			fCaller, fCallerOK := resolver.Body(fCandidate, fCall)
			return callerOK && selectedOK && fCallerOK && resolver.Bind(selected, caller, fCaller)
		})
	})
	var fTag engine.ReadRef[uint64, seedRecurrenceBits]
	fTagSelector := seedRecurrenceDeclareAt(t, solver, tagTrigger, "static-f-tag-selector", shard, fCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		_, present, valid := engine.ReadAt(access, fTag, 0)
		if !valid || !present {
			return valid
		}
		return engine.Activate(access, leakRelation, fCandidate, func(resolver engine.Relation) bool {
			output, outputOK := resolver.Root(shard, root)
			gFalseExit, exitOK := resolver.Body(gCandidate, gExit)
			return outputOK && exitOK && resolver.Bind(output, gFalseExit)
		})
	})
	fTag, ok = engine.Read(fTagSelector, 0, tag)
	if !ok {
		t.Fatal("Read(static f recurrence tag)")
	}
	query, ok := engine.DeclareQuery(solver, leak, shard, root, 0)
	if !ok {
		t.Fatal("DeclareQuery(static discharge sentinel)")
	}

	state := seedRecurrenceSolve(t, solver)
	if got := seedRecurrenceRead(t, query, state); got != 0 {
		t.Fatalf("f recurrence discharged a decision from g's head: leak=%d, want 0", got)
	}
}

// A selected recursive activation is first discovered by Activate, then
// starts a fresh evaluation of exactly its own Program decision interface.
// The first visit takes f's true branch and re-enters the selected body;
// after that exact boundary, f's false branch reaches the normal exit while
// preserving the recurrence-produced tag.
func TestRelationSelectedRecursiveActivationDischargesToExit(t *testing.T) {
	contract := seedRecurrenceContract(t, "op")
	p, project, shard := seedRecurrenceProgramAndLink(t, `
local function f()
  if unknown then return f() end
end
f()
`, contract)
	root := seedRecurrenceEntry(t, p)
	fFunction, fBody, fEntry := seedRecurrenceFunction(t, p, 0)
	fCall := seedRecurrenceCallInActivation(t, p, fBody, 0)
	rootCall := seedRecurrenceCallInActivation(t, p, root, 0)
	fApplication := seedRecurrenceCallApplication(t, project, shard, fCall)
	rootApplication := seedRecurrenceCallApplication(t, project, shard, rootCall)
	fCandidate := seedRecurrenceBodyCandidate(t, project, fApplication, shard, fFunction)
	rootCandidate := seedRecurrenceBodyCandidate(t, project, rootApplication, shard, fFunction)
	fExit, ok := p.BodyNormalExit(fBody)
	if !ok {
		t.Fatal("f has no normal exit for its false branch")
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatalf("new Solver: %v", err)
	}
	trigger := seedRecurrenceFactor(t, solver, "selected-recursion-trigger")
	selector := seedRecurrenceFactor(t, solver, "selected-recursion-selector")
	tag := seedRecurrenceFactor(t, solver, "selected-recursion-tag")
	exitTag := seedRecurrenceFactor(t, solver, "selected-recursion-exit")
	start, ok := engine.DeclareRule(solver, trigger, seedRecurrenceSemantic("rule/selected-recursion-start"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(rootApplication, 1)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return access.Set(0, seedRecurrenceOne)
	})
	if !ok {
		t.Fatal("DeclareRule(selected recursion start)")
	}
	recurrence, ok := engine.DeclareRule(solver, tag, seedRecurrenceSemantic("rule/selected-recursion-reentry"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fApplication, 1)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return access.Set(0, seedRecurrenceOne)
	})
	if !ok {
		t.Fatal("DeclareRule(selected recursion reentry)")
	}
	seedRecurrenceDeclareAt(t, solver, trigger, "selected-recursion-root-selector", shard, rootCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		return engine.Activate(access, start, rootCandidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(rootCall)
			selected, selectedOK := resolver.Body(fCandidate, fEntry)
			return callerOK && selectedOK && resolver.Bind(selected, caller)
		})
	})
	var triggerAtCall engine.ReadRef[uint64, seedRecurrenceBits]
	cycleSelector := seedRecurrenceDeclareAt(t, solver, selector, "selected-recursion-cycle-selector", shard, fCall, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		value, _, valid := engine.ReadAt(access, triggerAtCall, 0)
		if !valid {
			return false
		}
		// The root invocation starts the selected body but does not carry this
		// marker. Only the selected invocation may close the recurrence.
		if value != seedRecurrenceOne {
			return true
		}
		return engine.Activate(access, recurrence, fCandidate, func(resolver engine.Relation) bool {
			caller, callerOK := resolver.Caller(fCall)
			selected, selectedOK := resolver.Selected(fEntry)
			return callerOK && selectedOK && resolver.Bind(selected, caller)
		})
	})
	triggerAtCall, ok = engine.Read(cycleSelector, 0, trigger)
	if !ok {
		t.Fatal("Read(selected recursion trigger)")
	}
	var input engine.ReadRef[uint64, seedRecurrenceBits]
	exit, ok := engine.DeclareRule(solver, exitTag, seedRecurrenceSemantic("rule/selected-recursion-exit"), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, fExit)
	}, func(access engine.Access[uint64, seedRecurrenceBits]) bool {
		value, _, valid := engine.ReadAt(access, input, 0)
		if !valid {
			return false
		}
		if value != seedRecurrenceOne {
			return access.Prune()
		}
		return access.Set(0, seedRecurrenceOne)
	})
	if !ok {
		t.Fatal("DeclareRule(selected recursion exit)")
	}
	input, ok = engine.Read(exit, 0, tag)
	if !ok {
		t.Fatal("Read(selected recursion tag at exit)")
	}
	// Candidate queries observe a selected body but never seed an entry. This
	// sole root demand makes the enclosing Program execution live.
	rootQuery, ok := engine.DeclareQuery(solver, trigger, shard, root, 0)
	if !ok {
		t.Fatal("DeclareQuery(selected recursion root demand)")
	}
	tagQuery, ok := engine.DeclareCandidateQuery(solver, tag, fCandidate, shard, fEntry, 0)
	if !ok {
		t.Fatal("DeclareCandidateQuery(selected recursion tag)")
	}
	exitQuery, ok := engine.DeclareCandidateQuery(solver, exitTag, fCandidate, shard, fExit, 0)
	if !ok {
		t.Fatal("DeclareCandidateQuery(selected recursion exit)")
	}

	state := seedRecurrenceSolve(t, solver)
	if _, present := rootQuery.Read(state); !present {
		t.Fatal("selected recursion root demand did not publish")
	}
	if got := seedRecurrenceRead(t, tagQuery, state); got != seedRecurrenceOne {
		t.Fatalf("selected recursive reentry tag=%d, want %d", got, seedRecurrenceOne)
	}
	if got := seedRecurrenceRead(t, exitQuery, state); got != seedRecurrenceOne {
		t.Fatalf("selected recursive reentry did not reach false exit with tag: %d, want %d", got, seedRecurrenceOne)
	}
}
