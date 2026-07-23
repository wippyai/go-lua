package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalApplyStep is the frozen authority for one Apply equation. It points at
// the already-linked frame and its simultaneous caller→target binding; later
// fields retain only presealed factor/slot/path lenses derived from that same
// frame. The executor never recovers a call frame by inspecting Step syntax.
type formalApplyStep struct {
	owner   relationVar
	target  relationVar
	frame   callFrameTerm
	binding relationLazyApplyBinding
	linked  *linkedRelationFrame
	seeds   state.EntrySeedFactorPlan[FormalSlot]
	// valueAccess and valueFactorGroups are the complete registered-factor
	// vocabulary required to evaluate caller argument/ambient ValueTerms. They
	// are frozen from the term DAG once; Apply never guesses an axis from the
	// encountered value operation.
	valueAccess       state.TransferInputAccess
	valueFactorGroups []formalFiberGroupDescriptor
	// resultValueAccess is the same capability for retained callee result
	// expressions. It is frozen over every N5 source in the target relation and
	// materialized from the correlated target leaf at Apply execution.
	resultValueAccess       state.TransferInputAccess
	resultValueFactorGroups []formalFiberGroupDescriptor
}

func (s *formalApplyStep) validFor(program *RelationProgram, owner relationVar) bool {
	if s == nil || program == nil || owner == 0 || s.owner != owner || s.target == 0 || s.frame == 0 ||
		int(owner) > len(program.bodies) || int(s.target) > len(program.bodies) ||
		!s.binding.validFor(owner, s.target, s.frame) || s.linked == nil || !s.linked.valid() || !s.seeds.Valid() {
		return false
	}
	caller := &program.bodies[owner-1]
	_, present := program.formalFibers.span(s.target)
	callerSpan, callerPresent := program.formalFibers.span(owner)
	groupsValid := callerPresent && len(s.valueFactorGroups) == s.valueAccess.Lanes.Len()
	for _, group := range s.valueFactorGroups {
		groupsValid = groupsValid && group.valid() && group.variable == owner && group.kind != formalFiberGroupValues && group.forest == callerSpan.forest && s.valueAccess.Lanes.Has(group.lane.ID())
	}
	targetSpan, targetPresent := program.formalFibers.span(s.target)
	resultGroupsValid := targetPresent && len(s.resultValueFactorGroups) == s.resultValueAccess.Lanes.Len()
	for _, group := range s.resultValueFactorGroups {
		resultGroupsValid = resultGroupsValid && group.valid() && group.variable == s.target && group.kind != formalFiberGroupValues &&
			group.forest == targetSpan.forest && s.resultValueAccess.Lanes.Has(group.lane.ID())
	}
	return groupsValid && resultGroupsValid && int(s.frame) < len(caller.frames) && s.linked == &caller.frames[s.frame] &&
		s.linked.owner == owner && s.linked.target == s.target && s.linked.term == s.frame && present
}

func (s *formalApplyStep) validForFootprint(program *RelationProgram, footprint formalOperatorCoordinateFootprint) bool {
	if s == nil || program == nil || s.target == 0 || int(s.target) > len(program.bodies) {
		return false
	}
	target := &program.bodies[s.target-1]
	span, present := program.formalFibers.span(s.target)
	return present && footprint.source.KeySpace() == span.keys &&
		footprint.source.ValidFor(target.productDomain, span.keys)
}

