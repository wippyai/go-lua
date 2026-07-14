package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/region"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

type relationRegionComponent struct {
	id           int
	refs         []CellRef
	dependencies []int
	cyclic       bool
}

// relationRegionStamp is the small lattice carrier. Relation vectors live in
// solve-local transactional storage, so WTO rounds neither allocate nor copy
// them through the generic engine.
type relationRegionStamp struct {
	component int
	revision  uint64
}

type relationRegionRuntime struct {
	current  []Relation
	next     []Relation
	revision uint64
	visits   int
}

type relationRegionProgram struct {
	ordered        []CellRef
	byRef          map[CellRef]RelationCell
	components     []relationRegionComponent
	componentCells []int
	plan           *solve.WTOPlan[int]
}

// solveRelationCellsRegion is the private region-backed executor for a closed
// lexical RelationCell equation set. The semantic SCC partition is computed
// once. Each SCC becomes one generic solver cell carrying only a revision
// stamp, while its relation vector remains in solve-local transactional
// storage. The condensation plan is built directly and never re-runs Tarjan.
//
// This remains an explicit migration seam: no program/query caller selects it
// and there is no fallback hidden inside it.
func solveRelationCellsRegion(ctx context.Context, cells []RelationCell, options RelationSolveOptions) (RelationSnapshot, error) {
	program, err := prepareRelationCellsRegion(cells)
	if err != nil {
		return RelationSnapshot{}, err
	}
	return program.solve(ctx, options)
}

func prepareRelationCellsRegion(cells []RelationCell) (*relationRegionProgram, error) {
	ordered, byRef, err := validateRelationCells(cells)
	if err != nil {
		return nil, err
	}
	program := &relationRegionProgram{ordered: ordered, byRef: byRef}
	if len(ordered) == 0 {
		return program, nil
	}
	cellPlan := relationDependencyPlan(ordered, byRef)
	program.components = buildRelationRegionComponents(cellPlan, byRef)
	program.componentCells = make([]int, len(program.components))
	elements := make([]solve.WTOElement[int], len(program.components))
	influences := make([]solve.WTOInfluence[int], 0, len(program.components))
	for index, component := range program.components {
		program.componentCells[index] = index
		elements[index].Vertex = index
		if component.cyclic {
			elements[index].Body = []solve.WTOElement[int]{}
			influences = append(influences, solve.WTOInfluence[int]{From: index, To: index})
		}
		for _, dependency := range component.dependencies {
			influences = append(influences, solve.WTOInfluence[int]{From: dependency, To: index})
		}
	}
	program.plan, err = solve.FreezeWTOPlan(program.componentCells, elements, influences)
	if err != nil {
		return nil, err
	}
	return program, nil
}

func (program *relationRegionProgram) solve(ctx context.Context, options RelationSolveOptions) (RelationSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	options = options.normalized()
	if err := ctx.Err(); err != nil {
		return RelationSnapshot{}, err
	}
	if len(program.ordered) == 0 {
		return RelationSnapshot{values: make(map[CellRef]Relation)}, nil
	}
	ordered, byRef := program.ordered, program.byRef
	components, componentCells, plan := program.components, program.componentCells, program.plan

	values := make(map[CellRef]Relation, len(ordered))
	runtimes := make([]relationRegionRuntime, len(components))
	cyclicRefs := 0
	for _, component := range components {
		if component.cyclic {
			cyclicRefs += len(component.refs)
		}
	}
	currentStorage := make([]Relation, cyclicRefs)
	nextStorage := make([]Relation, cyclicRefs)
	offset := 0
	for index, component := range components {
		runtime := relationRegionRuntime{}
		if component.cyclic {
			end := offset + len(component.refs)
			runtime.current = currentStorage[offset:end]
			runtime.next = nextStorage[offset:end]
			offset = end
		}
		for relationIndex, ref := range component.refs {
			cell := byRef[ref]
			bottom := Relation{shape: cell.Shape, arena: cell.Arena}
			if component.cyclic {
				runtime.current[relationIndex] = bottom
			}
			values[ref] = bottom
		}
		runtimes[index] = runtime
	}

	var equationErr error
	result, err := region.RunPrepared(ctx, solve.EquationSystem[int, relationRegionStamp]{
		Lattice: relationRegionDomain(),
		Cells:   componentCells,
		InitialSparse: func(index int) (relationRegionStamp, bool) {
			return relationRegionStamp{component: components[index].id}, true
		},
		Evaluate: func(index int, read func(int) relationRegionStamp) relationRegionStamp {
			if equationErr != nil {
				return relationRegionStamp{component: components[index].id, revision: runtimes[index].revision}
			}
			runtime := &runtimes[index]
			component := components[index]
			// Declare every condensation influence to the generic executor. The
			// relation equations themselves read the dense solve-local value map.
			for _, dependency := range component.dependencies {
				read(dependency)
			}
			if !component.cyclic {
				ref := component.refs[0]
				previous := values[ref]
				relation, evaluateErr := evaluateRelationCell(ctx, byRef[ref], values)
				if evaluateErr != nil {
					equationErr = evaluateErr
					return relationRegionStamp{component: component.id, revision: runtime.revision}
				}
				relation = widenRelationCell(previous, relation, options.MaxRows)
				if !EqualRelation(previous, relation) {
					values[ref] = relation
					runtime.revision++
				}
				return relationRegionStamp{component: component.id, revision: runtime.revision}
			}

			failedReason := ""
			if runtime.visits >= options.MaxIterations {
				failedReason = "SCC iteration budget"
			} else {
				runtime.visits++
				for relationIndex, ref := range component.refs {
					relation, evaluateErr := evaluateRelationCell(ctx, byRef[ref], values)
					if evaluateErr != nil {
						equationErr = evaluateErr
						return relationRegionStamp{component: component.id, revision: runtime.revision}
					}
					// This is the one relation widening boundary. The region
					// lattice below carries stamps, never Relations.
					relation = widenRelationCell(runtime.current[relationIndex], relation, options.MaxRows)
					if relation.ContextualReason() != "" {
						failedReason = relation.ContextualReason()
						break
					}
					runtime.next[relationIndex] = relation
				}
			}
			if failedReason != "" {
				for relationIndex, ref := range component.refs {
					runtime.next[relationIndex] = contextualRelation(byRef[ref], failedReason)
				}
			}
			changed := false
			for relationIndex := range runtime.current {
				if !EqualRelation(runtime.current[relationIndex], runtime.next[relationIndex]) {
					changed = true
					break
				}
			}
			if changed {
				runtime.current, runtime.next = runtime.next, runtime.current
				runtime.revision++
				for relationIndex, ref := range component.refs {
					values[ref] = runtime.current[relationIndex]
				}
			}
			return relationRegionStamp{component: component.id, revision: runtime.revision}
		},
	}, plan, region.Options{})
	if err != nil {
		return RelationSnapshot{}, err
	}
	if equationErr != nil {
		return RelationSnapshot{}, equationErr
	}
	if err := ctx.Err(); err != nil {
		return RelationSnapshot{}, err
	}
	for index, component := range components {
		stable, ok := result.Values[index]
		if !ok || stable.component != component.id || stable.revision != runtimes[index].revision {
			return RelationSnapshot{}, fmt.Errorf("transformer: region omitted relation component %d", component.id)
		}
	}
	refs := append([]CellRef(nil), ordered...)
	return RelationSnapshot{refs: refs, values: values}, nil
}

