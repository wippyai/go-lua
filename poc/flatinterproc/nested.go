package flatinterproc

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// solveNested is the reference cost model: one complete intraprocedural WTO is
// the transfer function of each outer summary equation.
func solveNested(ctx context.Context, p program) (snapshot, error) {
	if err := p.validate(); err != nil {
		return snapshot{}, err
	}
	return solveNestedFrom(ctx, p, cloneContexts(p.roots))
}

func solveNestedFrom(ctx context.Context, p program, initialContexts map[summary.SummaryKey]state.State) (snapshot, error) {
	states := state.Domain(p.reg)
	summariesDomain := summary.NormalizedDomain(p.reg)
	contexts := cloneContexts(initialContexts)
	summaries := make(map[summary.SummaryKey]summary.Summary)
	totalTransfers := 0
	for round := 0; round < len(p.functions)*32; round++ {
		changed := false
		entries := cloneContexts(p.roots)
		for _, key := range sortedContextKeys(contexts) {
			got, calls, transfers, err := solveNestedBody(ctx, p, key, contexts[key], summaries, contexts)
			if err != nil {
				return snapshot{}, err
			}
			totalTransfers += transfers
			joinedSummary := summariesDomain.Join(summaries[key], got)
			if !summariesDomain.Equal(summaries[key], joinedSummary) {
				summaries[key] = joinedSummary
				changed = true
			}
			for callee, entry := range calls {
				if prior, ok := entries[callee]; ok {
					joined := states.Join(prior, entry)
					if !states.Equal(prior, joined) {
						entries[callee] = joined
					}
				} else {
					entries[callee] = entry
				}
			}
		}
		for key, entry := range entries {
			if prior, ok := contexts[key]; ok {
				joined := states.Join(prior, entry)
				if !states.Equal(prior, joined) {
					contexts[key] = joined
					changed = true
				}
			} else {
				contexts[key] = entry
				changed = true
			}
		}
		if !changed {
			return snapshot{summaries: cloneSummaryMap(summaries), contexts: contexts, transfers: totalTransfers}, nil
		}
	}
	return snapshot{}, errors.New("flatinterproc: nested reference did not converge")
}

func solveNestedBody(
	ctx context.Context,
	p program,
	key summary.SummaryKey,
	entry state.State,
	summaries map[summary.SummaryKey]summary.Summary,
	known map[summary.SummaryKey]state.State,
) (summary.Summary, map[summary.SummaryKey]state.State, int, error) {
	fn := p.functions[key.Ref]
	exit := len(fn.nodes)
	cells := make([]int, exit+1)
	for i := range cells {
		cells[i] = i
	}
	states := state.Domain(p.reg)
	calls := make(map[summary.SummaryKey]state.State)
	var stats solve.Stats
	system := solve.EquationSystem[int, state.State]{
		Lattice: states,
		Cells:   cells,
		Initial: func(point int) state.State {
			if point == fn.entry {
				return entry
			}
			return states.Bottom()
		},
		Transfer: func(point int, read func(int) state.State, emit func(int, state.State)) {
			if point == exit {
				return
			}
			in := read(point)
			if states.Equal(in, states.Bottom()) {
				return
			}
			node := fn.nodes[point]
			switch node.op {
			case opFork:
				for _, next := range node.next {
					emit(next, in)
				}
			case opConst:
				out := in.WriteValue(p.reg, valueSlot, node.value)
				for _, next := range node.next {
					emit(next, out)
				}
			case opCall:
				if node.external {
					out := in.WriteValue(p.reg, valueSlot, product.Top())
					for _, next := range node.next {
						emit(next, out)
					}
					return
				}
				argument := valueOf(p.reg, in)
				calleeKey := contextKey(p.reg, node.callee, argument, node.contextual)
				calleeEntry := stateWithValue(p.reg, argument)
				if prior, ok := calls[calleeKey]; ok {
					calls[calleeKey] = states.Join(prior, calleeEntry)
				} else {
					calls[calleeKey] = calleeEntry
				}
				if _, ok := known[calleeKey]; !ok {
					return
				}
				returned := returnAt(p.reg, summaries[calleeKey])
				if product.Equal(p.reg, returned, product.Bottom(p.reg)) {
					return
				}
				out := in.WriteValue(p.reg, valueSlot, returned)
				for _, next := range node.next {
					emit(next, out)
				}
			case opReturn:
				emit(exit, in)
			}
		},
		WidenAt:    func(point int) bool { return point < exit && fn.nodes[point].loopHead },
		WidenDelay: func(int) int { return 2 },
		Stats:      &stats,
	}
	plan := solve.NewWTOPlan(cells, func(point int) []int {
		if point == exit {
			return nil
		}
		out := append([]int{point}, fn.nodes[point].next...)
		if fn.nodes[point].op == opReturn {
			out = append(out, exit)
		}
		return out
	})
	result, _, err := solve.SolveWTOContextWithVersions(ctx, system, plan)
	if err != nil {
		return summary.Summary{}, nil, stats.TransferCalls, err
	}
	returned := valueOf(p.reg, result[exit])
	got := summary.Summary{}
	if !product.Equal(p.reg, returned, product.Bottom(p.reg)) {
		got = summary.Normalize(p.reg, summary.Summary{Returns: []product.Value{returned}})
	}
	return got, calls, stats.TransferCalls, nil
}

func cloneSummaryMap(in map[summary.SummaryKey]summary.Summary) map[summary.SummaryKey]summary.Summary {
	out := make(map[summary.SummaryKey]summary.Summary, len(in))
	for key, value := range in {
		out[key] = value.Clone()
	}
	return out
}