// freezeFormalApplyStep resolves the sole linked boundary frame once while the
// equation template is sealed. A nil result is valid only for non-Apply Steps.
func freezeFormalApplyStep(program *RelationProgram, owner relationVar, operator formalRelationOperatorRef) (*formalApplyStep, error) {
	step, ok := formalRelationStepOperator(operator)
	if !ok {
		return nil, fmt.Errorf("transformer: formal Apply freeze has no Step syntax")
	}
	if step.kind != boundaryStepApply {
		return nil, nil
	}
	if program == nil || owner == 0 || int(owner) > len(program.bodies) || step.apply.variable == 0 ||
		int(step.apply.variable) > len(program.bodies) || step.apply.frame == 0 {
		return nil, fmt.Errorf("transformer: formal Apply freeze has a foreign frame")
	}
	caller := &program.bodies[owner-1]
	if int(step.apply.frame) >= len(caller.frames) || int(step.apply.frame) >= len(operator.code.applicationGuards) {
		return nil, fmt.Errorf("transformer: formal Apply frame is undeclared")
	}
	linked := &caller.frames[step.apply.frame]
	guard := operator.code.applicationGuards[step.apply.frame]
	result := &formalApplyStep{
		owner: owner, target: step.apply.variable, frame: step.apply.frame,
		binding: guard.binding, linked: linked,
	}
	target := &program.bodies[result.target-1]
	seeds, seedErr := state.BindEntrySeedFactorPlan(program.registry, target.entrySeedPlan, func(slot statekey.Value) (FormalSlot, bool) {
		return formalInitialValueSlot(program.formalFibers.slots, target, slot)
	})
	if seedErr != nil {
		return nil, fmt.Errorf("transformer: formal Apply EntrySeed rekey: %w", seedErr)
	}
	result.seeds = seeds
	terms := make([]ValueTerm, 0, len(linked.rootCircuit)+len(linked.ambientCircuit))
	for _, wire := range linked.rootCircuit {
		terms = append(terms, wire.value)
	}
	for _, wire := range linked.ambientCircuit {
		terms = append(terms, wire.value)
	}
	result.valueAccess, result.valueFactorGroups, seedErr = freezeFormalValueFactorAccess(program, owner, terms...)
	if seedErr != nil {
		return nil, fmt.Errorf("transformer: formal Apply value access: %w", seedErr)
	}
	resultTerms := make([]ValueTerm, 0)
	for outcome := 1; outcome < len(target.relation.code.outcomes); outcome++ {
		resultTerms = append(resultTerms, target.relation.code.outcomes[outcome].returnTransaction.sources...)
	}
	result.resultValueAccess, result.resultValueFactorGroups, seedErr = freezeFormalValueFactorAccess(program, result.target, resultTerms...)
	if seedErr != nil {
		return nil, fmt.Errorf("transformer: formal Apply result value access: %w", seedErr)
	}
	if !guard.validFor(step.apply.frame, step.apply.variable) || !result.validFor(program, owner) ||
		!result.validForFootprint(program, operator.footprint) {
		return nil, fmt.Errorf("transformer: formal Apply frame is not a complete frozen boundary")
	}
	return result, nil
}

// formalApplyActual is one target input wire evaluated exactly once in a
// caller tuple leaf.  Value is the canonical missing-only EntrySeed result;
// Path and Slot, when present, are already in the caller's formal vocabulary.
// The target coordinates remain explicit because repeated actuals are
// distinct call-frame wires even when they denote the same caller root.
type formalApplyActual struct {
	targetRoot Root
	targetSlot FormalSlot
	value      product.Value
	slot       FormalSlot
	slotSet    bool
}

// formalApplyActualTuple is the sole per-leaf evaluation of a frozen call
// circuit.  Every downstream Apply law (identity image, reachability seeds,
// structural transport and contracts) consumes this immutable tuple; no
// group or axis is permitted to reevaluate caller syntax.
type formalApplyActualTuple struct {
	binding relationLazyApplyBinding
	// values is frame input-root order: structural call inputs followed by the
	// complete ambient circuit. Identity substitution consumes only the
	// structural prefix; output transport also consumes mutable ambients.
	values []formalApplyActual
}

// formalApplyCorrelatedRegion is one shared-DD leaf of the caller predecessor
// and the guard-composed stabilized callee outcome.  There is no Cartesian
// product of separately enumerated caller/callee leaves: both evaluators are
// cut from the same partition row and therefore share exactly one Care.
type formalApplyCorrelatedRegion struct {
	guard       decisionRef
	caller      formalTupleLeafEvaluator
	target      formalTupleLeafEvaluator
	occurrences []formalQualifiedOutcomeOccurrence
}

