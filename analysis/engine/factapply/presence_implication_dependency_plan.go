package factapply

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// PresenceImplicationDependencyBlock is one exact may-dependency SCC frozen
// from the guarded union inventory. Read coordinates may be shared by multiple
// blocks; write coordinates have exclusive SCC ownership.
type PresenceImplicationDependencyBlock struct {
	coordinateReads      []state.CoordinateSlot
	coordinateWrites     []state.CoordinateSlot
	valueReads           []statekey.ValueDependency
	valueWrites          []statekey.ValueDependency
	publications         []pathevidence.PathPresenceImplication
	rows                 []pathevidence.PathPresenceImplication
	rowSlots             []state.CoordinateSlot
	predicateActivations []pathPredicateActivation
	predecessors         []int
	requiresFeedback     bool
	pathMutation         bool
	mayContradict        bool
	valid                bool
	seal                 *presenceImplicationDependencySeal
	stageOrdinal         int
	blockOrdinal         int
	readInventory        state.CoordinateFactorInventory
	writeInventory       state.CoordinateFactorInventory
}

// PresenceImplicationDependencyPlan freezes topology before leaf values are
// known. Slots includes publication-created coordinates; ReducerWrites are the
// exact scalar outputs of the single family-skeleton/publication reducer.
type PresenceImplicationDependencyPlan struct {
	slots  []state.CoordinateSlot
	stages []PresenceImplicationDependencyStage
	access presenceKeyAccess
	valid  bool
	seal   *presenceImplicationDependencySeal
	source PresenceImplicationPlan
	domain state.ProductDomain
}

type PresenceImplicationDependencyStage struct {
	publications          []pathevidence.PathPresenceImplication
	reducerWrites         []state.CoordinateSlot
	reducerWritesSkeleton bool
	blocks                []PresenceImplicationDependencyBlock
	valid                 bool
	seal                  *presenceImplicationDependencySeal
	ordinal               int
	readInventory         state.CoordinateFactorInventory
	writeInventory        state.CoordinateFactorInventory
}

type presenceImplicationDependencySeal struct{}

// PresenceImplicationRootBinding is the immutable, collision-free mapping
// from a sealed dependency plan's neutral roots into one caller vocabulary.
// It is built before leaf execution; the closure kernel never discovers or
// manufactures scalar roots while iterating.
type PresenceImplicationRootBinding[K comparable] struct {
	seal             *presenceImplicationDependencySeal
	roots            map[statekey.ValueDependency]K
	valid            bool
	stageAuthorities []state.CoordinatePathEvidenceAuthority[K]
	blockAuthorities [][]state.CoordinatePathEvidenceAuthority[K]
}

