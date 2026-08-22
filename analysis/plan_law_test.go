package analysis

import (
	"context"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func planLawLink(t testing.TB) *link.Link {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	return mustLink(t, `return 1`, contract)
}

func planLawProgram(t testing.TB, text string) *program.Program {
	t.Helper()
	compiled, err := lower.Lower(lower.Source{Name: "plan-law.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func planLawMountedLink(t testing.TB, modules []linkproject.Module) *link.Link {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}

func TestCompiledPlanDuplicateMountsReuseArtifactAndFreshenResults(t *testing.T) {
	shared := planLawProgram(t, `return 1`)
	linked := planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: shared},
		{Name: "right", Program: shared},
	})
	plan, status := Compile(linked)
	if plan != nil {
		defer plan.Close()
	}
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("duplicate-mount compile = %v/%v", status, plan)
	}
	artifacts := plan.state.artifacts
	sharedProduct := artifacts.products[shared.ContentID()]
	if len(artifacts.mounts) != 2 || len(artifacts.products) != 1 ||
		sharedProduct.Artifact == nil || artifacts.mounts[0].Snapshot != artifacts.mounts[1].Snapshot ||
		artifacts.mounts[0].ModuleKey == artifacts.mounts[1].ModuleKey {
		t.Fatal("duplicate mounts did not reuse one Program artifact with distinct Link substitutions")
	}
	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || result == nil || result.BodyCount() != 2 {
		t.Fatalf("duplicate-mount solve = %v/%v bodies=%d", solveStatus, result, func() int {
			if result == nil {
				return 0
			}
			return result.BodyCount()
		}())
	}
	left, leftOK := result.BodyAt(0)
	right, rightOK := result.BodyAt(1)
	leftID, leftIDOK := left.ID()
	rightID, rightIDOK := right.ID()
	if !leftOK || !rightOK || !leftIDOK || !rightIDOK || leftID == rightID {
		t.Fatal("duplicate mounts collapsed their concrete result Bodies")
	}
}

func TestCompiledMountsCarrySealedSnapshot(t *testing.T) {
	shared := planLawProgram(t, `return 1`)
	plan, status := Compile(planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: shared},
		{Name: "right", Program: shared},
	}))
	if plan != nil {
		defer plan.Close()
	}
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	mounts := plan.state.artifacts.mounts
	if len(mounts) != 2 {
		t.Fatalf("mounts = %d", len(mounts))
	}
	for index, mount := range mounts {
		if !mount.Available() {
			t.Fatalf("mount %d is not valid", index)
		}
		if mount.Snapshot == nil || !mount.Snapshot.Available() {
			t.Fatalf("mount %d carries no sealed snapshot", index)
		}
		product := plan.state.artifacts.products[mount.ProgramID]
		artifact := product.Artifact
		if artifact == nil || !artifact.Available() ||
			mount.Snapshot.ArtifactID() != artifact.ID() ||
			mount.Snapshot.ProgramID() != mount.ProgramID ||
			mount.Snapshot.SchemaID() != artifact.CompileKey().ExecutionSchemaID().ContentID() {
			t.Fatalf("mount %d snapshot identities do not match the compile product", index)
		}
	}
	if mounts[0].Snapshot != mounts[1].Snapshot {
		t.Fatal("duplicate Program mounts did not share the compile-time snapshot")
	}
}

func TestCompiledPlanProgramChangeInvalidatesOnlyChangedArtifact(t *testing.T) {
	stable := planLawProgram(t, `return 1`)
	changed := planLawProgram(t, `return 2`)
	workspace := NewWorkspace()
	defer workspace.Close()
	first, firstStatus := workspace.Compile(planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: stable},
		{Name: "right", Program: stable},
	}))
	if first != nil {
		defer first.Close()
	}
	second, secondStatus := workspace.Compile(planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: stable},
		{Name: "right", Program: changed},
	}))
	if second != nil {
		defer second.Close()
	}
	if firstStatus != CompileComplete || secondStatus != CompileComplete || first == nil || second == nil ||
		first.state == nil || second.state == nil || first.state.artifacts == nil || second.state.artifacts == nil {
		t.Fatal("incremental artifact fixtures did not compile")
	}
	stableID := stable.ContentID()
	changedID := changed.ContentID()
	firstStable := first.state.artifacts.products[stableID]
	secondStable := second.state.artifacts.products[stableID]
	secondChanged := second.state.artifacts.products[changedID]
	if !stableID.Available() || !changedID.Available() || stableID == changedID || firstStable.Artifact == nil ||
		firstStable.Artifact != secondStable.Artifact || secondChanged.Artifact == nil || secondChanged.Artifact == secondStable.Artifact {
		t.Fatal("Program artifact cache failed stable reuse or changed-Program invalidation")
	}
	if result, status := second.Solve(context.Background()); status != AnalyzeComplete || result == nil || result.BodyCount() != 2 {
		t.Fatalf("changed-mount solve = %v/%v", status, result)
	}
}