// formalApplyCorrelatedRegions alpha-renames and closes every callee outcome
// through the frozen Apply guard boundary, joins them in target ownership, and
// partitions caller+target roots once in the forest-global DD.  This is the
// sole correlation seam for Apply; later factor laws operate only on the two
// leaf evaluators and the active occurrence vocabulary returned here.
func (a *formalTupleAlgebra) formalApplyCorrelatedRegions(
	operator formalRelationOperatorRef,
	predecessor formalRelationTuple,
	outcomes []formalRelationTuple,
) ([]formalApplyCorrelatedRegion, error) {
	regions, err := a.formalApplyCorrelatedTargetRegions(operator, predecessor, outcomes)
	if err != nil {
		return nil, err
	}
	for _, region := range regions {
		if len(region.occurrences) == 0 {
			return nil, fmt.Errorf("transformer: formal Apply live callee region has no outcome occurrence")
		}
	}
	return regions, nil
}

// formalApplyCorrelatedTargetRegions is the neutral caller+target correlation
// primitive shared by terminal Apply and point publication. It alpha-renames
// target guards and cuts both complete tuples under one Care, but deliberately
// imposes no terminal-occurrence requirement: an internal point is live before
// any N5 outcome exists.
func (a *formalTupleAlgebra) formalApplyCorrelatedTargetRegions(
	operator formalRelationOperatorRef,
	predecessor formalRelationTuple,
	targets []formalRelationTuple,
) ([]formalApplyCorrelatedRegion, error) {
	if a == nil || a.program == nil || a.program.formalGuards == nil ||
		operator.kind != formalRelationCellStep || operator.root == 0 || operator.step == 0 ||
		predecessor.variable == 0 || len(targets) == 0 {
		return nil, fmt.Errorf("transformer: formal Apply correlated partition is unowned")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return nil, err
	}
	site := formalRelationCell{
		Variable: predecessor.variable, Root: operator.root,
		Step: operator.step, Kind: formalRelationCellStep,
	}
	boundary, ok := a.program.formalGuards.applyBoundary(site)
	if !ok {
		return nil, fmt.Errorf("transformer: formal Apply has no frozen guard boundary")
	}
	step, stepOK := formalRelationStepOperator(operator)
	if !stepOK || step.kind != boundaryStepApply || step.apply.variable == 0 {
		return nil, fmt.Errorf("transformer: formal Apply correlated partition has no target")
	}
	callerSpan, callerDirectory, _, callerOK := a.span(predecessor.variable)
	if !callerOK || predecessor.root.owner != callerDirectory {
		return nil, errFormalComponentForeignOwner
	}
	callerCare, err := a.care(predecessor)
	if err != nil {
		return nil, err
	}
	callerRoots := make([]decisionRef, callerSpan.count)
	for ordinal := 0; ordinal < callerSpan.count; ordinal++ {
		value, readErr := callerDirectory.valueAt(predecessor.root, formalFiberOrdinal(ordinal))
		if readErr != nil {
			return nil, readErr
		}
		callerRoots[ordinal] = decisionRef(value)
	}
	// Outcomes are semantic alternatives, not product operands. Joining their
	// complete tuples here manufactures a product spelling that no Outcome
	// producer published (and can destroy return correlation). Compose and cut
	// each producer-owned alternative against the caller independently; final
	// Apply publication performs the one guarded lattice join in caller space.
	regions := make([]formalApplyCorrelatedRegion, 0)
	for index, candidate := range targets {
		if err := a.validateTuple(candidate); err != nil {
			return nil, err
		}
		if candidate.bottom() || candidate.variable != step.apply.variable {
			return nil, fmt.Errorf("transformer: formal Apply target %d has foreign ownership", index)
		}
		target, composeErr := a.composeGuardBoundary(candidate, boundary)
		if composeErr != nil {
			return nil, composeErr
		}
		if target.bottom() {
			continue
		}
		targetSpan, targetDirectory, targetAuthority, targetOK := a.span(target.variable)
		if !targetOK || target.root.owner != targetDirectory {
			return nil, errFormalComponentForeignOwner
		}
		targetCare, careErr := a.care(target)
		if careErr != nil {
			return nil, careErr
		}
		jointCare, jointErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, callerCare, targetCare, decisionLeafAnd)
		if jointErr != nil {
			return nil, jointErr
		}
		if jointCare == decisionFalse {
			continue
		}
		roots := make([]decisionRef, callerSpan.count+targetSpan.count)
		copy(roots, callerRoots)
		for ordinal := 0; ordinal < targetSpan.count; ordinal++ {
			value, readErr := targetDirectory.valueAt(target.root, formalFiberOrdinal(ordinal))
			if readErr != nil {
				return nil, readErr
			}
			roots[callerSpan.count+ordinal] = decisionRef(value)
		}
		rows, partitionErr := a.decisions.partitionLeafTuplesUnderCare(a.ctx, jointCare, roots)
		if partitionErr != nil {
			return nil, partitionErr
		}
		descriptors := targetSpan.forest.descriptors[targetSpan.first : targetSpan.first+targetSpan.count]
		for _, row := range rows {
			if len(row.leaves) != len(roots) {
				return nil, errDecisionMalformed
			}
			callerEvaluator, evalErr := a.newTupleLeafEvaluator(predecessor.variable, row.leaves[:callerSpan.count], row.care)
			if evalErr != nil {
				return nil, evalErr
			}
			targetEvaluator, evalErr := a.newTupleLeafEvaluator(target.variable, row.leaves[callerSpan.count:], row.care)
			if evalErr != nil {
				return nil, evalErr
			}
			// The correlation partition has produced the exact target product row
			// that all Apply stages consume. Register its complete canonical lane
			// spellings at that producer boundary; consumers keep their fail-closed
			// spelling lookup and do not rebuild a factor from partial leaves.
			if cacheErr := a.cacheFormalTupleLeafEvaluatorFactorSpellings(targetEvaluator); cacheErr != nil {
				return nil, cacheErr
			}
			// The same correlation partition owns the caller half of this exact
			// product row. Apply's factor transaction consumes that half as well,
			// so publish its canonical spellings before the strict lookup.
			if cacheErr := a.cacheFormalTupleLeafEvaluatorFactorSpellings(callerEvaluator); cacheErr != nil {
				return nil, cacheErr
			}
			// Guard composition plus the caller/target Care intersection is itself a
			// product-producing operator law. The narrower correlated region can have
			// a canonical target vector distinct from the callee Outcome's broad-Care
			// spelling, so freeze this exact vector under the Apply operator's already
			// registered coordinate selector before publishing the region. Execution
			// remains a strict lookup and never seals a missing capability lazily.
			if _, capabilityErr := a.internSelectedFormalFactorExecutionCapabilityRef(targetEvaluator, operator.footprint.sourceSelector); capabilityErr != nil {
				return nil, capabilityErr
			}
			occurrences := make([]formalQualifiedOutcomeOccurrence, 0)
			for ordinal, descriptor := range descriptors {
				if descriptor.role != formalFiberOutcome {
					continue
				}
				leaf, present := targetEvaluator.leaves.leaf(formalFiberOrdinal(ordinal))
				if !present {
					return nil, errFormalComponentMalformed
				}
				if leaf == 0 {
					continue
				}
				terminal, terminalErr := targetAuthority.terminal(leaf)
				if terminalErr != nil || terminal.kind != formalComponentOutcomeOccurrence || terminal.outcome.ref != descriptor.outcome {
					if terminalErr != nil {
						return nil, terminalErr
					}
					return nil, errFormalComponentMalformed
				}
				occurrences = append(occurrences, terminal.outcome)
			}
			regions = append(regions, formalApplyCorrelatedRegion{
				guard: row.care, caller: callerEvaluator, target: targetEvaluator, occurrences: occurrences,
			})
		}
	}
	return regions, nil
}

