package analysis

import (
	"context"
	"fmt"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/result"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/domain/composite"
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

func planLawArtifactFailure(linked *link.Link) string {
	if linked == nil || linked.Project() == nil {
		return "link-unavailable"
	}
	receipt, ok := composite.Global()
	if !ok {
		return "schema-unavailable"
	}
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		if !shardOK || !mountedOK || mounted == nil {
			return fmt.Sprintf("mount=%d:unavailable", index)
		}
		_, failure := composite.CompileArtifactDetailed(mounted, receipt)
		if failure.Available() {
			detail := ""
			if failure.Reason() == programartifact.CompileReasonOccurrenceValueSourceProof {
				row, rowOK := failure.Row()
				family, familyOK := failure.Subrow()
				if rowOK && familyOK {
					detail = ":" + planLawValueSourceFailure(mounted, family, row)
				}
			}
			return fmt.Sprintf("mount=%d:%s%s", index, failure.Error(), detail)
		}
	}
	if _, ok := compileValueCoordinates(linked); !ok {
		return "value-coordinates"
	}
	return "program artifacts compile succeeded"
}

func planLawValueSourceFailure(p *program.Program, family, row int) string {
	if p == nil || row < 0 {
		return "value-source-state"
	}
	input := p
	var term, owner keyspace.Term
	var rowOK bool
	switch family {
	case 1:
		term, owner, rowOK = p.Source().Literals().Nils().At(row)
	case 2:
		term, owner, _, rowOK = p.Source().Literals().Bools().At(row)
	case 3:
		term, owner, _, rowOK = p.Source().Literals().Integers().At(row)
	case 4:
		term, owner, _, rowOK = p.Source().Literals().Floats().At(row)
	case 5:
		term, owner, _, rowOK = p.Source().Literals().Strings().At(row)
	default:
		return fmt.Sprintf("value-source-family=%d", family)
	}
	body, bodyOK := input.Body(owner)
	containing, containingOK := input.ContainingBody(term)
	_, directOK := input.Span(term)
	root, rootExists := p.Source().Index().Root(term)
	rootOK := false
	if rootExists {
		_, rootOK = input.Span(root)
	}
	sourceID, sourceSpan, sourceTerm, anchorOK := p.ValueSourceIDAt(keyspace.Family(family), row)
	finishOK := sourceSpan.Available() && sourceTerm == term
	path, pathOK := p.Flow().ValueSourcePath(term)
	return fmt.Sprintf("value-source-family=%d row=%d row-ok=%v executable=%v body=%v containing=%v body-equal=%v direct-span=%v root-span=%v path=%v anchor=%v finish=%v",
		family, row, rowOK, p.Flow().Executable().Contains(term), bodyOK, containingOK,
		bodyOK && containingOK && body.Equal(containing), directOK, rootOK, pathOK && path.Available(), anchorOK && sourceID.Available(), finishOK)
}

func planLawRuleGeometryFailure(plan *Plan) string {
	if plan == nil || plan.state == nil || plan.state.artifacts == nil {
		return "artifact-unavailable"
	}
	for mountIndex, mount := range plan.state.artifacts.mounts {
		if !mount.valid() {
			return fmt.Sprintf("mount=%d snapshot-unavailable", mountIndex)
		}
		snapshot := mount.snapshot
		for index := 0; index < snapshot.PointCount(); index++ {
			point, pointOK := snapshot.PointAt(index)
			if !pointOK || !point.ID().Available() {
				return fmt.Sprintf("mount=%d point=%d available=%v id=%v", mountIndex, index, pointOK, point.ID().Available())
			}
			for decisionIndex := 0; decisionIndex < point.DecisionCount(); decisionIndex++ {
				decision, decisionOK := point.DecisionAt(decisionIndex)
				if !decisionOK || !decision.Available() {
					return fmt.Sprintf("mount=%d point=%d decision=%d available=%v", mountIndex, index, decisionIndex, decisionOK && decision.Available())
				}
			}
		}
		for index := 0; index < snapshot.StructuralEdgeCount(); index++ {
			edge, edgeOK := snapshot.StructuralEdgeAt(index)
			if !edgeOK || !edge.ID().Available() {
				return fmt.Sprintf("mount=%d edge=%d available=%v", mountIndex, index, edgeOK && edge.ID().Available())
			}
			guard, guarded := edge.GuardID()
			truth, truthOK := edge.Truth()
			decision, decisionOK := edge.DecisionID()
			reset, hasReset := edge.ResetDigest()
			mu, hasMu := edge.MuPathID()
			if guarded != truthOK || guarded != decisionOK || hasMu != hasReset || hasReset != reset.Available() || guarded != guard.Available() {
				return fmt.Sprintf("mount=%d edge=%d proof guard=%v/%v decision=%v/%v truth=%v/%v reset=%v/%v mu=%v/%v", mountIndex, index, guard.Available(), guarded, decision.Available(), decisionOK, truth, truthOK, reset.Available(), hasReset, mu.Available(), hasMu)
			}
		}
		for index := 0; index < snapshot.RegionCount(); index++ {
			region, regionOK := snapshot.RegionAt(index)
			if !regionOK || !region.ID().Available() || !region.Head().Available() || region.MemberCount() == 0 {
				return fmt.Sprintf("mount=%d region=%d available=%v id=%v head=%v members=%d", mountIndex, index, regionOK, region.ID().Available(), region.Head().Available(), region.MemberCount())
			}
		}
		for index := 0; index < snapshot.EventCount(); index++ {
			event, eventOK := snapshot.EventAt(index)
			if !eventOK || !event.PointID().Available() {
				return fmt.Sprintf("mount=%d event=%d available=%v", mountIndex, index, eventOK && event.PointID().Available())
			}
		}
		for index := 0; index < snapshot.RulePlacementCount(); index++ {
			row, ok := snapshot.RulePlacementAt(index)
			if !ok || !row.PointID().Available() || !row.Key().Available() {
				return fmt.Sprintf("mount=%d placement=%d available=%v point=%v key=%q", mountIndex, index, ok, row.PointID().Available(), row.Key())
			}
		}
	}
	return "rule-geometry-complete"
}

