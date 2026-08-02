package reachability_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/control/reachability"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/artifact"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type controlWitness uint8

const (
	witnessAssign controlWitness = iota
	witnessBind
	witnessCall
	witnessBody
	witnessLoop
	witnessBranch
	witnessFunction
	witnessReturn
	witnessBreak
	witnessGoto
	witnessLabel
)

type controlScenario struct {
	name    string
	source  string
	line    int
	witness controlWitness
}

// sourceControlCorpus is a legal, source-level control corpus. Each case
// identifies one top-level authored root, then verifies the public typed
// Program successors that follow from that exact root.
var sourceControlCorpus = []controlScenario{
	{name: "assign", source: "value = 1", line: 1, witness: witnessAssign},
	{name: "local-assign", source: "local value = 1", line: 1, witness: witnessBind},
	{name: "call-statement", source: "invoke()", line: 1, witness: witnessCall},
	{name: "do-block", source: "do local value = 1 end", line: 1, witness: witnessBody},
	{name: "while", source: "while true do local value = 1 end", line: 1, witness: witnessLoop},
	{name: "repeat", source: "repeat local value = 1 until true", line: 1, witness: witnessLoop},
	{name: "if-else", source: "if true then local yes = 1 else local no = 2 end", line: 1, witness: witnessBranch},
	{name: "if-no-else", source: "if true then local yes = 1 end", line: 1, witness: witnessBranch},
	{name: "number-for", source: "for index = 1, 2, 1 do local value = index end", line: 1, witness: witnessLoop},
	{name: "number-for-default-step", source: "for index = 1, 2 do local value = index end", line: 1, witness: witnessLoop},
	{name: "generic-for", source: "for key, value in iterator do local seen = key end", line: 1, witness: witnessLoop},
	{name: "function-definition", source: "function defined() end", line: 1, witness: witnessFunction},
	{name: "return", source: "return 1", line: 1, witness: witnessReturn},
	{name: "return-empty", source: "return", line: 1, witness: witnessReturn},
	{name: "return-list", source: "return 1, 2", line: 1, witness: witnessReturn},
	{name: "break-legal", source: "while true do break end", line: 1, witness: witnessBreak},
	{name: "goto-trailing-label", source: "goto trailing\nlocal value = 1\n::trailing::", line: 1, witness: witnessGoto},
	{name: "goto-forward", source: "goto done\n::done::", line: 1, witness: witnessGoto},
	{name: "goto-backward", source: "::again::\ngoto again", line: 2, witness: witnessGoto},
	{name: "label-normal", source: "::ready::\nlocal value = 1", line: 1, witness: witnessLabel},
}

func TestSourceControlCorpusReachability(t *testing.T) {
	const denominator = 20
	if len(sourceControlCorpus) != denominator {
		t.Fatalf("source-control reachability corpus = %d cases, want %d", len(sourceControlCorpus), denominator)
	}
	for _, scenario := range sourceControlCorpus {
		scenario := scenario
		if !t.Run(scenario.name, func(t *testing.T) {
			assertScenarioReachability(t, scenario)
		}) {
			return
		}
	}
	t.Logf("source-control reachability corpus: %d/%d scenarios", denominator, denominator)
}

func assertScenarioReachability(t *testing.T, scenario controlScenario) {
	t.Helper()
	project, shard, source := reachabilityProject(t, scenario)
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := reachability.Install(solver, project)
	if !ok {
		t.Fatal("install reachability domain")
	}
	queries := reachabilityQueries(t, domain, shard, source, scenario)
	if !solver.Seal() {
		t.Fatal("seal reachability scenario")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve reachability scenario")
	}
	for _, observation := range queries {
		value, present := observation.query.Read(state)
		if !present || value != reachability.Reachable {
			t.Fatalf("canonical term %v reachability = %v/%t, want reachable", observation.term, value, present)
		}
	}
}

func reachabilityProject(t *testing.T, scenario controlScenario) (*link.Link, link.Shard, *program.Program) {
	t.Helper()
	source, err := programlower.Lower(programlower.Source{
		Name: scenario.name + ".lua",
		Text: []byte(scenario.source),
	})
	if err != nil {
		t.Fatalf("lower %s: %v", scenario.name, err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []link.Module{{Name: scenario.name, Program: source}},
	})
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := project.ShardAt(0)
	if !ok {
		t.Fatal("project has no source shard")
	}
	return project, shard, source
}

