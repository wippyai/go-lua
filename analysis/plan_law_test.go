package analysis

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
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
	contract, err := profile.Contract()
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
	receipt, ok := programschema.Global()
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
		_, failure := schemaadapter.CompileDetailed(mounted.TransformerInput(), receipt)
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

func planLawArtifactScheduleRow(linked *link.Link, ordinal uint32) string {
	if linked == nil || linked.Project() == nil {
		return "unavailable"
	}
	receipt, ok := programschema.Global()
	if !ok {
		return "schema-unavailable"
	}
	mounts := linked.Project().Mounts()
	remaining := uint64(ordinal)
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		mounted, mountedOK := mounts.Program(shard)
		if !shardOK || !mountedOK || mounted == nil {
			return fmt.Sprintf("mount=%d:unavailable", mountIndex)
		}
		artifact, failure := schemaadapter.CompileDetailed(mounted.TransformerInput(), receipt)
		if failure.Available() || artifact == nil {
			return fmt.Sprintf("mount=%d:%s", mountIndex, failure.Error())
		}
		rows := uint64(artifact.EnvironmentEdgeCount() + artifact.LocalTransferCount())
		if remaining >= rows {
			remaining -= rows
			continue
		}
		if remaining < uint64(artifact.EnvironmentEdgeCount()) {
			edge, edgeOK := artifact.EnvironmentEdgeAt(int(remaining))
			return fmt.Sprintf("mount=%d environment=%d ok=%t from=%x to=%x", mountIndex, remaining, edgeOK, edge.From(), edge.To())
		}
		localIndex := int(remaining) - artifact.EnvironmentEdgeCount()
		edge, edgeOK := artifact.LocalTransferAt(localIndex)
		ranks := make(map[keyspace.ContentID]int, artifact.PointCount())
		for eventIndex := 0; eventIndex < artifact.WTOEventCount(); eventIndex++ {
			event, eventOK := artifact.WTOEventAt(eventIndex)
			if eventOK && event.Kind() == programartifact.WTOEventPoint {
				ranks[event.PointID()] = len(ranks)
			}
		}
		base, stage := edge.From(), edge.To()
		detail := make([]string, 0, 16)
		for edgeIndex := 0; edgeIndex < artifact.EnvironmentEdgeCount(); edgeIndex++ {
			candidate, candidateOK := artifact.EnvironmentEdgeAt(edgeIndex)
			if candidateOK && (candidate.From() == base || candidate.To() == base || candidate.From() == stage || candidate.To() == stage) {
				detail = append(detail, fmt.Sprintf("env[%d]=%x->%x", edgeIndex, candidate.From(), candidate.To()))
			}
		}
		for role := programartifact.RuleRoleValueSource; role <= programartifact.RuleRoleValuePresenceRefinement; role++ {
			for ruleIndex := 0; ruleIndex < artifact.RuleOccurrenceCount(role); ruleIndex++ {
				rule, ruleOK := artifact.RuleOccurrenceAt(role, ruleIndex)
				point, pointOK := rule.PointAt(0)
				input, inputOK := rule.InputPoint()
				if ruleOK && pointOK && (point == base || point == stage || inputOK && (input == base || input == stage)) {
					detail = append(detail, fmt.Sprintf("rule[%d:%d]=%x->%x", role, ruleIndex, input, point))
				}
			}
		}
		return fmt.Sprintf("mount=%d local=%d ok=%t from=%x rank=%d to=%x rank=%d full=%t adjacent={%s}", mountIndex, localIndex, edgeOK, edge.From(), ranks[edge.From()], edge.To(), ranks[edge.To()], edge.FullEnvironment(), strings.Join(detail, ","))
	}
	return fmt.Sprintf("out-of-range=%d", remaining)
}

