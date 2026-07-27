package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// RootAssignmentFactorStage is one ordered semantic phase of N4.  This is a
// closed language: execution adapters may factor a phase over their own
// representation, but may neither reorder phases nor invent another N4
// schedule.
type RootAssignmentFactorStage uint8

const (
	RootAssignmentFactorStageInvalid RootAssignmentFactorStage = iota
	RootAssignmentFactorStageObjectMaterialization
	RootAssignmentFactorStageSourceComposition
	RootAssignmentFactorStageValuePublication
	RootAssignmentFactorStageDynamicSource
	RootAssignmentFactorStagePathMutation
	RootAssignmentFactorStageEqualityQuotient
	RootAssignmentFactorStageScalarTransfer
	RootAssignmentFactorStageFreshEmpty
	RootAssignmentFactorStageCompletion
)

// RootAssignmentFactorProgram is the sole ordered, factor-native N4 program.
// The ProductDomain-sealed topology is retained with the semantic plan so
// adapters can bind only declared factors.  Unlisted factors are structural
// carry, not inputs that an adapter may materialize speculatively.
type RootAssignmentFactorProgram struct {
	plan     ResolvedRootAssignmentPlan
	topology state.RootAssignmentFactorPlan
	stages   []RootAssignmentFactorStage
}

// FactorProgram closes a resolved N4 plan into its one executable phase
// schedule. Optional phases are decided from sealed plan/domain capabilities,
// never from axis names or adapter inventory scans.
func (p ResolvedRootAssignmentPlan) FactorProgram() (RootAssignmentFactorProgram, error) {
	if !p.Valid() {
		return RootAssignmentFactorProgram{}, fmt.Errorf("factapply: invalid root-assignment factor program")
	}
	topology, ok := p.FactorPlan()
	if !ok || !p.authority.domain.OwnsRootAssignmentFactorPlan(topology) {
		return RootAssignmentFactorProgram{}, fmt.Errorf("factapply: root-assignment factor topology is unsealed")
	}
	shape, ok := p.SourceShape()
	if !ok {
		return RootAssignmentFactorProgram{}, fmt.Errorf("factapply: root-assignment source shape is unsealed")
	}
	stages := make([]RootAssignmentFactorStage, 0, 9)
	if shape == RootAssignmentSourceObjectLiteral {
		stages = append(stages, RootAssignmentFactorStageObjectMaterialization)
	}
	stages = append(stages,
		RootAssignmentFactorStageSourceComposition,
		RootAssignmentFactorStageValuePublication,
	)
	hasDynamic := shape == RootAssignmentSourceDynamicIndex
	// Every admitted root assignment has an exact target path.  The path phase
	// is present only when the selected product registers its owner.
	if _, present := p.authority.domain.PathEvidenceCoordinateFamily(); present {
		stages = append(stages, RootAssignmentFactorStagePathMutation)
	}
	// Dynamic read origins and key memberships describe the newly assigned
	// target. Publish them after destructive subtree replacement but before the
	// equality quotient, so the quotient observes and transports the new
	// sidecar. Publishing before replacement erased non-exact-key evidence;
	// publishing after equality left exact-key evidence unquotiented.
	if hasDynamic {
		stages = append(stages, RootAssignmentFactorStageDynamicSource)
	}
	_, publishesAlias := p.PublishedEqualityProof()
	if hasDynamic || publishesAlias {
		stages = append(stages, RootAssignmentFactorStageEqualityQuotient)
	}
	if _, present := p.ScalarFactorTransaction(); present {
		stages = append(stages, RootAssignmentFactorStageScalarTransfer)
	}
	queries, err := p.FactorCompletionFreshEmptyPaths()
	if err != nil {
		return RootAssignmentFactorProgram{}, err
	}
	if len(queries) != 0 {
		stages = append(stages, RootAssignmentFactorStageFreshEmpty)
	}
	if len(p.authority.domain.RootAssignmentCompletionLanes()) != 0 ||
		len(p.authority.domain.RootAssignmentCompletionCoordinateFamilies()) != 0 {
		stages = append(stages, RootAssignmentFactorStageCompletion)
	}
	return RootAssignmentFactorProgram{plan: p, topology: topology, stages: stages}, nil
}

func (p RootAssignmentFactorProgram) Valid() bool {
	return p.plan.Valid() && p.plan.authority.domain.OwnsRootAssignmentFactorPlan(p.topology) && len(p.stages) >= 2
}

