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

// importLawBits keeps these laws finite while leaving each observed relation
// fact distinct. The values deliberately represent only test facts; Link and
// the public engine relation APIs remain the sole topology authorities.
type importLawBits uint8

const (
	importLawFirst importLawBits = 1 << iota
	importLawSecond
	importLawLoaded
	importLawSelected
	importLawBoth = importLawFirst | importLawSecond
)

func importLawLattice() lattice.Lattice[importLawBits] {
	return lattice.Lattice[importLawBits]{
		Bottom: func() importLawBits { return 0 },
		Top:    func() importLawBits { return ^importLawBits(0) },
		Equal: func(left, right importLawBits) bool {
			return left == right
		},
		LessOrEq: func(left, right importLawBits) bool {
			return left&^right == 0
		},
		Join: func(left, right importLawBits) importLawBits {
			return left | right
		},
		Meet: func(left, right importLawBits) importLawBits {
			return left & right
		},
		Widen: func(left, right importLawBits) importLawBits {
			return left | right
		},
	}
}

func importLawSemantic(label string) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID(sha256.Sum256([]byte("relation-import-law/" + label))), Version: 1}
}

func importLawFactor(t testing.TB, solver *engine.Solver, label string) *engine.Factor[uint64, importLawBits] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, importLawBits]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    importLawSemantic("factor/" + label),
		Lattice:     importLawLattice(),
		Default:     0,
		Fingerprint: func(value importLawBits) uint64 { return uint64(value) },
		WidenRank: engine.Measure[uint64, importLawBits]{
			Width: 1,
			At: func(_ uint64, value importLawBits, _ int) uint64 {
				return uint64(8 - bits.OnesCount8(uint8(value)))
			},
		},
	})
	if !ok {
		t.Fatalf("DeclareFactor(%s)", label)
	}
	return factor
}

func importLawDeclareAt(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, importLawBits], label string, shard link.Shard, term program.Term, run func(engine.Access[uint64, importLawBits]) bool) *engine.Rule[uint64, importLawBits] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, importLawSemantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, run)
	if !ok {
		t.Fatalf("DeclareRule(At, %s)", label)
	}
	return rule
}

func importLawDeclareRelation(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, importLawBits], label string, application link.Application, inputs int, run func(engine.Access[uint64, importLawBits]) bool) *engine.Rule[uint64, importLawBits] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, importLawSemantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.Relation(application, inputs)
	}, run)
	if !ok {
		t.Fatalf("DeclareRule(Relation, %s)", label)
	}
	return rule
}

func importLawOutcomeQuery(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, importLawBits], importer importLawImporter) *engine.Query[uint64, importLawBits] {
	t.Helper()
	query, ok := engine.DeclareQuery(solver, factor, importer.shard, importer.destination, 0)
	if !ok {
		t.Fatal("DeclareQuery rejected the typed ImportOutcome destination")
	}
	return query
}

func importLawSealAndSolve(t testing.TB, solver *engine.Solver) *engine.State {
	t.Helper()
	if !solver.Seal() {
		t.Fatal("Solver.Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solver.Solve")
	}
	return state
}

func importLawRead(t testing.TB, query *engine.Query[uint64, importLawBits], state *engine.State) importLawBits {
	t.Helper()
	value, present := query.Read(state)
	if !present {
		t.Fatal("Query.Read has no published outcome")
	}
	return value
}

// importLawImporter contains only public Program and typed Link query
// results. In particular, it does not decode Application kinds or retain an
// engine coordinate.
type importLawImporter struct {
	shard           link.Shard
	importTerm      program.Term
	call            program.Term
	importApp       link.Application
	callApp         link.Application
	candidate       link.Candidate
	loadedShard     link.Shard
	loadedEntry     program.Term
	loadedReturn    program.Term
	destination     program.Term
	importAliasCell program.Term
}

type importLawFixture struct {
	link      *link.Link
	importers []importLawImporter
}

