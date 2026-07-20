package transformer

import (
	"fmt"
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// structuralEnvironment is compiler scratch for one CFG transfer. It is not a
// lattice value or a control alternative: all dataflow lives in the World
// State read/written by the emitted circuit.
type structuralEnvironment struct {
	values            map[symbol.ID]ValueTerm
	resultRoots       map[ResultRoot]ValueTerm
	operations        []Operation
	steps             []rowStep
	proofs            []BranchProofTerm
	observations      []ObservationTerm
	obligations       []observationObligation
	output            structuralOutputContribution
	rootAssignment    rootAssignmentTerm
	returnTransaction returnTransactionTerm
	genericBindings   map[symbol.ID]symbolicGenericBinding
	preserved         paramPreservationLedger
	callResidual      Guard
	callResultSymbols map[symbol.ID]struct{}
}

type structuralOutputContribution struct {
	maySuspend       bool
	externalAccess   []valueAccessTerm
	externalOperands callOutcomeOperandTerms
	externalSealed   bool
	returnConditions []returnConditionParamRefinementTerm
	paramObligations []callpayload.CallParamObligation
	paramExposures   []boundaryParamExposureTerm
	memberCalls      []boundaryMemberCallDiagnosticTerm
}

type structuralExpressionEdge struct {
	from cfg.Point
	to   cfg.Point
}

type structuralExpressionWrite struct {
	ref    factflow.ExprRef
	slot   statekey.Value
	source factflow.ValueSource
}

func (o structuralOutputContribution) clone() structuralOutputContribution {
	o.externalAccess = cloneValueAccessTerms(o.externalAccess)
	o.externalOperands = o.externalOperands.clone()
	o.returnConditions = append([]returnConditionParamRefinementTerm(nil), o.returnConditions...)
	o.paramObligations = cloneBoundaryParamObligations(o.paramObligations)
	o.paramExposures = cloneBoundaryParamExposures(o.paramExposures)
	o.memberCalls = cloneBoundaryMemberCallDiagnostics(o.memberCalls)
	return o
}

func (e structuralEnvironment) clone() structuralEnvironment {
	e.values = cloneValueBindings(e.values)
	e.resultRoots = cloneResultRootBindings(e.resultRoots)
	e.operations = append([]Operation(nil), e.operations...)
	e.steps = cloneRowSteps(e.steps)
	e.proofs = append([]BranchProofTerm(nil), e.proofs...)
	e.observations = append([]ObservationTerm(nil), e.observations...)
	e.obligations = append([]observationObligation(nil), e.obligations...)
	e.output = e.output.clone()
	e.genericBindings = cloneGenericBindings(e.genericBindings)
	e.preserved = e.preserved.clone()
	e.callResultSymbols = cloneSymbolSet(e.callResultSymbols)
	return e
}

func cloneSymbolSet(in map[symbol.ID]struct{}) map[symbol.ID]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[symbol.ID]struct{}, len(in))
	for id := range in {
		out[id] = struct{}{}
	}
	return out
}

func (e structuralEnvironment) edgeClone() structuralEnvironment {
	out := e.clone()
	out.operations = nil
	out.steps = nil
	out.proofs = nil
	out.observations = nil
	out.obligations = nil
	out.output = structuralOutputContribution{}
	out.rootAssignment = rootAssignmentTerm{}
	out.returnTransaction = returnTransactionTerm{}
	out.preserved = paramPreservationLedger{}
	return out
}

func cloneValueBindings(in map[symbol.ID]ValueTerm) map[symbol.ID]ValueTerm {
	out := make(map[symbol.ID]ValueTerm, len(in))
	for id, value := range in {
		out[id] = value
	}
	return out
}
func cloneResultRootBindings(in map[ResultRoot]ValueTerm) map[ResultRoot]ValueTerm {
	out := make(map[ResultRoot]ValueTerm, len(in))
	for root, value := range in {
		out[root] = value
	}
	return out
}
func cloneGenericBindings(in map[symbol.ID]symbolicGenericBinding) map[symbol.ID]symbolicGenericBinding {
	out := make(map[symbol.ID]symbolicGenericBinding, len(in))
	for id, value := range in {
		value.Identity = value.Identity.clone()
		out[id] = value
	}
	return out
}