// Stages returns the canonical N4 phase order.
func (p RootAssignmentFactorProgram) Stages() []RootAssignmentFactorStage {
	if !p.Valid() {
		return nil
	}
	return append([]RootAssignmentFactorStage(nil), p.stages...)
}

// PointEntryLanes are the only immutable point factors an adapter may bind.
func (p RootAssignmentFactorProgram) PointEntryLanes() []state.ProductLane {
	if !p.Valid() {
		return nil
	}
	return p.topology.PointEntryLanes()
}

// CurrentLanes are the only sequential factors an adapter may bind.
func (p RootAssignmentFactorProgram) CurrentLanes() []state.ProductLane {
	if !p.Valid() {
		return nil
	}
	return p.topology.CurrentLanes()
}

// CurrentWriteLanes are the only factors an adapter may publish. All other
// product groups must retain their original structural identity.
func (p RootAssignmentFactorProgram) CurrentWriteLanes() []state.ProductLane {
	if !p.Valid() {
		return nil
	}
	return p.topology.CurrentWriteLanes()
}

// ResolvedPlan returns the immutable semantic authority carried by this
// program. It is intentionally a value, not an adapter callback.
func (p RootAssignmentFactorProgram) ResolvedPlan() (ResolvedRootAssignmentPlan, bool) {
	return p.plan, p.Valid()
}

func (p RootAssignmentFactorProgram) ownsStage(want RootAssignmentFactorStage) bool {
	if !p.Valid() || want == RootAssignmentFactorStageInvalid {
		return false
	}
	for _, stage := range p.stages {
		if stage == want {
			return true
		}
	}
	return false
}

// ComposeSource evaluates the canonical source-composition phase.
func (p RootAssignmentFactorProgram) ComposeSource(primary product.Value, definitelyPresent bool) (product.Value, bool, error) {
	if !p.ownsStage(RootAssignmentFactorStageSourceComposition) {
		return product.Value{}, false, fmt.Errorf("factapply: RootAssignment source phase is unavailable")
	}
	return p.plan.ComposeFactorPrimarySourceValue(p.plan.authority.domain.Registry(), primary, definitelyPresent)
}

// ApplyValuePublication evaluates the canonical Values phase. False is the
// legitimate non-productive arm and requires structural carry by the adapter.
func (p RootAssignmentFactorProgram) ApplyValuePublication(composed product.Value, productive, valuesTop bool) (product.Value, bool, error) {
	if !p.ownsStage(RootAssignmentFactorStageValuePublication) ||
		!product.BelongsToRegistry(p.plan.authority.domain.Registry(), composed) {
		return product.Value{}, false, fmt.Errorf("factapply: invalid RootAssignment value phase")
	}
	if !productive {
		return product.Value{}, false, nil
	}
	write, err := p.plan.PrepareFactorValueWrite(composed)
	if err != nil {
		return product.Value{}, false, err
	}
	value, err := p.plan.authority.domain.ApplyRootAssignmentValueScalar(write, valuesTop)
	return value, err == nil, err
}

// ObjectConstructorLanes returns the exact registration-owned factor groups
// for the optional object-materialization phase.
func (p RootAssignmentFactorProgram) ObjectConstructorLanes(plan state.ObjectConstructorPlan) ([]state.ProductLane, error) {
	if !p.ownsStage(RootAssignmentFactorStageObjectMaterialization) {
		return nil, fmt.Errorf("factapply: RootAssignment object phase is unavailable")
	}
	return p.plan.authority.domain.ObjectConstructorLanes(plan)
}

// ApplyObjectMaterialization evaluates one registered object-constructor
// factor.  Adapters bind only ObjectConstructorLanes and structurally carry
// every other group.
func (p RootAssignmentFactorProgram) ApplyObjectMaterialization(plan state.ObjectConstructorPlan, values []state.ObjectConstructorValues, current state.LaneFactor) (state.LaneFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageObjectMaterialization) {
		return state.LaneFactor{}, fmt.Errorf("factapply: RootAssignment object phase is unavailable")
	}
	return p.plan.authority.domain.ApplyObjectConstructorFactor(plan, values, current)
}