func (t formalApplyActualTuple) valid() bool {
	if !t.binding.validFor(t.binding.caller, t.binding.target, t.binding.frame) ||
		len(t.values) < t.binding.targetShape.InputCount() {
		return false
	}
	for _, value := range t.values {
		if !value.targetSlot.Valid() || value.targetRoot.Kind == 0 ||
			!product.BelongsToRegistry(t.binding.callerArena.reg, value.value) ||
			value.slotSet && !value.slot.Valid() {
			return false
		}
	}
	return true
}

// evaluateFormalApplyActuals performs one complete frozen-lens evaluation in
// target input order.  EntrySeed is the existing prepared contract authority:
// it fills only product Bottom, so a route-supplied actual always wins.  Paths
// and direct-root slots are rekeyed into the caller's formal vocabulary here,
// once, before any registered factor law runs.
func (a *formalTupleAlgebra) evaluateFormalApplyActuals(
	step *formalApplyStep,
	evaluator formalTupleLeafEvaluator,
) (formalApplyActualTuple, error) {
	if step == nil {
		return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply actual tuple has no frozen step")
	}
	binding := step.binding
	if a == nil || a.program == nil || a.program.formalSlots == nil ||
		!binding.validFor(binding.caller, binding.target, binding.frame) ||
		!evaluator.valid() || evaluator.variable != binding.caller ||
		int(binding.caller) > len(a.program.bodies) || int(binding.target) > len(a.program.bodies) {
		return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply actual tuple is unowned")
	}
	caller := &a.program.bodies[binding.caller-1]
	target := &a.program.bodies[binding.target-1]
	callerSpan, callerOK := a.program.formalFibers.span(binding.caller)
	if !callerOK || callerSpan.keys == nil || !callerSpan.keys.Valid() ||
		!target.entrySeedPlan.Valid() || int(binding.frame) >= len(caller.frames) {
		return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply actual tuple has no frozen lens")
	}
	frame := &caller.frames[binding.frame]
	if !frame.valid() || frame.owner != binding.caller || frame.target != binding.target ||
		len(frame.rootCircuit) != binding.targetShape.InputCount() {
		return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply actual tuple differs from its frame")
	}
	capability, err := evaluator.materializeValueFactorAccess(step.valueAccess, step.valueFactorGroups)
	if err != nil {
		return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply value factors: %w", err)
	}

	out := formalApplyActualTuple{binding: binding, values: make([]formalApplyActual, frame.inputRootCount())}
	for index, wire := range frame.rootCircuit {
		valueRef, ok := binding.inputValue(wire.root)
		if !ok {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply input %d has no caller value", index)
		}
		pathRef, pathSet, ok := binding.inputPath(wire.root)
		if !ok {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply input %d has malformed caller path", index)
		}
		actual, err := evaluator.evaluateWithFactorAccess(formalQualifiedBinding{
			value: valueRef, path: pathRef, pathPresent: pathSet,
		}, capability)
		if err != nil {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply input %d value term %d path term %d: %w", index, valueRef.term, pathRef.term, err)
		}
		targetSlot, ok := a.program.formalSlots.Slot(target.body, wire.root)
		if !ok {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply input %d has no target FormalSlot", index)
		}
		actual.value, err = step.seeds.ApplyValue(a.program.registry, targetSlot, actual.value)
		if err != nil {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply input %d EntrySeed: %w", index, err)
		}
		entry := formalApplyActual{targetRoot: wire.root, targetSlot: targetSlot, value: actual.value}
		// Only an exact caller root owns a scalar slot. Arbitrary expressions and
		// aliases retain their value/path tuple without manufacturing storage.
		if wire.destination.hasValueRoot {
			if slot, present := a.program.formalSlots.Slot(caller.body, wire.destination.valueRoot); present {
				entry.slot, entry.slotSet = slot, true
			}
		}
		out.values[index] = entry
	}
	for ambientIndex, wire := range frame.ambientCircuit {
		index := len(frame.rootCircuit) + ambientIndex
		actual, err := evaluator.evaluateWithFactorAccess(formalQualifiedBinding{
			value:       relationArenaValueRef{owner: binding.caller, arena: binding.callerArena, term: wire.value},
			path:        relationArenaPathRef{owner: binding.caller, arena: binding.callerArena, term: wire.path},
			pathPresent: wire.path != 0,
		}, capability)
		if err != nil {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply ambient %d: %w", ambientIndex, err)
		}
		targetSlot, ok := a.program.formalSlots.Slot(target.body, Root{Kind: RootAmbient, Index: uint32(ambientIndex)})
		if !ok {
			return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply ambient %d has no target FormalSlot", ambientIndex)
		}
		entry := formalApplyActual{targetRoot: Root{Kind: RootAmbient, Index: uint32(ambientIndex)}, targetSlot: targetSlot, value: actual.value}
		if slot, present := a.program.formalSlots.Slot(caller.body, Root{Kind: RootAmbient, Index: uint32(ambientIndex)}); present {
			entry.slot, entry.slotSet = slot, true
		}
		out.values[index] = entry
	}
	if !out.valid() {
		return formalApplyActualTuple{}, fmt.Errorf("transformer: formal Apply actual tuple failed closure")
	}
	return out, nil
}