func TestCompiledPlanDuplicateMountsReuseArtifactAndFreshenResults(t *testing.T) {
	shared := planLawProgram(t, `return 1`)
	linked := planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: shared},
		{Name: "right", Program: shared},
	})
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("duplicate-mount compile = %v/%v", status, plan)
	}
	artifacts := plan.state.artifacts
	sharedArtifact := artifacts.byProgram[shared.ContentID()]
	if len(artifacts.mounts) != 2 || len(artifacts.byProgram) != 1 ||
		sharedArtifact == nil || artifacts.mounts[0].snapshot != artifacts.mounts[1].snapshot ||
		artifacts.mounts[0].moduleKey == artifacts.mounts[1].moduleKey {
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
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	mounts := plan.state.artifacts.mounts
	if len(mounts) != 2 {
		t.Fatalf("mounts = %d", len(mounts))
	}
	for index, mount := range mounts {
		if !mount.valid() {
			t.Fatalf("mount %d is not valid", index)
		}
		if mount.snapshot == nil || !mount.snapshot.Available() {
			t.Fatalf("mount %d carries no sealed snapshot", index)
		}
		artifact := plan.state.artifacts.byProgram[mount.programID]
		if artifact == nil || !artifact.Available() ||
			mount.snapshot.ArtifactID() != artifact.ID() ||
			mount.snapshot.ProgramID() != mount.programID ||
			mount.snapshot.SchemaID() != artifact.CompileKey().SchemaDigest() {
			t.Fatalf("mount %d snapshot identities do not match the compile product", index)
		}
	}
	if mounts[0].snapshot != mounts[1].snapshot {
		t.Fatal("duplicate Program mounts did not share the compile-time snapshot")
	}
	sealed, sealedOK := linkArtifactRows(mounts)
	if !sealedOK || len(sealed) != 2 {
		t.Fatalf("linkArtifactRows = %v/%d", sealedOK, len(sealed))
	}
	if sealed[0].Snapshot != mounts[0].snapshot || sealed[1].Snapshot != mounts[1].snapshot {
		t.Fatal("linkArtifactRows minted a new snapshot instead of projecting the compile-time view")
	}
}

func TestCompiledPlanProgramChangeInvalidatesOnlyChangedArtifact(t *testing.T) {
	stable := planLawProgram(t, `return 1`)
	changed := planLawProgram(t, `return 2`)
	first, firstStatus := Compile(planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: stable},
		{Name: "right", Program: stable},
	}))
	second, secondStatus := Compile(planLawMountedLink(t, []linkproject.Module{
		{Name: "left", Program: stable},
		{Name: "right", Program: changed},
	}))
	if firstStatus != CompileComplete || secondStatus != CompileComplete || first == nil || second == nil ||
		first.state == nil || second.state == nil || first.state.artifacts == nil || second.state.artifacts == nil {
		t.Fatal("incremental artifact fixtures did not compile")
	}
	stableID := stable.ContentID()
	changedID := changed.ContentID()
	firstStable := first.state.artifacts.byProgram[stableID]
	secondStable := second.state.artifacts.byProgram[stableID]
	secondChanged := second.state.artifacts.byProgram[changedID]
	if !stableID.Available() || !changedID.Available() || stableID == changedID || firstStable == nil ||
		firstStable != secondStable || secondChanged == nil || secondChanged == secondStable {
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
	contract, err := testfixture.StandardLibraryTarget()
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
		var diagnostics anadiag.AnalyzeDiagnostics
		geometry := "plan-unavailable"
		if plan, compileStatus := Compile(linked); compileStatus == CompileComplete && plan != nil {
			_, _, diagnostics = plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
			geometry = planLawRuleGeometryFailure(plan)
		}
		t.Fatalf("guard-partitioned storage transfer = status:%v result:%v bodies:%d artifact:%s geometry:%s diagnostics:%+v", status, result != nil, bodyCount, planLawArtifactFailure(linked), geometry, diagnostics)
	}
	plan, compileStatus := Compile(linked)
	if compileStatus != CompileComplete || plan == nil {
		t.Fatalf("guard-partitioned storage transfer placement compile = %v/%v", compileStatus, plan)
	}
	if failure := planLawStorageTransferPlacementFailure(plan); failure != "" {
		t.Fatalf("guard-partitioned storage transfer placement: %s", failure)
	}
}