// SealPresenceImplicationRootBinding binds every Values dependency in plan
// exactly once. resolve must be a function and distinct dependencies may not
// alias the same destination root.
func SealPresenceImplicationRootBinding[K comparable](
	plan PresenceImplicationDependencyPlan,
	resolve func(statekey.ValueDependency) (K, bool),
	valid func(K) bool,
) (PresenceImplicationRootBinding[K], error) {
	if !plan.valid || plan.seal == nil || resolve == nil || valid == nil {
		return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: invalid presence root binding authority")
	}
	dependencies := make(map[statekey.ValueDependency]struct{})
	for _, stage := range plan.stages {
		for _, block := range stage.blocks {
			for _, dependency := range append(append([]statekey.ValueDependency(nil), block.valueReads...), block.valueWrites...) {
				if !dependency.Valid() {
					return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: invalid presence root dependency")
				}
				dependencies[dependency] = struct{}{}
			}
		}
	}
	roots := make(map[statekey.ValueDependency]K, len(dependencies))
	used := make(map[K]statekey.ValueDependency, len(dependencies))
	for dependency := range dependencies {
		root, ok := resolve(dependency)
		if !ok || !valid(root) {
			return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: unbound presence root dependency")
		}
		if prior, duplicate := used[root]; duplicate && prior != dependency {
			return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: presence root binding is not injective")
		}
		used[root] = dependency
		roots[dependency] = root
	}
	out := PresenceImplicationRootBinding[K]{seal: plan.seal, roots: roots, valid: true}
	out.stageAuthorities = make([]state.CoordinatePathEvidenceAuthority[K], len(plan.stages))
	out.blockAuthorities = make([][]state.CoordinatePathEvidenceAuthority[K], len(plan.stages))
	for stageIndex, stage := range plan.stages {
		var err error
		out.stageAuthorities[stageIndex], err = state.SealCoordinatePathEvidenceAuthority(
			plan.domain, plan.source.keys, nil, nil, stage.readInventory, stage.writeInventory,
			false, stage.reducerWritesSkeleton, valid,
		)
		if err != nil {
			return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: presence stage %d authority: %w", stageIndex, err)
		}
		out.blockAuthorities[stageIndex] = make([]state.CoordinatePathEvidenceAuthority[K], len(stage.blocks))
		for blockIndex, block := range stage.blocks {
			reads, readOK := out.blockRoots(block.valueReads)
			writes, writeOK := out.blockRoots(block.valueWrites)
			if !readOK || !writeOK {
				return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: unbound presence block authority")
			}
			out.blockAuthorities[stageIndex][blockIndex], err = state.SealCoordinatePathEvidenceAuthority(
				plan.domain, plan.source.keys, reads, writes, block.readInventory, block.writeInventory,
				block.pathMutation, false, valid,
			)
			if err != nil {
				return PresenceImplicationRootBinding[K]{}, fmt.Errorf("factapply: presence stage %d block %d authority: %w", stageIndex, blockIndex, err)
			}
		}
	}
	return out, nil
}

func (b PresenceImplicationRootBinding[K]) StageAuthority(stage PresenceImplicationDependencyStage) (state.CoordinatePathEvidenceAuthority[K], bool) {
	if !b.valid || stage.seal != b.seal || stage.ordinal < 0 || stage.ordinal >= len(b.stageAuthorities) {
		return state.CoordinatePathEvidenceAuthority[K]{}, false
	}
	return b.stageAuthorities[stage.ordinal], true
}

func (b PresenceImplicationRootBinding[K]) BlockAuthority(block PresenceImplicationDependencyBlock) (state.CoordinatePathEvidenceAuthority[K], bool) {
	if !b.valid || block.seal != b.seal || block.stageOrdinal < 0 || block.stageOrdinal >= len(b.blockAuthorities) ||
		block.blockOrdinal < 0 || block.blockOrdinal >= len(b.blockAuthorities[block.stageOrdinal]) {
		return state.CoordinatePathEvidenceAuthority[K]{}, false
	}
	return b.blockAuthorities[block.stageOrdinal][block.blockOrdinal], true
}

func (b PresenceImplicationRootBinding[K]) blockRoots(dependencies []statekey.ValueDependency) ([]K, bool) {
	if !b.valid || b.seal == nil {
		return nil, false
	}
	out := make([]K, len(dependencies))
	for index, dependency := range dependencies {
		root, ok := b.roots[dependency]
		if !ok {
			return nil, false
		}
		out[index] = root
	}
	return out, true
}

// BlockRoots projects one already-sealed block inventory without discovering
// bindings. The returned order is the dependency plan's canonical order.
func (b PresenceImplicationRootBinding[K]) BlockRoots(dependencies []statekey.ValueDependency) ([]K, bool) {
	return b.blockRoots(dependencies)
}

type presenceImplicationDependencyStageBuild struct {
	slots []state.CoordinateSlot
	PresenceImplicationDependencyStage
}

