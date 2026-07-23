package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchRelationStepKind identifies one member of the ordered E3 branch
// relation transaction. The order is semantic: presence implications observe
// E1 refinements, path relations observe those presence results, and
// sufficient literal cases remain converse proof metadata rather than being
// misapplied as edge-implies-value refinements.
type BranchRelationStepKind uint8

const (
	BranchRelationStepInvalid BranchRelationStepKind = iota
	BranchRelationStepPresence
	BranchRelationStepPath
	BranchRelationStepLengthFloor
	BranchRelationStepNumericFloor
	BranchRelationStepNumericCeiling
	BranchRelationStepDifference
	BranchRelationStepEvidence
	BranchRelationStepDynamicPresence
	BranchRelationStepSufficientLiteralCase
)

// BranchRelationStep is immutable syntax. Exactly one payload is selected by
// Kind; it never contains State, a provider, or solve-local scratch.
type BranchRelationStep struct {
	kind       BranchRelationStepKind
	presence   factflow.BranchPresenceRelation
	path       factflow.BranchPathRelation
	length     factflow.BranchLenRefinement
	numFloor   factflow.BranchNumFloorRefinement
	numCeiling factflow.BranchNumCeilRefinement
	difference factflow.BranchDiffConstraint
	evidence   factflow.BranchPathEvidence
	sufficient factflow.BranchSufficientLiteralCase
	dynamic    branchDynamicPresenceProof
}

type branchDynamicPresenceProof struct {
	table    pathdom.Path
	key      product.Value
	keyBound bool
}

func (s BranchRelationStep) Kind() BranchRelationStepKind { return s.kind }

func (s BranchRelationStep) PresenceRelation() (factflow.BranchPresenceRelation, bool) {
	if s.kind != BranchRelationStepPresence {
		return factflow.BranchPresenceRelation{}, false
	}
	return factflow.NewBranchPresenceRelation(
		s.presence.TriggerPath(), s.presence.TriggerPresence(),
		s.presence.TargetPath(), s.presence.TargetPresence(),
	), true
}

func (s BranchRelationStep) PathRelation() (factflow.BranchPathRelation, bool) {
	if s.kind != BranchRelationStepPath {
		return factflow.BranchPathRelation{}, false
	}
	left, right := s.path.LeftPath(), s.path.RightPath()
	activeTrue, activeFalse := s.path.ActiveOnEdge(true), s.path.ActiveOnEdge(false)
	switch s.path.Kind() {
	case factflow.BranchPathRelationEqual:
		return factflow.NewBranchPathEquality(left, right, activeTrue, activeFalse), true
	case factflow.BranchPathRelationNotEqual:
		return factflow.NewBranchPathInequality(left, right, activeTrue, activeFalse), true
	case factflow.BranchPathRelationTypeMatch:
		return factflow.NewBranchPathTypeMatch(left, right, activeTrue, activeFalse), true
	case factflow.BranchPathRelationTypeUnmatch:
		return factflow.NewBranchPathTypeUnmatch(left, right, activeTrue, activeFalse), true
	default:
		return factflow.BranchPathRelation{}, false
	}
}

func (s BranchRelationStep) LengthFloor() (factflow.BranchLenRefinement, bool) {
	if s.kind != BranchRelationStepLengthFloor {
		return factflow.BranchLenRefinement{}, false
	}
	return factflow.NewBranchLenRefinementOnEdge(s.length.ArrayPath(), s.length.Floor(), s.length.Cond()), true
}

func (s BranchRelationStep) NumericFloor() (factflow.BranchNumFloorRefinement, bool) {
	if s.kind != BranchRelationStepNumericFloor {
		return factflow.BranchNumFloorRefinement{}, false
	}
	return factflow.NewBranchNumFloorRefinementOnEdge(s.numFloor.TargetPath(), s.numFloor.Floor(), s.numFloor.Cond()), true
}

func (s BranchRelationStep) NumericCeiling() (factflow.BranchNumCeilRefinement, bool) {
	if s.kind != BranchRelationStepNumericCeiling {
		return factflow.BranchNumCeilRefinement{}, false
	}
	return factflow.NewBranchNumCeilRefinementOnEdge(s.numCeiling.TargetPath(), s.numCeiling.Ceiling(), s.numCeiling.Cond()), true
}