func (p *PreparedPlanCompiler) freezeStructuralWorldProgramSurface(direct frozenLexicalCallSurface) (WorldProgram, error) {
	if p == nil || p.builder == nil || p.graph == nil || p.wtoTape == nil {
		return WorldProgram{}, fmt.Errorf("structural freezer requires prepared CFG ownership")
	}
	if err := p.sealDirectCallEnvironment(direct); err != nil {
		return WorldProgram{}, err
	}
	topologySize := len(p.wtoTape.points) + len(p.wtoTape.components)
	freezer, err := newWorldProgramFreezer(p.builder.Arena(), p.builder.EffectArena(), p.shape, topologySize)
	if err != nil {
		return WorldProgram{}, err
	}
	topology, err := newStructuralProgramTopology(p.wtoTape, freezer)
	if err != nil {
		return WorldProgram{}, err
	}
	expressionWrites, err := p.structuralExpressionWrites()
	if err != nil {
		return WorldProgram{}, err
	}
	// Frame-result terms are immutable syntax indexed by their producing call
	// point and result slot. Carry this finite catalog across point compilation
	// so a later Return source can name the exact deferred frame output without
	// introducing a dataflow environment or another solve.
	resultRoots := make(map[ResultRoot]ValueTerm)
	for dense, item := range p.wtoTape.points {
		point, ref := item.point, programRef(dense+1)
		freezer.publication.points = append(freezer.publication.points, worldPointPublication{point: point, ref: ref})
		node := &freezer.programs[ref]
		node.kind, node.point = programSequence, point
		before, err := p.freshStructuralEnvironment(resultRoots)
		if err != nil {
			return WorldProgram{}, err
		}
		env, err := p.lowerStructuralPoint(direct, point, before.clone())
		if err != nil {
			return WorldProgram{}, fmt.Errorf("point %d: %w", point, err)
		}
		for root, value := range env.resultRoots {
			resultRoots[root] = value
		}
		memberCalls := cloneBoundaryMemberCallDiagnostics(env.output.memberCalls)
		env.output.memberCalls = nil // call instruction is the sole producer
		if len(memberCalls) > 1 {
			return WorldProgram{}, fmt.Errorf("point %d has %d contextual member-call producers", point, len(memberCalls))
		}
		var pointExternalMemberCall boundaryMemberCallDiagnosticTerm
		if len(memberCalls) == 1 {
			pointExternalMemberCall = memberCalls[0]
		}
		var n4Effects []EffectTerm
		for _, step := range env.steps {
			switch step.kind {
			case rowStepEffect:
				kind := p.builder.EffectArena().Kind(step.effect)
				if kind == EffectAllocationTemplate || kind == EffectObjectMaterialization {
					node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionEffect, effect: step.effect}))
				} else {
					n4Effects = append(n4Effects, step.effect)
				}
			case rowStepCall:
				node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionCallFrame, call: step.call, guard: step.guard, memberCall: step.memberCall}))
			default:
				return WorldProgram{}, fmt.Errorf("point %d emitted invalid ordered step", point)
			}
		}
		lexicalCall, residualGuard := false, Guard(0)
		if direct != nil {
			if site, found := direct.lookup(point); found {
				lexicalCall, residualGuard = !site.residual, env.callResidual
			}
		}
		if _, call := p.plan.Facts().CallSiteView(point); call && !lexicalCall && p.structuralCallRequiresExternalInstruction(point) {
			if !env.output.externalSealed {
				return WorldProgram{}, fmt.Errorf("point %d external call has no sealed value dependency terms", point)
			}
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionExternalCall, point: point, guard: residualGuard, access: cloneValueAccessTerms(env.output.externalAccess), operands: env.output.externalOperands.clone(), writes: append([]ValueTerm(nil), p.base.externalResults[point]...), memberCall: pointExternalMemberCall}))
		} else if pointExternalMemberCall.site != 0 {
			return WorldProgram{}, fmt.Errorf("point %d external member-call diagnostic has no canonical external-call producer", point)
		}
		resultTransaction := factapply.PlanCallResultTransaction(p.base.facts, point)
		if resultTransaction.HasMaterializeSteps() {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{
				kind: instructionCallResults, result: resultTransaction, resultPhase: factapply.ConcreteCallResultPhaseMaterialize,
			}))
		}
		// N1 is an actual terminal in the frozen circuit. It cannot be represented
		// by an empty outcome: N0 must execute, then the normal world becomes
		// Bottom and neither N3 nor any N5 terminal publication exists.
		if p.base.facts.NoNormalReturn(point) {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionNoNormalReturn}))
			continue
		}
		presenceTransaction := factapply.PlanPathValuePresenceImplicationTransaction(p.base.facts, point)
		if presenceTransaction.HasPublicationSteps() {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{
				kind: instructionPresenceImplications, presence: presenceTransaction,
			}))
		}
		if resultTransaction.HasPostconditionSteps() {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{
				kind: instructionCallResults, result: resultTransaction, resultPhase: factapply.ConcreteCallResultPhasePostconditions,
			}))
		}
		channelTransaction := factapply.PlanChannelSelectTransaction(p.base.facts, point)
		if channelTransaction.HasPublicationSteps() {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{
				kind: instructionChannelSelect, channel: channelTransaction,
			}))
		}
		var rootTarget symbol.ID
		if env.rootAssignment.transaction.Valid() {
			rootTarget = env.rootAssignment.transaction.TargetSymbol()
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionRootAssignment, rootAssignment: env.rootAssignment}))
		}
		for _, effect := range n4Effects {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionEffect, effect: effect}))
		}
		var genericTarget symbol.ID
		if generic, ok := p.plan.GenericForOperation(point); ok {
			genericTarget = generic.Target()
		}
		suppressed := make([]symbol.ID, 0, len(env.callResultSymbols)+2)
		suppressed = append(suppressed, rootTarget, genericTarget)
		for id := range env.callResultSymbols {
			suppressed = append(suppressed, id)
		}
		if err := appendEnvironmentWrites(freezer, node, p.builder.Arena(), before.values, env.values, suppressed...); err != nil {
			return WorldProgram{}, err
		}
		if genericTarget != 0 {
			binding, exact := env.genericBindings[genericTarget]
			if !exact || binding.Projection == 0 || !binding.Identity.valid(p.builder.Arena(), p.shape) {
				return WorldProgram{}, fmt.Errorf("point %d generic-for has no sealed value dependency term", point)
			}
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionGenericFor, point: point, access: []valueAccessTerm{{term: binding.Projection}}, genericIdentity: binding.Identity.clone()}))
		}
		covariantTransaction := factapply.PlanCovariantExposureTransaction(p.base.facts, point)
		env.output.paramExposures = p.boundaryParamExposures(covariantTransaction)
		pointContribution := structuralContribution(env, false)
		edges := p.wtoTape.edges[item.edgeBegin:item.edgeEnd]
		if current := p.graph.Node(point); current != nil && current.Kind == cfg.NodeReturn {
			if !env.returnTransaction.transaction.Valid() {
				transaction, exact := factapply.PlanReturnTransactionSources(p.base.facts, point, nil)
				if !exact {
					return WorldProgram{}, fmt.Errorf("point %d implicit return has no N5 transaction", point)
				}
				env.returnTransaction = returnTransactionTerm{transaction: transaction}
			}
			contribution := structuralContribution(env, true)
			contribution.paramObligations = p.declaredParamObligations()
			contribution.covariant = covariantTransaction
			contribution.branchLiteralCases = p.branchSufficientOutcomeTerms(point)
			if resultTransaction.HasPublicationSteps() {
				contribution.resultPublication = resultTransaction
			}
			payload := freezer.appendReturn(contribution)
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionReturn, ret: payload}))
			continue
		}
		if covariantTransaction.HasStateSteps() {
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{
				kind: instructionCovariantExposure, covariant: covariantTransaction,
			}))
		}
		if p.graph.IsBranch(point) {
			if len(edges) != 2 {
				return WorldProgram{}, fmt.Errorf("branch %d has %d structural edges", point, len(edges))
			}
			choiceRef := programRef(len(freezer.programs))
			freezer.programs = append(freezer.programs, programNode{kind: programChoice, point: point})
			if contributionPresent(pointContribution, p.shape) {
				payload := freezer.appendReturn(pointContribution)
				node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionContribution, ret: payload}))
			}
			freezer.programs[ref].next = choiceRef
			for _, edge := range edges {
				cond := edge.cond
				edgeEnv, guard, branchTransaction, branchKey, err := p.structuralBranch(point, env.edgeClone(), cond)
				if err != nil {
					return WorldProgram{}, err
				}
				target, err := topology.edgeTarget(edge)
				if err != nil {
					return WorldProgram{}, fmt.Errorf("branch %d: %w", point, err)
				}
				edgeRef := programRef(len(freezer.programs))
				freezer.programs = append(freezer.programs, programNode{kind: programSequence, point: point, next: target})
				freezer.publication.edges = append(freezer.publication.edges, worldEdgePublication{from: point, to: p.wtoTape.points[edge.to].point, ref: edgeRef})
				if branchTransaction.HasStateSteps() || branchTransaction.HasSufficientLiteralCases() || branchTransaction.HasRefinements() {
					freezer.programs[edgeRef].instructions = append(freezer.programs[edgeRef].instructions, freezer.appendInstruction(instructionNode{
						kind: instructionBranchRelations, branch: branchTransaction, value: branchKey,
					}))
				}
				if err := appendEnvironmentWrites(freezer, &freezer.programs[edgeRef], p.builder.Arena(), env.values, edgeEnv.values, 0); err != nil {
					return WorldProgram{}, err
				}
				if err := p.appendStructuralExpressionWrites(freezer, &freezer.programs[edgeRef], expressionWrites, point, p.wtoTape.points[edge.to].point, edgeEnv); err != nil {
					return WorldProgram{}, err
				}
				edgeContribution := structuralContribution(edgeEnv, false)
				if contributionPresent(edgeContribution, p.shape) {
					payload := freezer.appendReturn(edgeContribution)
					freezer.programs[edgeRef].instructions = append(freezer.programs[edgeRef].instructions, freezer.appendInstruction(instructionNode{kind: instructionContribution, ret: payload}))
				}
				if cond {
					freezer.programs[choiceRef].guard, freezer.programs[choiceRef].whenTrue = guard, edgeRef
				} else {
					freezer.programs[choiceRef].whenFalse = edgeRef
				}
			}
			choice := freezer.programs[choiceRef]
			if choice.guard == 0 || choice.whenTrue == 0 || choice.whenFalse == 0 {
				return WorldProgram{}, fmt.Errorf("branch %d structural Choice incomplete", point)
			}
		} else if len(edges) == 1 {
			if err := p.appendStructuralExpressionWrites(freezer, node, expressionWrites, point, p.wtoTape.points[edges[0].to].point, env); err != nil {
				return WorldProgram{}, err
			}
			if contributionPresent(pointContribution, p.shape) {
				payload := freezer.appendReturn(pointContribution)
				node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionContribution, ret: payload}))
			}
			target, targetErr := topology.edgeTarget(edges[0])
			err = targetErr
			if err != nil {
				return WorldProgram{}, fmt.Errorf("point %d: %w", point, err)
			}
			freezer.programs[ref].next = target
		} else if len(edges) == 0 {
			if !env.returnTransaction.transaction.Valid() {
				transaction, exact := factapply.PlanReturnTransactionSources(p.base.facts, point, nil)
				if !exact {
					return WorldProgram{}, fmt.Errorf("point %d implicit fallthrough has no N5 transaction", point)
				}
				env.returnTransaction = returnTransactionTerm{transaction: transaction}
			}
			contribution := structuralContribution(env, true)
			contribution.paramObligations = p.declaredParamObligations()
			contribution.covariant = covariantTransaction
			payload := freezer.appendReturn(contribution)
			node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{kind: instructionReturn, ret: payload}))
		} else {
			return WorldProgram{}, fmt.Errorf("point %d has %d non-branch edges", point, len(edges))
		}
	}
	entry := p.wtoTape.denseIndex(p.graph.Entry())
	if entry < 0 {
		return WorldProgram{}, fmt.Errorf("structural freezer has no entry")
	}
	return freezer.seal(topology.entryTarget(uint32(entry)))
}