// ResolveDynamicSource evaluates the one canonical dynamic-source law from
// adapter-bound operands.
func (p RootAssignmentFactorProgram) ResolveDynamicSource(ctx context.Context, inputs RootAssignmentDynamicSourceInputs) (RootAssignmentDynamicSourceTransaction, error) {
	if !p.ownsStage(RootAssignmentFactorStageDynamicSource) {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic phase is unavailable")
	}
	dynamic, ok := p.plan.DynamicSourcePlan()
	if !ok {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic plan is unavailable")
	}
	return dynamic.Resolve(ctx, inputs)
}

// ApplyDynamicSource evaluates one registered dynamic-source output factor.
func (p RootAssignmentFactorProgram) ApplyDynamicSource(transaction RootAssignmentDynamicSourceTransaction, current state.LaneFactor) (state.LaneFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageDynamicSource) || !transaction.Valid() {
		return state.LaneFactor{}, fmt.Errorf("factapply: invalid RootAssignment dynamic-source phase")
	}
	return transaction.ApplyDynamicSourceFactor(current)
}

// RootAssignmentPathFactorInput is the exact path-family operand tuple for the
// canonical path phase. The adapter binds only registered coordinate slots;
// it never supplies State or a semantic callback.
type RootAssignmentPathFactorInput struct {
	Factors           state.PathSubtreeMutationFactors
	Authority         state.CoordinatePathEvidenceAuthority[statekey.Value]
	OldValue          product.Value
	Composed          product.Value
	Dynamic           RootAssignmentDynamicSourceTransaction
	HasDynamic        bool
	FormalStableRoots []formal.Root
}

// RootAssignmentPathFactorResult is the atomic result of path replacement,
// stable-root publication and path-equality publication. Descendant and
// quotient participants consume these sealed transactions; they do not
// reinterpret path semantics.
type RootAssignmentPathFactorResult struct {
	Factors    state.PathSubtreeMutationFactors
	Equalities []state.PathEqualityTransaction
}

// ApplyPathMutation is the sole ordered N4 path evaluator.
func (p RootAssignmentFactorProgram) ApplyPathMutation(input RootAssignmentPathFactorInput) (RootAssignmentPathFactorResult, error) {
	if !p.ownsStage(RootAssignmentFactorStagePathMutation) {
		return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: RootAssignment path phase is unavailable")
	}
	domain := p.plan.authority.domain
	reg := domain.Registry()
	if !product.BelongsToRegistry(reg, input.OldValue) || !product.BelongsToRegistry(reg, input.Composed) ||
		input.HasDynamic && !input.Dynamic.Valid() {
		return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: invalid RootAssignment path operands")
	}
	owner, hasOwner := domain.PathValueFamily()
	if !hasOwner {
		return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: RootAssignment path owner is unavailable")
	}
	factors := input.Factors
	var skeleton state.CoordinateFamilySkeleton
	var scalars []state.CoordinateScalarFactor
	foundOwner := false
	for _, factor := range factors.CoordinateFactors() {
		if factor.Family() == owner {
			skeleton, scalars, foundOwner = factor.Skeleton(), factor.Scalars(), true
			break
		}
	}
	if !foundOwner {
		return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: RootAssignment path factor tuple omits its owner")
	}
	subtree, hasSubtree, err := p.PrepareSubtree(skeleton, scalars)
	if err != nil {
		return RootAssignmentPathFactorResult{}, err
	}
	if hasSubtree {
		factors, err = domain.ApplyPathSubtreeMutationFactors(subtree, factors)
		if err != nil {
			return RootAssignmentPathFactorResult{}, err
		}
		foundOwner = false
		for _, factor := range factors.CoordinateFactors() {
			if factor.Family() == owner {
				skeleton, scalars, foundOwner = factor.Skeleton(), factor.Scalars(), true
				break
			}
		}
		if !foundOwner {
			return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: RootAssignment subtree result omits its owner")
		}
	}
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, state.ValueLaneFactor{}, true,
		input.Authority, state.PathDescendantMutationFactors{},
	)
	if err != nil {
		return RootAssignmentPathFactorResult{}, err
	}
	stable, err := p.plan.PrepareFactorStablePathEvidenceWithFormalRoots(carrier, input.Composed, product.Equal(reg, input.OldValue, input.Composed), input.FormalStableRoots)
	if err != nil {
		return RootAssignmentPathFactorResult{}, err
	}
	if _, valid := carrier.ApplyStableRootPathEvidence(stable); !valid {
		return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: RootAssignment stable-path mutation rejected")
	}
	equalities := make([]state.PathEqualityTransaction, 0, 2)
	if input.HasDynamic {
		transaction, publish, equalityErr := p.PrepareDynamicEquality(input.Dynamic, carrier)
		if equalityErr != nil {
			return RootAssignmentPathFactorResult{}, equalityErr
		}
		if publish {
			if _, equalityErr = domain.ApplyCoordinatePathEqualityTransaction(transaction, carrier); equalityErr != nil {
				return RootAssignmentPathFactorResult{}, equalityErr
			}
			equalities = append(equalities, transaction)
		}
	}
	if p.ownsStage(RootAssignmentFactorStageEqualityQuotient) {
		transaction, publish, equalityErr := p.PrepareEquality(carrier)
		if equalityErr != nil {
			return RootAssignmentPathFactorResult{}, equalityErr
		}
		if publish {
			if _, equalityErr = domain.ApplyCoordinatePathEqualityTransaction(transaction, carrier); equalityErr != nil {
				return RootAssignmentPathFactorResult{}, equalityErr
			}
			equalities = append(equalities, transaction)
		}
	}
	skeleton, scalars, _, _, _, _, err = carrier.Freeze()
	if err != nil {
		return RootAssignmentPathFactorResult{}, err
	}
	coordinates := factors.CoordinateFactors()
	replacedOwner := false
	for index, factor := range coordinates {
		if factor.Family() != owner {
			continue
		}
		coordinates[index], err = domain.SealCoordinateFamilyFactor(skeleton, scalars)
		if err != nil {
			return RootAssignmentPathFactorResult{}, err
		}
		replacedOwner = true
		break
	}
	if !replacedOwner {
		return RootAssignmentPathFactorResult{}, fmt.Errorf("factapply: RootAssignment path result has no owner")
	}
	factors, err = domain.SealPathSubtreeMutationFactors(factors.LaneFactors(), coordinates)
	if err != nil {
		return RootAssignmentPathFactorResult{}, err
	}
	return RootAssignmentPathFactorResult{Factors: factors, Equalities: equalities}, nil
}

