package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalRelationTemplate is the finite parametric equation syntax of one
// frozen relation forest. It is deliberately not a lattice value: recursive
// equations retain typed cell references and are interpreted later by the
// one WTO executor rather than being unfolded into an expression DAG.
type formalRelationTemplate struct {
	program    *RelationProgram
	region     *formalRelationRegionInventory
	rootInputs []formalRootInputTemplate
	constants  []formalRelationTupleConstant
	equations  []formalRelationEquation
	// applyCells is the canonical WTO-ordered internal Apply observation
	// inventory. Publication iterates this closed set directly; it never scans
	// the complete equation inventory after solving.
	applyCells []formalRelationCellRef
	// scopes is the one freeze-time lexical loop-scope inventory for relation
	// roots. An executor may index it only through an explicit Input source;
	// runtime syntax searches are therefore unnecessary.
	scopes [][]loopMuTerm
	sealed bool
}

// validFor is the O(1) execution-time ownership proof. All equation/operator/
// influence completeness is checked once by freezeFormalRelationTemplate before
// sealed is set; the executor must not re-scan immutable syntax per solve.
func (t *formalRelationTemplate) validFor(program *RelationProgram) bool {
	return t != nil && t.sealed && t.program == program && program != nil &&
		program.formalTemplate == t && program.formalRegion != nil &&
		t.region == program.formalRegion && t.region.plan != nil &&
		len(t.equations) == len(t.region.cells) && len(t.scopes) == len(program.bodies)
}

// formalRelationCellRef is a region-owned, dense reference to one declared
// equation. The region pointer is part of the capability; same-numbered cells
// from independently frozen forests are never interchangeable.
type formalRelationCellRef struct {
	region *formalRelationRegionInventory
	cell   formalRelationCell
	index  int
}

func (r formalRelationCellRef) valid() bool {
	if r.region == nil || r.region.plan == nil || !r.cell.valid() || r.index < 0 || r.index >= len(r.region.cells) || r.region.cells[r.index] != r.cell {
		return false
	}
	index, ok := r.region.plan.CanonicalIndex(r.cell)
	return ok && index == r.index
}

// formalRelationEquation is one immutable equation. Operator is existing
// sealed relation syntax; Inputs are its complete typed incoming influences.
// Backedges are ordinary Inputs, so recursive shape has constant size.
type formalRelationEquation struct {
	Cell              formalRelationCellRef
	Operator          formalRelationOperatorRef
	Inputs            []formalRelationTemplateInput
	Seeds             []formalRelationTupleConstantRef
	ApplyNonreturning []formalApplyNonreturningTransaction
}

// formalApplyNonreturningTransaction is one freeze-time paired call terminal.
// Site owns the already-frozen Apply operator; Predecessor and Target are its
// exact two equation inputs. The executor never groups influences or searches
// relation syntax at runtime.
type formalApplyNonreturningTransaction struct {
	Site        formalRelationCellRef
	Operator    formalRelationOperatorRef
	Predecessor formalRelationCellRef
	Target      formalRelationCellRef
}

type formalRelationTemplateInput struct {
	Source    formalRelationCellRef
	Influence formalRelationInfluenceKind
	ReadPoint cfg.Point
	Site      formalRelationCellRef
}

// formalRelationOperatorRef points into the already-sealed semantic owner. It
// does not duplicate a transaction payload or introduce an executable opcode
// language beside relationCode.
type formalRelationOperatorRef struct {
	kind formalRelationCellKind
	// footprint is the exact pre-schema coordinate declaration retained by
	// this executable operator. The WTO transaction cannot bind without it.
	footprint  formalOperatorCoordinateFootprint
	code       *relationCode
	region     *formalRelationRegionInventory
	root       relationRootRef
	scope      loopMuTerm
	step       uint32
	outcome    boundaryOutcomeRef
	definition formalRelationDefinitionRef
	resource   formalRelationResourceRef
	rootInput  *formalRootInputTemplate
	// apply is the frozen lexical composition transaction for the existing
	// relationCode Apply step.
	apply *formalApplyStep
	// pathReplacement is the frozen edge adapter for the one already-owned
	// ProductDomain transaction. Nil is legal only for every other Step kind;
	// an EffectPathStore without this exact transaction is rejected while the
	// template is frozen.
	pathReplacement *formalPathReplacementStep
	// pathInvalidation binds EffectInvalidatePath directly to the registered
	// path-subtree/descendant factor laws.
	pathInvalidation *formalPathInvalidationStep
	// indexMutation binds the canonical ordered dynamic-index transaction to
	// its registered product-factor laws. It is the sole formal execution path
	// for the existing relationCode EffectIndexMutation node.
	indexMutation *formalIndexMutationStep
	// allocationTemplate binds EffectAllocationTemplate directly to the shared
	// symbolic object-graph join transaction.
	allocationTemplate *formalAllocationTemplateStep
	// effectAccess is the canonical ProductDomain operation's sealed read/write
	// authority. EffectCatalog owns syntax admission only. Groups retain complete
	// registered lane correlation; ordinals are only the already-lowered sparse
	// DD projection.
	effectAccess        state.TransferAccess
	effectGroups        []formalFiberGroupDescriptor
	effectReadOrdinals  []formalFiberOrdinal
	effectWriteOrdinals []formalFiberOrdinal
	effectLift          formalClosedFactorLift
	// objectMaterialization binds EffectObjectMaterialization directly to the
	// registered object-graph factor law.
	objectMaterialization *formalObjectMaterializationStep
	// environmentWrite is the exact Values-group point transaction for one
	// existing relationCode EnvironmentWrite step. It retains only checked
	// group/slot capabilities and qualified sealed term syntax.
	environmentWrite *formalEnvironmentWriteStep
	// channelSelect is the frozen factor-native adapter for the existing ordered
	// N3 transaction. Concrete and formal execution share its sole evaluator.
	channelSelect *formalChannelSelectStep
	// branchRelations is the frozen dense factor program for the canonical E3
	// transaction. It owns no State adapter: formal leaves bind its registered
	// Values/lane/coordinate roles directly.
	branchRelations *formalBranchRelationsStep
	// callResults is the exact formal carrier binding of canonical N3.
	callResults *formalCallResultsStep
	// presenceImplications binds canonical N2 publication and closure directly
	// to formal Values and registered path-evidence factors.
	presenceImplications *formalPresenceImplicationStep
	// genericFor is the frozen binding of the one registered sparse factor
	// transaction. It is nil for every non-GenericFor Step.
	genericFor *formalGenericForStep
	// rootAssignment is the exact sparse binding of canonical N4. It owns no
	// transfer semantics; every phase delegates to RootAssignmentFactorProgram.
	rootAssignment *formalRootAssignmentStep
	// covariantExposure is the frozen exact sparse carrier binding for the one
	// canonical N6 factor law. Point-entry and current roles remain distinct.
	covariantExposure *formalCovariantExposureStep
	// contribution is the frozen immutable diagnostic-prefix transaction. Its
	// non-recursive syntax is retained for route-free final publication.
	contribution *formalContributionStep
	// externalCall is the one site-specialized provider and factor transaction.
	// It can only be frozen after Inputs are sealed because published reads are
	// part of the provider's exact operand row.
	externalCall *formalExternalCallStep
	// outcomeTransaction is the sole frozen complete-product N5 terminal.
	// Occurrence publication follows its stabilized factor result.
	outcomeTransaction *formalOutcomeStep
	// definitionTransaction is the sole typed declaration-boundary equation.
	// It pairs DefinitionSeed and DefinitionOutcome; neither influence is an
	// independently executable transfer.
	definitionTransaction *formalDefinitionTransaction
	// resourceTransaction is the sole recursive lexical-resource equation. It
	// joins owner Outcomes and Definition feedback inside their already-shared
	// owner product; no Apply frame or cross-keyspace transport is involved.
	resourceTransaction *formalResourceTransaction
	// stepCapability is the one freeze-time operator dispatch. Executors never
	// recover Step kind from relationCode syntax, and therefore cannot grow a
	// second per-input interpretation of the relation language.
	stepCapability formalRelationStepCapability
}

