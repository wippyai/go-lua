package factapply

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

type branchAtomFactorKernel func(branchAtomFactorRuntime, BranchRelationFactorFrame, BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error)

type branchAtomFactorRuntime struct {
	context transfer.EdgeContext
	domain  state.ProductDomain
}

// BranchRelationCoordinateLayout is one registered family component in a
// dense factor role. Slots are in sealed family order and are the only scalar
// coordinates visible to the semantic kernel.
type BranchRelationCoordinateLayout struct {
	family state.CoordinateFamily
	slots  []state.CoordinateSlot
}

func (l BranchRelationCoordinateLayout) Family() state.CoordinateFamily { return l.family }
func (l BranchRelationCoordinateLayout) Slots() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), l.slots...)
}

// BranchRelationFactorLayout is the closed dense operand shape of one atom.
// Concrete Values identities are private adapter bindings; kernels observe
// only ordinal vectors.
type BranchRelationFactorLayout struct {
	currentValues, originalValues           []BranchRelationValueRole
	currentValueWriteOrdinals               []int
	writeValuesTop                          bool
	currentLanes, originalLanes             []state.ProductLane
	currentLaneWriteOrdinals                []int
	currentCoordinates, originalCoordinates []BranchRelationCoordinateLayout
	writeCoordinateOrdinals                 [][]int
	writeCoordinateSkeleton                 []bool
	seal                                    *branchProgramSeal
}

func (l BranchRelationFactorLayout) CurrentValueCount() int  { return len(l.currentValues) }
func (l BranchRelationFactorLayout) OriginalValueCount() int { return len(l.originalValues) }
func (l BranchRelationFactorLayout) CurrentValueRoles() []BranchRelationValueRole {
	return cloneBranchValueRoles(l.currentValues)
}
func (l BranchRelationFactorLayout) OriginalValueRoles() []BranchRelationValueRole {
	return cloneBranchValueRoles(l.originalValues)
}
func (l BranchRelationFactorLayout) CurrentValueWriteOrdinals() []int {
	return append([]int(nil), l.currentValueWriteOrdinals...)
}
func (l BranchRelationFactorLayout) WritesValuesTop() bool { return l.writeValuesTop }
func (l BranchRelationFactorLayout) CurrentLanes() []state.ProductLane {
	return append([]state.ProductLane(nil), l.currentLanes...)
}
func (l BranchRelationFactorLayout) OriginalLanes() []state.ProductLane {
	return append([]state.ProductLane(nil), l.originalLanes...)
}
func (l BranchRelationFactorLayout) CurrentLaneWriteOrdinals() []int {
	return append([]int(nil), l.currentLaneWriteOrdinals...)
}
func (l BranchRelationFactorLayout) CurrentCoordinates() []BranchRelationCoordinateLayout {
	return cloneBranchCoordinateLayouts(l.currentCoordinates)
}
func (l BranchRelationFactorLayout) OriginalCoordinates() []BranchRelationCoordinateLayout {
	return cloneBranchCoordinateLayouts(l.originalCoordinates)
}
func (l BranchRelationFactorLayout) CurrentCoordinateWriteOrdinals() [][]int {
	out := make([][]int, len(l.writeCoordinateOrdinals))
	for index := range l.writeCoordinateOrdinals {
		out[index] = append([]int(nil), l.writeCoordinateOrdinals[index]...)
	}
	return out
}
func (l BranchRelationFactorLayout) CurrentCoordinateSkeletonWrites() []bool {
	return append([]bool(nil), l.writeCoordinateSkeleton...)
}

type BranchRelationCoordinateOperands struct {
	Skeleton state.CoordinateFamilySkeleton
	Scalars  []state.CoordinateScalarFactor
}

type BranchRelationFactorOperands struct {
	Values      []product.Value
	ValuesTop   bool
	Lanes       []state.LaneFactor
	Coordinates []BranchRelationCoordinateOperands
	Reachable   bool
}

type BranchRelationFactorRole uint8

const (
	BranchRelationFactorCurrent BranchRelationFactorRole = iota + 1
	BranchRelationFactorOriginal
)

// BranchRelationFactorFrame is an unforgeable dense tuple leaf. It contains
// no State and no concrete Values key; its semantic indices are ordinals in
// the sealed layout.
type BranchRelationFactorFrame struct {
	plan        *branchAtomFactorPlan
	role        BranchRelationFactorRole
	values      []product.Value
	valuesTop   bool
	lanes       []state.LaneFactor
	coordinates []BranchRelationCoordinateOperands
	reachable   bool
}

type BranchRelationFactorPatch struct {
	plan        *branchAtomFactorPlan
	values      []product.Value
	valuesTop   bool
	lanes       []state.LaneFactor
	coordinates []BranchRelationCoordinateOperands
	reachable   bool
}

// BranchRelationCoordinatePublicationLaw is the sealed family publication
// law owned by one factor coordinate.  Formal executors consume this exact
// law; they must not infer family carry from physical layout width.
type BranchRelationCoordinatePublicationLaw uint8

const (
	BranchRelationCoordinatePublicationPatch BranchRelationCoordinatePublicationLaw = iota + 1
	BranchRelationCoordinatePublicationReconcile
)

func (p BranchRelationFactorPatch) Values() []product.Value {
	return append([]product.Value(nil), p.values...)
}
func (p BranchRelationFactorPatch) ValuesTop() bool { return p.plan != nil && p.valuesTop }
func (p BranchRelationFactorPatch) Lanes() []state.LaneFactor {
	return append([]state.LaneFactor(nil), p.lanes...)
}
func (p BranchRelationFactorPatch) Coordinates() []BranchRelationCoordinateOperands {
	return cloneBranchCoordinateOperands(p.coordinates)
}
func (p BranchRelationFactorPatch) Reachable() bool { return p.plan != nil && p.reachable }