func (p PresenceImplicationDependencyPlan) Slots() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), p.slots...)
}
func (p PresenceImplicationDependencyPlan) Stages() []PresenceImplicationDependencyStage {
	return append([]PresenceImplicationDependencyStage(nil), p.stages...)
}
func (s PresenceImplicationDependencyStage) Publications() []pathevidence.PathPresenceImplication {
	return append([]pathevidence.PathPresenceImplication(nil), s.publications...)
}
func (s PresenceImplicationDependencyStage) ReducerWrites() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), s.reducerWrites...)
}
func (s PresenceImplicationDependencyStage) ReducerWritesSkeleton() bool {
	return s.reducerWritesSkeleton
}
func (s PresenceImplicationDependencyStage) Blocks() []PresenceImplicationDependencyBlock {
	return append([]PresenceImplicationDependencyBlock(nil), s.blocks...)
}
func (b PresenceImplicationDependencyBlock) CoordinateReads() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), b.coordinateReads...)
}
func (b PresenceImplicationDependencyBlock) CoordinateWrites() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), b.coordinateWrites...)
}
func (b PresenceImplicationDependencyBlock) ValueReads() []statekey.Value {
	return concretePresenceDependencies(b.valueReads)
}
func (b PresenceImplicationDependencyBlock) ValueWrites() []statekey.Value {
	return concretePresenceDependencies(b.valueWrites)
}
func (b PresenceImplicationDependencyBlock) ValueReadDependencies() []statekey.ValueDependency {
	return append([]statekey.ValueDependency(nil), b.valueReads...)
}
func (b PresenceImplicationDependencyBlock) ValueWriteDependencies() []statekey.ValueDependency {
	return append([]statekey.ValueDependency(nil), b.valueWrites...)
}

func concretePresenceDependencies(values []statekey.ValueDependency) []statekey.Value {
	out := make([]statekey.Value, 0, len(values))
	for _, dependency := range values {
		if concrete, ok := dependency.Concrete(); ok {
			out = append(out, concrete)
		}
	}
	return out
}
func (b PresenceImplicationDependencyBlock) Publications() []pathevidence.PathPresenceImplication {
	return append([]pathevidence.PathPresenceImplication(nil), b.publications...)
}
func (b PresenceImplicationDependencyBlock) Predecessors() []int {
	return append([]int(nil), b.predecessors...)
}
func (b PresenceImplicationDependencyBlock) RequiresFeedback() bool { return b.requiresFeedback }
func (b PresenceImplicationDependencyBlock) PathMutation() bool     { return b.pathMutation }
func (b PresenceImplicationDependencyBlock) MayContradict() bool    { return b.mayContradict }

// DependencyBlocks constructs the conservative may-alias hypergraph from the
// union coordinate descriptors. Runtime alias expansion is therefore a subset
// of the sealed block outputs and can never discover topology at a leaf.
func (p PresenceImplicationPlan) DependencyBlocks(
	domain state.ProductDomain,
	inventory state.CoordinateFactorInventory,
) (PresenceImplicationDependencyPlan, error) {
	if p.keys == nil || !p.keys.Valid() || !inventory.ValidFor(domain, p.keys) {
		return PresenceImplicationDependencyPlan{}, fmt.Errorf("factapply: invalid presence coordinate inventory")
	}
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		return PresenceImplicationDependencyPlan{}, fmt.Errorf("factapply: presence coordinate family is not registered")
	}
	slots, err := inventory.FamilySlots(family)
	if err != nil {
		return PresenceImplicationDependencyPlan{}, err
	}
	return p.dependencyBlocksFromSlots(domain, slots)
}

