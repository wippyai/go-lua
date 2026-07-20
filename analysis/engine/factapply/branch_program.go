package factapply

import (
	"context"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// branchAtomAccess is the sealed product footprint of one canonical branch
// operation. Whole-lane dependencies use ProductDomain-owned descriptors;
// Values and the registered path family retain their exact finite factors.
type branchAtomAccess struct {
	coordinateReads, coordinateWrites             []state.CoordinateSlot
	coordinateFamilyReads, coordinateFamilyWrites []state.CoordinateFamily
	valueReads, valueWrites                       []statekey.Value
	dependencyWrites                              []statekey.ValueDependency
	laneReads, laneWrites                         []state.ProductLane
	predicateActivations                          []pathPredicateActivation
	originalCoordinateReads                       []state.CoordinateSlot
	originalValueReads                            []statekey.Value
	originalLaneReads                             []state.ProductLane
}

func (a branchAtomAccess) coordinateReadsCopy() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), a.coordinateReads...)
}
func (a branchAtomAccess) coordinateWritesCopy() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), a.coordinateWrites...)
}
func (a branchAtomAccess) valueReadsCopy() []statekey.Value {
	return append([]statekey.Value(nil), a.valueReads...)
}
func (a branchAtomAccess) valueWritesCopy() []statekey.Value {
	return append([]statekey.Value(nil), a.valueWrites...)
}
func (a branchAtomAccess) laneReadsCopy() []state.ProductLane {
	return append([]state.ProductLane(nil), a.laneReads...)
}
func (a branchAtomAccess) laneWritesCopy() []state.ProductLane {
	return append([]state.ProductLane(nil), a.laneWrites...)
}
func (a branchAtomAccess) originalCoordinateReadsCopy() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), a.originalCoordinateReads...)
}
func (a branchAtomAccess) originalValueReadsCopy() []statekey.Value {
	return append([]statekey.Value(nil), a.originalValueReads...)
}
func (a branchAtomAccess) originalLaneReadsCopy() []state.ProductLane {
	return append([]state.ProductLane(nil), a.originalLaneReads...)
}

type branchAtomKernel func(branchAtomRuntime, state.State) (state.State, bool, error)

// branchAtom binds one prepared operation to the only kernel that can execute
// it. Access and Apply are sealed together; execution never dispatches on
// BranchRelationStepKind.
type branchAtom struct {
	access       branchAtomAccess
	apply        branchAtomKernel
	factor       branchAtomFactorKernel
	mutation     state.CoordinateBranchMutation
	valueRoles   branchAtomValueRoles
	presencePlan PresenceImplicationDependencyPlan
	dependencyID state.CoordinateDependencyID
	dynamic      bool
	fence        bool
	refinement   *branchValueRefinementFactor
	equality     *branchPathEqualityFactor
	pathRelation *branchPathRelationFactor
	source       BranchRelationStepKind
	seal         *branchProgramSeal
}

func (a branchAtom) accessCopy() branchAtomAccess { return cloneBranchAtomAccess(a.access) }

type branchTransactionStage struct {
	atoms []int
	seal  *branchProgramSeal
}

func (s branchTransactionStage) atomIndexes() []int {
	if s.seal == nil {
		return nil
	}
	return append([]int(nil), s.atoms...)
}

// preparedBranchTransaction is the sole executable form of a selected branch transaction.
// BranchRelationTransaction remains immutable syntax only.
type preparedBranchTransaction struct {
	transaction  BranchRelationTransaction
	atoms        []branchAtom
	factorPlans  []*branchAtomFactorPlan
	stages       []branchTransactionStage
	presencePlan PresenceImplicationDependencyPlan
	dependencies state.CoordinateDependencyPlan
	coordinates  []state.CoordinateSlot
	authority    *PathSemanticAuthority
	domain       state.ProductDomain
	seal         *branchProgramSeal
	dynamicAtom  int
	dynamicStep  int
	dynamicBound BranchRelationStep
	hasDynamic   bool
}

// BranchRelationFactors is the sealed factor/stage declaration of one
// transaction. It exposes only sealed product ownership and private-kernel
// execution; branch syntax never escapes into the transformer.
type BranchRelationFactors struct {
	prepared preparedBranchTransaction
	seal     *branchProgramSeal
}

type BranchRelationFactor struct {
	access                                branchAtomAccess
	currentValueReads, currentValueWrites int
	originalValueReads                    int
	index                                 int
	dynamic                               bool
	seal                                  *branchProgramSeal
}

type BranchRelationFactorStage struct {
	factors []int
	seal    *branchProgramSeal
}

func (f BranchRelationFactors) Len() int {
	if f.seal == nil || f.prepared.seal != f.seal {
		return 0
	}
	return len(f.prepared.atoms)
}

func (f BranchRelationFactors) Factor(index int) (BranchRelationFactor, bool) {
	if f.seal == nil || f.prepared.seal != f.seal || index < 0 || index >= len(f.prepared.atoms) {
		return BranchRelationFactor{}, false
	}
	atom := f.prepared.atoms[index]
	return BranchRelationFactor{
		access: atom.accessCopy(), currentValueReads: len(atom.valueRoles.currentReads), currentValueWrites: len(atom.valueRoles.currentWrites),
		originalValueReads: len(atom.valueRoles.originalReads), index: index, dynamic: atom.dynamic, seal: f.seal,
	}, true
}

// PresenceImplicationDependencyPlan returns the transaction-owned, sealed
// consequence topology for index. Ordinary branch factors have no such plan.
// The returned plan contains only prepared publication stages and dependency
// blocks; it exposes no branch syntax and performs no runtime discovery.
func (f BranchRelationFactors) PresenceImplicationDependencyPlan(index int) (PresenceImplicationDependencyPlan, bool) {
	if f.seal == nil || f.prepared.seal != f.seal || index < 0 || index >= len(f.prepared.atoms) {
		return PresenceImplicationDependencyPlan{}, false
	}
	plan := f.prepared.atoms[index].presencePlan
	if !plan.valid || plan.seal == nil {
		return PresenceImplicationDependencyPlan{}, false
	}
	return plan, true
}

// RequiresDynamicPresenceKey reports whether this declaration factor is the
// one variable-arity Values atom specialized by BindDynamicPresenceKey.
func (f BranchRelationFactor) RequiresDynamicPresenceKey() bool {
	return f.seal != nil && f.index >= 0 && f.dynamic
}

func (f BranchRelationFactors) Stages() []BranchRelationFactorStage {
	if f.seal == nil || f.prepared.seal != f.seal {
		return nil
	}
	out := make([]BranchRelationFactorStage, len(f.prepared.stages))
	for index, stage := range f.prepared.stages {
		out[index] = BranchRelationFactorStage{factors: stage.atomIndexes(), seal: f.seal}
	}
	return out
}

// StageIndependent proves that one sealed stage contains only pairwise
// independent factors. The producer scheduler places two atoms in the same
// stage only when neither dependency direction nor any declared read/write
// access conflicts. Consumers may therefore lower the factors to a
// deterministic sequence of global equations without changing the stage's
// parallel semantics: no factor can observe or overwrite another factor's
// contribution, and feasibility composes by conjunction.
func (f BranchRelationFactors) StageIndependent(stageIndex int) bool {
	if f.seal == nil || f.prepared.seal != f.seal || stageIndex < 0 || stageIndex >= len(f.prepared.stages) {
		return false
	}
	stage := f.prepared.stages[stageIndex]
	for leftIndex, leftAtom := range stage.atoms {
		if leftAtom < 0 || leftAtom >= len(f.prepared.atoms) {
			return false
		}
		left := f.prepared.atoms[leftAtom]
		for _, rightAtom := range stage.atoms[leftIndex+1:] {
			if rightAtom < 0 || rightAtom >= len(f.prepared.atoms) {
				return false
			}
			right := f.prepared.atoms[rightAtom]
			if f.prepared.dependencies.Depends(left.dependencyID, right.dependencyID) ||
				f.prepared.dependencies.Depends(right.dependencyID, left.dependencyID) ||
				branchAtomAccessConflict(f.prepared.domain, left.access, right.access) {
				return false
			}
		}
	}
	return true
}

// BindDynamicPresenceKey specializes the one key-dependent factor in a frozen
// declaration. All other factors, provider work, and stage boundaries are
// retained byte-for-byte; only the dynamic factor's exact coordinate access
// and kernel are derived from the bound scalar key.
func (f BranchRelationFactors) BindDynamicPresenceKey(
	reg *axis.Registry,
	key product.Value,
) (BranchRelationFactors, bool) {
	if f.seal == nil || f.prepared.seal != f.seal || reg == nil ||
		reg != f.prepared.domain.Registry() || !product.BelongsToRegistry(reg, key) {
		return BranchRelationFactors{}, false
	}
	if !f.prepared.hasDynamic || f.prepared.dynamicAtom < 0 || f.prepared.dynamicAtom >= len(f.prepared.atoms) ||
		f.prepared.dynamicStep < 0 || f.prepared.dynamicStep >= len(f.prepared.transaction.steps) {
		return BranchRelationFactors{}, false
	}
	template := cloneBranchRelationStep(f.prepared.dynamicBound)
	if template.kind != BranchRelationStepDynamicPresence || template.dynamic.keyBound {
		return BranchRelationFactors{}, false
	}
	template.dynamic.key = key
	template.dynamic.keyBound = true
	transaction := f.prepared.transaction.Clone()
	if transaction.steps[f.prepared.dynamicStep].kind != BranchRelationStepDynamicPresence {
		return BranchRelationFactors{}, false
	}
	transaction.steps[f.prepared.dynamicStep] = cloneBranchRelationStep(template)
	builder := branchProgramBuilder{
		authority:   f.prepared.authority,
		domain:      f.prepared.domain,
		transaction: transaction,
	}
	drafts, err := builder.dynamicPresenceAtoms(template)
	if err != nil || len(drafts) > 1 {
		return BranchRelationFactors{}, false
	}
	replacement := branchAtom{seal: f.seal, dynamic: true, apply: branchIdentityKernel}
	if len(drafts) == 1 {
		draft := drafts[0]
		replacement.apply = draft.apply
		replacement.access = cloneBranchAtomAccess(draft.access)
		replacement.dependencyID = f.prepared.atoms[f.prepared.dynamicAtom].dependencyID
		if draft.dependency {
			if len(draft.seed.ReadPaths) != 0 || len(draft.seed.ResolvePaths) != 0 || len(draft.seed.WritePaths) != 0 ||
				len(draft.seed.DescendantMutationRoots) != 0 || len(draft.seed.TransientEqualities) != 0 {
				return BranchRelationFactors{}, false
			}
			replacement.access.coordinateWrites = append(replacement.access.coordinateWrites, draft.seed.AddCoordinates...)
		}
	}
	prepared := f.prepared
	prepared.transaction = transaction
	prepared.atoms = append([]branchAtom(nil), f.prepared.atoms...)
	prepared.atoms[f.prepared.dynamicAtom] = replacement
	prepared.coordinates = append([]state.CoordinateSlot(nil), f.prepared.coordinates...)
	prepared.dynamicBound = cloneBranchRelationStep(template)
	return BranchRelationFactors{prepared: prepared, seal: f.seal}, true
}

