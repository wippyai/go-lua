package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalApplyCallOutcomeProjection is the complete stabilized input to the
// CallOutcome boundary adapter. Region retains caller/target correlation and
// publication is the already-executed canonical factor transaction in caller
// ownership. An adapter may project DTO lanes from these carriers, but may not
// evaluate another equation or reconstruct the boundary transfer.
type formalApplyCallOutcomeProjection struct {
	step        *formalApplyStep
	region      formalApplyCorrelatedRegion
	publication formalApplyLeafPublication
}

// detachFormalApplyCallOutcomes validates and encodes the exact observation
// witnesses produced by the final canonical Apply evaluations. It neither
// evaluates an equation nor mutates the stabilized solution map.
func (e *formalRelationExecution) detachFormalApplyCallOutcomes(
	ctx context.Context,
) (map[formalRelationCell]callpayload.CallOutcomeAlternativeSet, error) {
	if ctx == nil || e == nil || e.algebra == nil || e.algebra.program == nil ||
		e.algebra.program.formalTemplate == nil || e.values == nil {
		return nil, fmt.Errorf("transformer: formal Apply CallOutcome detachment is unowned")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	program := e.algebra.program
	out := make(map[formalRelationCell]callpayload.CallOutcomeAlternativeSet, len(program.formalTemplate.applyCells))
	factorOrdinals := make(map[relationVar][]int)
	for _, site := range program.formalTemplate.applyCells {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, present := e.values[site.cell]
		if !present {
			return nil, fmt.Errorf("transformer: formal Apply CallOutcome site is absent")
		}
		witness, witnessed := e.algebra.applyObservations[site.cell]
		if current.bottom() {
			out[site.cell] = callpayload.CallOutcomeAlternativeSet{}
			continue
		}
		if !witnessed || witness.observation.step == nil {
			return nil, fmt.Errorf("transformer: reachable Apply has no stabilized observation witness")
		}
		predecessor, exact := e.values[witness.predecessorCell]
		if !exact || !e.algebra.equal(predecessor, witness.predecessorValue) {
			return nil, fmt.Errorf("transformer: Apply observation predecessor is not stabilized")
		}
		if len(witness.outcomeCells) != len(witness.outcomeValues) {
			return nil, fmt.Errorf("transformer: Apply observation target row is malformed")
		}
		for index, cell := range witness.outcomeCells {
			value, exact := e.values[cell]
			if !exact || !e.algebra.equal(value, witness.outcomeValues[index]) {
				return nil, fmt.Errorf("transformer: Apply observation target %d is not stabilized", index)
			}
		}
		var alternatives callpayload.CallOutcomeAlternativeSet
		for _, observed := range witness.observation.regions {
			projected, err := e.projectFormalApplyCallOutcome(ctx, formalApplyCallOutcomeProjection{
				step: witness.observation.step, region: observed.region, publication: observed.publication,
			}, factorOrdinals)
			if err != nil {
				return nil, err
			}
			if projected.Empty() {
				return nil, fmt.Errorf("transformer: reachable internal Apply omitted its explicit CallOutcome")
			}
			alternatives = alternatives.Join(program.registry, projected)
		}
		out[site.cell] = alternatives.Normalize(program.registry)
	}
	if err := e.algebra.err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *formalRelationExecution) projectFormalApplyCallOutcome(
	ctx context.Context,
	projection formalApplyCallOutcomeProjection,
	factorOrdinals map[relationVar][]int,
) (callpayload.CallOutcomeAlternativeSet, error) {
	if e == nil || e.algebra == nil || projection.step == nil || projection.publication.normalReturn == nil ||
		!projection.region.caller.valid() || !projection.region.target.valid() {
		return callpayload.CallOutcomeAlternativeSet{}, fmt.Errorf("transformer: formal Apply CallOutcome adapter is unowned")
	}
	step, source := projection.step, projection.publication.normalReturn
	target := &e.algebra.program.bodies[step.target-1]
	encoder, err := factapply.PrepareNormalReturnFactorEncoder[FormalSlot](
		target.productDomain, source.selection, int(target.relation.shape.Params),
	)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	ordinals, prepared := factorOrdinals[step.target]
	if !prepared {
		ordinals, err = freezeFormalNormalReturnFactorOrdinals(encoder.Lanes(), source.factors)
		if err != nil {
			return callpayload.CallOutcomeAlternativeSet{}, err
		}
		factorOrdinals[step.target] = ordinals
	}
	factors, err := selectFormalNormalReturnFactors(encoder.Lanes(), source.factors, ordinals)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	operands := make([]factapply.NormalReturnFactorOperand[FormalSlot], len(source.roots))
	for index, root := range source.roots {
		operands[index] = factapply.NormalReturnFactorOperand[FormalSlot]{
			Slot: root.slot, BoundarySlot: root.boundarySlot, Value: root.value,
		}
	}
	boundary, err := encoder.Encode(ctx, nil, factors, operands)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	results, dense, err := e.formalApplyPublishedResults(projection)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	conditions, relations := projectLexicalParamOutcomeFacts(
		e.algebra.program.registry, boundary.Roots, int(target.relation.shape.Params), boundary.NormalReturnFacts,
	)
	childDiagnostics, _, err := e.algebra.formalDiagnosticLeaf(projection.region.target)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	caller := &e.algebra.program.bodies[step.owner-1]
	// The encoder reads the target's formal factor image. CallOutcome is a
	// caller-concrete DTO, so this adapter is the single structural ownership
	// boundary for nested heap keys before the outcome enters caller storage.
	heapOutcome, err := (callpayload.CallOutcome{HeapTableObjects: boundary.HeapTableObjects}).RekeyHeapTableObjects(
		projection.region.target.authority.coordinateKeys, caller.keys,
	)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, fmt.Errorf("transformer: formal Apply heap publication: %w", err)
	}
	projectedDiagnostics, err := projectLinkedCallDiagnosticsWithPath(caller, target, step.linked, childDiagnostics,
		func(path pathdom.Path) (pathdom.Path, bool) {
			return projectFormalLinkedBoundaryDiagnosticPath(projection.region.caller, step, path)
		})
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	declared := declaredBoundaryReturns(target)
	var outcomes []callpayload.CallOutcome
	for _, occurrence := range projection.region.occurrences {
		out, err := e.projectFormalApplyOccurrence(projection.region, occurrence)
		if err != nil {
			return callpayload.CallOutcomeAlternativeSet{}, err
		}
		out.Results = append([]callpayload.CallResult(nil), results...)
		out.NormalReturnFacts = callboundary.NormalizeNormalReturnFacts(e.algebra.program.registry,
			boundary.NormalReturnFacts.Append(out.NormalReturnFacts))
		out.HeapTableObjects = heapOutcome.HeapTableObjects
		out.Placements = boundary.Placements
		out.ParamConditions = append([]callpayload.CallParamCondition(nil), conditions...)
		out.ParamPathRelations = append([]callpayload.CallParamPathRelation(nil), relations...)
		projectedDiagnostics.ApplyTo(e.algebra.program.registry, &out)
		rows := [][]product.Value{append([]product.Value(nil), dense...)}
		correlations, presenceRelations, exact := inferReturnRowCorrelations(e.algebra.program.registry,
			[]summary.Summary{{Returns: append([]product.Value(nil), dense...)}}, rows, declared)
		if !exact {
			return callpayload.CallOutcomeAlternativeSet{}, fmt.Errorf("transformer: formal Apply return correlation is not exact")
		}
		for _, correlation := range correlations {
			out.ReturnConditionSlots = append(out.ReturnConditionSlots, callpayload.CallReturnConditionSlotRefinement{
				ReturnIndex: correlation.ReturnIndex, ReturnValue: correlation.ReturnValue,
				TargetIndex: correlation.TargetIndex, Value: correlation.Value,
			})
		}
		for _, relation := range presenceRelations {
			out.ReturnPresenceRelations = appendUniqueReturnPresence(out.ReturnPresenceRelations, callpayload.CallReturnPresenceRelation{
				TriggerIndex: relation.TriggerIndex, TriggerPresence: relation.TriggerPresence,
				TargetIndex: relation.TargetIndex, TargetPresence: relation.TargetPresence,
			})
		}
		for _, relation := range projectReturnPresenceTransaction(occurrence.code.outcomes[occurrence.ref].resultPublication) {
			out.ReturnPresenceRelations = appendUniqueReturnPresence(out.ReturnPresenceRelations, relation)
		}
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(e.algebra.program.registry, out)
		if err := validateRelationCallOutcomeCanonicalLanes(out); err != nil {
			return callpayload.CallOutcomeAlternativeSet{}, err
		}
		outcomes = append(outcomes, out)
	}
	return callpayload.NewCallOutcomeAlternativeSet(e.algebra.program.registry, outcomes...), nil
}