// formalQualifiedBinding is the sole symbolic Middle payload. Value and its
// optional address are one immutable alternative; collecting joins therefore
// cannot manufacture a value/path cross-product. Both references remain in
// their sealed lexical arena and Apply never imports either DAG.
type formalQualifiedBinding struct {
	value       relationArenaValueRef
	path        relationArenaPathRef
	pathPresent bool
	// scope is the lexical mu lifetime of an arbitrary expression. Direct IN
	// roots use zero; borrowed target expressions retain their Outcome site.
	scope loopMuTerm
	// apply is non-zero only when value/path are a borrowed target-arena
	// expression under one exact call frame.  Keeping the environment beside
	// the two correlated terms makes structural Apply a view, never a DAG copy.
	apply formalApplyTermView
}

func (b formalQualifiedBinding) fingerprint() uint64 {
	hash := formalComponentMix(uint64(b.value.term), uint64(b.value.owner))
	if b.pathPresent {
		hash = formalComponentMix(hash, uint64(b.path.term))
		hash = formalComponentMix(hash, 1)
	}
	hash = formalComponentMix(hash, uint64(b.scope))
	if b.apply.present() {
		hash = formalComponentMix(hash, uint64(b.apply.binding.caller))
		hash = formalComponentMix(hash, uint64(b.apply.binding.target))
		hash = formalComponentMix(hash, uint64(b.apply.binding.frame))
		hash = formalComponentMix(hash, uint64(b.apply.callerScope))
		hash = formalComponentMix(hash, 2)
	}
	return hash
}