func branchIdentityKernel(_ branchAtomRuntime, out state.State) (state.State, bool, error) {
	return out, true, nil
}

func (f BranchRelationFactor) CoordinateReads() []state.CoordinateSlot {
	return f.access.coordinateReadsCopy()
}
func (f BranchRelationFactor) CoordinateWrites() []state.CoordinateSlot {
	return f.access.coordinateWritesCopy()
}
func (f BranchRelationFactor) ValueReads() []statekey.Value    { return f.access.valueReadsCopy() }
func (f BranchRelationFactor) ValueWrites() []statekey.Value   { return f.access.valueWritesCopy() }
func (f BranchRelationFactor) LaneReads() []state.ProductLane  { return f.access.laneReadsCopy() }
func (f BranchRelationFactor) LaneWrites() []state.ProductLane { return f.access.laneWritesCopy() }
func (f BranchRelationFactor) OriginalCoordinateReads() []state.CoordinateSlot {
	return f.access.originalCoordinateReadsCopy()
}
func (f BranchRelationFactor) OriginalValueReads() []statekey.Value {
	return f.access.originalValueReadsCopy()
}
func (f BranchRelationFactor) OriginalLaneReads() []state.ProductLane {
	return f.access.originalLaneReadsCopy()
}
func (f BranchRelationFactor) CurrentValuesTopRead() bool {
	return len(f.access.valueReads) != 0 || len(f.access.valueWrites) != 0 || f.currentValueReads != 0 || f.currentValueWrites != 0 ||
		branchAccessHasLane(f.access.laneReads, state.LaneValues)
}
func (f BranchRelationFactor) OriginalValuesTopRead() bool {
	return len(f.access.originalValueReads) != 0 || f.originalValueReads != 0 || branchAccessHasLane(f.access.originalLaneReads, state.LaneValues)
}
func (f BranchRelationFactor) ValuesTopWrite() bool {
	return branchAccessHasLane(f.access.laneWrites, state.LaneValues)
}
func (f BranchRelationFactor) ValuesTopPreserve() bool {
	return (len(f.access.valueWrites) != 0 || f.currentValueWrites != 0) && !f.ValuesTopWrite()
}
func (f BranchRelationFactor) CurrentReachabilityRead() bool { return f.seal != nil }
func (f BranchRelationFactor) OriginalReachabilityRead() bool {
	return f.seal != nil && (len(f.access.originalCoordinateReads) != 0 ||
		len(f.access.originalValueReads) != 0 || f.originalValueReads != 0 || len(f.access.originalLaneReads) != 0)
}

func branchAccessHasLane(lanes []state.ProductLane, id state.LaneID) bool {
	for _, lane := range lanes {
		if lane.ID() == id {
			return true
		}
	}
	return false
}

func (s BranchRelationFactorStage) Factors() []int {
	if s.seal == nil {
		return nil
	}
	return append([]int(nil), s.factors...)
}

// ApplyFactor invokes the private canonical kernel for one declared factor.
// Callers are responsible for materializing only the factor's declared input
// and publishing only its declared output through their sealed carriers.
func (f BranchRelationFactors) ApplyFactor(index int, edge transfer.EdgeContext, original, input state.State) ConcreteBranchRelationResult {
	if f.seal == nil || f.prepared.seal != f.seal || index < 0 || index >= len(f.prepared.atoms) {
		return ConcreteBranchRelationResult{Output: input, Err: fmt.Errorf("factapply: invalid branch relation factor")}
	}
	if edge.Registry != f.prepared.domain.Registry() || !edge.HasCond ||
		edge.Edge.From != f.prepared.transaction.point || edge.Edge.Cond != f.prepared.transaction.cond {
		return ConcreteBranchRelationResult{Output: input, Err: fmt.Errorf("factapply: branch relation factor edge mismatch")}
	}
	if plan := f.prepared.atoms[index].presencePlan; plan.valid && plan.seal != nil {
		return ConcreteBranchRelationResult{Output: input, Err: fmt.Errorf("factapply: consequence factor requires coordinate-round execution")}
	}
	if edge.Context == nil {
		edge.Context = context.Background()
	}
	token := tokenOf(edge.Session)
	if token != nil && token.Canceled() {
		return ConcreteBranchRelationResult{Output: input, Canceled: true}
	}
	if f.prepared.atoms[index].factor != nil {
		return f.applyConcreteFactor(index, edge, original, input)
	}
	runtime := branchAtomRuntime{
		context: edge, resolver: f.prepared.authority.resolver,
		projectPath: f.prepared.authority.projectPath, typeValues: f.prepared.authority.typeValues,
		refinements: append([]factflow.BranchRefinement(nil), f.prepared.transaction.refinements...),
		point:       f.prepared.transaction.point, original: original, token: token,
	}
	next, feasible, err := f.prepared.atoms[index].apply(runtime, input)
	if err != nil {
		return ConcreteBranchRelationResult{Output: input, Canceled: err == context.Canceled, Err: err}
	}
	if !feasible {
		return ConcreteBranchRelationResult{Output: next}
	}
	return ConcreteBranchRelationResult{Output: next}
}

type branchProgramSeal struct{}

type branchAtomRuntime struct {
	context     transfer.EdgeContext
	resolver    *visibility.Resolver
	projectPath PathTypeProjector
	typeValues  *typevalue.Cache
	refinements []factflow.BranchRefinement
	point       cfg.Point
	original    state.State
	token       *cancellation.Token
}

type pathPredicateActivationKind uint8

const pathPredicateActivationTruthiness pathPredicateActivationKind = 1

type pathPredicateActivation struct {
	path   keyspace.Key
	kind   pathPredicateActivationKind
	truthy bool
}

type branchAtomDraft struct {
	seed             state.CoordinateDependencySeed
	access           branchAtomAccess
	apply            branchAtomKernel
	factor           branchAtomFactorKernel
	mutation         state.CoordinateBranchMutation
	valueRoleSources branchAtomValueRoleSources
	consequence      bool
	dependency       bool
	original         bool
	careActivation   bool
	careTruthy       bool
	dynamic          bool
	refinement       *branchValueRefinementDraft
	equality         *branchPathEqualityDraft
	pathRelation     *branchPathRelationDraft
	source           BranchRelationStepKind
}

type branchProgramBuilder struct {
	authority   *PathSemanticAuthority
	domain      state.ProductDomain
	transaction BranchRelationTransaction
	drafts      []branchAtomDraft
	post        []branchAtomDraft
	declaration bool
	nextID      state.CoordinateDependencyID
	dynamicStep BranchRelationStep
	dynamicAt   int
	hasDynamic  bool
}

// PrepareBranchRelationFactors freezes the transaction-owned factor and stage
// declaration. An unbound dynamic key is represented by the registered whole
// path family; executable preparation still requires the exact key binding.
func (a *PathSemanticAuthority) PrepareBranchRelationFactors(
	domain state.ProductDomain,
	transaction BranchRelationTransaction,
	inventory state.CoordinateFactorInventory,
) (BranchRelationFactors, error) {
	prepared, err := a.prepareBranchTransactionMode(domain, transaction, inventory, true)
	if err != nil {
		return BranchRelationFactors{}, err
	}
	return BranchRelationFactors{prepared: prepared, seal: prepared.seal}, nil
}

// PrepareFormalBranchRelationFactors prepares the same canonical atom/stage
// program in a lexical formal-root keyspace. Visibility and SSA selection are
// unchanged; only exact interned structural keys cross the sealed registered
// rekey authority. No branch syntax or semantic kernel is reinterpreted.
func (a *PathSemanticAuthority) PrepareFormalBranchRelationFactors(
	domain state.ProductDomain,
	transaction BranchRelationTransaction,
	inventory state.CoordinateFactorInventory,
	rekey state.CoordinateFormalRootRekey,
	target *keyspace.KeySpace,
) (BranchRelationFactors, error) {
	authority, err := a.ProjectFormal(domain, rekey, target)
	if err != nil {
		return BranchRelationFactors{}, err
	}
	return authority.PrepareBranchRelationFactors(domain, transaction, inventory)
}

func (a *PathSemanticAuthority) prepareBranchTransactionMode(
	domain state.ProductDomain,
	transaction BranchRelationTransaction,
	inventory state.CoordinateFactorInventory,
	declaration bool,
) (preparedBranchTransaction, error) {
	if a == nil || !a.Valid() || !domain.Valid() || domain.Registry() == nil || !transaction.ValidForRegistry(domain.Registry()) ||
		!inventory.ValidFor(domain, a.resolver.KeySpace()) {
		return preparedBranchTransaction{}, fmt.Errorf("factapply: invalid branch transaction authority")
	}
	if transaction.RequiresDynamicPresenceKey() && !declaration {
		return preparedBranchTransaction{}, fmt.Errorf("factapply: branch transaction requires its frozen dynamic key")
	}
	b := branchProgramBuilder{authority: a, domain: domain, transaction: transaction.Clone(), declaration: declaration}
	_, hasConsequenceClosure := domain.PathEvidenceCoordinateFamily()
	activeRefinements := selectActiveBranchRefinements(transaction.refinements, transaction.cond)
	for _, refinement := range activeRefinements {
		path := refinement.TargetPathRef()
		b.append(b.guardAtom(path, refinement.Refinement(), activeBranchRefinementHasStrictPrefix(activeRefinements, path)))
	}
	if len(activeRefinements) != 0 && hasConsequenceClosure {
		b.drafts = append(b.drafts, branchAtomDraft{consequence: true})
	}
	for index := 0; index < transaction.Len(); index++ {
		step, _ := transaction.Step(index)
		if step.kind == BranchRelationStepDynamicPresence {
			if b.hasDynamic {
				return preparedBranchTransaction{}, fmt.Errorf("factapply: branch transaction has multiple dynamic-presence bindings")
			}
			b.dynamicStep = cloneBranchRelationStep(step)
			b.dynamicAt = index
			b.hasDynamic = true
		}
		drafts, err := b.prepareStep(step)
		if err != nil {
			return preparedBranchTransaction{}, err
		}
		for _, draft := range drafts {
			draft.source = step.Kind()
			b.append(draft)
		}
	}
	// Every branch transaction has one final consequence barrier. Constructors
	// insert any required immediate barriers next to their action atom.
	if hasConsequenceClosure {
		b.drafts = append(b.drafts, branchAtomDraft{consequence: true})
	}
	for _, draft := range b.post {
		b.append(draft)
	}
	b.drafts = normalizeBranchConsequenceBarriers(b.drafts)
	return b.seal(inventory)
}