func freezeFormalNormalReturnFactorOrdinals(want []state.ProductLane, available []state.LaneFactor) ([]int, error) {
	out := make([]int, len(want))
	for index, lane := range want {
		out[index] = -1
		for ordinal, factor := range available {
			if factor.Lane() == lane {
				out[index] = ordinal
				break
			}
		}
		if out[index] < 0 {
			return nil, fmt.Errorf("transformer: formal normal-return lane %q is absent", lane.ID())
		}
	}
	return out, nil
}

func selectFormalNormalReturnFactors(want []state.ProductLane, available []state.LaneFactor, ordinals []int) ([]state.LaneFactor, error) {
	if len(want) != len(ordinals) {
		return nil, fmt.Errorf("transformer: formal normal-return lane plan is malformed")
	}
	out := make([]state.LaneFactor, len(want))
	for index, ordinal := range ordinals {
		if ordinal < 0 || ordinal >= len(available) || available[ordinal].Lane() != want[index] {
			return nil, fmt.Errorf("transformer: formal normal-return lane %q changed after preparation", want[index].ID())
		}
		out[index] = available[ordinal]
	}
	return out, nil
}

func (e *formalRelationExecution) formalApplyPublishedResults(
	projection formalApplyCallOutcomeProjection,
) ([]callpayload.CallResult, []product.Value, error) {
	leaf := projection.region.caller
	selection, err := denseFormalFiberLeafSelection(leaf.span, projection.publication.leaves)
	if err != nil {
		return nil, nil, err
	}
	leaf.leaves = selection
	leaf.guard = projection.publication.guard
	if !leaf.valid() {
		return nil, nil, errFormalComponentForeignOwner
	}
	reg := e.algebra.program.registry
	results := make([]callpayload.CallResult, 0, len(projection.step.linked.resultSelectors))
	for _, selector := range projection.step.linked.resultSelectors {
		if !linkedResultHasStateTarget(selector) {
			continue
		}
		descriptor, err := e.algebra.applyResultDescriptor(projection.step.owner, projection.step.linked.point, selector.slot)
		if err != nil {
			return nil, nil, err
		}
		ordinal, present := leaf.span.ordinal(descriptor)
		if !present {
			return nil, nil, errFormalComponentMalformed
		}
		value := product.Absent(reg)
		resultLeaf, selected := leaf.leaves.leaf(ordinal)
		if !selected {
			return nil, nil, errFormalComponentMalformed
		}
		if resultLeaf != 0 {
			terminal, terminalErr := leaf.authority.terminal(resultLeaf)
			if terminalErr != nil {
				return nil, nil, terminalErr
			}
			if terminal.kind != formalComponentBindings || len(terminal.bindings) == 0 {
				return nil, nil, errFormalComponentMalformed
			}
			value = product.Bottom(reg)
			for _, binding := range terminal.bindings {
				evaluated, evaluateErr := leaf.evaluate(binding)
				if evaluateErr != nil {
					op := valueOp(0)
					root := Root{}
					if binding.value.arena != nil && int(binding.value.term) < len(binding.value.arena.values) {
						op = binding.value.arena.values[binding.value.term].op
						root = binding.value.arena.values[binding.value.term].root
					}
					return nil, nil, fmt.Errorf("transformer: formal Apply projected result %d binding owner %d term %d op %d root=%d/%d apply=%t: %w", selector.slot, binding.value.owner, binding.value.term, op, root.Kind, root.Index, binding.apply.present(), evaluateErr)
				}
				value = product.Join(reg, value, evaluated.value)
			}
		}
		results = append(results, callpayload.CallResult{Index: int(selector.slot), Value: value})
	}
	return results, denseCallResults(reg, results), nil
}

