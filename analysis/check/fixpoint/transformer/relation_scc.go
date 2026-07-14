package transformer

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// RelationEquation computes one lexical cell from a read-only snapshot of
// its explicitly declared dependencies. The returned relation must retain the
// arena and boundary shape owned by the cell.
type RelationEquation func(context.Context, RelationView) (Relation, error)

// RelationCell is one equation in a lexical relation system. Dependencies are
// callee cells read by Equation; they form the complete static call graph.
// Arena and Shape are intentionally per-cell rather than program-global.
type RelationCell struct {
	Ref          CellRef
	Arena        *Arena
	Shape        Shape
	Dependencies []CellRef
	Equation     RelationEquation
}

// RelationSolveOptions bound cyclic growth. Exceeding either budget fails the
// complete SCC closed to contextual Top; rows are never independently cut.
type RelationSolveOptions struct {
	MaxRows       int
	MaxIterations int
}

func (o RelationSolveOptions) normalized() RelationSolveOptions {
	if o.MaxRows <= 0 {
		o.MaxRows = 4096
	}
	if o.MaxIterations <= 0 {
		o.MaxIterations = 256
	}
	return o
}

// RelationView is a transaction-local view. Lookup records undeclared reads,
// allowing the solver to reject a dynamic call edge even if Equation ignores
// the returned false value.
type RelationView struct {
	values    map[CellRef]Relation
	allowed   map[CellRef]struct{}
	violation *CellRef
	violated  *bool
}

func (v RelationView) Lookup(ref CellRef) (Relation, bool) {
	if _, ok := v.allowed[ref]; !ok {
		if v.violation != nil {
			*v.violation = ref
		}
		if v.violated != nil {
			*v.violated = true
		}
		return Relation{}, false
	}
	r, ok := v.values[ref]
	return r, ok
}

// RelationSnapshot is the only publication product. Its order is stable and
// its relations are immutable. A failed or canceled solve returns the zero
// snapshot, never a partially populated one.
type RelationSnapshot struct {
	refs   []CellRef
	values map[CellRef]Relation
}

type RelationSnapshotEntry struct {
	Ref      CellRef
	Relation Relation
}

func (s RelationSnapshot) Lookup(ref CellRef) (Relation, bool) {
	r, ok := s.values[ref]
	return r, ok
}

func (s RelationSnapshot) Entries() []RelationSnapshotEntry {
	entries := make([]RelationSnapshotEntry, 0, len(s.refs))
	for _, ref := range s.refs {
		entries = append(entries, RelationSnapshotEntry{Ref: ref, Relation: s.values[ref]})
	}
	return entries
}

// SolveRelationCells solves a lexical call graph into relation-valued cells.
// Dependency SCCs use synchronous rounds, making the result independent of
// input order. All state remains scratch until the final stable snapshot. A
// nil context is treated as context.Background.
func SolveRelationCells(ctx context.Context, cells []RelationCell, options RelationSolveOptions) (RelationSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	options = options.normalized()
	ordered, byRef, err := validateRelationCells(cells)
	if err != nil {
		return RelationSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return RelationSnapshot{}, err
	}
	values := make(map[CellRef]Relation, len(ordered))
	for _, ref := range ordered {
		cell := byRef[ref]
		values[ref] = Relation{shape: cell.Shape, arena: cell.Arena}
	}
	components := relationComponentsFromWTO(relationDependencyPlan(ordered, byRef).Elements())
	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return RelationSnapshot{}, err
		}
		cyclic := len(component) > 1 || hasRelationDependency(byRef[component[0]], component[0])
		if !cyclic {
			next, err := evaluateRelationCell(ctx, byRef[component[0]], values)
			if err != nil {
				return RelationSnapshot{}, err
			}
			values[component[0]] = widenRelationCell(values[component[0]], next, options.MaxRows)
			continue
		}
		converged := false
		for iteration := 0; iteration < options.MaxIterations; iteration++ {
			if err := ctx.Err(); err != nil {
				return RelationSnapshot{}, err
			}
			round := make(map[CellRef]Relation, len(component))
			changed := false
			failedReason := ""
			for _, ref := range component {
				next, err := evaluateRelationCell(ctx, byRef[ref], values)
				if err != nil {
					return RelationSnapshot{}, err
				}
				next = widenRelationCell(values[ref], next, options.MaxRows)
				if next.contextual != "" {
					failedReason = next.contextual
					break
				}
				round[ref] = next
				changed = changed || !EqualRelation(values[ref], next)
			}
			if err := ctx.Err(); err != nil {
				return RelationSnapshot{}, err
			}
			if failedReason != "" {
				// Every cell in a recursive component is one transaction. A
				// contextual member invalidates all mutually recursive results.
				for _, ref := range component {
					values[ref] = contextualRelation(byRef[ref], failedReason)
				}
				converged = true
				break
			}
			for _, ref := range component {
				values[ref] = round[ref]
			}
			if !changed {
				converged = true
				break
			}
		}
		if !converged {
			for _, ref := range component {
				cell := byRef[ref]
				values[ref] = contextualRelation(cell, "SCC iteration budget")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return RelationSnapshot{}, err
	}
	refs := append([]CellRef(nil), ordered...)
	return RelationSnapshot{refs: refs, values: values}, nil
}