func formalQualifiedBindingLess(left, right formalQualifiedBinding) bool {
	if left.apply.present() != right.apply.present() {
		return !left.apply.present()
	}
	if left.apply.present() {
		if left.apply.binding.caller != right.apply.binding.caller {
			return left.apply.binding.caller < right.apply.binding.caller
		}
		if left.apply.binding.target != right.apply.binding.target {
			return left.apply.binding.target < right.apply.binding.target
		}
		if left.apply.binding.frame != right.apply.binding.frame {
			return left.apply.binding.frame < right.apply.binding.frame
		}
		if left.apply.callerScope != right.apply.callerScope {
			return left.apply.callerScope < right.apply.callerScope
		}
	}
	if left.value.owner != right.value.owner {
		return left.value.owner < right.value.owner
	}
	if left.value.term != right.value.term {
		return left.value.term < right.value.term
	}
	if left.pathPresent != right.pathPresent {
		return !left.pathPresent
	}
	if left.scope != right.scope {
		return left.scope < right.scope
	}
	return left.path.term < right.path.term
}

func formalQualifiedBindingsEqual(left, right []formalQualifiedBinding) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func compactFormalQualifiedBindings(bindings []formalQualifiedBinding) []formalQualifiedBinding {
	if len(bindings) == 0 {
		return bindings
	}
	out := bindings[:1]
	for _, binding := range bindings[1:] {
		if binding != out[len(out)-1] {
			out = append(out, binding)
		}
	}
	return out
}

func formalQualifiedBindingsSubset(left, right []formalQualifiedBinding) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		switch {
		case left[leftIndex] == right[rightIndex]:
			leftIndex++
			rightIndex++
		case formalQualifiedBindingLess(right[rightIndex], left[leftIndex]):
			rightIndex++
		default:
			return false
		}
	}
	return leftIndex == len(left)
}

