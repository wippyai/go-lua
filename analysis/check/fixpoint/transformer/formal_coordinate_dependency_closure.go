package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalCoordinateDependencyClosure is the sole static fixed point for the
// coordinate vocabulary. Local operator declarations and cross-body Apply
// images are one mutually recursive system: separating them into a local
// refold followed by whole-program Apply rounds both repeated work and could
// miss a footprint change when the body inventory retained the same length.
//
// Every node below has a stable dense ID. Edges are sealed and sorted once;
// evaluation enqueues only exact dependents whose canonical set value may
// change. There is no iteration budget and no solve-time WTO reuse.
type formalCoordinateDependencyClosure struct {
	program *RelationProgram
	region  *formalRelationRegionInventory
	forest  *formalFiberInventory
	keys    []*keyspace.KeySpace
	rekeys  []state.CoordinateFormalRootRekey

	bodySeeds []state.CoordinateFactorInventory
	bodies    []state.CoordinateFactorInventory
	pointwise []relationCoordinateFactorInventory

	cells                     []formalRelationCell
	cellIndex                 map[formalRelationCell]int
	cellValue                 []formalOperatorCoordinateFootprint
	cellBody                  [][]int
	cellFrames                [][]int
	cellInputIdentity         []formalIdentityEnvironment
	cellIdentity              []formalIdentityEnvironment
	cellIdentityFolds         []formalIdentityEnvironmentFold
	cellIdentityContributions [][]formalIdentityContribution
	cellIdentityWrites        [][]formalDynamicIdentityPublication
	identityProducerCatalogs  []map[state.DynamicReadIdentityProducer]struct{}
	identityProducerIndexes   []state.DynamicReadIdentityProducerIndex

	selectors      []state.CoordinateFactorInventory
	selectorCells  [][]int
	selectorMember []map[int]struct{}

	frames           []formalStaticApplyCoordinateFrame
	frameByOwnerTerm map[formalFrameFootprintKey]int
	bodyFolds        []formalCoordinateInventoryFold
	selectorFolds    []formalCoordinateInventoryFold
	cellFolds        []formalCoordinateInventoryFold
	cellSourceFolds  []*formalCoordinateInventoryFold

	dependents [][]int
	queued     []bool
	queue      []int
	head       int

	bodyNodeFirst, cellNodeFirst, selectorNodeFirst, frameNodeFirst int
}

// formalContributionFold is the one fixed-shape contribution tree used by
// both coordinate inventories and identity environments. Leaves retain exact
// producer identities; changing one leaf recomputes only its logarithmic
// ancestor path in the sealed canonical order.
type formalContributionFold[T any] struct {
	empty T
	base  int
	width int
	nodes []T
	equal func(T, T) (bool, error)
	join  func(T, T) (T, error)
	valid func(T) bool
}

func newFormalContributionFold[T any](
	width int,
	empty T,
	equal func(T, T) (bool, error),
	join func(T, T) (T, error),
	valid func(T) bool,
) formalContributionFold[T] {
	base := 1
	for base < width {
		base <<= 1
	}
	if width == 0 {
		width = 1
	}
	fold := formalContributionFold[T]{
		empty: empty, base: base, width: width, nodes: make([]T, base*2),
		equal: equal, join: join, valid: valid,
	}
	for index := 1; index < len(fold.nodes); index++ {
		fold.nodes[index] = empty
	}
	return fold
}

func (f *formalContributionFold[T]) set(index int, next T) (bool, error) {
	if f == nil || index < 0 || index >= f.width || f.equal == nil || f.join == nil || f.valid == nil || !f.valid(next) {
		return false, fmt.Errorf("transformer: formal contribution is unowned")
	}
	leaf := f.base + index
	equal, err := f.equal(f.nodes[leaf], next)
	if err != nil || equal {
		return false, err
	}
	f.nodes[leaf] = next
	for leaf >>= 1; leaf != 0; leaf >>= 1 {
		joined, joinErr := f.join(f.nodes[leaf<<1], f.nodes[leaf<<1|1])
		if joinErr != nil {
			return false, joinErr
		}
		f.nodes[leaf] = joined
	}
	return true, nil
}

func (f *formalContributionFold[T]) root() T {
	if f == nil || len(f.nodes) < 2 {
		var zero T
		return zero
	}
	return f.nodes[1]
}

type formalCoordinateInventoryFold struct {
	domain state.ProductDomain
	keys   *keyspace.KeySpace
	empty  state.CoordinateFactorInventory
	tree   formalContributionFold[state.CoordinateFactorInventory]
}

// formalIdentityContributionID is the sealed position of one identity-flow
// contribution. ID zero is reserved for a root cell's body seed; remaining
// IDs follow the canonical incoming-influence order frozen with the region.
type formalIdentityContributionID uint32

type formalIdentityContribution struct {
	target int
	id     formalIdentityContributionID
}

type formalIdentityEnvironmentFold struct {
	tree formalContributionFold[formalIdentityEnvironment]
}

func newFormalIdentityEnvironmentFold(closure *formalCoordinateDependencyClosure, body, width int) formalIdentityEnvironmentFold {
	empty := formalIdentityEnvironment{}
	return formalIdentityEnvironmentFold{tree: newFormalContributionFold(
		width,
		empty,
		func(left, right formalIdentityEnvironment) (bool, error) {
			return closure.formalIdentityEnvironmentEqual(left, right), nil
		},
		func(left, right formalIdentityEnvironment) (formalIdentityEnvironment, error) {
			return closure.unionFormalIdentityEnvironments(body, left, right)
		},
		func(formalIdentityEnvironment) bool { return closure != nil },
	)}
}