func (p PresenceImplicationPlan) dependencyBlocksFromSlots(
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) (PresenceImplicationDependencyPlan, error) {
	groups := make([][]pathevidence.PathPresenceImplication, 0)
	pending := make([]pathevidence.PathPresenceImplication, 0)
	flush := func() {
		if len(pending) != 0 {
			groups = append(groups, append([]pathevidence.PathPresenceImplication(nil), pending...))
			pending = pending[:0]
		}
	}
	if p.barriers == ConcretePresenceImplicationDescendantInvalidationBarriers {
		for _, publication := range p.publications {
			if presenceImplicationTargetInvalidatesDescendants(publication) {
				flush()
				groups = append(groups, []pathevidence.PathPresenceImplication{publication})
				continue
			}
			pending = append(pending, publication)
		}
		flush()
	} else {
		pending = append(pending, p.publications...)
		flush()
	}
	if len(groups) == 0 {
		groups = append(groups, nil)
	}
	seal := new(presenceImplicationDependencySeal)
	out := PresenceImplicationDependencyPlan{slots: append([]state.CoordinateSlot(nil), slots...), valid: true, seal: seal, source: p, domain: domain}
	for stageOrdinal, publications := range groups {
		stagePlan := p
		stagePlan.publications = publications
		stage, err := stagePlan.dependencyStage(domain, out.slots)
		if err != nil {
			return PresenceImplicationDependencyPlan{}, err
		}
		out.slots = stage.slots
		sealedStage := stage.PresenceImplicationDependencyStage
		sealedStage = presencePublicationAffectedStage(sealedStage)
		sealedStage.seal = seal
		sealedStage.ordinal = stageOrdinal
		sealedStage.readInventory, err = domain.SealCoordinateFactorInventory(p.keys, sealedStage.reducerWrites)
		if err != nil {
			return PresenceImplicationDependencyPlan{}, err
		}
		sealedStage.writeInventory = sealedStage.readInventory
		for index := range sealedStage.blocks {
			sealedStage.blocks[index].seal = seal
			sealedStage.blocks[index].stageOrdinal = stageOrdinal
			sealedStage.blocks[index].blockOrdinal = index
			sealedStage.blocks[index].readInventory, err = domain.SealCoordinateFactorInventory(p.keys, sealedStage.blocks[index].coordinateReads)
			if err != nil {
				return PresenceImplicationDependencyPlan{}, err
			}
			sealedStage.blocks[index].writeInventory, err = domain.SealCoordinateFactorInventory(p.keys, sealedStage.blocks[index].coordinateWrites)
			if err != nil {
				return PresenceImplicationDependencyPlan{}, err
			}
		}
		out.stages = append(out.stages, sealedStage)
	}
	support, err := domain.PathCoordinateSupportPaths(out.slots)
	if err != nil {
		return PresenceImplicationDependencyPlan{}, err
	}
	support = append(support, presenceImplicationPaths(p.publications)...)
	extra, accessErr := freezePresenceKeyAccess(p.resolver, p.point, p.keys, support)
	if accessErr != nil {
		return PresenceImplicationDependencyPlan{}, accessErr
	}
	if p.access.valid() {
		out.access, err = p.access.merge(extra)
	} else {
		out.access = extra
	}
	if err != nil {
		return PresenceImplicationDependencyPlan{}, err
	}
	out.source.keys = p.keys
	out.source.access = out.access
	out.source.resolver = nil
	return out, nil
}

// presencePublicationAffectedStage applies the semi-naive delta law to one
// ordered publication barrier. The reducer changes only its published row
// membership coordinates; therefore the exact consequence work is the SCCs
// owning those rows plus their downstream condensation cone. Unrelated sealed
// rows remain in the coordinate inventory but are never replayed.
func presencePublicationAffectedStage(stage PresenceImplicationDependencyStage) PresenceImplicationDependencyStage {
	if len(stage.publications) == 0 || len(stage.blocks) == 0 {
		return stage
	}
	selected := make([]bool, len(stage.blocks))
	for index, block := range stage.blocks {
		selected[index] = len(block.publications) != 0
	}
	for changed := true; changed; {
		changed = false
		for index, block := range stage.blocks {
			if selected[index] {
				continue
			}
			for _, predecessor := range block.predecessors {
				if predecessor >= 0 && predecessor < len(selected) && selected[predecessor] {
					selected[index], changed = true, true
					break
				}
			}
		}
	}
	remap := make(map[int]int)
	blocks := make([]PresenceImplicationDependencyBlock, 0, len(stage.blocks))
	for oldIndex, block := range stage.blocks {
		if !selected[oldIndex] {
			continue
		}
		remap[oldIndex] = len(blocks)
		copyBlock := block
		copyBlock.predecessors = nil
		for _, predecessor := range block.predecessors {
			if mapped, present := remap[predecessor]; present {
				copyBlock.predecessors = append(copyBlock.predecessors, mapped)
			}
		}
		blocks = append(blocks, copyBlock)
	}
	stage.blocks = blocks
	return stage
}