type branchAtomFactorPlan struct {
	layout                BranchRelationFactorLayout
	kernel                branchAtomFactorKernel
	mutation              state.CoordinateBranchMutation
	pathReadAuthority     state.CoordinatePathEvidenceAuthority[BranchRelationValueRole]
	pathWriteAuthority    state.CoordinatePathEvidenceAuthority[BranchRelationValueRole]
	coordinatePublication []BranchRelationCoordinatePublicationLaw
}

func (f BranchRelationFactors) FactorLayout(index int) (BranchRelationFactorLayout, bool) {
	if !f.validFactorPlan(index) {
		return BranchRelationFactorLayout{}, false
	}
	return cloneBranchFactorLayout(f.prepared.factorPlans[index].layout), true
}

// FactorCoordinatePublicationLaw reports the canonical law already sealed by
// factor preparation for one current coordinate operand.
func (f BranchRelationFactors) FactorCoordinatePublicationLaw(index, coordinateIndex int) (BranchRelationCoordinatePublicationLaw, bool) {
	if !f.validFactorPlan(index) {
		return 0, false
	}
	plan := f.prepared.factorPlans[index]
	if coordinateIndex < 0 || coordinateIndex >= len(plan.coordinatePublication) {
		return 0, false
	}
	return plan.coordinatePublication[coordinateIndex], true
}

// FactorSource reports immutable preparation provenance for diagnostics only;
// execution never dispatches on this syntax tag.
func (f BranchRelationFactors) FactorSource(index int) BranchRelationStepKind {
	if f.seal == nil || f.prepared.seal != f.seal || index < 0 || index >= len(f.prepared.atoms) {
		return 0
	}
	return f.prepared.atoms[index].source
}

func (f BranchRelationFactors) validFactorPlan(index int) bool {
	return f.seal != nil && f.prepared.seal == f.seal && index >= 0 &&
		index < len(f.prepared.factorPlans) && f.prepared.factorPlans[index] != nil
}

func (f BranchRelationFactors) BindFactorFrame(index int, role BranchRelationFactorRole, operands BranchRelationFactorOperands) (BranchRelationFactorFrame, error) {
	if !f.validFactorPlan(index) {
		return BranchRelationFactorFrame{}, fmt.Errorf("factapply: factor-native branch atom is absent")
	}
	plan := f.prepared.factorPlans[index]
	values, lanes, coordinates := plan.layout.currentValues, plan.layout.currentLanes, plan.layout.currentCoordinates
	if role == BranchRelationFactorOriginal {
		values, lanes, coordinates = plan.layout.originalValues, plan.layout.originalLanes, plan.layout.originalCoordinates
	} else if role != BranchRelationFactorCurrent {
		return BranchRelationFactorFrame{}, fmt.Errorf("factapply: invalid branch factor role")
	}
	if len(operands.Values) != len(values) || len(operands.Lanes) != len(lanes) || len(operands.Coordinates) != len(coordinates) {
		return BranchRelationFactorFrame{}, fmt.Errorf("factapply: branch factor operand width mismatch")
	}
	for index, value := range operands.Values {
		if !product.BelongsToRegistry(f.prepared.domain.Registry(), value) {
			return BranchRelationFactorFrame{}, fmt.Errorf("factapply: foreign branch Values operand %d", index)
		}
	}
	for index, lane := range operands.Lanes {
		if lane.Lane() != lanes[index] {
			return BranchRelationFactorFrame{}, fmt.Errorf("factapply: reordered branch lane operand %d", index)
		}
	}
	if err := validateBranchCoordinateOperands(f.prepared.domain, coordinates, operands.Coordinates); err != nil {
		return BranchRelationFactorFrame{}, err
	}
	return BranchRelationFactorFrame{
		plan: plan, role: role, values: append([]product.Value(nil), operands.Values...), valuesTop: operands.ValuesTop,
		lanes: append([]state.LaneFactor(nil), operands.Lanes...), coordinates: cloneBranchCoordinateOperands(operands.Coordinates), reachable: operands.Reachable,
	}, nil
}

func validateBranchCoordinateOperands(domain state.ProductDomain, layouts []BranchRelationCoordinateLayout, operands []BranchRelationCoordinateOperands) error {
	if len(layouts) != len(operands) {
		return fmt.Errorf("factapply: branch coordinate operand width mismatch")
	}
	for index, group := range operands {
		layout := layouts[index]
		if group.Skeleton.Family() != layout.family || len(group.Scalars) != len(layout.slots) {
			return fmt.Errorf("factapply: branch coordinate group %d differs from layout", index)
		}
		for scalarIndex, scalar := range group.Scalars {
			equal, err := domain.CoordinateSlotEqual(scalar.Slot(), layout.slots[scalarIndex])
			if err != nil || !equal {
				return fmt.Errorf("factapply: branch coordinate scalar %d/%d is foreign", index, scalarIndex)
			}
		}
	}
	return nil
}

// bindBranchCoordinateLayout projects one family's sparse canonical spelling
// onto the dense, sealed operand shape consumed by a branch factor. Optional
// coordinates omitted by the family representation remain real factor
// operands at their registered skeleton-relative defaults.
func bindBranchCoordinateLayout(
	domain state.ProductDomain,
	layout BranchRelationCoordinateLayout,
	skeleton state.CoordinateFamilySkeleton,
	explicit []state.CoordinateScalarFactor,
) (BranchRelationCoordinateOperands, error) {
	if skeleton.Family() != layout.family {
		return BranchRelationCoordinateOperands{}, fmt.Errorf("factapply: branch coordinate family differs from layout")
	}
	scalars := make([]state.CoordinateScalarFactor, len(layout.slots))
	for index, slot := range layout.slots {
		scalar, found, err := findBranchCoordinateScalar(domain, explicit, slot)
		if err != nil {
			return BranchRelationCoordinateOperands{}, err
		}
		if !found {
			scalar, err = domain.CoordinateDefault(skeleton, slot)
			if err != nil {
				return BranchRelationCoordinateOperands{}, err
			}
		}
		scalars[index] = scalar
	}
	return BranchRelationCoordinateOperands{Skeleton: skeleton, Scalars: scalars}, nil
}