func (s BranchRelationStep) DifferenceConstraint() (factflow.BranchDiffConstraint, bool) {
	if s.kind != BranchRelationStepDifference {
		return factflow.BranchDiffConstraint{}, false
	}
	return factflow.NewBranchScaledConstraintOnEdge(
		s.difference.CoHi(), s.difference.HiPath(), s.difference.HiIsLength(),
		s.difference.CoHi2(), s.difference.Hi2Path(), s.difference.Hi2IsLength(),
		s.difference.LoPath(), s.difference.LoIsLength(), s.difference.C(), s.difference.Cond(),
	), true
}

func (s BranchRelationStep) PathEvidence() (factflow.BranchPathEvidence, bool) {
	if s.kind != BranchRelationStepEvidence {
		return factflow.BranchPathEvidence{}, false
	}
	return cloneBranchPathEvidence(s.evidence), true
}

func (s BranchRelationStep) SufficientLiteralCase() (factflow.BranchSufficientLiteralCase, bool) {
	if s.kind != BranchRelationStepSufficientLiteralCase {
		return factflow.BranchSufficientLiteralCase{}, false
	}
	return factflow.NewBranchSufficientLiteralCase(
		s.sufficient.TargetPath(), s.sufficient.LiteralValue(), s.sufficient.Edge(),
	), true
}

// DynamicPresenceTable returns the table path whose exact key is bound at the
// application boundary. It exposes immutable transaction syntax for static
// Values-access derivation without exposing the mutable key binding.
func (s BranchRelationStep) DynamicPresenceTable() (pathdom.Path, bool) {
	if s.kind != BranchRelationStepDynamicPresence {
		return pathdom.Path{}, false
	}
	return s.dynamic.table.Clone(), true
}

// WithDynamicPresenceProof adds the E3 proof implied by a truthy dynamic
// table read. The key remains a symbolic boundary input until the route owner
// binds it immediately before execution.
func (t BranchRelationTransaction) WithDynamicPresenceProof(table pathdom.Path) (BranchRelationTransaction, bool) {
	if table.IsEmpty() || table.Symbol == 0 || table.Version != 0 {
		return BranchRelationTransaction{}, false
	}
	for _, step := range t.steps {
		if step.kind == BranchRelationStepDynamicPresence {
			// One selected branch condition has exactly one dynamic key. A
			// second step would incorrectly share the first step's binding.
			return BranchRelationTransaction{}, false
		}
	}
	t.steps = append(append([]BranchRelationStep(nil), t.steps...), BranchRelationStep{
		kind:    BranchRelationStepDynamicPresence,
		dynamic: branchDynamicPresenceProof{table: table.Clone()},
	})
	return t, true
}

// RequiresDynamicPresenceKey reports that the route must bind its exact key
// ValueTerm before applying this transaction.
func (t BranchRelationTransaction) RequiresDynamicPresenceKey() bool {
	for _, step := range t.steps {
		if step.kind == BranchRelationStepDynamicPresence && !step.dynamic.keyBound {
			return true
		}
	}
	return false
}

// BindDynamicPresenceKey returns an owned executable transaction. A broad key
// is still a valid binding: execution simply cannot derive one static path
// proof from it and therefore publishes none.
func (t BranchRelationTransaction) BindDynamicPresenceKey(reg *axis.Registry, key product.Value) (BranchRelationTransaction, bool) {
	if reg == nil || !product.BelongsToRegistry(reg, key) || !t.RequiresDynamicPresenceKey() {
		return BranchRelationTransaction{}, false
	}
	steps := append([]BranchRelationStep(nil), t.steps...)
	for index := range steps {
		if steps[index].kind == BranchRelationStepDynamicPresence {
			steps[index].dynamic.key = key
			steps[index].dynamic.keyBound = true
		}
	}
	t.steps = steps
	return t, true
}

// BranchRelationTransaction is the complete immutable E3 branch relation
// program for one selected CFG edge. Refinements are retained only as the
// canonical trigger proof consumed by presence relations; they are not
// replayed by this transaction.
type BranchRelationTransaction struct {
	point       cfg.Point
	cond        bool
	refinements []factflow.BranchRefinement
	steps       []BranchRelationStep
}