func (e *formalRelationExecution) projectFormalApplyOccurrence(
	region formalApplyCorrelatedRegion,
	occurrence formalQualifiedOutcomeOccurrence,
) (callpayload.CallOutcome, error) {
	if occurrence.code == nil || occurrence.ref == 0 || int(occurrence.ref) >= len(occurrence.code.outcomes) ||
		occurrence.code.terms == nil || region.target.variable == 0 {
		return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal Apply outcome occurrence is malformed")
	}
	payload := occurrence.code.outcomes[occurrence.ref]
	out := callpayload.CallOutcome{
		SuspensionKnown:        payload.suspensionKnown,
		MaySuspend:             payload.maySuspend,
		ProtectedCallTypestate: payload.protectedCallTypestate.Clone(),
	}
	for _, operation := range payload.operations {
		if operation.Descriptor != DescriptorObligation {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal call outcome retained unsupported descriptor %q", operation.Descriptor)
		}
		value, exact := region.target.evalArenaValue(region.target.variable, occurrence.code.terms, operation.Value, occurrence.scope, formalApplyTermView{})
		if !exact {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal call obligation %d is not exact", operation.Slot)
		}
		out.ParamObligations = append(out.ParamObligations, callpayload.CallParamObligation{ParamIndex: int(operation.Slot), Value: value})
	}
	for _, proof := range payload.proofs {
		path, exact := proof.placeholderPath(occurrence.code.terms)
		if !exact {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal call proof has no placeholder path")
		}
		if proof.Key != 0 {
			value, exact := region.target.evalArenaValue(region.target.variable, occurrence.code.terms, proof.Key, occurrence.scope, formalApplyTermView{})
			if !exact {
				return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal call proof key is not exact")
			}
			segment, exact := typevalue.ExactScalarKeySegment(e.algebra.program.registry, nil, value)
			if !exact {
				return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal call proof key is not a finite scalar")
			}
			path.Segments = append(path.Segments, segment)
		}
		out.NormalReturnFacts.BranchProofs = append(out.NormalReturnFacts.BranchProofs, callboundary.BranchProof{
			Kind: proof.Kind, Path: path, Presence: proof.Presence,
		})
	}
	values, err := region.target.valuesFactor()
	if err != nil {
		return callpayload.CallOutcome{}, err
	}
	for _, refinement := range payload.refinements {
		root, exact := refinement.preservedRoot(occurrence.code.terms)
		if !exact || root.Kind != RootParam {
			continue
		}
		concrete, exact := region.target.body.rootValueSlot(root)
		if !exact {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal preserved parameter has no concrete slot")
		}
		slot, exact := formalMiddleSlotForStateKey(e.algebra.program, region.target.body, concrete)
		if !exact {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: formal preserved parameter has no slot")
		}
		value, useful := callboundary.ProjectPathRefinementValue(e.algebra.program.registry,
			formalApplyValueAt(values, slot, product.Bottom(e.algebra.program.registry)))
		if useful {
			out.ParamPathRefinements = append(out.ParamPathRefinements, callpayload.CallParamPathRefinement{
				Path: pathdom.NewPlaceholder(int(root.Index)), Value: value,
			})
		}
	}
	for _, condition := range payload.returnConditions {
		out.ReturnConditionRefinements = append(out.ReturnConditionRefinements, callpayload.CallReturnConditionRefinement{
			ReturnIndex: condition.ReturnIndex, ReturnValue: condition.ReturnValue,
			Target: condition.Target.Clone(), Value: condition.Value,
		})
	}
	return out, nil
}