type formalRelationStepCapability uint8

const (
	formalRelationStepCapabilityInvalid formalRelationStepCapability = iota
	formalRelationStepCapabilityApply
	formalRelationStepCapabilityPathReplacement
	formalRelationStepCapabilityPathInvalidation
	formalRelationStepCapabilityIndexMutation
	formalRelationStepCapabilityAllocationTemplate
	formalRelationStepCapabilityObjectMaterialization
	formalRelationStepCapabilityEnvironmentWrite
	formalRelationStepCapabilityChannelSelect
	formalRelationStepCapabilityBranchRelations
	formalRelationStepCapabilityCallResults
	formalRelationStepCapabilityPresenceImplications
	formalRelationStepCapabilityLoopControl
	formalRelationStepCapabilityGenericFor
	formalRelationStepCapabilityRootAssignment
	formalRelationStepCapabilityCovariantExposure
	formalRelationStepCapabilityContribution
	formalRelationStepCapabilityExternalCall
)

// bindFormalRelationStepCapability closes the finite relationCode Step
// vocabulary over the already-frozen factor adapters. A missing adapter is a
// freeze error: incomplete semantics never become executable relation IR.
func bindFormalRelationStepCapability(operator *formalRelationOperatorRef, step boundaryStep) error {
	if operator == nil || operator.kind != formalRelationCellStep {
		return fmt.Errorf("boundary kind %d has no owned formal operator", step.kind)
	}
	operator.stepCapability = formalRelationStepCapabilityInvalid
	require := func(present bool, capability formalRelationStepCapability, name string) error {
		if !present {
			return fmt.Errorf("boundary kind %d has no complete %s factor transaction", step.kind, name)
		}
		operator.stepCapability = capability
		return nil
	}
	switch step.kind {
	case boundaryStepEffect:
		if operator.code == nil || operator.code.effects == nil {
			return fmt.Errorf("Effect boundary has no effect arena")
		}
		kind := operator.code.effects.Kind(step.effect)
		descriptor, registered := DefaultEffectCatalog().Descriptor(kind)
		if !registered || descriptor.Kind() != kind {
			return fmt.Errorf("Effect boundary kind %d has no registered descriptor", kind)
		}
		switch kind {
		case EffectInvalidatePath:
			return require(operator.pathInvalidation != nil, formalRelationStepCapabilityPathInvalidation, "path invalidation")
		case EffectIndexMutation:
			return require(operator.indexMutation != nil, formalRelationStepCapabilityIndexMutation, "index mutation")
		case EffectAllocationTemplate:
			return require(operator.allocationTemplate != nil, formalRelationStepCapabilityAllocationTemplate, "allocation template")
		case EffectObjectMaterialization:
			return require(operator.objectMaterialization != nil, formalRelationStepCapabilityObjectMaterialization, "object materialization")
		case EffectPathStore:
			if operator.pathReplacement == nil {
				node := operator.code.effects.nodes[step.effect]
				return fmt.Errorf(
					"EffectPathStore shape assignment=%t static=%t heaps=%d entries=%d list-floor=%d has no complete formal factor transaction",
					node.pathStoreHasAssignment, node.pathStoreHasStatic, len(node.pathStoreObject.Heaps), len(node.pathStoreObject.Entries), node.pathStoreObject.ListFloor,
				)
			}
			operator.stepCapability = formalRelationStepCapabilityPathReplacement
			return nil
		default:
			return fmt.Errorf("Effect boundary kind %d has no formal capability", kind)
		}
	case boundaryStepApply:
		return require(operator.apply != nil, formalRelationStepCapabilityApply, "Apply")
	case boundaryStepExternalCall:
		return require(operator.externalCall != nil, formalRelationStepCapabilityExternalCall, "ExternalCall")
	case boundaryStepRootAssignment:
		return require(operator.rootAssignment != nil, formalRelationStepCapabilityRootAssignment, "RootAssignment")
	case boundaryStepEnvironmentWrite:
		return require(operator.environmentWrite != nil, formalRelationStepCapabilityEnvironmentWrite, "EnvironmentWrite")
	case boundaryStepGenericFor:
		return require(operator.genericFor != nil, formalRelationStepCapabilityGenericFor, "GenericFor")
	case boundaryStepContribution:
		return require(operator.contribution != nil, formalRelationStepCapabilityContribution, "Contribution")
	case boundaryStepLoopFeedback, boundaryStepLoopExit:
		// The Step is the exact identity source consumed by the typed control
		// influence. Feedback closure and exit preservation remain solely owned
		// by evaluateFormalControlInput.
		operator.stepCapability = formalRelationStepCapabilityLoopControl
		return nil
	case boundaryStepBranchRelations:
		return require(operator.branchRelations != nil, formalRelationStepCapabilityBranchRelations, "BranchRelations")
	case boundaryStepCallResults:
		return require(operator.callResults != nil, formalRelationStepCapabilityCallResults, "CallResults")
	case boundaryStepPresenceImplications:
		return require(operator.presenceImplications != nil, formalRelationStepCapabilityPresenceImplications, "PresenceImplications")
	case boundaryStepChannelSelect:
		return require(operator.channelSelect != nil, formalRelationStepCapabilityChannelSelect, "ChannelSelect")
	case boundaryStepCovariantExposure:
		return require(operator.covariantExposure != nil, formalRelationStepCapabilityCovariantExposure, "CovariantExposure")
	case boundaryStepInvalid:
		return fmt.Errorf("invalid boundary Step has no formal capability")
	default:
		return fmt.Errorf("boundary kind %d is outside the sealed Step vocabulary", step.kind)
	}
}