func (p *PreparedPlanCompiler) structuralExpressionWrites() (map[structuralExpressionEdge][]structuralExpressionWrite, error) {
	if p == nil || p.plan == nil || p.graph == nil || p.builder == nil {
		return nil, fmt.Errorf("structural expressions have no prepared owner")
	}
	writes := make(map[structuralExpressionEdge][]structuralExpressionWrite)
	var compileErr error
	p.plan.ForEachStructuralExpressionRegion(func(ref factflow.ExprRef, region factflow.StructuralExpressionRegion) bool {
		operation, exact := p.plan.Facts().ExpressionOperation(ref)
		if !exact || operation.Kind() != factflow.ExpressionOperationBinary || operation.Op() != "and" && operation.Op() != "or" {
			compileErr = fmt.Errorf("logical expression %d has no exact binary operation", ref)
			return false
		}
		if _, bound := p.builder.Arena().expressionValue(ref); !bound {
			compileErr = fmt.Errorf("logical expression %d has no bound result cell", ref)
			return false
		}
		slot := statekey.ExpressionValue(uint32(ref))
		owned := make(map[cfg.Point]struct{}, len(region.OwnedRHSPoints()))
		for _, point := range region.OwnedRHSPoints() {
			owned[point] = struct{}{}
		}
		predecessors := cfg.PredecessorsReadOnly(p.graph, region.Join())
		if len(predecessors) < 2 {
			compileErr = fmt.Errorf("logical expression %d join %d has %d predecessors", ref, region.Join(), len(predecessors))
			return false
		}
		for _, predecessor := range predecessors {
			var source factflow.ValueSource
			_, rhsOwned := owned[predecessor]
			switch {
			case predecessor == region.Branch():
				source = operation.Left()
			case rhsOwned:
				source = operation.Right()
			default:
				compileErr = fmt.Errorf("logical expression %d join predecessor %d is outside its certified region", ref, predecessor)
				return false
			}
			edge := structuralExpressionEdge{from: predecessor, to: region.Join()}
			writes[edge] = append(writes[edge], structuralExpressionWrite{ref: ref, slot: slot, source: source})
		}
		return true
	})
	if compileErr != nil {
		return nil, fmt.Errorf("structural expression cells: %w", compileErr)
	}
	for edge := range writes {
		sort.Slice(writes[edge], func(i, j int) bool { return writes[edge][i].ref < writes[edge][j].ref })
	}
	return writes, nil
}

func (p *PreparedPlanCompiler) appendStructuralExpressionWrites(
	freezer *worldProgramFreezer,
	node *programNode,
	writes map[structuralExpressionEdge][]structuralExpressionWrite,
	from, to cfg.Point,
	env structuralEnvironment,
) error {
	for _, write := range writes[structuralExpressionEdge{from: from, to: to}] {
		ctx := p.base
		ctx.point, ctx.locals, ctx.resultRoots, ctx.genericBindings = from, env.values, env.resultRoots, env.genericBindings
		value, err := exactCompilerScalarSourceTerm(ctx, write.source)
		if err != nil {
			return fmt.Errorf("logical expression %d edge %d->%d: %w", write.ref, from, to, err)
		}
		if value == 0 || !p.builder.Arena().validEnvironmentSlot(write.slot) {
			return fmt.Errorf("logical expression %d edge %d->%d has invalid result write", write.ref, from, to)
		}
		node.instructions = append(node.instructions, freezer.appendInstruction(instructionNode{
			kind: instructionEnvironmentWrite, slot: write.slot, value: value,
		}))
	}
	return nil
}

// structuralCallRequiresExternalInstruction distinguishes a call whose N0
// producer must execute from a signature allocation already frozen as one
// route-owned effect/value transaction.  Emitting both would create two owners
// for the same call and demand an unrelated external read/provider row.
func (p *PreparedPlanCompiler) structuralCallRequiresExternalInstruction(point cfg.Point) bool {
	if p == nil || p.plan == nil {
		return true
	}
	if _, module := p.plan.ModuleLoadOperation(point); module {
		return true
	}
	_, allocation := p.plan.SignatureAllocationOperation(point)
	return !allocation
}

