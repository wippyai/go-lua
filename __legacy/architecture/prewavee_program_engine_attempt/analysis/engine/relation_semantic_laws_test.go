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

// directCallFixture is deliberately a Program/Link fixture, rather than an
// engine topology fixture. Every term, Application, and Candidate comes from
// the sealed public authorities that production uses.
type directCallFixture struct {
	program   *program.Program
	link      *link.Link
	shard     link.Shard
	call      program.Term
	app       link.Application
	candidate link.Candidate
	body      program.Term
	entry     program.Term
}

func directCallFixtureFor(t testing.TB) directCallFixture {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{
		Name: "relation.lua",
		Text: []byte(`local function id(value) return value end; id(1)`),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	shard := fixtureShard(t, project, p)
	call, ok := p.CallAt(0)
	if !ok {
		t.Fatal("missing direct call")
	}
	function, ok := p.FunctionAt(0)
	if !ok {
		t.Fatal("missing direct function")
	}
	_, body, _, ok := p.Function(function)
	if !ok {
		t.Fatal("missing function body")
	}
	entry, ok := p.BodyEntry(body)
	if !ok {
		t.Fatal("missing function body entry")
	}
	app := fixtureCallApplication(t, project, p, call)
	candidate, ok := project.CandidateForFunction(app, shard, function)
	if !ok {
		t.Fatal("missing direct body candidate")
	}
	return directCallFixture{
		program: p, link: project, shard: shard, call: call, app: app,
		candidate: candidate, body: body, entry: entry,
	}
}

func directCallFixtureWithSecondRoot(t testing.TB) (directCallFixture, *program.Program, link.Shard, program.Term) {
	t.Helper()
	main, err := programlower.Lower(programlower.Source{
		Name: "main.lua",
		Text: []byte(`local function id(value) return value end; id(1)`),
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := programlower.Lower(programlower.Source{Name: "other.lua", Text: []byte(`local marker = 0`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: main}, {Name: "other", Program: other}}})
	if err != nil {
		t.Fatal(err)
	}
	shard := fixtureShard(t, project, main)
	otherShard := fixtureShard(t, project, other)
	call, ok := main.CallAt(0)
	if !ok {
		t.Fatal("missing direct call")
	}
	function, ok := main.FunctionAt(0)
	if !ok {
		t.Fatal("missing direct function")
	}
	_, body, _, ok := main.Function(function)
	if !ok {
		t.Fatal("missing function body")
	}
	entry, ok := main.BodyEntry(body)
	if !ok {
		t.Fatal("missing function body entry")
	}
	app := fixtureCallApplication(t, project, main, call)
	candidate, ok := project.CandidateForFunction(app, shard, function)
	if !ok {
		t.Fatal("missing direct body candidate")
	}
	otherEntry, ok := other.Entry()
	if !ok {
		t.Fatal("missing second root entry")
	}
	return directCallFixture{program: main, link: project, shard: shard, call: call, app: app, candidate: candidate, body: body, entry: entry}, other, otherShard, otherEntry
}

func fixtureShard(t testing.TB, project *link.Link, wanted *program.Program) link.Shard {
	t.Helper()
	for index := 0; index < project.ShardCount(); index++ {
		shard, ok := project.ShardAt(index)
		if !ok {
			continue
		}
		current, ok := project.Program(shard)
		if ok && current == wanted {
			return shard
		}
	}
	t.Fatal("missing fixture shard")
	return 0
}

func fixtureCallApplication(t testing.TB, project *link.Link, owner *program.Program, call program.Term) link.Application {
	t.Helper()
	for index := 0; index < project.ApplicationCount(); index++ {
		application, ok := project.ApplicationAt(index)
		if !ok {
			continue
		}
		shard, term, ok := project.CallApplication(application)
		programValue, ownerOK := project.Program(shard)
		if ok && ownerOK && programValue == owner && term == call {
			return application
		}
	}
	t.Fatal("missing fixture call application")
	return link.Application{}
}

func fixtureUnconditionalActivationEdge(t testing.TB, value *program.Program, activation program.Term) program.Edge {
	t.Helper()
	count, ok := value.ActivationEdgeCount(activation)
	if !ok {
		t.Fatalf("ActivationEdgeCount(%v)", activation)
	}
	for index := 0; index < count; index++ {
		edge, ok := value.ActivationEdgeAt(activation, index)
		if !ok {
			t.Fatalf("ActivationEdgeAt(%v, %d)", activation, index)
		}
		if _, _, conditional := edge.Decision(); !conditional {
			return edge
		}
	}
	t.Fatal("fixture activation has no unconditional Edge")
	return program.Edge{}
}

func relationSemantic(label string) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID(sha256.Sum256([]byte(label))), Version: 1}
}