type reachabilityObservation struct {
	term  program.Term
	query *engine.Query[uint64, reachability.Value]
}

func reachabilityQueries(t *testing.T, domain *reachability.Domain, shard link.Shard, source *program.Program, scenario controlScenario) []reachabilityObservation {
	t.Helper()
	entry, ok := source.Entry()
	if !ok {
		t.Fatal("source has no entry")
	}
	root := sourceRootOnLine(t, source, entry, scenario.line)
	terms := reachabilityWitnesses(t, source, root, scenario.witness)
	assertEntryReachableTerms(t, source, terms)
	queries := make([]reachabilityObservation, 0, len(terms))
	for _, term := range terms {
		query, ok := domain.Query(shard, term)
		if !ok {
			t.Fatalf("declare reachability Query for canonical term %v", term)
		}
		queries = append(queries, reachabilityObservation{term: term, query: query})
	}
	return queries
}

// assertEntryReachableTerms keeps the oracle on canonical causal coordinates.
// It is a test-only guard over the public Program edge rows; it does not
// expose topology from the reachability package.
func assertEntryReachableTerms(t *testing.T, source *program.Program, terms []program.Term) {
	t.Helper()
	entry, ok := source.Entry()
	if !ok {
		t.Fatal("source has no entry")
	}
	count, ok := source.ActivationEdgeCount(entry)
	if !ok {
		t.Fatalf("ActivationEdgeCount(%v) absent", entry)
	}
	reachable := map[program.Term]bool{entry: true}
	for changed := true; changed; {
		changed = false
		for index := 0; index < count; index++ {
			edge, ok := source.ActivationEdgeAt(entry, index)
			if !ok {
				t.Fatalf("ActivationEdgeAt(%v, %d) absent", entry, index)
			}
			if reachable[edge.From()] && !reachable[edge.To()] {
				reachable[edge.To()] = true
				changed = true
			}
		}
	}
	for _, term := range terms {
		activation, ok := source.Activation(term)
		if !ok || activation != entry || !reachable[term] {
			t.Fatalf("query term %v is not on an Entry-reachable activation path", term)
		}
	}
}

func TestEquationCacheStableUnderUnrelatedModuleOrder(t *testing.T) {
	main := lowerReachabilityProgram(t, "main.lua", "local value = 1")
	unrelated := lowerReachabilityProgram(t, "unrelated.lua", "local other = 2")

	cacheFor := func(modules []link.Module) artifact.EquationCache {
		contract, err := target.Seal(&target.Spec{})
		if err != nil {
			t.Fatal(err)
		}
		project, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
		if err != nil {
			t.Fatal(err)
		}
		var shard link.Shard
		for index := 0; index < project.ShardCount(); index++ {
			candidate, ok := project.ShardAt(index)
			if programValue, found := project.Program(candidate); ok && found && programValue == main {
				shard = candidate
				break
			}
		}
		if shard == 0 {
			t.Fatal("main shard")
		}
		solver, err := engine.New(project)
		if err != nil {
			t.Fatal(err)
		}
		domain, ok := reachability.Install(solver, project)
		if !ok {
			t.Fatal("install reachability domain")
		}
		entry, ok := main.Entry()
		if !ok {
			t.Fatal("main entry")
		}
		root := sourceRootOnLine(t, main, entry, 1)
		term, ok := main.BindValuesEntry(root)
		if !ok {
			t.Fatal("main BindValuesEntry")
		}
		assertEntryReachableTerms(t, main, []program.Term{term})
		if _, ok := domain.Query(shard, term); !ok || !solver.Seal() {
			t.Fatal("seal main reachability query")
		}
		cache, ok := solver.EquationCache(shard)
		if !ok {
			t.Fatal("reachability equation cache")
		}
		return cache
	}

	base := cacheFor([]link.Module{{Name: "main", Program: main}})
	added := cacheFor([]link.Module{
		{Name: "main", Program: main},
		{Name: "unrelated", Program: unrelated},
	})
	reordered := cacheFor([]link.Module{
		{Name: "unrelated", Program: unrelated},
		{Name: "main", Program: main},
	})
	for _, candidate := range []artifact.EquationCache{added, reordered} {
		if !reflect.DeepEqual(base.Rules, candidate.Rules) || !reflect.DeepEqual(base.Boundary, candidate.Boundary) {
			t.Fatal("unrelated module order changed main reachability cache identities")
		}
	}
}