func (p RootAssignmentFactorProgram) PrepareSubtree(skeleton state.CoordinateFamilySkeleton, scalars []state.CoordinateScalarFactor) (state.PathSubtreeMutation, bool, error) {
	if !p.ownsStage(RootAssignmentFactorStagePathMutation) {
		return state.PathSubtreeMutation{}, false, fmt.Errorf("factapply: RootAssignment path phase is unavailable")
	}
	return p.plan.PrepareFactorPathSubtree(skeleton, scalars)
}

func (p RootAssignmentFactorProgram) PrepareDynamicEquality(transaction RootAssignmentDynamicSourceTransaction, carrier *state.CoordinatePathEvidenceCarrier[statekey.Value]) (state.PathEqualityTransaction, bool, error) {
	if !p.ownsStage(RootAssignmentFactorStageEqualityQuotient) || !transaction.Valid() {
		return state.PathEqualityTransaction{}, false, fmt.Errorf("factapply: RootAssignment dynamic equality phase is unavailable")
	}
	return transaction.PrepareCoordinatePathEquality(carrier)
}

func (p RootAssignmentFactorProgram) PrepareEquality(carrier *state.CoordinatePathEvidenceCarrier[statekey.Value]) (state.PathEqualityTransaction, bool, error) {
	if !p.ownsStage(RootAssignmentFactorStageEqualityQuotient) {
		return state.PathEqualityTransaction{}, false, fmt.Errorf("factapply: RootAssignment equality phase is unavailable")
	}
	return p.plan.PrepareFactorPathEquality(carrier)
}

func (p RootAssignmentFactorProgram) ApplyEqualityFactor(transaction state.PathEqualityTransaction, current state.LaneFactor) (state.LaneFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageEqualityQuotient) {
		return state.LaneFactor{}, fmt.Errorf("factapply: RootAssignment equality phase is unavailable")
	}
	return p.plan.authority.domain.ApplyPathEqualityTransactionFactor(transaction, current)
}

