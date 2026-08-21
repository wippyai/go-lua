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

func TestWorkspaceCloseWaitsForAdmittedCompileAndPlanThenReleasesProducts(t *testing.T) {
	workspace := NewWorkspace()
	plan, status := workspace.Compile(planLawLink(t))
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("Workspace.Compile = %v/%v", status, plan)
	}
	if products, admitted := workspace.beginCompile(); !admitted || products == nil {
		t.Fatal("failed to admit concurrent Workspace compile")
	}
	closed := make(chan bool, 1)
	go func() { closed <- workspace.Close() }()
	select {
	case <-closed:
		t.Error("Workspace.Close returned while a compile and Plan were active")
	case <-time.After(20 * time.Millisecond):
	}
	if !plan.Close() {
		t.Error("active Workspace Plan did not close")
	}
	select {
	case <-closed:
		t.Error("Workspace.Close returned while an admitted compile was active")
	case <-time.After(20 * time.Millisecond):
	}
	workspace.finishCompile(false)
	select {
	case ok := <-closed:
		if !ok {
			t.Error("Workspace.Close failed after its Plan and compile closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Workspace.Close did not complete after all owned work closed")
	}
	workspace.lifecycleMu.Lock()
	released := workspace.closed && workspace.artifacts == nil && workspace.compiles == 0 && workspace.plans == 0
	workspace.lifecycleMu.Unlock()
	if !released || plan.workspace != nil || plan.state.artifacts != nil {
		t.Fatal("closed Workspace retained compiler products or its closed Plan")
	}
	if next, nextStatus := workspace.Compile(planLawLink(t)); next != nil || nextStatus != CompileInvalid {
		t.Fatalf("closed Workspace.Compile = %v/%v", nextStatus, next)
	}
}