func importLawFixtureFor(t testing.TB, names ...string) importLawFixture {
	t.Helper()
	if len(names) == 0 {
		t.Fatal("an import law fixture needs an importer")
	}
	dependency, err := programlower.Lower(programlower.Source{
		Name: "dep.lua",
		Text: []byte(`return 1`),
	})
	if err != nil {
		t.Fatalf("lower dependency: %v", err)
	}
	modules := make([]link.Module, 0, len(names)+1)
	programs := make([]*program.Program, len(names))
	for index, name := range names {
		value, err := programlower.Lower(programlower.Source{
			Name: name + ".lua",
			Text: []byte(`local dependency = require("dep"); return dependency`),
		})
		if err != nil {
			t.Fatalf("lower importer %q: %v", name, err)
		}
		programs[index] = value
		modules = append(modules, link.Module{Name: name, Program: value})
	}
	modules = append(modules, link.Module{Name: "dep", Program: dependency})
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatalf("seal target Contract: %v", err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
	if err != nil {
		t.Fatalf("seal Link: %v", err)
	}
	fixture := importLawFixture{link: project, importers: make([]importLawImporter, len(programs))}
	for index, importer := range programs {
		fixture.importers[index] = importLawImporterFor(t, project, importer, dependency)
	}
	return fixture
}

func importLawImporterFor(t testing.TB, project *link.Link, importer, dependency *program.Program) importLawImporter {
	t.Helper()
	for index := 0; index < project.ApplicationCount(); index++ {
		application, ok := project.ApplicationAt(index)
		if !ok {
			t.Fatalf("ApplicationAt(%d)", index)
		}
		loader, sourceShard, importTerm, result, importOK := project.ImportApplication(application)
		if !importOK {
			continue
		}
		source, sourceOK := project.Program(sourceShard)
		if !sourceOK || source != importer {
			continue
		}
		loadedShard, loadedEntry, demandOK := project.ImportDemand(application)
		loaded, loadedOK := project.Program(loadedShard)
		if !demandOK || !loadedOK || loaded != dependency {
			continue
		}
		resultShard, resultEntry, resultDemandOK := project.LoadDemand(result)
		if !resultDemandOK || resultShard != loadedShard || resultEntry != loadedEntry {
			t.Fatal("ImportApplication LoadResult did not retain ImportDemand")
		}
		resultSourceShard, call, alias, resultOK := project.ImportResult(application)
		if !resultOK || resultSourceShard != sourceShard || call == 0 || alias == 0 {
			t.Fatal("ImportResult did not retain the canonical source Call and alias")
		}
		callApp, callAppOK := importLawCallApplication(project, sourceShard, call)
		if !callAppOK {
			t.Fatal("missing Call Application for ImportResult Call")
		}
		candidate, candidateOK := project.CandidateForSeed(callApp, loader)
		if !candidateOK {
			t.Fatal("Import loader Seed did not select its Call Candidate")
		}
		loadedApp, loadedResult, loadOK := project.CandidateLoad(candidate)
		if !loadOK || !importLawSameApplication(project, loadedApp, application) {
			t.Fatal("loader Candidate did not retain its exact Import Application")
		}
		candidateShard, candidateEntry, candidateDemandOK := project.LoadDemand(loadedResult)
		if !candidateDemandOK || candidateShard != loadedShard || candidateEntry != loadedEntry {
			t.Fatal("loader Candidate did not retain the imported root demand")
		}
		from, returned, to, destination, outcomeOK := project.ImportOutcome(application, program.OutcomeReturn)
		if !outcomeOK || from != loadedShard || to != sourceShard || returned == 0 || destination == 0 {
			t.Fatal("ImportOutcome(Return) did not retain both canonical boundary sides")
		}
		return importLawImporter{
			shard: sourceShard, importTerm: importTerm, call: call,
			importApp: application, callApp: callApp, candidate: candidate,
			loadedShard: loadedShard, loadedEntry: loadedEntry, loadedReturn: returned,
			destination: destination, importAliasCell: alias,
		}
	}
	t.Fatal("missing typed Import Application for importer")
	return importLawImporter{}
}

func importLawCallApplication(project *link.Link, wantedShard link.Shard, wantedCall program.Term) (link.Application, bool) {
	if project == nil {
		return link.Application{}, false
	}
	for index := 0; index < project.ApplicationCount(); index++ {
		application, ok := project.ApplicationAt(index)
		if !ok {
			return link.Application{}, false
		}
		shard, call, callOK := project.CallApplication(application)
		if callOK && shard == wantedShard && call == wantedCall {
			return application, true
		}
	}
	return link.Application{}, false
}

func importLawSameApplication(project *link.Link, left, right link.Application) bool {
	order, ok := project.CompareApplication(left, right)
	return ok && order == 0
}

func importLawSameCandidate(project *link.Link, left, right link.Candidate) bool {
	order, ok := project.CompareCandidate(left, right)
	return ok && order == 0
}

// An Import Application and an ImportOutcome query are structural evidence,
// not root demand. The imported Entry remains absent until a selector resolves
// the loader Candidate and explicitly binds that existing candidate-zero root.
func TestImportLawImportedEntryNeedsAnActiveRelation(t *testing.T) {
	t.Run("unselected import never seeds the registered module", func(t *testing.T) {
		fixture := importLawFixtureFor(t, "main")
		importer := fixture.importers[0]
		solver, err := engine.New(fixture.link)
		if err != nil {
			t.Fatal(err)
		}
		transport := importLawFactor(t, solver, "unselected-transport")
		outcome := importLawFactor(t, solver, "unselected-outcome")
		loaded := importLawFactor(t, solver, "unselected-loaded")
		importLawDeclareRelation(t, solver, transport, "unselected-import", importer.callApp, 1, func(access engine.Access[uint64, importLawBits]) bool {
			return access.Set(0, importLawSelected)
		})
		importLawDeclareAt(t, solver, loaded, "unselected-dependency-entry", importer.loadedShard, importer.loadedEntry, func(access engine.Access[uint64, importLawBits]) bool {
			return access.Set(0, importLawLoaded)
		})
		query := importLawOutcomeQuery(t, solver, outcome, importer)

		state := importLawSealAndSolve(t, solver)
		if got, present := query.Read(state); present || got != 0 {
			t.Fatalf("unselected importer outcome = %d/%t, want absent", got, present)
		}
	})

	t.Run("active import binds the existing root and executes it", func(t *testing.T) {
		fixture := importLawFixtureFor(t, "main")
		importer := fixture.importers[0]
		solver, err := engine.New(fixture.link)
		if err != nil {
			t.Fatal(err)
		}
		selector := importLawFactor(t, solver, "selected-selector")
		transport := importLawFactor(t, solver, "selected-transport")
		loaded := importLawFactor(t, solver, "selected-loaded")
		outcome := importLawFactor(t, solver, "selected-outcome")
		importLawDeclareAt(t, solver, loaded, "selected-dependency-entry", importer.loadedShard, importer.loadedEntry, func(access engine.Access[uint64, importLawBits]) bool {
			return access.Set(0, importLawLoaded)
		})
		ingress := importLawDeclareRelation(t, solver, transport, "selected-ingress", importer.callApp, 1, func(access engine.Access[uint64, importLawBits]) bool {
			candidate, application, visible := access.Selection()
			if !visible || !importLawSameCandidate(fixture.link, candidate, importer.candidate) || !importLawSameApplication(fixture.link, application, importer.callApp) {
				return false
			}
			return access.Set(0, importLawSelected)
		})
		var loadedRead engine.ReadRef[uint64, importLawBits]
		egress := importLawDeclareRelation(t, solver, outcome, "selected-egress", importer.callApp, 1, func(access engine.Access[uint64, importLawBits]) bool {
			candidate, application, visible := access.Selection()
			if !visible || !importLawSameCandidate(fixture.link, candidate, importer.candidate) || !importLawSameApplication(fixture.link, application, importer.callApp) {
				return false
			}
			value, present, valid := engine.ReadAt(access, loadedRead, 0)
			if !valid || !present {
				return false
			}
			return access.Set(0, value)
		})
		var readOK bool
		loadedRead, readOK = engine.Read(egress, 0, loaded)
		if !readOK {
			t.Fatal("Read did not bind the imported Entry fact")
		}
		importLawDeclareAt(t, solver, selector, "selected-import-call", importer.shard, importer.call, func(access engine.Access[uint64, importLawBits]) bool {
			ingressOK := engine.Activate(access, ingress, importer.candidate, func(relation engine.Relation) bool {
				caller, callerOK := relation.Caller(importer.call)
				root, rootOK := relation.Root(importer.loadedShard, importer.loadedEntry)
				return callerOK && rootOK && relation.Bind(root, caller)
			})
			egressOK := engine.Activate(access, egress, importer.candidate, func(relation engine.Relation) bool {
				loadedRoot, loadedOK := relation.Root(importer.loadedShard, importer.loadedEntry)
				destination, destinationOK := relation.Caller(importer.destination)
				return loadedOK && destinationOK && relation.Bind(destination, loadedRoot)
			})
			return ingressOK && egressOK
		})
		query := importLawOutcomeQuery(t, solver, outcome, importer)

		state := importLawSealAndSolve(t, solver)
		if got := importLawRead(t, query, state); got != importLawLoaded {
			t.Fatalf("active importer outcome = %d, want loaded fact %d", got, importLawLoaded)
		}
	})
}

// Two loader Candidates may resolve to one candidate-zero imported Entry.
// Their ingress facts join at that shared root, while two ordered egress
// relations still read each importer's own caller fact and publish it only to
// that importer's typed ImportOutcome destination.
func TestImportLawSharedRootPreservesImporterProvenance(t *testing.T) {
	fixture := importLawFixtureFor(t, "first", "second")
	first, second := fixture.importers[0], fixture.importers[1]
	if first.loadedShard != second.loadedShard || first.loadedEntry != second.loadedEntry {
		t.Fatal("two ImportApplications did not expose one shared candidate-zero root")
	}
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatal(err)
	}
	selector := importLawFactor(t, solver, "shared-selector")
	callerFacts := importLawFactor(t, solver, "shared-caller-facts")
	sharedFacts := importLawFactor(t, solver, "shared-root-facts")
	entryFact := importLawFactor(t, solver, "shared-entry-observed")
	outcomes := importLawFactor(t, solver, "shared-outcomes")
	importLawDeclareAt(t, solver, callerFacts, "shared-first-caller", first.shard, first.call, func(access engine.Access[uint64, importLawBits]) bool {
		return access.Set(0, importLawFirst)
	})
	importLawDeclareAt(t, solver, callerFacts, "shared-second-caller", second.shard, second.call, func(access engine.Access[uint64, importLawBits]) bool {
		return access.Set(0, importLawSecond)
	})
	importLawDeclareAt(t, solver, entryFact, "shared-dependency-entry", first.loadedShard, first.loadedEntry, func(access engine.Access[uint64, importLawBits]) bool {
		return access.Set(0, importLawLoaded)
	})

	var firstIngressRead, secondIngressRead engine.ReadRef[uint64, importLawBits]
	firstIngress := importLawDeclareRelation(t, solver, sharedFacts, "shared-first-ingress", first.callApp, 1, func(access engine.Access[uint64, importLawBits]) bool {
		value, present, valid := engine.ReadAt(access, firstIngressRead, 0)
		if !valid || !present {
			return false
		}
		return access.Set(0, value)
	})
	secondIngress := importLawDeclareRelation(t, solver, sharedFacts, "shared-second-ingress", second.callApp, 1, func(access engine.Access[uint64, importLawBits]) bool {
		value, present, valid := engine.ReadAt(access, secondIngressRead, 0)
		if !valid || !present {
			return false
		}
		return access.Set(0, value)
	})
	var readOK bool
	firstIngressRead, readOK = engine.Read(firstIngress, 0, callerFacts)
	if !readOK {
		t.Fatal("Read did not bind first ingress caller fact")
	}
	secondIngressRead, readOK = engine.Read(secondIngress, 0, callerFacts)
	if !readOK {
		t.Fatal("Read did not bind second ingress caller fact")
	}

	var firstCallerRead, firstSharedRead, secondCallerRead, secondSharedRead engine.ReadRef[uint64, importLawBits]
	firstEgress := importLawDeclareRelation(t, solver, outcomes, "shared-first-egress", first.callApp, 2, func(access engine.Access[uint64, importLawBits]) bool {
		caller, callerPresent, callerValid := engine.ReadAt(access, firstCallerRead, 0)
		shared, sharedPresent, sharedValid := engine.ReadAt(access, firstSharedRead, 0)
		if !callerValid || !sharedValid || !callerPresent || !sharedPresent {
			return false
		}
		if shared != importLawBoth {
			return access.Set(0, 0)
		}
		return access.Set(0, caller)
	})
	secondEgress := importLawDeclareRelation(t, solver, outcomes, "shared-second-egress", second.callApp, 2, func(access engine.Access[uint64, importLawBits]) bool {
		caller, callerPresent, callerValid := engine.ReadAt(access, secondCallerRead, 0)
		shared, sharedPresent, sharedValid := engine.ReadAt(access, secondSharedRead, 0)
		if !callerValid || !sharedValid || !callerPresent || !sharedPresent {
			return false
		}
		if shared != importLawBoth {
			return access.Set(0, 0)
		}
		return access.Set(0, caller)
	})
	firstCallerRead, readOK = engine.Read(firstEgress, 0, callerFacts)
	if !readOK {
		t.Fatal("Read did not bind the first ordered caller input")
	}
	firstSharedRead, readOK = engine.Read(firstEgress, 1, sharedFacts)
	if !readOK {
		t.Fatal("Read did not bind the first ordered shared-root input")
	}
	secondCallerRead, readOK = engine.Read(secondEgress, 0, callerFacts)
	if !readOK {
		t.Fatal("Read did not bind the second ordered caller input")
	}
	secondSharedRead, readOK = engine.Read(secondEgress, 1, sharedFacts)
	if !readOK {
		t.Fatal("Read did not bind the second ordered shared-root input")
	}

	bindImporter := func(index int, importer importLawImporter, ingress, egress *engine.Rule[uint64, importLawBits]) {
		importLawDeclareAt(t, solver, selector, "shared-selector-"+string(rune('a'+index)), importer.shard, importer.call, func(access engine.Access[uint64, importLawBits]) bool {
			ingressOK := engine.Activate(access, ingress, importer.candidate, func(relation engine.Relation) bool {
				caller, callerOK := relation.Caller(importer.call)
				root, rootOK := relation.Root(importer.loadedShard, importer.loadedEntry)
				return callerOK && rootOK && relation.Bind(root, caller)
			})
			egressOK := engine.Activate(access, egress, importer.candidate, func(relation engine.Relation) bool {
				caller, callerOK := relation.Caller(importer.call)
				root, rootOK := relation.Root(importer.loadedShard, importer.loadedEntry)
				destination, destinationOK := relation.Caller(importer.destination)
				return callerOK && rootOK && destinationOK && relation.Bind(destination, caller, root)
			})
			return ingressOK && egressOK
		})
	}
	bindImporter(0, first, firstIngress, firstEgress)
	bindImporter(1, second, secondIngress, secondEgress)
	firstQuery := importLawOutcomeQuery(t, solver, outcomes, first)
	secondQuery := importLawOutcomeQuery(t, solver, outcomes, second)

	state := importLawSealAndSolve(t, solver)
	if got := importLawRead(t, firstQuery, state); got != importLawFirst {
		t.Fatalf("first importer outcome = %d, want its own fact %d", got, importLawFirst)
	}
	if got := importLawRead(t, secondQuery, state); got != importLawSecond {
		t.Fatalf("second importer outcome = %d, want its own fact %d", got, importLawSecond)
	}
}