// PlanBranchRelationTransaction freezes the exact concrete occurrence rules:
// every presence relation is considered on both edges, path relations are
// selected by ActiveOnEdge, and sufficient cases are retained on the edge they
// prove sufficient. Facts already returns detached immutable payloads.
func PlanBranchRelationTransaction(facts factflow.Facts, point cfg.Point, cond bool) BranchRelationTransaction {
	branch := NewBranchAlgebra(facts, point)
	presence := facts.BranchPresenceRelations(point)
	paths := branch.ActivePathRelations(cond)
	cases := branch.SufficientLiteralCases()
	lengths := facts.BranchLenRefinements(point)
	floors := facts.BranchNumFloorRefinements(point)
	ceilings := facts.BranchNumCeilRefinements(point)
	differences := facts.BranchDiffConstraints(point)
	steps := make([]BranchRelationStep, 0, len(presence)+len(paths)+len(lengths)+len(floors)+len(ceilings)+len(differences)+len(cases))
	for _, fact := range lengths {
		if fact.Cond() == cond {
			steps = append(steps, BranchRelationStep{kind: BranchRelationStepLengthFloor, length: fact})
		}
	}
	for _, fact := range floors {
		if fact.Cond() == cond {
			steps = append(steps, BranchRelationStep{kind: BranchRelationStepNumericFloor, numFloor: fact})
		}
	}
	for _, fact := range ceilings {
		if fact.Cond() == cond {
			steps = append(steps, BranchRelationStep{kind: BranchRelationStepNumericCeiling, numCeiling: fact})
		}
	}
	for _, fact := range differences {
		if fact.Cond() == cond {
			steps = append(steps, BranchRelationStep{kind: BranchRelationStepDifference, difference: fact})
		}
	}
	for _, relation := range presence {
		steps = append(steps, BranchRelationStep{kind: BranchRelationStepPresence, presence: relation})
	}
	for _, relation := range paths {
		steps = append(steps, BranchRelationStep{kind: BranchRelationStepPath, path: relation})
	}
	branch.ForEachPathEvidence(func(proof factflow.BranchPathEvidence) bool {
		if proof.ActiveOnEdge(cond) ||
			(proof.Kind() == factflow.BranchPathEvidenceTruthy && proof.OppositeEdgeImpliesFalsy() && proof.ActiveOnEdge(!cond)) {
			steps = append(steps, BranchRelationStep{kind: BranchRelationStepEvidence, evidence: proof})
		}
		return true
	})
	for _, literalCase := range cases {
		// A sufficient case is converse metadata (value implies its recorded
		// edge), so outcome publication needs the whole partition even while
		// compiling one selected edge.
		steps = append(steps, BranchRelationStep{kind: BranchRelationStepSufficientLiteralCase, sufficient: literalCase})
	}
	return BranchRelationTransaction{
		point: point, cond: cond, refinements: branch.Refinements(), steps: steps,
	}
}

func (t BranchRelationTransaction) Point() cfg.Point { return t.point }
func (t BranchRelationTransaction) Cond() bool       { return t.cond }
func (t BranchRelationTransaction) Len() int         { return len(t.steps) }

// Clone returns a deeply detached copy suitable for sealed boundary syntax.
func (t BranchRelationTransaction) Clone() BranchRelationTransaction {
	t.refinements = append([]factflow.BranchRefinement(nil), t.refinements...)
	for index, refinement := range t.refinements {
		trueValue, hasTrue := refinement.TrueValue()
		falseValue, hasFalse := refinement.FalseValue()
		t.refinements[index] = factflow.NewBranchRefinement(
			refinement.TargetPath(), trueValue, hasTrue, falseValue, hasFalse,
		)
	}
	t.steps = append([]BranchRelationStep(nil), t.steps...)
	for index := range t.steps {
		t.steps[index] = cloneBranchRelationStep(t.steps[index])
	}
	return t
}