func bindBranchPathDescendantMutationFactors(
	domain state.ProductDomain,
	layout BranchRelationFactorLayout,
	frame BranchRelationFactorFrame,
) (state.PathDescendantMutationFactors, error) {
	topology, err := domain.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return state.PathDescendantMutationFactors{}, err
	}
	lanes := make([]state.LaneFactor, 0, len(topology.Lanes()))
	for _, wanted := range topology.Lanes() {
		found := false
		for index, lane := range layout.currentLanes {
			if lane == wanted {
				lanes = append(lanes, frame.lanes[index])
				found = true
				break
			}
		}
		if !found {
			return state.PathDescendantMutationFactors{}, fmt.Errorf("factapply: descendant mutation lane is absent")
		}
	}
	families := topology.Families()
	var keys *keyspace.KeySpace
	for index, group := range layout.currentCoordinates {
		if len(families) != 0 && group.family == families[0] {
			keys = frame.coordinates[index].Skeleton.KeySpace()
			break
		}
	}
	if keys == nil || !keys.Valid() {
		return state.PathDescendantMutationFactors{}, fmt.Errorf("factapply: descendant mutation keyspace is absent")
	}
	coordinates := make([]state.CoordinateFamilyFactor, 0, len(families)-1)
	for _, family := range families[1:] {
		found := false
		for index, group := range layout.currentCoordinates {
			if group.family != family {
				continue
			}
			factor, sealErr := domain.SealCoordinateFamilyFactor(frame.coordinates[index].Skeleton, frame.coordinates[index].Scalars)
			if sealErr != nil {
				return state.PathDescendantMutationFactors{}, sealErr
			}
			coordinates = append(coordinates, factor)
			found = true
			break
		}
		if !found {
			bottom, bottomErr := domain.LaneBottom(family.Lane())
			if bottomErr != nil {
				return state.PathDescendantMutationFactors{}, bottomErr
			}
			skeleton, scalars, decomposeErr := domain.DecomposeCoordinateFamily(bottom, family, keys)
			if decomposeErr != nil {
				return state.PathDescendantMutationFactors{}, decomposeErr
			}
			factor, sealErr := domain.SealCoordinateFamilyFactor(skeleton, scalars)
			if sealErr != nil {
				return state.PathDescendantMutationFactors{}, sealErr
			}
			coordinates = append(coordinates, factor)
		}
	}
	return domain.SealPathDescendantMutationFactors(lanes, coordinates)
}

func applyBranchPathDescendantCoordinateFactors(
	domain state.ProductDomain,
	layout BranchRelationFactorLayout,
	coordinates []BranchRelationCoordinateOperands,
	factors []state.CoordinateFamilyFactor,
) ([]BranchRelationCoordinateOperands, error) {
	out := cloneBranchCoordinateOperands(coordinates)
	for _, factor := range factors {
		found := false
		for index, group := range layout.currentCoordinates {
			if group.family != factor.Family() {
				continue
			}
			var err error
			out[index], err = bindBranchCoordinateLayout(domain, group, factor.Skeleton(), factor.Scalars())
			if err != nil {
				return nil, err
			}
			found = true
			break
		}
		if !found {
			bottom, err := domain.CoordinateFamilyFactorIsBottom(factor)
			if err != nil {
				return nil, err
			}
			if !bottom {
				return nil, fmt.Errorf("factapply: descendant mutation coordinate output is absent")
			}
		}
	}
	return out, nil
}

func (f BranchRelationFactors) ApplyFactorFrames(index int, edge transfer.EdgeContext, original, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
	if !f.validFactorPlan(index) {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: factor-native branch atom is absent")
	}
	plan := f.prepared.factorPlans[index]
	if original.plan != plan || original.role != BranchRelationFactorOriginal || current.plan != plan || current.role != BranchRelationFactorCurrent {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: foreign branch factor frame")
	}
	if edge.Registry != f.prepared.domain.Registry() || !edge.HasCond || edge.Edge.From != f.prepared.transaction.point || edge.Edge.Cond != f.prepared.transaction.cond {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch factor edge mismatch")
	}
	if edge.Context == nil {
		edge.Context = context.Background()
	}
	if token := tokenOf(edge.Session); token != nil && token.Canceled() {
		return BranchRelationFactorPatch{}, true, nil
	}
	patch, feasible, err := plan.kernel(branchAtomFactorRuntime{context: edge, domain: f.prepared.domain}, original, current)
	if err != nil {
		return BranchRelationFactorPatch{}, err == context.Canceled, err
	}
	if patch.plan != plan {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch factor kernel returned an undeclared patch")
	}
	// Product bottom has one canonical sparse spelling: reachability false and
	// no component payload. A rejecting factor may have computed bottom lane or
	// coordinate images internally, but those values are unobservable once the
	// branch is infeasible and must not manufacture write requirements.
	if !feasible || !current.reachable {
		return BranchRelationFactorPatch{plan: plan, reachable: false}, false, nil
	}
	if err := validateBranchValuePatch(f.prepared.domain, plan, current, patch); err != nil {
		return BranchRelationFactorPatch{}, false, err
	}
	if err := validateBranchLanePatch(f.prepared.domain, plan, current.lanes, patch.lanes); err != nil {
		return BranchRelationFactorPatch{}, false, err
	}
	if err := validateBranchCoordinatePatch(f.prepared.domain, plan, current.coordinates, patch.coordinates); err != nil {
		return BranchRelationFactorPatch{}, false, err
	}
	patch.reachable = feasible && current.reachable
	return patch, false, nil
}