// sealDirectCallEnvironment closes the caller's lexical environment over the
// exact non-parameter roots consumed by every callee frame. These symbols are
// part of the call surface, so sealing only the caller plan's own boundary and
// assignment targets loses globals/captures that the callee names directly.
// The same environment terms then flow through ordinary State transport.
func (p *PreparedPlanCompiler) sealDirectCallEnvironment(direct frozenLexicalCallSurface) error {
	if p == nil || p.plan == nil || p.builder == nil || direct == nil {
		return nil
	}
	owned := make(map[symbol.ID]struct{}, len(p.environmentSymbols))
	for _, id := range p.environmentSymbols {
		owned[id] = struct{}{}
	}
	for raw := 0; raw < p.plan.PointCount(); raw++ {
		site, ok := direct.lookup(cfg.Point(raw))
		if !ok {
			continue
		}
		for _, candidate := range site.candidates {
			target := candidate.target
			if len(target.boundary.Captures) != int(target.shape.Captures) || len(target.boundary.Globals) != int(target.shape.Globals) ||
				len(target.boundary.Ambients) != int(target.shape.Ambients) || !validAmbientRoots(target.boundary.Ambients) {
				return fmt.Errorf("direct call point %d boundary width differs from target shape", raw)
			}
			ambientSymbols := make([]symbol.ID, len(target.boundary.Ambients))
			for index, root := range target.boundary.Ambients {
				ambientSymbols[index] = root.Symbol
			}
			for _, ids := range [][]symbol.ID{target.boundary.Captures, target.boundary.Globals, ambientSymbols} {
				for _, id := range ids {
					if id == 0 {
						return fmt.Errorf("direct call point %d has zero environment symbol", raw)
					}
					if _, exists := owned[id]; exists {
						continue
					}
					if p.builder.Arena().bindEnvironmentSymbol(id) == 0 {
						return fmt.Errorf("direct call point %d environment symbol %d could not be sealed", raw, id)
					}
					owned[id] = struct{}{}
					p.environmentSymbols = append(p.environmentSymbols, id)
				}
			}
		}
	}
	sort.Slice(p.environmentSymbols, func(i, j int) bool { return p.environmentSymbols[i] < p.environmentSymbols[j] })
	return nil
}

type structuralProgramTopology struct {
	tape       *symbolicWTOTape
	freezer    *worldProgramFreezer
	binders    []loopMuTerm
	wrappers   []programRef
	portals    []map[programRef]programRef
	exitRoutes []map[programRef]uint32
}

func newStructuralProgramTopology(tape *symbolicWTOTape, freezer *worldProgramFreezer) (*structuralProgramTopology, error) {
	if tape == nil || freezer == nil {
		return nil, fmt.Errorf("structural freezer has no WTO topology owner")
	}
	out := &structuralProgramTopology{
		tape: tape, freezer: freezer, binders: make([]loopMuTerm, len(tape.components)),
		wrappers: make([]programRef, len(tape.components)), portals: make([]map[programRef]programRef, len(tape.components)),
		exitRoutes: make([]map[programRef]uint32, len(tape.components)),
	}
	for index, component := range tape.components {
		members := make([]cfg.Point, 0, component.end-component.begin)
		for dense := component.begin; dense < component.end; dense++ {
			members = append(members, tape.points[dense].point)
		}
		backedges := make([]loopMuBackedge, 0)
		for _, edge := range tape.edges {
			if edge.kind == symbolicWTOEdgeBackedge && edge.component == int32(index) {
				backedges = append(backedges, loopMuBackedge{from: tape.points[edge.from].point, to: tape.points[edge.to].point})
			}
		}
		var parent loopMuTerm
		if component.parent >= 0 {
			parent = out.binders[component.parent]
		}
		binder := freezer.terms.loopMu(tape.points[component.head].point, parent, members, backedges)
		if binder == 0 {
			return nil, fmt.Errorf("structural freezer could not seal LoopMu component %d", index)
		}
		wrapper := programRef(len(tape.points) + index + 1)
		body := programRef(component.head + 1)
		out.binders[index], out.wrappers[index], out.exitRoutes[index] = binder, wrapper, make(map[programRef]uint32)
		out.portals[index] = map[programRef]programRef{body: wrapper}
		freezer.programs[wrapper] = programNode{kind: programLoopMu, point: tape.points[component.head].point, binder: binder, body: body}
	}
	return out, nil
}

func (t *structuralProgramTopology) entryTarget(dense uint32) programRef {
	point := t.tape.points[dense]
	if point.headComponent >= 0 {
		return t.wrappers[point.headComponent]
	}
	return programRef(dense + 1)
}

func (t *structuralProgramTopology) edgeTarget(edge symbolicWTOTapeEdge) (programRef, error) {
	var target programRef
	if edge.kind == symbolicWTOEdgeBackedge {
		if edge.component < 0 || int(edge.component) >= len(t.binders) {
			return 0, fmt.Errorf("backedge has no LoopMu owner")
		}
		target = t.appendLoopTerminal(instructionLoopFeedback, edge.component, 0)
	} else {
		target = t.entryTargetForEdge(edge)
	}
	component := t.tape.points[edge.from].component
	for remaining := edge.exitCount; remaining > 0; remaining-- {
		if component < 0 || int(component) >= len(t.binders) {
			return 0, fmt.Errorf("edge exit count exceeds component ancestry")
		}
		if edge.kind == symbolicWTOEdgeBackedge && component == edge.component {
			break
		}
		route := t.exitRoute(component, target)
		target = t.appendLoopTerminal(instructionLoopExit, component, route)
		component = t.tape.components[component].parent
	}
	return target, nil
}

func (t *structuralProgramTopology) entryTargetForEdge(edge symbolicWTOTapeEdge) programRef {
	if edge.enterCount == 0 {
		return programRef(edge.to + 1)
	}
	target := programRef(edge.to + 1)
	component := t.tape.points[edge.to].component
	for depth := 0; depth < edge.enterCount; depth++ {
		target = t.entryPortal(component, target)
		component = t.tape.components[component].parent
	}
	return target
}

// entryPortal starts one existing lexical mu binder at the exact CFG entry
// target. Reducible loops reuse the canonical head wrapper; irreducible SCCs
// receive one wrapper per external portal, all sharing the same binder and
// exit-route inventory. WTO head selection remains scheduling metadata and
// can therefore never redirect semantic control through the head.
func (t *structuralProgramTopology) entryPortal(component int32, target programRef) programRef {
	if component < 0 || int(component) >= len(t.binders) || target == 0 {
		return 0
	}
	if portal, ok := t.portals[component][target]; ok {
		return portal
	}
	portal := programRef(len(t.freezer.programs))
	t.freezer.programs = append(t.freezer.programs, programNode{
		kind: programLoopPortal, point: t.tape.points[t.tape.components[component].head].point,
		binder: t.binders[component], body: target,
	})
	t.portals[component][target] = portal
	return portal
}

func (t *structuralProgramTopology) exitRoute(component int32, target programRef) uint32 {
	if route, ok := t.exitRoutes[component][target]; ok {
		return route
	}
	route := uint32(len(t.freezer.programs[t.wrappers[component]].exits))
	t.freezer.programs[t.wrappers[component]].exits = append(t.freezer.programs[t.wrappers[component]].exits, target)
	t.exitRoutes[component][target] = route
	return route
}

func (t *structuralProgramTopology) appendLoopTerminal(kind instructionKind, component int32, route uint32) programRef {
	ref := programRef(len(t.freezer.programs))
	instruction := t.freezer.appendInstruction(instructionNode{kind: kind, binder: t.binders[component], route: route})
	t.freezer.programs = append(t.freezer.programs, programNode{kind: programSequence, point: t.tape.points[t.tape.components[component].head].point, instructions: []instructionRef{instruction}})
	return ref
}