func relationBits() lattice.Lattice[uint8] {
	return lattice.Lattice[uint8]{
		Bottom:   func() uint8 { return 0 },
		Top:      func() uint8 { return ^uint8(0) },
		Equal:    func(left, right uint8) bool { return left == right },
		LessOrEq: func(left, right uint8) bool { return left&^right == 0 },
		Join:     func(left, right uint8) uint8 { return left | right },
		Meet:     func(left, right uint8) uint8 { return left & right },
		Widen:    func(left, right uint8) uint8 { return left | right },
	}
}

func relationFactor(t testing.TB, solver *engine.Solver, label string) *engine.Factor[uint64, uint8] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, uint8]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    relationSemantic("factor/" + label),
		Lattice:     relationBits(),
		Default:     0,
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		WidenRank: engine.Measure[uint64, uint8]{
			Width: 1,
			At: func(_ uint64, value uint8, _ int) uint64 {
				return uint64(8 - bits.OnesCount8(value))
			},
		},
	})
	if !ok {
		t.Fatalf("engine.DeclareFactor(%s)", label)
	}
	return factor
}

func declareAt[K ~uint64, V any](t testing.TB, solver *engine.Solver, output *engine.Factor[K, V], semantic string, shard link.Shard, term program.Term, run func(engine.Access[K, V]) bool) *engine.Rule[K, V] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, output, relationSemantic(semantic), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, run)
	if !ok {
		t.Fatalf("engine.DeclareRule(%s)", semantic)
	}
	return rule
}

// TestRelationCarryPreservesOnlyItsDeclaredFactor establishes the core
// no-ambient-transport law: a relation contribution starts at the canonical
// zero product and can cross only the explicitly carried output engine.Factor.
func TestRelationCarryPreservesOnlyItsDeclaredFactor(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	carried := relationFactor(t, solver, "carried")
	sibling := relationFactor(t, solver, "sibling")
	trigger := relationFactor(t, solver, "trigger")

	var selected *engine.Rule[uint64, uint8]
	var carriedRead engine.ReadRef[uint64, uint8]
	selected, ok := engine.DeclareRule(solver, carried, relationSemantic("rule/selected-carry"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 1)
	}, func(access engine.Access[uint64, uint8]) bool {
		candidate, application, visible := access.Selection()
		if !visible || !sameFixtureCandidate(fixture.link, candidate, fixture.candidate) || !sameFixtureApplication(fixture.link, application, fixture.app) {
			return false
		}
		return engine.Carry(access, carriedRead)
	})
	if !ok {
		t.Fatal("Declare relation engine.Rule")
	}
	carriedRead, ok = engine.Read(selected, 0, carried)
	if !ok {
		t.Fatal("declare carried engine.ReadRef")
	}

	declareAt(t, solver, carried, "rule/source-carried", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x01)
	})
	declareAt(t, solver, sibling, "rule/source-sibling", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x02)
	})
	declareAt(t, solver, trigger, "rule/select", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return engine.Activate(access, selected, fixture.candidate, func(relation engine.Relation) bool {
			caller, callerOK := relation.Caller(fixture.call)
			body, bodyOK := relation.Selected(fixture.entry)
			return callerOK && bodyOK && relation.Bind(body, caller)
		})
	})

	root, ok := engine.DeclareQuery(solver, trigger, fixture.shard, fixture.call, 0)
	if !ok || root == nil {
		t.Fatal("declare root query")
	}
	carriedQuery, ok := engine.DeclareCandidateQuery(solver, carried, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("declare carried candidate query")
	}
	siblingQuery, ok := engine.DeclareCandidateQuery(solver, sibling, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("declare sibling candidate query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve")
	}
	if value, present := root.Read(state); !present || value != 0 {
		t.Fatalf("root trigger=%d/%v", value, present)
	}
	if value, present := carriedQuery.Read(state); !present || value != 0x01 {
		t.Fatalf("carried relation result=%d/%v", value, present)
	}
	if value, present := siblingQuery.Read(state); !present || value != 0 {
		t.Fatalf("undeclared sibling crossed relation=%d/%v", value, present)
	}
}