func validateBranchValuePatch(domain state.ProductDomain, plan *branchAtomFactorPlan, current BranchRelationFactorFrame, patch BranchRelationFactorPatch) error {
	writes := plan.layout.currentValueWriteOrdinals
	if len(writes) == 0 && !plan.layout.writeValuesTop {
		if len(patch.values) != 0 || patch.valuesTop {
			return fmt.Errorf("factapply: branch factor returned an undeclared Values patch")
		}
		return nil
	}
	if len(patch.values) != len(plan.layout.currentValues) || len(current.values) != len(patch.values) {
		return fmt.Errorf("factapply: branch factor Values patch has wrong width")
	}
	if !plan.layout.writeValuesTop && patch.valuesTop != current.valuesTop {
		return fmt.Errorf("factapply: branch factor writes undeclared Values Top")
	}
	for index, value := range patch.values {
		writable := false
		for _, ordinal := range writes {
			writable = writable || ordinal == index
		}
		if !writable && !product.Equal(domain.Registry(), current.values[index], value) {
			return fmt.Errorf("factapply: branch factor writes undeclared Values coordinate %d", index)
		}
	}
	return nil
}

func validateBranchLanePatch(domain state.ProductDomain, plan *branchAtomFactorPlan, current, patch []state.LaneFactor) error {
	if len(patch) == 0 {
		return nil
	}
	if len(patch) != len(plan.layout.currentLanes) || len(current) != len(patch) {
		return fmt.Errorf("factapply: branch factor lane patch has wrong width")
	}
	for index, factor := range patch {
		if factor.Lane() != plan.layout.currentLanes[index] {
			return fmt.Errorf("factapply: branch factor reordered lane %d", index)
		}
		writable := false
		for _, ordinal := range plan.layout.currentLaneWriteOrdinals {
			writable = writable || ordinal == index
		}
		if !writable {
			equal, err := domain.LaneCanonicalRepresentationEqual(current[index], factor)
			if err != nil || !equal {
				return fmt.Errorf("factapply: branch factor writes undeclared lane %d", index)
			}
		}
	}
	return nil
}

func validateBranchCoordinatePatch(domain state.ProductDomain, plan *branchAtomFactorPlan, current, patch []BranchRelationCoordinateOperands) error {
	if err := validateBranchCoordinateOperands(domain, plan.layout.currentCoordinates, patch); err != nil {
		return err
	}
	if len(current) != len(patch) || len(plan.layout.writeCoordinateSkeleton) != len(patch) {
		return fmt.Errorf("factapply: branch factor patch coordinate authority mismatch")
	}
	for groupIndex, group := range patch {
		writes := plan.layout.writeCoordinateOrdinals[groupIndex]
		skeletonWritable := plan.layout.writeCoordinateSkeleton[groupIndex]
		if !skeletonWritable {
			equal, err := domain.CoordinateSkeletonRepresentationEqual(current[groupIndex].Skeleton, group.Skeleton)
			if err != nil || !equal {
				return fmt.Errorf("factapply: branch factor patch writes undeclared coordinate skeleton %q/%d", group.Skeleton.Family().ID(), groupIndex)
			}
		}
		// A writable skeleton owns the complete selected family image.  Family
		// reconciliation may recanonicalize sibling scalar spellings when its
		// topology changes, so treating the scalar write list as an independent
		// authority would reject the canonical image produced by the registered
		// family law.  The dense layout still confines that authority to this
		// selected family and its selected slots.
		if skeletonWritable {
			continue
		}
		for scalarIndex, scalar := range group.Scalars {
			owned := false
			for _, ordinal := range writes {
				owned = owned || ordinal == scalarIndex
			}
			if !owned {
				equal, err := domain.CoordinateScalarRepresentationEqual(current[groupIndex].Scalars[scalarIndex], scalar)
				if err != nil || !equal {
					return fmt.Errorf("factapply: branch factor patch writes undeclared coordinate %d/%d", groupIndex, scalarIndex)
				}
			}
		}
	}
	return nil
}

func branchPathProofKernel(proof pathevidence.BranchProof) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if current.plan == nil || !current.reachable {
			return BranchRelationFactorPatch{plan: current.plan}, false, nil
		}
		family, ok := runtime.domain.PathEvidenceCoordinateFamily()
		if !ok {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch proof has no path-evidence family")
		}
		groupIndex := -1
		for index, layout := range current.plan.layout.currentCoordinates {
			if layout.family == family {
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch proof coordinate carrier is absent")
		}
		group := current.coordinates[groupIndex]
		carrier, err := state.OpenCoordinatePathEvidenceCarrier(
			runtime.domain, group.Skeleton, group.Scalars,
			state.ValueFactor[BranchRelationValueRole]{Values: map[BranchRelationValueRole]product.Value{}}, true,
			current.plan.pathWriteAuthority, state.PathDescendantMutationFactors{},
		)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		if _, accepted := carrier.AddProof(proof); !accepted {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch proof publication rejected")
		}
		if _, accepted := carrier.CloseProofsAcrossKnownEqualities(); !accepted {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch proof closure rejected")
		}
		skeleton, explicit, _, _, _, reachable, err := carrier.Freeze()
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		coordinates := cloneBranchCoordinateOperands(current.coordinates)
		coordinates[groupIndex], err = bindBranchCoordinateLayout(
			runtime.domain, current.plan.layout.currentCoordinates[groupIndex], skeleton, explicit,
		)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		return BranchRelationFactorPatch{plan: current.plan, coordinates: coordinates, reachable: reachable}, reachable, nil
	}
}