func (p RootAssignmentFactorProgram) ApplyScalarFactor(pointEntry, current state.LaneFactor) (state.LaneFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageScalarTransfer) {
		return state.LaneFactor{}, fmt.Errorf("factapply: RootAssignment scalar phase is unavailable")
	}
	transaction, ok := p.plan.ScalarFactorTransaction()
	if !ok {
		return state.LaneFactor{}, fmt.Errorf("factapply: RootAssignment scalar transaction is unavailable")
	}
	return p.plan.authority.domain.ApplyRootAssignmentScalarFactor(transaction, pointEntry, current)
}

func (p RootAssignmentFactorProgram) ApplyScalarCoordinate(currentSkeleton state.CoordinateFamilySkeleton, currentTarget, pointSource state.CoordinateScalarFactor, hasPointSource bool) (state.CoordinateFamilySkeleton, state.CoordinateScalarFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageScalarTransfer) {
		return state.CoordinateFamilySkeleton{}, state.CoordinateScalarFactor{}, fmt.Errorf("factapply: RootAssignment scalar phase is unavailable")
	}
	transaction, ok := p.plan.ScalarFactorTransaction()
	if !ok {
		return state.CoordinateFamilySkeleton{}, state.CoordinateScalarFactor{}, fmt.Errorf("factapply: RootAssignment scalar transaction is unavailable")
	}
	return p.plan.authority.domain.ApplyRootAssignmentScalarCoordinate(transaction, currentSkeleton, currentTarget, pointSource, hasPointSource)
}

// FreshEmptyPredicate is one sealed completion query answer. Program order,
// not an adapter callback, binds answers to the authority-owned query list.
type FreshEmptyPredicate struct {
	Path  pathdom.Path
	Fresh bool
}

func (p RootAssignmentFactorProgram) EvaluateFreshEmpty(skeleton state.CoordinateFamilySkeleton, value product.Value) (bool, error) {
	if !p.ownsStage(RootAssignmentFactorStageFreshEmpty) || !product.BelongsToRegistry(p.plan.authority.domain.Registry(), value) {
		return false, fmt.Errorf("factapply: invalid RootAssignment fresh-empty phase")
	}
	id, exact := identityvalue.ExactID(p.plan.authority.domain.Registry(), value)
	if !exact {
		return false, nil
	}
	return p.plan.authority.domain.CoordinateRootAssignmentFreshEmpty(skeleton, id)
}

func (p RootAssignmentFactorProgram) PrepareCompletion(reg *axis.Registry, primary product.Value, predicates []FreshEmptyPredicate) (state.RootAssignmentFactorTransaction, error) {
	if !p.ownsStage(RootAssignmentFactorStageCompletion) || reg != p.plan.authority.domain.Registry() {
		return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: RootAssignment completion phase is unavailable")
	}
	queries, err := p.plan.FactorCompletionFreshEmptyPaths()
	if err != nil {
		return state.RootAssignmentFactorTransaction{}, err
	}
	queryIndex := 0
	for index := range predicates {
		for queryIndex < len(queries) && !queries[queryIndex].Equal(predicates[index].Path) {
			queryIndex++
		}
		if queryIndex >= len(queries) {
			return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: RootAssignment completion predicate order mismatch")
		}
		queryIndex++
	}
	fresh := func(candidate pathdom.Path) bool {
		for index := range predicates {
			if predicates[index].Path.Equal(candidate) {
				return predicates[index].Fresh
			}
		}
		return false
	}
	return p.plan.PrepareFactorCompletion(reg, primary, fresh)
}

func (p RootAssignmentFactorProgram) ApplyCompletionFactor(transaction state.RootAssignmentFactorTransaction, current state.LaneFactor) (state.LaneFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageCompletion) {
		return state.LaneFactor{}, fmt.Errorf("factapply: RootAssignment completion phase is unavailable")
	}
	return p.plan.authority.domain.ApplyRootAssignmentCompletionFactor(transaction, current)
}

func (p RootAssignmentFactorProgram) ApplyCompletionCoordinate(transaction state.RootAssignmentFactorTransaction, skeleton state.CoordinateFamilySkeleton, scalar state.CoordinateScalarFactor) (state.CoordinateFamilySkeleton, state.CoordinateScalarFactor, error) {
	if !p.ownsStage(RootAssignmentFactorStageCompletion) {
		return state.CoordinateFamilySkeleton{}, state.CoordinateScalarFactor{}, fmt.Errorf("factapply: RootAssignment completion phase is unavailable")
	}
	return p.plan.authority.domain.ApplyRootAssignmentCompletionCoordinate(transaction, skeleton, scalar)
}