// TestRelationCarryIsSingleUse makes transfer linear at the Product terminal.
// The result is observed through the selected candidate occurrence, not through
// callback bookkeeping.
func TestRelationCarryIsSingleUse(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	value := relationFactor(t, solver, "linear-carry")
	trigger := relationFactor(t, solver, "linear-carry-trigger")

	var selected *engine.Rule[uint64, uint8]
	var read engine.ReadRef[uint64, uint8]
	selected, ok := engine.DeclareRule(solver, value, relationSemantic("rule/linear-carry"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 1)
	}, func(access engine.Access[uint64, uint8]) bool {
		return engine.Carry(access, read) && !engine.Carry(access, read)
	})
	if !ok {
		t.Fatal("declare linear engine.Carry engine.Rule")
	}
	read, ok = engine.Read(selected, 0, value)
	if !ok {
		t.Fatal("declare linear engine.Carry engine.ReadRef")
	}
	declareAt(t, solver, value, "rule/linear-carry-source", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x01)
	})
	declareAt(t, solver, trigger, "rule/linear-carry-select", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return engine.Activate(access, selected, fixture.candidate, func(relation engine.Relation) bool {
			caller, callerOK := relation.Caller(fixture.call)
			body, bodyOK := relation.Selected(fixture.entry)
			return callerOK && bodyOK && relation.Bind(body, caller)
		})
	})
	if _, ok := engine.DeclareQuery(solver, trigger, fixture.shard, fixture.call, 0); !ok {
		t.Fatal("declare selector query")
	}
	query, ok := engine.DeclareCandidateQuery(solver, value, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("declare carried candidate query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve")
	}
	if got, present := query.Read(state); !present || got != 0x01 {
		t.Fatalf("single engine.Carry result=%d/%t, want 1/present", got, present)
	}
}

// TestRelationKeepContributesPresentZero distinguishes an ordinary zero from
// absence. A selected body entered through a engine.Relation whose engine.Rule writes no
// engine.Factor must be reachable with the canonical zero vector; it is not an
// implicit copy of the caller and it is not pruned.
func TestRelationKeepContributesPresentZero(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	output := relationFactor(t, solver, "present-zero-output")
	trigger := relationFactor(t, solver, "present-zero-trigger")
	target, ok := engine.DeclareRule(solver, output, relationSemantic("rule/present-zero"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 1)
	}, func(engine.Access[uint64, uint8]) bool {
		return true
	})
	if !ok {
		t.Fatal("declare present-zero engine.Relation engine.Rule")
	}
	declareAt(t, solver, trigger, "rule/present-zero-select", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return engine.Activate(access, target, fixture.candidate, func(relation engine.Relation) bool {
			caller, callerOK := relation.Caller(fixture.call)
			body, bodyOK := relation.Selected(fixture.entry)
			return callerOK && bodyOK && relation.Bind(body, caller)
		})
	})
	if _, ok := engine.DeclareQuery(solver, trigger, fixture.shard, fixture.call, 0); !ok {
		t.Fatal("declare present-zero selector query")
	}
	query, ok := engine.DeclareCandidateQuery(solver, output, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("declare present-zero output query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve")
	}
	if got, present := query.Read(state); !present || got != 0 {
		t.Fatalf("Keep relation result=%d/%t, want 0/present", got, present)
	}
}

// TestRelationPruneContributesNothing is the complementary law: Prune is not
// a zero write. It removes the one compatible Product terminal, leaving the
// selected candidate occurrence absent even though its selector executed.
func TestRelationPruneContributesNothing(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	output := relationFactor(t, solver, "prune-output")
	trigger := relationFactor(t, solver, "prune-trigger")
	target, ok := engine.DeclareRule(solver, output, relationSemantic("rule/prune"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 1)
	}, func(access engine.Access[uint64, uint8]) bool {
		return access.Prune()
	})
	if !ok {
		t.Fatal("declare Prune engine.Relation engine.Rule")
	}
	declareAt(t, solver, trigger, "rule/prune-select", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return engine.Activate(access, target, fixture.candidate, func(relation engine.Relation) bool {
			caller, callerOK := relation.Caller(fixture.call)
			body, bodyOK := relation.Selected(fixture.entry)
			return callerOK && bodyOK && relation.Bind(body, caller)
		})
	})
	if _, ok := engine.DeclareQuery(solver, trigger, fixture.shard, fixture.call, 0); !ok {
		t.Fatal("declare Prune selector query")
	}
	query, ok := engine.DeclareCandidateQuery(solver, output, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("declare Prune output query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve")
	}
	if got, present := query.Read(state); present || got != 0 {
		t.Fatalf("Prune relation result=%d/%t, want 0/absent", got, present)
	}
}