func unionFormalQualifiedBindings(left, right []formalQualifiedBinding) []formalQualifiedBinding {
	out := make([]formalQualifiedBinding, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		switch {
		case left[leftIndex] == right[rightIndex]:
			out = append(out, left[leftIndex])
			leftIndex++
			rightIndex++
		case formalQualifiedBindingLess(left[leftIndex], right[rightIndex]):
			out = append(out, left[leftIndex])
			leftIndex++
		default:
			out = append(out, right[rightIndex])
			rightIndex++
		}
	}
	out = append(out, left[leftIndex:]...)
	out = append(out, right[rightIndex:]...)
	return out
}

// applyOutcome executes one complete formal boundary transaction over the
// caller predecessor and stabilized callee outcomes. Every registered factor,
// result scalar and symbolic result binding is prepared in scratch and the
// caller directory is published once; there is no return-only side path.
func (a *formalTupleAlgebra) applyOutcome(operator formalRelationOperatorRef, predecessor formalRelationTuple, outcomes []formalRelationTuple) (formalRelationTuple, formalApplyObservation, error) {
	if a == nil || operator.kind != formalRelationCellStep || operator.code == nil || operator.root == 0 ||
		int(operator.root) >= len(operator.code.nodes) || operator.step == 0 || int(operator.step) > len(operator.code.nodes[operator.root].steps) ||
		!operator.apply.validFor(a.program, predecessor.variable) ||
		!operator.apply.validForFootprint(a.program, operator.footprint) {
		return formalRelationTuple{}, formalApplyObservation{}, fmt.Errorf("transformer: formal Apply operator is malformed")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		if err != nil {
			return formalRelationTuple{}, formalApplyObservation{}, fmt.Errorf("transformer: formal Apply predecessor validation: %w", err)
		}
		return formalRelationTuple{}, formalApplyObservation{}, fmt.Errorf("transformer: formal Apply predecessor is Bottom")
	}
	_, _, callerAuthority, callerOK := a.span(predecessor.variable)
	if !callerOK || callerAuthority.code != operator.code || operator.apply.owner != predecessor.variable {
		return formalRelationTuple{}, formalApplyObservation{}, fmt.Errorf("transformer: formal Apply predecessor has foreign ownership")
	}
	regions, err := a.formalApplyCorrelatedRegions(operator, predecessor, outcomes)
	if err != nil {
		return formalRelationTuple{}, formalApplyObservation{}, fmt.Errorf("transformer: formal Apply correlation: %w", err)
	}
	publications := make([]formalApplyLeafPublication, 0, len(regions))
	observed := make([]formalApplyObservedRegion, 0, len(regions))
	for _, region := range regions {
		publication, reachable, regionErr := a.formalApplyRegionPublication(operator.apply, operator.footprint, region, formalApplyTerminalNormal)
		if regionErr != nil {
			return formalRelationTuple{}, formalApplyObservation{}, fmt.Errorf("transformer: formal Apply region publication: %w", regionErr)
		}
		if reachable {
			publications = append(publications, publication)
			observed = append(observed, formalApplyObservedRegion{region: region, publication: publication})
		}
	}
	observation := formalApplyObservation{step: operator.apply, regions: observed}
	if len(publications) == 0 {
		return formalRelationTuple{}, observation, nil
	}
	result, err := a.publishFormalApplyLeafPublications(predecessor, publications)
	if err != nil {
		err = fmt.Errorf("transformer: formal Apply caller publication: %w", err)
	}
	return result, observation, err
}