func (p *PreparedPlanCompiler) freshStructuralEnvironment(resultRoots map[ResultRoot]ValueTerm) (structuralEnvironment, error) {
	env := structuralEnvironment{
		values: make(map[symbol.ID]ValueTerm, len(p.environmentSymbols)), resultRoots: cloneResultRootBindings(resultRoots),
		genericBindings: make(map[symbol.ID]symbolicGenericBinding, len(p.base.genericBindings)),
		preserved:       newBoundaryPreservationLedger(p.shape.Params, p.shape.Captures),
	}
	for _, id := range p.environmentSymbols {
		term, ok := p.builder.Arena().environmentValue(id)
		if !ok {
			return structuralEnvironment{}, fmt.Errorf("structural freezer: symbol %d has no sealed environment term", id)
		}
		env.values[id] = term
	}
	for id, binding := range p.base.genericBindings {
		env.genericBindings[id] = binding
	}
	return env, nil
}

func (p *PreparedPlanCompiler) lowerStructuralPoint(direct frozenLexicalCallSurface, point cfg.Point, env structuralEnvironment) (structuralEnvironment, error) {
	ctx := p.base
	ctx.locals, ctx.resultRoots, ctx.genericBindings, ctx.rowSteps, ctx.rowOutput, ctx.structuralOutput, ctx.structuralEnvironment, ctx.rootAssignment, ctx.returnTransaction = env.values, env.resultRoots, env.genericBindings, &env.steps, nil, &env.output, true, &env.rootAssignment, &env.returnTransaction
	ctx.point = point
	if fact, ok := p.plan.Facts().Return(point); ok {
		materialization, err := compileBoundaryReturnObjectMaterialization(ctx, point, fact)
		if err != nil {
			return structuralEnvironment{}, fmt.Errorf("return object materialization: %w", err)
		}
		if materialization != 0 {
			env.steps = append(env.steps, localEffectStep(materialization))
		}
	}
	if site, ok := p.plan.Facts().CallSiteView(point); ok {
		for slot := 0; slot < site.ArgumentSourceCount(); slot++ {
			anchor, durable := p.plan.CallArgumentObservationAnchor(point, uint32(slot))
			if durable {
				env.obligations = recordobservationObligation(env.obligations, observationObligation{BodyOwner: p.plan.ObservationBody(), Anchor: anchor, Guard: p.builder.Arena().True()})
			}
		}
		materialization, err := compileBoundaryCallObjectMaterialization(ctx, point, site)
		if err != nil {
			return structuralEnvironment{}, fmt.Errorf("call object materialization: %w", err)
		}
		if materialization != 0 {
			env.steps = append(env.steps, localEffectStep(materialization))
		}
	}
	cursor := p.plan.Cursor(point)
	for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
		handler := p.compiler.facts[cell.Kind()]
		target, directCall := frozenLexicalCallTarget{}, false
		var finiteSite frozenLexicalCallSite
		if cell.Kind() == operationplan.CallSite && direct != nil {
			if site, found := direct.lookup(point); found {
				if len(site.candidates) == 1 && site.candidates[0].identity == (identity.ID{}) && !site.residual {
					target, directCall = site.candidates[0].target, true
				} else {
					finiteSite = site
				}
			}
		}
		if len(finiteSite.candidates) != 0 {
			site, found := ctx.facts.CallSiteView(point)
			if !found {
				return structuralEnvironment{}, fmt.Errorf("finite call point %d has no call-site fact", point)
			}
			guards, residual, err := p.finiteCallGuards(ctx, site, finiteSite)
			if err != nil {
				return structuralEnvironment{}, err
			}
			memberCall := directMemberCallDiagnostic(ctx, point)
			for index, candidate := range finiteSite.candidates {
				bindings, err := exactDirectCallBindings(ctx, candidate.target.shape, candidate.target.boundary, site)
				if err != nil {
					return structuralEnvironment{}, err
				}
				if err := p.deferStructuralCall(&env, candidate.target, bindings, site, guards[index], memberCall); err != nil {
					return structuralEnvironment{}, err
				}
				ctx.locals, ctx.rowSteps = env.values, &env.steps
			}
			env.callResidual = residual
			if !finiteSite.residual {
				continue
			}
			// The residual branch retains the canonical dynamic/external producer.
			// Its stateful instruction is guarded when the WorldProgram is frozen;
			// this point-local lowering records only its conservative output lanes.
			env.preserved.observeFact(ctx, point, cell.Kind())
			if err := handler.Preflight(ctx, point); err != nil {
				return structuralEnvironment{}, fmt.Errorf("%s: %w", cell.Kind(), err)
			}
			if err := handler.Lower(ctx, point, &env.operations); err != nil {
				return structuralEnvironment{}, fmt.Errorf("%s: %w", cell.Kind(), err)
			}
			continue
		}
		if directCall {
			site, found := ctx.facts.CallSiteView(point)
			if !found {
				return structuralEnvironment{}, fmt.Errorf("direct call point %d has no call-site fact", point)
			}
			bindings, err := exactDirectCallBindings(ctx, target.shape, target.boundary, site)
			if err != nil {
				return structuralEnvironment{}, err
			}
			if err := p.deferStructuralCall(&env, target, bindings, site, 0, directMemberCallDiagnostic(ctx, point)); err != nil {
				return structuralEnvironment{}, err
			}
			ctx.locals, ctx.rowSteps = env.values, &env.steps
			continue
		}
		env.preserved.observeFact(ctx, point, cell.Kind())
		if err := handler.Preflight(ctx, point); err != nil {
			return structuralEnvironment{}, fmt.Errorf("%s: %w", cell.Kind(), err)
		}
		if err := handler.Lower(ctx, point, &env.operations); err != nil {
			return structuralEnvironment{}, fmt.Errorf("%s: %w", cell.Kind(), err)
		}
	}
	extensions := p.plan.ExtensionCursor(point)
	for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
		handler := p.compiler.extensions[cell.Kind()]
		env.preserved.observeExtension(cell.Kind())
		if err := handler.Preflight(ctx, point); err != nil {
			return structuralEnvironment{}, fmt.Errorf("extension %d: %w", cell.Kind(), err)
		}
		if err := handler.Lower(ctx, point, &env.operations); err != nil {
			return structuralEnvironment{}, fmt.Errorf("extension %d: %w", cell.Kind(), err)
		}
	}
	env.values, env.genericBindings = ctx.locals, ctx.genericBindings
	if assignment, ok := p.plan.Facts().RootAssignment(point); ok && !exactDirectLexicalDeclaration(ctx, assignment) {
		anchor, durable := p.plan.AssignmentObservationAnchor(point)
		if durable {
			env.obligations = recordobservationObligation(env.obligations, observationObligation{BodyOwner: p.plan.ObservationBody(), Anchor: anchor, Guard: p.builder.Arena().True()})
			value, present := env.values[assignment.TargetSymbol()]
			if !present {
				return structuralEnvironment{}, fmt.Errorf("observation: assignment target %d has no structural environment value", assignment.TargetSymbol())
			}
			env.observations = recordObservationTerm(env.observations, ObservationTerm{BodyOwner: p.plan.ObservationBody(), Kind: ObservationAssignment, Anchor: anchor, Guard: p.builder.Arena().True(), Actual: value})
		}
	}
	if site, ok := p.plan.Facts().CallSiteView(point); ok {
		for targetIndex := 0; targetIndex < site.ResultTargetCount(); targetIndex++ {
			target, found := site.ResultTargetAt(targetIndex)
			if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
				continue
			}
			anchor, durable := p.plan.CallResultObservationAnchor(point, uint32(targetIndex))
			if !durable {
				continue
			}
			env.obligations = recordobservationObligation(env.obligations, observationObligation{BodyOwner: p.plan.ObservationBody(), Anchor: anchor, Guard: p.builder.Arena().True()})
			if value, present := env.values[target.TargetSymbol()]; present {
				env.observations = recordObservationTerm(env.observations, ObservationTerm{BodyOwner: p.plan.ObservationBody(), Kind: ObservationCallResult, Anchor: anchor, Guard: p.builder.Arena().True(), Slot: uint32(targetIndex), Actual: value})
			}
		}
	}
	return env, nil
}

