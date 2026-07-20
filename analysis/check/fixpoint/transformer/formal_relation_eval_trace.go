package transformer

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// formalRelationEvalTrace is diagnostic authority for one execution. It is
// absent unless GOLUA_TRACE_FORMAL_EQUATIONS names a positive slow-equation
// threshold (for example "500ms"), so production evaluation has no timer,
// counter, formatting, or allocation cost.
type formalRelationEvalTrace struct {
	threshold time.Duration
	sequence  uint64
	active    *formalRelationEvalTraceDetail
}

type formalRelationEvalTraceDetail struct {
	rootAssignmentCurrentRoots, rootAssignmentPointRoots       int
	rootAssignmentWriteRoots, rootAssignmentRegions            int
	rootAssignmentLeafWrites                                   int
	rootAssignmentLeafTime                                     time.Duration
	rootAssignmentStageTime                                    [10]time.Duration
	rootAssignmentCurrentSupport, rootAssignmentPointSupport   []uint32
	outcomeRegions, outcomeWrites                              int
	outcomeReadRoots, outcomeNonterminalRoots                  int
	outcomeDistinctRoots, outcomeDistinctTopVariables          int
	outcomeSupportNodes, outcomeSupportVariables               int
	outcomeSupportRanks                                        []uint32
	outcomeSupportOrdinals                                     map[uint32][]formalFiberOrdinal
	outcomePlan                                                *formalOutcomeStep
	definitionCalls, definitionCallerRoots                     int
	definitionTargetRoots, definitionRows                      int
	definitionCapabilityCount                                  int
	definitionSupportRanks                                     []uint32
	definitionPartitionApplyOps                                uint64
	definitionCapabilityApplyOps                               uint64
	definitionPartitionTime                                    time.Duration
	definitionCapabilityTime                                   time.Duration
	definitionEquationCalls                                    int
	definitionInputs, definitionLiveOutcomes                   int
	definitionRead, definitionSeedJoin                         formalRelationEvalTracePhase
	definitionSeedValidate, definitionTargetValidate           formalRelationEvalTracePhase
	definitionCompose, definitionTargetJoin                    formalRelationEvalTracePhase
	definitionCorrelation, definitionCorrelationSetup          formalRelationEvalTracePhase
	definitionExecute, definitionPublish                       formalRelationEvalTracePhase
	guardComposeRead, guardComposeSubstitute                   formalRelationEvalTracePhase
	guardComposeGroups, guardComposeClose                      formalRelationEvalTracePhase
	guardComposeJoin, guardComposeGroupPartition               formalRelationEvalTracePhase
	guardComposeGroupLeaves, guardComposeScalarJoin            formalRelationEvalTracePhase
	guardComposeValidate, guardComposePublish                  formalRelationEvalTracePhase
	guardComposeCloseStates, guardComposeCloseJoins            int
	guardComposeGroupRegions                                   int
	branchRelationsPlan                                        *formalBranchRelationsStep
	branchRelationFactors                                      []formalBranchRelationEvalTraceFactor
	externalCallPlan                                           *formalExternalCallStep
	externalCallProviderInputs, externalCallProviderRoots      int
	externalCallProviderRegions, externalCallProviderEvals     int
	externalCallCommitRoots, externalCallCommitRegions         int
	externalCallPublicationConditions, externalCallDeltaWrites int
	externalCallProviderSupport, externalCallOutcomeSupport    []uint32
	externalCallCommitSupport                                  []uint32
	externalCallInput, externalCallProvider                    formalRelationEvalTracePhase
	externalCallProviderOutcome                                formalRelationEvalTracePhase
	externalCallCommitPartition, externalCallOuter             formalRelationEvalTracePhase
	externalCallNormal, externalCallCorrelation                formalRelationEvalTracePhase
	externalCallDiagnostics, externalCallLedger                formalRelationEvalTracePhase
	externalCallPublication                                    formalRelationEvalTracePhase
}