func freezeFormalRelationTemplate(program *RelationProgram) (*formalRelationTemplate, error) {
	if program == nil || program.formalRegion == nil || program.formalRegion.plan == nil ||
		!program.formalRegion.plan.Matches(program.formalRegion.cells) {
		return nil, fmt.Errorf("transformer: formal relation template has no sealed region")
	}
	region := program.formalRegion
	rootInputs, err := freezeFormalRootInputTemplates(program)
	if err != nil {
		return nil, err
	}
	template := &formalRelationTemplate{
		program: program, region: region, rootInputs: rootInputs,
		equations: make([]formalRelationEquation, len(region.cells)),
		scopes:    make([][]loopMuTerm, len(program.bodies)),
	}
	for bodyIndex := range program.bodies {
		code := program.bodies[bodyIndex].relation.code
		scopes, _, scopeErr := formalGuardLexicalScopes(code)
		if scopeErr != nil {
			return nil, fmt.Errorf("transformer: formal relation template member %d lexical scopes: %w", bodyIndex+1, scopeErr)
		}
		template.scopes[bodyIndex] = scopes
	}
	// Freeze every operator before accepting any influence. WTO order is not a
	// topological order inside recursive components, so a source operator may
	// occur after its target equation in the canonical inventory.
	for index, cell := range region.cells {
		canonical, ok := region.plan.CanonicalIndex(cell)
		if !ok || canonical != index {
			return nil, fmt.Errorf("transformer: formal relation template cell %d is outside WTO order", index)
		}
		cellRef := formalRelationCellRef{region: region, cell: cell, index: index}
		operator, err := freezeFormalRelationOperator(program, region, rootInputs, cell)
		if err != nil {
			return nil, fmt.Errorf("transformer: formal relation template cell %+v: %w", cell, err)
		}
		if cell.Kind == formalRelationCellOutcome {
			site, siteErr := uniqueFormalOutcomeOccurrenceSite(operator.code, template.scopes[cell.Variable-1], cell.Outcome)
			if siteErr != nil {
				return nil, fmt.Errorf("transformer: formal relation Outcome operator: %w", siteErr)
			}
			operator.root, operator.scope = site.root, site.scope
			outcomeTransaction, freezeErr := freezeFormalOutcomeStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation Outcome N5 operator: %w", freezeErr)
			}
			operator.outcomeTransaction = outcomeTransaction
		} else if cell.Kind == formalRelationCellDefinition {
			definitionTransaction, freezeErr := freezeFormalDefinitionTransaction(program, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation Definition operator: %w", freezeErr)
			}
			operator.definitionTransaction = definitionTransaction
		} else if cell.Kind == formalRelationCellResource {
			resourceTransaction, freezeErr := freezeFormalResourceTransaction(program, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation Resource operator: %w", freezeErr)
			}
			operator.resourceTransaction = resourceTransaction
		} else if cell.Root != 0 {
			scopes := template.scopes[cell.Variable-1]
			if int(cell.Root) >= len(scopes) {
				return nil, fmt.Errorf("transformer: formal relation operator root has no lexical scope")
			}
			operator.scope = scopes[cell.Root]
		}
		if cell.Kind == formalRelationCellStep {
			step, stepOK := formalRelationStepOperator(operator)
			if !stepOK {
				return nil, fmt.Errorf("transformer: formal relation Step operator is malformed")
			}
			pathReplacement, freezeErr := freezeFormalPathReplacementStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation Effect operator: %w", freezeErr)
			}
			operator.pathReplacement = pathReplacement
			pathInvalidation, freezeErr := freezeFormalPathInvalidationStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation PathInvalidation operator: %w", freezeErr)
			}
			operator.pathInvalidation = pathInvalidation
			indexMutation, freezeErr := freezeFormalIndexMutationStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation IndexMutation operator: %w", freezeErr)
			}
			operator.indexMutation = indexMutation
			allocationTemplate, freezeErr := freezeFormalAllocationTemplateStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation AllocationTemplate operator: %w", freezeErr)
			}
			operator.allocationTemplate = allocationTemplate
			objectMaterialization, freezeErr := freezeFormalObjectMaterializationStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation ObjectMaterialization operator: %w", freezeErr)
			}
			operator.objectMaterialization = objectMaterialization
			apply, freezeErr := freezeFormalApplyStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal Apply operator: %w", freezeErr)
			}
			operator.apply = apply
			environmentWrite, freezeErr := freezeFormalEnvironmentWriteStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal relation EnvironmentWrite operator: %w", freezeErr)
			}
			operator.environmentWrite = environmentWrite
			channelSelect, freezeErr := freezeFormalChannelSelectStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal ChannelSelect operator: %w", freezeErr)
			}
			operator.channelSelect = channelSelect
			branchRelations, freezeErr := freezeFormalBranchRelationsStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal BranchRelations operator: %w", freezeErr)
			}
			operator.branchRelations = branchRelations
			callResults, freezeErr := freezeFormalCallResultsStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal CallResults N3 operator: %w", freezeErr)
			}
			operator.callResults = callResults
			presenceImplications, freezeErr := freezeFormalPresenceImplicationStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal PresenceImplications N2 operator: %w", freezeErr)
			}
			operator.presenceImplications = presenceImplications
			genericFor, freezeErr := freezeFormalGenericForStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal GenericFor operator: %w", freezeErr)
			}
			operator.genericFor = genericFor
			rootAssignment, freezeErr := freezeFormalRootAssignmentStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal RootAssignment N4 operator: %w", freezeErr)
			}
			operator.rootAssignment = rootAssignment
			covariantExposure, freezeErr := freezeFormalCovariantExposureStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal CovariantExposure N6 operator: %w", freezeErr)
			}
			operator.covariantExposure = covariantExposure
			contribution, freezeErr := freezeFormalContributionStep(program, cell.Variable, operator)
			if freezeErr != nil {
				return nil, fmt.Errorf("transformer: formal Contribution operator: %w", freezeErr)
			}
			operator.contribution = contribution
			if step.kind != boundaryStepExternalCall {
				if capabilityErr := bindFormalRelationStepCapability(&operator, step); capabilityErr != nil {
					return nil, fmt.Errorf("transformer: formal relation Step operator: %w", capabilityErr)
				}
			}
			if step.kind == boundaryStepEffect {
				access, groups, reads, writes, accessErr := freezeFormalEffectTransferAccess(program, cell.Variable, operator)
				if accessErr != nil {
					return nil, fmt.Errorf("transformer: formal Effect access: %w", accessErr)
				}
				operator.effectAccess, operator.effectGroups = access, groups
				operator.effectReadOrdinals, operator.effectWriteOrdinals = reads, writes
				span, owned := program.formalFibers.span(cell.Variable)
				if !owned {
					return nil, fmt.Errorf("transformer: formal Effect lift has no frozen product span")
				}
				operator.effectLift, accessErr = sealFormalClosedFactorLift(span, [][]formalFiberOrdinal{reads}, writes)
				if accessErr != nil {
					return nil, fmt.Errorf("transformer: formal Effect lift: %w", accessErr)
				}
			}
		}
		template.equations[index] = formalRelationEquation{Cell: cellRef, Operator: operator}
		if operator.stepCapability == formalRelationStepCapabilityApply {
			template.applyCells = append(template.applyCells, cellRef)
		}
	}
	if err := freezeFormalInitialStateSeeds(program, template); err != nil {
		return nil, err
	}
	for index, cell := range region.cells {
		cellRef := template.equations[index].Cell
		operator := template.equations[index].Operator
		incoming := append([]formalRelationInfluence(nil), region.incoming[cell]...)
		sortFormalRelationInfluences(incoming)
		inputs := make([]formalRelationTemplateInput, len(incoming))
		for inputIndex, influence := range incoming {
			if influence.Target != cell || influence.Kind == formalRelationInfluenceInvalid {
				return nil, fmt.Errorf("transformer: formal relation template has malformed incoming influence")
			}
			sourceIndex, declared := region.plan.CanonicalIndex(influence.Source)
			if !declared || !region.plan.CoversInfluence(influence.Source, cell) {
				return nil, fmt.Errorf("transformer: formal relation template input is outside WTO plan")
			}
			input := formalRelationTemplateInput{
				Source:    formalRelationCellRef{region: region, cell: influence.Source, index: sourceIndex},
				Influence: influence.Kind,
				ReadPoint: influence.ReadPoint,
			}
			if influence.Site.valid() {
				siteIndex, siteDeclared := region.plan.CanonicalIndex(influence.Site)
				if !siteDeclared {
					return nil, fmt.Errorf("transformer: formal relation template site is undeclared")
				}
				input.Site = formalRelationCellRef{region: region, cell: influence.Site, index: siteIndex}
			}
			if !input.valid(cellRef) || !formalRelationInfluenceLegal(template, input, cellRef, operator) {
				return nil, fmt.Errorf("transformer: formal relation template input has invalid operator semantics")
			}
			inputs[inputIndex] = input
		}
		template.equations[index].Inputs = inputs
		// ExternalCall is the only Step whose frozen semantic program depends on
		// the complete influence row: each published read is a distinct provider
		// input wire. Freeze it here, never in the earlier operator-only pass.
		externalCall, externalCallErr := freezeFormalExternalCallStep(
			program, cell.Variable, template.equations[index].Operator, template.equations[index],
		)
		if externalCallErr != nil {
			return nil, fmt.Errorf("transformer: formal ExternalCall operator: %w", externalCallErr)
		}
		if externalCall != nil {
			template.equations[index].Operator.externalCall = externalCall
		}
		if template.equations[index].Cell.cell.Kind == formalRelationCellStep {
			step, stepOK := formalRelationStepOperator(template.equations[index].Operator)
			if !stepOK {
				return nil, fmt.Errorf("transformer: formal relation Step operator is malformed after input freeze")
			}
			if step.kind == boundaryStepExternalCall {
				if capabilityErr := bindFormalRelationStepCapability(&template.equations[index].Operator, step); capabilityErr != nil {
					return nil, fmt.Errorf("transformer: formal relation Step operator: %w", capabilityErr)
				}
			}
		}
		if !formalRelationEquationComplete(template, template.equations[index]) {
			return nil, fmt.Errorf("transformer: formal relation template equation is incomplete or duplicated")
		}
		transactions, transactionErr := freezeFormalApplyNonreturningTransactions(template, template.equations[index])
		if transactionErr != nil {
			return nil, transactionErr
		}
		template.equations[index].ApplyNonreturning = transactions
	}
	template.sealed = true
	return template, nil
}