func (p PresenceImplicationPlan) dependencyStage(
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) (presenceImplicationDependencyStageBuild, error) {
	if p.reg == nil || p.keys == nil || !p.keys.Valid() || !domain.Valid() || domain.Registry() != p.reg {
		return presenceImplicationDependencyStageBuild{}, fmt.Errorf("factapply: invalid presence dependency plan")
	}
	keys := p.keys
	publicationSlots := make(map[pathevidence.PathPresenceImplication]state.CoordinateSlot, len(p.publications))
	dependencySlots := append([]state.CoordinateSlot(nil), slots...)
	for _, publication := range p.publications {
		slot, slotErr := domain.PresenceImplicationCoordinateSlot(keys, publication)
		if slotErr != nil {
			return presenceImplicationDependencyStageBuild{}, slotErr
		}
		publicationSlots[publication] = slot
		dependencySlots = appendUniqueCoordinateSlot(domain, dependencySlots, slot)
	}
	if err := sortPresenceCoordinateSlots(domain, dependencySlots); err != nil {
		return presenceImplicationDependencyStageBuild{}, err
	}
	var canonical []pathevidence.PathPresenceImplication
	var rowSlots []state.CoordinateSlot
	for _, slot := range dependencySlots {
		shapes, shapeErr := domain.PresenceImplicationShapes(keys, []state.CoordinateSlot{slot})
		if shapeErr != nil {
			return presenceImplicationDependencyStageBuild{}, shapeErr
		}
		if len(shapes) == 1 {
			canonical = append(canonical, shapes[0])
			rowSlots = append(rowSlots, slot)
		}
	}
	if len(canonical) == 0 {
		augmented := append([]state.CoordinateSlot(nil), slots...)
		if err := sortPresenceCoordinateSlots(domain, augmented); err != nil {
			return presenceImplicationDependencyStageBuild{}, err
		}
		return presenceImplicationDependencyStageBuild{slots: augmented, PresenceImplicationDependencyStage: PresenceImplicationDependencyStage{valid: true}}, nil
	}
	seeds := make([]state.CoordinateDependencySeed, len(canonical))
	for index, row := range canonical {
		rowSlot := rowSlots[index]
		seed := state.CoordinateDependencySeed{
			ID:              state.CoordinateDependencyID(index + 1),
			ReadPaths:       []keyspace.Key{row.Trigger},
			WritePaths:      []keyspace.Key{row.Target},
			ReadCoordinates: []state.CoordinateSlot{rowSlot},
		}
		if row.HasTriggerPathEqual {
			seed.ReadPaths = append(seed.ReadPaths, row.TriggerOther)
		}
		if presenceImplicationTargetInvalidatesDescendants(row) {
			seed.DescendantMutationRoots = []keyspace.Key{row.Target}
		}
		seeds[index] = seed
	}
	dependencyPlan, err := domain.PlanPathCoordinateDependencies(keys, dependencySlots, seeds)
	if err != nil {
		return presenceImplicationDependencyStageBuild{}, err
	}
	dependencies := make([]state.CoordinateDependency, len(canonical))
	for index, seed := range seeds {
		dependency, present := dependencyPlan.Dependency(seed.ID)
		if !present {
			return presenceImplicationDependencyStageBuild{}, fmt.Errorf("factapply: incomplete path dependency certificate")
		}
		dependencies[index] = dependency
	}
	components, ok := presenceDependencyComponents(len(dependencies), func(writer, reader int) bool {
		return dependencyPlan.Depends(dependencies[writer].ID(), dependencies[reader].ID())
	})
	if !ok {
		return presenceImplicationDependencyStageBuild{}, fmt.Errorf("factapply: invalid presence dependency schedule")
	}
	augmented := dependencyPlan.Coordinates()
	out := presenceImplicationDependencyStageBuild{slots: augmented, PresenceImplicationDependencyStage: PresenceImplicationDependencyStage{
		publications: append([]pathevidence.PathPresenceImplication(nil), p.publications...), reducerWritesSkeleton: len(p.publications) != 0, valid: true,
	}}
	for _, publication := range p.publications {
		out.reducerWrites = appendUniqueCoordinateSlot(domain, out.reducerWrites, publicationSlots[publication])
	}
	componentOf := make([]int, len(dependencies))
	for blockIndex, component := range components {
		for _, rowIndex := range component {
			componentOf[rowIndex] = blockIndex
		}
	}
	predecessors := make([][]int, len(components))
	requiresFeedback := make([]bool, len(components))
	for blockIndex, component := range components {
		local := make(map[int]int, len(component))
		for index, row := range component {
			local[row] = index
		}
		color := make([]uint8, len(component))
		var visit func(int) bool
		visit = func(writer int) bool {
			color[writer] = 1
			for _, readerRow := range component {
				reader := local[readerRow]
				if !dependencyPlan.Feeds(dependencies[component[writer]].ID(), dependencies[readerRow].ID()) {
					continue
				}
				if color[reader] == 1 || color[reader] == 0 && visit(reader) {
					return true
				}
			}
			color[writer] = 2
			return false
		}
		for row := range component {
			if color[row] == 0 && visit(row) {
				requiresFeedback[blockIndex] = true
				break
			}
		}
	}
	for writer := range dependencies {
		for reader := range dependencies {
			if !dependencyPlan.Feeds(dependencies[writer].ID(), dependencies[reader].ID()) {
				continue
			}
			from, to := componentOf[writer], componentOf[reader]
			if from == to {
				continue
			}
			if from >= to {
				return presenceImplicationDependencyStageBuild{}, fmt.Errorf("factapply: presence dependency condensation is not topological")
			}
			present := false
			for _, predecessor := range predecessors[to] {
				present = present || predecessor == from
			}
			if !present {
				predecessors[to] = append(predecessors[to], from)
			}
		}
	}
	for blockIndex := range predecessors {
		sort.Ints(predecessors[blockIndex])
	}
	for blockIndex, component := range components {
		block := PresenceImplicationDependencyBlock{valid: true, mayContradict: len(component) != 0}
		block.predecessors = append([]int(nil), predecessors[blockIndex]...)
		block.requiresFeedback = requiresFeedback[blockIndex]
		for _, rowIndex := range component {
			row := canonical[rowIndex]
			rowSlot := rowSlots[rowIndex]
			dependency := dependencies[rowIndex]
			block.rows = append(block.rows, row)
			block.rowSlots = append(block.rowSlots, rowSlot)
			for _, slot := range dependency.CoordinateReads() {
				block.coordinateReads = appendUniqueCoordinateSlot(domain, block.coordinateReads, slot)
			}
			for _, slot := range dependency.CoordinateWrites() {
				block.coordinateWrites = appendUniqueCoordinateSlot(domain, block.coordinateWrites, slot)
			}
			for _, location := range dependency.LocationReads() {
				if location.IsRoot() {
					block.valueReads = appendUniqueValueDependency(block.valueReads, location.Root)
				}
			}
			for _, location := range dependency.LocationWrites() {
				if location.IsRoot() {
					block.valueReads = appendUniqueValueDependency(block.valueReads, location.Root)
					block.valueWrites = appendUniqueValueDependency(block.valueWrites, location.Root)
				}
			}
			block.pathMutation = block.pathMutation || len(dependency.MutationRegions()) != 0
			for publication, slot := range publicationSlots {
				equal, equalErr := domain.CoordinateSlotEqual(slot, rowSlot)
				if equalErr != nil {
					return presenceImplicationDependencyStageBuild{}, equalErr
				}
				if equal {
					block.publications = append(block.publications, publication)
				}
			}
		}
		if err := sortPresenceCoordinateSlots(domain, block.coordinateReads); err != nil {
			return presenceImplicationDependencyStageBuild{}, err
		}
		if err := sortPresenceCoordinateSlots(domain, block.coordinateWrites); err != nil {
			return presenceImplicationDependencyStageBuild{}, err
		}
		out.blocks = append(out.blocks, block)
	}
	return out, nil
}

