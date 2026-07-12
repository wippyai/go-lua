package symboliccall

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// Stats exposes the existing solver's transfer count.
type Stats struct{ Solver solve.Stats }

// Compile solves direct-call transformer equations using the production WTO
// engine. The declared graph is callee -> caller because callee growth
// influences callers.
func Compile(ctx context.Context, definitions []Definition, stats *Stats) (map[FunctionID]Transformer, error) {
	byID := make(map[FunctionID]Definition, len(definitions))
	cells := make([]FunctionID, 0, len(definitions))
	callers := make(map[FunctionID][]FunctionID)
	for _, def := range definitions {
		if def.ID == "" {
			return nil, fmt.Errorf("symboliccall: empty function id")
		}
		if _, duplicate := byID[def.ID]; duplicate {
			return nil, fmt.Errorf("symboliccall: duplicate function %q", def.ID)
		}
		byID[def.ID] = def
		cells = append(cells, def.ID)
	}
	// Every equation emits its own next approximation. WTO requires that
	// recurrence to be declared even for otherwise acyclic functions.
	for _, id := range cells {
		callers[id] = append(callers[id], id)
	}
	for _, def := range definitions {
		seen := map[FunctionID]bool{}
		walkCalls(def.Returns, func(callee FunctionID) {
			if seen[callee] {
				return
			}
			seen[callee] = true
			if _, ok := byID[callee]; ok {
				callers[callee] = append(callers[callee], def.ID)
			}
		})
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i] < cells[j] })
	for id := range callers {
		sort.Slice(callers[id], func(i, j int) bool { return callers[id][i] < callers[id][j] })
	}
	domain := lattice.Lattice[Transformer]{
		Bottom:   func() Transformer { return Transformer{} },
		Top:      func() Transformer { return contextual(0, "top") },
		Equal:    transformerEqual,
		LessOrEq: func(a, b Transformer) bool { return transformerEqual(joinTransformer(a, b), b) },
		Join:     joinTransformer,
		Widen:    joinTransformer,
	}
	var solverStats *solve.Stats
	if stats != nil {
		solverStats = &stats.Solver
	}
	system := solve.EquationSystem[FunctionID, Transformer]{
		Lattice: domain,
		Cells:   cells,
		Transfer: func(id FunctionID, read func(FunctionID) Transformer, emit func(FunctionID, Transformer)) {
			def := byID[id]
			missing := ""
			checkedRead := func(callee FunctionID) Transformer {
				if _, ok := byID[callee]; !ok {
					missing = string(callee)
					return Transformer{}
				}
				return read(callee)
			}
			got := resolveDefinition(def, checkedRead)
			if missing != "" {
				got = contextual(def.Params, "unknown callee: "+missing)
			}
			emit(id, got)
		},
		Stats: solverStats,
	}
	plan := solve.NewWTOPlan(cells, func(id FunctionID) []FunctionID { return callers[id] })
	result, _, err := solve.SolveWTOContextWithVersions(ctx, system, plan)
	return result, err
}

func walkCalls(exprs []Expr, visit func(FunctionID)) {
	var walk func(Expr)
	walk = func(expr Expr) {
		if expr.n == nil {
			return
		}
		if expr.n.op == opCall {
			visit(expr.n.callee)
		}
		for _, arg := range expr.n.args {
			walk(arg)
		}
	}
	for _, expr := range exprs {
		walk(expr)
	}
}