func cloneBranchRelationStep(step BranchRelationStep) BranchRelationStep {
	switch step.kind {
	case BranchRelationStepPresence:
		step.presence = factflow.NewBranchPresenceRelation(
			step.presence.TriggerPath(), step.presence.TriggerPresence(), step.presence.TargetPath(), step.presence.TargetPresence(),
		)
	case BranchRelationStepPath:
		left, right := step.path.LeftPath(), step.path.RightPath()
		activeTrue, activeFalse := step.path.ActiveOnEdge(true), step.path.ActiveOnEdge(false)
		switch step.path.Kind() {
		case factflow.BranchPathRelationEqual:
			step.path = factflow.NewBranchPathEquality(left, right, activeTrue, activeFalse)
		case factflow.BranchPathRelationNotEqual:
			step.path = factflow.NewBranchPathInequality(left, right, activeTrue, activeFalse)
		case factflow.BranchPathRelationTypeMatch:
			step.path = factflow.NewBranchPathTypeMatch(left, right, activeTrue, activeFalse)
		case factflow.BranchPathRelationTypeUnmatch:
			step.path = factflow.NewBranchPathTypeUnmatch(left, right, activeTrue, activeFalse)
		}
	case BranchRelationStepLengthFloor:
		step.length = factflow.NewBranchLenRefinementOnEdge(step.length.ArrayPath(), step.length.Floor(), step.length.Cond())
	case BranchRelationStepNumericFloor:
		step.numFloor = factflow.NewBranchNumFloorRefinementOnEdge(step.numFloor.TargetPath(), step.numFloor.Floor(), step.numFloor.Cond())
	case BranchRelationStepNumericCeiling:
		step.numCeiling = factflow.NewBranchNumCeilRefinementOnEdge(step.numCeiling.TargetPath(), step.numCeiling.Ceiling(), step.numCeiling.Cond())
	case BranchRelationStepDifference:
		step.difference, _ = step.DifferenceConstraint()
	case BranchRelationStepEvidence:
		step.evidence = cloneBranchPathEvidence(step.evidence)
	case BranchRelationStepDynamicPresence:
		step.dynamic.table = step.dynamic.table.Clone()
	case BranchRelationStepSufficientLiteralCase:
		step.sufficient = factflow.NewBranchSufficientLiteralCase(
			step.sufficient.TargetPath(), step.sufficient.LiteralValue(), step.sufficient.Edge(),
		)
	}
	return step
}

func cloneBranchPathEvidence(proof factflow.BranchPathEvidence) factflow.BranchPathEvidence {
	cond := proof.ActiveOnEdge(true)
	switch proof.Kind() {
	case factflow.BranchPathEvidencePresence:
		value, ok := proof.Presence()
		if !ok {
			return factflow.BranchPathEvidence{}
		}
		return factflow.NewBranchPathPresenceEvidenceOnEdge(proof.Path(), value, cond)
	case factflow.BranchPathEvidenceEqual:
		other, ok := proof.OtherPath()
		if !ok {
			return factflow.BranchPathEvidence{}
		}
		return factflow.NewBranchPathEqualityEvidenceOnEdge(proof.Path(), other, cond)
	case factflow.BranchPathEvidenceNotEqual:
		other, ok := proof.OtherPath()
		if !ok {
			return factflow.BranchPathEvidence{}
		}
		return factflow.NewBranchPathInequalityEvidenceOnEdge(proof.Path(), other, cond)
	case factflow.BranchPathEvidenceTruthy:
		if proof.OppositeEdgeImpliesFalsy() {
			return factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(proof.Path(), cond)
		}
		return factflow.NewBranchPathTruthyEvidenceOnEdge(proof.Path(), cond)
	case factflow.BranchPathEvidenceIndexInRange:
		other, ok := proof.OtherPath()
		if !ok {
			return factflow.BranchPathEvidence{}
		}
		return factflow.NewBranchIndexInRangeEvidenceOnEdge(proof.Path(), other, cond)
	case factflow.BranchPathEvidenceFrozenTable:
		producer, ok := proof.ProducerPoint()
		if !ok {
			return factflow.BranchPathEvidence{}
		}
		return factflow.NewBranchFrozenTableEvidenceOnEdge(proof.Path(), producer, cond)
	default:
		return factflow.BranchPathEvidence{}
	}
}

func (t BranchRelationTransaction) HasStateSteps() bool {
	for _, step := range t.steps {
		if step.kind != BranchRelationStepSufficientLiteralCase {
			return true
		}
	}
	return false
}

func (t BranchRelationTransaction) HasSufficientLiteralCases() bool {
	for _, step := range t.steps {
		if step.kind == BranchRelationStepSufficientLiteralCase {
			return true
		}
	}
	return false
}

