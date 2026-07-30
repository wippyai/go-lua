package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalRelationCellKind is the closed structural vocabulary of the
// parametric relation equation system.  A cell is owned by lexical syntax;
// caller identity and concrete State are deliberately unrepresentable.
type formalRelationCellKind uint8

const (
	formalRelationCellInvalid formalRelationCellKind = iota
	formalRelationCellNode
	formalRelationCellStep
	formalRelationCellOutcome
	formalRelationCellNonreturning
	formalRelationCellDefinition
	formalRelationCellResource
)

type formalRelationDefinitionRef uint32
type formalRelationResourceRef uint32

// formalRelationCell is the complete static identity of one equation cell.
// For a node, Root is nonzero and Step/Outcome are zero.  For a step, Root and
// Step are nonzero.  For an outcome, only Outcome is nonzero.
type formalRelationCell struct {
	Variable   relationVar
	Root       relationRootRef
	Step       uint32
	Outcome    boundaryOutcomeRef
	Definition formalRelationDefinitionRef
	Resource   formalRelationResourceRef
	Kind       formalRelationCellKind
}

func (c formalRelationCell) valid() bool {
	if c.Variable == 0 {
		return false
	}
	switch c.Kind {
	case formalRelationCellNode:
		return c.Root != 0 && c.Step == 0 && c.Outcome == 0 && c.Definition == 0 && c.Resource == 0
	case formalRelationCellStep:
		return c.Root != 0 && c.Step != 0 && c.Outcome == 0 && c.Definition == 0 && c.Resource == 0
	case formalRelationCellOutcome:
		return c.Root == 0 && c.Step == 0 && c.Outcome != 0 && c.Definition == 0 && c.Resource == 0
	case formalRelationCellNonreturning:
		return c.Root == 0 && c.Step == 0 && c.Outcome == 0 && c.Definition == 0 && c.Resource == 0
	case formalRelationCellDefinition:
		return c.Root == 0 && c.Step == 0 && c.Outcome == 0 && c.Definition != 0 && c.Resource == 0
	case formalRelationCellResource:
		return c.Root == 0 && c.Step == 0 && c.Outcome == 0 && c.Definition == 0 && c.Resource != 0
	default:
		return false
	}
}

type formalRelationInfluenceKind uint8

const (
	formalRelationInfluenceInvalid formalRelationInfluenceKind = iota
	formalRelationInfluenceFlow
	formalRelationInfluenceChoiceTrue
	formalRelationInfluenceChoiceFalse
	formalRelationInfluenceLoopFeedback
	formalRelationInfluenceLoopExit
	formalRelationInfluenceCalleeOutcome
	formalRelationInfluenceLocalNonreturning
	formalRelationInfluenceApplyNonreturningPredecessor
	formalRelationInfluenceCalleeNonreturning
	formalRelationInfluenceDefinitionSeed
	formalRelationInfluenceDefinitionOutcome
	formalRelationInfluenceResourceSeed
	formalRelationInfluenceResourceFeedback
	formalRelationInfluenceStepNodeEntry
	formalRelationInfluenceStepPublishedRead
	formalRelationInfluenceClosureDefinition
)

// formalRelationInfluence is an immutable semantic dependency.  Distinct
// guarded contributions may share Source and Target, so influences are not
// deduplicated even though the WTO successor graph is.
type formalRelationInfluence struct {
	Source formalRelationCell
	Target formalRelationCell
	Kind   formalRelationInfluenceKind
	// ReadPoint is present only on StepPublishedRead. It preserves the exact
	// Read(point) bucket when one equation source is published at multiple CFG
	// points; execution must never reconstruct that grouping from syntax.
	ReadPoint cfg.Point
	// Site pairs the predecessor and callee-nonreturning inputs of one exact
	// Apply occurrence. It is metadata, not another equation dependency.
	Site formalRelationCell
}

type formalRelationDefinition struct {
	owner, target relationVar
	point         cfg.Point
	frame         callFrameTerm
	external      bool
	cell          formalRelationCell
}

type formalRelationResource struct {
	owner   relationVar
	members []formalRelationDefinitionRef
	cell    formalRelationCell
}

