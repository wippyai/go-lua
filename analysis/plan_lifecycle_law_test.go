package analysis

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestPlanCloseWaitsForActiveSolveLease(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil || plan.state == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	state, leased := plan.acquire()
	if !leased {
		t.Fatal("failed to acquire plan lease")
	}
	closed := make(chan bool, 1)
	go func() { closed <- plan.Close() }()
	select {
	case <-closed:
		t.Fatal("Close returned while an active lease was held")
	case <-time.After(20 * time.Millisecond):
	}
	state.releaseLease()
	select {
	case ok := <-closed:
		if !ok {
			t.Fatal("Close failed after the active lease released")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not complete after the active lease released")
	}
	if got := plan.SourceID(); got.Available() {
		t.Fatal("closed Plan retained a readable SourceID")
	}
	if result, solveStatus := plan.Solve(context.Background()); result != nil || solveStatus != AnalyzeInvalid {
		t.Fatalf("closed Plan Solve = %v/%v", solveStatus, result)
	}
}

func TestArtifactCacheSurvivesSequentialPlanClose(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local retained_cache_probe = 17
return retained_cache_probe`, contract)
	receipt, ok := composite.Global()
	if !ok || !receipt.Available() {
		t.Fatal("global schema unavailable")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	mounted, mountedOK := mounts.Program(shard)
	if !shardOK || !mountedOK || mounted == nil {
		t.Fatal("probe mount unavailable")
	}
	grammar, grammarOK := composite.ArtifactGrammar(receipt)
	compileKey, keyOK := programartifact.NewCompileKey(mounted, grammar)
	if !grammarOK || !keyOK || !compileKey.Available() {
		t.Fatal("compile key unavailable")
	}
	first, firstStatus, firstDiagnostics := CompileWithDiagnostics(linked)
	if firstStatus != CompileComplete || first == nil || first.state == nil || first.state.artifacts == nil {
		t.Fatalf("first compile = %v/%v diagnostics=%+v", firstStatus, first, firstDiagnostics)
	}
	firstArtifact := first.state.artifacts.byProgram[mounted.ContentID()]
	if len(first.state.artifacts.mounts) == 0 {
		t.Fatal("first compile retained no mounted artifact")
	}
	firstTemplate := first.state.artifacts.mounts[0].template
	firstRoles := first.state.artifacts.mounts[0].roles
	if firstArtifact == nil || !first.Close() {
		t.Fatal("first Plan did not retain a closable cached artifact")
	}
	second, secondStatus := Compile(linked)
	if secondStatus != CompileComplete || second == nil || second.state == nil || second.state.artifacts == nil {
		t.Fatalf("second compile = %v/%v", secondStatus, second)
	}
	defer second.Close()
	secondArtifact := second.state.artifacts.byProgram[mounted.ContentID()]
	if len(second.state.artifacts.mounts) == 0 {
		t.Fatal("second compile retained no mounted artifact")
	}
	if secondArtifact != firstArtifact || second.state.artifacts.mounts[0].template != firstTemplate || second.state.artifacts.mounts[0].roles != firstRoles {
		t.Fatal("sequential Compile→Close→Compile did not reuse the immutable artifact/template")
	}
	globalArtifactCache.Lock()
	entry := globalArtifactCache.entries[compileKey.ID()]
	globalArtifactCache.Unlock()
	if entry == nil || !entry.complete || entry.artifact != firstArtifact || entry.template != firstTemplate || entry.roles != firstRoles {
		t.Fatal("successful artifact/template cache entry was not retained independently of Plan lifetime")
	}
}

func TestArtifactCacheConcurrentCompileSharesOneImmutableEntry(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local concurrent_cache_probe = 31
return concurrent_cache_probe`, contract)
	const workers = 8
	plans := make([]*Plan, workers)
	statuses := make([]CompileStatus, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := range plans {
		go func(index int) {
			defer wait.Done()
			plans[index], statuses[index] = Compile(linked)
		}(index)
	}
	wait.Wait()
	defer func() {
		for _, plan := range plans {
			if plan != nil {
				plan.Close()
			}
		}
	}()
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	mounted, mountedOK := mounts.Program(shard)
	if !shardOK || !mountedOK || mounted == nil {
		t.Fatal("concurrent probe mount unavailable")
	}
	programID := mounted.ContentID()
	var artifact any
	var template any
	for index, plan := range plans {
		if statuses[index] != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
			t.Fatalf("concurrent Compile[%d] = %v/%v", index, statuses[index], plan)
		}
		gotArtifact := plan.state.artifacts.byProgram[programID]
		if len(plan.state.artifacts.mounts) == 0 {
			t.Fatalf("concurrent Compile[%d] retained no mounted artifact", index)
		}
		if gotArtifact == nil {
			t.Fatalf("concurrent Compile[%d] cache payload unavailable", index)
		}
		if artifact == nil {
			artifact = gotArtifact
		} else if artifact != gotArtifact {
			t.Fatal("concurrent Compile did not join one immutable artifact cache entry")
		}
		gotTemplate := plan.state.artifacts.mounts[0].template
		if gotTemplate == nil {
			t.Fatalf("concurrent Compile[%d] template unavailable", index)
		}
		if template == nil {
			template = gotTemplate
		} else if template != gotTemplate {
			t.Fatal("concurrent Compile did not join one immutable template cache entry")
		}
	}
}