func freezeFormalApplyNonreturningTransactions(
	template *formalRelationTemplate,
	equation formalRelationEquation,
) ([]formalApplyNonreturningTransaction, error) {
	if equation.Cell.cell.Kind != formalRelationCellNonreturning {
		return nil, nil
	}
	type partial struct {
		predecessor formalRelationCellRef
		target      formalRelationCellRef
	}
	bySite := make(map[formalRelationCellRef]partial)
	for _, input := range equation.Inputs {
		if input.Influence != formalRelationInfluenceApplyNonreturningPredecessor &&
			input.Influence != formalRelationInfluenceCalleeNonreturning {
			continue
		}
		if !input.Site.valid() || input.Site.index >= len(template.equations) {
			return nil, fmt.Errorf("transformer: formal nonreturning Apply has no sealed Site")
		}
		pair := bySite[input.Site]
		switch input.Influence {
		case formalRelationInfluenceApplyNonreturningPredecessor:
			if pair.predecessor.valid() {
				return nil, fmt.Errorf("transformer: formal nonreturning Apply Site has duplicate predecessor")
			}
			pair.predecessor = input.Source
		case formalRelationInfluenceCalleeNonreturning:
			if pair.target.valid() {
				return nil, fmt.Errorf("transformer: formal nonreturning Apply Site has duplicate target")
			}
			pair.target = input.Source
		}
		bySite[input.Site] = pair
	}
	transactions := make([]formalApplyNonreturningTransaction, 0, len(bySite))
	for site, pair := range bySite {
		operator := template.equations[site.index].Operator
		if !pair.predecessor.valid() || !pair.target.valid() || operator.kind != formalRelationCellStep ||
			operator.stepCapability != formalRelationStepCapabilityApply || operator.apply == nil {
			return nil, fmt.Errorf("transformer: formal nonreturning Apply Site is not a complete typed transaction")
		}
		transactions = append(transactions, formalApplyNonreturningTransaction{
			Site: site, Operator: operator, Predecessor: pair.predecessor, Target: pair.target,
		})
	}
	for index := 1; index < len(transactions); index++ {
		value := transactions[index]
		position := index
		for position > 0 && formalRelationCellLess(value.Site.cell, transactions[position-1].Site.cell) {
			transactions[position] = transactions[position-1]
			position--
		}
		transactions[position] = value
	}
	return transactions, nil
}