// ApplyCoordinateReducer atomically publishes one ordered barrier stage into
// the family carrier. It performs storage insertion only; consequence blocks
// then close the exact SCCs in stage order through ApplyCoordinateBlock.
func ApplyPresenceImplicationCoordinateReducer[K comparable](
	p PresenceImplicationDependencyPlan,
	carrier *state.CoordinatePathEvidenceCarrier[K],
	stage PresenceImplicationDependencyStage,
	authority state.CoordinatePathEvidenceAuthority[K],
) error {
	if !p.valid || p.seal == nil || stage.seal != p.seal || p.source.reg == nil || !p.access.valid() || carrier == nil || !carrier.Valid() {
		return fmt.Errorf("factapply: invalid presence coordinate reducer")
	}
	if !carrier.MatchesAuthority(authority) {
		return fmt.Errorf("factapply: presence coordinate reducer authority mismatch")
	}
	staged := carrier.Clone()
	if staged == nil {
		return fmt.Errorf("factapply: presence coordinate reducer cannot fork")
	}
	storage := &coordinatePresenceStorage[K]{value: staged, feasible: true}
	if !stage.valid {
		return fmt.Errorf("factapply: unsealed presence coordinate reducer stage")
	}
	for _, publication := range stage.publications {
		if _, ok := storage.AddImplication(publication); !ok {
			return fmt.Errorf("factapply: invalid coordinate implication publication")
		}
	}
	if !carrier.Commit(staged) {
		return fmt.Errorf("factapply: coordinate implication publication commit failed")
	}
	return nil
}