// TestLocalEdgeRulesCarryIndependently keeps two declared local transfers
// independent at their common Program edge.
func TestLocalEdgeRulesCarryIndependentlyWithinOneProductTerminal(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := fixture.program.Entry()
	if !ok {
		t.Fatal("missing root entry")
	}
	edge := fixtureUnconditionalActivationEdge(t, fixture.program, entry)
	left := relationFactor(t, solver, "local-independent-carry-left")
	right := relationFactor(t, solver, "local-independent-carry-right")
	declareAt(t, solver, left, "rule/local-carry-left-source", fixture.shard, edge.From(), func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x01)
	})
	declareAt(t, solver, right, "rule/local-carry-right-source", fixture.shard, edge.From(), func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x02)
	})

	var firstRead, secondRead engine.ReadRef[uint64, uint8]
	firstRule, ok := engine.DeclareRule(solver, left, relationSemantic("rule/local-carry-first"), func(binding *engine.RuleBinding) bool {
		return binding.From(fixture.shard, edge)
	}, func(access engine.Access[uint64, uint8]) bool {
		return engine.Carry(access, firstRead)
	})
	if !ok {
		t.Fatal("declare first local edge engine.Carry engine.Rule")
	}
	secondRule, ok := engine.DeclareRule(solver, right, relationSemantic("rule/local-carry-second"), func(binding *engine.RuleBinding) bool {
		return binding.From(fixture.shard, edge)
	}, func(access engine.Access[uint64, uint8]) bool {
		return engine.Carry(access, secondRead)
	})
	if !ok {
		t.Fatal("declare second local edge engine.Carry engine.Rule")
	}
	firstRead, ok = engine.Read(firstRule, 0, left)
	if !ok {
		t.Fatal("declare first local engine.Carry engine.ReadRef")
	}
	secondRead, ok = engine.Read(secondRule, 0, right)
	if !ok {
		t.Fatal("declare second local engine.Carry engine.ReadRef")
	}
	leftQuery, ok := engine.DeclareQuery(solver, left, fixture.shard, edge.To(), 0)
	if !ok {
		t.Fatal("declare left local engine.Carry query")
	}
	rightQuery, ok := engine.DeclareQuery(solver, right, fixture.shard, edge.To(), 0)
	if !ok {
		t.Fatal("declare right local engine.Carry query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve")
	}
	if got, present := leftQuery.Read(state); !present || got != 0x01 {
		t.Fatalf("left local engine.Carry result=%d/%t, want 1/present", got, present)
	}
	if got, present := rightQuery.Read(state); !present || got != 0x02 {
		t.Fatalf("right local engine.Carry result=%d/%t, want 2/present", got, present)
	}
}

// TestRelationInputsRemainOrderedAndReadCapabilitiesStayOwned proves two
// independent invariants at once: an n-ary relation keeps its declared tuple
// order, and a engine.ReadRef from a different engine.Rule cannot become an ambient lookup.
func TestRelationInputsRemainOrderedAndReadCapabilitiesStayOwned(t *testing.T) {
	fixture, _, otherShard, otherEntry := directCallFixtureWithSecondRoot(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	input := relationFactor(t, solver, "ordered-input")
	output := relationFactor(t, solver, "ordered-output")
	trigger := relationFactor(t, solver, "ordered-trigger")
	foreign := relationFactor(t, solver, "foreign-read")

	var left, right, foreignRead engine.ReadRef[uint64, uint8]
	selected, ok := engine.DeclareRule(solver, output, relationSemantic("rule/ordered-relation"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 2)
	}, func(access engine.Access[uint64, uint8]) bool {
		first, firstPresent, firstOK := engine.ReadAt(access, left, 0)
		second, secondPresent, secondOK := engine.ReadAt(access, right, 0)
		if !firstOK || !secondOK || !firstPresent || !secondPresent {
			return false
		}
		if _, _, valid := engine.ReadAt(access, foreignRead, 0); valid {
			return false
		}
		return access.Set(0, first<<4|second)
	})
	if !ok {
		t.Fatal("Declare ordered relation")
	}
	left, ok = engine.Read(selected, 0, input)
	if !ok {
		t.Fatal("declare first engine.ReadRef")
	}
	right, ok = engine.Read(selected, 1, input)
	if !ok {
		t.Fatal("declare second engine.ReadRef")
	}
	foreignRule := declareAt(t, solver, foreign, "rule/foreign-read", fixture.shard, fixture.call, func(engine.Access[uint64, uint8]) bool { return true })
	foreignRead, ok = engine.Read(foreignRule, 0, input)
	if !ok {
		t.Fatal("declare foreign engine.ReadRef")
	}
	declareAt(t, solver, input, "rule/main-input", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 1)
	})
	declareAt(t, solver, input, "rule/other-input", otherShard, otherEntry, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 2)
	})
	declareAt(t, solver, trigger, "rule/ordered-select", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return engine.Activate(access, selected, fixture.candidate, func(relation engine.Relation) bool {
			caller, callerOK := relation.Caller(fixture.call)
			root, rootOK := relation.Root(otherShard, otherEntry)
			body, bodyOK := relation.Selected(fixture.entry)
			return callerOK && rootOK && bodyOK && relation.Bind(body, caller, root)
		})
	})
	if _, ok := engine.DeclareQuery(solver, trigger, fixture.shard, fixture.call, 0); !ok {
		t.Fatal("declare main root query")
	}
	if _, ok := engine.DeclareQuery(solver, input, otherShard, otherEntry, 0); !ok {
		t.Fatal("declare second root query")
	}
	query, ok := engine.DeclareCandidateQuery(solver, output, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("declare ordered candidate query")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok {
		t.Fatal("Solve")
	}
	if value, present := query.Read(state); !present || value != 0x12 {
		t.Fatalf("ordered relation value=%#x/%v", value, present)
	}
}

