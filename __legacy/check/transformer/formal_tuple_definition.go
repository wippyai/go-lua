package transformer

import (
	"fmt"
	"time"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalDefinitionTransaction is the frozen declaration-boundary law. A
// definition is not a call: its result-free linked frame binds one symbolic
// target transformer to an owner publication/resource world. DefinitionSeed
// and DefinitionOutcome are therefore operands of this one transaction, not
// independently executable influences.
type formalDefinitionTransaction struct {
	ref          formalRelationDefinitionRef
	boundary     formalGuardBoundary
	apply        formalApplyStep
	seedCount    int
	outcomeCount int
}

func (t *formalDefinitionTransaction) validFor(program *RelationProgram, operator formalRelationOperatorRef) bool {
	if t == nil || program == nil || operator.kind != formalRelationCellDefinition ||
		operator.region != program.formalRegion || operator.definition == 0 ||
		t.ref != operator.definition || int(t.ref) >= len(operator.region.definitions) ||
		!t.boundary.valid() || t.boundary.owner != program.formalGuards {
		return false
	}
	definition := operator.region.definitions[t.ref]
	return definition.cell.valid() && definition.cell.Kind == formalRelationCellDefinition &&
		definition.owner == t.apply.owner && definition.target == t.apply.target &&
		definition.frame == t.apply.frame && t.apply.validFor(program, definition.owner) &&
		t.apply.validForFootprint(program, operator.footprint) &&
		t.seedCount > 0 && t.outcomeCount >= 0 &&
		len(t.apply.linked.resultSelectors) == 0 &&
		len(t.apply.linked.resultSources) == 0
}

func freezeFormalDefinitionTransaction(program *RelationProgram, operator formalRelationOperatorRef) (*formalDefinitionTransaction, error) {
	if program == nil || program.formalRegion == nil || program.formalGuards == nil ||
		operator.kind != formalRelationCellDefinition || operator.region != program.formalRegion ||
		operator.definition == 0 || int(operator.definition) >= len(operator.region.definitions) {
		return nil, fmt.Errorf("definition transaction is unowned")
	}
	definition := operator.region.definitions[operator.definition]
	if definition.owner == 0 || int(definition.owner) > len(program.bodies) ||
		definition.target == 0 || int(definition.target) > len(program.bodies) || definition.frame == 0 {
		return nil, fmt.Errorf("definition transaction has a foreign boundary")
	}
	owner := &program.bodies[definition.owner-1]
	if int(definition.frame) >= len(owner.frames) || int(definition.frame) >= len(owner.relation.code.applicationGuards) {
		return nil, fmt.Errorf("definition transaction frame is undeclared")
	}
	linked := &owner.frames[definition.frame]
	guard := owner.relation.code.applicationGuards[definition.frame]
	if !guard.definition || !guard.validFor(definition.frame, definition.target) ||
		linked.point != definition.point ||
		len(linked.resultSelectors) != 0 || len(linked.resultSources) != 0 {
		return nil, fmt.Errorf("definition transaction is not a result-free frozen boundary (guard=%t point=%d/%d selectors=%d sources=%d)",
			guard.definition && guard.validFor(definition.frame, definition.target), linked.point, definition.point,
			len(linked.resultSelectors), len(linked.resultSources))
	}
	target := &program.bodies[definition.target-1]
	seeds, err := state.BindEntrySeedFactorPlan(program.registry, target.entrySeedPlan, func(slot statekey.Value) (FormalSlot, bool) {
		return formalInitialValueSlot(program.formalFibers.slots, target, slot)
	})
	if err != nil {
		return nil, fmt.Errorf("definition EntrySeed rekey: %w", err)
	}
	boundary, ok := program.formalGuards.definitionBoundary(operator.definition)
	if !ok {
		return nil, fmt.Errorf("definition transaction has no frozen guard boundary")
	}
	transaction := &formalDefinitionTransaction{
		ref: operator.definition, boundary: boundary,
		apply: formalApplyStep{
			owner: definition.owner, target: definition.target, frame: definition.frame,
			binding: guard.binding, linked: linked, seeds: seeds,
		},
	}
	terms := make([]ValueTerm, 0, len(linked.rootCircuit)+len(linked.ambientCircuit))
	for _, wire := range linked.rootCircuit {
		terms = append(terms, wire.value)
	}
	for _, wire := range linked.ambientCircuit {
		terms = append(terms, wire.value)
	}
	transaction.apply.valueAccess, transaction.apply.valueFactorGroups, err = freezeFormalValueFactorAccess(program, definition.owner, terms...)
	if err != nil {
		return nil, fmt.Errorf("definition value access: %w", err)
	}
	for _, influence := range operator.region.incoming[definition.cell] {
		switch influence.Kind {
		case formalRelationInfluenceDefinitionSeed:
			transaction.seedCount++
		case formalRelationInfluenceDefinitionOutcome:
			transaction.outcomeCount++
		}
	}
	if !transaction.validFor(program, operator) {
		return nil, fmt.Errorf("definition transaction failed closure")
	}
	return transaction, nil
}

// evaluateFormalDefinitionEquation interprets the complete definition operand
// row atomically. Multiple owner publications and multiple target outcomes
// join in their own authorities before the cross-boundary transaction; no
// caller-specific target equation state is created.
func evaluateFormalDefinitionEquation(
	algebra *formalTupleAlgebra,
	equation formalRelationEquation,
	read func(formalRelationCell) formalRelationTuple,
) formalRelationTuple {
	transaction := equation.Operator.definitionTransaction
	if algebra == nil || transaction == nil || !transaction.validFor(algebra.program, equation.Operator) {
		algebra.fail(fmt.Errorf("transformer: formal Definition transaction is malformed"))
		return formalRelationTuple{}
	}
	var traceDetail *formalRelationEvalTraceDetail
	if algebra.evalTrace != nil && algebra.evalTrace.active != nil {
		traceDetail = algebra.evalTrace.active
		traceDetail.definitionEquationCalls++
		traceDetail.definitionInputs += len(equation.Inputs)
	}
	var seed formalRelationTuple
	outcomes := make([]formalRelationTuple, 0, len(equation.Inputs))
	seedInputs, outcomeInputs := 0, 0
	for _, input := range equation.Inputs {
		var readStarted time.Time
		var readApplyBefore uint64
		if traceDetail != nil {
			readStarted = time.Now()
			readApplyBefore = algebra.decisions.applyOps
		}
		value := read(input.Source.cell)
		if traceDetail != nil {
			traceDetail.definitionRead.count++
			traceDetail.definitionRead.elapsed += time.Since(readStarted)
			traceDetail.definitionRead.applyOps += algebra.decisions.applyOps - readApplyBefore
		}
		switch input.Influence {
		case formalRelationInfluenceDefinitionSeed:
			seedInputs++
			if value.bottom() {
				continue
			}
			if seed.bottom() {
				seed = value
			} else {
				var joinStarted time.Time
				var joinApplyBefore uint64
				if traceDetail != nil {
					joinStarted = time.Now()
					joinApplyBefore = algebra.decisions.applyOps
				}
				seed = algebra.combine(formalComponentJoin, seed, value)
				if traceDetail != nil {
					traceDetail.definitionSeedJoin.count++
					traceDetail.definitionSeedJoin.elapsed += time.Since(joinStarted)
					traceDetail.definitionSeedJoin.applyOps += algebra.decisions.applyOps - joinApplyBefore
				}
				if err := algebra.err(); err != nil {
					return formalRelationTuple{}
				}
			}
		case formalRelationInfluenceDefinitionOutcome:
			outcomeInputs++
			if !value.bottom() {
				outcomes = append(outcomes, value)
				if traceDetail != nil {
					traceDetail.definitionLiveOutcomes++
				}
			}
		default:
			algebra.fail(fmt.Errorf("transformer: formal Definition has foreign influence %d", input.Influence))
			return formalRelationTuple{}
		}
	}
	if seedInputs != transaction.seedCount || outcomeInputs != transaction.outcomeCount {
		algebra.fail(fmt.Errorf(
			"transformer: formal Definition %d operand row is incomplete (seed=%d/%d outcome=%d/%d total=%d)",
			transaction.ref, seedInputs, transaction.seedCount, outcomeInputs, transaction.outcomeCount, len(equation.Inputs),
		))
		return formalRelationTuple{}
	}
	if seed.bottom() || len(outcomes) == 0 {
		return formalRelationTuple{}
	}
	result, err := algebra.applyDefinition(equation.Operator, transaction, seed, outcomes)
	if err != nil {
		algebra.fail(err)
		return formalRelationTuple{}
	}
	return result
}

// applyDefinition instantiates one already-solved target relation in an owner
// world. The shared linked-frame product transport is identical to Apply, but
// the result-free definition boundary and guard substitution are distinct
// typed authorities; no fake call site or continuation is manufactured.
func (a *formalTupleAlgebra) applyDefinition(
	operator formalRelationOperatorRef,
	transaction *formalDefinitionTransaction,
	seed formalRelationTuple,
	outcomes []formalRelationTuple,
) (formalRelationTuple, error) {
	if a == nil || transaction == nil || !transaction.validFor(a.program, operator) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Definition transaction is unowned")
	}
	var traceDetail *formalRelationEvalTraceDetail
	if a.evalTrace != nil && a.evalTrace.active != nil {
		traceDetail = a.evalTrace.active
	}
	var validateStarted time.Time
	var validateApplyBefore uint64
	if traceDetail != nil {
		validateStarted = time.Now()
		validateApplyBefore = a.decisions.applyOps
	}
	validateErr := a.validateTuple(seed)
	if traceDetail != nil {
		traceDetail.definitionSeedValidate.count++
		traceDetail.definitionSeedValidate.elapsed += time.Since(validateStarted)
		traceDetail.definitionSeedValidate.applyOps += a.decisions.applyOps - validateApplyBefore
	}
	if validateErr != nil || seed.bottom() || seed.variable != transaction.apply.owner {
		if validateErr != nil {
			return formalRelationTuple{}, validateErr
		}
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Definition seed has foreign ownership")
	}
	var correlationStarted time.Time
	var correlationApplyBefore uint64
	if traceDetail != nil {
		correlationStarted = time.Now()
		correlationApplyBefore = a.decisions.applyOps
	}
	regions, err := a.formalDefinitionCorrelatedRegions(operator.footprint, transaction, seed, outcomes)
	if traceDetail != nil {
		traceDetail.definitionCorrelation.count++
		traceDetail.definitionCorrelation.elapsed += time.Since(correlationStarted)
		traceDetail.definitionCorrelation.applyOps += a.decisions.applyOps - correlationApplyBefore
	}
	if err != nil {
		return formalRelationTuple{}, err
	}
	publications := make([]formalApplyLeafPublication, 0, len(regions))
	for _, region := range regions {
		var executeStarted time.Time
		var executeApplyBefore uint64
		if traceDetail != nil {
			executeStarted = time.Now()
			executeApplyBefore = a.decisions.applyOps
		}
		publication, reachable, regionErr := a.formalApplyRegionPublication(&transaction.apply, operator.footprint, region, formalApplyTerminalDefinition)
		if traceDetail != nil {
			traceDetail.definitionExecute.count++
			traceDetail.definitionExecute.elapsed += time.Since(executeStarted)
			traceDetail.definitionExecute.applyOps += a.decisions.applyOps - executeApplyBefore
		}
		if regionErr != nil {
			return formalRelationTuple{}, regionErr
		}
		if reachable {
			publications = append(publications, publication)
		}
	}
	if len(publications) == 0 {
		return formalRelationTuple{}, nil
	}
	var publishStarted time.Time
	var publishApplyBefore uint64
	if traceDetail != nil {
		publishStarted = time.Now()
		publishApplyBefore = a.decisions.applyOps
	}
	result, publishErr := a.publishFormalApplyLeafPublications(seed, publications)
	if traceDetail != nil {
		traceDetail.definitionPublish.count++
		traceDetail.definitionPublish.elapsed += time.Since(publishStarted)
		traceDetail.definitionPublish.applyOps += a.decisions.applyOps - publishApplyBefore
	}
	return result, publishErr
}