// normalizeBranchConsequenceBarriers gives each branch transaction the
// minimal exact consequence schedule. An original-only atom observes only the
// immutable edge input and has no product writes, so it may cross a pending
// consequence fence: its feasibility result and predicate-care activation are
// independent of every current-state write before the fence. Folding those
// atoms into the pending interval lets one consequence round observe both the
// writes and activations. Current-dependent atoms retain the fence, and
// repeated fences collapse to one.
func normalizeBranchConsequenceBarriers(drafts []branchAtomDraft) []branchAtomDraft {
	out := make([]branchAtomDraft, 0, len(drafts))
	pending := false
	flush := func() {
		if pending {
			out = append(out, branchAtomDraft{consequence: true})
			pending = false
		}
	}
	for _, draft := range drafts {
		if draft.consequence {
			pending = true
			continue
		}
		if pending && !branchOriginalDraftMayCrossConsequence(draft) {
			flush()
		}
		out = append(out, draft)
	}
	flush()
	return out
}

func branchOriginalDraftMayCrossConsequence(draft branchAtomDraft) bool {
	if !draft.original || (draft.apply == nil && draft.factor == nil) || draft.dynamic || draft.consequence ||
		len(draft.access.coordinateWrites) != 0 || len(draft.access.valueWrites) != 0 || len(draft.access.laneWrites) != 0 {
		return false
	}
	seed := draft.seed
	return len(seed.WritePaths) == 0 && len(seed.DescendantMutationRoots) == 0 &&
		len(seed.AddCoordinates) == 0 && len(seed.TransientEqualities) == 0
}

func (b *branchProgramBuilder) append(draft branchAtomDraft) {
	if draft.apply == nil && draft.factor == nil && draft.refinement == nil && draft.equality == nil && draft.pathRelation == nil && !draft.consequence {
		return
	}
	if draft.dependency {
		b.nextID++
		draft.seed.ID = b.nextID
	}
	b.drafts = append(b.drafts, draft)
}

func (b *branchProgramBuilder) key(path pathdom.Path) (keyspace.Key, bool) {
	return visibility.AddressAt(b.authority.resolver, b.transaction.point, path).RootOrVisibleKeyspaceKey()
}

// equalityKey selects the evolving SSA root even for a syntactic root path.
// Path refinements for descendants are point-visible; using the structural
// unversioned root here would make their congruence cones disjoint.
func (b *branchProgramBuilder) equalityKey(path pathdom.Path) (keyspace.Key, bool) {
	address := visibility.AddressAt(b.authority.resolver, b.transaction.point, path)
	if key, ok := address.VisibleKeyspaceKey(); ok {
		return key, true
	}
	return address.RootOrVisibleKeyspaceKey()
}

func (b *branchProgramBuilder) paths(paths ...pathdom.Path) []keyspace.Key {
	out := make([]keyspace.Key, 0, len(paths))
	for _, path := range paths {
		if key, ok := b.key(path); ok {
			out = appendBranchPath(out, key)
		}
	}
	return out
}

func (b *branchProgramBuilder) root(path pathdom.Path) (keyspace.Key, bool) {
	if path.Symbol == 0 {
		return keyspace.Key{}, false
	}
	return b.key(pathdom.Path{Symbol: path.Symbol})
}

func (b *branchProgramBuilder) lane(id state.LaneID) []state.ProductLane {
	lane, ok := b.domain.ProductLane(id)
	if !ok || lane.ID() == state.LaneValues || lane.ID() == state.LanePathEvidence {
		return nil
	}
	return []state.ProductLane{lane}
}

func (b *branchProgramBuilder) resolveLanes(paths ...pathdom.Path) []state.ProductLane {
	for _, path := range paths {
		if len(path.Segments) != 0 {
			return append([]state.ProductLane(nil), b.domain.PathResolutionLanes()...)
		}
	}
	return nil
}

func (b *branchProgramBuilder) mutationLanes() []state.ProductLane {
	topology, err := b.domain.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return nil
	}
	return topology.Lanes()
}

func (b *branchProgramBuilder) refinementAccess(path pathdom.Path, refinement factflow.ValueRefinement) (state.CoordinateDependencySeed, []state.ProductLane, []state.ProductLane) {
	key, ok := b.key(path)
	if !ok {
		return state.CoordinateDependencySeed{}, nil, nil
	}
	seed := state.CoordinateDependencySeed{ResolvePaths: []keyspace.Key{key}, WritePaths: []keyspace.Key{key}}
	reads := b.resolveLanes(path)
	var writes []state.ProductLane
	if len(path.Segments) != 0 && descendantRefinementMayNarrowPathOrigin(b.domain.Registry(), refinement) {
		if root, rootOK := b.root(path); rootOK {
			seed.WritePaths = appendBranchPath(seed.WritePaths, root)
			seed.DescendantMutationRoots = appendBranchPath(seed.DescendantMutationRoots, root)
		}
		reads = appendProductLanes(reads, b.mutationLanes()...)
		writes = appendProductLanes(writes, b.mutationLanes()...)
	} else if rootRefinementInvalidatesDescendants(b.domain.Registry(), refinement) {
		seed.DescendantMutationRoots = []keyspace.Key{key}
		reads = appendProductLanes(reads, b.mutationLanes()...)
		writes = appendProductLanes(writes, b.mutationLanes()...)
	}
	return seed, reads, writes
}

func (b *branchProgramBuilder) guardAtom(path pathdom.Path, refinement factflow.ValueRefinement, invalidatePrefix bool) branchAtomDraft {
	seed, reads, writes := b.refinementAccess(path, refinement)
	if invalidatePrefix {
		if key, ok := b.key(path); ok {
			seed.SubtreeMutationRoots = appendBranchPath(seed.SubtreeMutationRoots, key)
		}
		reads = appendProductLanes(reads, b.mutationLanes()...)
		writes = appendProductLanes(writes, b.mutationLanes()...)
	}
	role, ok := branchLexicalValueRoleSource(path.Symbol)
	if !ok {
		return branchAtomDraft{}
	}
	return branchAtomDraft{
		seed: seed, dependency: true,
		access:           branchAtomAccess{laneReads: reads, laneWrites: writes},
		valueRoleSources: branchAtomValueRoleSources{currentReads: []branchValueRoleSource{role}, currentWrites: []branchValueRoleSource{role}},
		refinement:       &branchValueRefinementDraft{path: path, refinement: refinement, invalidatePrefix: invalidatePrefix},
	}
}

func (b *branchProgramBuilder) prepareStep(step BranchRelationStep) ([]branchAtomDraft, error) {
	switch step.Kind() {
	case BranchRelationStepPresence:
		relation, _ := step.PresenceRelation()
		return b.presenceAtoms(relation), nil
	case BranchRelationStepPath:
		relation, _ := step.PathRelation()
		return b.pathRelationAtoms(relation)
	case BranchRelationStepLengthFloor:
		fact, _ := step.LengthFloor()
		atom, err := b.lengthAtom(fact)
		return []branchAtomDraft{atom}, err
	case BranchRelationStepNumericFloor:
		fact, _ := step.NumericFloor()
		atom, err := b.numericFloorAtom(fact)
		return []branchAtomDraft{atom}, err
	case BranchRelationStepNumericCeiling:
		fact, _ := step.NumericCeiling()
		atom, err := b.numericCeilingAtom(fact)
		return []branchAtomDraft{atom}, err
	case BranchRelationStepDifference:
		fact, _ := step.DifferenceConstraint()
		atom, err := b.differenceAtom(fact)
		return []branchAtomDraft{atom}, err
	case BranchRelationStepEvidence:
		proof, _ := step.PathEvidence()
		return b.evidenceAtoms(proof)
	case BranchRelationStepDynamicPresence:
		return b.dynamicPresenceAtoms(step)
	case BranchRelationStepSufficientLiteralCase:
		return nil, nil
	default:
		return nil, fmt.Errorf("factapply: unsupported branch relation step %d", step.Kind())
	}
}

func (b *branchProgramBuilder) presenceAtoms(relation factflow.BranchPresenceRelation) []branchAtomDraft {
	if refinement, selected := branchPresenceRelationStaticRefinement(
		b.domain.Registry(), b.transaction.cond, b.transaction.refinements, relation,
	); selected {
		return []branchAtomDraft{
			b.guardAtom(relation.TargetPathRef(), refinement, false),
			{consequence: true},
		}
	}
	if !branchPresenceRelationNeedsNonBooleanTrigger(b.transaction.cond, b.transaction.refinements, relation) {
		return nil
	}
	refinement := presenceRefinement(b.domain.Registry(), relation.TargetPresence())
	action := b.guardAtom(relation.TargetPathRef(), refinement, false)
	trigger := relation.TriggerPathRef()
	action.seed.ResolvePaths = append(action.seed.ResolvePaths, b.paths(trigger)...)
	action.access.laneReads = appendProductLanes(action.access.laneReads, b.resolveLanes(trigger)...)
	if role, ok := branchLexicalValueRoleSource(trigger.Symbol); ok {
		action.valueRoleSources.currentReads = append(action.valueRoleSources.currentReads, role)
	}
	action.refinement.nonBooleanTrigger = trigger
	return []branchAtomDraft{action, {consequence: true}}
}

