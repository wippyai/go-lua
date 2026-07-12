package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SymbolicCFGExpandTransfer may replace one incoming row with correlated
// alternatives. Direct calls use this seam to cross-product callee Relation
// rows; ordinary transfers return a one-element slice.
type SymbolicCFGExpandTransfer func(cfg.Point, SymbolicCFGRow) ([]SymbolicCFGRow, error)

// SolveAcyclicCFGExpandedRows is the row-expanding counterpart of
// SolveAcyclicCFGRows. It preserves the same deterministic topology, branch,
// and budget rules while deduplicating only after the complete point transfer.
func SolveAcyclicCFGExpandedRows(graph cfg.Graph, arena *Arena, initial SymbolicCFGRow, transfer SymbolicCFGExpandTransfer, branch SymbolicCFGBranch, options SymbolicCFGOptions) (map[cfg.Point][]SymbolicCFGRow, error) {
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
		var transferred []SymbolicCFGRow
		for _, in := range incoming {
			produced := []SymbolicCFGRow{cloneCFGRow(in)}
			if transfer != nil {
				produced, err = transfer(point, cloneCFGRow(in))
				if err != nil {
					return nil, fmt.Errorf("transformer: symbolic CFG point %d: %w", point, err)
				}
			}
			for _, out := range produced {
				if !validCFGRow(arena, options.Shape, out) {
					return nil, fmt.Errorf("transformer: symbolic CFG point %d produced an invalid row", point)
				}
				transferred = append(transferred, out)
			}
		}
		transferred = dedupCFGRows(arena, transferred)
		if len(transferred) > options.MaxRows {
			return nil, fmt.Errorf("transformer: symbolic CFG row budget at point %d", point)
		}
		for _, out := range transferred {
			successors := cfg.SuccessorsReadOnly(graph, point)
			if len(successors) == 0 {
				continue
			}
			if graph.IsBranch(point) {
				if branch == nil || len(successors) != 2 {
					return nil, fmt.Errorf("transformer: symbolic CFG branch %d has no exact branch algebra", point)
				}
				for _, successor := range successors {
					cond, ok := graph.EdgeCond(point, successor)
					if !ok {
						return nil, fmt.Errorf("transformer: symbolic CFG branch %d edge polarity missing", point)
					}
					next, edgeGuard, branchErr := branch(point, cloneCFGRow(out), cond)
					if branchErr != nil {
						return nil, fmt.Errorf("transformer: symbolic CFG branch %d edge %t: %w", point, cond, branchErr)
					}
					if next.Guard != out.Guard {
						return nil, fmt.Errorf("transformer: symbolic CFG branch %d edge %t replaced the incoming guard", point, cond)
					}
					if !validCFGRow(arena, options.Shape, next) || !arena.validGuard(edgeGuard, options.Shape) {
						return nil, fmt.Errorf("transformer: symbolic CFG branch %d edge %t produced an invalid row", point, cond)
					}
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
			rows[successor] = dedupCFGRows(arena, rows[successor])
			if len(rows[successor]) > options.MaxRows {
				return nil, fmt.Errorf("transformer: symbolic CFG row budget at point %d", successor)
			}
		}
	}
	return rows, nil
}