func TestArtifactCacheChangedFullKeyDoesNotAlias(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	left := mustLink(t, `return 101`, contract)
	right := mustLink(t, `return 102`, contract)
	leftPlan, leftStatus := Compile(left)
	rightPlan, rightStatus := Compile(right)
	if leftStatus != CompileComplete || rightStatus != CompileComplete || leftPlan == nil || rightPlan == nil {
		t.Fatalf("changed-key compile = %v/%v %v/%v", leftStatus, leftPlan, rightStatus, rightPlan)
	}
	defer leftPlan.Close()
	defer rightPlan.Close()
	receipt, receiptOK := composite.Global()
	leftInput := planLifecycleInput(t, left)
	rightInput := planLifecycleInput(t, right)
	grammar, grammarOK := composite.ArtifactGrammar(receipt)
	leftKey, leftKeyOK := programartifact.NewCompileKey(leftInput, grammar)
	rightKey, rightKeyOK := programartifact.NewCompileKey(rightInput, grammar)
	if !receiptOK || !grammarOK || !leftKeyOK || !rightKeyOK || leftKey.ID() == rightKey.ID() {
		t.Fatal("changed Program did not issue a distinct full compiler key")
	}
	leftArtifact := leftPlan.state.artifacts.byProgram[leftInput.ContentID()]
	rightArtifact := rightPlan.state.artifacts.byProgram[rightInput.ContentID()]
	if leftArtifact == nil || rightArtifact == nil || leftArtifact == rightArtifact || leftPlan.state.artifacts.mounts[0].template == nil || rightPlan.state.artifacts.mounts[0].template == nil || leftPlan.state.artifacts.mounts[0].template == rightPlan.state.artifacts.mounts[0].template {
		t.Fatal("changed full compiler key aliased one immutable artifact/template")
	}
}