func (f *formalIdentityEnvironmentFold) set(id formalIdentityContributionID, next formalIdentityEnvironment) (bool, error) {
	return f.tree.set(int(id), next)
}

func (f *formalIdentityEnvironmentFold) root() formalIdentityEnvironment {
	if f == nil {
		return formalIdentityEnvironment{}
	}
	return f.tree.root()
}

func newFormalCoordinateInventoryFold(domain state.ProductDomain, keys *keyspace.KeySpace, width int) (formalCoordinateInventoryFold, error) {
	empty, err := domain.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		return formalCoordinateInventoryFold{}, err
	}
	fold := formalCoordinateInventoryFold{domain: domain, keys: keys, empty: empty}
	fold.tree = newFormalContributionFold(
		width,
		empty,
		domain.CoordinateFactorInventoriesEqual,
		func(left, right state.CoordinateFactorInventory) (state.CoordinateFactorInventory, error) {
			return domain.UnionCoordinateFactorInventories(keys, left, right)
		},
		func(value state.CoordinateFactorInventory) bool { return value.ValidFor(domain, keys) },
	)
	return fold, nil
}

func (f *formalCoordinateInventoryFold) set(index int, next state.CoordinateFactorInventory) error {
	if f == nil {
		return fmt.Errorf("transformer: formal coordinate contribution is unowned")
	}
	_, err := f.tree.set(index, next)
	return err
}

func (f *formalCoordinateInventoryFold) contribution(value state.CoordinateFactorInventory) state.CoordinateFactorInventory {
	if value.KeySpace() == nil {
		return f.empty
	}
	return value
}

func (f *formalCoordinateInventoryFold) root() state.CoordinateFactorInventory { return f.tree.root() }

type formalStaticApplyCoordinateFrame struct {
	caller, target int
	frame          *linkedRelationFrame
	cells          []int
	wirePlans      []state.CoordinateFormalRootRekey
	rootMap        state.BoundaryRootMap
	sourceRoots    []state.BoundaryFactorRoot
	footprint      state.BoundaryCoordinateFootprintPlan
	mapped         state.CoordinateFactorInventory
	sourceOwned    []state.CoordinateSlot
	image          state.CoordinateFactorInventory
	selector       state.CoordinateFactorInventory
	identityImage  *state.CoordinateIdentityTermImage
	resultSupport  []formalIdentitySupport
	inputIdentity  formalIdentityEnvironment
	outputIdentity formalIdentityEnvironment
}

func freezeFormalCoordinateDependencyClosure(
	program *RelationProgram,
	region *formalRelationRegionInventory,
	forest *formalFiberInventory,
	formalKeys []*keyspace.KeySpace,
	rekeys []state.CoordinateFormalRootRekey,
	inventories []state.CoordinateFactorInventory,
) error {
	closure, err := newFormalCoordinateDependencyClosure(program, region, forest, formalKeys, rekeys, inventories)
	if err != nil {
		return err
	}
	if err := closure.solve(); err != nil {
		return err
	}
	copy(inventories, closure.bodies)
	return closure.publish()
}

