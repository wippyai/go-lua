package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// SymbolicCFGRow is one correlated control-flow alternative. Values are
// compiler-local lexical bindings; Guard retains the path condition.
type SymbolicCFGRow struct {
	Guard           Guard
	Values          map[symbol.ID]ValueTerm
	Operations      []Operation
	Effects         []EffectTerm
	Proofs          []BranchProofTerm
	Observations    []ObservationTerm
	Output          summary.Summary
	genericBindings map[symbol.ID]symbolicGenericBinding
	paramPreserved  paramPreservationLedger
}

type SymbolicCFGTransfer func(cfg.Point, SymbolicCFGRow) (SymbolicCFGRow, error)

// SymbolicCFGBranch lowers one polarized edge. It may update row-local
// bindings/evidence, but must preserve the incoming path Guard; the solver
// conjoins edgeGuard itself so callbacks cannot accidentally drop correlation.
type SymbolicCFGBranch func(cfg.Point, SymbolicCFGRow, bool) (row SymbolicCFGRow, edgeGuard Guard, err error)

type SymbolicCFGOptions struct {
	Shape   Shape
	MaxRows int
}

func (o SymbolicCFGOptions) normalized() SymbolicCFGOptions {
	if o.MaxRows <= 0 {
		o.MaxRows = 256
	}
	return o
}

// SolveAcyclicCFGRows propagates immutable correlated rows in topological
// order. Joins union rows rather than independently joining bindings, so path
// correlation is not lost. Cycles fail closed for the subsequent WTO/SCC
// stage; no prefix rows are returned on any error or budget overflow. This
// stage propagates guard-only edges. Refinements and evidence must be lowered
// by a later edge-row transfer rather than being silently discarded here.
func SolveAcyclicCFGRows(graph cfg.Graph, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGTransfer, branch SymbolicCFGBranch, options SymbolicCFGOptions) (map[cfg.Point][]SymbolicCFGRow, error) {
	var expanded SymbolicCFGExpandTransfer
	if transfer != nil {
		expanded = func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			out, err := transfer(point, row)
			if err != nil {
				return nil, err
			}
			return []SymbolicCFGRow{out}, nil
		}
	}
	return SolveAcyclicCFGExpandedRows(graph, arena, initial, expanded, branch, options)
}

func acyclicCFGOrder(graph cfg.Graph) ([]cfg.Point, error) {
	indegree := make([]int, graph.Size())
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
			indegree[successor]++
		}
	}
	ready := []cfg.Point{}
	for point, count := range indegree {
		if count == 0 {
			ready = append(ready, cfg.Point(point))
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	order := make([]cfg.Point, 0, graph.Size())
	for len(ready) > 0 {
		point := ready[0]
		ready = ready[1:]
		order = append(order, point)
		for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
			indegree[successor]--
			if indegree[successor] == 0 {
				ready = append(ready, successor)
				sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
			}
		}
	}
	if len(order) != graph.Size() {
		return nil, fmt.Errorf("transformer: cyclic CFG requires WTO/SCC rows")
	}
	return order, nil
}

func validCFGRow(arena *Arena, shape Shape, row SymbolicCFGRow) bool {
	if !arena.validGuard(row.Guard, shape) || !row.paramPreserved.valid(shape.Params+shape.Captures) ||
		(row.paramPreserved.tracked && row.paramPreserved.boundaryParams != shape.Params) {
		return false
	}
	for _, value := range row.Values {
		if !arena.validValue(value, shape, make(map[ValueTerm]bool)) {
			return false
		}
	}
	for _, effect := range row.Effects {
		if effect == 0 {
			return false
		}
	}
	for _, proof := range row.Proofs {
		if !proof.valid(arena, shape) {
			return false
		}
	}
	for _, observation := range row.Observations {
		if !observation.valid(arena, shape) {
			return false
		}
	}
	return true
}

func cloneCFGRow(row SymbolicCFGRow) SymbolicCFGRow {
	out := SymbolicCFGRow{
		Guard:           row.Guard,
		Values:          make(map[symbol.ID]ValueTerm, len(row.Values)),
		Operations:      append([]Operation(nil), row.Operations...),
		Effects:         append([]EffectTerm(nil), row.Effects...),
		Proofs:          append([]BranchProofTerm(nil), row.Proofs...),
		Observations:    append([]ObservationTerm(nil), row.Observations...),
		Output:          row.Output.Clone(),
		genericBindings: make(map[symbol.ID]symbolicGenericBinding, len(row.genericBindings)),
		paramPreserved:  row.paramPreserved.clone(),
	}
	for key, value := range row.Values {
		out.Values[key] = value
	}
	for key, value := range row.genericBindings {
		out.genericBindings[key] = value
	}
	return out
}

// dedupCFGRows compares interned term handles directly. It deliberately avoids
// canonical strings and sorting: rows from one Arena have structural identity,
// so exact duplicate elimination is allocation-free.
func dedupCFGRows(arena *Arena, rows []SymbolicCFGRow) []SymbolicCFGRow {
	if len(rows) < 2 {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		duplicate := false
		for i := range out {
			if equalCFGRow(arena, row, out[i]) {
				out[i].Observations = unionObservationTerms(arena, out[i].Observations, row.Observations)
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, row)
		}
	}
	return out
}

func equalCFGRow(arena *Arena, left, right SymbolicCFGRow) bool {
	if left.Guard != right.Guard || len(left.Values) != len(right.Values) || len(left.Operations) != len(right.Operations) || len(left.Effects) != len(right.Effects) || len(left.Proofs) != len(right.Proofs) || len(left.genericBindings) != len(right.genericBindings) || !left.paramPreserved.equal(right.paramPreserved) {
		return false
	}
	if !summary.Equal(arena.reg, left.Output, right.Output) {
		return false
	}
	for key, value := range left.Values {
		if right.Values[key] != value {
			return false
		}
	}
	for i := range left.Operations {
		if left.Operations[i] != right.Operations[i] {
			return false
		}
	}
	for i := range left.Effects {
		if left.Effects[i] != right.Effects[i] {
			return false
		}
	}
	for i := range left.Proofs {
		if left.Proofs[i] != right.Proofs[i] {
			return false
		}
	}
	for key, value := range left.genericBindings {
		if right.genericBindings[key] != value {
			return false
		}
	}
	return true
}