type formalRelationRegionInventory struct {
	cells        []formalRelationCell
	incoming     map[formalRelationCell][]formalRelationInfluence
	successors   map[formalRelationCell][]formalRelationCell
	roots        []formalRelationCell
	outcomes     [][]formalRelationCell
	nonreturning []formalRelationCell
	definitions  []formalRelationDefinition
	resources    []formalRelationResource
	stepInputs   map[formalRelationCell]formalRelationStepDependencyContract
	widen        map[formalRelationCell]bool
	// representative and stepSegments are sealed when the lexical graph is
	// quotiented after coordinate/identity footprint closure. At that point
	// cells/incoming/successors become the sole solver graph; the full lexical
	// graph is discarded rather than retained as a peer authority.
	representative map[formalRelationCell]formalRelationCell
	stepSegments   map[formalRelationCell][]formalRelationStepSegment
	quotiented     bool
	plan           *solve.WTOPlan[formalRelationCell]
}

// formalRelationStepSegment is the minimal lexical metadata needed to execute
// one absorbed Step after the full pre-quotient graph has been discarded.
// Inputs exclude the segment's internal Flow edge; every source is already
// rewritten to its retained representative.
type formalRelationStepSegment struct {
	cell formalRelationCell
}

type formalRelationLoopTarget struct {
	body  relationRootRef
	exits []relationRootRef
}

type formalRelationRecursiveCallEdge struct {
	caller, target relationVar
	site           formalRelationCell
}

// freezeFormalRelationRegionInventory builds the complete equation universe
// once, after relationCode sealing.  Evaluation is forbidden from discovering
// cells, call targets, loop routes, or WTO edges.
func freezeFormalRelationRegionInventory(program *RelationProgram) (*formalRelationRegionInventory, error) {
	if program == nil || len(program.bodies) == 0 {
		return nil, fmt.Errorf("transformer: formal relation inventory is unowned")
	}
	inventory := &formalRelationRegionInventory{
		incoming:       make(map[formalRelationCell][]formalRelationInfluence),
		successors:     make(map[formalRelationCell][]formalRelationCell),
		stepInputs:     make(map[formalRelationCell]formalRelationStepDependencyContract),
		roots:          make([]formalRelationCell, len(program.bodies)),
		outcomes:       make([][]formalRelationCell, len(program.bodies)),
		nonreturning:   make([]formalRelationCell, len(program.bodies)),
		widen:          make(map[formalRelationCell]bool),
		representative: make(map[formalRelationCell]formalRelationCell),
		stepSegments:   make(map[formalRelationCell][]formalRelationStepSegment),
	}
	declared := make(map[formalRelationCell]struct{})
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		body := &program.bodies[bodyIndex]
		code := body.relation.code
		if body.variable != variable || code == nil || !code.sealed || code.root == 0 || int(code.root) >= len(code.nodes) {
			return nil, fmt.Errorf("transformer: formal relation %d has no sealed lexical code", variable)
		}
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			nodeCell := formalRelationCell{Variable: variable, Root: root, Kind: formalRelationCellNode}
			if err := inventory.declare(nodeCell, declared); err != nil {
				return nil, err
			}
			for stepIndex := range code.nodes[root].steps {
				stepCell := formalRelationCell{Variable: variable, Root: root, Step: uint32(stepIndex + 1), Kind: formalRelationCellStep}
				if err := inventory.declare(stepCell, declared); err != nil {
					return nil, err
				}
			}
		}
		inventory.roots[bodyIndex] = formalRelationCell{Variable: variable, Root: code.root, Kind: formalRelationCellNode}
		nonreturning := formalRelationCell{Variable: variable, Kind: formalRelationCellNonreturning}
		if err := inventory.declare(nonreturning, declared); err != nil {
			return nil, err
		}
		inventory.nonreturning[bodyIndex] = nonreturning
		for outcome := boundaryOutcomeRef(1); int(outcome) < len(code.outcomes); outcome++ {
			cell := formalRelationCell{Variable: variable, Outcome: outcome, Kind: formalRelationCellOutcome}
			if err := inventory.declare(cell, declared); err != nil {
				return nil, err
			}
			inventory.outcomes[bodyIndex] = append(inventory.outcomes[bodyIndex], cell)
		}
	}
	if err := inventory.declareDefinitions(program, declared); err != nil {
		return nil, err
	}

	for bodyIndex := range program.bodies {
		if err := inventory.linkBody(program, relationVar(bodyIndex+1), declared); err != nil {
			return nil, err
		}
	}
	if err := inventory.markRecursiveCallWidenHeads(program); err != nil {
		return nil, err
	}
	if err := inventory.linkDefinitions(program, declared); err != nil {
		return nil, err
	}
	if err := inventory.linkClosureDefinitions(program, declared); err != nil {
		return nil, err
	}
	if err := inventory.validateStepDependencyContracts(); err != nil {
		return nil, err
	}
	for cell := range inventory.successors {
		sort.Slice(inventory.successors[cell], func(i, j int) bool {
			return formalRelationCellLess(inventory.successors[cell][i], inventory.successors[cell][j])
		})
	}
	inventory.plan = solve.NewWTOPlan(inventory.cells, func(cell formalRelationCell) []formalRelationCell {
		return inventory.successors[cell]
	})
	if inventory.plan == nil || !inventory.plan.Matches(inventory.cells) {
		return nil, fmt.Errorf("transformer: formal relation WTO does not cover its sealed cell inventory")
	}
	if err := inventory.validateTypedWidenHeads(); err != nil {
		return nil, err
	}
	return inventory, nil
}