func newFormalCoordinateDependencyClosure(
	program *RelationProgram,
	region *formalRelationRegionInventory,
	forest *formalFiberInventory,
	formalKeys []*keyspace.KeySpace,
	rekeys []state.CoordinateFormalRootRekey,
	inventories []state.CoordinateFactorInventory,
) (*formalCoordinateDependencyClosure, error) {
	if program == nil || region == nil || forest == nil || len(formalKeys) != len(program.bodies) ||
		len(rekeys) != len(program.bodies) || len(inventories) != len(program.bodies) {
		return nil, fmt.Errorf("transformer: formal coordinate dependency closure is unowned")
	}
	c := &formalCoordinateDependencyClosure{
		program: program, region: region, forest: forest, keys: formalKeys, rekeys: rekeys,
		bodySeeds:                 append([]state.CoordinateFactorInventory(nil), inventories...),
		bodies:                    append([]state.CoordinateFactorInventory(nil), inventories...),
		pointwise:                 make([]relationCoordinateFactorInventory, len(program.bodies)),
		cells:                     append([]formalRelationCell(nil), region.cells...),
		cellIndex:                 make(map[formalRelationCell]int, len(region.cells)),
		cellValue:                 make([]formalOperatorCoordinateFootprint, len(region.cells)),
		cellBody:                  make([][]int, len(program.bodies)),
		cellFrames:                make([][]int, len(region.cells)),
		cellInputIdentity:         make([]formalIdentityEnvironment, len(region.cells)),
		cellIdentity:              make([]formalIdentityEnvironment, len(region.cells)),
		cellIdentityContributions: make([][]formalIdentityContribution, len(region.cells)),
		cellIdentityWrites:        make([][]formalDynamicIdentityPublication, len(region.cells)),
		identityProducerCatalogs:  make([]map[state.DynamicReadIdentityProducer]struct{}, len(program.bodies)),
		identityProducerIndexes:   make([]state.DynamicReadIdentityProducerIndex, len(program.bodies)),
		selectors:                 make([]state.CoordinateFactorInventory, len(program.bodies)),
		selectorCells:             make([][]int, len(program.bodies)),
		selectorMember:            make([]map[int]struct{}, len(program.bodies)),
		frameByOwnerTerm:          make(map[formalFrameFootprintKey]int),
	}
	for index, cell := range c.cells {
		c.cellIndex[cell] = index
		if cell.Variable == 0 || int(cell.Variable) > len(program.bodies) {
			return nil, fmt.Errorf("transformer: formal coordinate cell has foreign owner")
		}
		c.cellBody[cell.Variable-1] = append(c.cellBody[cell.Variable-1], index)
	}
	for index, cell := range c.cells {
		if cell.Kind != formalRelationCellStep {
			continue
		}
		body := &program.bodies[cell.Variable-1]
		step := body.relation.code.nodes[cell.Root].steps[cell.Step-1]
		writes, freezeErr := freezeFormalDynamicIdentityPublications(body, formalKeys[cell.Variable-1], rekeys[cell.Variable-1], step)
		if freezeErr != nil {
			return nil, freezeErr
		}
		c.cellIdentityWrites[index] = writes
	}
	for bodyIndex := range program.bodies {
		c.identityProducerCatalogs[bodyIndex] = make(map[state.DynamicReadIdentityProducer]struct{})
		for _, cellIndex := range c.cellBody[bodyIndex] {
			for _, publication := range c.cellIdentityWrites[cellIndex] {
				c.identityProducerCatalogs[bodyIndex][publication.producer] = struct{}{}
			}
		}
		atoms := make([]state.DynamicReadIdentityProducer, 0, len(c.identityProducerCatalogs[bodyIndex]))
		for atom := range c.identityProducerCatalogs[bodyIndex] {
			atoms = append(atoms, atom)
		}
		index, err := program.bodies[bodyIndex].productDomain.SealDynamicReadIdentityProducerIndex(formalKeys[bodyIndex], atoms)
		if err != nil {
			return nil, err
		}
		c.identityProducerIndexes[bodyIndex] = index
	}
	for bodyIndex := range program.bodies {
		body := &program.bodies[bodyIndex]
		if body.pathSemantics != nil && body.pathSemantics.Valid() {
			pointwise, err := freezeRelationCoordinateFactorInventory(body)
			if err != nil {
				return nil, err
			}
			c.pointwise[bodyIndex] = pointwise
		}
		empty, err := body.productDomain.SealCoordinateFactorInventory(formalKeys[bodyIndex], nil)
		if err != nil {
			return nil, err
		}
		c.selectors[bodyIndex] = empty
		cone, err := freezeFormalNormalTerminalConeCells(body, region)
		if err != nil {
			return nil, err
		}
		c.selectorMember[bodyIndex] = make(map[int]struct{}, len(cone))
		for _, cell := range cone {
			index, ok := c.cellIndex[cell]
			if !ok {
				return nil, fmt.Errorf("transformer: formal terminal cone names undeclared cell")
			}
			c.selectorCells[bodyIndex] = append(c.selectorCells[bodyIndex], index)
			c.selectorMember[bodyIndex][index] = struct{}{}
		}
	}
	frames, err := freezeFormalStaticApplyCoordinateFrames(c)
	if err != nil {
		return nil, err
	}
	c.frames = frames
	for frameIndex := range c.frames {
		frame := &c.frames[frameIndex]
		key := formalFrameFootprintKey{variable: relationVar(frame.caller + 1), frame: frame.frame.term}
		if _, duplicate := c.frameByOwnerTerm[key]; duplicate {
			return nil, fmt.Errorf("transformer: duplicate formal coordinate Apply frame")
		}
		c.frameByOwnerTerm[key] = frameIndex
		frame.resultSupport = make([]formalIdentitySupport, int(frame.frame.shape.Results))
		for _, cell := range frame.cells {
			c.cellFrames[cell] = append(c.cellFrames[cell], frameIndex)
		}
		if len(frame.sourceOwned) != 0 {
			body := &program.bodies[frame.target]
			owned, err := body.productDomain.SealCoordinateFactorInventory(formalKeys[frame.target], frame.sourceOwned)
			if err != nil {
				return nil, err
			}
			joined, err := body.productDomain.UnionCoordinateFactorInventories(formalKeys[frame.target], c.bodySeeds[frame.target], owned)
			if err != nil {
				return nil, err
			}
			c.bodySeeds[frame.target], err = body.productDomain.CloseCoordinateFactorInventory(formalKeys[frame.target], joined)
			if err != nil {
				return nil, err
			}
			c.bodies[frame.target] = c.bodySeeds[frame.target]
		}
	}
	if err := c.initializeContributionFolds(); err != nil {
		return nil, err
	}
	c.sealDependencies()
	return c, nil
}