// sourceOperator resolves immutable syntax only for an already-declared input
// capability. It never discovers an equation dependency or reads solver state.
func (t *formalRelationTemplate) sourceOperator(input formalRelationTemplateInput) (formalRelationOperatorRef, loopMuTerm, bool) {
	if t == nil || !t.sealed || !input.Source.valid() || input.Source.region != t.region ||
		input.Source.index < 0 || input.Source.index >= len(t.equations) {
		return formalRelationOperatorRef{}, 0, false
	}
	operator := t.equations[input.Source.index].Operator
	cell := input.Source.cell
	if operator.kind != cell.Kind || cell.Variable == 0 || int(cell.Variable) > len(t.scopes) {
		return formalRelationOperatorRef{}, 0, false
	}
	return operator, operator.scope, true
}

func (t *formalRelationTemplate) equation(cell formalRelationCell) (formalRelationEquation, bool) {
	if t == nil || t.region == nil || t.region.plan == nil {
		return formalRelationEquation{}, false
	}
	index, ok := t.region.plan.CanonicalIndex(cell)
	if !ok || index < 0 || index >= len(t.equations) || t.equations[index].Cell.cell != cell {
		return formalRelationEquation{}, false
	}
	return t.equations[index], true
}

func (i formalRelationTemplateInput) valid(target formalRelationCellRef) bool {
	if !target.valid() || !i.Source.valid() || i.Source.region != target.region || i.Influence == formalRelationInfluenceInvalid ||
		!target.region.plan.CoversInfluence(i.Source.cell, target.cell) {
		return false
	}
	if i.Influence == formalRelationInfluenceCalleeNonreturning || i.Influence == formalRelationInfluenceApplyNonreturningPredecessor {
		return i.ReadPoint == 0 && i.Site.valid() && i.Site.region == target.region && i.Site.cell.Kind == formalRelationCellStep
	}
	noSite := !i.Site.cell.valid() && i.Site.region == nil && i.Site.index == 0
	if i.Influence == formalRelationInfluenceStepPublishedRead {
		return noSite
	}
	return noSite && i.ReadPoint == 0
}

func formalRelationInfluenceLegal(template *formalRelationTemplate, input formalRelationTemplateInput, target formalRelationCellRef, targetOperator formalRelationOperatorRef) bool {
	if template == nil || template.region == nil || !input.valid(target) || targetOperator.kind != target.cell.Kind ||
		input.Source.index < 0 || input.Source.index >= len(template.equations) {
		return false
	}
	sourceOperator := template.equations[input.Source.index].Operator
	if sourceOperator.kind != input.Source.cell.Kind {
		return false
	}
	var siteOperator formalRelationOperatorRef
	if input.Site.valid() {
		if input.Site.index < 0 || input.Site.index >= len(template.equations) {
			return false
		}
		siteOperator = template.equations[input.Site.index].Operator
		if siteOperator.kind != formalRelationCellStep {
			return false
		}
	}
	noSite := !input.Site.cell.valid() && input.Site.region == nil && input.Site.index == 0
	source, destination := input.Source.cell, target.cell
	sameVariable := source.Variable == destination.Variable

	switch input.Influence {
	case formalRelationInfluenceFlow:
		return noSite && sameVariable && formalRelationFlowLegal(source, destination, sourceOperator, targetOperator)
	case formalRelationInfluenceChoiceTrue, formalRelationInfluenceChoiceFalse:
		node, ok := formalRelationNodeOperator(sourceOperator)
		if !noSite || !sameVariable || !ok || source.Kind != formalRelationCellNode || destination.Kind != formalRelationCellNode || node.kind != relationNodeChoice {
			return false
		}
		want := node.whenTrue
		if input.Influence == formalRelationInfluenceChoiceFalse {
			want = node.whenFalse
		}
		return want != 0 && destination.Root == want
	case formalRelationInfluenceLoopFeedback, formalRelationInfluenceLoopExit:
		step, ok := formalRelationStepOperator(sourceOperator)
		if !noSite || !sameVariable || !ok || source.Kind != formalRelationCellStep || destination.Kind != formalRelationCellNode {
			return false
		}
		return formalRelationLoopInfluenceLegal(sourceOperator.code, step, destination.Root, input.Influence)
	case formalRelationInfluenceCalleeOutcome:
		step, ok := formalRelationStepOperator(targetOperator)
		return noSite && ok && source.Kind == formalRelationCellOutcome && destination.Kind == formalRelationCellStep &&
			step.kind == boundaryStepApply && step.apply.variable == source.Variable
	case formalRelationInfluenceStepNodeEntry, formalRelationInfluenceStepPublishedRead:
		_, stepOK := formalRelationStepOperator(targetOperator)
		return noSite && sameVariable && stepOK && source.valid() && destination.Kind == formalRelationCellStep &&
			template.region.stepDependencyDeclared(destination, source, input.Influence, input.ReadPoint)
	case formalRelationInfluenceLocalNonreturning:
		node, ok := formalRelationNodeOperator(sourceOperator)
		return noSite && sameVariable && ok && source.Kind == formalRelationCellNode && destination.Kind == formalRelationCellNonreturning &&
			node.kind == relationNodeNonreturning
	case formalRelationInfluenceApplyNonreturningPredecessor:
		step, ok := formalRelationStepOperator(siteOperator)
		return ok && destination.Kind == formalRelationCellNonreturning && destination.Variable == input.Site.cell.Variable &&
			step.kind == boundaryStepApply && formalRelationStepPredecessor(source, input.Site.cell)
	case formalRelationInfluenceCalleeNonreturning:
		step, ok := formalRelationStepOperator(siteOperator)
		return ok && source.Kind == formalRelationCellNonreturning && destination.Kind == formalRelationCellNonreturning &&
			destination.Variable == input.Site.cell.Variable && step.kind == boundaryStepApply && step.apply.variable == source.Variable
	case formalRelationInfluenceDefinitionSeed:
		return noSite && formalRelationDefinitionSeedLegal(template.region, source, destination, sourceOperator, targetOperator)
	case formalRelationInfluenceDefinitionOutcome:
		definition, ok := formalRelationDefinitionOperator(targetOperator)
		return noSite && ok && source.Kind == formalRelationCellOutcome && destination.Kind == formalRelationCellDefinition && source.Variable == definition.target
	case formalRelationInfluenceResourceSeed:
		resource, ok := formalRelationResourceOperator(targetOperator)
		return noSite && ok && source.Kind == formalRelationCellOutcome && destination.Kind == formalRelationCellResource && source.Variable == resource.owner
	case formalRelationInfluenceResourceFeedback:
		definition, definitionOK := formalRelationDefinitionOperator(sourceOperator)
		resource, resourceOK := formalRelationResourceOperator(targetOperator)
		return noSite && definitionOK && resourceOK && source.Kind == formalRelationCellDefinition && destination.Kind == formalRelationCellResource &&
			definition.external && definition.owner == resource.owner && formalRelationResourceContains(resource, source.Definition)
	case formalRelationInfluenceClosureDefinition:
		definition, definitionOK := formalRelationDefinitionOperator(sourceOperator)
		step, stepOK := formalRelationStepOperator(targetOperator)
		return noSite && definitionOK && stepOK && source.Kind == formalRelationCellDefinition && destination.Kind == formalRelationCellStep &&
			definition.external && step.kind == boundaryStepApply && definition.target == step.apply.variable &&
			formalRelationClosureProducerMatches(targetOperator.code, step.apply.frame, definition.owner)
	default:
		return false
	}
}