func branchCoordinateMutationKernel(runtime branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
	if current.plan == nil || len(current.coordinates) != 1 || len(current.coordinates[0].Scalars) != 1 {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: coordinate mutation operands are incomplete")
	}
	group := current.coordinates[0]
	nextSkeleton, nextScalar, err := runtime.domain.ApplyCoordinateBranchMutation(current.plan.mutation, group.Skeleton, group.Scalars[0])
	if err != nil {
		return BranchRelationFactorPatch{}, false, err
	}
	return BranchRelationFactorPatch{
		plan: current.plan, coordinates: []BranchRelationCoordinateOperands{{Skeleton: nextSkeleton, Scalars: []state.CoordinateScalarFactor{nextScalar}}},
		reachable: current.reachable,
	}, current.reachable, nil
}

// branchLexicalTruthinessKernel is the canonical root-value feasibility
// kernel. Concrete and formal callers bind the same lexical role; neither
// resolves a State path inside the semantic operation.
func branchLexicalTruthinessKernel(wantTruthy bool) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, original, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if original.plan == nil || current.plan != original.plan || len(original.values) != 1 || len(current.values) != 0 {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: lexical truthiness operands are incomplete")
		}
		feasible := current.reachable
		value := original.values[0]
		if original.reachable && !product.Equal(runtime.domain.Registry(), value, product.Bottom(runtime.domain.Registry())) {
			if wantTruthy {
				feasible = feasible && valuerefine.CanBeTruthy(runtime.domain.Registry(), value)
			} else {
				feasible = feasible && valuerefine.CanBeFalsy(runtime.domain.Registry(), value)
			}
		}
		return BranchRelationFactorPatch{plan: current.plan, reachable: feasible}, feasible, nil
	}
}

func branchIdentityFactorKernel(_ branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
	return BranchRelationFactorPatch{plan: current.plan, reachable: current.reachable}, current.reachable, nil
}

func sealBranchAtomFactorPlan(domain state.ProductDomain, keys *keyspace.KeySpace, atom branchAtom, seal *branchProgramSeal) (*branchAtomFactorPlan, error) {
	if atom.factor == nil {
		return nil, nil
	}
	if atom.seal != seal || atom.apply != nil {
		return nil, fmt.Errorf("factapply: branch atom has parallel State and factor kernels")
	}
	layout, err := freezeBranchFactorLayout(domain, atom, seal)
	if err != nil {
		return nil, err
	}
	plan := &branchAtomFactorPlan{
		layout: layout, kernel: atom.factor, mutation: atom.mutation,
		coordinatePublication: make([]BranchRelationCoordinatePublicationLaw, len(layout.currentCoordinates)),
	}
	for index := range layout.currentCoordinates {
		if index >= len(layout.writeCoordinateSkeleton) || !layout.writeCoordinateSkeleton[index] {
			plan.coordinatePublication[index] = BranchRelationCoordinatePublicationPatch
			continue
		}
		plan.coordinatePublication[index] = BranchRelationCoordinatePublicationReconcile
	}
	if keys == nil {
		return plan, nil
	}
	pathFamily, hasPathFamily := domain.PathEvidenceCoordinateFamily()
	for _, group := range layout.currentCoordinates {
		if !hasPathFamily || group.family != pathFamily {
			continue
		}
		inventory, inventoryErr := domain.SealCoordinateFactorInventory(keys, group.slots)
		if inventoryErr != nil {
			return nil, inventoryErr
		}
		valid := func(role BranchRelationValueRole) bool { return role.validFor(seal) }
		plan.pathReadAuthority, inventoryErr = state.SealCoordinatePathEvidenceAuthority(
			domain, keys, nil, nil, inventory, inventory, false, false, valid,
		)
		if inventoryErr != nil {
			return nil, inventoryErr
		}
		plan.pathWriteAuthority, inventoryErr = state.SealCoordinatePathEvidenceAuthority(
			domain, keys, nil, nil, inventory, inventory, false, true, valid,
		)
		if inventoryErr != nil {
			return nil, inventoryErr
		}
		break
	}
	return plan, nil
}