func planLawHeapSealFailure(linked *link.Link) string {
	if linked == nil || linked.Project() == nil {
		return "link-unavailable"
	}
	receipt, ok := programschema.Global()
	if !ok {
		return "schema-unavailable"
	}
	mounts := linked.Project().Mounts()
	rows := make([]heapdomain.ArtifactMount, mounts.Count())
	for index := range rows {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK {
			return fmt.Sprintf("mount=%d:unavailable", index)
		}
		artifact, failure := schemaadapter.CompileDetailed(mounted.TransformerInput(), receipt)
		if failure.Available() || artifact == nil {
			return fmt.Sprintf("mount=%d:artifact:%s", index, failure.Error())
		}
		programID := mounted.TransformerInput().ContentID()
		var mountOK bool
		rows[index], mountOK = heapdomain.NewArtifactMount(artifact, module, programID)
		if !mountOK {
			return fmt.Sprintf("mount=%d:receipt", index)
		}
	}
	_, failure := heapdomain.SealWithArtifacts(linked, rows)
	return failure.String()
}

func planLawValueSourceFailure(p *program.Program, family, row int) string {
	if p == nil || row < 0 {
		return "value-source-state"
	}
	input := p.TransformerInput()
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
	_, rootOK := input.RootSpan(term)
	anchor, anchorOK := input.ValueSourceAnchor(term)
	finish, finishOK := anchor.Finish()
	attachments := input.PointAttachments(finish)
	path, pathOK := p.Flow().ValueSourcePath(term)
	return fmt.Sprintf("value-source-family=%d row=%d row-ok=%v executable=%v body=%v containing=%v body-equal=%v direct-span=%v root-span=%v path=%v anchor=%v finish=%v attachments=%d",
		family, row, rowOK, p.Flow().Executable().Contains(term), bodyOK, containingOK,
		bodyOK && containingOK && body.Equal(containing), directOK, rootOK, pathOK && path.Available(), anchorOK, finishOK, attachments.Count())
}

