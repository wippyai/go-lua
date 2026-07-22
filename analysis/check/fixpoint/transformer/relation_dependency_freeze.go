package transformer

import "fmt"

// relationDependencyFreezeResultVersion is intentionally a structural-result
// version, not a cache key. The closure key is carried by demand.
const relationDependencyFreezeResultVersion = "observation-v1"

// relationTier2ArtifactHandle is the immutable hand-off from local syntax and
// SCC/link sealing into tier 3. The slice headers are copied, while their
// elements remain the sealed artifacts produced by tier 2. No tier-3 phase
// writes through either handle.
type relationTier2ArtifactHandle struct {
	syntax []relationBodySyntaxArtifact
	links  []relationSCCLinkArtifact
}

func freezeRelationTier2ArtifactHandle(program *RelationProgram) (relationTier2ArtifactHandle, error) {
	if program == nil || len(program.syntax) == 0 || len(program.syntax) != len(program.links) {
		return relationTier2ArtifactHandle{}, fmt.Errorf("transformer: dependency freeze has no complete tier-2 artifact inventory")
	}
	handle := relationTier2ArtifactHandle{
		syntax: append([]relationBodySyntaxArtifact(nil), program.syntax...),
		links:  append([]relationSCCLinkArtifact(nil), program.links...),
	}
	for index := range handle.syntax {
		if !handle.syntax[index].valid() || !handle.links[index].validFor(handle.syntax[index].body, handle.syntax[index].variable) {
			return relationTier2ArtifactHandle{}, fmt.Errorf("transformer: dependency freeze has malformed tier-2 artifact %d", index+1)
		}
	}
	return handle, nil
}

// relationDependencyEvaluatorBinding names the one evaluator/template pair
// selected by full-result-v1. It carries no entry State: root input is applied
// only by executeFormalRootRelation, after the substitution seam.
type relationDependencyEvaluatorBinding struct {
	program  *RelationProgram
	template *formalRelationTemplate
	coverage observationCoverageGuard
	sealed   bool
}

func (b relationDependencyEvaluatorBinding) validFor(program *RelationProgram) bool {
	return b.sealed && b.program == program && b.template != nil && b.template.validFor(program) && b.coverage.demand.valid()
}

// relationDependencyFreeze is tier 3. It owns the complete dependency closure
// produced from immutable tier-2 artifacts: formal fiber inventory, observable
// quotient, equation template, and its evaluator binding. The RelationProgram
// body view is materialized afresh from the handle only for this transaction;
// no tier-2 artifact is used as a workspace. This product deliberately owns
// neither a workspace nor an entry value.
//
// It is embedded in RelationProgram only to preserve the executor's compact
// field selectors; it is the sole owner of those promoted products.
type relationDependencyFreeze struct {
	version          string
	demand           ObservationContract
	tier2            relationTier2ArtifactHandle
	formalSlots      *SlotSpace
	formalFibers     *formalFiberInventory
	formalComponents *formalComponentTerminalSchema
	formalRegion     *formalRelationRegionInventory
	formalGuards     *formalGuardVocabulary
	formalTemplate   *formalRelationTemplate
	observability    *formalRelationRegionInventory
	evaluator        relationDependencyEvaluatorBinding
	sealed           bool
}

func (d relationDependencyFreeze) validFor(program *RelationProgram) bool {
	return d.sealed && d.version == relationDependencyFreezeResultVersion && program != nil && len(program.bodies) == len(d.tier2.syntax) &&
		d.formalSlots != nil && d.formalFibers != nil && d.formalComponents != nil && d.formalRegion != nil &&
		d.demand.valid() && d.formalGuards != nil && d.formalGuards.valid() && d.formalTemplate != nil &&
		d.observability == d.formalRegion && d.evaluator.validFor(program)
}

// freezeRelationDependencyFreeze is the sole tier-3 construction transaction.
// It receives sealed tier-2 artifacts and seals the exact closure named by
// demand. Summary-v1 retains the same evaluator topology but its publication
// path does not freeze the complete point-state surface.
func freezeRelationDependencyFreeze(program *RelationProgram, demand ObservationContract, telemetry *FreezeTelemetry) error {
	coverage, err := newObservationCoverageGuard(demand)
	if err != nil {
		return err
	}
	handle, err := freezeRelationTier2ArtifactHandle(program)
	if err != nil {
		return err
	}
	program.relationDependencyFreeze = relationDependencyFreeze{
		version: relationDependencyFreezeResultVersion,
		demand:  demand,
		tier2:   handle,
	}
	program.bodies = make([]relationProgramBody, len(handle.syntax))
	for index := range handle.syntax {
		body, bodyErr := handle.syntax[index].materializeRelationProgramBody(handle.links[index])
		if bodyErr != nil {
			return bodyErr
		}
		program.bodies[index] = body
	}

	regionStarted := telemetry.begin(FreezePhaseRegionWTO)
	formalSlots, err := freezeSlotSpace(program)
	if err != nil {
		return err
	}
	program.formalSlots = formalSlots
	formalRegion, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		return err
	}
	program.formalRegion = formalRegion
	telemetry.end(FreezePhaseRegionWTO, regionStarted)

	formalFibers, err := freezeFormalFiberInventoryWithSlotsTelemetry(program, formalSlots, telemetry)
	if err != nil {
		return err
	}
	program.formalFibers = formalFibers

	quotientStarted := telemetry.begin(FreezePhaseObservableQuotient)
	if err := formalRegion.freezeObservableStepQuotient(program); err != nil {
		return err
	}
	program.observability = formalRegion
	telemetry.end(FreezePhaseObservableQuotient, quotientStarted)

	templateStarted := telemetry.begin(FreezePhaseTemplateBinding)
	formalComponents, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		return err
	}
	program.formalComponents = formalComponents
	formalGuards, err := freezeFormalGuardVocabulary(program)
	if err != nil {
		return err
	}
	if !formalGuards.valid() {
		return fmt.Errorf("transformer: formal guard vocabulary failed ownership validation")
	}
	program.formalGuards = formalGuards
	formalTemplate, err := freezeFormalRelationTemplate(program)
	if err != nil {
		return err
	}
	program.formalTemplate = formalTemplate
	program.evaluator = relationDependencyEvaluatorBinding{program: program, template: formalTemplate, coverage: coverage, sealed: true}
	telemetry.end(FreezePhaseTemplateBinding, templateStarted)

	program.sealed = true
	if !program.relationDependencyFreeze.validFor(program) {
		return fmt.Errorf("transformer: observation dependency freeze failed ownership validation")
	}
	return nil
}