func lowerReachabilityProgram(t *testing.T, name, text string) *program.Program {
	t.Helper()
	source, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatalf("lower %s: %v", name, err)
	}
	return source
}

func sourceRootOnLine(t *testing.T, source *program.Program, body program.Term, line int) program.Term {
	t.Helper()
	length, ok := source.BodySourceLen(body)
	if !ok {
		t.Fatalf("BodySourceLen(%v) absent", body)
	}
	var matches []program.Term
	for index := 0; index < length; index++ {
		term, ok := source.BodySourceAt(body, index)
		if !ok {
			t.Fatalf("BodySourceAt(%v, %d) absent", body, index)
		}
		span, ok := source.Span(term)
		if ok && span.StartLine == line {
			matches = append(matches, term)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("top-level source root at line %d = %d, want one", line, len(matches))
	}
	return matches[0]
}

func reachabilityWitnesses(t *testing.T, source *program.Program, root program.Term, witness controlWitness) []program.Term {
	t.Helper()
	terms := make([]program.Term, 0, 8)
	seen := make(map[program.Term]struct{})
	add := func(name string, term program.Term, ok bool) {
		t.Helper()
		if !ok || term == 0 {
			t.Fatalf("%s(%v) absent", name, root)
		}
		if _, duplicate := seen[term]; !duplicate {
			seen[term] = struct{}{}
			terms = append(terms, term)
		}
	}
	switch witness {
	case witnessAssign:
		entry, ok := source.AssignFirstEntry(root)
		add("AssignFirstEntry", entry, ok)
	case witnessBind:
		entry, ok := source.BindValuesEntry(root)
		add("BindValuesEntry", entry, ok)
	case witnessCall:
		entry, ok := source.CallCalleeEntry(root)
		add("CallCalleeEntry", entry, ok)
	case witnessBody:
		entry, ok := source.BodyFirst(root)
		add("BodyFirst", entry, ok)
	case witnessLoop:
		entry, ok := source.LoopEntry(root)
		add("LoopEntry", entry, ok)
		body, ok := source.LoopContinueBody(root)
		if !ok {
			t.Fatalf("LoopContinueBody(%v) absent", root)
		}
		add("LoopContinueBody", body, true)
		exit, ok := source.LoopExit(root)
		add("LoopExit", exit, ok)
		if condition, present := source.LoopRepeatConditionEntry(root); present {
			add("LoopRepeatConditionEntry", condition, true)
		}
	case witnessBranch:
		condition, ok := source.BranchConditionEntry(root)
		add("BranchConditionEntry", condition, ok)
		whenTrue, ok := source.BranchTruthyBody(root)
		add("BranchTruthyBody", whenTrue, ok)
		whenFalse, ok := source.BranchFalsyBody(root)
		add("BranchFalsyBody", whenFalse, ok)
		trueExit, ok := source.BodyNormalExit(whenTrue)
		add("true BodyNormalExit", trueExit, ok)
		falseExit, ok := source.BodyNormalExit(whenFalse)
		add("false BodyNormalExit", falseExit, ok)
	case witnessFunction:
		entry, ok := source.AssignFirstEntry(root)
		add("function AssignFirstEntry", entry, ok)
	case witnessReturn:
		entry, ok := source.ReturnValuesEntry(root)
		add("ReturnValuesEntry", entry, ok)
		exit, ok := source.ReturnExit(root)
		add("ReturnExit", exit, ok)
	case witnessBreak:
		broken, ok := source.BreakAt(0)
		add("BreakAt", broken, ok)
		_, loop, ok := source.Break(broken)
		if !ok || loop != root {
			t.Fatalf("Break(%v) loop = %v/%t, want source loop %v", broken, loop, ok, root)
		}
		exit, ok := source.BreakExit(broken)
		add("BreakExit", exit, ok)
		loopExit, ok := source.LoopExit(root)
		add("LoopExit", loopExit, ok)
	case witnessGoto:
		add("Goto", root, true)
		_, label, ok := source.Goto(root)
		if !ok || label == 0 {
			t.Fatalf("Goto(%v) has no resolved Label", root)
		}
		exit, ok := source.GotoExit(root)
		if !ok || exit == 0 {
			t.Fatalf("GotoExit(%v) absent", root)
		}
		resume, ok := source.LabelResume(label)
		add("LabelResume", resume, ok)
	case witnessLabel:
		resume, ok := source.LabelResume(root)
		add("LabelResume", resume, ok)
	default:
		t.Fatalf("unknown control witness %d", witness)
	}
	return terms
}