func TestConvenienceCompileReleasesItsEphemeralWorkspaceWithPlan(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local ephemeral_workspace_probe = 13
return ephemeral_workspace_probe`, contract)
	mounted := planLifecycleInput(t, linked)
	first, firstStatus := Compile(linked)
	if firstStatus != CompileComplete || first == nil || first.state == nil || first.state.artifacts == nil || first.workspace == nil {
		t.Fatalf("first convenience compile = %v/%v", firstStatus, first)
	}
	firstWorkspace := first.workspace
	firstProduct := first.state.artifacts.products[mounted.ContentID()]
	firstArtifact := firstProduct.Artifact
	firstMount := first.state.artifacts.mounts[0]
	firstWorkspace.lifecycleMu.Lock()
	owned := firstWorkspace.ephemeral && firstWorkspace.closing && !firstWorkspace.closed && firstWorkspace.plans == 1 && firstWorkspace.artifacts != nil
	firstWorkspace.lifecycleMu.Unlock()
	if !owned || firstArtifact == nil || firstMount.Snapshot == nil || firstProduct.Template == nil || firstProduct.Roles == nil {
		t.Fatal("convenience Plan did not own one live ephemeral Workspace product")
	}
	if !first.Close() {
		t.Fatal("first convenience Plan did not close")
	}
	firstWorkspace.lifecycleMu.Lock()
	released := firstWorkspace.closed && firstWorkspace.artifacts == nil && firstWorkspace.plans == 0 && firstWorkspace.compiles == 0
	firstWorkspace.lifecycleMu.Unlock()
	if !released || first.workspace != nil || first.state.artifacts != nil || first.state.binding != nil || first.state.ordinary != nil || first.state.composition.Published() || first.state.selectSites != nil || first.state.selectHandlers != nil {
		t.Fatal("Plan.Close retained its ephemeral Workspace or compiler products")
	}

	second, secondStatus := Compile(linked)
	if secondStatus != CompileComplete || second == nil || second.state == nil || second.state.artifacts == nil {
		t.Fatalf("second convenience compile = %v/%v", secondStatus, second)
	}
	defer second.Close()
	secondProduct := second.state.artifacts.products[mounted.ContentID()]
	secondArtifact := secondProduct.Artifact
	secondMount := second.state.artifacts.mounts[0]
	if secondArtifact == nil || secondArtifact == firstArtifact || secondMount.Snapshot == firstMount.Snapshot || secondProduct.Template == firstProduct.Template || secondProduct.Roles == firstProduct.Roles {
		t.Fatal("unrelated convenience Compiles shared one Workspace product")
	}
	if secondArtifact.ID() != firstArtifact.ID() || secondMount.Snapshot.ArtifactID() != firstMount.Snapshot.ArtifactID() || secondProduct.Template.ArtifactID() != firstProduct.Template.ArtifactID() {
		t.Fatal("ephemeral Workspace boundary changed immutable product identity")
	}
}

func TestConvenienceAnalyzeReleasesEphemeralProductsAfterDetach(t *testing.T) {
	workspace := newWorkspace(true)
	result, status := workspace.analyze(context.Background(), planLawLink(t))
	if status != AnalyzeComplete || result == nil {
		t.Fatalf("convenience Analyze = %v/%v", status, result)
	}
	workspace.lifecycleMu.Lock()
	released := workspace.ephemeral && workspace.closed && workspace.artifacts == nil && workspace.compiles == 0 && workspace.plans == 0
	workspace.lifecycleMu.Unlock()
	if !released {
		t.Fatal("convenience Analyze retained its ephemeral compiler products after Result detachment")
	}
}

func TestWorkspaceReusesProductsAcrossSequentialPlanClose(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace()
	defer workspace.Close()
	linked := mustLink(t, `local retained_cache_probe = 17
return retained_cache_probe`, contract)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	mounted, mountedOK := mounts.Program(shard)
	if !shardOK || !mountedOK || mounted == nil {
		t.Fatal("probe mount unavailable")
	}
	first, firstStatus, firstDiagnostics := workspace.CompileWithDiagnostics(linked)
	if firstStatus != CompileComplete || first == nil || first.state == nil || first.state.artifacts == nil {
		t.Fatalf("first compile = %v/%v diagnostics=%+v", firstStatus, first, firstDiagnostics)
	}
	firstProduct := first.state.artifacts.products[mounted.ContentID()]
	firstArtifact := firstProduct.Artifact
	if len(first.state.artifacts.mounts) == 0 {
		t.Fatal("first compile retained no mounted artifact")
	}
	firstTemplate := firstProduct.Template
	firstRoles := firstProduct.Roles
	if firstArtifact == nil || !first.Close() {
		t.Fatal("first Plan did not retain a closable cached artifact")
	}
	second, secondStatus := workspace.Compile(linked)
	if secondStatus != CompileComplete || second == nil || second.state == nil || second.state.artifacts == nil {
		t.Fatalf("second compile = %v/%v", secondStatus, second)
	}
	defer second.Close()
	secondProduct := second.state.artifacts.products[mounted.ContentID()]
	secondArtifact := secondProduct.Artifact
	if len(second.state.artifacts.mounts) == 0 {
		t.Fatal("second compile retained no mounted artifact")
	}
	if secondArtifact != firstArtifact || secondProduct.Template != firstTemplate || secondProduct.Roles != firstRoles {
		t.Fatal("sequential Workspace Compile→Plan.Close→Compile did not reuse the immutable product")
	}
}

func TestWorkspaceConcurrentCompileSharesOneImmutableProduct(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local concurrent_cache_probe = 31
return concurrent_cache_probe`, contract)
	workspace := NewWorkspace()
	defer workspace.Close()
	const workers = 8
	plans := make([]*Plan, workers)
	statuses := make([]CompileStatus, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := range plans {
		go func(index int) {
			defer wait.Done()
			plans[index], statuses[index] = workspace.Compile(linked)
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
		gotProduct := plan.state.artifacts.products[programID]
		gotArtifact := gotProduct.Artifact
		if len(plan.state.artifacts.mounts) == 0 {
			t.Fatalf("concurrent Compile[%d] retained no mounted artifact", index)
		}
		if gotArtifact == nil {
			t.Fatalf("concurrent Compile[%d] cache payload unavailable", index)
		}
		if artifact == nil {
			artifact = gotArtifact
		} else if artifact != gotArtifact {
			t.Fatal("concurrent Workspace Compile did not join one immutable artifact product")
		}
		gotTemplate := gotProduct.Template
		if gotTemplate == nil {
			t.Fatalf("concurrent Compile[%d] template unavailable", index)
		}
		if template == nil {
			template = gotTemplate
		} else if template != gotTemplate {
			t.Fatal("concurrent Workspace Compile did not join one immutable template")
		}
	}
}

func TestWorkspaceChangedFullKeyDoesNotAlias(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	left := mustLink(t, `return 101`, contract)
	right := mustLink(t, `return 102`, contract)
	workspace := NewWorkspace()
	defer workspace.Close()
	leftPlan, leftStatus := workspace.Compile(left)
	rightPlan, rightStatus := workspace.Compile(right)
	if leftStatus != CompileComplete || rightStatus != CompileComplete || leftPlan == nil || rightPlan == nil {
		t.Fatalf("changed-key compile = %v/%v %v/%v", leftStatus, leftPlan, rightStatus, rightPlan)
	}
	defer leftPlan.Close()
	defer rightPlan.Close()
	leftInput := planLifecycleInput(t, left)
	rightInput := planLifecycleInput(t, right)
	grammar := workspace.compilation.ExecutionSchemaID()
	leftKey, leftKeyOK := programartifact.NewCompileKey(leftInput, grammar)
	rightKey, rightKeyOK := programartifact.NewCompileKey(rightInput, grammar)
	if !grammar.Available() || !leftKeyOK || !rightKeyOK || leftKey.ID() == rightKey.ID() {
		t.Fatal("changed Program did not issue a distinct full compiler key")
	}
	leftProduct := leftPlan.state.artifacts.products[leftInput.ContentID()]
	rightProduct := rightPlan.state.artifacts.products[rightInput.ContentID()]
	leftArtifact := leftProduct.Artifact
	rightArtifact := rightProduct.Artifact
	if leftArtifact == nil || rightArtifact == nil || leftArtifact == rightArtifact || leftProduct.Template == nil || rightProduct.Template == nil || leftProduct.Template == rightProduct.Template {
		t.Fatal("changed full compiler key aliased one immutable artifact/template")
	}
}

func TestWorkspaceReusesTemplateAcrossIndependentEqualProgramsAndLinks(t *testing.T) {
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
	workspace := NewWorkspace()
	defer workspace.Close()
	left, leftStatus := workspace.Compile(leftLink)
	right, rightStatus := workspace.Compile(rightLink)
	if leftStatus != CompileComplete || rightStatus != CompileComplete || left == nil || right == nil {
		t.Fatalf("independent compile = %v/%v %v/%v", leftStatus, left, rightStatus, right)
	}
	defer left.Close()
	defer right.Close()
	if len(left.state.artifacts.mounts) != 1 || len(right.state.artifacts.mounts) != 1 {
		t.Fatal("independent template mount cardinality")
	}
	leftMount, rightMount := left.state.artifacts.mounts[0], right.state.artifacts.mounts[0]
	leftProduct := left.state.artifacts.products[leftMount.ProgramID]
	rightProduct := right.state.artifacts.products[rightMount.ProgramID]
	leftArtifact := leftProduct.Artifact
	rightArtifact := rightProduct.Artifact
	if leftArtifact == nil || leftArtifact != rightArtifact || leftProduct.Template == nil || leftProduct.Template != rightProduct.Template || leftProduct.Roles == nil || leftProduct.Roles != rightProduct.Roles {
		t.Fatal("equal Programs in independent Links did not share one content-addressed template")
	}
	leftBoundaryCount, leftBoundariesPublished := leftMount.Program.FunctionBoundaryCount()
	if !leftBoundariesPublished || leftBoundaryCount == 0 {
		t.Fatalf("shared Program function interfaces = %d/%v", leftBoundaryCount, leftBoundariesPublished)
	}
	if left.state.committed != nil || right.state.committed != nil || len(left.state.querySites) != 0 || len(right.state.querySites) != 0 {
		t.Fatal("Compile instantiated runtime topology before Solve ownership")
	}
	leftDiagnostic, leftStage, leftRule, leftInstantiated := left.state.instantiateRuntimeTopology()
	rightDiagnostic, rightStage, rightRule, rightInstantiated := right.state.instantiateRuntimeTopology()
	if !leftInstantiated || !rightInstantiated {
		t.Fatalf("runtime topology instantiation = %v/%+v/%v/%v %v/%+v/%v/%v", leftInstantiated, leftDiagnostic, leftStage, leftRule, rightInstantiated, rightDiagnostic, rightStage, rightRule)
	}
	leftGraph, rightGraph := left.state.committed, right.state.committed
	if _, _, _, replayed := left.state.instantiateRuntimeTopology(); !replayed || left.state.committed != leftGraph {
		t.Fatal("repeated Solve boundary rematerialized the left runtime topology")
	}
	if _, _, _, replayed := right.state.instantiateRuntimeTopology(); !replayed || right.state.committed != rightGraph {
		t.Fatal("repeated Solve boundary rematerialized the right runtime topology")
	}
	if left.state.committed == nil || right.state.committed == nil {
		t.Fatal("Solve boundary did not retain the runtime topology")
	}
	// The function declaration interface is owned by program/artifact. Both
	// Links resolve it through the one shared content-addressed Artifact, so
	// the interface outlives every Link-local release/replay.
	rightBoundaryCount, rightBoundariesPublished := rightMount.Program.FunctionBoundaryCount()
	if !rightBoundariesPublished || leftBoundaryCount != rightBoundaryCount {
		t.Fatal("independent Links did not share one function declaration authority")
	}
	programCaptures, programVarargs := 0, 0
	bodyCount, bodiesPublished := leftMount.Program.BodyCount()
	if !bodiesPublished {
		t.Fatal("shared cold Body family unavailable")
	}
	for index := 0; index < leftBoundaryCount; index++ {
		reusable, reusableOK := leftMount.Program.FunctionBoundaryAt(index)
		bodyOutcomes := 0
		for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
			body, bodyOK := leftMount.Program.BodyAt(bodyIndex)
			if bodyOK && body.ID() == reusable.BodyID() {
				bodyOutcomes = body.OutcomeCount()
				break
			}
		}
		if !reusableOK || !reusable.Available() || !reusable.ID().Available() || !reusable.BodyID().Available() || !reusable.EntryID().Available() || bodyOutcomes == 0 {
			t.Fatalf("function interface[%d] is incomplete", index)
		}
		for position := 0; position < reusable.FormalCount(); position++ {
			port, portOK := leftMount.Program.FunctionFormalFor(index, position)
			if !portOK || !port.ID().Available() || !port.CellID().Available() || !port.StorageCellID().Available() {
				t.Fatalf("function formal[%d,%d] is incomplete", index, position)
			}
		}
		if vararg, varargOK := leftMount.Program.FunctionVarargFor(index, 0); varargOK {
			if !vararg.ID().Available() || !vararg.CellID().Available() {
				t.Fatalf("function vararg[%d] is incomplete", index)
			}
			programVarargs++
		}
		for position := 0; position < reusable.CaptureCount(); position++ {
			capture, captureOK := leftMount.Program.FunctionCaptureFor(index, position)
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

func compileArtifactSet(t *testing.T, workspace *Workspace, linked *link.Link) *compiledArtifactSet {
	t.Helper()
	plan, status := workspace.Compile(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	t.Cleanup(func() { _ = plan.Close() })
	return plan.state.artifacts
}

func TestWorkspaceProductsReuseAcrossMountPermutation(t *testing.T) {
	workspace := NewWorkspace()
	t.Cleanup(func() { _ = workspace.Close() })
	leftProg := planLawProgram(t, `return 11`)
	rightProg := planLawProgram(t, `return 22`)
	first := compileArtifactSet(t, workspace, planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: leftProg}, {Name: "right", Program: rightProg},
	}))
	second := compileArtifactSet(t, workspace, planLawMountedLink(t, []linkproject.Module{
		{Name: "right", Program: rightProg}, {Name: "left", Program: leftProg},
	}))
	if first.products[leftProg.ContentID()].Artifact == nil || first.products[leftProg.ContentID()].Artifact != second.products[leftProg.ContentID()].Artifact ||
		first.products[rightProg.ContentID()].Artifact == nil || first.products[rightProg.ContentID()].Artifact != second.products[rightProg.ContentID()].Artifact {
		t.Fatal("mount permutation did not reuse Workspace products")
	}
}

func TestWorkspaceProductsInvalidateOnlyReplacedSibling(t *testing.T) {
	workspace := NewWorkspace()
	t.Cleanup(func() { _ = workspace.Close() })
	leftProg := planLawProgram(t, `return 11`)
	rightProg := planLawProgram(t, `return 22`)
	replacement := planLawProgram(t, `return 33`)
	base := compileArtifactSet(t, workspace, planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: leftProg}, {Name: "right", Program: rightProg},
	}))
	changed := compileArtifactSet(t, workspace, planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: replacement}, {Name: "right", Program: rightProg},
	}))
	if base.products[leftProg.ContentID()].Artifact == nil || changed.products[replacement.ContentID()].Artifact == nil ||
		base.products[leftProg.ContentID()].Artifact == changed.products[replacement.ContentID()].Artifact {
		t.Fatal("replaced sibling reused the previous artifact")
	}
	if base.products[rightProg.ContentID()].Artifact == nil || base.products[rightProg.ContentID()].Artifact != changed.products[rightProg.ContentID()].Artifact {
		t.Fatal("unrelated sibling lost its Workspace product")
	}
}

func TestWorkspaceProductsReuseAcrossActorRename(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	prog := planLawProgram(t, `return 7`)
	workspace := NewWorkspace()
	t.Cleanup(func() { _ = workspace.Close() })
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
	first := compileArtifactSet(t, workspace, seal("actor"))
	second := compileArtifactSet(t, workspace, seal("other-actor"))
	if first.products[prog.ContentID()].Artifact == nil || first.products[prog.ContentID()].Artifact != second.products[prog.ContentID()].Artifact {
		t.Fatal("actor rename did not reuse the Workspace product")
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
