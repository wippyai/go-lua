package equation

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// BoundCyclicArtifact is a concrete-entry specialization of a frozen cyclic
// certificate. It has no symbolic carrier and no State conversion path.
type BoundCyclicArtifact struct {
	Artifact CyclicArtifact
	Entry    EntryBinding
	ByCell   map[CellID]BoundCyclicEquation
}

// BoundCyclicEquation keeps a cell identity attached to its already-concrete
// operand row. A cyclic kernel can therefore never be selected by a raw name.
type BoundCyclicEquation struct {
	Cell     CellID
	Equation BoundEquation
}

func BindCyclicEntry(artifact CyclicArtifact, entry EntryBinding) (BoundCyclicArtifact, error) {
	bound, err := BindEntry(artifact.Artifact, entry)
	if err != nil {
		return BoundCyclicArtifact{}, err
	}
	byCell := make(map[CellID]BoundCyclicEquation, len(bound.Equations))
	for _, equation := range bound.Equations {
		cell, ok := artifact.CellForTarget[equation.Target]
		if !ok {
			return BoundCyclicArtifact{}, fmt.Errorf("equation: bound cyclic artifact has no cell for %s", equation.Target.Name)
		}
		byCell[cell] = BoundCyclicEquation{Cell: cell, Equation: equation}
	}
	if len(byCell) != len(bound.Equations) {
		return BoundCyclicArtifact{}, fmt.Errorf("equation: bound cyclic artifact has duplicate cells")
	}
	return BoundCyclicArtifact{Artifact: artifact, Entry: EntryBinding{Parameter: entry.Parameter, Value: append([]byte(nil), entry.Value...)}, ByCell: byCell}, nil
}

// GuardedLeaf is a closed concrete product under one frozen guard partition.
// Guard is a canonical body-owned spelling, not a runtime predicate callback.
type GuardedLeaf struct {
	Guard   string
	Closure OutputClosure
}

// GuardedPartition is the complete guarded product for one cell. It is copied
// at every kernel boundary, so an aborted solve cannot leak mutable scratch.
type GuardedPartition struct{ Leaves []GuardedLeaf }

func (p GuardedPartition) clone() GuardedPartition {
	out := GuardedPartition{Leaves: make([]GuardedLeaf, len(p.Leaves))}
	for i, leaf := range p.Leaves {
		closure, err := joinClosure(leaf.Closure)
		if err != nil {
			return GuardedPartition{}
		}
		out.Leaves[i] = GuardedLeaf{Guard: leaf.Guard, Closure: closure}
	}
	return out
}

func canonicalPartition(in GuardedPartition) (GuardedPartition, error) {
	out := in.clone()
	if len(in.Leaves) != len(out.Leaves) {
		return GuardedPartition{}, fmt.Errorf("equation: malformed guarded partition")
	}
	for i := range out.Leaves {
		if out.Leaves[i].Guard == "" {
			return GuardedPartition{}, fmt.Errorf("equation: guarded partition has an empty guard")
		}
	}
	sort.Slice(out.Leaves, func(i, j int) bool { return out.Leaves[i].Guard < out.Leaves[j].Guard })
	merged := out.Leaves[:0]
	for _, leaf := range out.Leaves {
		if len(merged) == 0 || merged[len(merged)-1].Guard != leaf.Guard {
			merged = append(merged, leaf)
			continue
		}
		closure, err := joinClosure(merged[len(merged)-1].Closure, leaf.Closure)
		if err != nil {
			return GuardedPartition{}, err
		}
		merged[len(merged)-1].Closure = closure
	}
	out.Leaves = merged
	return out, nil
}

func partitionBytes(partition GuardedPartition) []byte {
	canonical, err := canonicalPartition(partition)
	if err != nil {
		return nil
	}
	out := appendText(nil, "equation/guarded-partition/v1")
	out = appendU64(out, uint64(len(canonical.Leaves)))
	for _, leaf := range canonical.Leaves {
		out = appendText(out, leaf.Guard)
		out = appendBytes(out, leaf.Closure.bytes())
	}
	return out
}

func partitionEqual(left, right GuardedPartition) bool {
	return bytes.Equal(partitionBytes(left), partitionBytes(right))
}