func (b *branchProgramBuilder) pathRelationAtoms(relation factflow.BranchPathRelation) ([]branchAtomDraft, error) {
	left, right := relation.LeftPath(), relation.RightPath()
	seed := state.CoordinateDependencySeed{}
	reads, writes := []state.ProductLane(nil), []state.ProductLane(nil)
	barrier := false
	switch relation.Kind() {
	case factflow.BranchPathRelationEqual:
		seed, reads, writes = b.equalityAccess(left, right)
		seed.WritePaths = append(seed.WritePaths, b.paths(left, right)...)
		leftKey, leftOK := factKeyspaceKeyAt(b.authority.resolver, b.transaction.point, left)
		rightKey, rightOK := factKeyspaceKeyAt(b.authority.resolver, b.transaction.point, right)
		if leftOK && rightOK && leftKey != rightKey {
			seed.TransientEqualities = []state.CoordinateDependencyEquality{{Left: leftKey, Right: rightKey}}
		}
	case factflow.BranchPathRelationNotEqual:
		seed, reads, writes = b.originRelationAccess(left, right)
	case factflow.BranchPathRelationTypeMatch, factflow.BranchPathRelationTypeUnmatch:
		seed.ResolvePaths = b.paths(right, left)
		seed.WritePaths = b.paths(left)
		reads = b.resolveLanes(right, left)
		barrier = true
	default:
		return nil, fmt.Errorf("factapply: invalid branch path relation")
	}
	action := branchAtomDraft{
		seed: seed, dependency: true, access: branchAtomAccess{laneReads: reads, laneWrites: writes},
		pathRelation: &branchPathRelationDraft{kind: relation.Kind(), left: left, right: right},
	}
	for _, path := range []pathdom.Path{left, right} {
		if role, ok := branchLexicalValueRoleSource(path.Symbol); ok {
			action.valueRoleSources.currentReads = append(action.valueRoleSources.currentReads, role)
		}
	}
	writePaths := []pathdom.Path(nil)
	switch relation.Kind() {
	case factflow.BranchPathRelationEqual:
		writePaths = []pathdom.Path{left, right}
	case factflow.BranchPathRelationNotEqual:
		if len(left.Segments) != 0 {
			writePaths = append(writePaths, left)
		}
		if len(right.Segments) != 0 {
			writePaths = append(writePaths, right)
		}
	case factflow.BranchPathRelationTypeMatch, factflow.BranchPathRelationTypeUnmatch:
		writePaths = []pathdom.Path{left}
	}
	for _, path := range writePaths {
		if role, ok := branchLexicalValueRoleSource(path.Symbol); ok {
			action.valueRoleSources.currentWrites = append(action.valueRoleSources.currentWrites, role)
		}
	}
	if relation.Kind() == factflow.BranchPathRelationEqual {
		_, leftSelect := channelSelectResultPathFromChannel(left)
		_, rightSelect := channelSelectResultPathFromChannel(right)
		if !leftSelect && !rightSelect {
			if leftKey, leftOK := b.equalityKey(left); leftOK {
				if rightKey, rightOK := b.equalityKey(right); rightOK {
					if leftKey == rightKey {
						action.seed = state.CoordinateDependencySeed{}
						action.dependency = false
						action.access = branchAtomAccess{}
						action.pathRelation = nil
						action.factor = branchIdentityFactorKernel
					} else if leftRole, leftRoleOK := branchLexicalValueRoleSource(left.Symbol); leftRoleOK {
						if rightRole, rightRoleOK := branchLexicalValueRoleSource(right.Symbol); rightRoleOK {
							action.pathRelation = nil
							action.equality = &branchPathEqualityDraft{left: leftKey, right: rightKey, leftSymbol: left.Symbol, rightSymbol: right.Symbol}
							action.valueRoleSources = branchAtomValueRoleSources{
								currentReads:  []branchValueRoleSource{leftRole, rightRole},
								currentWrites: []branchValueRoleSource{leftRole, rightRole},
							}
						}
					}
				}
			}
		}
	}
	if relation.Kind() == factflow.BranchPathRelationNotEqual && b.pathIsStructuralRoot(left) && b.pathIsStructuralRoot(right) {
		action.seed = state.CoordinateDependencySeed{}
		action.dependency = false
		action.access = branchAtomAccess{}
		action.pathRelation = nil
		action.factor = branchIdentityFactorKernel
	}
	out := []branchAtomDraft{action}
	if barrier {
		out = append(out, branchAtomDraft{consequence: true})
	}
	return out, nil
}

func (b *branchProgramBuilder) pathIsStructuralRoot(path pathdom.Path) bool {
	key, ok := b.key(path)
	if !ok {
		return false
	}
	root, ok := b.authority.resolver.KeySpace().StructuralRoot(key)
	return ok && root == key
}

func (b *branchProgramBuilder) originRelationAccess(paths ...pathdom.Path) (state.CoordinateDependencySeed, []state.ProductLane, []state.ProductLane) {
	seed := state.CoordinateDependencySeed{ResolvePaths: b.paths(paths...)}
	reads := b.resolveLanes(paths...)
	writes := []state.ProductLane(nil)
	for _, path := range paths {
		if len(path.Segments) == 0 {
			continue
		}
		if root, ok := b.root(path); ok {
			seed.WritePaths = appendBranchPath(seed.WritePaths, root)
			seed.DescendantMutationRoots = appendBranchPath(seed.DescendantMutationRoots, root)
		}
		reads = appendProductLanes(reads, b.mutationLanes()...)
		writes = appendProductLanes(writes, b.mutationLanes()...)
	}
	return seed, reads, writes
}

func (b *branchProgramBuilder) equalityAccess(left, right pathdom.Path) (state.CoordinateDependencySeed, []state.ProductLane, []state.ProductLane) {
	seed, reads, writes := b.originRelationAccess(left, right)
	if _, ok := channelSelectResultPathFromChannel(left); ok {
		reads = appendProductLanes(reads, b.lane(state.LaneChannelSelect)...)
	}
	if _, ok := channelSelectResultPathFromChannel(right); ok {
		reads = appendProductLanes(reads, b.lane(state.LaneChannelSelect)...)
	}
	return seed, reads, writes
}

func (b *branchProgramBuilder) lengthAtom(fact factflow.BranchLenRefinement) (branchAtomDraft, error) {
	path := fact.ArrayPathRef()
	if path.Symbol == 0 {
		return branchAtomDraft{}, nil
	}
	key, ok := visibility.AddressAt(b.authority.resolver, b.transaction.point, path).VisibleKeyspaceKey()
	if !ok {
		return branchAtomDraft{}, nil
	}
	mutation, err := b.domain.PrepareCoordinateBranchBound(
		state.CoordinateBoundLength, state.CoordinateBoundLower,
		b.authority.resolver.KeySpace(), key, fact.Floor(),
	)
	return b.coordinateMutationAtom(mutation), err
}

func (b *branchProgramBuilder) numericFloorAtom(fact factflow.BranchNumFloorRefinement) (branchAtomDraft, error) {
	path := fact.TargetPathRef()
	if path.Symbol == 0 {
		return branchAtomDraft{}, nil
	}
	key, ok := visibility.AddressAt(b.authority.resolver, b.transaction.point, path).RootOrVisibleKeyspaceKey()
	if !ok {
		return branchAtomDraft{}, nil
	}
	mutation, err := b.domain.PrepareCoordinateBranchBound(
		state.CoordinateBoundValue, state.CoordinateBoundLower,
		b.authority.resolver.KeySpace(), key, fact.Floor(),
	)
	return b.coordinateMutationAtom(mutation), err
}

func (b *branchProgramBuilder) numericCeilingAtom(fact factflow.BranchNumCeilRefinement) (branchAtomDraft, error) {
	path := fact.TargetPathRef()
	if path.Symbol == 0 {
		return branchAtomDraft{}, nil
	}
	key, ok := visibility.AddressAt(b.authority.resolver, b.transaction.point, path).RootOrVisibleKeyspaceKey()
	if !ok {
		return branchAtomDraft{}, nil
	}
	mutation, err := b.domain.PrepareCoordinateBranchBound(
		state.CoordinateBoundValue, state.CoordinateBoundUpper,
		b.authority.resolver.KeySpace(), key, fact.Ceiling(),
	)
	return b.coordinateMutationAtom(mutation), err
}

func (b *branchProgramBuilder) differenceAtom(fact factflow.BranchDiffConstraint) (branchAtomDraft, error) {
	hi, ok := relationGraphKeyAt(b.authority.resolver, b.transaction.point, fact.HiPath(), fact.HiIsLength())
	if !ok {
		return branchAtomDraft{}, nil
	}
	lo, ok := relationGraphKeyAt(b.authority.resolver, b.transaction.point, fact.LoPath(), fact.LoIsLength())
	if !ok {
		return branchAtomDraft{}, nil
	}
	constraint := state.RelConstraint{CoA: fact.CoHi(), A: hi, C: lo, K: fact.C()}
	if fact.HasHi2() {
		constraint.B, ok = relationGraphKeyAt(b.authority.resolver, b.transaction.point, fact.Hi2Path(), fact.Hi2IsLength())
		if !ok {
			return branchAtomDraft{}, nil
		}
		constraint.CoB = fact.CoHi2()
	}
	mutation, err := b.domain.PrepareCoordinateBranchConstraint(b.authority.resolver.KeySpace(), constraint)
	return b.coordinateMutationAtom(mutation), err
}

func (b *branchProgramBuilder) coordinateMutationAtom(mutation state.CoordinateBranchMutation) branchAtomDraft {
	slot := mutation.Slot()
	return branchAtomDraft{
		dependency: true,
		access: branchAtomAccess{
			coordinateReads:        []state.CoordinateSlot{slot},
			coordinateWrites:       []state.CoordinateSlot{slot},
			coordinateFamilyWrites: []state.CoordinateFamily{slot.Family()},
		},
		factor:   branchCoordinateMutationKernel,
		mutation: mutation,
	}
}

