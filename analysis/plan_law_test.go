package analysis

import (
	"context"
	"sync"
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/target/profile"
)

func planLawLink(t testing.TB) *link.Link {
	t.Helper()
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	return mustLink(t, `return 1`, contract)
}

func TestCompiledPlanAnalyzeParity(t *testing.T) {
	linked := planLawLink(t)
	plan, compileStatus := Compile(linked)
	if compileStatus != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", compileStatus, plan)
	}
	solved, solveStatus := plan.Solve(context.Background())
	direct, directStatus := Analyze(context.Background(), linked)
	if solveStatus != AnalyzeComplete || directStatus != AnalyzeComplete || solved == nil || direct == nil {
		t.Fatalf("solve/direct = %v/%v", solveStatus, directStatus)
	}
	if solved.SourceID() != direct.SourceID() || solved.ContentID() != direct.ContentID() || solved.BodyCount() != direct.BodyCount() {
		t.Fatalf("solve/direct projection mismatch")
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
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil {
		t.Fatalf("literal zero-row outcome solve = %v/%v", status, result)
	}
	row, rowOK := result.BodyAt(0)
	if !rowOK || row.ValueCount() == 0 {
		t.Fatal("literal return lost its detached Value identity")
	}
	for index := 0; index < row.ValueCount(); index++ {
		valueID, _, valueOK := row.ValueAt(index)
		if !valueOK {
			t.Fatal("literal return Value row")
		}
		if !valueID.Available() {
			t.Fatal("literal return Value identity")
		}
	}
}

// One fixed storage transfer may carry several symbolic rows when the source
// coordinate was populated under distinct branch guards. The Rule occurrence
// is singular; its derivation dispositions are the exact guarded partition
// and every row must pass the same source/destination/value proof.
func TestCompiledPlanAcceptsGuardPartitionedStorageTransfer(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `
local function move(flag: boolean)
    local source: number | string
    if flag then
        source = 1
    else
        source = "x"
    end
    local destination = source
    return destination
end
return move
	`, contract)
	result, status := Analyze(context.Background(), linked)
	bodyCount := 0
	if result != nil {
		bodyCount = result.BodyCount()
	}
	if status != AnalyzeComplete || result == nil || bodyCount < 2 {
		t.Fatalf("guard-partitioned storage transfer = status:%v result:%v bodies:%d", status, result != nil, bodyCount)
	}
}

func TestCompiledPlanRepeatedSolveLaw(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	first, firstStatus := plan.Solve(context.Background())
	second, secondStatus := plan.Solve(context.Background())
	if firstStatus != AnalyzeComplete || secondStatus != AnalyzeComplete || first == nil || second == nil {
		t.Fatalf("repeated solve = %v/%v", firstStatus, secondStatus)
	}
	if first.SourceID() != second.SourceID() || first.ContentID() != second.ContentID() || first.BodyCount() != second.BodyCount() {
		t.Fatal("repeated plan solve changed detached projection")
	}
}

func TestCompiledPlanConcurrentSolveLaw(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	const count = 2
	results := make([]*Result, count)
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
}

func TestCompiledPlanCancellationDoesNotPoisonReuse(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, canceledStatus := plan.Solve(canceled)
	if result != nil || canceledStatus != AnalyzeIncomplete {
		t.Fatalf("canceled solve = %v/%v", canceledStatus, result)
	}
	reused, reusedStatus := plan.Solve(context.Background())
	if reusedStatus != AnalyzeComplete || reused == nil {
		t.Fatalf("reuse after cancellation = %v/%v", reusedStatus, reused)
	}
}