// formalDefinitionCorrelatedRegions cuts the owner seed and every guard-bound
// target outcome under one shared DD Care. This is definition BindIn/feedback
// as a relation operation; it does not enumerate or reconstruct State.
func (a *formalTupleAlgebra) formalDefinitionCorrelatedRegions(
	footprint formalOperatorCoordinateFootprint,
	transaction *formalDefinitionTransaction,
	seed formalRelationTuple,
	outcomes []formalRelationTuple,
) ([]formalApplyCorrelatedRegion, error) {
	if a == nil || transaction == nil || len(outcomes) == 0 ||
		seed.variable != transaction.apply.owner || !transaction.boundary.valid() {
		return nil, fmt.Errorf("transformer: formal Definition correlated partition is unowned")
	}
	var traceDetail *formalRelationEvalTraceDetail
	if a.evalTrace != nil && a.evalTrace.active != nil {
		traceDetail = a.evalTrace.active
		traceDetail.definitionCalls++
	}
	var target formalRelationTuple
	for index, candidate := range outcomes {
		var validateStarted time.Time
		var validateApplyBefore uint64
		if traceDetail != nil {
			validateStarted = time.Now()
			validateApplyBefore = a.decisions.applyOps
		}
		validateErr := a.validateTuple(candidate)
		if traceDetail != nil {
			traceDetail.definitionTargetValidate.count++
			traceDetail.definitionTargetValidate.elapsed += time.Since(validateStarted)
			traceDetail.definitionTargetValidate.applyOps += a.decisions.applyOps - validateApplyBefore
		}
		if validateErr != nil {
			return nil, validateErr
		}
		if candidate.bottom() || candidate.variable != transaction.apply.target {
			return nil, fmt.Errorf("transformer: formal Definition target %d has foreign ownership", index)
		}
		var composeStarted time.Time
		var composeApplyBefore uint64
		if traceDetail != nil {
			composeStarted = time.Now()
			composeApplyBefore = a.decisions.applyOps
		}
		composed, err := a.composeGuardBoundary(candidate, transaction.boundary)
		if traceDetail != nil {
			traceDetail.definitionCompose.count++
			traceDetail.definitionCompose.elapsed += time.Since(composeStarted)
			traceDetail.definitionCompose.applyOps += a.decisions.applyOps - composeApplyBefore
		}
		if err != nil {
			return nil, err
		}
		if target.bottom() {
			target = composed
		} else {
			var joinStarted time.Time
			var joinApplyBefore uint64
			if traceDetail != nil {
				joinStarted = time.Now()
				joinApplyBefore = a.decisions.applyOps
			}
			target = a.combine(formalComponentJoin, target, composed)
			if traceDetail != nil {
				traceDetail.definitionTargetJoin.count++
				traceDetail.definitionTargetJoin.elapsed += time.Since(joinStarted)
				traceDetail.definitionTargetJoin.applyOps += a.decisions.applyOps - joinApplyBefore
			}
			if err := a.err(); err != nil {
				return nil, err
			}
		}
	}
	if target.bottom() {
		return nil, nil
	}
	var setupStarted time.Time
	var setupApplyBefore uint64
	if traceDetail != nil {
		setupStarted = time.Now()
		setupApplyBefore = a.decisions.applyOps
	}
	callerSpan, callerDirectory, _, callerOK := a.span(seed.variable)
	targetSpan, targetDirectory, _, targetOK := a.span(target.variable)
	if !callerOK || !targetOK || seed.root.owner != callerDirectory || target.root.owner != targetDirectory {
		return nil, errFormalComponentForeignOwner
	}
	callerCare, err := a.care(seed)
	if err != nil {
		return nil, err
	}
	targetCare, err := a.care(target)
	if err != nil {
		return nil, err
	}
	jointCare, err := a.decisions.apply(a.ctx, uint8(decisionAnd), true, callerCare, targetCare, decisionLeafAnd)
	if err != nil || jointCare == decisionFalse {
		return nil, err
	}
	roots := make([]decisionRef, callerSpan.count+targetSpan.count)
	for ordinal := 0; ordinal < callerSpan.count; ordinal++ {
		value, readErr := callerDirectory.valueAt(seed.root, formalFiberOrdinal(ordinal))
		if readErr != nil {
			return nil, readErr
		}
		roots[ordinal] = decisionRef(value)
	}
	for ordinal := 0; ordinal < targetSpan.count; ordinal++ {
		value, readErr := targetDirectory.valueAt(target.root, formalFiberOrdinal(ordinal))
		if readErr != nil {
			return nil, readErr
		}
		roots[callerSpan.count+ordinal] = decisionRef(value)
	}
	var partitionStarted time.Time
	var partitionApplyBefore uint64
	if traceDetail != nil {
		traceDetail.definitionCallerRoots += callerSpan.count
		traceDetail.definitionTargetRoots += targetSpan.count
		traceRoots := make([]decisionRef, 0, len(roots)+1)
		traceRoots = append(traceRoots, jointCare)
		traceRoots = append(traceRoots, roots...)
		traceDetail.definitionSupportRanks = formalRelationTraceSupportRanks(&a.decisions, traceRoots...)
		traceDetail.definitionCorrelationSetup.count++
		traceDetail.definitionCorrelationSetup.elapsed += time.Since(setupStarted)
		traceDetail.definitionCorrelationSetup.applyOps += a.decisions.applyOps - setupApplyBefore
		partitionStarted = time.Now()
		partitionApplyBefore = a.decisions.applyOps
	}
	rows, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, jointCare, roots)
	if traceDetail != nil {
		traceDetail.definitionPartitionTime += time.Since(partitionStarted)
		traceDetail.definitionPartitionApplyOps += a.decisions.applyOps - partitionApplyBefore
		traceDetail.definitionRows += len(rows)
	}
	if err != nil {
		return nil, err
	}
	regions := make([]formalApplyCorrelatedRegion, 0, len(rows))
	for _, row := range rows {
		if len(row.leaves) != len(roots) {
			return nil, errDecisionMalformed
		}
		caller, evalErr := a.newTupleLeafEvaluator(seed.variable, row.leaves[:callerSpan.count], row.care)
		if evalErr != nil {
			return nil, evalErr
		}
		callee, evalErr := a.newTupleLeafEvaluator(target.variable, row.leaves[callerSpan.count:], row.care)
		if evalErr != nil {
			return nil, evalErr
		}
		// The definition correlation owns this exact target product row. Publish
		// its canonical lane spellings before the shared Apply executor consumes
		// them; that executor remains a strict lookup and never rebuilds a factor
		// from its leaves.
		if cacheErr := a.cacheFormalTupleLeafEvaluatorFactorSpellings(callee); cacheErr != nil {
			return nil, cacheErr
		}
		// Definition owns both halves of its correlated product row. Its shared
		// Apply transaction also consumes the caller half, whose exact lane
		// spellings must therefore be published here rather than reconstructed by
		// that strict consumer.
		if cacheErr := a.cacheFormalTupleLeafEvaluatorFactorSpellings(caller); cacheErr != nil {
			return nil, cacheErr
		}
		// Boundary composition and the joint owner/target Care can produce a
		// canonical product vector that does not occur in the broad-Care Outcome.
		// Freeze that exact row with the Definition operator's sole registered
		// source footprint; execution remains a strict capability lookup.
		var capabilityStarted time.Time
		var capabilityApplyBefore uint64
		if traceDetail != nil {
			traceDetail.definitionCapabilityCount++
			capabilityStarted = time.Now()
			capabilityApplyBefore = a.decisions.applyOps
		}
		_, capabilityErr := a.internSelectedFormalFactorExecutionCapabilityRef(callee, footprint.sourceSelector)
		if traceDetail != nil {
			traceDetail.definitionCapabilityTime += time.Since(capabilityStarted)
			traceDetail.definitionCapabilityApplyOps += a.decisions.applyOps - capabilityApplyBefore
		}
		if capabilityErr != nil {
			return nil, capabilityErr
		}
		regions = append(regions, formalApplyCorrelatedRegion{guard: row.care, caller: caller, target: callee})
	}
	return regions, nil
}