func (b *branchProgramBuilder) evidenceAtoms(proof factflow.BranchPathEvidence) ([]branchAtomDraft, error) {
	if proof.Kind() == factflow.BranchPathEvidenceTruthy {
		return b.truthyAtoms(proof)
	}
	stateProof, ok := branchPathEvidenceAt(b.authority.resolver, b.transaction.point, proof)
	seed := state.CoordinateDependencySeed{}
	primary := b.paths(proof.PathRef())
	var secondary []keyspace.Key
	other, hasOther := proof.OtherPathRef()
	if hasOther {
		secondary = b.paths(other)
	}
	reads, writes := []state.ProductLane(nil), []state.ProductLane(nil)
	switch proof.Kind() {
	case factflow.BranchPathEvidencePresence, factflow.BranchPathEvidenceNotEqual:
	case factflow.BranchPathEvidenceEqual:
		seed, reads, writes = b.equalityAccess(proof.PathRef(), other)
		seed.WritePaths = append(seed.WritePaths, primary...)
		seed.WritePaths = append(seed.WritePaths, secondary...)
		leftKey, leftOK := factKeyspaceKeyAt(b.authority.resolver, b.transaction.point, proof.PathRef())
		rightKey, rightOK := factKeyspaceKeyAt(b.authority.resolver, b.transaction.point, other)
		if leftOK && rightOK && leftKey != rightKey {
			seed.TransientEqualities = []state.CoordinateDependencyEquality{{Left: leftKey, Right: rightKey}}
		}
		reads = appendProductLanes(reads, b.lane(state.LaneTypestates)...)
		writes = appendProductLanes(writes, b.lane(state.LaneTypestates)...)
	case factflow.BranchPathEvidenceIndexInRange:
		seed.ReadPaths = append(seed.ReadPaths, primary...)
		seed.ResolvePaths = append(seed.ResolvePaths, secondary...)
		reads = b.resolveLanes(other)
		reads = appendProductLanes(reads, b.lane(state.LaneNumCeils)...)
		writes = appendProductLanes(writes, b.lane(state.LaneNumCeils)...)
	case factflow.BranchPathEvidenceFrozenTable:
		seed.ResolvePaths = primary
		reads = appendProductLanes(b.resolveLanes(proof.PathRef()), b.lane(state.LaneFrozenTables)...)
		writes = appendProductLanes(writes, b.lane(state.LaneFrozenTables)...)
	default:
		return nil, fmt.Errorf("factapply: invalid branch path evidence")
	}
	if ok {
		slot, err := b.domain.PathBranchProofCoordinateSlot(b.authority.resolver.KeySpace(), stateProof)
		if err != nil {
			return nil, err
		}
		seed.AddCoordinates = []state.CoordinateSlot{slot}
	}
	action := branchAtomDraft{
		seed: seed, dependency: true, access: branchAtomAccess{laneReads: reads, laneWrites: writes},
		apply: func(runtime branchAtomRuntime, out state.State) (state.State, bool, error) {
			next := applyBranchIndexStaticLengthCeil(runtime.typeValues, runtime.context, runtime.resolver, runtime.projectPath, out, proof)
			next = applyBranchPathEvidence(runtime.typeValues, runtime.context, runtime.resolver, runtime.projectPath, next, proof)
			return next, !stateIsBottom(runtime.context.Registry, next), nil
		},
	}
	bindPathProofFamily := func() error {
		family, present := b.domain.PathEvidenceCoordinateFamily()
		if !present {
			return fmt.Errorf("factapply: branch proof has no path-evidence family")
		}
		action.access.coordinateFamilyReads = appendCoordinateFamilies(action.access.coordinateFamilyReads, family)
		action.access.coordinateFamilyWrites = appendCoordinateFamilies(action.access.coordinateFamilyWrites, family)
		return nil
	}
	factorNativeProof := proof.Kind() == factflow.BranchPathEvidencePresence
	if proof.Kind() == factflow.BranchPathEvidenceNotEqual {
		_, leftSelect := channelSelectResultPathFromChannel(proof.PathRef())
		other, hasOther := proof.OtherPathRef()
		_, rightSelect := channelSelectResultPathFromChannel(other)
		factorNativeProof = hasOther && !leftSelect && !rightSelect
	}
	if proof.Kind() == factflow.BranchPathEvidenceEqual {
		_, leftSelect := channelSelectResultPathFromChannel(proof.PathRef())
		other, hasOther := proof.OtherPathRef()
		_, rightSelect := channelSelectResultPathFromChannel(other)
		if ok && hasOther && !leftSelect && !rightSelect {
			leftKey, leftOK := b.equalityKey(proof.PathRef())
			rightKey, rightOK := b.equalityKey(other)
			leftRole, leftRoleOK := branchLexicalValueRoleSource(proof.PathRef().Symbol)
			rightRole, rightRoleOK := branchLexicalValueRoleSource(other.Symbol)
			if leftOK && rightOK && leftRoleOK && rightRoleOK {
				action.apply = nil
				action.valueRoleSources = branchAtomValueRoleSources{
					currentReads:  []branchValueRoleSource{leftRole, rightRole},
					currentWrites: []branchValueRoleSource{leftRole, rightRole},
				}
				if leftKey == rightKey {
					action.factor = branchPathProofKernel(stateProof)
					if err := bindPathProofFamily(); err != nil {
						return nil, err
					}
				} else {
					action.equality = &branchPathEqualityDraft{
						left: leftKey, right: rightKey,
						leftSymbol: proof.PathRef().Symbol, rightSymbol: other.Symbol,
						persistent: true,
					}
				}
			}
		}
	}
	if proof.Kind() == factflow.BranchPathEvidenceFrozenTable {
		target, targetOK := b.key(proof.PathRef())
		role, roleOK := branchLexicalValueRoleSource(proof.PathRef().Symbol)
		resolved, resolvedErr := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), target, proof.PathRef().Symbol)
		if targetOK && roleOK && resolvedErr == nil {
			action.apply = nil
			action.factor = branchFrozenTableKernel(branchFrozenTableFactor{
				path: resolved, root: proof.PathRef().Symbol,
				typeValues: b.authority.typeValues, project: b.authority.projectPath,
			})
			action.valueRoleSources.currentReads = []branchValueRoleSource{role}
		}
	}
	if ok && factorNativeProof {
		action.apply = nil
		action.factor = branchPathProofKernel(stateProof)
		if err := bindPathProofFamily(); err != nil {
			return nil, err
		}
	}
	out := []branchAtomDraft{action}
	if proof.Kind() != factflow.BranchPathEvidenceFrozenTable {
		out = append(out, branchAtomDraft{consequence: true})
	}
	return out, nil
}

func (b *branchProgramBuilder) truthyAtoms(proof factflow.BranchPathEvidence) ([]branchAtomDraft, error) {
	path := proof.PathRef()
	paths := b.paths(path)
	oppositeFalsy := !proof.ActiveOnEdge(b.transaction.cond) &&
		proof.ActiveOnEdge(!b.transaction.cond) && proof.OppositeEdgeImpliesFalsy()
	activateCare := proof.ActiveOnEdge(b.transaction.cond) || oppositeFalsy
	seed := state.CoordinateDependencySeed{ResolvePaths: paths}
	reads := b.resolveLanes(path)
	check := branchAtomDraft{
		seed: seed, dependency: true, original: true, careActivation: activateCare, careTruthy: !oppositeFalsy, access: branchAtomAccess{laneReads: reads},
	}
	if len(path.Segments) == 0 {
		role, ok := branchLexicalValueRoleSource(path.Symbol)
		if !ok {
			return nil, fmt.Errorf("factapply: truthiness root is not lexical")
		}
		check.factor = branchLexicalTruthinessKernel(!oppositeFalsy)
		check.valueRoleSources.originalReads = []branchValueRoleSource{role}
	} else {
		target, targetOK := b.key(path)
		if !targetOK {
			return nil, fmt.Errorf("factapply: descendant truthiness path is unresolved")
		}
		resolved, err := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), target, path.Symbol)
		if err != nil {
			return nil, fmt.Errorf("factapply: descendant truthiness path: %w", err)
		}
		role, roleOK := branchLexicalValueRoleSource(path.Symbol)
		if !roleOK {
			return nil, fmt.Errorf("factapply: descendant truthiness root is not lexical")
		}
		check.factor = branchDescendantTruthinessKernel(branchDescendantTruthinessFactor{
			path: resolved, root: path.Symbol, wantTruthy: !oppositeFalsy,
			typeValues: b.authority.typeValues, project: b.authority.projectPath,
		})
		check.valueRoleSources.originalReads = []branchValueRoleSource{role}
	}
	if !oppositeFalsy {
		return []branchAtomDraft{check}, nil
	}
	// Opposite-edge parent-origin recovery exists only for descendant paths.
	// Its canonical law is identity on a lexical root, so do not schedule an
	// executable atom that carries no semantic operation.
	if len(path.Segments) == 0 {
		return []branchAtomDraft{check}, nil
	}
	trueLiteral := b.authority.typeValues.FromTypeWithWitness(b.domain.Registry(), typ.LiteralBool(true))
	b.post = append(b.post, b.guardAtom(path, factflow.NewNegatedLiteralConstraint(trueLiteral), false))
	return []branchAtomDraft{check}, nil
}

func (b *branchProgramBuilder) dynamicPresenceAtoms(step BranchRelationStep) ([]branchAtomDraft, error) {
	if !step.dynamic.keyBound {
		if !b.declaration {
			return nil, fmt.Errorf("factapply: dynamic presence key is not bound")
		}
		family, ok := b.domain.PathValueFamily()
		if !ok {
			return nil, nil
		}
		lane := []state.ProductLane{family.Lane()}
		return []branchAtomDraft{{
			dependency: true,
			dynamic:    true,
			access:     branchAtomAccess{laneReads: lane, laneWrites: lane},
			apply: func(_ branchAtomRuntime, out state.State) (state.State, bool, error) {
				return out, false, fmt.Errorf("factapply: unbound dynamic-presence declaration is not executable")
			},
		}}, nil
	}
	member, ok := typevalue.ExactScalarKeySegment(b.domain.Registry(), b.authority.typeValues, step.dynamic.key)
	if !ok {
		return nil, nil
	}
	targetPath := step.dynamic.table.Append(member)
	pathKey, ok := b.key(targetPath)
	if !ok {
		return nil, nil
	}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: pathKey, Presence: presence.Present()}
	slot, err := b.domain.PathBranchProofCoordinateSlot(b.authority.resolver.KeySpace(), proof)
	if err != nil {
		return nil, err
	}
	action := branchAtomDraft{
		seed: state.CoordinateDependencySeed{AddCoordinates: []state.CoordinateSlot{slot}}, dependency: true,
		dynamic: true,
		apply: func(_ branchAtomRuntime, out state.State) (state.State, bool, error) {
			return out.AddBranchProof(proof), true, nil
		},
	}
	return []branchAtomDraft{action}, nil
}