func planLawRuleGeometryFailure(plan *Plan) string {
	if plan == nil || plan.state == nil || plan.state.artifacts == nil {
		return "artifact-unavailable"
	}
	for mountIndex, mount := range plan.state.artifacts.mounts {
		if mount.artifact == nil {
			return fmt.Sprintf("mount=%d artifact-unavailable", mountIndex)
		}
		for index := 0; index < mount.artifact.PointCount(); index++ {
			point, pointOK := mount.artifact.PointAt(index)
			if !pointOK || !point.Available() || !point.ID().Available() {
				return fmt.Sprintf("mount=%d point=%d available=%v id=%v", mountIndex, index, pointOK && point.Available(), point.ID().Available())
			}
			if _, initialOK := point.Initial(); !initialOK {
				return fmt.Sprintf("mount=%d point=%d initial-unavailable", mountIndex, index)
			}
			for decisionIndex := 0; decisionIndex < point.DecisionCount(); decisionIndex++ {
				decision, decisionOK := point.DecisionAt(decisionIndex)
				if !decisionOK || !decision.Available() {
					return fmt.Sprintf("mount=%d point=%d decision=%d available=%v", mountIndex, index, decisionIndex, decisionOK && decision.Available())
				}
			}
		}
		for index := 0; index < mount.artifact.EnvironmentEdgeCount(); index++ {
			edge, edgeOK := mount.artifact.EnvironmentEdgeAt(index)
			if !edgeOK || !edge.Available() {
				return fmt.Sprintf("mount=%d edge=%d available=%v", mountIndex, index, edgeOK && edge.Available())
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
		for index := 0; index < mount.artifact.RegionCount(); index++ {
			region, regionOK := mount.artifact.RegionAt(index)
			if !regionOK || !region.ID().Available() || !region.Head().Available() || region.MemberCount() == 0 {
				return fmt.Sprintf("mount=%d region=%d available=%v id=%v head=%v members=%d", mountIndex, index, regionOK, region.ID().Available(), region.Head().Available(), region.MemberCount())
			}
		}
		for index := 0; index < mount.artifact.WTOEventCount(); index++ {
			event, eventOK := mount.artifact.WTOEventAt(index)
			if !eventOK || !event.Available() {
				return fmt.Sprintf("mount=%d event=%d available=%v", mountIndex, index, eventOK && event.Available())
			}
		}
		for role := programartifact.RuleRoleValueSource; role <= programartifact.RuleRoleValuePresenceRefinement; role++ {
			if !mount.artifact.RuleRoleSupported(role) {
				continue
			}
			for index := 0; index < mount.artifact.RuleOccurrenceCount(role); index++ {
				row, ok := mount.artifact.RuleOccurrenceAt(role, index)
				if !ok || row.PointCount() == 0 {
					return fmt.Sprintf("mount=%d role=%d row=%d available=%v points=%d", mountIndex, role, index, ok, row.PointCount())
				}
			}
		}
	}
	return "rule-geometry-complete"
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
	if len(artifacts.mounts) != 2 || len(artifacts.byProgram) != 1 ||
		artifacts.mounts[0].artifact == nil || artifacts.mounts[0].artifact != artifacts.mounts[1].artifact ||
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
	stableID := stable.TransformerInput().ContentID()
	changedID := changed.TransformerInput().ContentID()
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
		var diagnostics AnalyzeDiagnostics
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
	wantKinds := map[programartifact.OccurrenceKind]bool{
		programartifact.OccurrenceStorageRead:         false,
		programartifact.OccurrenceStorageBindTransfer: false,
		programartifact.OccurrenceStorageWrite:        false,
	}
	for mountIndex, mount := range plan.state.artifacts.mounts {
		artifact := mount.artifact
		if artifact == nil || !artifact.Available() {
			return fmt.Sprintf("mount=%d artifact", mountIndex)
		}
		for occurrenceIndex := 0; occurrenceIndex < artifact.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := artifact.OccurrenceAt(occurrenceIndex)
			if !occurrenceOK {
				return fmt.Sprintf("mount=%d occurrence=%d", mountIndex, occurrenceIndex)
			}
			kind := occurrence.Kind()
			if _, required := wantKinds[kind]; !required {
				continue
			}
			wantKinds[kind] = true
			matches := 0
			for ruleIndex := 0; ruleIndex < artifact.RuleOccurrenceCount(programartifact.RuleRoleValueStorageTransfer); ruleIndex++ {
				rule, ruleOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleValueStorageTransfer, ruleIndex)
				if !ruleOK || rule.ID() != occurrence.ID() {
					continue
				}
				matches++
				point, pointOK := rule.PointAt(0)
				input, inputOK := rule.InputPoint()
				if !pointOK || !inputOK || rule.Stage() != programartifact.RuleStageLocal || point == input {
					return fmt.Sprintf("mount=%d kind=%d rule=%d local", mountIndex, kind, ruleIndex)
				}
				switch kind {
				case programartifact.OccurrenceStorageRead, programartifact.OccurrenceStorageBindTransfer:
					if rule.InputKind() != programartifact.RuleInputEntry {
						return fmt.Sprintf("mount=%d kind=%d rule=%d entry", mountIndex, kind, ruleIndex)
					}
				case programartifact.OccurrenceStorageWrite:
					route, routeOK := rule.PredecessorRouteID()
					if rule.InputKind() != programartifact.RuleInputPredecessor || !routeOK || !route.Available() {
						return fmt.Sprintf("mount=%d kind=%d rule=%d predecessor", mountIndex, kind, ruleIndex)
					}
				}
				localFromOccurrence := false
				for edgeIndex := 0; edgeIndex < artifact.LocalTransferCount(); edgeIndex++ {
					edge, edgeOK := artifact.LocalTransferAt(edgeIndex)
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
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.binding == nil || plan.state.binding.binding == nil || !plan.state.binding.binding.Sealed() {
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
	if plan.state.binding != binding || plan.state.binding.binding != binding.binding || !binding.binding.Sealed() {
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

func TestMountedReceiptPathCompletesEngineSolve(t *testing.T) {
	plan, status := Compile(planLawLink(t))
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	result, solveStatus, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if result == nil || solveStatus != AnalyzeComplete || result.BodyCount() == 0 {
		t.Fatalf("receipt solve = %v/%v", solveStatus, result)
	}
	if diagnostics.Phase != AnalyzeDiagnosticPhaseComplete || diagnostics.Reason != AnalyzeDiagnosticReasonNone {
		t.Fatalf("receipt solve diagnostics = phase:%v reason:%v", diagnostics.Phase, diagnostics.Reason)
	}
	if plan.state.ordinary != nil {
		t.Fatal("diagnostic solve populated the ordinary solver cache")
	}
}