// ApplyCoordinateBlock closes one presealed dependency SCC using the same
// immutable-round consequence kernel as concrete State execution. The caller
// opens the carrier with exactly block's declared reads/writes and freezes it
// after success; runtime topology cannot expand the block.
func ApplyPresenceImplicationCoordinateBlock[K comparable](
	p PresenceImplicationDependencyPlan,
	ctx context.Context,
	carrier *state.CoordinatePathEvidenceCarrier[K],
	block PresenceImplicationDependencyBlock,
	binding PresenceImplicationRootBinding[K],
) (bool, error) {
	if ctx == nil || !p.valid || p.seal == nil || block.seal != p.seal || p.source.reg == nil || !p.access.valid() || carrier == nil || !carrier.Valid() || !block.valid {
		return false, fmt.Errorf("factapply: invalid presence coordinate block")
	}
	_, readOK := binding.blockRoots(block.valueReads)
	_, writeOK := binding.blockRoots(block.valueWrites)
	authority, authorityOK := binding.BlockAuthority(block)
	if binding.seal != p.seal || !readOK || !writeOK || !authorityOK || !carrier.MatchesAuthority(authority) {
		return false, fmt.Errorf("factapply: presence coordinate block authority mismatch")
	}
	storage := &coordinatePresenceStorage[K]{value: carrier, roots: binding.roots, feasible: true}
	rows, rowErr := presenceImplicationBlockRows(p, storage, block)
	if rowErr != nil {
		return false, rowErr
	}
	canceled, err := closePresenceImplicationRows(
		p.source.reg, p.access, storage, rows, tokenOf(cancellation.FromContext(ctx)),
	)
	if err != nil {
		return false, err
	}
	if canceled {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		return false, context.Canceled
	}
	return storage.feasible, nil
}