func (i *formalRelationRegionInventory) declare(cell formalRelationCell, declared map[formalRelationCell]struct{}) error {
	if !cell.valid() {
		return fmt.Errorf("transformer: malformed formal relation cell")
	}
	if _, duplicate := declared[cell]; duplicate {
		return fmt.Errorf("transformer: duplicate formal relation cell %+v", cell)
	}
	declared[cell] = struct{}{}
	i.cells = append(i.cells, cell)
	return nil
}

func (i *formalRelationRegionInventory) declareDefinitions(program *RelationProgram, declared map[formalRelationCell]struct{}) error {
	topology, err := freezeFormalDefinitionResourceTopology(program)
	if err != nil {
		return err
	}
	i.definitions = append([]formalRelationDefinition(nil), topology.definitions...)
	i.resources = append([]formalRelationResource(nil), topology.resources...)
	for ref := formalRelationDefinitionRef(1); int(ref) < len(i.definitions); ref++ {
		if err := i.declare(i.definitions[ref].cell, declared); err != nil {
			return err
		}
	}
	for ref := formalRelationResourceRef(1); int(ref) < len(i.resources); ref++ {
		cell := i.resources[ref].cell
		if err := i.declare(cell, declared); err != nil {
			return err
		}
		i.widen[cell] = true
	}
	return nil
}

