package analysis

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
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
	contract, err := profile.Contract()
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
	compileKey, keyOK := composite.NewArtifactCompileKey(mounted, receipt)
	if !keyOK || !compileKey.Available() {
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
	contract, err := profile.Contract()
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
	contract, err := profile.Contract()
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
	leftKey, leftKeyOK := composite.NewArtifactCompileKey(leftInput, receipt)
	rightKey, rightKeyOK := composite.NewArtifactCompileKey(rightInput, receipt)
	if !receiptOK || !leftKeyOK || !rightKeyOK || leftKey.ID() == rightKey.ID() {
		t.Fatal("changed Program did not issue a distinct full compiler key")
	}
	leftArtifact := leftPlan.state.artifacts.byProgram[leftInput.ContentID()]
	rightArtifact := rightPlan.state.artifacts.byProgram[rightInput.ContentID()]
	if leftArtifact == nil || rightArtifact == nil || leftArtifact == rightArtifact || leftPlan.state.artifacts.mounts[0].template == nil || rightPlan.state.artifacts.mounts[0].template == nil || leftPlan.state.artifacts.mounts[0].template == rightPlan.state.artifacts.mounts[0].template {
		t.Fatal("changed full compiler key aliased one immutable artifact/template")
	}
}

func TestArtifactTemplateReusesAcrossIndependentEqualProgramsAndLinks(t *testing.T) {
	contract, err := profile.Contract()
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
	if leftMount.artifact == nil || leftMount.artifact != rightMount.artifact || leftMount.template == nil || leftMount.template != rightMount.template || leftMount.roles == nil || leftMount.roles != rightMount.roles {
		t.Fatal("equal Programs in independent Links did not share one content-addressed template")
	}
	if leftMount.artifact.FunctionBoundaryCount() == 0 || leftMount.template.FunctionCount() != leftMount.artifact.FunctionBoundaryCount() {
		t.Fatalf("shared template function interfaces = %d, artifact %d", leftMount.template.FunctionCount(), leftMount.artifact.FunctionBoundaryCount())
	}
	if left.state.graph != nil || right.state.graph != nil || left.state.queryPlan != nil || right.state.queryPlan != nil {
		t.Fatal("Compile instantiated runtime topology before Solve ownership")
	}
	leftDiagnostic, leftInstantiated := left.state.instantiateRuntimeTopology()
	rightDiagnostic, rightInstantiated := right.state.instantiateRuntimeTopology()
	if !leftInstantiated || !rightInstantiated {
		t.Fatalf("runtime topology instantiation = %v/%+v %v/%+v", leftInstantiated, leftDiagnostic, rightInstantiated, rightDiagnostic)
	}
	leftGraph, rightGraph := left.state.graph, right.state.graph
	if _, replayed := left.state.instantiateRuntimeTopology(); !replayed || left.state.graph != leftGraph {
		t.Fatal("repeated Solve boundary rematerialized the left runtime topology")
	}
	if _, replayed := right.state.instantiateRuntimeTopology(); !replayed || right.state.graph != rightGraph {
		t.Fatal("repeated Solve boundary rematerialized the right runtime topology")
	}
	if left.state.graph == nil || right.state.graph == nil || left.state.graph.MountedFunctionCount() != leftMount.template.FunctionCount() || right.state.graph.MountedFunctionCount() != rightMount.template.FunctionCount() {
		t.Fatal("released graph did not retain the compact mounted function directory")
	}
	mountedCaptures, mountedVarargs := 0, 0
	for index := 0; index < left.state.graph.MountedFunctionCount(); index++ {
		leftFunction, leftOK := left.state.graph.MountedFunctionAt(index)
		rightFunction, rightOK := right.state.graph.MountedFunctionAt(index)
		reusable, reusableOK := leftMount.artifact.FunctionBoundaryAt(index)
		if !leftOK || !rightOK || !reusableOK || leftFunction.ReusableID() != reusable.ID() || rightFunction.ReusableID() != reusable.ID() ||
			leftFunction.BodyID() != reusable.BodyID() || leftFunction.EntryID() != reusable.EntryID() || leftFunction.FormalCount() != reusable.FormalCount() || leftFunction.OutcomeCount() != reusable.OutcomeCount() {
			t.Fatalf("mounted function interface[%d] did not survive release/replay", index)
		}
		for position := 0; position < leftFunction.FormalCount(); position++ {
			leftPort, leftPortOK := leftFunction.FormalAt(position)
			rightPort, rightPortOK := rightFunction.FormalAt(position)
			programPort, programPortOK := reusable.FormalAt(position)
			if !leftPortOK || !rightPortOK || !programPortOK || leftPort.ID() != programPort.ID() || rightPort.ID() != programPort.ID() ||
				leftPort.CellID() != programPort.CellID() || leftPort.StorageCellID() != programPort.StorageCellID() {
				t.Fatalf("mounted function formal[%d,%d] lost reusable identity", index, position)
			}
		}
		leftVararg, leftVarargOK := leftFunction.Vararg()
		rightVararg, rightVarargOK := rightFunction.Vararg()
		programVararg, programVarargOK := reusable.Vararg()
		if leftVarargOK != programVarargOK || rightVarargOK != programVarargOK || programVarargOK &&
			(leftVararg.ID() != programVararg.ID() || rightVararg.ID() != programVararg.ID() || leftVararg.CellID() != programVararg.CellID()) {
			t.Fatalf("mounted function vararg[%d] lost reusable identity", index)
		}
		if programVarargOK {
			mountedVarargs++
		}
		if leftFunction.CaptureCount() != reusable.CaptureCount() || rightFunction.CaptureCount() != reusable.CaptureCount() {
			t.Fatalf("mounted function capture count[%d] diverged", index)
		}
		for position := 0; position < leftFunction.CaptureCount(); position++ {
			leftCapture, leftCaptureOK := leftFunction.CaptureAt(position)
			rightCapture, rightCaptureOK := rightFunction.CaptureAt(position)
			programCapture, programCaptureOK := reusable.CaptureAt(position)
			if !leftCaptureOK || !rightCaptureOK || !programCaptureOK || leftCapture.ID() != programCapture.ID() || rightCapture.ID() != programCapture.ID() ||
				leftCapture.InnerCellID() != programCapture.InnerCellID() || leftCapture.OuterCellID() != programCapture.OuterCellID() ||
				leftCapture.InnerBodyID() != programCapture.InnerBodyID() || leftCapture.OuterBodyID() != programCapture.OuterBodyID() {
				t.Fatalf("mounted function capture[%d,%d] lost reusable direction", index, position)
			}
			mountedCaptures++
		}
	}
	if mountedCaptures == 0 || mountedVarargs == 0 {
		t.Fatalf("mounted interface fixture missed capture/vararg rows: %d/%d", mountedCaptures, mountedVarargs)
	}
	if left.state.binding == nil || right.state.binding == nil || left.state.binding == right.state.binding {
		t.Fatal("independent Links aliased the Link-local binding")
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

func TestCompiledStateDirectFieldsExcludeLegacyOwners(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	path := filepath.Join(filepath.Dir(current), "analyze.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		typeDeclaration, ok := declaration.(*ast.GenDecl)
		if !ok || typeDeclaration.Tok != token.TYPE {
			continue
		}
		for _, specification := range typeDeclaration.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "compiledState" {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatal("compiledState is not a struct")
			}
			for _, field := range structure.Fields.List {
				fieldSource := source[field.Pos()-1 : field.End()-1]
				for _, forbidden := range []string{"link.Link", "linkproject.", "program.Program", "flow."} {
					if strings.Contains(string(fieldSource), forbidden) {
						t.Fatalf("compiledState retains forbidden owner field %q", fieldSource)
					}
				}
			}
			return
		}
	}
	t.Fatal("compiledState declaration unavailable")
}

func TestPublishedArtifactMountTypesExcludeProjectCoordinates(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	path := filepath.Join(filepath.Dir(current), "artifact_plan.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"mountedProgramArtifact": false, "compiledArtifactSet": false}
	for _, declaration := range parsed.Decls {
		typeDeclaration, ok := declaration.(*ast.GenDecl)
		if !ok || typeDeclaration.Tok != token.TYPE {
			continue
		}
		for _, specification := range typeDeclaration.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, tracked := want[typeSpec.Name.Name]; !tracked {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s is not a struct", typeSpec.Name.Name)
			}
			want[typeSpec.Name.Name] = true
			for _, field := range structure.Fields.List {
				fieldSource := string(source[field.Pos()-1 : field.End()-1])
				for _, forbidden := range []string{"link.", "linkproject.", "program.", "flow."} {
					if strings.Contains(fieldSource, forbidden) {
						t.Fatalf("published %s retains construction owner field %q", typeSpec.Name.Name, fieldSource)
					}
				}
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("published mount declaration unavailable: %s", name)
		}
	}
}

// The public Plan holds compiledState by pointer, so a retained legacy owner
// anywhere in this static type graph is a published reachability edge even
// when a particular fixture leaves that field nil. Construction may receive a
// Link; the assembled Plan must contain only scalar IDs, artifacts, receipts,
// and domain-owned detached capabilities.
func TestCompiledPlanTypeGraphExcludesLegacyOwners(t *testing.T) {
	forbidden := map[string]struct{}{
		"github.com/wippyai/go-lua/analysis/program/link.Link":                    {},
		"github.com/wippyai/go-lua/analysis/program/link/project.Application":     {},
		"github.com/wippyai/go-lua/analysis/program/link/project.CallApplication": {},
		"github.com/wippyai/go-lua/analysis/program/link/project.Component":       {},
		"github.com/wippyai/go-lua/analysis/program/link/project.Key":             {},
		"github.com/wippyai/go-lua/analysis/program/link/project.Shard":           {},
		"github.com/wippyai/go-lua/analysis/program/link/boundary.Component":      {},
		"github.com/wippyai/go-lua/analysis/program/link/boundary.Value":          {},
		"github.com/wippyai/go-lua/analysis/program/link/boundary.Values":         {},
		"github.com/wippyai/go-lua/analysis/program/link/boundary.Seed":           {},
		"github.com/wippyai/go-lua/analysis/program/link/boundary.Endpoint":       {},
		"github.com/wippyai/go-lua/analysis/program/link/host.Component":          {},
		"github.com/wippyai/go-lua/analysis/program/link/host.GlobalBinding":      {},
		"github.com/wippyai/go-lua/analysis/program/link/host.BootRoot":           {},
		"github.com/wippyai/go-lua/analysis/program/link/host.ProviderCapability": {},
		"github.com/wippyai/go-lua/analysis/program/link/static.Component":        {},
		"github.com/wippyai/go-lua/analysis/program/link/static.Namespace":        {},
		"github.com/wippyai/go-lua/analysis/program/link/static.Resolver":         {},
		"github.com/wippyai/go-lua/analysis/program/link/static.InputRef":         {},
		"github.com/wippyai/go-lua/analysis/program/link/static.Expression":       {},
		"github.com/wippyai/go-lua/analysis/program/link/module.Component":        {},
		"github.com/wippyai/go-lua/analysis/program.Program":                      {},
		"github.com/wippyai/go-lua/analysis/program/flow.Component":               {},
	}
	seen := make(map[reflect.Type]struct{})
	var visit func(reflect.Type, []string)
	visit = func(current reflect.Type, path []string) {
		if current == nil {
			return
		}
		for current.Kind() == reflect.Pointer || current.Kind() == reflect.Slice || current.Kind() == reflect.Array {
			current = current.Elem()
		}
		if current.Kind() == reflect.Map {
			visit(current.Key(), append(path, "key"))
			visit(current.Elem(), append(path, "value"))
			return
		}
		if _, visited := seen[current]; visited {
			return
		}
		seen[current] = struct{}{}
		identity := current.PkgPath() + "." + current.Name()
		if _, prohibited := forbidden[identity]; prohibited {
			t.Fatalf("published Plan type graph retains %s via %s", identity, strings.Join(path, "."))
		}
		if current.Kind() != reflect.Struct {
			return
		}
		for index := 0; index < current.NumField(); index++ {
			field := current.Field(index)
			visit(field.Type, append(path, field.Name))
		}
	}
	visit(reflect.TypeOf(compiledState{}), []string{"compiledState"})
}