func directMemberCallDiagnostic(ctx planCompileContext, point cfg.Point) boundaryMemberCallDiagnosticTerm {
	memberCall, _, exact := boundaryMemberCallFromSite(ctx, point)
	if !exact {
		return boundaryMemberCallDiagnosticTerm{}
	}
	return memberCall
}

func (p *PreparedPlanCompiler) deferStructuralCall(env *structuralEnvironment, target frozenLexicalCallTarget, bindings DirectCallBindings, site factflow.CallSiteView, guard Guard, memberCall boundaryMemberCallDiagnosticTerm) error {
	rootBindings, err := NewTermRootBindings(target.shape, p.shape, bindings.Values, bindings.Paths)
	if err != nil {
		return err
	}
	targets, err := exactDirectCallTargets(site)
	if err != nil {
		return err
	}
	// A final open call contributes its complete value list to the caller's
	// declared return tuple.  Factflow records the explicit AST consumer; the
	// structural compiler must materialize the remaining semantic return slots
	// so the frame retains their values and cross-slot correlations.
	if site.Context() == factflow.CallSiteContextReturnSource && site.OpenTail() {
		limit := target.results
		if callerResults := uint32(planReturnArity(p.plan)); callerResults < limit {
			limit = callerResults
		}
		seen := make(map[int]struct{}, len(targets))
		for _, result := range targets {
			seen[result.slot] = struct{}{}
		}
		for slot := 0; slot < int(limit); slot++ {
			if _, exists := seen[slot]; exists {
				continue
			}
			targets = append(targets, directCallTarget{slot: slot, kind: factflow.CallResultTargetReturn})
		}
	}
	point, exact := site.Point()
	if !exact {
		return fmt.Errorf("structural call has no exact point")
	}
	// A frame models the callee's complete declared result tuple, not merely
	// the destinations named by the caller syntax.  In particular, Lua tail
	// return forwarding can consume an open result list whose later slots carry
	// error/value correlation even when the return expression has one AST node.
	// Sizing from syntactic targets alone silently drops those tuple coordinates.
	width := target.results
	for _, result := range targets {
		if next := uint32(result.slot + 1); next > width {
			width = next
		}
	}
	// Every lexical alternative publishes into one caller-owned point/slot
	// carrier. Consumers never name an alternative-owned FrameResult: doing so
	// bypasses the already-joined boundary value and makes ordinary N4 copy a
	// Bottom scalar alongside otherwise valid structural facts.
	for slot := uint32(0); slot < width; slot++ {
		if p.builder.arena.bindCallResult(point, int(slot)) == 0 {
			return fmt.Errorf("structural call result %d has no point-owned register", slot)
		}
	}
	producer := p.exactClosureProducerFrame(site, env.resultRoots)
	frame := p.builder.arena.relationFrameWithClosureProducer(target.variable, producer, point, 0, target.shape, rootBindings.values, rootBindings.paths, width)
	if frame == 0 {
		return fmt.Errorf("structural call frame is invalid")
	}
	step := deferredCallStep(frame)
	step.guard = guard
	step.memberCall = memberCall
	env.steps = append(env.steps, step)
	for _, result := range targets {
		value, exact := p.builder.arena.callResultValue(point, result.slot)
		if !exact || value == 0 {
			return fmt.Errorf("structural call result %d has no canonical point-owned register", result.slot)
		}
		if result.symbol != 0 {
			if env.callResultSymbols == nil {
				env.callResultSymbols = make(map[symbol.ID]struct{})
			}
			env.callResultSymbols[result.symbol] = struct{}{}
			if prior := env.values[result.symbol]; prior != 0 && prior != value {
				value = p.builder.arena.JoinValue(prior, value)
			}
			env.values[result.symbol] = value
		}
		root := ResultRoot{Point: point, Slot: uint32(result.slot)}
		if prior := env.resultRoots[root]; prior != 0 && prior != value {
			value = p.builder.arena.JoinValue(prior, value)
		}
		env.resultRoots[root] = value
	}
	return nil
}

// exactClosureProducerFrame follows the immutable lexical assignment chain of
// a direct callee back to one prior frame result. It records no abstract value
// and performs no analysis: factflow owns the definitions and resultRoots owns
// the already-frozen frame term. Ambiguous or mutable chains simply have no
// closure-resource provenance and retain the ordinary caller-boundary law.
func (p *PreparedPlanCompiler) exactClosureProducerFrame(site factflow.CallSiteView, results map[ResultRoot]ValueTerm) callFrameTerm {
	if p == nil || p.plan == nil || p.builder == nil || len(results) == 0 {
		return 0
	}
	source, exact := site.CalleeSource()
	if !exact {
		return 0
	}
	callee := site.CalleePathRef()
	switch source.Kind {
	case factflow.ValueSourceExpression:
		var sourcePathExact bool
		callee, sourcePathExact = p.plan.Facts().ExpressionPathRef(source.ExprRef)
		if !source.HasExpr || !sourcePathExact {
			return 0
		}
	case factflow.ValueSourcePath:
		if source.PathKey == "" || callee.Key() != source.PathKey {
			return 0
		}
	default:
		return 0
	}
	if callee.Symbol == 0 || callee.Version != 0 || len(callee.Segments) != 0 {
		return 0
	}
	return p.exactClosureProducerForSymbol(callee.Symbol, results, make(map[symbol.ID]bool))
}