// planLawStorageTransferPlacementFailure verifies the sole artifact-native
// placement authority for fixed storage transfers.  Reads and binds must keep
// their Program-issued Entry input; writes must retain the exact parent route
// predecessor.  All execute at the separate Local point after Finish.
func planLawStorageTransferPlacementFailure(plan *Plan) string {
	if plan == nil || plan.state == nil || plan.state.artifacts == nil {
		return "plan"
	}
	wantKinds := map[uint8]bool{
		uint8(programartifact.OccurrenceStorageRead):         false,
		uint8(programartifact.OccurrenceStorageBindTransfer): false,
		uint8(programartifact.OccurrenceStorageWrite):        false,
	}
	for mountIndex, mount := range plan.state.artifacts.mounts {
		if !mount.valid() {
			return fmt.Sprintf("mount=%d snapshot", mountIndex)
		}
		snapshot := mount.snapshot
		for occurrenceIndex := 0; occurrenceIndex < snapshot.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := snapshot.OccurrenceAt(occurrenceIndex)
			if !occurrenceOK {
				return fmt.Sprintf("mount=%d occurrence=%d", mountIndex, occurrenceIndex)
			}
			kind := occurrence.Kind()
			if _, required := wantKinds[kind]; !required {
				continue
			}
			wantKinds[kind] = true
			matches := 0
			for ruleIndex := 0; ruleIndex < snapshot.RulePlacementCount(); ruleIndex++ {
				rule, ruleOK := snapshot.RulePlacementAt(ruleIndex)
				if !ruleOK || rule.Key() != "value-transfer" || rule.OccurrenceID() != occurrence.ID() {
					continue
				}
				matches++
				point := rule.PointID()
				input := rule.InputPointID()
				if !point.Available() || !input.Available() || rule.Stage() != uint8(programartifact.RuleStageLocal) || point == input {
					return fmt.Sprintf("mount=%d kind=%d rule=%d local", mountIndex, kind, ruleIndex)
				}
				switch kind {
				case uint8(programartifact.OccurrenceStorageRead), uint8(programartifact.OccurrenceStorageBindTransfer):
					if rule.InputKind() != uint8(programartifact.RuleInputEntry) {
						return fmt.Sprintf("mount=%d kind=%d rule=%d entry", mountIndex, kind, ruleIndex)
					}
				case uint8(programartifact.OccurrenceStorageWrite):
					route := rule.PredecessorRouteID()
					if rule.InputKind() != uint8(programartifact.RuleInputPredecessor) || !route.Available() {
						return fmt.Sprintf("mount=%d kind=%d rule=%d predecessor", mountIndex, kind, ruleIndex)
					}
				}
				localFromOccurrence := false
				for edgeIndex := 0; edgeIndex < snapshot.LocalTransferCount(); edgeIndex++ {
					edge, edgeOK := snapshot.LocalTransferAt(edgeIndex)
					if !edgeOK || edge.To() != point {
						continue
					}
					for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
						parent, parentOK := occurrence.PointAt(pointIndex)
						if parentOK && parent == edge.From() {
							localFromOccurrence = true
						}
					}
				}
				if !localFromOccurrence {
					return fmt.Sprintf("mount=%d kind=%d rule=%d local-parent", mountIndex, kind, ruleIndex)
				}
			}
			if matches == 0 {
				return fmt.Sprintf("mount=%d kind=%d transfers=%d", mountIndex, kind, matches)
			}
		}
	}
	for kind, found := range wantKinds {
		if !found {
			return fmt.Sprintf("missing kind=%d", kind)
		}
	}
	return ""
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

func TestDeadlockDataflowNodeCompilesProgramArtifact(t *testing.T) {
	linked := fixtureLink(t, "regression/deadlock-dataflow-node")
	if got := planLawArtifactFailure(linked); got != "program artifacts compile succeeded" {
		t.Fatal(got)
	}
	receipt, ok := composite.Global()
	if !ok {
		t.Fatal("schema-unavailable")
	}
	artifacts, artifactsOK := compileProgramArtifacts(linked, receipt)
	if !artifactsOK || artifacts == nil || len(artifacts.mounts) == 0 {
		t.Fatal("program artifacts and scalar template")
	}
}