func (i *formalRelationRegionInventory) linkDefinitions(program *RelationProgram, declared map[formalRelationCell]struct{}) error {
	resourceByOwner := make(map[relationVar]formalRelationResourceRef, len(i.resources)-1)
	for ref := formalRelationResourceRef(1); int(ref) < len(i.resources); ref++ {
		resource := i.resources[ref]
		resourceByOwner[resource.owner] = ref
		seeded := false
		for _, outcome := range i.outcomes[resource.owner-1] {
			if err := i.addInfluence(formalRelationInfluence{Source: outcome, Target: resource.cell, Kind: formalRelationInfluenceResourceSeed}, declared); err != nil {
				return err
			}
			seeded = true
		}
		if !seeded {
			return fmt.Errorf("transformer: formal relation resource owner %d has no normal outcome", resource.owner)
		}
	}
	for ref := formalRelationDefinitionRef(1); int(ref) < len(i.definitions); ref++ {
		definition := i.definitions[ref]
		if definition.external {
			resourceRef := resourceByOwner[definition.owner]
			if resourceRef == 0 || int(resourceRef) >= len(i.resources) {
				return fmt.Errorf("transformer: formal relation definition %d has no lexical resource world", ref)
			}
			resource := i.resources[resourceRef]
			if err := i.addInfluence(formalRelationInfluence{Source: resource.cell, Target: definition.cell, Kind: formalRelationInfluenceDefinitionSeed}, declared); err != nil {
				return err
			}
		} else {
			code := program.bodies[definition.owner-1].relation.code
			seeded := false
			for _, publication := range code.publication.points {
				if publication.point != definition.point || publication.ref == 0 {
					continue
				}
				source := formalRelationCell{Variable: definition.owner, Root: publication.ref, Kind: formalRelationCellNode}
				if err := i.addInfluence(formalRelationInfluence{Source: source, Target: definition.cell, Kind: formalRelationInfluenceDefinitionSeed}, declared); err != nil {
					return err
				}
				seeded = true
			}
			if !seeded {
				return fmt.Errorf("transformer: formal relation definition %d owner point %d has no lexical publication", ref, definition.point)
			}
		}
		// A definition is an owner-owned instantiation artifact. The target body
		// retains its one symbolic root seed; each normal target transformer is
		// bound with the owner publication/resource inside this cell instead of
		// feeding a caller-specific value into the target root.
		for _, outcome := range i.outcomes[definition.target-1] {
			if err := i.addInfluence(formalRelationInfluence{Source: outcome, Target: definition.cell, Kind: formalRelationInfluenceDefinitionOutcome}, declared); err != nil {
				return err
			}
		}
		if definition.external {
			resource := i.resources[resourceByOwner[definition.owner]]
			if err := i.addInfluence(formalRelationInfluence{Source: definition.cell, Target: resource.cell, Kind: formalRelationInfluenceResourceFeedback}, declared); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *formalRelationRegionInventory) linkClosureDefinitions(program *RelationProgram, declared map[formalRelationCell]struct{}) error {
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		code := program.bodies[bodyIndex].relation.code
		if code == nil || code.terms == nil {
			continue
		}
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			for stepIndex, step := range code.nodes[root].steps {
				if step.kind != boundaryStepApply || step.apply.frame == 0 || int(step.apply.frame) >= len(code.terms.callFrames) {
					continue
				}
				frame := code.terms.callFrames[step.apply.frame]
				if frame.closureProducer == 0 {
					continue
				}
				var producer relationVar
				for candidateRoot := relationRootRef(1); int(candidateRoot) < len(code.nodes); candidateRoot++ {
					for _, candidate := range code.nodes[candidateRoot].steps {
						if candidate.kind != boundaryStepApply || candidate.apply.frame != frame.closureProducer {
							continue
						}
						if producer != 0 && producer != candidate.apply.variable {
							return fmt.Errorf("transformer: closure producer frame %d has multiple lexical targets", frame.closureProducer)
						}
						producer = candidate.apply.variable
					}
				}
				if producer == 0 {
					return fmt.Errorf("transformer: closure Apply frame %d has no lexical producer", step.apply.frame)
				}
				var selected formalRelationCell
				for resourceRef := formalRelationResourceRef(1); int(resourceRef) < len(i.resources); resourceRef++ {
					resource := i.resources[resourceRef]
					if resource.owner != producer {
						continue
					}
					for _, definitionRef := range resource.members {
						if i.definitions[definitionRef].target != step.apply.variable {
							continue
						}
						if selected.valid() {
							return fmt.Errorf("transformer: closure Apply frame %d has multiple lexical resource worlds", step.apply.frame)
						}
						selected = i.definitions[definitionRef].cell
					}
				}
				if !selected.valid() {
					return fmt.Errorf("transformer: closure Apply frame %d has no lexical resource world", step.apply.frame)
				}
				target := formalRelationCell{Variable: variable, Root: root, Step: uint32(stepIndex + 1), Kind: formalRelationCellStep}
				if err := i.addStepDependency(target, formalRelationInfluence{Source: selected, Target: target, Kind: formalRelationInfluenceClosureDefinition}, declared); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (i *formalRelationRegionInventory) linkBody(program *RelationProgram, variable relationVar, declared map[formalRelationCell]struct{}) error {
	code := program.bodies[variable-1].relation.code
	loops := make(map[loopMuTerm]formalRelationLoopTarget)
	for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
		node := code.nodes[root]
		if node.kind != relationNodeLoopMu {
			continue
		}
		if _, duplicate := loops[node.binder]; duplicate {
			return fmt.Errorf("transformer: formal relation %d loop binder %d has multiple entries", variable, node.binder)
		}
		bodyCell := formalRelationCell{Variable: variable, Root: node.body, Kind: formalRelationCellNode}
		if _, ok := declared[bodyCell]; !ok {
			return fmt.Errorf("transformer: formal relation %d loop binder %d has no declared body", variable, node.binder)
		}
		loops[node.binder] = formalRelationLoopTarget{body: node.body, exits: append([]relationRootRef(nil), node.exits...)}
		i.widen[bodyCell] = true
	}
	for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
		node := code.nodes[root]
		nodeCell := formalRelationCell{Variable: variable, Root: root, Kind: formalRelationCellNode}
		switch node.kind {
		case relationNodeBottom:
		case relationNodeNonreturning:
			if err := i.addInfluence(formalRelationInfluence{
				Source: nodeCell, Target: i.nonreturning[variable-1], Kind: formalRelationInfluenceLocalNonreturning,
			}, declared); err != nil {
				return err
			}
		case relationNodeSequence:
			previous := nodeCell
			for stepIndex, step := range node.steps {
				stepCell := formalRelationCell{Variable: variable, Root: root, Step: uint32(stepIndex + 1), Kind: formalRelationCellStep}
				if err := i.addStepDependency(stepCell, formalRelationInfluence{Source: previous, Target: stepCell, Kind: formalRelationInfluenceFlow}, declared); err != nil {
					return err
				}
				if err := i.linkStepExecutionDependencies(program, variable, nodeCell, stepCell, step, declared); err != nil {
					return err
				}
				if step.kind == boundaryStepApply {
					if step.apply.variable == 0 || int(step.apply.variable) > len(program.bodies) {
						return fmt.Errorf("transformer: formal relation %d Apply has foreign target %d", variable, step.apply.variable)
					}
					for _, outcome := range i.outcomes[step.apply.variable-1] {
						if err := i.addStepDependency(stepCell, formalRelationInfluence{Source: outcome, Target: stepCell, Kind: formalRelationInfluenceCalleeOutcome}, declared); err != nil {
							return err
						}
					}
					if err := i.addInfluence(formalRelationInfluence{
						Source: previous, Target: i.nonreturning[variable-1],
						Kind: formalRelationInfluenceApplyNonreturningPredecessor, Site: stepCell,
					}, declared); err != nil {
						return err
					}
					if err := i.addInfluence(formalRelationInfluence{
						Source: i.nonreturning[step.apply.variable-1], Target: i.nonreturning[variable-1],
						Kind: formalRelationInfluenceCalleeNonreturning, Site: stepCell,
					}, declared); err != nil {
						return err
					}
				}
				previous = stepCell
				if step.kind != boundaryStepLoopFeedback && step.kind != boundaryStepLoopExit {
					continue
				}
				loop, ok := loops[step.binder]
				if !ok {
					return fmt.Errorf("transformer: formal relation %d loop control %d has no canonical binder", variable, step.binder)
				}
				target, kind := loop.body, formalRelationInfluenceLoopFeedback
				if step.kind == boundaryStepLoopExit {
					if int(step.route) >= len(loop.exits) {
						return fmt.Errorf("transformer: formal relation %d loop exit %d is outside binder %d", variable, step.route, step.binder)
					}
					target, kind = loop.exits[step.route], formalRelationInfluenceLoopExit
				}
				if target != 0 {
					if err := i.addInfluence(formalRelationInfluence{Source: stepCell, Target: formalRelationCell{Variable: variable, Root: target, Kind: formalRelationCellNode}, Kind: kind}, declared); err != nil {
						return err
					}
				}
			}
			if node.next != 0 {
				if err := i.addInfluence(formalRelationInfluence{Source: previous, Target: formalRelationCell{Variable: variable, Root: node.next, Kind: formalRelationCellNode}, Kind: formalRelationInfluenceFlow}, declared); err != nil {
					return err
				}
			}
		case relationNodeOutcome:
			if err := i.addInfluence(formalRelationInfluence{Source: nodeCell, Target: formalRelationCell{Variable: variable, Outcome: node.outcome, Kind: formalRelationCellOutcome}, Kind: formalRelationInfluenceFlow}, declared); err != nil {
				return err
			}
		case relationNodeChoice:
			for _, branch := range []struct {
				root relationRootRef
				kind formalRelationInfluenceKind
			}{{node.whenTrue, formalRelationInfluenceChoiceTrue}, {node.whenFalse, formalRelationInfluenceChoiceFalse}} {
				if branch.root == 0 {
					continue
				}
				if err := i.addInfluence(formalRelationInfluence{Source: nodeCell, Target: formalRelationCell{Variable: variable, Root: branch.root, Kind: formalRelationCellNode}, Kind: branch.kind}, declared); err != nil {
					return err
				}
			}
		case relationNodeLoopMu, relationNodeLoopPortal:
			if err := i.addInfluence(formalRelationInfluence{Source: nodeCell, Target: formalRelationCell{Variable: variable, Root: node.body, Kind: formalRelationCellNode}, Kind: formalRelationInfluenceFlow}, declared); err != nil {
				return err
			}
		default:
			return fmt.Errorf("transformer: formal relation %d node %d has invalid syntax", variable, root)
		}
	}
	return nil
}

// markRecursiveCallWidenHeads derives the unique deterministic DFS backedges
// from the already-frozen lexical recursive-SCC authority. The exact Apply
// occurrence, rather than either function as a whole, owns widening.
func (i *formalRelationRegionInventory) markRecursiveCallWidenHeads(program *RelationProgram) error {
	componentOf := make([]uint32, len(program.bodies)+1)
	for componentIndex, members := range program.recursiveSCCs {
		if len(members) == 0 {
			return fmt.Errorf("transformer: formal recursive component %d is empty", componentIndex+1)
		}
		previous := relationVar(0)
		for _, member := range members {
			if member == 0 || int(member) > len(program.bodies) || member <= previous || componentOf[member] != 0 {
				return fmt.Errorf("transformer: formal recursive component %d has non-canonical member %d", componentIndex+1, member)
			}
			componentOf[member] = uint32(componentIndex + 1)
			previous = member
		}
	}

	adjacency := make([][]formalRelationRecursiveCallEdge, len(program.bodies)+1)
	for bodyIndex := range program.bodies {
		caller := relationVar(bodyIndex + 1)
		component := componentOf[caller]
		if component == 0 {
			continue
		}
		code := program.bodies[bodyIndex].relation.code
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			for stepIndex, step := range code.nodes[root].steps {
				if step.kind != boundaryStepApply || step.apply.variable == 0 || int(step.apply.variable) >= len(componentOf) || componentOf[step.apply.variable] != component {
					continue
				}
				adjacency[caller] = append(adjacency[caller], formalRelationRecursiveCallEdge{
					caller: caller,
					target: step.apply.variable,
					site:   formalRelationCell{Variable: caller, Root: root, Step: uint32(stepIndex + 1), Kind: formalRelationCellStep},
				})
			}
		}
	}

	color := make([]uint8, len(program.bodies)+1)
	type frame struct {
		variable relationVar
		next     int
	}
	for root := relationVar(1); int(root) <= len(program.bodies); root++ {
		if componentOf[root] == 0 || color[root] != 0 {
			continue
		}
		color[root] = 1
		stack := []frame{{variable: root}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next == len(adjacency[top.variable]) {
				color[top.variable] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			edge := adjacency[top.variable][top.next]
			top.next++
			switch color[edge.target] {
			case 0:
				color[edge.target] = 1
				stack = append(stack, frame{variable: edge.target})
			case 1:
				i.widen[edge.site] = true
			}
		}
	}
	return nil
}

func (i *formalRelationRegionInventory) validateTypedWidenHeads() error {
	var validate func([]solve.WTOElement[formalRelationCell]) error
	validate = func(elements []solve.WTOElement[formalRelationCell]) error {
		for _, element := range elements {
			if element.IsComponent() && !i.componentOwnsTypedWidenHead(element) {
				return fmt.Errorf("transformer: formal WTO component headed by %+v has no typed widening head", element.Vertex)
			}
			if err := validate(element.Body); err != nil {
				return err
			}
		}
		return nil
	}
	return validate(i.plan.Elements())
}

func (i *formalRelationRegionInventory) componentOwnsTypedWidenHead(component solve.WTOElement[formalRelationCell]) bool {
	cells := make(map[formalRelationCell]struct{})
	var collect func(solve.WTOElement[formalRelationCell])
	collect = func(element solve.WTOElement[formalRelationCell]) {
		cells[element.Vertex] = struct{}{}
		for _, nested := range element.Body {
			collect(nested)
		}
	}
	collect(component)
	for cell := range cells {
		if i.widen[cell] {
			return true
		}
	}
	// Recursive nonreturning propagation is a separate finite-height Boolean
	// SCC. Its typed owner is the selected Apply cut carried by Site metadata;
	// widening the Boolean cell itself would be meaningless.
	for target := range cells {
		if target.Kind != formalRelationCellNonreturning {
			continue
		}
		for _, influence := range i.incoming[target] {
			if influence.Kind != formalRelationInfluenceCalleeNonreturning || !i.widen[influence.Site] {
				continue
			}
			if _, internal := cells[influence.Source]; internal {
				return true
			}
		}
	}
	return false
}

func (i *formalRelationRegionInventory) addInfluence(influence formalRelationInfluence, declared map[formalRelationCell]struct{}) error {
	if influence.Kind == formalRelationInfluenceInvalid {
		return fmt.Errorf("transformer: formal relation influence has invalid semantics")
	}
	if _, ok := declared[influence.Source]; !ok {
		return fmt.Errorf("transformer: formal relation influence has undeclared source %+v", influence.Source)
	}
	if _, ok := declared[influence.Target]; !ok {
		return fmt.Errorf("transformer: formal relation influence has undeclared target %+v", influence.Target)
	}
	if influence.Kind == formalRelationInfluenceStepPublishedRead {
		if influence.Site.valid() {
			return fmt.Errorf("transformer: published-read influence has invalid point metadata")
		}
	} else if influence.ReadPoint != 0 {
		return fmt.Errorf("transformer: non-read influence invented point metadata")
	}
	if influence.Kind == formalRelationInfluenceCalleeNonreturning || influence.Kind == formalRelationInfluenceApplyNonreturningPredecessor {
		if _, ok := declared[influence.Site]; !ok || influence.Site.Kind != formalRelationCellStep {
			return fmt.Errorf("transformer: callee-nonreturning influence has no declared Apply site")
		}
	} else if influence.Site.valid() {
		return fmt.Errorf("transformer: non-Apply influence invented site metadata")
	}
	i.incoming[influence.Target] = append(i.incoming[influence.Target], influence)
	for _, target := range i.successors[influence.Source] {
		if target == influence.Target {
			return nil
		}
	}
	i.successors[influence.Source] = append(i.successors[influence.Source], influence.Target)
	return nil
}

// freezeObservableStepQuotient removes only fixed-point variables whose value
// is not observable outside one acyclic lexical Step chain. The semantic Steps
// themselves survive in stepSegments and are executed in their original order
// by the retained terminal equation.
//
// This runs after formal coordinate/identity footprints have been frozen from
// the complete lexical graph. On return cells/incoming/successors/plan are the
// one canonical solver graph; the pre-quotient edge graph is unreachable.
func (i *formalRelationRegionInventory) freezeObservableStepQuotient(program *RelationProgram) error {
	if i == nil || program == nil || i.quotiented || len(i.cells) == 0 || i.plan == nil {
		return fmt.Errorf("transformer: observable Step quotient is unowned or already sealed")
	}
	lexicalCells := append([]formalRelationCell(nil), i.cells...)
	outgoing := make(map[formalRelationCell][]formalRelationInfluence, len(lexicalCells))
	for _, row := range i.incoming {
		for _, influence := range row {
			outgoing[influence.Source] = append(outgoing[influence.Source], influence)
		}
	}

	retained := make(map[formalRelationCell]bool, len(lexicalCells))
	for _, cell := range lexicalCells {
		if cell.Kind != formalRelationCellStep {
			retained[cell] = true
		}
		if i.widen[cell] {
			retained[cell] = true
		}
		i.representative[cell] = cell
	}
	// Point/edge publication and nodeReads all name the same sealed output-cell
	// vocabulary. Retain every such Step even when it would otherwise be a
	// linear intermediate.
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		code := program.bodies[bodyIndex].relation.code
		if code == nil {
			return fmt.Errorf("transformer: observable Step quotient body %d has no relation code", variable)
		}
		markPublication := func(ref relationRootRef) {
			cell := formalPublicationOutputCell(program, variable, ref)
			if cell.Kind == formalRelationCellStep {
				retained[cell] = true
			}
			// Published-read dependencies use the point-local output rather than
			// terminal reduction. Preserve that identity as well.
			if local, dependency, valid := formalRelationPublishedOutputCell(variable, code, ref); valid && dependency && local.Kind == formalRelationCellStep {
				retained[local] = true
			}
		}
		for _, publication := range code.publication.points {
			markPublication(publication.ref)
		}
		for _, publication := range code.publication.edges {
			markPublication(publication.ref)
		}
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			node := code.nodes[root]
			if node.kind != relationNodeSequence || len(node.steps) == 0 {
				continue
			}
			last := formalRelationCell{Variable: variable, Root: root, Step: uint32(len(node.steps)), Kind: formalRelationCellStep}
			retained[last] = true
			for index, step := range node.steps {
				if step.kind == boundaryStepApply || step.kind == boundaryStepExternalCall ||
					step.kind == boundaryStepLoopFeedback || step.kind == boundaryStepLoopExit {
					retained[formalRelationCell{Variable: variable, Root: root, Step: uint32(index + 1), Kind: formalRelationCellStep}] = true
					// Call/control cutpoints own external observation and pairing.
					// Keep their immediate predecessor outside the same pipeline so
					// their operand row remains a self-contained named boundary.
					if index > 0 {
						retained[formalRelationCell{Variable: variable, Root: root, Step: uint32(index), Kind: formalRelationCellStep}] = true
					}
				}
			}
		}
	}

	// A non-terminal Step is absorbable exactly when nobody can distinguish its
	// result: its sole outgoing influence is immediate lexical Flow.
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		code := program.bodies[bodyIndex].relation.code
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			node := code.nodes[root]
			if node.kind != relationNodeSequence {
				continue
			}
			for index := len(node.steps) - 2; index >= 0; index-- {
				cell := formalRelationCell{Variable: variable, Root: root, Step: uint32(index + 1), Kind: formalRelationCellStep}
				next := formalRelationCell{Variable: variable, Root: root, Step: uint32(index + 2), Kind: formalRelationCellStep}
				row := outgoing[cell]
				if retained[cell] || len(row) != 1 || row[0].Kind != formalRelationInfluenceFlow || row[0].Target != next {
					retained[cell] = true
					continue
				}
				representative := i.representative[next]
				if !representative.valid() || !retained[representative] {
					return fmt.Errorf("transformer: observable Step quotient has no retained terminal for %+v", cell)
				}
				i.representative[cell] = representative
			}
		}
	}

	for _, cell := range lexicalCells {
		representative, present := i.representative[cell]
		if !present || !representative.valid() || !retained[representative] {
			return fmt.Errorf("transformer: lexical cell %+v has no unique observable representative", cell)
		}
		if cell.Kind == formalRelationCellStep {
			i.stepSegments[representative] = append(i.stepSegments[representative], formalRelationStepSegment{cell: cell})
		}
	}

	quotientIncoming := make(map[formalRelationCell][]formalRelationInfluence)
	quotientSuccessors := make(map[formalRelationCell][]formalRelationCell)
	for _, row := range i.incoming {
		for _, influence := range row {
			source := i.representative[influence.Source]
			target := i.representative[influence.Target]
			if source == target && influence.Kind == formalRelationInfluenceFlow {
				continue
			}
			influence.Source, influence.Target = source, target
			duplicate := false
			for _, prior := range quotientIncoming[target] {
				if prior == influence {
					duplicate = true
					break
				}
			}
			if !duplicate {
				quotientIncoming[target] = append(quotientIncoming[target], influence)
			}
			seenSuccessor := false
			for _, prior := range quotientSuccessors[source] {
				if prior == target {
					seenSuccessor = true
					break
				}
			}
			if !seenSuccessor {
				quotientSuccessors[source] = append(quotientSuccessors[source], target)
			}
		}
	}
	quotientCells := make([]formalRelationCell, 0, len(lexicalCells))
	for _, cell := range lexicalCells {
		if i.representative[cell] == cell {
			quotientCells = append(quotientCells, cell)
		}
	}
	for cell := range quotientSuccessors {
		sort.Slice(quotientSuccessors[cell], func(left, right int) bool {
			return formalRelationCellLess(quotientSuccessors[cell][left], quotientSuccessors[cell][right])
		})
	}
	i.cells, i.incoming, i.successors = quotientCells, quotientIncoming, quotientSuccessors
	i.plan = solve.NewWTOPlan(i.cells, func(cell formalRelationCell) []formalRelationCell { return i.successors[cell] })
	if i.plan == nil || !i.plan.Matches(i.cells) {
		return fmt.Errorf("transformer: observable Step quotient WTO does not cover retained cells")
	}
	i.quotiented = true
	return i.validateTypedWidenHeads()
}

func (i *formalRelationRegionInventory) representativeCell(cell formalRelationCell) (formalRelationCell, bool) {
	if i == nil || !i.quotiented {
		return formalRelationCell{}, false
	}
	representative, present := i.representative[cell]
	return representative, present && representative.valid()
}

func formalRelationCellLess(left, right formalRelationCell) bool {
	if left.Variable != right.Variable {
		return left.Variable < right.Variable
	}
	if left.Root != right.Root {
		return left.Root < right.Root
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Step != right.Step {
		return left.Step < right.Step
	}
	if left.Outcome != right.Outcome {
		return left.Outcome < right.Outcome
	}
	if left.Definition != right.Definition {
		return left.Definition < right.Definition
	}
	return left.Resource < right.Resource
}