func (b *branchProgramBuilder) seal(inventory state.CoordinateFactorInventory) (preparedBranchTransaction, error) {
	seeds := make([]state.CoordinateDependencySeed, 0)
	for _, draft := range b.drafts {
		if draft.dependency && branchDependencySeedUsesPathFamily(draft.seed) {
			seeds = append(seeds, draft.seed)
		}
	}
	var dependencies state.CoordinateDependencyPlan
	pathFamily, hasPathFamily := b.domain.PathEvidenceCoordinateFamily()
	coordinates := make([]state.CoordinateSlot, 0)
	if hasPathFamily {
		var familyErr error
		coordinates, familyErr = inventory.FamilySlots(pathFamily)
		if familyErr != nil {
			return preparedBranchTransaction{}, familyErr
		}
	}
	var err error
	if len(seeds) != 0 {
		dependencies, err = b.domain.PlanPathCoordinateDependencies(b.authority.resolver.KeySpace(), coordinates, seeds)
		if err != nil {
			return preparedBranchTransaction{}, err
		}
		coordinates = dependencies.Coordinates()
	}
	var presencePlan PresenceImplicationDependencyPlan
	if hasPathFamily {
		presencePlan, err = (PresenceImplicationPlan{
			reg: b.domain.Registry(), keys: b.authority.resolver.KeySpace(), resolver: b.authority.resolver, point: b.transaction.point,
			barriers: ConcretePresenceImplicationTrailingBarrier,
		}).dependencyBlocksFromSlots(b.domain, coordinates)
		if err != nil {
			return preparedBranchTransaction{}, err
		}
	}
	seal := new(branchProgramSeal)
	program := preparedBranchTransaction{
		transaction: b.transaction.Clone(), dependencies: dependencies,
		coordinates:  append([]state.CoordinateSlot(nil), coordinates...),
		presencePlan: presencePlan,
		authority:    b.authority, domain: b.domain, seal: seal,
	}
	coordinateUniverseSlots := inventory.Slots()
	coordinateUniverseSlots = appendBranchCoordinateSlots(b.domain, coordinateUniverseSlots, coordinates...)
	for _, draft := range b.drafts {
		coordinateUniverseSlots = appendBranchCoordinateSlots(b.domain, coordinateUniverseSlots, draft.seed.AddCoordinates...)
		coordinateUniverseSlots = appendBranchCoordinateSlots(b.domain, coordinateUniverseSlots, draft.access.coordinateReads...)
		coordinateUniverseSlots = appendBranchCoordinateSlots(b.domain, coordinateUniverseSlots, draft.access.coordinateWrites...)
	}
	coordinateUniverse, err := b.domain.SealCoordinateFactorInventory(
		b.authority.resolver.KeySpace(), coordinateUniverseSlots,
	)
	if err != nil {
		return preparedBranchTransaction{}, fmt.Errorf("factapply: branch coordinate universe: %w", err)
	}
	coordinateUniverse, err = b.domain.CloseCoordinateFactorInventory(
		b.authority.resolver.KeySpace(), coordinateUniverse,
	)
	if err != nil {
		return preparedBranchTransaction{}, fmt.Errorf("factapply: branch coordinate universe closure: %w", err)
	}
	coordinateUniverseSlots = coordinateUniverse.Slots()
	pendingConsequenceWrites := branchAtomAccess{}
	for _, draft := range b.drafts {
		atom := branchAtom{seal: seal, dynamic: draft.dynamic, source: draft.source}
		if draft.consequence {
			selected, selectErr := selectPresenceImplicationAffectedCone(b.domain, presencePlan, pendingConsequenceWrites)
			if selectErr != nil {
				return preparedBranchTransaction{}, selectErr
			}
			pendingConsequenceWrites = branchAtomAccess{}
			if !presenceImplicationPlanHasBlocks(selected) {
				continue
			}
			atom.access = cloneBranchAtomAccess(branchConsequenceAccess(b.domain, selected))
			atom.fence = true
			atom.apply = branchIdentityKernel
			atom.presencePlan = selected
		} else {
			atom.apply = draft.apply
			atom.factor = draft.factor
			if draft.refinement != nil {
				atom.refinement, err = freezeBranchValueRefinementFactor(b, coordinateUniverse, draft.refinement, seal)
				if err != nil {
					return preparedBranchTransaction{}, err
				}
				atom.factor = branchValueRefinementKernel(atom.refinement)
			}
			if draft.equality != nil {
				atom.equality, err = freezeBranchPathEqualityFactor(b, coordinateUniverse, draft.equality, seal)
				if err != nil {
					return preparedBranchTransaction{}, err
				}
				atom.factor = branchPathEqualityKernel(atom.equality)
			}
			if draft.pathRelation != nil {
				atom.pathRelation, err = freezeBranchPathRelationFactor(b, coordinateUniverse, draft.pathRelation, seal)
				if err != nil {
					return preparedBranchTransaction{}, err
				}
				atom.factor = branchPathRelationKernel(atom.pathRelation)
			}
			atom.mutation = draft.mutation
			atom.valueRoles.currentReads, err = sealBranchValueRoleSources(draft.valueRoleSources.currentReads, seal)
			if err != nil {
				return preparedBranchTransaction{}, err
			}
			atom.valueRoles.currentWrites, err = sealBranchValueRoleSources(draft.valueRoleSources.currentWrites, seal)
			if err != nil {
				return preparedBranchTransaction{}, err
			}
			atom.valueRoles.originalReads, err = sealBranchValueRoleSources(draft.valueRoleSources.originalReads, seal)
			if err != nil {
				return preparedBranchTransaction{}, err
			}
			if (atom.apply == nil) == (atom.factor == nil) {
				return preparedBranchTransaction{}, fmt.Errorf("factapply: branch atom must have exactly one semantic kernel")
			}
			atom.dependencyID = draft.seed.ID
			atom.access = cloneBranchAtomAccess(draft.access)
			if dependency, ok := dependencies.Dependency(draft.seed.ID); ok {
				bindBranchDependencyAccess(&atom.access, dependency)
				if len(dependency.MutationRegions()) != 0 {
					topology, topologyErr := b.domain.SealPathDescendantMutationFactorTopology()
					if topologyErr != nil {
						return preparedBranchTransaction{}, topologyErr
					}
					families := topology.Families()
					for _, family := range families[1:] {
						slots := make([]state.CoordinateSlot, 0)
						for _, slot := range coordinateUniverseSlots {
							if slot.Family() == family {
								slots = append(slots, slot)
							}
						}
						atom.access.coordinateReads = appendBranchCoordinateSlots(b.domain, atom.access.coordinateReads, slots...)
						atom.access.coordinateWrites = appendBranchCoordinateSlots(b.domain, atom.access.coordinateWrites, slots...)
					}
				}
			} else if branchDependencySeedUsesPathFamily(draft.seed) {
				return preparedBranchTransaction{}, fmt.Errorf("factapply: branch dependency %d is absent from its sealed plan", draft.seed.ID)
			}
			if atom.refinement != nil {
				if err := bindBranchValueRefinementAccess(&atom); err != nil {
					return preparedBranchTransaction{}, err
				}
			}
			if atom.equality != nil {
				if err := bindBranchPathEqualityAccess(&atom); err != nil {
					return preparedBranchTransaction{}, err
				}
			}
			if atom.pathRelation != nil {
				if err := bindBranchPathRelationAccess(&atom, atom.pathRelation); err != nil {
					return preparedBranchTransaction{}, err
				}
			}
			if draft.original {
				if draft.careActivation {
					for _, path := range append(append([]keyspace.Key(nil), draft.seed.ReadPaths...), draft.seed.ResolvePaths...) {
						atom.access.predicateActivations = append(atom.access.predicateActivations, pathPredicateActivation{
							path: path, kind: pathPredicateActivationTruthiness, truthy: draft.careTruthy,
						})
					}
				}
				atom.access.originalCoordinateReads = atom.access.coordinateReads
				atom.access.originalValueReads = atom.access.valueReads
				atom.access.originalLaneReads = atom.access.laneReads
				atom.access.coordinateReads = nil
				atom.access.valueReads = nil
				atom.access.laneReads = nil
			}
			if atom.factor != nil {
				if err := bindBranchFactorValueRoles(&atom); err != nil {
					return preparedBranchTransaction{}, err
				}
			}
			pendingConsequenceWrites = mergeBranchAtomAccess(pendingConsequenceWrites, atom.access)
		}
		atomIndex := len(program.atoms)
		program.atoms = append(program.atoms, atom)
		factorPlan, factorErr := sealBranchAtomFactorPlan(b.domain, b.authority.resolver.KeySpace(), atom, seal)
		if factorErr != nil {
			return preparedBranchTransaction{}, factorErr
		}
		program.factorPlans = append(program.factorPlans, factorPlan)
		if draft.dynamic {
			if !b.hasDynamic || program.hasDynamic {
				return preparedBranchTransaction{}, fmt.Errorf("factapply: dynamic-presence factor template drifted")
			}
			program.dynamicAtom = atomIndex
			program.dynamicStep = b.dynamicAt
			program.dynamicBound = cloneBranchRelationStep(b.dynamicStep)
			program.hasDynamic = true
		}
	}
	if b.hasDynamic && !b.dynamicStep.dynamic.keyBound && !program.hasDynamic {
		return preparedBranchTransaction{}, fmt.Errorf("factapply: dynamic-presence factor template is missing")
	}
	program.stages, err = scheduleBranchAtoms(b.domain, program.atoms, dependencies, seal)
	if err != nil {
		return preparedBranchTransaction{}, err
	}
	return program, nil
}