// formalApplyIdentityAuthority builds the simultaneous identity image of one
// frozen call frame from exact caller actuals.  It is deliberately stated for
// the complete target input circuit: duplicate actuals remain distinct wires
// whose images coincide (f(x,x)), while the target formal variables never
// acquire caller-owned syntax.
func (a *formalTupleAlgebra) formalApplyIdentityAuthority(
	binding relationLazyApplyBinding,
	actuals formalApplyActualTuple,
) (*state.IdentitySubstitutionAuthority, error) {
	if a == nil || a.program == nil || a.program.formalSlots == nil ||
		!binding.validFor(binding.caller, binding.target, binding.frame) ||
		!actuals.valid() || actuals.binding != binding ||
		int(binding.caller) > len(a.program.bodies) {
		return nil, fmt.Errorf("transformer: formal Apply identity authority is unowned")
	}
	caller := &a.program.bodies[binding.caller-1]
	if int(binding.frame) >= len(caller.frames) {
		return nil, fmt.Errorf("transformer: formal Apply identity frame is undeclared")
	}
	frame := &caller.frames[binding.frame]
	if !frame.valid() || frame.owner != binding.caller || frame.target != binding.target ||
		frame.term != binding.frame || len(frame.rootCircuit) != binding.targetShape.InputCount() {
		return nil, fmt.Errorf("transformer: formal Apply identity frame differs from its binding")
	}
	bindings := make([]identity.Binding, len(frame.rootCircuit))
	for index, actual := range actuals.values[:len(frame.rootCircuit)] {
		if index >= len(frame.rootCircuit) || actual.targetRoot != frame.rootCircuit[index].root {
			return nil, fmt.Errorf("transformer: formal Apply identity input %d differs from its frozen wire", index)
		}
		root, ok := actual.targetSlot.Root()
		if !ok {
			return nil, fmt.Errorf("transformer: formal Apply identity input %d has no target formal root", index)
		}
		bindings[index] = identity.Binding{
			Variable: identity.NewFormalVarRoot(root),
			Image:    product.Get(a.program.registry, actual.value, identity.Key),
		}
	}
	substitution, ok := identity.NewSubstitution(bindings)
	if !ok {
		return nil, fmt.Errorf("transformer: formal Apply identity substitution is not a function")
	}
	return state.NewIdentitySubstitutionAuthority(substitution, frame.allocations), nil
}

// formalApplyInputBinding retains a sealed target expression as an immutable
// evaluation view. Direct input roots must retain the view as well: their
// caller term supplies the actual while the target Middle fiber supplies the
// callee-local constraint accumulated by guards. Apply publication can still
// recover the exact caller alias/path from the frozen input wire.
func formalApplyInputBinding(binding relationLazyApplyBinding, source ValueTerm, sourcePath PathTerm, targetScope loopMuTerm) (formalQualifiedBinding, error) {
	if source == 0 || int(source) >= len(binding.targetArena.values) || binding.callerCode == nil ||
		int(binding.frame) >= len(binding.callerCode.applicationGuards) ||
		!binding.callerCode.applicationGuards[binding.frame].validFor(binding.frame, binding.target) {
		return formalQualifiedBinding{}, errFormalComponentForeignOwner
	}
	plan := &binding.callerCode.applicationGuards[binding.frame]
	if _, owned := plan.targetScopes[targetScope]; !owned {
		return formalQualifiedBinding{}, errFormalComponentForeignOwner
	}
	view, err := newFormalApplyTermView(binding, source, sourcePath, plan.callerScope)
	if err != nil {
		return formalQualifiedBinding{}, err
	}
	return view.bind(source, sourcePath, targetScope), nil
}

func (a *formalTupleAlgebra) applyResultDescriptor(caller relationVar, point cfg.Point, result uint32) (formalFiberDescriptor, error) {
	if a == nil || a.program == nil || a.program.formalSlots == nil || caller == 0 || int(caller) > len(a.program.bodies) {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal Apply result descriptor is unowned")
	}
	body := &a.program.bodies[caller-1]
	root, ok := body.relation.arena.middleRoot(statekey.CallResult(uint32(point), result))
	if !ok {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal Apply result has no Middle register")
	}
	slot, ok := a.program.formalSlots.Slot(body.body, root)
	if !ok {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal Apply result has no FormalSlot")
	}
	span, _, _, ok := a.span(caller)
	if !ok {
		return formalFiberDescriptor{}, errFormalComponentForeignOwner
	}
	var found formalFiberDescriptor
	for _, descriptor := range span.descriptors() {
		if descriptor.role != formalFiberMiddleValue || descriptor.slot != slot {
			continue
		}
		if found.role != formalFiberInvalid {
			return formalFiberDescriptor{}, fmt.Errorf("transformer: formal Apply result descriptor is ambiguous")
		}
		found = descriptor
	}
	if found.role == formalFiberInvalid {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal Apply result descriptor is missing")
	}
	return found, nil
}