func (c *formalCoordinateDependencyClosure) initializeContributionFolds() error {
	c.bodyFolds = make([]formalCoordinateInventoryFold, len(c.program.bodies))
	c.selectorFolds = make([]formalCoordinateInventoryFold, len(c.program.bodies))
	for bodyIndex := range c.program.bodies {
		body := &c.program.bodies[bodyIndex]
		var err error
		c.bodyFolds[bodyIndex], err = newFormalCoordinateInventoryFold(body.productDomain, c.keys[bodyIndex], 1+len(c.cellBody[bodyIndex]))
		if err != nil {
			return err
		}
		c.selectorFolds[bodyIndex], err = newFormalCoordinateInventoryFold(body.productDomain, c.keys[bodyIndex], len(c.selectorCells[bodyIndex]))
		if err != nil {
			return err
		}
	}
	c.cellFolds = make([]formalCoordinateInventoryFold, len(c.cells))
	c.cellSourceFolds = make([]*formalCoordinateInventoryFold, len(c.cells))
	c.cellIdentityFolds = make([]formalIdentityEnvironmentFold, len(c.cells))
	for cellIndex, cell := range c.cells {
		bodyIndex := int(cell.Variable - 1)
		body := &c.program.bodies[bodyIndex]
		fold, err := newFormalCoordinateInventoryFold(body.productDomain, c.keys[bodyIndex], 1+len(c.cellFrames[cellIndex]))
		if err != nil {
			return err
		}
		c.cellFolds[cellIndex] = fold
		incoming := append([]formalRelationInfluence(nil), c.region.incoming[cell]...)
		sortFormalRelationInfluences(incoming)
		identityWidth := 1
		for _, influence := range incoming {
			if influence.Source.Variable != cell.Variable || influence.Kind == formalRelationInfluenceStepPublishedRead || influence.Kind == formalRelationInfluenceClosureDefinition {
				continue
			}
			source, present := c.cellIndex[influence.Source]
			if !present {
				continue
			}
			id := formalIdentityContributionID(identityWidth)
			identityWidth++
			c.cellIdentityContributions[source] = append(c.cellIdentityContributions[source], formalIdentityContribution{target: cellIndex, id: id})
		}
		c.cellIdentityFolds[cellIndex] = newFormalIdentityEnvironmentFold(c, bodyIndex, identityWidth)
		if cell.Kind == formalRelationCellResource || len(c.cellFrames[cellIndex]) == 0 {
			continue
		}
		target := c.frames[c.cellFrames[cellIndex][0]].target
		for _, frameIndex := range c.cellFrames[cellIndex][1:] {
			if c.frames[frameIndex].target != target {
				return fmt.Errorf("transformer: one formal operator has multiple Apply target keyspaces")
			}
		}
		targetBody := &c.program.bodies[target]
		source, err := newFormalCoordinateInventoryFold(targetBody.productDomain, c.keys[target], len(c.cellFrames[cellIndex]))
		if err != nil {
			return err
		}
		c.cellSourceFolds[cellIndex] = &source
	}
	return nil
}