// TestSolvePublishedStateIsReusableOnlyByItsOwner exercises State as the
// public immutable-generation capability: a completed State can be supplied
// back to its owning Solver without changing a Query's result, while an
// independently constructed Solver cannot accept or read it.
func TestSolvePublishedStateIsReusableOnlyByItsOwner(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	value := relationFactor(t, solver, "published-state-value")
	declareAt(t, solver, value, "rule/published-state-value", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x01)
	})
	query, ok := engine.DeclareQuery(solver, value, fixture.shard, fixture.call, 0)
	if !ok {
		t.Fatal("declare owning Query")
	}
	if !solver.Seal() {
		t.Fatal("seal owning Solver")
	}
	published, ok := solver.Solve(context.Background(), nil)
	if !ok || published == nil {
		t.Fatal("publish owning State")
	}
	if value, present := query.Read(published); !present || value != 0x01 {
		t.Fatalf("published Query = %#x/%t, want 0x01/true", value, present)
	}
	reused, ok := solver.Solve(context.Background(), published)
	if !ok || reused == nil {
		t.Fatal("reuse published State")
	}
	if value, present := query.Read(reused); !present || value != 0x01 {
		t.Fatalf("reused Query = %#x/%t, want 0x01/true", value, present)
	}

	foreignFixture := directCallFixtureFor(t)
	foreign, err := engine.New(foreignFixture.link)
	if err != nil {
		t.Fatal(err)
	}
	foreignValue := relationFactor(t, foreign, "foreign-state-value")
	declareAt(t, foreign, foreignValue, "rule/foreign-state-value", foreignFixture.shard, foreignFixture.call, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 0x02)
	})
	foreignQuery, ok := engine.DeclareQuery(foreign, foreignValue, foreignFixture.shard, foreignFixture.call, 0)
	if !ok {
		t.Fatal("declare foreign Query")
	}
	if !foreign.Seal() {
		t.Fatal("seal foreign Solver")
	}
	foreignState, ok := foreign.Solve(context.Background(), nil)
	if !ok || foreignState == nil {
		t.Fatal("publish foreign State")
	}
	if value, present := foreignQuery.Read(foreignState); !present || value != 0x02 {
		t.Fatalf("foreign Query = %#x/%t, want 0x02/true", value, present)
	}
	if state, accepted := solver.Solve(context.Background(), foreignState); accepted || state != nil {
		t.Fatal("owning Solver accepted a State from an independent Solver")
	}
	if _, present := query.Read(foreignState); present {
		t.Fatal("owning Query read a State from an independent Solver")
	}
}

func sameFixtureCandidate(project *link.Link, left, right link.Candidate) bool {
	order, ok := project.CompareCandidate(left, right)
	return ok && order == 0
}

func sameFixtureApplication(project *link.Link, left, right link.Application) bool {
	order, ok := project.CompareApplication(left, right)
	return ok && order == 0
}