// The profile supplies several outcome exits for one executable body. A
// literal return reaches only its return path; the other attached exits are
// valid zero-row observations and must not mask the reachable Value row.
func TestCompiledPlanZeroRowOutcomeDoesNotMaskLiteralReturn(t *testing.T) {
	linked := planLawLink(t)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	availableExits := 0
	for _, outcome := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if exit, exitOK := program.Flow().Outcomes().BodyExit(body, outcome); exitOK && exit != 0 {
			availableExits++
		}
	}
	if !shardOK || !programOK || availableExits < 2 {
		t.Fatalf("literal body exits = %d, want multiple available exits", availableExits)
	}
	analysisResult, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("literal zero-row outcome solve = %v/%v", status, analysisResult)
	}
	row, rowOK := analysisResult.BodyAt(0)
	bodyID, bodyIDOK := row.ID()
	if !rowOK || !bodyIDOK || !bodyID.Available() {
		t.Fatal("literal return lost its detached body identity")
	}
	values := linked.Boundary().Values()
	canonical := make([]identity.ContentID, values.Count())
	for index := range canonical {
		value, valueOK := values.At(index)
		valueID, valueIDOK := values.ID(value)
		if !valueOK || !valueIDOK || !valueID.Available() {
			t.Fatalf("canonical ingress Value %d", index)
		}
		canonical[index] = valueID
	}
	publication, publicationOK := analysisResult.FamilyByKey(value.SummaryResultFamily)
	if !publicationOK {
		t.Fatal("literal return has no unique typed value publication")
	}
	seen := make(map[identity.ContentID]bool, len(canonical))
	found := false
	for queryIndex := 0; queryIndex < publication.QueryCount(); queryIndex++ {
		query, queryOK := publication.QueryAt(queryIndex)
		if !queryOK || query.Status() != result.QueryHit {
			continue
		}
		reachesBody := false
		for bodyIndex := 0; bodyIndex < query.BodyCount(); bodyIndex++ {
			candidate, candidateOK := query.BodyAt(bodyIndex)
			candidateID, candidateIDOK := candidate.ID()
			if candidateOK && candidateIDOK && candidateID == bodyID {
				reachesBody = true
				break
			}
		}
		if !reachesBody {
			continue
		}
		cell, cellOK := query.Cell()
		view, refusal := plane.Admit(value.SummaryResultLayout, cell.Present(), cell.RowCount(), cell.Payload())
		if !cellOK || refusal.Available() {
			t.Fatalf("literal return hit has no typed value summary: %s", refusal)
		}
		for index := 0; index < view.RowCount(); index++ {
			row, rowOK := view.At(index)
			if !rowOK || !row.ID().Available() {
				t.Fatal("literal return summary coordinate")
			}
			seen[row.ID()] = true
		}
		found = true
		break
	}
	if !found {
		t.Fatal("literal return has no hit value summary reaching its body")
	}
	for _, valueID := range canonical {
		if !seen[valueID] {
			t.Fatalf("canonical ingress Value %s is absent from the typed Result summary", valueID)
		}
	}
}

func TestCompiledPlanRepeatedSolveLaw(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.binding == nil || plan.state.binding.SchemaBinding() == nil || !plan.state.binding.SchemaBinding().Sealed() {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	defer plan.Close()
	binding := plan.state.binding
	if plan.state.ordinary != nil {
		t.Fatal("Compile constructed the ordinary runtime solver")
	}
	first, firstStatus := plan.Solve(context.Background())
	ordinary := plan.state.ordinary
	if ordinary == nil || !plan.state.ordinaryOK {
		t.Fatal("first ordinary solve did not retain its runtime solver")
	}
	second, secondStatus := plan.Solve(context.Background())
	if firstStatus != AnalyzeComplete || secondStatus != AnalyzeComplete || first == nil || second == nil {
		t.Fatalf("repeated solve = %v/%v", firstStatus, secondStatus)
	}
	if first.SourceID() != second.SourceID() || first.ContentID() != second.ContentID() || first.BodyCount() != second.BodyCount() {
		t.Fatal("repeated plan solve changed detached projection")
	}
	if plan.state.binding != binding || plan.state.binding.SchemaBinding() != binding.SchemaBinding() || !binding.SchemaBinding().Sealed() {
		t.Fatal("repeated plan solve rebuilt or mutated the sealed Link binding")
	}
	if plan.state.ordinary != ordinary {
		t.Fatal("repeated plan solve rebuilt the ordinary runtime solver")
	}
}

func TestCompiledPlanConcurrentSolveLaw(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	defer plan.Close()
	const count = 2
	results := make([]*result.Result, count)
	statuses := make([]AnalyzeStatus, count)
	var wait sync.WaitGroup
	wait.Add(count)
	for index := 0; index < count; index++ {
		go func(index int) {
			defer wait.Done()
			results[index], statuses[index] = plan.Solve(context.Background())
		}(index)
	}
	wait.Wait()
	for index := range results {
		if statuses[index] != AnalyzeComplete || results[index] == nil {
			t.Fatalf("concurrent solve[%d] = %v/%v", index, statuses[index], results[index])
		}
		if results[index].SourceID() != plan.SourceID() {
			t.Fatalf("concurrent solve[%d] source fence changed", index)
		}
	}
	if results[0].ContentID() != results[1].ContentID() {
		t.Fatal("concurrent solves changed detached projection")
	}
	if plan.state.ordinary == nil || !plan.state.ordinaryOK {
		t.Fatal("concurrent solves did not join one ordinary runtime solver")
	}
}

func TestCompiledPlanCancellationDoesNotPoisonReuse(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	defer plan.Close()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, canceledStatus := plan.Solve(canceled)
	if result != nil || canceledStatus != AnalyzeIncomplete {
		t.Fatalf("canceled solve = %v/%v", canceledStatus, result)
	}
	ordinary := plan.state.ordinary
	if ordinary == nil || !plan.state.ordinaryOK {
		t.Fatal("canceled first solve discarded the immutable ordinary solver")
	}
	reused, reusedStatus := plan.Solve(context.Background())
	if reusedStatus != AnalyzeComplete || reused == nil {
		t.Fatalf("reuse after cancellation = %v/%v", reusedStatus, reused)
	}
	if plan.state.ordinary != ordinary {
		t.Fatal("reuse after cancellation rebuilt the ordinary solver")
	}
}