func TestArtifactTemplateReusesAcrossIndependentEqualProgramsAndLinks(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	const source = `local outer = 43
local function shared_template_probe(value, ...)
  local function nested()
    return outer, value
  end
  return nested()
end
return shared_template_probe(43)`
	leftLink := mustLink(t, source, contract)
	rightLink := mustLink(t, source, contract)
	if leftLink == rightLink {
		t.Fatal("independent Link fixtures aliased")
	}
	left, leftStatus := Compile(leftLink)
	right, rightStatus := Compile(rightLink)
	if leftStatus != CompileComplete || rightStatus != CompileComplete || left == nil || right == nil {
		t.Fatalf("independent compile = %v/%v %v/%v", leftStatus, left, rightStatus, right)
	}
	defer left.Close()
	defer right.Close()
	if len(left.state.artifacts.mounts) != 1 || len(right.state.artifacts.mounts) != 1 {
		t.Fatal("independent template mount cardinality")
	}
	leftMount, rightMount := left.state.artifacts.mounts[0], right.state.artifacts.mounts[0]
	leftArtifact := left.state.artifacts.byProgram[leftMount.programID]
	rightArtifact := right.state.artifacts.byProgram[rightMount.programID]
	if leftArtifact == nil || leftArtifact != rightArtifact || leftMount.template == nil || leftMount.template != rightMount.template || leftMount.roles == nil || leftMount.roles != rightMount.roles {
		t.Fatal("equal Programs in independent Links did not share one content-addressed template")
	}
	leftBoundaryCount, leftBoundariesPublished := leftMount.program.FunctionBoundaryCount()
	if !leftBoundariesPublished || leftBoundaryCount == 0 {
		t.Fatalf("shared Program function interfaces = %d/%v", leftBoundaryCount, leftBoundariesPublished)
	}
	if left.state.committed.program != nil || right.state.committed.program != nil || len(left.state.querySites) != 0 || len(right.state.querySites) != 0 {
		t.Fatal("Compile instantiated runtime topology before Solve ownership")
	}
	leftDiagnostic, leftInstantiated := left.state.instantiateRuntimeTopology()
	rightDiagnostic, rightInstantiated := right.state.instantiateRuntimeTopology()
	if !leftInstantiated || !rightInstantiated {
		t.Fatalf("runtime topology instantiation = %v/%+v %v/%+v", leftInstantiated, leftDiagnostic, rightInstantiated, rightDiagnostic)
	}
	leftGraph, rightGraph := left.state.committed.program, right.state.committed.program
	if _, replayed := left.state.instantiateRuntimeTopology(); !replayed || left.state.committed.program != leftGraph {
		t.Fatal("repeated Solve boundary rematerialized the left runtime topology")
	}
	if _, replayed := right.state.instantiateRuntimeTopology(); !replayed || right.state.committed.program != rightGraph {
		t.Fatal("repeated Solve boundary rematerialized the right runtime topology")
	}
	if left.state.committed.program == nil || right.state.committed.program == nil {
		t.Fatal("Solve boundary did not retain the runtime topology")
	}
	// The function declaration interface is owned by program/artifact. Both
	// Links resolve it through the one shared content-addressed Artifact, so
	// the interface outlives every Link-local release/replay.
	rightBoundaryCount, rightBoundariesPublished := rightMount.program.FunctionBoundaryCount()
	if !rightBoundariesPublished || leftBoundaryCount != rightBoundaryCount {
		t.Fatal("independent Links did not share one function declaration authority")
	}
	programCaptures, programVarargs := 0, 0
	bodyCount, bodiesPublished := leftMount.program.BodyCount()
	if !bodiesPublished {
		t.Fatal("shared cold Body family unavailable")
	}
	for index := 0; index < leftBoundaryCount; index++ {
		reusable, reusableOK := leftMount.program.FunctionBoundaryAt(index)
		bodyOutcomes := 0
		for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
			body, bodyOK := leftMount.program.BodyAt(bodyIndex)
			if bodyOK && body.ID() == reusable.BodyID() {
				bodyOutcomes = body.OutcomeCount()
				break
			}
		}
		if !reusableOK || !reusable.Available() || !reusable.ID().Available() || !reusable.BodyID().Available() || !reusable.EntryID().Available() || bodyOutcomes == 0 {
			t.Fatalf("function interface[%d] is incomplete", index)
		}
		for position := 0; position < reusable.FormalCount(); position++ {
			port, portOK := leftMount.program.FunctionFormalFor(index, position)
			if !portOK || !port.ID().Available() || !port.CellID().Available() || !port.StorageCellID().Available() {
				t.Fatalf("function formal[%d,%d] is incomplete", index, position)
			}
		}
		if vararg, varargOK := leftMount.program.FunctionVarargFor(index, 0); varargOK {
			if !vararg.ID().Available() || !vararg.CellID().Available() {
				t.Fatalf("function vararg[%d] is incomplete", index)
			}
			programVarargs++
		}
		for position := 0; position < reusable.CaptureCount(); position++ {
			capture, captureOK := leftMount.program.FunctionCaptureFor(index, position)
			if !captureOK || !capture.ID().Available() || capture.InnerCellID() == capture.OuterCellID() ||
				!capture.InnerBodyID().Available() || !capture.OuterBodyID().Available() || capture.InnerBodyID() == capture.OuterBodyID() {
				t.Fatalf("function capture[%d,%d] lost reusable direction", index, position)
			}
			programCaptures++
		}
	}
	if programCaptures == 0 || programVarargs == 0 {
		t.Fatalf("function interface fixture missed capture/vararg rows: %d/%d", programCaptures, programVarargs)
	}
	if left.state.binding == nil || right.state.binding == nil || left.state.binding == right.state.binding {
		t.Fatal("independent Links aliased the Link-local binding")
	}
}