func (p *PreparedPlanCompiler) exactClosureProducerForSymbol(target symbol.ID, results map[ResultRoot]ValueTerm, active map[symbol.ID]bool) callFrameTerm {
	if target == 0 || active[target] {
		return 0
	}
	active[target] = true
	defer delete(active, target)
	var source factflow.ValueSource
	found := false
	for raw := 0; raw < p.plan.PointCount(); raw++ {
		assignment, ok := p.plan.Facts().RootAssignment(cfg.Point(raw))
		if !ok || assignment.TargetSymbol() != target {
			continue
		}
		if found {
			return 0
		}
		found, source = true, assignment.Source()
	}
	if !found {
		return 0
	}
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.ResultIndex >= 0 {
		term := results[ResultRoot{Point: source.CallPoint, Slot: uint32(source.ResultIndex)}]
		if term == 0 || int(term) >= len(p.builder.arena.values) {
			return 0
		}
		producer, exact := p.uniqueFrameResultProducer(term, make(map[ValueTerm]bool))
		if !exact {
			return 0
		}
		return producer
	}
	var aliasPathSymbol symbol.ID
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return 0
		}
		alias, exact := p.plan.Facts().ExpressionPathRef(source.ExprRef)
		if !exact || alias.Version != 0 || len(alias.Segments) != 0 {
			return 0
		}
		aliasPathSymbol = alias.Symbol
	case factflow.ValueSourcePath:
		alias, exact := pathaddr.LocalPathFromKey(source.PathKey)
		if !exact || alias.Version != 0 || len(alias.Segments) != 0 {
			return 0
		}
		aliasPathSymbol = alias.Symbol
	default:
		return 0
	}
	if aliasPathSymbol == 0 {
		return 0
	}
	return p.exactClosureProducerForSymbol(aliasPathSymbol, results, active)
}

// uniqueFrameResultProducer recognizes the canonical join spelling created
// when structural control merges one call-result register. Environment terms
// contribute no producer identity; distinct frame results are ambiguous and
// therefore cannot select one closure resource.
func (p *PreparedPlanCompiler) uniqueFrameResultProducer(term ValueTerm, active map[ValueTerm]bool) (callFrameTerm, bool) {
	producer, found, ambiguous := p.scanFrameResultProducer(term, active)
	return producer, found && !ambiguous && producer != 0
}

func (p *PreparedPlanCompiler) scanFrameResultProducer(term ValueTerm, active map[ValueTerm]bool) (callFrameTerm, bool, bool) {
	if p == nil || p.builder == nil || term == 0 || int(term) >= len(p.builder.arena.values) || active[term] {
		return 0, false, active[term]
	}
	active[term] = true
	defer delete(active, term)
	node := p.builder.arena.values[term]
	switch node.op {
	case valueFrameResult:
		return node.frame, node.frame != 0, node.frame == 0
	case valueRefinement, valueFalsyAbsentRefinement, valueExpressionRefinement:
		if len(node.args) != 1 {
			return 0, false, true
		}
		return p.scanFrameResultProducer(node.args[0], active)
	case valueJoin:
		var producer callFrameTerm
		found := false
		for _, child := range node.args {
			candidate, childFound, ambiguous := p.scanFrameResultProducer(child, active)
			if ambiguous {
				return 0, false, true
			}
			if !childFound {
				continue
			}
			if producer != 0 && producer != candidate {
				return 0, false, true
			}
			producer = candidate
			found = true
		}
		return producer, found, false
	default:
		return 0, false, false
	}
}

func (p *PreparedPlanCompiler) finiteCallGuards(ctx planCompileContext, site factflow.CallSiteView, finite frozenLexicalCallSite) ([]Guard, Guard, error) {
	if p == nil || p.builder == nil || !finite.valid() {
		return nil, 0, fmt.Errorf("finite call has no sealed candidates")
	}
	// The callee operand's ValueSource is the canonical producer authority.
	// In particular, chained calls name a selected result register whose
	// diagnostic path is versioned; treating that path as a lexical binding
	// loses the already-frozen frame result.  Use the same scalar admission
	// boundary as direct-call composition, and retain the path only for older
	// fact producers that do not expose a callee source.
	var value ValueTerm
	var err error
	if source, exact := site.CalleeSource(); exact {
		value, err = exactCompilerScalarSourceTerm(ctx, source)
	} else {
		value, err = exactCompilerStaticPathTerm(ctx, site.CalleePathRef())
	}
	if err != nil {
		return nil, 0, fmt.Errorf("finite call target: %w", err)
	}
	arena := p.builder.Arena()
	guards := make([]Guard, len(finite.candidates))
	residual := make([]Guard, 0, len(finite.candidates))
	for index, candidate := range finite.candidates {
		constant := arena.Constant(identityvalue.Present(ctx.registry, candidate.identity))
		equal, ok := arena.ScalarBinaryValue("==", value, constant)
		if !ok || equal == 0 {
			return nil, 0, fmt.Errorf("finite call identity %d has no equality term", candidate.identity.Index)
		}
		guards[index] = arena.Truthy(equal)
		residual = append(residual, arena.Falsy(equal))
	}
	if !finite.residual {
		return guards, 0, nil
	}
	return guards, arena.And(residual...), nil
}

func (p *PreparedPlanCompiler) structuralBranch(point cfg.Point, env structuralEnvironment, cond bool) (structuralEnvironment, Guard, factapply.BranchRelationTransaction, ValueTerm, error) {
	arena := p.builder.Arena()
	if p.base.facts.BranchEdgeUnreachable(point, cond) {
		return env, arena.False(), factapply.BranchRelationTransaction{}, 0, nil
	}
	transaction := factapply.PlanBranchRelationTransaction(p.base.facts, point, cond)
	if genericForBranchHead(p.graph, p.plan, point) || numericForBranchHead(p.base, point) {
		continuation := arena.loopContinuationValue(point)
		if continuation == 0 {
			return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: loop continuation atom failed symbolic construction")
		}
		if cond {
			return env, arena.Truthy(continuation), transaction, 0, nil
		}
		return env, arena.Falsy(continuation), transaction, 0, nil
	}
	ctx := p.base
	ctx.locals, ctx.resultRoots, ctx.genericBindings = env.values, env.resultRoots, env.genericBindings
	ctx.point = point
	branch := factapply.NewBranchAlgebra(ctx.facts, point)
	source, ok := branch.ConditionSource()
	if !ok {
		return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: missing condition source")
	}
	condition, err := exactCompilerSourceTerm(ctx, source)
	if err != nil {
		return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: contextual condition source")
	}
	conditionNode := arena.values[condition]
	if conditionNode.op == valueDynamicRead {
		condition = arena.dynamicReadValueAtPaths(point, conditionNode.args[0], conditionNode.path, conditionNode.args[1], conditionNode.keyPath, conditionNode.indexShape, conditionNode.rangePath, conditionNode.integerProof)
		if condition == 0 {
			return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: dynamic condition has no lexical visibility owner")
		}
	}
	if err := validateRepresentedBranchEvidence(ctx, branch, condition); err != nil {
		return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, err
	}
	transaction = factapply.PlanBranchRelationTransaction(ctx.facts, point, cond)
	dynamicKey := ValueTerm(0)
	descriptor, _ := branch.Condition()
	conditionNode = arena.values[condition]
	if descriptor.TruthyOnEdge(cond) && conditionNode.op == valueDynamicRead {
		if len(conditionNode.args) != 2 || conditionNode.args[1] == 0 {
			return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: contextual dynamic condition evidence")
		}
		dynamic, dynamicIndex := ctx.facts.DynamicIndexExpression(source.ExprRef)
		if source.Kind == factflow.ValueSourceExpression && source.HasExpr && dynamicIndex {
			var attached bool
			transaction, attached = transaction.WithDynamicPresenceProof(dynamic.TablePathRef())
			if !attached {
				return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: dynamic condition table path is not exact")
			}
			dynamicKey = conditionNode.args[1]
		} else {
			path, exactPath := predicateSourcePath(ctx.facts, source)
			pathTerm, exactTerm := exactCompilerPathTerm(ctx, path)
			if !exactPath || !exactTerm || pathTerm != condition {
				return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, fmt.Errorf("branch: dynamic condition has no exact static path")
			}
		}
	}
	truthy, falsy, err := lowerBranchConditionGuards(arena, branch, func(source factflow.ValueSource) (ValueTerm, bool) {
		term, resolveErr := exactCompilerSourceTerm(ctx, source)
		if resolveErr != nil {
			return 0, false
		}
		term = arena.predicateObservationValue(point, term)
		return term, term != 0
	}, true, true, transaction.HasSufficientLiteralCases())
	if err != nil {
		return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, err
	}
	updates, err := lowerCompilerBranchRefinements(arena, branch, cond, ctx, condition)
	if err != nil {
		return structuralEnvironment{}, 0, factapply.BranchRelationTransaction{}, 0, err
	}
	applyStructuralBranchRootRefinements(env.values, updates)
	if cond {
		return env, truthy, transaction, dynamicKey, nil
	}
	return env, falsy, transaction, dynamicKey, nil
}