func freezeBranchFactorLayout(domain state.ProductDomain, atom branchAtom, seal *branchProgramSeal) (BranchRelationFactorLayout, error) {
	access := atom.access
	currentValues, ok := canonicalBranchValueRoles(append(cloneBranchValueRoles(atom.valueRoles.currentReads), atom.valueRoles.currentWrites...), seal)
	if !ok {
		return BranchRelationFactorLayout{}, fmt.Errorf("factapply: branch factor has a foreign current Values role")
	}
	originalValues, ok := canonicalBranchValueRoles(atom.valueRoles.originalReads, seal)
	if !ok {
		return BranchRelationFactorLayout{}, fmt.Errorf("factapply: branch factor has a foreign original Values role")
	}
	currentCoordinates, err := freezeBranchCoordinateLayouts(
		domain,
		append(append([]state.CoordinateSlot(nil), access.coordinateReads...), access.coordinateWrites...),
		append(append([]state.CoordinateFamily(nil), access.coordinateFamilyReads...), access.coordinateFamilyWrites...),
	)
	if err != nil {
		return BranchRelationFactorLayout{}, err
	}
	originalCoordinates, err := freezeBranchCoordinateLayouts(domain, access.originalCoordinateReads, nil)
	if err != nil {
		return BranchRelationFactorLayout{}, err
	}
	writes := make([][]int, len(currentCoordinates))
	skeletonWrites := make([]bool, len(currentCoordinates))
	valueWrites := make([]int, 0, len(atom.valueRoles.currentWrites))
	for _, write := range atom.valueRoles.currentWrites {
		found := false
		for index, role := range currentValues {
			if role == write {
				valueWrites = append(valueWrites, index)
				found = true
				break
			}
		}
		if !found {
			return BranchRelationFactorLayout{}, fmt.Errorf("factapply: Values write is outside current layout")
		}
	}
	laneWrites := make([]int, 0, len(access.laneWrites))
	currentLanes := canonicalBranchFactorLanes(access.laneReads)
	for _, write := range access.laneWrites {
		for index, lane := range currentLanes {
			if lane == write {
				laneWrites = append(laneWrites, index)
				break
			}
		}
	}
	for writeIndex, write := range access.coordinateWrites {
		found := false
		for groupIndex, group := range currentCoordinates {
			for scalarIndex, slot := range group.slots {
				equal, equalErr := domain.CoordinateSlotEqual(write, slot)
				if equalErr != nil {
					return BranchRelationFactorLayout{}, equalErr
				}
				if equal {
					writes[groupIndex] = append(writes[groupIndex], scalarIndex)
					found = true
				}
			}
		}
		if !found {
			return BranchRelationFactorLayout{}, fmt.Errorf("factapply: coordinate write %d is outside current layout", writeIndex)
		}
	}
	for _, family := range access.coordinateFamilyWrites {
		found := false
		for groupIndex, group := range currentCoordinates {
			if group.family == family {
				skeletonWrites[groupIndex] = true
				found = true
				break
			}
		}
		if !found {
			return BranchRelationFactorLayout{}, fmt.Errorf("factapply: coordinate family write is outside current layout")
		}
	}
	return BranchRelationFactorLayout{
		currentValues: currentValues, originalValues: originalValues,
		currentValueWriteOrdinals: valueWrites, writeValuesTop: branchAccessHasLane(access.laneWrites, state.LaneValues),
		currentLanes: currentLanes, originalLanes: canonicalBranchFactorLanes(access.originalLaneReads), currentLaneWriteOrdinals: laneWrites,
		currentCoordinates: currentCoordinates, originalCoordinates: originalCoordinates, writeCoordinateOrdinals: writes,
		writeCoordinateSkeleton: skeletonWrites, seal: seal,
	}, nil
}

func freezeBranchCoordinateLayouts(domain state.ProductDomain, slots []state.CoordinateSlot, families []state.CoordinateFamily) ([]BranchRelationCoordinateLayout, error) {
	out := make([]BranchRelationCoordinateLayout, 0)
	for _, slot := range slots {
		group := -1
		for index := range out {
			if out[index].family == slot.Family() {
				group = index
				break
			}
		}
		if group < 0 {
			out = append(out, BranchRelationCoordinateLayout{family: slot.Family()})
			group = len(out) - 1
		}
		duplicate := false
		for _, prior := range out[group].slots {
			equal, err := domain.CoordinateSlotEqual(prior, slot)
			if err != nil {
				return nil, err
			}
			duplicate = duplicate || equal
		}
		if !duplicate {
			out[group].slots = append(out[group].slots, slot)
		}
	}
	for index := range out {
		sort.SliceStable(out[index].slots, func(left, right int) bool {
			less, err := domain.CoordinateSlotLess(out[index].slots[left], out[index].slots[right])
			return err == nil && less
		})
	}
	for _, family := range families {
		found := false
		for _, group := range out {
			found = found || group.family == family
		}
		if !found {
			out = append(out, BranchRelationCoordinateLayout{family: family})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i].family, out[j].family
		if left.Lane().Ordinal() != right.Lane().Ordinal() {
			return left.Lane().Ordinal() < right.Lane().Ordinal()
		}
		return left.Ordinal() < right.Ordinal()
	})
	return out, nil
}

func canonicalBranchFactorLanes(lanes []state.ProductLane) []state.ProductLane {
	out := make([]state.ProductLane, 0, len(lanes))
	for _, lane := range lanes {
		if lane.ID() == state.LaneValues {
			continue
		}
		found := false
		for _, prior := range out {
			found = found || prior == lane
		}
		if !found {
			out = append(out, lane)
		}
	}
	return out
}

func cloneBranchCoordinateLayouts(in []BranchRelationCoordinateLayout) []BranchRelationCoordinateLayout {
	out := make([]BranchRelationCoordinateLayout, len(in))
	for index, group := range in {
		out[index] = BranchRelationCoordinateLayout{family: group.family, slots: append([]state.CoordinateSlot(nil), group.slots...)}
	}
	return out
}

func cloneBranchCoordinateOperands(in []BranchRelationCoordinateOperands) []BranchRelationCoordinateOperands {
	out := make([]BranchRelationCoordinateOperands, len(in))
	for index, group := range in {
		out[index] = BranchRelationCoordinateOperands{Skeleton: group.Skeleton, Scalars: append([]state.CoordinateScalarFactor(nil), group.Scalars...)}
	}
	return out
}

func cloneBranchFactorLayout(in BranchRelationFactorLayout) BranchRelationFactorLayout {
	in.currentValues = cloneBranchValueRoles(in.currentValues)
	in.originalValues = cloneBranchValueRoles(in.originalValues)
	in.currentValueWriteOrdinals = append([]int(nil), in.currentValueWriteOrdinals...)
	in.currentLanes = append([]state.ProductLane(nil), in.currentLanes...)
	in.originalLanes = append([]state.ProductLane(nil), in.originalLanes...)
	in.currentLaneWriteOrdinals = append([]int(nil), in.currentLaneWriteOrdinals...)
	in.currentCoordinates = cloneBranchCoordinateLayouts(in.currentCoordinates)
	in.originalCoordinates = cloneBranchCoordinateLayouts(in.originalCoordinates)
	in.writeCoordinateOrdinals = make([][]int, len(in.writeCoordinateOrdinals))
	for index := range in.writeCoordinateOrdinals {
		in.writeCoordinateOrdinals[index] = append([]int(nil), in.writeCoordinateOrdinals[index]...)
	}
	in.writeCoordinateSkeleton = append([]bool(nil), in.writeCoordinateSkeleton...)
	return in
}