// ApplyCoordinateRound evaluates exactly one consequence round for one
// presealed dependency block. carrier is the immutable trigger snapshot; the
// returned carrier is an independent patch candidate. Runtime row inventory
// cannot enlarge block, and this method never performs SCC discovery,
// restart, or until-no-change iteration.
func ApplyPresenceImplicationCoordinateRound[K comparable](
	p PresenceImplicationDependencyPlan,
	ctx context.Context,
	carrier *state.CoordinatePathEvidenceCarrier[K],
	block PresenceImplicationDependencyBlock,
	binding PresenceImplicationRootBinding[K],
) (next *state.CoordinatePathEvidenceCarrier[K], feasible, changed bool, err error) {
	if ctx == nil || !p.valid || p.seal == nil || block.seal != p.seal || p.source.reg == nil ||
		!p.access.valid() || carrier == nil || !carrier.Valid() || !block.valid {
		return nil, false, false, fmt.Errorf("factapply: invalid presence coordinate round")
	}
	_, readOK := binding.blockRoots(block.valueReads)
	_, writeOK := binding.blockRoots(block.valueWrites)
	authority, authorityOK := binding.BlockAuthority(block)
	if binding.seal != p.seal || !readOK || !writeOK || !authorityOK || !carrier.MatchesAuthority(authority) {
		return nil, false, false, fmt.Errorf("factapply: presence coordinate round authority mismatch")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, false, err
	}
	next = carrier.Clone()
	if next == nil {
		return nil, false, false, fmt.Errorf("factapply: presence coordinate round cannot fork")
	}
	var storage presenceImplicationStorage = &coordinatePresenceStorage[K]{value: next, roots: binding.roots, feasible: true}
	if len(block.predicateActivations) != 0 {
		storage = &careActivatedPresenceStorage{
			presenceImplicationStorage: storage, reg: p.source.reg, keys: p.access.keys,
			activations: append([]pathPredicateActivation(nil), block.predicateActivations...),
		}
	}
	rows, rowErr := presenceImplicationBlockRows(p, storage, block)
	if rowErr != nil {
		return nil, false, false, rowErr
	}
	changed, err = applyPresenceImplicationRowsRound(
		p.source.reg, p.access, storage, rows,
	)
	if err != nil {
		return nil, false, false, err
	}
	coordinate := storage
	if activated, ok := storage.(*careActivatedPresenceStorage); ok {
		coordinate = activated.presenceImplicationStorage
	}
	base, ok := coordinate.(*coordinatePresenceStorage[K])
	if !ok {
		return nil, false, false, fmt.Errorf("factapply: invalid presence coordinate storage result")
	}
	return next, base.feasible, changed, nil
}

func presenceImplicationBlockRows(
	p PresenceImplicationDependencyPlan,
	storage presenceImplicationStorage,
	block PresenceImplicationDependencyBlock,
) ([]pathevidence.PathPresenceImplication, error) {
	if !p.domain.Valid() || storage == nil {
		return nil, fmt.Errorf("factapply: invalid presence implication block inventory")
	}
	rows, ok := storage.SnapshotImplications()
	if !ok {
		return nil, fmt.Errorf("factapply: presence implication block snapshot failed")
	}
	out := make([]pathevidence.PathPresenceImplication, 0, len(rows))
	for _, row := range rows {
		slot, err := p.domain.PresenceImplicationCoordinateSlot(p.access.keys, row)
		if err != nil {
			return nil, err
		}
		for _, owned := range block.rowSlots {
			equal, equalErr := p.domain.CoordinateSlotEqual(slot, owned)
			if equalErr != nil {
				return nil, equalErr
			}
			if equal {
				out = append(out, row)
				break
			}
		}
	}
	return out, nil
}

func appendUniqueCoordinateSlot(domain state.ProductDomain, out []state.CoordinateSlot, value state.CoordinateSlot) []state.CoordinateSlot {
	for _, prior := range out {
		equal, err := domain.CoordinateSlotEqual(prior, value)
		if err == nil && equal {
			return out
		}
	}
	return append(out, value)
}

func appendUniqueValueDependency(out []statekey.ValueDependency, value statekey.ValueDependency) []statekey.ValueDependency {
	if !value.Valid() {
		return out
	}
	for _, prior := range out {
		if prior == value {
			return out
		}
	}
	return append(out, value)
}

func sortPresenceCoordinateSlots(domain state.ProductDomain, slots []state.CoordinateSlot) error {
	for index := 1; index < len(slots); index++ {
		for current := index; current > 0; current-- {
			less, err := domain.CoordinateSlotLess(slots[current], slots[current-1])
			if err != nil {
				return err
			}
			if !less {
				break
			}
			slots[current], slots[current-1] = slots[current-1], slots[current]
		}
	}
	return nil
}