func widenRelationCell(previous, next Relation, maxRows int) Relation {
	// Contextual is Top. Preserve the original fail-closed reason and owning
	// identity rather than asking the generic lattice join to synthesize a new
	// incompatibility reason.
	if previous.contextual != "" {
		return previous
	}
	if next.contextual != "" {
		return next
	}
	return WidenRelation(previous, next, maxRows)
}

func evaluateRelationCell(ctx context.Context, cell RelationCell, values map[CellRef]Relation) (Relation, error) {
	allowed := make(map[CellRef]struct{}, len(cell.Dependencies))
	for _, dep := range cell.Dependencies {
		allowed[dep] = struct{}{}
	}
	var violation CellRef
	violated := false
	view := RelationView{values: values, allowed: allowed, violation: &violation, violated: &violated}
	next, err := cell.Equation(ctx, view)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Relation{}, ctxErr
	}
	if violated {
		return Relation{}, fmt.Errorf("transformer: cell %v read undeclared dependency %v", cell.Ref, violation)
	}
	if err != nil {
		return contextualRelation(cell, "equation: "+err.Error()), nil
	}
	if next.arena != cell.Arena || next.shape != cell.Shape {
		return contextualRelation(cell, "foreign relation identity"), nil
	}
	return next, nil
}

func contextualRelation(cell RelationCell, reason string) Relation {
	return Relation{shape: cell.Shape, arena: cell.Arena, contextual: reason, widened: true}
}

func validateRelationCells(cells []RelationCell) ([]CellRef, map[CellRef]RelationCell, error) {
	byRef := make(map[CellRef]RelationCell, len(cells))
	for _, source := range cells {
		if source.Arena == nil || source.Arena.reg == nil {
			return nil, nil, fmt.Errorf("transformer: cell %v has no arena", source.Ref)
		}
		if source.Equation == nil {
			return nil, nil, fmt.Errorf("transformer: cell %v has no equation", source.Ref)
		}
		if _, duplicate := byRef[source.Ref]; duplicate {
			return nil, nil, fmt.Errorf("transformer: duplicate cell %v", source.Ref)
		}
		source.Dependencies = canonicalCellRefs(source.Dependencies)
		byRef[source.Ref] = source
	}
	ordered := make([]CellRef, 0, len(byRef))
	for ref := range byRef {
		ordered = append(ordered, ref)
	}
	sortCellRefs(ordered)
	for _, ref := range ordered {
		for _, dep := range byRef[ref].Dependencies {
			if _, ok := byRef[dep]; !ok {
				return nil, nil, fmt.Errorf("transformer: cell %v declares unknown dependency %v", ref, dep)
			}
		}
	}
	return ordered, byRef, nil
}

func canonicalCellRefs(refs []CellRef) []CellRef {
	out := append([]CellRef(nil), refs...)
	sortCellRefs(out)
	if len(out) == 0 {
		return nil
	}
	write := 1
	for _, ref := range out[1:] {
		if ref != out[write-1] {
			out[write] = ref
			write++
		}
	}
	return out[:write]
}

func sortCellRefs(refs []CellRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Function != refs[j].Function {
			return refs[i].Function < refs[j].Function
		}
		return refs[i].Slot < refs[j].Slot
	})
}

func hasRelationDependency(cell RelationCell, target CellRef) bool {
	i := sort.Search(len(cell.Dependencies), func(i int) bool {
		ref := cell.Dependencies[i]
		return ref.Function > target.Function || ref.Function == target.Function && ref.Slot >= target.Slot
	})
	return i < len(cell.Dependencies) && cell.Dependencies[i] == target
}

func relationDependencyPlan(ordered []CellRef, byRef map[CellRef]RelationCell) *solve.WTOPlan[CellRef] {
	successors := make(map[CellRef][]CellRef, len(ordered))
	for _, consumer := range ordered {
		for _, dependency := range byRef[consumer].Dependencies {
			successors[dependency] = append(successors[dependency], consumer)
		}
	}
	return solve.NewWTOPlan(ordered, func(ref CellRef) []CellRef { return successors[ref] })
}

// relationComponentsFromWTO projects the generic canonical WTO into the
// synchronous semantic SCC vectors used by relation evaluation. A top-level
// component contains its head and all nested WTO body vertices.
func relationComponentsFromWTO(elements []solve.WTOElement[CellRef]) [][]CellRef {
	cellCount := 0
	var count func([]solve.WTOElement[CellRef])
	count = func(items []solve.WTOElement[CellRef]) {
		cellCount += len(items)
		for _, item := range items {
			count(item.Body)
		}
	}
	count(elements)
	storage := make([]CellRef, 0, cellCount)
	components := make([][]CellRef, 0, len(elements))
	var collect func([]solve.WTOElement[CellRef])
	collect = func(items []solve.WTOElement[CellRef]) {
		for _, item := range items {
			storage = append(storage, item.Vertex)
			collect(item.Body)
		}
	}
	for _, element := range elements {
		start := len(storage)
		storage = append(storage, element.Vertex)
		collect(element.Body)
		refs := storage[start:len(storage)]
		sortCellRefs(refs)
		components = append(components, refs)
	}
	return components
}