func partitionJoin(left, right GuardedPartition) GuardedPartition {
	return partitionMerge(nil, left, right)
}

// partitionMerge is the guarded-product lattice operation. Leaves are matched
// by guard cube, and within a cube the fact owner's merge resolves the rows
// that one trip of a recurrence and the next disagree about. A nil merge keeps
// the strict reading, in which such a disagreement is a defect.
func partitionMerge(merge FactMerge, left, right GuardedPartition) GuardedPartition {
	byGuard := make(map[string]OutputClosure, len(left.Leaves)+len(right.Leaves))
	for _, partition := range []GuardedPartition{left, right} {
		for _, leaf := range partition.Leaves {
			current, exists := byGuard[leaf.Guard]
			if !exists {
				byGuard[leaf.Guard] = leaf.Closure
				continue
			}
			joined, err := mergeClosures(merge, current, leaf.Closure)
			if err != nil {
				return GuardedPartition{}
			}
			byGuard[leaf.Guard] = joined
		}
	}
	out := GuardedPartition{Leaves: make([]GuardedLeaf, 0, len(byGuard))}
	for guard, closure := range byGuard {
		out.Leaves = append(out.Leaves, GuardedLeaf{Guard: guard, Closure: closure})
	}
	canonical, err := canonicalPartition(out)
	if err != nil {
		return GuardedPartition{}
	}
	return canonical
}

func partitionLessOrEqual(left, right GuardedPartition) bool {
	rightByGuard := make(map[string]OutputClosure, len(right.Leaves))
	for _, leaf := range right.Leaves {
		rightByGuard[leaf.Guard] = leaf.Closure
	}
	for _, leaf := range left.Leaves {
		other, ok := rightByGuard[leaf.Guard]
		if !ok || !closureContainedBy(leaf.Closure, other) {
			return false
		}
	}
	return true
}