func projectFormalLinkedBoundaryDiagnosticPath(
	caller formalTupleLeafEvaluator,
	step *formalApplyStep,
	targetPath pathdom.Path,
) (pathdom.Path, bool) {
	if step == nil || !caller.valid() {
		return pathdom.Path{}, false
	}
	target := &caller.algebra.program.bodies[step.target-1]
	root, suffix, exact := bodyBoundaryRootForDiagnosticPath(target, targetPath)
	if !exact {
		return pathdom.Path{}, false
	}
	if root.Kind == RootParam {
		return targetPath.Clone(), true
	}
	offset := step.linked.shape.offset(root.Kind) + int(root.Index)
	if offset < 0 || offset >= len(step.linked.rootCircuit) || step.linked.rootCircuit[offset].root != root {
		return pathdom.Path{}, false
	}
	wire := step.linked.rootCircuit[offset]
	if wire.path != 0 {
		base, ok := caller.evalArenaPath(step.owner, caller.body.relation.arena, wire.path, formalApplyTermView{})
		if ok && !base.IsEmpty() {
			return base.AppendSegments(suffix), true
		}
	}
	if wire.value == 0 || int(wire.value) >= len(caller.body.relation.arena.values) {
		return pathdom.Path{}, false
	}
	node := caller.body.relation.arena.values[wire.value]
	if node.op == valueEnvironment {
		if symbol := rootSymbol(node.slot); symbol != 0 {
			return pathdom.NewPath(symbol, "").AppendSegments(suffix), true
		}
	}
	if node.op == valueRoot && caller.body.relation.shape.validateInput(node.root) {
		base, ok := bodyRelativeBoundaryDiagnosticRootPath(caller.body, node.root, nil)
		if ok {
			return base.AppendSegments(suffix), true
		}
	}
	return pathdom.Path{}, false
}
