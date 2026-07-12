package flatinterproc

import (
	"context"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type cellKind uint8

const (
	flowCell cellKind = iota + 1
	summaryCell
)

type globalCell struct {
	key   summary.SummaryKey
	kind  cellKind
	point int
}

func (c globalCell) less(other globalCell) bool {
	if c.key != other.key {
		return c.key.Less(other.key)
	}
	if c.kind != other.kind {
		return c.kind < other.kind
	}
	return c.point < other.point
}

type globalValue struct {
	flow    state.State
	summary summary.Summary
}

func globalDomain(p program) lattice.Lattice[globalValue] {
	states := state.Domain(p.reg) // all registered lanes, unchanged
	summaries := summary.NormalizedDomain(p.reg)
	return lattice.Lattice[globalValue]{
		Bottom: func() globalValue {
			return globalValue{flow: states.Bottom(), summary: summaries.Bottom()}
		},
		Top: func() globalValue {
			return globalValue{flow: states.Top(), summary: summary.Summary{Returns: []product.Value{product.Top()}}}
		},
		Equal: func(a, b globalValue) bool {
			return states.Equal(a.flow, b.flow) && summaries.Equal(a.summary, b.summary)
		},
		LessOrEq: func(a, b globalValue) bool {
			return states.LessOrEq(a.flow, b.flow) && summaries.LessOrEq(a.summary, b.summary)
		},
		Join: func(a, b globalValue) globalValue {
			return globalValue{flow: states.Join(a.flow, b.flow), summary: summaries.Join(a.summary, b.summary)}
		},
		Widen: func(a, b globalValue) globalValue {
			return globalValue{flow: states.Widen(a.flow, b.flow), summary: summaries.Widen(a.summary, b.summary)}
		},
		Narrow: func(a, b globalValue) globalValue {
			flow := b.flow
			if states.Narrow != nil {
				flow = states.Narrow(a.flow, b.flow)
			}
			// Summary has no independent narrowing API. Re-projecting the
			// narrowed flow is justified by the current transfer result.
			return globalValue{flow: flow, summary: summary.Normalize(p.reg, b.summary)}
		},
	}
}

type flatEngine struct {
	program   program
	published snapshot
}

func solveFlatPreseeded(ctx context.Context, p program, contexts map[summary.SummaryKey]state.State) (snapshot, error) {
	got, discovered, err := solveFlatKnown(ctx, p, cloneContexts(contexts))
	if err != nil {
		return snapshot{}, err
	}
	states := state.Domain(p.reg)
	for key, contributed := range discovered {
		seeded, ok := contexts[key]
		if !ok || !states.LessOrEq(contributed, seeded) {
			return snapshot{}, errors.New("flatinterproc: preseeded context set is incomplete")
		}
	}
	got.contexts = cloneContexts(contexts)
	return got, nil
}

func (e *flatEngine) solve(ctx context.Context) (snapshot, error) {
	if err := e.program.validate(); err != nil {
		return snapshot{}, err
	}
	contexts := cloneContexts(e.program.roots)
	totalTransfers := 0
	for discoveryRound := 0; discoveryRound <= len(e.program.functions)*8; discoveryRound++ {
		got, discovered, err := solveFlatKnown(ctx, e.program, contexts)
		if err != nil {
			return e.published.clone(), err
		}
		totalTransfers += got.transfers
		changed := false
		for key, entry := range discovered {
			if prior, ok := contexts[key]; ok {
				joined := state.Domain(e.program.reg).Join(prior, entry)
				if !state.Domain(e.program.reg).Equal(prior, joined) {
					contexts[key] = joined
					changed = true
				}
				continue
			}
			contexts[key] = entry
			changed = true
		}
		if !changed {
			got.contexts = cloneContexts(contexts)
			got.transfers = totalTransfers
			e.published = got.clone() // transactional publication
			return got, nil
		}
	}
	return e.published.clone(), errors.New("flatinterproc: context discovery did not converge")
}

func solveFlatKnown(ctx context.Context, p program, contexts map[summary.SummaryKey]state.State) (snapshot, map[summary.SummaryKey]state.State, error) {
	domain := globalDomain(p)
	cells := globalCells(p, contexts)
	known := make(map[summary.SummaryKey]struct{}, len(contexts))
	for key := range contexts {
		known[key] = struct{}{}
	}
	discovered := make(map[summary.SummaryKey]state.State)
	states := state.Domain(p.reg)
	var stats solve.Stats
	system := solve.EquationSystem[globalCell, globalValue]{
		Lattice: domain,
		Cells:   cells,
		Initial: func(cell globalCell) globalValue {
			if cell.kind != flowCell {
				return domain.Bottom()
			}
			fn := p.functions[cell.key.Ref]
			if cell.point != fn.entry {
				return domain.Bottom()
			}
			return globalValue{flow: contexts[cell.key], summary: summary.Summary{}}
		},
		Transfer: func(cell globalCell, read func(globalCell) globalValue, emit func(globalCell, globalValue)) {
			if cell.kind != flowCell {
				return
			}
			in := read(cell).flow
			if states.Equal(in, states.Bottom()) {
				return
			}
			fn := p.functions[cell.key.Ref]
			node := fn.nodes[cell.point]
			emitFlow := func(point int, flow state.State) {
				emit(globalCell{key: cell.key, kind: flowCell, point: point}, globalValue{flow: flow, summary: summary.Summary{}})
			}
			switch node.op {
			case opFork:
				for _, next := range node.next {
					emitFlow(next, in)
				}
			case opConst:
				out := in.WriteValue(p.reg, valueSlot, node.value)
				for _, next := range node.next {
					emitFlow(next, out)
				}
			case opCall:
				if node.external {
					out := in.WriteValue(p.reg, valueSlot, product.Top())
					for _, next := range node.next {
						emitFlow(next, out)
					}
					return
				}
				argument := valueOf(p.reg, in)
				calleeKey := contextKey(p.reg, node.callee, argument, node.contextual)
				calleeEntry := stateWithValue(p.reg, argument)
				// Contributions belong to the next immutable context generation.
				// Record them even for a known key: a call that becomes reachable
				// during summary convergence can enlarge that key's entry state.
				if prior, seen := discovered[calleeKey]; seen {
					discovered[calleeKey] = states.Join(prior, calleeEntry)
				} else {
					discovered[calleeKey] = calleeEntry
				}
				if _, ok := known[calleeKey]; !ok {
					return
				}
				// Known context entries are immutable inputs of this solve generation.
				// They were already joined during context discovery and are seeded by
				// Initial. Re-emitting them here creates a false caller/callee cycle in
				// the WTO graph without adding information.
				calleeSummary := read(globalCell{key: calleeKey, kind: summaryCell}).summary
				returned := returnAt(p.reg, calleeSummary)
				// Known internal Bottom means no return yet. This is the required
				// monotone seam; missing external/dynamic calls use Top above.
				if product.Equal(p.reg, returned, product.Bottom(p.reg)) {
					return
				}
				out := in.WriteValue(p.reg, valueSlot, returned)
				for _, next := range node.next {
					emitFlow(next, out)
				}
			case opReturn:
				sum := summary.Normalize(p.reg, summary.Summary{Returns: []product.Value{valueOf(p.reg, in)}})
				emit(globalCell{key: cell.key, kind: summaryCell}, globalValue{flow: states.Bottom(), summary: sum})
			}
		},
		WidenAt: func(cell globalCell) bool {
			return cell.kind == flowCell && p.functions[cell.key.Ref].nodes[cell.point].loopHead
		},
		WidenDelay: func(globalCell) int { return 2 },
		Stats:      &stats,
	}
	plan := solve.NewWTOPlan(cells, func(cell globalCell) []globalCell { return flatInfluences(p, contexts, cell) })
	result, _, err := solve.SolveWTOContextWithVersions(ctx, system, plan)
	if err != nil {
		return snapshot{}, nil, err
	}
	summaries := make(map[summary.SummaryKey]summary.Summary, len(contexts))
	for key := range contexts {
		summaries[key] = result[globalCell{key: key, kind: summaryCell}].summary
	}
	return snapshot{summaries: summaries, contexts: cloneContexts(contexts), transfers: stats.TransferCalls}, discovered, nil
}

func globalCells(p program, contexts map[summary.SummaryKey]state.State) []globalCell {
	var cells []globalCell
	for _, key := range sortedContextKeys(contexts) {
		fn := p.functions[key.Ref]
		for point := range fn.nodes {
			cells = append(cells, globalCell{key: key, kind: flowCell, point: point})
		}
		cells = append(cells, globalCell{key: key, kind: summaryCell})
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].less(cells[j]) })
	return cells
}

func flatInfluences(p program, contexts map[summary.SummaryKey]state.State, cell globalCell) []globalCell {
	if cell.kind == summaryCell {
		var callers []globalCell
		for key := range contexts {
			fn := p.functions[key.Ref]
			for point, node := range fn.nodes {
				if node.op == opCall && !node.external && node.callee == cell.key.Ref {
					callers = append(callers, globalCell{key: key, kind: flowCell, point: point})
				}
			}
		}
		return callers
	}
	fn := p.functions[cell.key.Ref]
	node := fn.nodes[cell.point]
	out := []globalCell{cell} // transfer reads its own accumulated flow
	for _, next := range node.next {
		out = append(out, globalCell{key: cell.key, kind: flowCell, point: next})
	}
	if node.op == opReturn {
		out = append(out, globalCell{key: cell.key, kind: summaryCell})
	}
	return out
}

func cloneContexts(in map[summary.SummaryKey]state.State) map[summary.SummaryKey]state.State {
	out := make(map[summary.SummaryKey]state.State, len(in))
	for key, entry := range in {
		out[key] = entry
	}
	return out
}