func formalRelationNodeOperator(operator formalRelationOperatorRef) (relationNode, bool) {
	if operator.kind != formalRelationCellNode || operator.code == nil || operator.root == 0 || int(operator.root) >= len(operator.code.nodes) {
		return relationNode{}, false
	}
	return operator.code.nodes[operator.root], true
}

func formalRelationStepOperator(operator formalRelationOperatorRef) (boundaryStep, bool) {
	if operator.kind != formalRelationCellStep || operator.code == nil || operator.root == 0 || int(operator.root) >= len(operator.code.nodes) ||
		operator.code.nodes[operator.root].kind != relationNodeSequence || operator.step == 0 || int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return boundaryStep{}, false
	}
	return operator.code.nodes[operator.root].steps[operator.step-1], true
}

func formalRelationDefinitionOperator(operator formalRelationOperatorRef) (formalRelationDefinition, bool) {
	if operator.kind != formalRelationCellDefinition || operator.region == nil || operator.definition == 0 || int(operator.definition) >= len(operator.region.definitions) {
		return formalRelationDefinition{}, false
	}
	definition := operator.region.definitions[operator.definition]
	return definition, definition.cell.Definition == operator.definition
}

func formalRelationResourceOperator(operator formalRelationOperatorRef) (formalRelationResource, bool) {
	if operator.kind != formalRelationCellResource || operator.region == nil || operator.resource == 0 || int(operator.resource) >= len(operator.region.resources) {
		return formalRelationResource{}, false
	}
	resource := operator.region.resources[operator.resource]
	return resource, resource.cell.Resource == operator.resource
}

func formalRelationStepPredecessor(source, site formalRelationCell) bool {
	if source.Variable != site.Variable || site.Kind != formalRelationCellStep || source.Root != site.Root || site.Step == 0 {
		return false
	}
	if site.Step == 1 {
		return source.Kind == formalRelationCellNode && source.Step == 0
	}
	return source.Kind == formalRelationCellStep && source.Step+1 == site.Step
}

func formalRelationFlowLegal(source, target formalRelationCell, sourceOperator, targetOperator formalRelationOperatorRef) bool {
	switch target.Kind {
	case formalRelationCellStep:
		_, targetOK := formalRelationStepOperator(targetOperator)
		node, nodeOK := formalRelationNodeOperator(sourceOperator)
		if targetOK && nodeOK && source.Kind == formalRelationCellNode && node.kind == relationNodeSequence {
			return formalRelationStepPredecessor(source, target)
		}
		_, sourceOK := formalRelationStepOperator(sourceOperator)
		return targetOK && sourceOK && formalRelationStepPredecessor(source, target)
	case formalRelationCellNode:
		node, ok := formalRelationNodeOperator(sourceOperator)
		if ok {
			switch node.kind {
			case relationNodeSequence:
				return len(node.steps) == 0 && node.next == target.Root
			case relationNodeLoopMu, relationNodeLoopPortal:
				return node.body == target.Root
			}
			return false
		}
		if source.Kind != formalRelationCellStep {
			return false
		}
		_, stepOK := formalRelationStepOperator(sourceOperator)
		if !stepOK || sourceOperator.code == nil || source.Root == 0 || int(source.Root) >= len(sourceOperator.code.nodes) {
			return false
		}
		owner := sourceOperator.code.nodes[source.Root]
		return owner.kind == relationNodeSequence && int(source.Step) == len(owner.steps) && owner.next == target.Root
	case formalRelationCellOutcome:
		node, ok := formalRelationNodeOperator(sourceOperator)
		return ok && source.Kind == formalRelationCellNode && node.kind == relationNodeOutcome && node.outcome == target.Outcome
	default:
		return false
	}
}

func formalRelationLoopInfluenceLegal(code *relationCode, step boundaryStep, target relationRootRef, influence formalRelationInfluenceKind) bool {
	if code == nil || target == 0 || step.binder == 0 {
		return false
	}
	for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
		node := code.nodes[root]
		if node.kind != relationNodeLoopMu || node.binder != step.binder {
			continue
		}
		if influence == formalRelationInfluenceLoopFeedback {
			return step.kind == boundaryStepLoopFeedback && node.body == target
		}
		return step.kind == boundaryStepLoopExit && int(step.route) < len(node.exits) && node.exits[step.route] == target
	}
	return false
}