func buildRelationRegionComponents(plan *solve.WTOPlan[CellRef], byRef map[CellRef]RelationCell) []relationRegionComponent {
	partition := relationComponentsFromWTO(plan.Elements())
	componentAt := make([]int, len(byRef))
	for index, refs := range partition {
		for _, ref := range refs {
			position, ok := plan.CanonicalIndex(ref)
			if !ok {
				panic("transformer: frozen relation plan lost a declared cell")
			}
			componentAt[position] = index
		}
	}
	dependencyCapacity := 0
	for _, cell := range byRef {
		dependencyCapacity += len(cell.Dependencies)
	}
	dependencyStorage := make([]int, 0, dependencyCapacity)
	components := make([]relationRegionComponent, len(partition))
	for index, refs := range partition {
		dependencyStart := len(dependencyStorage)
		cyclic := len(refs) > 1
		for _, ref := range refs {
			if hasRelationDependency(byRef[ref], ref) {
				cyclic = true
			}
			for _, dependency := range byRef[ref].Dependencies {
				position, ok := plan.CanonicalIndex(dependency)
				if !ok {
					panic("transformer: frozen relation plan lost a dependency")
				}
				dependencyComponent := componentAt[position]
				if dependencyComponent != index {
					seen := false
					for _, existing := range dependencyStorage[dependencyStart:] {
						if existing == dependencyComponent {
							seen = true
							break
						}
					}
					if !seen {
						dependencyStorage = append(dependencyStorage, dependencyComponent)
					}
				}
			}
		}
		// The WTO is dependency-first, so component indexes are already the
		// canonical order required by the frozen condensation plan.
		dependencies := dependencyStorage[dependencyStart:]
		for left := 1; left < len(dependencies); left++ {
			for right := left; right > 0 && dependencies[right] < dependencies[right-1]; right-- {
				dependencies[right], dependencies[right-1] = dependencies[right-1], dependencies[right]
			}
		}
		components[index] = relationRegionComponent{
			id: index + 1, refs: refs, dependencies: dependencies, cyclic: cyclic,
		}
	}
	return components
}

func relationRegionDomain() lattice.Lattice[relationRegionStamp] {
	equal := func(left, right relationRegionStamp) bool { return left == right }
	lessOrEq := func(left, right relationRegionStamp) bool {
		return left.component == 0 || left.component == right.component && left.revision <= right.revision
	}
	join := func(left, right relationRegionStamp) relationRegionStamp {
		if left.component == 0 {
			return right
		}
		if right.component == 0 || left.component != right.component || left.revision >= right.revision {
			return left
		}
		return right
	}
	return lattice.Lattice[relationRegionStamp]{
		Bottom:   func() relationRegionStamp { return relationRegionStamp{} },
		Equal:    equal,
		LessOrEq: lessOrEq,
		Join:     join,
	}
}