func applyStructuralBranchRootRefinements(values map[symbol.ID]ValueTerm, updates []SymbolicBranchRefinement) {
	for _, update := range updates {
		target := update.TargetPathRef()
		if len(target.Segments) == 0 {
			// The structural environment map owns lexical roots only. Descendant
			// refinements are already carried by the exact branch/path transaction;
			// installing a member value here would replace its table root.
			values[target.Symbol] = update.Value()
		}
	}
}

// branchSufficientOutcomeTerms binds converse literal cases to the exact edge
// region reaching one return. This is the same dominance/post-dominance
// occurrence rule used by concrete summary publication, but the payload stays
// symbolic until the stabilized tuple is projected.
func (p *PreparedPlanCompiler) branchSufficientOutcomeTerms(returnPoint cfg.Point) []branchSufficientOutcomeTerm {
	if p == nil || p.graph == nil || p.plan == nil {
		return nil
	}
	idom := dominance.ComputeImmediateDominators(p.graph)
	postdom := dominance.ComputeImmediatePostDominators(p.graph)
	var out []branchSufficientOutcomeTerm
	for _, branch := range cfg.RPOReadOnly(p.graph) {
		if branch == returnPoint || !p.graph.IsBranch(branch) {
			continue
		}
		successors := cfg.SuccessorsReadOnly(p.graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(p.graph, branch)
		if len(successors) != len(conditions) {
			continue
		}
		for index, successor := range successors {
			if !dominance.Dominates(idom, successor, returnPoint) || !dominance.PostDominates(postdom, returnPoint, successor) {
				continue
			}
			selected := conditions[index]
			transaction := factapply.PlanBranchRelationTransaction(p.plan.Facts(), branch, selected)
			for stepIndex := 0; stepIndex < transaction.Len(); stepIndex++ {
				step, _ := transaction.Step(stepIndex)
				literalCase, ok := step.SufficientLiteralCase()
				if !ok {
					continue
				}
				out = append(out, branchSufficientOutcomeTerm{
					branch: branch, selectedEdge: selected, literalCase: literalCase,
				})
			}
		}
	}
	return out
}

func appendEnvironmentWrites(f *worldProgramFreezer, node *programNode, arena *Arena, before, after map[symbol.ID]ValueTerm, suppressed ...symbol.ID) error {
	suppressedSet := make(map[symbol.ID]struct{}, len(suppressed))
	for _, id := range suppressed {
		if id != 0 {
			suppressedSet[id] = struct{}{}
		}
	}
	ids := make([]symbol.ID, 0)
	for id, value := range after {
		if _, skip := suppressedSet[id]; !skip && before[id] != value {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		slot := statekey.SymbolValue(id)
		if !arena.validEnvironmentSlot(slot) {
			return fmt.Errorf("structural freezer: write to unsealed symbol %d", id)
		}
		node.instructions = append(node.instructions, f.appendInstruction(instructionNode{kind: instructionEnvironmentWrite, slot: slot, value: after[id]}))
	}
	return nil
}

func structuralContribution(env structuralEnvironment, terminal bool) semanticContribution {
	return semanticContribution{suspensionKnown: terminal, maySuspend: env.output.maySuspend, operations: env.operations,
		observations: env.observations, observationObligations: env.obligations,
		preserved: env.preserved, returnConditions: env.output.returnConditions, paramExposures: env.output.paramExposures,
		paramObligations:  env.output.paramObligations,
		returnTransaction: env.returnTransaction.clone()}
}

func (p *PreparedPlanCompiler) declaredParamObligations() []callpayload.CallParamObligation {
	if p == nil || p.plan == nil || p.registry == nil {
		return nil
	}
	contracts := p.plan.BoundaryParamContracts()
	out := make([]callpayload.CallParamObligation, 0, len(contracts))
	for index, contract := range contracts {
		contractType, typed := typevalue.TypeOf(p.registry, contract)
		if !usefulBoundaryDiagnosticValue(p.registry, contract) || !typed || contractType == nil ||
			typ.IsAny(contractType) || typ.IsUnknown(contractType) || typ.IsNever(contractType) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{
			ParamIndex: index, Value: contract, SignatureSurface: true,
		})
	}
	return out
}

// boundaryParamExposures projects the exact N6 event vocabulary onto the
// lexical parameter boundary. Reachability remains structural: the returned
// terms are attached to this point's contribution and therefore publish only
// from stabilized worlds which reach the point.
func (p *PreparedPlanCompiler) boundaryParamExposures(transaction factapply.CovariantExposureTransaction) []boundaryParamExposureTerm {
	if p == nil || p.plan == nil || p.builder == nil {
		return nil
	}
	var out []boundaryParamExposureTerm
	for index := 0; index < transaction.Len(); index++ {
		step, ok := transaction.Step(index)
		if !ok {
			continue
		}
		exposure := step.Exposure()
		source := exposure.SourcePath()
		paramIndex, boundary := p.plan.BoundaryParamIndex(source.Symbol)
		if !boundary || source.Symbol == 0 || source.Version != 0 {
			continue
		}
		path := p.builder.Arena().Path(Root{Kind: RootParam, Index: uint32(paramIndex)}, source.Segments...)
		if path == 0 || !product.BelongsToRegistry(p.registry, exposure.WideValue()) {
			continue
		}
		out = append(out, boundaryParamExposureTerm{
			source: path, contract: exposure.WideValue(), kind: exposure.Kind(),
		})
	}
	return out
}

func contributionPresent(payload semanticContribution, shape Shape) bool {
	if payload.preserved.equal(newBoundaryPreservationLedger(shape.Params, shape.Captures)) {
		payload.preserved = paramPreservationLedger{}
	}
	return !payload.empty()
}