func (f BranchRelationFactors) applyConcreteFactor(index int, edge transfer.EdgeContext, original, input state.State) ConcreteBranchRelationResult {
	originalFrame, err := f.bindConcreteFactorFrame(index, BranchRelationFactorOriginal, original)
	if err != nil {
		return ConcreteBranchRelationResult{Output: input, Err: err}
	}
	currentFrame, err := f.bindConcreteFactorFrame(index, BranchRelationFactorCurrent, input)
	if err != nil {
		return ConcreteBranchRelationResult{Output: input, Err: err}
	}
	patch, canceled, err := f.ApplyFactorFrames(index, edge, originalFrame, currentFrame)
	if err != nil || canceled {
		return ConcreteBranchRelationResult{Output: input, Canceled: canceled, Err: err}
	}
	out, err := f.applyConcreteCoordinatePatch(input, patch)
	if err != nil {
		return ConcreteBranchRelationResult{Output: input, Err: err}
	}
	return ConcreteBranchRelationResult{Output: out}
}

func (f BranchRelationFactors) bindConcreteFactorFrame(index int, role BranchRelationFactorRole, source state.State) (BranchRelationFactorFrame, error) {
	plan := f.prepared.factorPlans[index]
	layout := plan.layout.currentCoordinates
	valueRoles := plan.layout.currentValues
	if role == BranchRelationFactorOriginal {
		layout = plan.layout.originalCoordinates
		valueRoles = plan.layout.originalValues
	}
	values := make([]product.Value, len(valueRoles))
	for index, valueRole := range valueRoles {
		slot, ok := valueRole.concreteSlot(f.seal)
		if !ok {
			return BranchRelationFactorFrame{}, fmt.Errorf("factapply: branch Values role has no concrete binding")
		}
		values[index] = source.ReadValue(f.prepared.domain.Registry(), slot)
	}
	_, valueFactor := state.DecomposeValueLane(f.prepared.domain.Lattice(), source)
	coordinates := make([]BranchRelationCoordinateOperands, len(layout))
	for index, group := range layout {
		factor, err := f.prepared.domain.DecomposeLanes(source, []state.ProductLane{group.family.Lane()})
		if err != nil {
			return BranchRelationFactorFrame{}, err
		}
		skeleton, explicit, err := f.prepared.domain.DecomposeCoordinateFamily(factor[0], group.family, f.prepared.authority.resolver.KeySpace())
		if err != nil {
			return BranchRelationFactorFrame{}, err
		}
		coordinates[index], err = bindBranchCoordinateLayout(f.prepared.domain, group, skeleton, explicit)
		if err != nil {
			return BranchRelationFactorFrame{}, err
		}
	}
	lanes, err := f.prepared.domain.DecomposeLanes(source, plan.layout.currentLanes)
	if role == BranchRelationFactorOriginal {
		lanes, err = f.prepared.domain.DecomposeLanes(source, plan.layout.originalLanes)
	}
	if err != nil {
		return BranchRelationFactorFrame{}, err
	}
	return f.BindFactorFrame(index, role, BranchRelationFactorOperands{
		Values: values, ValuesTop: valueFactor.Top, Lanes: lanes, Coordinates: coordinates,
		Reachable: !stateIsBottom(f.prepared.domain.Registry(), source),
	})
}

func (f BranchRelationFactors) applyConcreteCoordinatePatch(input state.State, patch BranchRelationFactorPatch) (state.State, error) {
	if !patch.reachable {
		return f.prepared.domain.Lattice().Bottom(), nil
	}
	out := input
	if len(patch.values) != 0 {
		residual, values := state.DecomposeValueLane(f.prepared.domain.Lattice(), out)
		values.Top = patch.valuesTop
		if values.Values == nil {
			values.Values = make(map[statekey.Value]product.Value)
		}
		for index, role := range patch.plan.layout.currentValues {
			slot, ok := role.concreteSlot(f.seal)
			if !ok {
				return state.State{}, fmt.Errorf("factapply: branch Values patch has no concrete binding")
			}
			value := patch.values[index]
			if values.Top || product.Equal(f.prepared.domain.Registry(), value, product.Bottom(f.prepared.domain.Registry())) {
				delete(values.Values, slot)
			} else {
				values.Values[slot] = value
			}
		}
		out = state.RecomposeValueLane(f.prepared.domain.Registry(), f.prepared.domain.Lattice(), residual, values)
	}
	if len(patch.lanes) != 0 {
		delta, err := f.prepared.domain.ComposeSparse(patch.lanes)
		if err != nil {
			return state.State{}, err
		}
		out, err = f.prepared.domain.PatchFactors(out, delta, state.NewLaneSet(branchLaneIDs(patch.plan.layout.currentLanes)...))
		if err != nil {
			return state.State{}, err
		}
	}
	if len(patch.coordinates) == 0 {
		return out, nil
	}
	written := make(map[state.LaneID]state.LaneFactor)
	for groupIndex, groupPatch := range patch.coordinates {
		family := patch.plan.layout.currentCoordinates[groupIndex].family
		lane := family.Lane()
		current, exists := written[lane.ID()]
		if !exists {
			laneFactor, err := f.prepared.domain.DecomposeLanes(out, []state.ProductLane{lane})
			if err != nil {
				return state.State{}, err
			}
			current = laneFactor[0]
		}
		var err error
		written[lane.ID()], err = f.applyFactorCoordinatePatch(patch.plan, groupIndex, current, groupPatch)
		if err != nil {
			return state.State{}, err
		}
	}
	lanes := make([]state.ProductLane, 0, len(written))
	factors := make([]state.LaneFactor, 0, len(written))
	for _, lane := range f.prepared.domain.LaneInventory() {
		if factor, ok := written[lane.ID()]; ok {
			lanes = append(lanes, lane)
			factors = append(factors, factor)
		}
	}
	delta, err := f.prepared.domain.ComposeSparse(factors)
	if err != nil {
		return state.State{}, err
	}
	return f.prepared.domain.PatchFactors(out, delta, state.NewLaneSet(branchLaneIDs(lanes)...))
}