func compileArtifactSet(t *testing.T, linked *link.Link) *compiledArtifactSet {
	t.Helper()
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	t.Cleanup(func() { _ = plan.Close() })
	return plan.state.artifacts
}

func TestArtifactCacheReusesAcrossMountPermutation(t *testing.T) {
	leftProg := planLawProgram(t, `return 11`)
	rightProg := planLawProgram(t, `return 22`)
	first := compileArtifactSet(t, planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: leftProg}, {Name: "right", Program: rightProg},
	}))
	second := compileArtifactSet(t, planLawMountedLink(t, []linkproject.Module{
		{Name: "right", Program: rightProg}, {Name: "left", Program: leftProg},
	}))
	if first.byProgram[leftProg.ContentID()] == nil || first.byProgram[leftProg.ContentID()] != second.byProgram[leftProg.ContentID()] ||
		first.byProgram[rightProg.ContentID()] == nil || first.byProgram[rightProg.ContentID()] != second.byProgram[rightProg.ContentID()] {
		t.Fatal("mount permutation did not reuse cached artifacts")
	}
}

func TestArtifactCacheInvalidatesOnlyReplacedSibling(t *testing.T) {
	leftProg := planLawProgram(t, `return 11`)
	rightProg := planLawProgram(t, `return 22`)
	replacement := planLawProgram(t, `return 33`)
	base := compileArtifactSet(t, planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: leftProg}, {Name: "right", Program: rightProg},
	}))
	changed := compileArtifactSet(t, planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: replacement}, {Name: "right", Program: rightProg},
	}))
	if base.byProgram[leftProg.ContentID()] == nil || changed.byProgram[replacement.ContentID()] == nil ||
		base.byProgram[leftProg.ContentID()] == changed.byProgram[replacement.ContentID()] {
		t.Fatal("replaced sibling reused the previous artifact")
	}
	if base.byProgram[rightProg.ContentID()] == nil || base.byProgram[rightProg.ContentID()] != changed.byProgram[rightProg.ContentID()] {
		t.Fatal("unrelated sibling lost its cached artifact")
	}
}

func TestArtifactCacheReusesAcrossActorRename(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	prog := planLawProgram(t, `return 7`)
	seal := func(actor string) *link.Link {
		t.Helper()
		linked, err := link.Seal(&link.Spec{
			Target:  contract,
			Modules: []linkproject.Module{{Name: "main", Program: prog}},
			Module: linkmodule.Spec{
				Actors:             []linkmodule.ActorSpec{{Name: actor}},
				ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{Actor: actor, Instances: []string{"cache-main"}, Representative: "cache-main"}},
				AnalysisRoots:      []linkmodule.AnalysisRootSpec{{Name: "main", Module: "main", Actor: actor, Instance: "cache-main"}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return linked
	}
	first := compileArtifactSet(t, seal("actor"))
	second := compileArtifactSet(t, seal("other-actor"))
	if first.byProgram[prog.ContentID()] == nil || first.byProgram[prog.ContentID()] != second.byProgram[prog.ContentID()] {
		t.Fatal("actor rename did not reuse the cached artifact")
	}
}

func planLifecycleInput(t testing.TB, linked *link.Link) *program.Program {
	t.Helper()
	if linked == nil || linked.Project() == nil {
		t.Fatal("link unavailable")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	mounted, mountedOK := mounts.Program(shard)
	if !shardOK || !mountedOK || mounted == nil {
		t.Fatal("mounted Program unavailable")
	}
	return mounted
}