func bindBranchFactorValueRoles(atom *branchAtom) error {
	if atom == nil || atom.factor == nil {
		return nil
	}
	bind := func(concrete *[]statekey.Value, roles []BranchRelationValueRole, label string) error {
		// Formal keyspace projection replaces the lexical root with a typed
		// formal root, so the coordinate dependency has no concrete State slot.
		// The sealed lexical role remains the complete representation-neutral
		// access declaration in that mode.
		if len(*concrete) == 0 && len(roles) != 0 {
			return nil
		}
		if len(*concrete) != len(roles) {
			return fmt.Errorf("factapply: factor-native branch %s Values roles differ from concrete dependency access", label)
		}
		matched := make([]bool, len(roles))
		for _, slot := range *concrete {
			symbol, ok := statekey.ParseSymbolValue(slot)
			if !ok {
				return fmt.Errorf("factapply: factor-native branch %s Values access is not lexical", label)
			}
			found := false
			for index, role := range roles {
				if !matched[index] && role.symbol == symbol {
					matched[index], found = true, true
					break
				}
			}
			if !found {
				return fmt.Errorf("factapply: factor-native branch %s Values role mismatch", label)
			}
		}
		if atom.refinement == nil {
			*concrete = nil
		}
		return nil
	}
	if err := bind(&atom.access.valueReads, atom.valueRoles.currentReads, "current-read"); err != nil {
		return err
	}
	if err := bind(&atom.access.valueWrites, atom.valueRoles.currentWrites, "current-write"); err != nil {
		return err
	}
	return bind(&atom.access.originalValueReads, atom.valueRoles.originalReads, "original-read")
}

func presenceImplicationPlanHasBlocks(plan PresenceImplicationDependencyPlan) bool {
	for _, stage := range plan.stages {
		if len(stage.blocks) != 0 {
			return true
		}
	}
	return false
}

func branchDependencySeedUsesPathFamily(seed state.CoordinateDependencySeed) bool {
	return len(seed.ReadPaths) != 0 || len(seed.ResolvePaths) != 0 || len(seed.WritePaths) != 0 ||
		len(seed.DescendantMutationRoots) != 0 || len(seed.AddCoordinates) != 0 ||
		len(seed.SubtreeMutationRoots) != 0 ||
		len(seed.TransientEqualities) != 0
}

// selectPresenceImplicationAffectedCone retains exactly the dependency SCCs
// whose reads can observe this branch transaction's writes, plus their
// transitive downstream consumers. The relation inventory remains body-wide;
// execution is the mathematical affected cone, not a whole-inventory replay.
func selectPresenceImplicationAffectedCone(
	domain state.ProductDomain,
	plan PresenceImplicationDependencyPlan,
	seed branchAtomAccess,
) (PresenceImplicationDependencyPlan, error) {
	if !plan.valid || plan.seal == nil {
		return PresenceImplicationDependencyPlan{}, fmt.Errorf("factapply: invalid presence consequence topology")
	}
	if len(plan.source.publications) != 0 {
		return PresenceImplicationDependencyPlan{}, fmt.Errorf("factapply: branch consequence unexpectedly owns publications")
	}
	frontier := cloneBranchAtomAccess(seed)
	out := PresenceImplicationDependencyPlan{valid: true, seal: plan.seal, source: plan.source, access: plan.access, domain: plan.domain}
	for _, stage := range plan.stages {
		selected := make([]bool, len(stage.blocks))
		changed := true
		for changed {
			changed = false
			for index, block := range stage.blocks {
				feeds := branchAccessFeedsPresenceBlock(domain, frontier, block)
				if !feeds {
					for _, predecessor := range block.predecessors {
						if predecessor >= 0 && predecessor < len(selected) && selected[predecessor] {
							feeds = true
							break
						}
					}
				}
				if selected[index] || !feeds {
					continue
				}
				selected[index], changed = true, true
				frontier.coordinateWrites = appendBranchCoordinateSlots(domain, frontier.coordinateWrites, block.coordinateWrites...)
				frontier.valueWrites = appendBranchValues(frontier.valueWrites, block.ValueWrites()...)
				for _, dependency := range block.ValueWriteDependencies() {
					frontier.dependencyWrites = appendUniqueValueDependency(frontier.dependencyWrites, dependency)
				}
				if block.pathMutation {
					topology, err := domain.SealPathDescendantMutationFactorTopology()
					if err != nil {
						return PresenceImplicationDependencyPlan{}, err
					}
					frontier.laneWrites = appendProductLanes(frontier.laneWrites, topology.Lanes()...)
				}
			}
		}
		remap := make(map[int]int)
		stageOrdinal := len(out.stages)
		empty, err := domain.SealCoordinateFactorInventory(plan.source.keys, nil)
		if err != nil {
			return PresenceImplicationDependencyPlan{}, err
		}
		sealedStage := PresenceImplicationDependencyStage{
			valid: true, seal: plan.seal, ordinal: stageOrdinal,
			readInventory: empty, writeInventory: empty,
		}
		for oldIndex, block := range stage.blocks {
			if !selected[oldIndex] {
				continue
			}
			remap[oldIndex] = len(sealedStage.blocks)
			copyBlock := block
			copyBlock.stageOrdinal = stageOrdinal
			copyBlock.blockOrdinal = len(sealedStage.blocks)
			copyBlock.predicateActivations = presenceBlockPredicateActivations(frontier, block)
			copyBlock.predecessors = nil
			for _, predecessor := range block.predecessors {
				if mapped, present := remap[predecessor]; present {
					copyBlock.predecessors = append(copyBlock.predecessors, mapped)
				}
			}
			sealedStage.blocks = append(sealedStage.blocks, copyBlock)
			out.slots = appendBranchCoordinateSlots(domain, out.slots, block.coordinateReads...)
			out.slots = appendBranchCoordinateSlots(domain, out.slots, block.coordinateWrites...)
		}
		out.stages = append(out.stages, sealedStage)
	}
	if err := sortPresenceCoordinateSlots(domain, out.slots); err != nil {
		return PresenceImplicationDependencyPlan{}, err
	}
	return out, nil
}

func branchAccessFeedsPresenceBlock(
	domain state.ProductDomain,
	access branchAtomAccess,
	block PresenceImplicationDependencyBlock,
) bool {
	for _, activation := range access.predicateActivations {
		for _, row := range block.rows {
			if activation.path == row.Trigger || row.HasTriggerPathEqual && activation.path == row.TriggerOther {
				return true
			}
		}
	}
	for _, write := range access.valueWrites {
		for _, candidate := range append(append([]statekey.Value(nil), block.ValueReads()...), block.ValueWrites()...) {
			if write == candidate {
				return true
			}
		}
	}
	for _, write := range access.dependencyWrites {
		for _, candidate := range append(append([]statekey.ValueDependency(nil), block.valueReads...), block.valueWrites...) {
			if write == candidate {
				return true
			}
		}
	}
	for _, lane := range access.laneWrites {
		if lane.ID() == state.LaneValues && (len(block.valueReads) != 0 || len(block.valueWrites) != 0) {
			return true
		}
		for _, candidate := range append(append([]state.CoordinateSlot(nil), block.coordinateReads...), block.coordinateWrites...) {
			if lane.Ordinal() == candidate.Family().Lane().Ordinal() {
				return true
			}
		}
	}
	for _, write := range access.coordinateWrites {
		for _, candidate := range append(append([]state.CoordinateSlot(nil), block.coordinateReads...), block.coordinateWrites...) {
			equal, err := domain.CoordinateSlotEqual(write, candidate)
			if err == nil && equal {
				return true
			}
		}
	}
	return false
}

func presenceBlockPredicateActivations(access branchAtomAccess, block PresenceImplicationDependencyBlock) []pathPredicateActivation {
	var out []pathPredicateActivation
	for _, activation := range access.predicateActivations {
		for _, row := range block.rows {
			if activation.path != row.Trigger && (!row.HasTriggerPathEqual || activation.path != row.TriggerOther) {
				continue
			}
			duplicate := false
			for _, prior := range out {
				duplicate = duplicate || prior == activation
			}
			if !duplicate {
				out = append(out, activation)
			}
			break
		}
	}
	return out
}

func branchConsequenceAccess(domain state.ProductDomain, plan PresenceImplicationDependencyPlan) branchAtomAccess {
	out := branchAtomAccess{}
	if _, enabled := domain.PathEvidenceCoordinateFamily(); !enabled {
		return out
	}
	mutatesPath := false
	for _, stage := range plan.Stages() {
		for _, block := range stage.Blocks() {
			out.coordinateReads = appendBranchCoordinateSlots(domain, out.coordinateReads, block.CoordinateReads()...)
			out.coordinateWrites = appendBranchCoordinateSlots(domain, out.coordinateWrites, block.CoordinateWrites()...)
			out.valueReads = appendBranchValues(out.valueReads, block.ValueReads()...)
			out.valueWrites = appendBranchValues(out.valueWrites, block.ValueWrites()...)
			mutatesPath = mutatesPath || block.PathMutation()
		}
	}
	if mutatesPath {
		topology, err := domain.SealPathDescendantMutationFactorTopology()
		if err == nil {
			out.laneReads = appendProductLanes(out.laneReads, topology.Lanes()...)
			out.laneWrites = appendProductLanes(out.laneWrites, topology.Lanes()...)
		}
	}
	return out
}

func mergeBranchAtomAccess(left, right branchAtomAccess) branchAtomAccess {
	left.coordinateReads = append(left.coordinateReads, right.coordinateReads...)
	left.coordinateWrites = append(left.coordinateWrites, right.coordinateWrites...)
	left.coordinateFamilyReads = appendCoordinateFamilies(left.coordinateFamilyReads, right.coordinateFamilyReads...)
	left.coordinateFamilyWrites = appendCoordinateFamilies(left.coordinateFamilyWrites, right.coordinateFamilyWrites...)
	left.valueReads = appendBranchValues(left.valueReads, right.valueReads...)
	left.valueWrites = appendBranchValues(left.valueWrites, right.valueWrites...)
	for _, dependency := range right.dependencyWrites {
		left.dependencyWrites = appendUniqueValueDependency(left.dependencyWrites, dependency)
	}
	left.laneReads = appendProductLanes(left.laneReads, right.laneReads...)
	left.laneWrites = appendProductLanes(left.laneWrites, right.laneWrites...)
	left.predicateActivations = append(left.predicateActivations, right.predicateActivations...)
	left.originalCoordinateReads = append(left.originalCoordinateReads, right.originalCoordinateReads...)
	left.originalValueReads = appendBranchValues(left.originalValueReads, right.originalValueReads...)
	left.originalLaneReads = appendProductLanes(left.originalLaneReads, right.originalLaneReads...)
	return left
}