func closureContainedBy(left, right OutputClosure) bool {
	left, leftErr := joinClosure(left)
	right, rightErr := joinClosure(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	containsFacts := func(need, have []Fact) bool {
		index := make(map[string][]byte, len(have))
		for _, fact := range have {
			index[fact.Key] = fact.Value
		}
		for _, fact := range need {
			if value, ok := index[fact.Key]; !ok || !bytes.Equal(value, fact.Value) {
				return false
			}
		}
		return true
	}
	if !containsFacts(left.Values, right.Values) || !containsFacts(left.Outcomes, right.Outcomes) || !containsFacts(left.Diagnostics, right.Diagnostics) {
		return false
	}
	for _, rekey := range left.AllocationRekeys {
		found := false
		for _, candidate := range right.AllocationRekeys {
			found = found || candidate == rekey
		}
		if !found {
			return false
		}
	}
	return true
}

func guardKey(guards []Guard) string {
	if len(guards) == 0 {
		return "<unguarded>"
	}
	parts := make([]string, len(guards))
	for i, guard := range guards {
		parts[i] = string(guard.Encoding)
	}
	sort.Strings(parts)
	encoded := make([][]byte, len(parts))
	for i := range parts {
		encoded[i] = []byte(parts[i])
	}
	return string(bytes.Join(encoded, []byte("\x00")))
}

// CyclicSnapshot is the immutable current solution visible to one concrete
// kernel. Read is dynamically audited by solve against the frozen WTO plan.
type CyclicSnapshot struct {
	read         func(CellID) GuardedPartition
	predecessors map[CellID][]CellID
}

func (s CyclicSnapshot) Read(cell CellID) GuardedPartition {
	if s.read == nil {
		return GuardedPartition{}
	}
	return s.read(cell).clone()
}

// Predecessors returns the explicit equation dependencies for cell. Control
// recurrence stays in the WTO certificate, not in this data-read inventory.
func (s CyclicSnapshot) Predecessors(cell CellID) []CellID {
	return append([]CellID(nil), s.predecessors[cell]...)
}

// CyclicKernel is the concrete transaction boundary for cyclic execution.
// Its input admits only bound operands and guarded concrete partitions.
type CyclicKernel interface {
	Execute(context.Context, BoundCyclicEquation, CyclicSnapshot) (TransactionResult, error)
}

type CyclicKernelFunc func(context.Context, BoundCyclicEquation, CyclicSnapshot) (TransactionResult, error)

func (f CyclicKernelFunc) Execute(ctx context.Context, equation BoundCyclicEquation, snapshot CyclicSnapshot) (TransactionResult, error) {
	return f(ctx, equation, snapshot)
}

type CyclicKernelBinding struct {
	KernelID   string
	ContractID ContentID
	Kernel     CyclicKernel
	Verify     func(AccessRecord) error
}
type CyclicKernelRegistry struct {
	bindings map[string]map[ContentID]CyclicKernelBinding
}

func NewCyclicKernelRegistry(bindings []CyclicKernelBinding) (*CyclicKernelRegistry, error) {
	registry := &CyclicKernelRegistry{bindings: make(map[string]map[ContentID]CyclicKernelBinding)}
	for _, binding := range bindings {
		if binding.KernelID == "" || !binding.ContractID.Valid() || binding.Kernel == nil {
			return nil, fmt.Errorf("equation: malformed cyclic kernel binding")
		}
		if registry.bindings[binding.KernelID] == nil {
			registry.bindings[binding.KernelID] = make(map[ContentID]CyclicKernelBinding)
		}
		if _, duplicate := registry.bindings[binding.KernelID][binding.ContractID]; duplicate {
			return nil, fmt.Errorf("equation: duplicate cyclic kernel binding %q", binding.KernelID)
		}
		registry.bindings[binding.KernelID][binding.ContractID] = binding
	}
	return registry, nil
}

func (r *CyclicKernelRegistry) resolve(equation BoundCyclicEquation) (CyclicKernelBinding, bool) {
	if r == nil {
		return CyclicKernelBinding{}, false
	}
	binding, ok := r.bindings[equation.Equation.KernelID][equation.Equation.Occurrence.ContractID]
	return binding, ok
}

// WideningTrace is a canonical, output-independent observation of an ascent
// update. It catches schedule drift even when two executions publish equal
// final closures.
type WideningTrace struct {
	Cell                     CellID
	Visit                    int
	Widened                  bool
	Previous, Joined, Result []byte
}

func (t WideningTrace) Equal(other WideningTrace) bool {
	return t.Cell == other.Cell && t.Visit == other.Visit && t.Widened == other.Widened && bytes.Equal(t.Previous, other.Previous) && bytes.Equal(t.Joined, other.Joined) && bytes.Equal(t.Result, other.Result)
}

type CyclicEvaluation struct {
	Closure       OutputClosure
	Transactions  int
	WideningTrace []WideningTrace
}

// CyclicShadowCase pairs the retained production publication with a concrete
// cyclic artifact and its production widening trace.  A trace mismatch is an
// error even when the published closure happens to be equal.
type CyclicShadowCase struct {
	Name       string
	Artifact   CyclicArtifact
	Entry      EntryBinding
	Selectors  []string
	Production func() (OutputClosure, []WideningTrace, error)
}

func RunCyclicShadow(ctx context.Context, vm *CyclicVM, cases []CyclicShadowCase) (ShadowReport, error) {
	report := ShadowReport{Cases: len(cases)}
	for _, shadow := range cases {
		if shadow.Name == "" || shadow.Production == nil {
			return report, fmt.Errorf("equation: malformed cyclic shadow case")
		}
		production, trace, err := shadow.Production()
		if err != nil {
			return report, fmt.Errorf("equation: cyclic shadow %s production: %w", shadow.Name, err)
		}
		production, err = joinClosure(production)
		if err != nil {
			return report, fmt.Errorf("equation: cyclic shadow %s production output: %w", shadow.Name, err)
		}
		bound, err := BindCyclicEntry(shadow.Artifact, shadow.Entry)
		if err != nil {
			return report, fmt.Errorf("equation: cyclic shadow %s binding: %w", shadow.Name, err)
		}
		evaluation, err := vm.Evaluate(ctx, bound, shadow.Selectors)
		if err != nil {
			return report, fmt.Errorf("equation: cyclic shadow %s evaluation: %w", shadow.Name, err)
		}
		if !production.Equal(evaluation.Closure) {
			return report, fmt.Errorf("equation: cyclic shadow %s published output differs", shadow.Name)
		}
		if len(trace) != len(evaluation.WideningTrace) {
			return report, fmt.Errorf("equation: cyclic shadow %s widening trace differs", shadow.Name)
		}
		for index := range trace {
			if !trace[index].Equal(evaluation.WideningTrace[index]) {
				return report, fmt.Errorf("equation: cyclic shadow %s widening trace differs", shadow.Name)
			}
		}
		report.Passed++
	}
	return report, nil
}

// RecurrenceLattice is the fact owner's abstract domain for a body whose
// operations are evaluated more than once. Join accumulates the publications
// two trips of a recurrence produced; Widen is the terminating operator the
// stabilization loop applies from a cell's second visit on and must reach a
// stationary value along every ascending chain. Both are asked only about
// rows that share a key and a guard cube and disagree, so an operation whose
// result does not depend on the recurrence is never approximated.
type RecurrenceLattice struct {
	Join  FactMerge
	Widen FactMerge
}

func (l RecurrenceLattice) valid() bool { return l.Join != nil && l.Widen != nil }

type CyclicVM struct {
	registry *CyclicKernelRegistry
	lattice  RecurrenceLattice
}

func NewCyclicVM(registry *CyclicKernelRegistry, lattice RecurrenceLattice) (*CyclicVM, error) {
	if registry == nil {
		return nil, fmt.Errorf("equation: nil cyclic kernel registry")
	}
	if !lattice.valid() {
		return nil, fmt.Errorf("equation: cyclic VM has no recurrence lattice")
	}
	return &CyclicVM{registry: registry, lattice: lattice}, nil
}

// Evaluate is transactional: every map, trace, and partition belongs to this
// invocation. It returns a result only after ascent, narrowing, audit, and a
// final cancellation check all complete.
func (vm *CyclicVM) Evaluate(ctx context.Context, bound BoundCyclicArtifact, selectorIDs []string) (CyclicEvaluation, error) {
	if ctx == nil || vm == nil || vm.registry == nil || !bound.Entry.valid() || len(bound.ByCell) == 0 {
		return CyclicEvaluation{}, fmt.Errorf("equation: invalid bound cyclic artifact")
	}
	if err := ctx.Err(); err != nil {
		return CyclicEvaluation{}, err
	}
	plan, err := bound.Artifact.RestrictPlan(selectorIDs)
	if err != nil {
		return CyclicEvaluation{}, err
	}
	cells := flattenCyclicPlan(plan.Elements())
	planned := make(map[CellID]bool, len(cells))
	for _, cell := range cells {
		if _, ok := bound.ByCell[cell]; !ok {
			return CyclicEvaluation{}, fmt.Errorf("equation: cyclic plan has no bound concrete cell %q", cell)
		}
		planned[cell] = true
	}
	// The predecessor relation is the artifact's complete semantic edge set,
	// not the emission chain alone. Emission order names one straight-line
	// prefix; the frozen CFG edges additionally name every back edge, so a
	// consumer inside a loop observes the publications the previous trip
	// carried rather than the pre-loop state peeled once.
	predecessors := make(map[CellID][]CellID, len(cells))
	seenEdge := make(map[[2]CellID]bool, len(cells))
	for _, edge := range bound.Artifact.Dependencies {
		if !planned[edge.From] || !planned[edge.To] {
			continue
		}
		key := [2]CellID{edge.To, edge.From}
		if seenEdge[key] {
			continue
		}
		seenEdge[key] = true
		predecessors[edge.To] = append(predecessors[edge.To], edge.From)
	}
	widenAt := make(map[CellID]bool, len(bound.Artifact.WidenCells))
	for _, cell := range bound.Artifact.WidenCells {
		widenAt[cell] = true
	}
	var trace []WideningTrace
	var executionErr error
	transactions := 0
	domain := lattice.Lattice[GuardedPartition]{
		Bottom: func() GuardedPartition { return GuardedPartition{} }, Equal: partitionEqual, LessOrEq: partitionLessOrEqual,
		Join: func(left, right GuardedPartition) GuardedPartition {
			return partitionMerge(vm.lattice.Join, left, right)
		},
		Widen: func(left, right GuardedPartition) GuardedPartition {
			return partitionMerge(vm.lattice.Widen, left, right)
		},
	}
	values, err := solve.SolveWTOContext(ctx, solve.EquationSystem[CellID, GuardedPartition]{
		Lattice: domain, Cells: cells, WidenAt: func(cell CellID) bool { return widenAt[cell] },
		Evaluate: func(cell CellID, read func(CellID) GuardedPartition) GuardedPartition {
			if executionErr != nil {
				return GuardedPartition{}
			}
			if err := ctx.Err(); err != nil {
				executionErr = err
				return GuardedPartition{}
			}
			equation, ok := bound.ByCell[cell]
			if !ok {
				executionErr = fmt.Errorf("equation: cyclic plan has no bound concrete cell %q", cell)
				return GuardedPartition{}
			}
			binding, ok := vm.registry.resolve(equation)
			if !ok {
				executionErr = fmt.Errorf("equation: no contract-bound cyclic kernel for %s", equation.Equation.Target.Name)
				return GuardedPartition{}
			}
			result, callErr := binding.Kernel.Execute(ctx, equation, CyclicSnapshot{read: read, predecessors: predecessors})
			if callErr != nil {
				executionErr = fmt.Errorf("equation: cyclic transaction %s: %w", equation.Equation.Target.Name, callErr)
				return GuardedPartition{}
			}
			if !result.Complete {
				executionErr = fmt.Errorf("equation: cyclic transaction %s: %w", equation.Equation.Target.Name, ErrIncompleteTransaction)
				return GuardedPartition{}
			}
			if binding.Verify != nil {
				if verifyErr := binding.Verify(result.Access); verifyErr != nil {
					executionErr = fmt.Errorf("equation: cyclic transaction %s access audit: %w", equation.Equation.Target.Name, verifyErr)
					return GuardedPartition{}
				}
			}
			// A cyclic publication carries the branch view it was produced
			// under, the same stamp the acyclic VM applies. Without it an
			// arm-local write rejoins the solution unguarded and becomes
			// visible to the other arm and to everything past the branch.
			closure, canonicalErr := joinClosure(stampClosure(result.Closure, equation.Equation.Guards))
			if canonicalErr != nil {
				executionErr = fmt.Errorf("equation: cyclic transaction %s output: %w", equation.Equation.Target.Name, canonicalErr)
				return GuardedPartition{}
			}
			transactions++
			return GuardedPartition{Leaves: []GuardedLeaf{{Guard: guardKey(equation.Equation.Guards), Closure: closure}}}
		},
		UpdateObserver: func(cell CellID, update solve.UpdateEvent[GuardedPartition]) {
			if !widenAt[cell] {
				return
			}
			trace = append(trace, WideningTrace{Cell: cell, Visit: update.Visit, Widened: update.Widened, Previous: partitionBytes(update.Previous), Joined: partitionBytes(update.Joined), Result: partitionBytes(update.Result)})
		},
	}, plan)
	if err != nil {
		return CyclicEvaluation{}, err
	}
	if executionErr != nil {
		return CyclicEvaluation{}, executionErr
	}
	if err := ctx.Err(); err != nil {
		return CyclicEvaluation{}, err
	}
	closure := OutputClosure{}
	for _, cell := range cells {
		for _, leaf := range values[cell].Leaves {
			closure, err = joinClosure(closure, leaf.Closure)
			if err != nil {
				return CyclicEvaluation{}, fmt.Errorf("equation: cyclic closure: %w", err)
			}
		}
	}
	return CyclicEvaluation{Closure: closure, Transactions: transactions, WideningTrace: trace}, nil
}

func flattenCyclicPlan(elements []solve.WTOElement[CellID]) []CellID {
	var cells []CellID
	var visit func([]solve.WTOElement[CellID])
	visit = func(items []solve.WTOElement[CellID]) {
		for _, item := range items {
			cells = append(cells, item.Vertex)
			visit(item.Body)
		}
	}
	visit(elements)
	return cells
}
