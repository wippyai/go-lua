package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// SymbolicCFGRow is one correlated control-flow alternative. Values are
// compiler-local lexical bindings; Guard retains the path condition.
type SymbolicCFGRow struct {
	Guard  Guard
	Values map[symbol.ID]ValueTerm
}

type SymbolicCFGTransfer func(cfg.Point, SymbolicCFGRow) (SymbolicCFGRow, error)
type SymbolicCFGBranch func(cfg.Point, SymbolicCFGRow) (truthy, falsy Guard, err error)

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
	if graph == nil || arena == nil {
		return nil, fmt.Errorf("transformer: symbolic CFG requires graph and arena")
	}
	options = options.normalized()
	if !validCFGRow(arena, options.Shape, initial) {
		return nil, fmt.Errorf("transformer: symbolic CFG initial row is invalid for boundary shape")
	}
	order, err := acyclicCFGOrder(graph)
	if err != nil {
		return nil, err
	}
	rows := make(map[cfg.Point][]SymbolicCFGRow, len(order))
	rows[graph.Entry()] = []SymbolicCFGRow{cloneCFGRow(initial)}
	for _, point := range order {
		incoming := rows[point]
		if len(incoming) == 0 {
			continue
		}
		for _, in := range incoming {
			out := cloneCFGRow(in)
			if transfer != nil {
				out, err = transfer(point, out)
				if err != nil {
					return nil, fmt.Errorf("transformer: symbolic CFG point %d: %w", point, err)
				}
			}
			if !validCFGRow(arena, options.Shape, out) {
				return nil, fmt.Errorf("transformer: symbolic CFG point %d produced an invalid row", point)
			}
			successors := cfg.SuccessorsReadOnly(graph, point)
			if len(successors) == 0 {
				continue
			}
			if graph.IsBranch(point) {
				if branch == nil || len(successors) != 2 {
					return nil, fmt.Errorf("transformer: symbolic CFG branch %d has no exact branch algebra", point)
				}
				truthy, falsy, branchErr := branch(point, out)
				if branchErr != nil {
					return nil, fmt.Errorf("transformer: symbolic CFG branch %d: %w", point, branchErr)
				}
				if !arena.validGuard(truthy, options.Shape) || !arena.validGuard(falsy, options.Shape) {
					return nil, fmt.Errorf("transformer: symbolic CFG branch %d produced an invalid guard", point)
				}
				for _, successor := range successors {
					cond, ok := graph.EdgeCond(point, successor)
					if !ok {
						return nil, fmt.Errorf("transformer: symbolic CFG branch %d edge polarity missing", point)
					}
					edgeGuard := falsy
					if cond {
						edgeGuard = truthy
					}
					next := cloneCFGRow(out)
					next.Guard = arena.And(out.Guard, edgeGuard)
					if next.Guard != arena.False() {
						rows[successor] = append(rows[successor], next)
					}
				}
			} else {
				if len(successors) != 1 {
					return nil, fmt.Errorf("transformer: non-branch point %d has %d successors", point, len(successors))
				}
				rows[successors[0]] = append(rows[successors[0]], cloneCFGRow(out))
			}
		}
		for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
			rows[successor] = dedupCFGRows(rows[successor])
			if len(rows[successor]) > options.MaxRows {
				return nil, fmt.Errorf("transformer: symbolic CFG row budget at point %d", successor)
			}
		}
	}
	return rows, nil
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
	if !arena.validGuard(row.Guard, shape) {
		return false
	}
	for _, value := range row.Values {
		if !arena.validValue(value, shape, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}

func cloneCFGRow(row SymbolicCFGRow) SymbolicCFGRow {
	out := SymbolicCFGRow{Guard: row.Guard, Values: make(map[symbol.ID]ValueTerm, len(row.Values))}
	for key, value := range row.Values {
		out.Values[key] = value
	}
	return out
}

// dedupCFGRows compares interned term handles directly. It deliberately avoids
// canonical strings and sorting: rows from one Arena have structural identity,
// so exact duplicate elimination is allocation-free.
func dedupCFGRows(rows []SymbolicCFGRow) []SymbolicCFGRow {
	if len(rows) < 2 {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		duplicate := false
		for _, kept := range out {
			if equalCFGRow(row, kept) {
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

func equalCFGRow(left, right SymbolicCFGRow) bool {
	if left.Guard != right.Guard || len(left.Values) != len(right.Values) {
		return false
	}
	for key, value := range left.Values {
		if right.Values[key] != value {
			return false
		}
	}
	return true
}