// ApplyFactorCoordinatePatch is the one canonical publication law for a
// factor-native BranchRelations coordinate result. Concrete State and formal
// tuple executors both call this seam: skeleton-writing path-mutation
// families replace their complete image, while all other coordinate results
// are sparse patches over the current registered lane spelling.
func (f BranchRelationFactors) ApplyFactorCoordinatePatch(
	index int,
	coordinateIndex int,
	current state.LaneFactor,
	patch BranchRelationCoordinateOperands,
) (state.LaneFactor, error) {
	if !f.validFactorPlan(index) {
		return state.LaneFactor{}, fmt.Errorf("factapply: branch coordinate patch has no factor plan")
	}
	return f.applyFactorCoordinatePatch(f.prepared.factorPlans[index], coordinateIndex, current, patch)
}

// ApplyFactorCoordinateFamilyPatch is the family-native form of
// ApplyFactorCoordinatePatch. It executes the identical registered
// publication law without materializing or copying sibling families in the
// same physical product lane.
func (f BranchRelationFactors) ApplyFactorCoordinateFamilyPatch(
	index int,
	coordinateIndex int,
	current state.CoordinateFamilyFactor,
	patch BranchRelationCoordinateOperands,
) (state.CoordinateFamilyFactor, error) {
	if !f.validFactorPlan(index) {
		return state.CoordinateFamilyFactor{}, fmt.Errorf("factapply: branch coordinate patch has no factor plan")
	}
	plan := f.prepared.factorPlans[index]
	if coordinateIndex < 0 || coordinateIndex >= len(plan.layout.currentCoordinates) ||
		coordinateIndex >= len(plan.layout.writeCoordinateSkeleton) {
		return state.CoordinateFamilyFactor{}, fmt.Errorf("factapply: branch coordinate patch is outside its factor layout")
	}
	family := plan.layout.currentCoordinates[coordinateIndex].family
	if current.Family() != family || patch.Skeleton.Family() != family {
		return state.CoordinateFamilyFactor{}, fmt.Errorf("factapply: branch coordinate family patch ownership mismatch")
	}
	law := plan.coordinatePublication[coordinateIndex]
	if law == BranchRelationCoordinatePublicationPatch {
		return f.prepared.domain.PatchCoordinateFamilyFactor(current, patch.Skeleton, patch.Scalars)
	}
	if law == BranchRelationCoordinatePublicationReconcile {
		return f.prepared.domain.ReconcileCoordinateFamilyFactor(current, patch.Skeleton, patch.Scalars)
	}
	return state.CoordinateFamilyFactor{}, fmt.Errorf("factapply: branch coordinate publication law is invalid")
}

func (f BranchRelationFactors) applyFactorCoordinatePatch(
	plan *branchAtomFactorPlan,
	coordinateIndex int,
	current state.LaneFactor,
	patch BranchRelationCoordinateOperands,
) (state.LaneFactor, error) {
	if plan == nil {
		return state.LaneFactor{}, fmt.Errorf("factapply: branch coordinate patch has no factor plan")
	}
	if coordinateIndex < 0 || coordinateIndex >= len(plan.layout.currentCoordinates) ||
		coordinateIndex >= len(plan.layout.writeCoordinateSkeleton) {
		return state.LaneFactor{}, fmt.Errorf("factapply: branch coordinate patch is outside its factor layout")
	}
	family := plan.layout.currentCoordinates[coordinateIndex].family
	law := plan.coordinatePublication[coordinateIndex]
	if law == BranchRelationCoordinatePublicationPatch {
		out, err := f.prepared.domain.PatchCoordinateFamily(current, patch.Skeleton, patch.Scalars)
		if err != nil {
			return state.LaneFactor{}, fmt.Errorf("factapply: branch coordinate family %q patch: %w", family.ID(), err)
		}
		return out, nil
	}
	if law == BranchRelationCoordinatePublicationReconcile {
		out, err := f.prepared.domain.ReconcileCoordinateFamily(current, patch.Skeleton, patch.Scalars)
		if err != nil {
			return state.LaneFactor{}, fmt.Errorf("factapply: branch coordinate family %q reconciliation: %w", family.ID(), err)
		}
		return out, nil
	}
	return state.LaneFactor{}, fmt.Errorf("factapply: branch coordinate publication law is invalid")
}

func branchLaneIDs(lanes []state.ProductLane) []state.LaneID {
	out := make([]state.LaneID, len(lanes))
	for index, lane := range lanes {
		out[index] = lane.ID()
	}
	return out
}

func findBranchCoordinateScalar(domain state.ProductDomain, factors []state.CoordinateScalarFactor, slot state.CoordinateSlot) (state.CoordinateScalarFactor, bool, error) {
	for _, factor := range factors {
		equal, err := domain.CoordinateSlotEqual(factor.Slot(), slot)
		if err != nil {
			return state.CoordinateScalarFactor{}, false, err
		}
		if equal {
			return factor, true, nil
		}
	}
	return state.CoordinateScalarFactor{}, false, nil
}