func bindBranchDependencyAccess(access *branchAtomAccess, dependency state.CoordinateDependency) {
	access.coordinateReads = append(access.coordinateReads, dependency.CoordinateReads()...)
	access.coordinateWrites = append(access.coordinateWrites, dependency.CoordinateWrites()...)
	for _, location := range dependency.LocationReads() {
		if location.IsRoot() {
			if concrete, ok := location.Root.Concrete(); ok {
				access.valueReads = appendBranchValues(access.valueReads, concrete)
			}
		}
	}
	for _, location := range dependency.LocationWrites() {
		if location.IsRoot() {
			if concrete, ok := location.Root.Concrete(); ok {
				access.valueWrites = appendBranchValues(access.valueWrites, concrete)
			}
		}
	}
}

func scheduleBranchAtoms(domain state.ProductDomain, atoms []branchAtom, dependencies state.CoordinateDependencyPlan, seal *branchProgramSeal) ([]branchTransactionStage, error) {
	stages := make([]branchTransactionStage, 0)
	stageOf := make([]int, len(atoms))
	floor := 0
	for index, atom := range atoms {
		if atom.fence {
			stageOf[index] = len(stages)
			stages = append(stages, branchTransactionStage{atoms: []int{index}, seal: seal})
			floor = stageOf[index] + 1
			continue
		}
		stageIndex := floor
		for priorIndex := 0; priorIndex < index; priorIndex++ {
			if stageOf[priorIndex] < floor {
				continue
			}
			prior := atoms[priorIndex]
			if dependencies.Depends(prior.dependencyID, atom.dependencyID) ||
				dependencies.Depends(atom.dependencyID, prior.dependencyID) ||
				branchAtomAccessConflict(domain, prior.access, atom.access) {
				if after := stageOf[priorIndex] + 1; after > stageIndex {
					stageIndex = after
				}
			}
		}
		for len(stages) <= stageIndex {
			stages = append(stages, branchTransactionStage{seal: seal})
		}
		stages[stageIndex].atoms = append(stages[stageIndex].atoms, index)
		stageOf[index] = stageIndex
	}
	return stages, nil
}

func branchAtomAccessConflict(domain state.ProductDomain, left, right branchAtomAccess) bool {
	if productLaneWritesConflict(left.laneWrites, right.laneReads, right.laneWrites) || productLaneWritesConflict(right.laneWrites, left.laneReads, left.laneWrites) {
		return true
	}
	if branchValuesWriteConflict(left.valueWrites, right.valueReads, right.valueWrites) || branchValuesWriteConflict(right.valueWrites, left.valueReads, left.valueWrites) {
		return true
	}
	if branchLaneCoordinateConflict(left.laneReads, left.laneWrites, right.coordinateReads, right.coordinateWrites, right.coordinateFamilyReads, right.coordinateFamilyWrites) ||
		branchLaneCoordinateConflict(right.laneReads, right.laneWrites, left.coordinateReads, left.coordinateWrites, left.coordinateFamilyReads, left.coordinateFamilyWrites) ||
		branchLaneValuesConflict(left.laneReads, left.laneWrites, right.valueReads, right.valueWrites) ||
		branchLaneValuesConflict(right.laneReads, right.laneWrites, left.valueReads, left.valueWrites) {
		return true
	}
	if branchCoordinateFamilyWriteConflict(left.coordinateFamilyWrites, right.coordinateFamilyReads, right.coordinateFamilyWrites, right.coordinateReads, right.coordinateWrites) ||
		branchCoordinateFamilyWriteConflict(right.coordinateFamilyWrites, left.coordinateFamilyReads, left.coordinateFamilyWrites, left.coordinateReads, left.coordinateWrites) ||
		branchCoordinateFamilyReadSlotWriteConflict(left.coordinateFamilyReads, right.coordinateWrites) ||
		branchCoordinateFamilyReadSlotWriteConflict(right.coordinateFamilyReads, left.coordinateWrites) {
		return true
	}
	for _, write := range left.coordinateWrites {
		for _, candidate := range append(append([]state.CoordinateSlot(nil), right.coordinateReads...), right.coordinateWrites...) {
			equal, err := domain.CoordinateSlotEqual(write, candidate)
			if err == nil && equal {
				return true
			}
		}
	}
	for _, write := range right.coordinateWrites {
		for _, candidate := range append(append([]state.CoordinateSlot(nil), left.coordinateReads...), left.coordinateWrites...) {
			equal, err := domain.CoordinateSlotEqual(write, candidate)
			if err == nil && equal {
				return true
			}
		}
	}
	return false
}

func branchLaneCoordinateConflict(reads, writes []state.ProductLane, coordinateReads, coordinateWrites []state.CoordinateSlot, familyReads, familyWrites []state.CoordinateFamily) bool {
	for _, lane := range writes {
		for _, slot := range append(append([]state.CoordinateSlot(nil), coordinateReads...), coordinateWrites...) {
			if slot.Family().Lane() == lane {
				return true
			}
		}
		for _, family := range familyReads {
			if family.Lane() == lane {
				return true
			}
		}
		for _, family := range familyWrites {
			if family.Lane() == lane {
				return true
			}
		}
	}
	for _, lane := range reads {
		for _, slot := range coordinateWrites {
			if slot.Family().Lane() == lane {
				return true
			}
		}
		for _, family := range familyWrites {
			if family.Lane() == lane {
				return true
			}
		}
	}
	return false
}

// branchCoordinateFamilyWriteConflict enforces skeleton ownership. A family
// write replaces or reconciles the family's image, so it conflicts with every
// read or write of that family, including exact scalar slots. Exact scalar
// writes do not pass through this function and remain independently schedulable
// when their slots are disjoint.
func branchCoordinateFamilyWriteConflict(writes, familyReads, familyWrites []state.CoordinateFamily, coordinateReads, coordinateWrites []state.CoordinateSlot) bool {
	for _, write := range writes {
		for _, family := range familyReads {
			if write == family {
				return true
			}
		}
		for _, family := range familyWrites {
			if write == family {
				return true
			}
		}
		for _, slot := range coordinateReads {
			if write == slot.Family() {
				return true
			}
		}
		for _, slot := range coordinateWrites {
			if write == slot.Family() {
				return true
			}
		}
	}
	return false
}

func branchCoordinateFamilyReadSlotWriteConflict(reads []state.CoordinateFamily, writes []state.CoordinateSlot) bool {
	for _, read := range reads {
		for _, write := range writes {
			if read == write.Family() {
				return true
			}
		}
	}
	return false
}

func branchLaneValuesConflict(reads, writes []state.ProductLane, valueReads, valueWrites []statekey.Value) bool {
	for _, lane := range writes {
		if lane.ID() == state.LaneValues && (len(valueReads) != 0 || len(valueWrites) != 0) {
			return true
		}
	}
	for _, lane := range reads {
		if lane.ID() == state.LaneValues && len(valueWrites) != 0 {
			return true
		}
	}
	return false
}

func productLaneWritesConflict(writes, reads, otherWrites []state.ProductLane) bool {
	for _, write := range writes {
		for _, candidate := range append(append([]state.ProductLane(nil), reads...), otherWrites...) {
			if write == candidate {
				return true
			}
		}
	}
	return false
}

func branchValuesWriteConflict(writes, reads, otherWrites []statekey.Value) bool {
	for _, write := range writes {
		for _, candidate := range append(append([]statekey.Value(nil), reads...), otherWrites...) {
			if write == candidate {
				return true
			}
		}
	}
	return false
}

func cloneBranchAtomAccess(in branchAtomAccess) branchAtomAccess {
	return branchAtomAccess{
		coordinateReads: append([]state.CoordinateSlot(nil), in.coordinateReads...), coordinateWrites: append([]state.CoordinateSlot(nil), in.coordinateWrites...),
		coordinateFamilyReads: append([]state.CoordinateFamily(nil), in.coordinateFamilyReads...), coordinateFamilyWrites: append([]state.CoordinateFamily(nil), in.coordinateFamilyWrites...),
		valueReads: append([]statekey.Value(nil), in.valueReads...), valueWrites: append([]statekey.Value(nil), in.valueWrites...),
		dependencyWrites: append([]statekey.ValueDependency(nil), in.dependencyWrites...),
		laneReads:        append([]state.ProductLane(nil), in.laneReads...), laneWrites: append([]state.ProductLane(nil), in.laneWrites...),
		predicateActivations:    append([]pathPredicateActivation(nil), in.predicateActivations...),
		originalCoordinateReads: append([]state.CoordinateSlot(nil), in.originalCoordinateReads...),
		originalValueReads:      append([]statekey.Value(nil), in.originalValueReads...),
		originalLaneReads:       append([]state.ProductLane(nil), in.originalLaneReads...),
	}
}

func appendCoordinateFamilies(out []state.CoordinateFamily, values ...state.CoordinateFamily) []state.CoordinateFamily {
	for _, value := range values {
		found := false
		for _, prior := range out {
			found = found || prior == value
		}
		if !found {
			out = append(out, value)
		}
	}
	return out
}

func appendProductLanes(out []state.ProductLane, values ...state.ProductLane) []state.ProductLane {
	for _, value := range values {
		present := false
		for _, prior := range out {
			present = present || prior == value
		}
		if !present {
			out = append(out, value)
		}
	}
	return out
}

func appendBranchPath(out []keyspace.Key, value keyspace.Key) []keyspace.Key {
	for _, prior := range out {
		if prior == value {
			return out
		}
	}
	return append(out, value)
}

func appendBranchValues(out []statekey.Value, values ...statekey.Value) []statekey.Value {
	for _, value := range values {
		present := false
		for _, prior := range out {
			present = present || prior == value
		}
		if !present {
			out = append(out, value)
		}
	}
	return out
}

func appendBranchCoordinateSlots(domain state.ProductDomain, out []state.CoordinateSlot, values ...state.CoordinateSlot) []state.CoordinateSlot {
	for _, value := range values {
		present := false
		for _, prior := range out {
			equal, err := domain.CoordinateSlotEqual(prior, value)
			present = present || err == nil && equal
		}
		if !present {
			out = append(out, value)
		}
	}
	return out
}