func formalRelationDefinitionSeedLegal(region *formalRelationRegionInventory, source, target formalRelationCell, sourceOperator, targetOperator formalRelationOperatorRef) bool {
	definition, ok := formalRelationDefinitionOperator(targetOperator)
	if !ok || target.Kind != formalRelationCellDefinition {
		return false
	}
	if definition.external {
		resource, resourceOK := formalRelationResourceOperator(sourceOperator)
		return resourceOK && source.Kind == formalRelationCellResource && resource.owner == definition.owner &&
			formalRelationResourceContains(resource, target.Definition)
	}
	_, nodeOK := formalRelationNodeOperator(sourceOperator)
	if !nodeOK || source.Kind != formalRelationCellNode || source.Variable != definition.owner || region == nil {
		return false
	}
	for _, publication := range sourceOperator.code.publication.points {
		if publication.point == definition.point && publication.ref == source.Root {
			return true
		}
	}
	return false
}

type formalRelationInfluenceCounts [formalRelationInfluenceClosureDefinition + 1]uint32

// formalRelationEquationComplete closes the structural law: legal inputs are
// not merely admissible in isolation; every operator owns the complete finite
// set of influences induced by its sealed syntax. Counts are sufficient once
// inputs are operator-legal and duplicate identities are forbidden.
func formalRelationEquationComplete(template *formalRelationTemplate, equation formalRelationEquation) bool {
	if template == nil || template.region == nil || !equation.Cell.valid() {
		return false
	}
	if equation.Cell.cell.Kind == formalRelationCellStep && equation.Operator.stepCapability == formalRelationStepCapabilityInvalid {
		return false
	}
	var actual formalRelationInfluenceCounts
	for index, input := range equation.Inputs {
		for previous := 0; previous < index; previous++ {
			candidate := equation.Inputs[previous]
			if candidate.Source.cell == input.Source.cell && candidate.Influence == input.Influence &&
				candidate.ReadPoint == input.ReadPoint && candidate.Site.cell == input.Site.cell {
				return false
			}
		}
		actual[input.Influence]++
	}
	expected, ok := formalRelationExpectedInfluenceCounts(template, equation)
	return ok && actual == expected
}

func formalRelationExpectedInfluenceCounts(template *formalRelationTemplate, equation formalRelationEquation) (formalRelationInfluenceCounts, bool) {
	cell, operator, region := equation.Cell.cell, equation.Operator, template.region
	var counts formalRelationInfluenceCounts
	add := func(kind formalRelationInfluenceKind) { counts[kind]++ }
	code := operator.code
	switch cell.Kind {
	case formalRelationCellNode:
		if code == nil {
			return counts, false
		}
		loops := make(map[loopMuTerm]formalRelationLoopTarget)
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			node := code.nodes[root]
			if node.kind == relationNodeLoopMu {
				loops[node.binder] = formalRelationLoopTarget{body: node.body, exits: node.exits}
			}
		}
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			node := code.nodes[root]
			switch node.kind {
			case relationNodeSequence:
				if node.next == cell.Root {
					add(formalRelationInfluenceFlow)
				}
				for _, step := range node.steps {
					loop, exists := loops[step.binder]
					if !exists {
						continue
					}
					if step.kind == boundaryStepLoopFeedback && loop.body == cell.Root {
						add(formalRelationInfluenceLoopFeedback)
					}
					if step.kind == boundaryStepLoopExit && int(step.route) < len(loop.exits) && loop.exits[step.route] == cell.Root {
						add(formalRelationInfluenceLoopExit)
					}
				}
			case relationNodeChoice:
				if node.whenTrue == cell.Root {
					add(formalRelationInfluenceChoiceTrue)
				}
				if node.whenFalse == cell.Root {
					add(formalRelationInfluenceChoiceFalse)
				}
			case relationNodeLoopMu, relationNodeLoopPortal:
				if node.body == cell.Root {
					add(formalRelationInfluenceFlow)
				}
			}
		}
	case formalRelationCellStep:
		step, ok := formalRelationStepOperator(operator)
		if !ok {
			return counts, false
		}
		add(formalRelationInfluenceFlow)
		for _, kind := range []formalRelationInfluenceKind{
			formalRelationInfluenceStepNodeEntry,
			formalRelationInfluenceStepPublishedRead,
		} {
			for count := region.stepDependencyCount(cell, kind); count > 0; count-- {
				add(kind)
			}
		}
		if step.kind == boundaryStepApply {
			if step.apply.variable == 0 || int(step.apply.variable) > len(region.outcomes) {
				return counts, false
			}
			for range region.outcomes[step.apply.variable-1] {
				add(formalRelationInfluenceCalleeOutcome)
			}
			closureDefinitions, valid := formalRelationClosureDefinitionCount(region, code, step)
			if !valid {
				return counts, false
			}
			for index := 0; index < closureDefinitions; index++ {
				add(formalRelationInfluenceClosureDefinition)
			}
		}
	case formalRelationCellOutcome:
		if code == nil {
			return counts, false
		}
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			node := code.nodes[root]
			if node.kind == relationNodeOutcome && node.outcome == cell.Outcome {
				add(formalRelationInfluenceFlow)
			}
		}
	case formalRelationCellNonreturning:
		if code == nil {
			return counts, false
		}
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			node := code.nodes[root]
			if node.kind == relationNodeNonreturning {
				add(formalRelationInfluenceLocalNonreturning)
			}
			for _, step := range node.steps {
				if step.kind == boundaryStepApply {
					add(formalRelationInfluenceApplyNonreturningPredecessor)
					add(formalRelationInfluenceCalleeNonreturning)
				}
			}
		}
	case formalRelationCellDefinition:
		definition, ok := formalRelationDefinitionOperator(operator)
		if !ok || definition.target == 0 || int(definition.target) > len(region.outcomes) {
			return counts, false
		}
		if definition.external {
			add(formalRelationInfluenceDefinitionSeed)
		} else {
			ownerIndex := region.planIndex(region.roots[definition.owner-1])
			if ownerIndex < 0 || ownerIndex >= len(template.equations) {
				return counts, false
			}
			ownerCode := template.equations[ownerIndex].Operator.code
			if ownerCode == nil {
				return counts, false
			}
			for _, publication := range ownerCode.publication.points {
				if publication.point == definition.point && publication.ref != 0 {
					add(formalRelationInfluenceDefinitionSeed)
				}
			}
		}
		for range region.outcomes[definition.target-1] {
			add(formalRelationInfluenceDefinitionOutcome)
		}
	case formalRelationCellResource:
		resource, ok := formalRelationResourceOperator(operator)
		if !ok || resource.owner == 0 || int(resource.owner) > len(region.outcomes) {
			return counts, false
		}
		for range region.outcomes[resource.owner-1] {
			add(formalRelationInfluenceResourceSeed)
		}
		for range resource.members {
			add(formalRelationInfluenceResourceFeedback)
		}
	default:
		return counts, false
	}
	return counts, true
}