// HasRefinements reports whether this selected edge owns guard refinements.
// They are replayed first by ApplyBranchRelations, and remain proof inputs for
// resolved call-outcome correlations in the same transaction.
func (t BranchRelationTransaction) HasRefinements() bool { return len(t.refinements) != 0 }

// RefinementTargetPaths returns the exact finite path inventory the selected
// guard-refinement phase may update before structural relation steps run.
// The detached paths let access planners seal Values ownership without
// exposing or replaying the refinement payload through a second interpreter.
func (t BranchRelationTransaction) RefinementTargetPaths() []pathdom.Path {
	out := make([]pathdom.Path, len(t.refinements))
	for index, refinement := range t.refinements {
		out[index] = refinement.TargetPath()
	}
	return out
}

// Valid reports whether every member obeys the frozen edge-occurrence and
// tagged-payload invariants. It does not inspect any execution authority.
func (t BranchRelationTransaction) Valid() bool {
	for _, refinement := range t.refinements {
		if refinement.TargetPathRef().IsEmpty() {
			return false
		}
	}
	for _, step := range t.steps {
		switch step.kind {
		case BranchRelationStepPresence:
			if step.presence.TriggerPathRef().IsEmpty() || step.presence.TargetPathRef().IsEmpty() {
				return false
			}
		case BranchRelationStepPath:
			if !step.path.ActiveOnEdge(t.cond) || step.path.LeftPath().IsEmpty() || step.path.RightPath().IsEmpty() {
				return false
			}
		case BranchRelationStepLengthFloor:
			if step.length.Cond() != t.cond || step.length.ArrayPathRef().IsEmpty() {
				return false
			}
		case BranchRelationStepNumericFloor:
			if step.numFloor.Cond() != t.cond || step.numFloor.TargetPathRef().IsEmpty() {
				return false
			}
		case BranchRelationStepNumericCeiling:
			if step.numCeiling.Cond() != t.cond || step.numCeiling.TargetPathRef().IsEmpty() {
				return false
			}
		case BranchRelationStepDifference:
			if step.difference.Cond() != t.cond || step.difference.HiPath().IsEmpty() || step.difference.LoPath().IsEmpty() {
				return false
			}
		case BranchRelationStepEvidence:
			active := step.evidence.ActiveOnEdge(t.cond)
			oppositeFalsy := step.evidence.Kind() == factflow.BranchPathEvidenceTruthy &&
				step.evidence.OppositeEdgeImpliesFalsy() && step.evidence.ActiveOnEdge(!t.cond)
			if (!active && !oppositeFalsy) || step.evidence.PathRef().IsEmpty() {
				return false
			}
		case BranchRelationStepDynamicPresence:
			if step.dynamic.table.IsEmpty() || step.dynamic.table.Symbol == 0 || step.dynamic.table.Version != 0 {
				return false
			}
		case BranchRelationStepSufficientLiteralCase:
			if step.sufficient.TargetPathRef().IsEmpty() {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ValidForRegistry additionally proves ownership of every product embedded in
// branch refinements, dynamic bindings, and sufficient literal metadata.
func (t BranchRelationTransaction) ValidForRegistry(reg *axis.Registry) bool {
	if reg == nil || !t.Valid() {
		return false
	}
	for _, refinement := range t.refinements {
		for _, cond := range []bool{false, true} {
			value, ok := refinement.ValueForEdge(cond)
			if !ok {
				continue
			}
			if constraint, ok := value.Constraint(); ok && !product.BelongsToRegistry(reg, constraint) {
				return false
			}
		}
	}
	for _, step := range t.steps {
		switch step.kind {
		case BranchRelationStepDynamicPresence:
			if step.dynamic.keyBound && !product.BelongsToRegistry(reg, step.dynamic.key) {
				return false
			}
		case BranchRelationStepSufficientLiteralCase:
			if !product.BelongsToRegistry(reg, step.sufficient.LiteralValue()) {
				return false
			}
		}
	}
	return true
}

// Step returns one immutable transaction member without exposing the backing
// slice. Payload path accessors return detached paths at their own boundary.
func (t BranchRelationTransaction) Step(index int) (BranchRelationStep, bool) {
	if index < 0 || index >= len(t.steps) {
		return BranchRelationStep{}, false
	}
	return cloneBranchRelationStep(t.steps[index]), true
}

type ConcreteBranchRelationResult struct {
	Output   state.State
	Canceled bool
	Err      error
}

// PathSemanticAuthority is the prepared body's frozen path-resolution
// authority shared by typed semantic transactions. It contains no State and no
// callback into the legacy body solver; tuple-coordinate execution supplies
// the State explicitly.
type PathSemanticAuthority struct {
	resolver    *visibility.Resolver
	projectPath PathTypeProjector
	widen       CovariantWiden
	typeValues  *typevalue.Cache
}

func NewPathSemanticAuthority(resolver *visibility.Resolver, projectPath PathTypeProjector, typeValues *typevalue.Cache) *PathSemanticAuthority {
	return NewPathSemanticAuthorityWithWiden(resolver, projectPath, nil, typeValues)
}

// NewPathSemanticAuthorityWithWiden freezes the complete path-side semantic
// authority required by call transactions. The widening callback belongs to
// the prepared language body, not to an individual solve, so carrying it here
// keeps parameter-exposure effects on the same canonical transaction path as
// every other call effect.
func NewPathSemanticAuthorityWithWiden(
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	widen CovariantWiden,
	typeValues *typevalue.Cache,
) *PathSemanticAuthority {
	if resolver == nil || resolver.KeySpace() == nil {
		return nil
	}
	return &PathSemanticAuthority{resolver: resolver, projectPath: projectPath, widen: widen, typeValues: typeValues}
}

func (a *PathSemanticAuthority) Valid() bool {
	return a != nil && a.resolver != nil && a.resolver.KeySpace() != nil
}

// VisibleLocalPathKey returns the exact point-visible State address for a
// symbol-rooted lexical path. Relation frames use it to bind their structural
// root circuit to the same SSA namespace as path-evidence transactions; the
// authority remains the sole owner of visibility resolution.
func (a *PathSemanticAuthority) VisibleLocalPathKey(point cfg.Point, path pathdom.Path) (keyspace.Key, bool) {
	if !a.Valid() || path.IsEmpty() || path.Symbol == 0 {
		return keyspace.Key{}, false
	}
	return a.resolver.VisibleLocalKeyspaceKeyAt(point, path)
}

// VisibleInputLocalPathKey returns the exact SSA address reaching point before
// point-local definitions. Boundary frames use this for callee input roots:
// a capture need not have a version at the synthetic CFG entry, while its
// first real use still has one immutable incoming version.
func (a *PathSemanticAuthority) VisibleInputLocalPathKey(point cfg.Point, path pathdom.Path) (keyspace.Key, bool) {
	if !a.Valid() || path.IsEmpty() || path.Symbol == 0 {
		return keyspace.Key{}, false
	}
	return a.resolver.Before().VisibleLocalKeyspaceKeyAt(point, path)
}

// KeySpace returns the immutable coordinate vocabulary owned by this path
// authority. Carrier-neutral factor programs use it to bind registered path
// queries without reaching through a guarded executor.
func (a *PathSemanticAuthority) KeySpace() *keyspace.KeySpace {
	if a == nil || a.resolver == nil {
		return nil
	}
	return a.resolver.KeySpace()
}

// FreezePathAddress seals the structural/visible dual address chosen by this
// body's resolver. Linked relation frames call it once while the forest is
// frozen; tuple execution never resolves visibility dynamically.
func (a *PathSemanticAuthority) FreezePathAddress(point cfg.Point, path pathdom.Path) (ResolvedPathAddress, error) {
	if !a.Valid() {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: path address requires valid semantic authority")
	}
	return FreezeResolvedPathAddress(a.resolver, point, path)
}

// FreezeInputPathAddress is the input-snapshot counterpart of
// FreezePathAddress. It seals authority already computed by SSA visibility;
// it never synthesizes a version for a boundary root.
func (a *PathSemanticAuthority) FreezeInputPathAddress(point cfg.Point, path pathdom.Path) (ResolvedPathAddress, error) {
	if !a.Valid() {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: input path address requires valid semantic authority")
	}
	return FreezeResolvedPathAddress(a.resolver.Before(), point, path)
}