type formalBranchRelationEvalTraceFactor struct {
	factor, source                          int
	consequence                             bool
	currentRoots, originalRoots, writeRoots int
	currentSupport, originalSupport         []uint32
	regions, leafWrites                     int
	total, partition                        formalRelationEvalTracePhase
	leafTime                                time.Duration
	leafApplyOps                            uint64
}

type formalRelationEvalTracePhase struct {
	count    int
	applyOps uint64
	elapsed  time.Duration
	mallocs  uint64
	bytes    uint64
}

type formalRelationEvalTracePhaseMark struct {
	started  time.Time
	applyOps uint64
	mallocs  uint64
	bytes    uint64
}

func beginFormalRelationEvalTracePhase(algebra *formalTupleAlgebra) formalRelationEvalTracePhaseMark {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	mark := formalRelationEvalTracePhaseMark{started: time.Now(), mallocs: memory.Mallocs, bytes: memory.TotalAlloc}
	if algebra != nil {
		mark.applyOps = algebra.decisions.applyOps
	}
	return mark
}

func finishFormalRelationEvalTracePhase(algebra *formalTupleAlgebra, phase *formalRelationEvalTracePhase, mark formalRelationEvalTracePhaseMark) {
	if phase == nil {
		return
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	phase.count++
	phase.elapsed += time.Since(mark.started)
	phase.mallocs += memory.Mallocs - mark.mallocs
	phase.bytes += memory.TotalAlloc - mark.bytes
	if algebra != nil {
		phase.applyOps += algebra.decisions.applyOps - mark.applyOps
	}
}

type formalRelationEvalTraceSnapshot struct {
	decisionNodes, decisionTerminals, decisionUnique int
	decisionApply, decisionITE, decisionCare         int
	decisionCareApply                                int
	decisionApplyOps                                 uint64
	componentTerminals, directoryNodes               int
	mallocs, bytes                                   uint64
}

func newFormalRelationEvalTrace() *formalRelationEvalTrace {
	raw := strings.TrimSpace(os.Getenv("GOLUA_TRACE_FORMAL_EQUATIONS"))
	if raw == "" {
		return nil
	}
	threshold, err := time.ParseDuration(raw)
	if err != nil || threshold <= 0 {
		fmt.Fprintf(os.Stderr, "FORMAL_EQUATION_TRACE_CONFIG value=%s error=%v\n", strconv.Quote(raw), err)
		return nil
	}
	return &formalRelationEvalTrace{threshold: threshold}
}

func snapshotFormalRelationEvaluation(algebra *formalTupleAlgebra) formalRelationEvalTraceSnapshot {
	if algebra == nil {
		return formalRelationEvalTraceSnapshot{}
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	snapshot := formalRelationEvalTraceSnapshot{
		decisionNodes:      len(algebra.decisions.nodes),
		decisionTerminals:  len(algebra.decisions.terminals),
		decisionUnique:     len(algebra.decisions.unique),
		decisionApply:      len(algebra.decisions.applyMemo),
		decisionITE:        len(algebra.decisions.iteMemo),
		decisionCare:       len(algebra.decisions.careMemo),
		decisionCareApply:  len(algebra.decisions.careApplyMemo),
		decisionApplyOps:   algebra.decisions.applyOps,
		componentTerminals: len(algebra.components.terminals),
		mallocs:            memory.Mallocs,
		bytes:              memory.TotalAlloc,
	}
	for _, directory := range algebra.directories {
		if directory != nil {
			snapshot.directoryNodes += len(directory.nodes)
		}
	}
	return snapshot
}

func (t *formalRelationEvalTrace) evaluate(
	algebra *formalTupleAlgebra,
	equation formalRelationEquation,
	read func(formalRelationCell) formalRelationTuple,
) formalRelationTuple {
	sequence := atomic.AddUint64(&t.sequence, 1)
	started := time.Now()
	before := snapshotFormalRelationEvaluation(algebra)
	detail := &formalRelationEvalTraceDetail{}
	t.active = detail
	done := make(chan struct{})
	timer := time.AfterFunc(t.threshold, func() {
		select {
		case <-done:
			return
		default:
		}
		fmt.Fprintf(os.Stderr, "FORMAL_EQUATION_ACTIVE seq=%d elapsed=%s %s\n", sequence, time.Since(started).Round(time.Millisecond), formatFormalRelationEquationTrace(algebra, equation))
	})
	result := evaluateFormalRelationEquation(algebra, equation, read)
	t.active = nil
	close(done)
	timer.Stop()
	elapsed := time.Since(started)
	if elapsed < t.threshold {
		return result
	}
	after := snapshotFormalRelationEvaluation(algebra)
	fmt.Fprintf(os.Stderr,
		"FORMAL_EQUATION_SLOW seq=%d elapsed=%s %s dd_nodes=%+d dd_terminals=%+d dd_unique=%+d dd_apply_memo=%+d dd_ite_memo=%+d dd_care_memo=%+d dd_care_apply_memo=%+d dd_apply_ops=%+d component_terminals=%+d directory_nodes=%+d mallocs=%+d bytes=%+d outcome_read_roots=%d outcome_nonterminal_roots=%d outcome_distinct_roots=%d outcome_distinct_top_variables=%d outcome_support_nodes=%d outcome_support_variables=%d outcome_regions=%d outcome_writes=%d\n",
		sequence, elapsed.Round(time.Millisecond), formatFormalRelationEquationTrace(algebra, equation),
		after.decisionNodes-before.decisionNodes,
		after.decisionTerminals-before.decisionTerminals,
		after.decisionUnique-before.decisionUnique,
		after.decisionApply-before.decisionApply,
		after.decisionITE-before.decisionITE,
		after.decisionCare-before.decisionCare,
		after.decisionCareApply-before.decisionCareApply,
		after.decisionApplyOps-before.decisionApplyOps,
		after.componentTerminals-before.componentTerminals,
		after.directoryNodes-before.directoryNodes,
		after.mallocs-before.mallocs,
		after.bytes-before.bytes,
		detail.outcomeReadRoots,
		detail.outcomeNonterminalRoots,
		detail.outcomeDistinctRoots,
		detail.outcomeDistinctTopVariables,
		detail.outcomeSupportNodes,
		detail.outcomeSupportVariables,
		detail.outcomeRegions,
		detail.outcomeWrites,
	)
	if detail.rootAssignmentRegions != 0 {
		fmt.Fprintf(os.Stderr,
			"FORMAL_ROOT_ASSIGNMENT seq=%d current_roots=%d point_roots=%d write_roots=%d current_support=%v point_support=%v regions=%d leaf_writes=%d leaf_time=%s stage_time=%v\n",
			sequence, detail.rootAssignmentCurrentRoots, detail.rootAssignmentPointRoots,
			detail.rootAssignmentWriteRoots, detail.rootAssignmentCurrentSupport,
			detail.rootAssignmentPointSupport, detail.rootAssignmentRegions,
			detail.rootAssignmentLeafWrites, detail.rootAssignmentLeafTime.Round(time.Microsecond),
			detail.rootAssignmentStageTime,
		)
	}
	if detail.outcomePlan != nil {
		fmt.Fprintf(os.Stderr, "FORMAL_OUTCOME_PLAN seq=%d %s\n", sequence, formatFormalOutcomePlanTrace(detail.outcomePlan))
		for _, rank := range detail.outcomeSupportRanks {
			fmt.Fprintf(os.Stderr, "FORMAL_OUTCOME_SUPPORT seq=%d rank=%d atom=%s components=%s\n", sequence, rank,
				formatFormalGuardRankTrace(algebra, rank), formatFormalOutcomeOrdinalsTrace(algebra, equation, detail.outcomeSupportOrdinals[rank]))
		}
	}
	if detail.definitionCalls != 0 {
		fmt.Fprintf(os.Stderr,
			"FORMAL_DEFINITION_CORRELATION seq=%d calls=%d caller_roots=%d target_roots=%d support_ranks=%v rows=%d partition_apply_ops=%d partition_time=%s capability_count=%d capability_apply_ops=%d capability_time=%s\n",
			sequence,
			detail.definitionCalls,
			detail.definitionCallerRoots,
			detail.definitionTargetRoots,
			detail.definitionSupportRanks,
			detail.definitionRows,
			detail.definitionPartitionApplyOps,
			detail.definitionPartitionTime.Round(time.Microsecond),
			detail.definitionCapabilityCount,
			detail.definitionCapabilityApplyOps,
			detail.definitionCapabilityTime.Round(time.Microsecond),
		)
	}
	if detail.externalCallPlan != nil {
		fmt.Fprintf(os.Stderr,
			"FORMAL_EXTERNAL_CALL seq=%d point=%d provider_inputs=%d provider_roots=%d provider_support=%v provider_regions=%d provider_evals=%d outcome_support=%v commit_roots=%d commit_support=%v commit_regions=%d publication_conditions=%d delta_writes=%d input=%s provider=%s provider_outcome=%s commit_partition=%s outer=%s normal=%s correlation=%s diagnostics=%s ledger=%s publication=%s\n",
			sequence, detail.externalCallPlan.point,
			detail.externalCallProviderInputs, detail.externalCallProviderRoots,
			detail.externalCallProviderSupport, detail.externalCallProviderRegions,
			detail.externalCallProviderEvals, detail.externalCallOutcomeSupport,
			detail.externalCallCommitRoots, detail.externalCallCommitSupport,
			detail.externalCallCommitRegions, detail.externalCallPublicationConditions,
			detail.externalCallDeltaWrites,
			formatFormalRelationEvalTracePhase(detail.externalCallInput),
			formatFormalRelationEvalTracePhase(detail.externalCallProvider),
			formatFormalRelationEvalTracePhase(detail.externalCallProviderOutcome),
			formatFormalRelationEvalTracePhase(detail.externalCallCommitPartition),
			formatFormalRelationEvalTracePhase(detail.externalCallOuter),
			formatFormalRelationEvalTracePhase(detail.externalCallNormal),
			formatFormalRelationEvalTracePhase(detail.externalCallCorrelation),
			formatFormalRelationEvalTracePhase(detail.externalCallDiagnostics),
			formatFormalRelationEvalTracePhase(detail.externalCallLedger),
			formatFormalRelationEvalTracePhase(detail.externalCallPublication),
		)
	}
	if detail.definitionEquationCalls != 0 {
		fmt.Fprintf(os.Stderr,
			"FORMAL_DEFINITION_PHASES seq=%d equations=%d inputs=%d live_outcomes=%d read=%s seed_join=%s seed_validate=%s target_validate=%s compose=%s target_join=%s correlation=%s correlation_setup=%s execute=%s publish=%s\n",
			sequence,
			detail.definitionEquationCalls,
			detail.definitionInputs,
			detail.definitionLiveOutcomes,
			formatFormalRelationEvalTracePhase(detail.definitionRead),
			formatFormalRelationEvalTracePhase(detail.definitionSeedJoin),
			formatFormalRelationEvalTracePhase(detail.definitionSeedValidate),
			formatFormalRelationEvalTracePhase(detail.definitionTargetValidate),
			formatFormalRelationEvalTracePhase(detail.definitionCompose),
			formatFormalRelationEvalTracePhase(detail.definitionTargetJoin),
			formatFormalRelationEvalTracePhase(detail.definitionCorrelation),
			formatFormalRelationEvalTracePhase(detail.definitionCorrelationSetup),
			formatFormalRelationEvalTracePhase(detail.definitionExecute),
			formatFormalRelationEvalTracePhase(detail.definitionPublish),
		)
	}
	if detail.guardComposeClose.count != 0 {
		fmt.Fprintf(os.Stderr,
			"FORMAL_GUARD_COMPOSE seq=%d read=%s substitute=%s groups=%s close=%s join=%s group_partition=%s group_leaves=%s scalar_join=%s validate=%s publish=%s close_states=%d close_joins=%d group_regions=%d\n",
			sequence,
			formatFormalRelationEvalTracePhase(detail.guardComposeRead),
			formatFormalRelationEvalTracePhase(detail.guardComposeSubstitute),
			formatFormalRelationEvalTracePhase(detail.guardComposeGroups),
			formatFormalRelationEvalTracePhase(detail.guardComposeClose),
			formatFormalRelationEvalTracePhase(detail.guardComposeJoin),
			formatFormalRelationEvalTracePhase(detail.guardComposeGroupPartition),
			formatFormalRelationEvalTracePhase(detail.guardComposeGroupLeaves),
			formatFormalRelationEvalTracePhase(detail.guardComposeScalarJoin),
			formatFormalRelationEvalTracePhase(detail.guardComposeValidate),
			formatFormalRelationEvalTracePhase(detail.guardComposePublish),
			detail.guardComposeCloseStates,
			detail.guardComposeCloseJoins,
			detail.guardComposeGroupRegions,
		)
	}
	if detail.branchRelationsPlan != nil {
		fmt.Fprintf(os.Stderr, "FORMAL_BRANCH_RELATIONS seq=%d stages=%d factors=%d\n",
			sequence, len(detail.branchRelationsPlan.stages), len(detail.branchRelationFactors))
		for _, factor := range detail.branchRelationFactors {
			fmt.Fprintf(os.Stderr,
				"FORMAL_BRANCH_FACTOR seq=%d factor=%d source=%d consequence=%t current_roots=%d original_roots=%d write_roots=%d current_support=%v original_support=%v regions=%d leaf_writes=%d total=%s partition=%s leaf_time=%s leaf_apply_ops=%d\n",
				sequence, factor.factor, factor.source, factor.consequence,
				factor.currentRoots, factor.originalRoots, factor.writeRoots,
				factor.currentSupport, factor.originalSupport, factor.regions, factor.leafWrites,
				formatFormalRelationEvalTracePhase(factor.total), formatFormalRelationEvalTracePhase(factor.partition),
				factor.leafTime.Round(time.Microsecond), factor.leafApplyOps,
			)
		}
	}
	return result
}

func formatFormalRelationEvalTracePhase(phase formalRelationEvalTracePhase) string {
	return fmt.Sprintf("%d/%s/%d/%d/%d", phase.count, phase.elapsed.Round(time.Microsecond), phase.applyOps, phase.mallocs, phase.bytes)
}

// formalRelationTraceSupportRanks returns the exact ordered decision-variable
// support of roots. It is called only while the opt-in equation trace is
// active; ordinary evaluation pays neither its traversal nor its allocations.
func formalRelationTraceSupportRanks(kernel *decisionKernel, roots ...decisionRef) []uint32 {
	if kernel == nil || len(roots) == 0 {
		return nil
	}
	seen := make(map[decisionRef]struct{}, len(roots))
	ranks := make(map[uint32]struct{})
	stack := append([]decisionRef(nil), roots...)
	for len(stack) != 0 {
		last := len(stack) - 1
		root := stack[last]
		stack = stack[:last]
		if _, present := seen[root]; present || int(root) >= len(kernel.nodes) {
			continue
		}
		seen[root] = struct{}{}
		node := kernel.nodes[root]
		if node.terminal {
			continue
		}
		ranks[node.variable] = struct{}{}
		stack = append(stack, node.low, node.high)
	}
	ordered := make([]uint32, 0, len(ranks))
	for rank := range ranks {
		ordered = append(ordered, rank)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered
}

func formatFormalOutcomePlanTrace(plan *formalOutcomeStep) string {
	bindings := make([]string, 0, plan.transaction.ResultBindingCount())
	for index := 0; index < plan.transaction.ResultBindingCount(); index++ {
		source, target, ok := plan.transaction.ResultBinding(index)
		projects, projectsOK := plan.transaction.ResultBindingProjectsHeap(index)
		bindings = append(bindings, fmt.Sprintf("%d>%d:%t:%t:%t", source, target, ok, projectsOK, projects))
	}
	reads, writes, components := 0, 0, 0
	for _, lift := range []formalClosedFactorLift{plan.bindingLift, plan.presenceLift, plan.covariantLift} {
		if !lift.sealed {
			continue
		}
		components++
		for _, role := range lift.roles {
			reads += len(role.reads)
		}
		writes += len(lift.writes)
	}
	return fmt.Sprintf("reads=%d writes=%d components=%d value_groups=%d result_bindings=%v targets=%v covariant_steps=%d covariant_bindings=%d covariant_topology=%d",
		reads, writes, components, len(plan.valueFactorGroups), bindings, plan.targets,
		plan.covariant.Len(), len(plan.covariantBindings), plan.covariantTopology.Len())
}

func formatFormalGuardRankTrace(algebra *formalTupleAlgebra, rank uint32) string {
	if algebra != nil && algebra.program != nil && algebra.program.formalGuards != nil {
		for key, candidate := range algebra.program.formalGuards.ranks {
			if candidate == rank {
				return fmt.Sprintf("%q/var=%d/scope=%d/root=%d/step=%d/definition=%d", key.arena.canonicalValue(key.term), key.variable, key.scope, key.root, key.step, key.definition)
			}
		}
	}
	return "unknown"
}

func formatFormalOutcomeOrdinalsTrace(algebra *formalTupleAlgebra, equation formalRelationEquation, ordinals []formalFiberOrdinal) string {
	span, _, _, ok := algebra.span(equation.Cell.cell.Variable)
	if !ok {
		return "[]"
	}
	parts := make([]string, 0, len(ordinals))
	for _, ordinal := range ordinals {
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		parts = append(parts, fmt.Sprintf("%d/r%d/l%s/f%s", ordinal, descriptor.role, descriptor.lane.ID(), descriptor.family.ID()))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func formatFormalRelationEquationTrace(algebra *formalTupleAlgebra, equation formalRelationEquation) string {
	cell := equation.Cell.cell
	body := ""
	if algebra != nil && algebra.program != nil && cell.Variable != 0 && int(cell.Variable) <= len(algebra.program.bodies) {
		body = algebra.program.bodies[cell.Variable-1].body.String()
	}
	boundary, point := boundaryStepInvalid, 0
	if step, ok := formalRelationStepOperator(equation.Operator); ok {
		boundary, point = step.kind, int(step.point)
	}
	shape := ""
	if plan := equation.Operator.outcomeTransaction; plan != nil {
		reads, writes, components := 0, 0, 0
		for _, lift := range []formalClosedFactorLift{plan.bindingLift, plan.presenceLift, plan.covariantLift} {
			if !lift.sealed {
				continue
			}
			components++
			for _, role := range lift.roles {
				reads += len(role.reads)
			}
			writes += len(lift.writes)
		}
		shape = fmt.Sprintf(" outcome_reads=%d outcome_writes=%d outcome_demands=%d outcome_sources=%d outcome_components=%d",
			reads, writes, len(plan.demands), len(plan.sources), components)
	}
	return fmt.Sprintf("body=%s cell=%+v capability=%d boundary=%d point=%d inputs=%d seeds=%d nonreturning=%d%s",
		body, cell, equation.Operator.stepCapability, boundary, point, len(equation.Inputs), len(equation.Seeds), len(equation.ApplyNonreturning), shape)
}