func (r *formalRelationRegionInventory) planIndex(cell formalRelationCell) int {
	if r == nil || r.plan == nil {
		return -1
	}
	index, ok := r.plan.CanonicalIndex(cell)
	if !ok {
		return -1
	}
	return index
}

func formalRelationClosureDefinitionCount(region *formalRelationRegionInventory, code *relationCode, step boundaryStep) (int, bool) {
	if code == nil || code.terms == nil || step.apply.frame == 0 || int(step.apply.frame) >= len(code.terms.callFrames) {
		return 0, true
	}
	producerFrame := code.terms.callFrames[step.apply.frame].closureProducer
	if producerFrame == 0 {
		return 0, true
	}
	var producer relationVar
	for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
		for _, candidate := range code.nodes[root].steps {
			if candidate.kind != boundaryStepApply || candidate.apply.frame != producerFrame {
				continue
			}
			if producer != 0 && producer != candidate.apply.variable {
				return 0, false
			}
			producer = candidate.apply.variable
		}
	}
	if producer == 0 {
		return 0, false
	}
	count := 0
	for resourceRef := formalRelationResourceRef(1); int(resourceRef) < len(region.resources); resourceRef++ {
		resource := region.resources[resourceRef]
		if resource.owner != producer {
			continue
		}
		for _, definitionRef := range resource.members {
			if region.definitions[definitionRef].target == step.apply.variable {
				count++
			}
		}
	}
	return count, count == 1
}

func formalRelationResourceContains(resource formalRelationResource, definition formalRelationDefinitionRef) bool {
	for _, member := range resource.members {
		if member == definition {
			return true
		}
	}
	return false
}

func formalRelationClosureProducerMatches(code *relationCode, frame callFrameTerm, owner relationVar) bool {
	if code == nil || code.terms == nil || frame == 0 || int(frame) >= len(code.terms.callFrames) {
		return false
	}
	closureProducer := code.terms.callFrames[frame].closureProducer
	if closureProducer == 0 {
		return false
	}
	matches := 0
	for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
		for _, step := range code.nodes[root].steps {
			if step.kind == boundaryStepApply && step.apply.frame == closureProducer && step.apply.variable == owner {
				matches++
			}
		}
	}
	return matches == 1
}

func freezeFormalRelationOperator(program *RelationProgram, region *formalRelationRegionInventory, rootInputs []formalRootInputTemplate, cell formalRelationCell) (formalRelationOperatorRef, error) {
	if program == nil || region == nil || !cell.valid() || cell.Variable == 0 || int(cell.Variable) > len(program.bodies) {
		return formalRelationOperatorRef{}, fmt.Errorf("operator is unowned")
	}
	if program.formalFibers == nil || program.formalFibers.operatorFootprints == nil {
		return formalRelationOperatorRef{}, fmt.Errorf("operator has no coordinate footprint declarations")
	}
	footprint, err := program.formalFibers.operatorFootprints.bind(program, cell)
	if err != nil {
		return formalRelationOperatorRef{}, err
	}
	code := program.bodies[cell.Variable-1].relation.code
	if code == nil || !code.sealed {
		return formalRelationOperatorRef{}, fmt.Errorf("operator has no sealed relationCode")
	}
	operator := formalRelationOperatorRef{kind: cell.Kind, footprint: footprint}
	switch cell.Kind {
	case formalRelationCellNode:
		if cell.Root == 0 || int(cell.Root) >= len(code.nodes) || code.nodes[cell.Root].kind == relationNodeInvalid {
			return formalRelationOperatorRef{}, fmt.Errorf("node operator is outside relationCode")
		}
		operator.code, operator.root = code, cell.Root
		if cell == region.roots[cell.Variable-1] {
			if int(cell.Variable) > len(rootInputs) || !rootInputs[cell.Variable-1].valid() {
				return formalRelationOperatorRef{}, fmt.Errorf("lexical root operator has no formal input")
			}
			operator.rootInput = &rootInputs[cell.Variable-1]
		}
	case formalRelationCellStep:
		if cell.Root == 0 || int(cell.Root) >= len(code.nodes) || cell.Step == 0 || int(cell.Step) > len(code.nodes[cell.Root].steps) ||
			code.nodes[cell.Root].steps[cell.Step-1].kind == boundaryStepInvalid {
			return formalRelationOperatorRef{}, fmt.Errorf("step operator is outside relationCode")
		}
		operator.code, operator.root, operator.step = code, cell.Root, cell.Step
	case formalRelationCellOutcome:
		if cell.Outcome == 0 || int(cell.Outcome) >= len(code.outcomes) {
			return formalRelationOperatorRef{}, fmt.Errorf("outcome operator is outside relationCode")
		}
		operator.code, operator.outcome = code, cell.Outcome
	case formalRelationCellNonreturning:
		operator.code = code
	case formalRelationCellDefinition:
		if cell.Definition == 0 || int(cell.Definition) >= len(region.definitions) || region.definitions[cell.Definition].cell != cell {
			return formalRelationOperatorRef{}, fmt.Errorf("definition operator is outside formal inventory")
		}
		operator.region, operator.definition = region, cell.Definition
	case formalRelationCellResource:
		if cell.Resource == 0 || int(cell.Resource) >= len(region.resources) || region.resources[cell.Resource].cell != cell {
			return formalRelationOperatorRef{}, fmt.Errorf("resource operator is outside formal inventory")
		}
		operator.region, operator.resource = region, cell.Resource
	default:
		return formalRelationOperatorRef{}, fmt.Errorf("operator has invalid kind")
	}
	return operator, nil
}

func sortFormalRelationInfluences(in []formalRelationInfluence) {
	// Insertion sort is deliberate: incoming rows are normally tiny, and this
	// avoids a closure allocation in immutable-template construction.
	for index := 1; index < len(in); index++ {
		value := in[index]
		position := index
		for position > 0 && formalRelationInfluenceLess(value, in[position-1]) {
			in[position] = in[position-1]
			position--
		}
		in[position] = value
	}
}

func formalRelationInfluenceLess(left, right formalRelationInfluence) bool {
	if left.Source != right.Source {
		return formalRelationCellLess(left.Source, right.Source)
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.ReadPoint != right.ReadPoint {
		return left.ReadPoint < right.ReadPoint
	}
	return formalRelationCellLess(left.Site, right.Site)
}