func freezeFormalNormalTerminalConeCells(body *relationProgramBody, region *formalRelationRegionInventory) ([]formalRelationCell, error) {
	if body == nil || region == nil || body.variable == 0 || int(body.variable) > len(region.outcomes) {
		return nil, fmt.Errorf("transformer: formal normal-terminal coordinate cone is unowned")
	}
	queue := append([]formalRelationCell(nil), region.outcomes[body.variable-1]...)
	if len(queue) == 0 {
		queue = append(queue, region.roots[body.variable-1])
	}
	visited := make(map[formalRelationCell]struct{}, len(queue))
	var out []formalRelationCell
	for len(queue) != 0 {
		cell := queue[0]
		queue = queue[1:]
		if cell.Variable != body.variable {
			continue
		}
		if _, seen := visited[cell]; seen {
			continue
		}
		visited[cell] = struct{}{}
		out = append(out, cell)
		for _, influence := range region.incoming[cell] {
			if influence.Source.Variable == body.variable {
				queue = append(queue, influence.Source)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return formalRelationCellLess(out[i], out[j]) })
	return out, nil
}

func (c *formalCoordinateDependencyClosure) sealDependencies() {
	c.bodyNodeFirst = 0
	c.cellNodeFirst = len(c.bodies)
	c.selectorNodeFirst = c.cellNodeFirst + len(c.cells)
	c.frameNodeFirst = c.selectorNodeFirst + len(c.selectors)
	nodes := c.frameNodeFirst + len(c.frames)
	c.dependents = make([][]int, nodes)
	c.queued = make([]bool, nodes)
	add := func(from, to int) { c.dependents[from] = append(c.dependents[from], to) }
	for bodyIndex := range c.bodies {
		bodyNode := c.bodyNodeFirst + bodyIndex
		for _, cellIndex := range c.cellBody[bodyIndex] {
			cell := c.cells[cellIndex]
			if formalCoordinateFootprintDependsOnBodyInventory(c.program, cell) {
				add(bodyNode, c.cellNodeFirst+cellIndex)
			}
			add(c.cellNodeFirst+cellIndex, bodyNode)
			if _, selected := c.selectorMember[bodyIndex][cellIndex]; selected {
				add(c.cellNodeFirst+cellIndex, c.selectorNodeFirst+bodyIndex)
			}
		}
		for frameIndex := range c.frames {
			if c.frames[frameIndex].caller == bodyIndex {
				add(bodyNode, c.frameNodeFirst+frameIndex)
			}
			if c.frames[frameIndex].target == bodyIndex {
				add(c.selectorNodeFirst+bodyIndex, c.frameNodeFirst+frameIndex)
			}
		}
	}
	for frameIndex, frame := range c.frames {
		for _, cellIndex := range frame.cells {
			add(c.cellNodeFirst+cellIndex, c.frameNodeFirst+frameIndex)
			add(c.frameNodeFirst+frameIndex, c.cellNodeFirst+cellIndex)
		}
		if c.region != nil && frame.target >= 0 && frame.target < len(c.region.outcomes) {
			if frame.target < len(c.region.roots) {
				root := c.region.roots[frame.target]
				if rootIndex, ok := c.cellIndex[root]; ok {
					add(c.frameNodeFirst+frameIndex, c.cellNodeFirst+rootIndex)
				}
			}
			for _, outcome := range c.region.outcomes[frame.target] {
				if outcomeIndex, ok := c.cellIndex[outcome]; ok {
					add(c.cellNodeFirst+outcomeIndex, c.frameNodeFirst+frameIndex)
				}
			}
		}
	}
	// Identity producer support is evaluated on the same sealed relation graph
	// as coordinate declarations. These edges are not a second solver: they
	// merely make the existing cell nodes observe their exact reaching cells.
	var identityIncoming map[formalRelationCell][]formalRelationInfluence
	if c.region != nil {
		identityIncoming = c.region.incoming
	}
	for target, incoming := range identityIncoming {
		targetIndex, targetOK := c.cellIndex[target]
		if !targetOK {
			continue
		}
		for _, influence := range incoming {
			if influence.Source.Variable != target.Variable || influence.Kind == formalRelationInfluenceStepPublishedRead || influence.Kind == formalRelationInfluenceClosureDefinition {
				continue
			}
			if sourceIndex, sourceOK := c.cellIndex[influence.Source]; sourceOK {
				add(c.cellNodeFirst+sourceIndex, c.cellNodeFirst+targetIndex)
			}
		}
	}
	for index := range c.dependents {
		sort.Ints(c.dependents[index])
		out := c.dependents[index][:0]
		for _, dependent := range c.dependents[index] {
			if len(out) == 0 || out[len(out)-1] != dependent {
				out = append(out, dependent)
			}
		}
		c.dependents[index] = out
	}
	for node := 0; node < nodes; node++ {
		c.enqueue(node)
	}
}

func formalCoordinateFootprintDependsOnBodyInventory(program *RelationProgram, cell formalRelationCell) bool {
	if program == nil || cell.Variable == 0 || int(cell.Variable) > len(program.bodies) {
		return false
	}
	if cell.Kind == formalRelationCellOutcome {
		// N5 reads the existing registered identity graph. Heap family keys are
		// not derivable from source terms alone, so the frozen body inventory is
		// a semantic input even when N6 is empty.
		return true
	}
	if cell.Kind != formalRelationCellStep {
		return false
	}
	body := &program.bodies[cell.Variable-1]
	if cell.Root == 0 || int(cell.Root) >= len(body.relation.code.nodes) || cell.Step == 0 || int(cell.Step) > len(body.relation.code.nodes[cell.Root].steps) {
		return false
	}
	switch body.relation.code.nodes[cell.Root].steps[cell.Step-1].kind {
	case boundaryStepPresenceImplications, boundaryStepRootAssignment, boundaryStepCovariantExposure:
		return true
	default:
		return false
	}
}

func (c *formalCoordinateDependencyClosure) enqueue(node int) {
	if node < 0 || node >= len(c.queued) || c.queued[node] {
		return
	}
	c.queued[node] = true
	c.queue = append(c.queue, node)
}

func (c *formalCoordinateDependencyClosure) solve() error {
	for c.head < len(c.queue) {
		node := c.queue[c.head]
		c.head++
		c.queued[node] = false
		changed, err := c.evaluate(node)
		if err != nil {
			return err
		}
		if changed {
			for _, dependent := range c.dependents[node] {
				c.enqueue(dependent)
			}
		}
	}
	return nil
}

func (c *formalCoordinateDependencyClosure) evaluate(node int) (bool, error) {
	switch {
	case node < c.cellNodeFirst:
		return c.evaluateBody(node - c.bodyNodeFirst)
	case node < c.selectorNodeFirst:
		return c.evaluateCell(node - c.cellNodeFirst)
	case node < c.frameNodeFirst:
		return c.evaluateSelector(node - c.selectorNodeFirst)
	default:
		return c.evaluateFrame(node - c.frameNodeFirst)
	}
}

func (c *formalCoordinateDependencyClosure) evaluateBody(index int) (bool, error) {
	body := &c.program.bodies[index]
	fold := &c.bodyFolds[index]
	if err := fold.set(0, c.bodySeeds[index]); err != nil {
		return false, err
	}
	for contribution, cell := range c.cellBody[index] {
		if err := fold.set(contribution+1, fold.contribution(c.cellValue[cell].inventory)); err != nil {
			return false, err
		}
	}
	next := fold.root()
	var err error
	next, err = body.productDomain.CloseCoordinateFactorInventory(c.keys[index], next)
	if err != nil {
		return false, err
	}
	equal, err := formalCoordinateInventoriesEqual(body.productDomain, c.bodies[index], next)
	if err == nil && !equal {
		c.bodies[index] = next
	}
	return !equal, err
}

func (c *formalCoordinateDependencyClosure) evaluateCell(index int) (bool, error) {
	cell := c.cells[index]
	bodyIndex := int(cell.Variable - 1)
	body := &c.program.bodies[bodyIndex]
	identityInput, err := c.identityCellInput(index)
	if err != nil {
		return false, err
	}
	nextIdentity, err := c.transferIdentityCell(index, identityInput)
	if err != nil {
		return false, err
	}
	identityChanged := !c.formalIdentityEnvironmentEqual(c.cellInputIdentity[index], identityInput) ||
		!c.formalIdentityEnvironmentEqual(c.cellIdentity[index], nextIdentity)
	if identityChanged {
		c.cellInputIdentity[index] = identityInput
		outputChanged := !c.formalIdentityEnvironmentEqual(c.cellIdentity[index], nextIdentity)
		c.cellIdentity[index] = nextIdentity
		if outputChanged {
			for _, contribution := range c.cellIdentityContributions[index] {
				if _, err := c.cellIdentityFolds[contribution.target].set(contribution.id, nextIdentity); err != nil {
					return false, err
				}
			}
		}
	}
	base, err := c.freezeCellBase(index, cell, c.bodies[bodyIndex], nextIdentity)
	if err != nil {
		return false, err
	}
	fold := &c.cellFolds[index]
	if err := fold.set(0, base); err != nil {
		return false, err
	}
	sourceFold := c.cellSourceFolds[index]
	for contribution, frameIndex := range c.cellFrames[index] {
		frame := &c.frames[frameIndex]
		if err := fold.set(contribution+1, fold.contribution(frame.image)); err != nil {
			return false, err
		}
		// Resource owns only the caller image. Apply and Definition retain the
		// exact target selector used by their boundary execution.
		if sourceFold != nil {
			if err := sourceFold.set(contribution, sourceFold.contribution(frame.selector)); err != nil {
				return false, err
			}
		}
	}
	next := fold.root()
	next, err = body.productDomain.CloseCoordinateFactorInventory(c.keys[bodyIndex], next)
	if err != nil {
		return false, err
	}
	var source state.CoordinateFactorInventory
	if sourceFold != nil {
		target := c.frames[c.cellFrames[index][0]].target
		targetBody := &c.program.bodies[target]
		source = sourceFold.root()
		source, err = targetBody.productDomain.CloseCoordinateFactorInventory(c.keys[target], source)
		if err != nil {
			return false, err
		}
	}
	prior := c.cellValue[index]
	inventoryEqual := prior.inventory.KeySpace() != nil
	if inventoryEqual {
		inventoryEqual, err = formalCoordinateInventoriesEqual(body.productDomain, prior.inventory, next)
		if err != nil {
			return false, err
		}
	}
	sourceEqual := prior.source.KeySpace() == nil && source.KeySpace() == nil
	if prior.source.KeySpace() != nil && source.KeySpace() != nil {
		target := c.frames[c.cellFrames[index][0]].target
		sourceEqual, err = formalCoordinateInventoriesEqual(c.program.bodies[target].productDomain, prior.source, source)
		if err != nil {
			return false, err
		}
	}
	if inventoryEqual && sourceEqual {
		return identityChanged, nil
	}
	c.cellValue[index] = formalOperatorCoordinateFootprint{cell: cell, inventory: next, source: source}
	return true, nil
}

func (c *formalCoordinateDependencyClosure) freezeCellBase(index int, cell formalRelationCell, current state.CoordinateFactorInventory, identities formalIdentityEnvironment) (state.CoordinateFactorInventory, error) {
	body := &c.program.bodies[cell.Variable-1]
	keys := c.keys[cell.Variable-1]
	switch cell.Kind {
	case formalRelationCellStep:
		node := body.relation.code.nodes[cell.Root]
		step := node.steps[cell.Step-1]
		var normalReturnIdentities formalIdentitySupport
		if step.kind == boundaryStepExternalCall {
			var identityErr error
			step.operands.each(func(value ValueTerm) bool {
				var support formalIdentitySupport
				support, identityErr = c.identityValueSupport(int(cell.Variable-1), identities, value, make(map[ValueTerm]bool))
				if identityErr != nil {
					return false
				}
				normalReturnIdentities = unionFormalIdentitySupport(normalReturnIdentities, support)
				return true
			})
			if identityErr != nil {
				return state.CoordinateFactorInventory{}, identityErr
			}
		}
		return freezeFormalStepCoordinateFootprint(c.forest, body, cell.Variable, keys, c.rekeys[cell.Variable-1], c.pointwise[cell.Variable-1], current, normalReturnIdentities, step)
	case formalRelationCellOutcome:
		outcome := body.relation.code.outcomes[cell.Outcome]
		identityTerms, err := c.identityValuesSupport(int(cell.Variable-1), c.cellIdentity[index], outcome.returnTransaction.sources)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		return freezeFormalOutcomeCoordinateFootprint(body, keys, c.rekeys[cell.Variable-1], cell.Outcome, current, identityTerms)
	default:
		return body.productDomain.SealCoordinateFactorInventory(keys, nil)
	}
}

func (c *formalCoordinateDependencyClosure) evaluateSelector(index int) (bool, error) {
	body := &c.program.bodies[index]
	fold := &c.selectorFolds[index]
	for contribution, cell := range c.selectorCells[index] {
		if err := fold.set(contribution, fold.contribution(c.cellValue[cell].inventory)); err != nil {
			return false, err
		}
	}
	next := fold.root()
	var err error
	next, err = body.productDomain.CloseCoordinateFactorInventory(c.keys[index], next)
	if err != nil {
		return false, err
	}
	equal, err := formalCoordinateInventoriesEqual(body.productDomain, c.selectors[index], next)
	if err == nil && !equal {
		c.selectors[index] = next
	}
	return !equal, err
}

func (c *formalCoordinateDependencyClosure) evaluateFrame(index int) (bool, error) {
	frame := &c.frames[index]
	caller, target := &c.program.bodies[frame.caller], &c.program.bodies[frame.target]
	selector := c.selectors[frame.target]
	identityImage, resultSupport, identityChanged, err := c.evaluateFrameIdentity(index, selector)
	if err != nil {
		return false, err
	}
	frame.identityImage, frame.resultSupport = identityImage, resultSupport
	if len(frame.wirePlans) != 0 {
		selector, err = target.productDomain.ProjectCoordinateFactorInventoryFormalBoundary(selector, frame.wirePlans...)
		if err != nil {
			return false, err
		}
	}
	image := frame.image
	if image.KeySpace() == nil {
		image = frame.mapped
	}
	if len(frame.sourceRoots) != 0 {
		nextPlan, added, footprintErr := frame.footprint.AdvanceWithIdentityImage(selector, c.bodies[frame.caller], identityImage)
		if footprintErr != nil {
			return false, footprintErr
		}
		// Advance may consume a premise without immediately proving an output.
		// Retain that progress even when added is empty.
		frame.footprint = nextPlan
		image, err = caller.productDomain.UnionCoordinateFactorInventories(c.keys[frame.caller], image, added)
		if err != nil {
			return false, err
		}
	}
	image, err = caller.productDomain.CloseCoordinateFactorInventory(c.keys[frame.caller], image)
	if err != nil {
		return false, err
	}
	imageEqual := frame.image.KeySpace() != nil
	if imageEqual {
		imageEqual, err = formalCoordinateInventoriesEqual(caller.productDomain, frame.image, image)
		if err != nil {
			return false, err
		}
	}
	selectorEqual := frame.selector.KeySpace() != nil
	if selectorEqual {
		selectorEqual, err = formalCoordinateInventoriesEqual(target.productDomain, frame.selector, selector)
		if err != nil {
			return false, err
		}
	}
	if imageEqual && selectorEqual {
		return identityChanged, nil
	}
	frame.image, frame.selector = image, selector
	return true, nil
}

func (c *formalCoordinateDependencyClosure) publish() error {
	selectorCatalog, err := freezeFormalApplyCoordinateSelectorCatalog(c)
	if err != nil {
		return err
	}
	declarations := newFormalOperatorCoordinateFootprints()
	for index, value := range c.cellValue {
		cell := c.cells[index]
		body := &c.program.bodies[cell.Variable-1]
		if value.inventory.KeySpace() == nil {
			return fmt.Errorf("transformer: formal coordinate worklist left an operator undeclared")
		}
		if err := declarations.declare(body, cell, value.inventory); err != nil {
			return err
		}
		published := declarations.byCell[cell]
		published.source = value.source
		published.sourceSelector = selectorCatalog.byCell[cell]
		if value.source.KeySpace() != nil && !published.sourceSelector.valid() {
			return fmt.Errorf("transformer: Apply source selector has no frozen catalog reference")
		}
		declarations.byCell[cell] = published
	}
	c.forest.operatorFootprints = declarations
	c.forest.applySelectors = selectorCatalog
	if c.forest.applyCoordinateTrace != nil {
		for index := range c.frames {
			frame := &c.frames[index]
			owner := relationVar(frame.caller + 1)
			if !formalApplyTraceEnabled(owner, frame.frame.term) {
				continue
			}
			cells := make([]state.CoordinateFactorInventory, 0, len(frame.cells))
			for _, cell := range frame.cells {
				cells = append(cells, c.cellValue[cell].inventory)
			}
			c.forest.applyCoordinateTrace[formalFrameFootprintKey{variable: owner, frame: frame.frame.term}] = formalApplyCoordinateStaticTrace{
				image: frame.image, selector: frame.selector, cells: cells,
				sourceOwned: append([]state.CoordinateSlot(nil), frame.sourceOwned...),
				footprint:   frame.footprint.TraceSnapshot(),
			}
		}
	}
	return nil
}

func freezeFormalStaticApplyCoordinateFrames(c *formalCoordinateDependencyClosure) ([]formalStaticApplyCoordinateFrame, error) {
	operatorCells := freezeFormalFrameFootprintCells(c.forest)
	var out []formalStaticApplyCoordinateFrame
	for callerIndex := range c.program.bodies {
		caller := &c.program.bodies[callerIndex]
		for frameIndex := range caller.frames {
			frame := &caller.frames[frameIndex]
			if !frame.valid() || frame.owner != caller.variable || frame.target == 0 || int(frame.target) > len(c.program.bodies) {
				continue
			}
			targetIndex := int(frame.target - 1)
			prepared, err := freezeFormalStaticApplyCoordinateFrame(c, callerIndex, targetIndex, frame, operatorCells[formalFrameFootprintKey{variable: caller.variable, frame: frame.term}])
			if err != nil {
				return nil, err
			}
			out = append(out, prepared)
		}
	}
	return out, nil
}

func freezeFormalStaticApplyCoordinateFrame(c *formalCoordinateDependencyClosure, callerIndex, targetIndex int, frame *linkedRelationFrame, cells []formalRelationCell) (formalStaticApplyCoordinateFrame, error) {
	caller, target := &c.program.bodies[callerIndex], &c.program.bodies[targetIndex]
	callerKeys, targetKeys := c.keys[callerIndex], c.keys[targetIndex]
	type coordinateWire struct {
		source      keyspace.Key
		sourceSlot  statekey.Value
		target      formal.Root
		path        keyspace.Key
		destination int
		formal      bool
	}
	var wires []coordinateWire
	var scalarTargets []keyspace.Key
	for destinationIndex, destination := range frame.boundary.destinations {
		var formalPath keyspace.Key
		if destination.path.Kind != keyspace.KindInvalid {
			var err error
			formalPath, err = caller.productDomain.RekeyStructuralKeyFormal(c.rekeys[callerIndex], destination.path)
			if err != nil {
				return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology destination %d kind=%d: %w", destinationIndex, destination.kind, err)
			}
		}
		if formalPath.Kind != keyspace.KindInvalid && destination.slot != 0 {
			scalarTargets = append(scalarTargets, formalPath)
		}
		if destination.hasRoot {
			slot, ok := c.program.formalSlots.Slot(caller.body, destination.valueRoot)
			if !ok {
				return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology destination %d has no formal slot", destinationIndex)
			}
			root, ok := slot.Root()
			if !ok {
				return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology destination %d has no formal scalar root", destinationIndex)
			}
			rootPath, ok := callerKeys.InternFormalRoot(root)
			if !ok {
				return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology destination %d scalar root is foreign", destinationIndex)
			}
			scalarTargets = append(scalarTargets, rootPath)
		}
		for _, edge := range frame.boundary.edges {
			if edge.destination != destinationIndex {
				continue
			}
			sourcePath := edge.source.Path
			sourceAlreadyFormal := false
			if sourcePath.Kind == keyspace.KindInvalid && edge.root.Kind != 0 {
				slot, ok := c.program.formalSlots.Slot(target.body, edge.root)
				if !ok {
					return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology source has no formal slot")
				}
				root, ok := slot.Root()
				if !ok {
					return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology source has no formal root")
				}
				sourcePath, ok = targetKeys.InternFormalRoot(root)
				if !ok {
					return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology source is outside target keyspace")
				}
				sourceAlreadyFormal = true
			}
			wire := coordinateWire{sourceSlot: edge.source.Slot, path: formalPath, destination: destinationIndex}
			if sourcePath.Kind != keyspace.KindInvalid {
				if sourceAlreadyFormal {
					wire.source = sourcePath
				} else {
					var err error
					wire.source, err = target.productDomain.RekeyStructuralKeyFormal(c.rekeys[targetIndex], sourcePath)
					if err != nil {
						return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: Apply topology source kind=%d destination=%d: %w", edge.kind, destinationIndex, err)
					}
				}
			}
			if wire.source.Kind != keyspace.KindInvalid && formalPath.Kind != keyspace.KindInvalid {
				wire.target, wire.formal = callerKeys.DescribeFormalRoot(formalPath)
				if !wire.formal {
					return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: formal Apply coordinate destination has no formal root")
				}
			}
			wires = append(wires, wire)
		}
	}
	prepared := formalStaticApplyCoordinateFrame{caller: callerIndex, target: targetIndex, frame: frame}
	var mapped []state.CoordinateSlot
	for _, cell := range cells {
		index, ok := c.cellIndex[cell]
		if !ok {
			return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: formal Apply coordinate footprint has no operator")
		}
		prepared.cells = append(prepared.cells, index)
	}
	if len(prepared.cells) == 0 {
		return formalStaticApplyCoordinateFrame{}, fmt.Errorf("transformer: formal Apply coordinate footprint has no operator")
	}
	for _, scalarTarget := range scalarTargets {
		rootSlots, err := caller.productDomain.BoundaryRootCoordinateSlots(callerKeys, []keyspace.Key{scalarTarget})
		if err != nil {
			return formalStaticApplyCoordinateFrame{}, err
		}
		mapped = append(mapped, rootSlots...)
	}
	type sourceRoot struct {
		path keyspace.Key
		slot statekey.Value
	}
	sourceIndexes := make(map[sourceRoot]int, len(wires))
	for _, wire := range wires {
		if wire.source.Kind != keyspace.KindInvalid {
			sourceSlots, err := target.productDomain.BoundaryRootCoordinateSlots(targetKeys, []keyspace.Key{wire.source})
			if err != nil {
				return formalStaticApplyCoordinateFrame{}, err
			}
			prepared.sourceOwned = append(prepared.sourceOwned, sourceSlots...)
		}
		if wire.path.Kind != keyspace.KindInvalid {
			rootSlots, err := caller.productDomain.BoundaryRootCoordinateSlots(callerKeys, []keyspace.Key{wire.path})
			if err != nil {
				return formalStaticApplyCoordinateFrame{}, err
			}
			mapped = append(mapped, rootSlots...)
		}
		key := sourceRoot{path: wire.source, slot: wire.sourceSlot}
		from, exists := sourceIndexes[key]
		if !exists {
			from = len(prepared.sourceRoots)
			sourceIndexes[key] = from
			prepared.sourceRoots = append(prepared.sourceRoots, state.BoundaryFactorRoot{Slot: wire.sourceSlot, Path: wire.source})
		}
		prepared.rootMap = append(prepared.rootMap, state.BoundaryRootBinding{FromRoot: from, ToRoot: wire.destination, To: wire.path})
		if wire.formal {
			plan, err := target.productDomain.SealCoordinateFormalRootRekey(caller.body, targetKeys, callerKeys, []state.CoordinateFormalRootBinding{{Source: wire.source, Target: wire.target}})
			if err != nil {
				return formalStaticApplyCoordinateFrame{}, err
			}
			prepared.wirePlans = append(prepared.wirePlans, plan)
		}
	}
	sealedMapped, err := caller.productDomain.SealCoordinateFactorInventory(callerKeys, mapped)
	if err != nil {
		return formalStaticApplyCoordinateFrame{}, err
	}
	prepared.mapped = sealedMapped
	if len(prepared.sourceRoots) != 0 {
		prepared.footprint, err = caller.productDomain.PrepareBoundaryCoordinateFootprintPlan(
			target.productDomain,
			frame.allocations,
			callerKeys,
			prepared.rootMap,
			frame.existentials,
			prepared.sourceRoots,
		)
		if err != nil {
			return formalStaticApplyCoordinateFrame{}, err
		}
	}
	return prepared, nil
}
