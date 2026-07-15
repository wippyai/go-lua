package visibility

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Definition describes a symbol definition at a CFG point.
type Definition struct {
	Point  cfg.Point
	Symbol symbol.ID
	Root   string
}

// BuildConfig configures forward construction of point-visible versions.
type BuildConfig struct {
	Graph       cfg.Graph
	Definitions []Definition
}

// BuildForward constructs a visibility table by propagating symbol definitions
// through the CFG. At joins, differing incoming versions for the same symbol
// are represented by a stable synthetic phi version for that point and symbol.
func BuildForward(config BuildConfig) *Table {
	graph := config.Graph
	if graph == nil {
		return &Table{}
	}
	rpo := graph.RPO()
	if len(rpo) == 0 {
		return &Table{}
	}

	rpoIndex := make(map[cfg.Point]int, len(rpo))
	for i, point := range rpo {
		rpoIndex[point] = i
	}

	definitions := normalizeDefinitions(config.Definitions, rpoIndex)
	next := make(map[symbol.ID]int)
	defsAt := make(map[cfg.Point][]ssa.Version)
	for _, def := range definitions {
		next[def.Symbol]++
		version := ssa.Version{Root: def.Root, Symbol: def.Symbol, ID: next[def.Symbol]}
		defsAt[def.Point] = append(defsAt[def.Point], version)
	}

	stateLen := pointStateLen(rpo)
	in := make([]*versionState, stateLen)
	out := make([]*versionState, stateLen)
	initializedOut := make([]bool, stateLen)
	phis := make(map[lookup]ssa.Version)

	// Seed once in RPO, then revisit only successors whose predecessor output
	// actually changed. This is the same finite monotone equation system as the
	// former whole-graph retry loop, without re-executing settled acyclic rows.
	queue := append([]cfg.Point(nil), rpo...)
	queued := make([]bool, stateLen)
	for _, point := range rpo {
		queued[point] = true
	}
	for head := 0; head < len(queue); head++ {
		point := queue[head]
		queued[point] = false
		nextIn := mergePredecessors(graph, point, out, initializedOut, phis, next)
		if versionStatesEqual(in[point], nextIn) {
			nextIn = in[point]
		} else {
			in[point] = nextIn
		}

		nextOut := versionStateWithDefinitions(nextIn, defsAt[point])
		outChanged := !versionStatesEqual(out[point], nextOut)
		if outChanged {
			out[point] = nextOut
		}
		initializedOut[point] = true
		if !outChanged {
			continue
		}
		for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
			if int(successor) >= len(queued) || queued[successor] {
				continue
			}
			queued[successor] = true
			queue = append(queue, successor)
		}
	}

	// in/out are immutable snapshots at this point. Retaining them directly is
	// both the exact lookup relation and the compact representation: unchanged
	// straight-line points share maps, and we avoid rematerializing every
	// point×symbol pair into two additional hash tables.
	return &Table{
		visibleAt: out,
		inputAt:   in,
		flowInput: true,
	}
}

func pointStateLen(points []cfg.Point) int {
	max := cfg.Point(0)
	for _, point := range points {
		if point > max {
			max = point
		}
	}
	return int(max) + 1
}

func normalizeDefinitions(definitions []Definition, rpoIndex map[cfg.Point]int) []Definition {
	if len(definitions) == 0 {
		return nil
	}
	out := make([]Definition, 0, len(definitions))
	for _, def := range definitions {
		if def.Symbol == 0 {
			continue
		}
		if _, ok := rpoIndex[def.Point]; !ok {
			continue
		}
		out = append(out, def)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := rpoIndex[out[i].Point]
		right := rpoIndex[out[j].Point]
		if left != right {
			return left < right
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out
}

func mergePredecessors(
	graph cfg.Graph,
	point cfg.Point,
	out []*versionState,
	initializedOut []bool,
	phis map[lookup]ssa.Version,
	next map[symbol.ID]int,
) *versionState {
	preds := cfg.PredecessorsReadOnly(graph, point)
	if len(preds) == 0 {
		return nil
	}
	knownPreds := make([]cfg.Point, 0, len(preds))
	for _, pred := range preds {
		if int(pred) < len(initializedOut) && initializedOut[pred] {
			knownPreds = append(knownPreds, pred)
		}
	}
	if len(knownPreds) == 0 {
		return nil
	}
	if len(knownPreds) == 1 {
		return out[knownPreds[0]]
	}

	allSame := true
	firstState := out[knownPreds[0]]
	for _, pred := range knownPreds[1:] {
		if out[pred] != firstState {
			allSame = false
			break
		}
	}
	if allSame {
		return firstState
	}

	symbols := make([]symbol.ID, 0)
	for _, pred := range knownPreds {
		out[pred].forEach(func(sym symbol.ID, _ ssa.Version) { symbols = append(symbols, sym) })
	}
	if len(symbols) == 0 {
		return nil
	}
	sort.Slice(symbols, func(i, j int) bool { return symbols[i] < symbols[j] })

	builder := versionStateBuilder{}
	for i, sym := range symbols {
		if i != 0 && symbols[i-1] == sym {
			continue
		}
		first := ssa.Version{}
		same := true
		root := ""
		for i, pred := range knownPreds {
			version := out[pred].lookup(sym)
			if root == "" {
				root = version.Root
			}
			if i == 0 {
				first = version
				continue
			}
			if !versionSemanticallyEqual(version, first) {
				same = false
			}
		}
		if same {
			if !first.IsZero() {
				builder.append(sym, first)
			}
			continue
		}
		key := lookup{point: point, symbol: sym}
		phi := phis[key]
		if phi.IsZero() {
			next[sym]++
			phi = ssa.Version{Root: root, Symbol: sym, ID: next[sym]}
			phis[key] = phi
		}
		builder.append(sym, phi)
	}
	return builder.build()
}

func versionStateWithDefinitions(
	base *versionState,
	defs []ssa.Version,
) *versionState {
	if len(defs) == 0 {
		return base
	}
	next := base
	for _, version := range defs {
		next = next.with(version)
	}
	return next
}

func versionSemanticallyEqual(left, right ssa.Version) bool {
	return left.Symbol == right.Symbol && left.ID == right.ID
}

func versionMapsEqual(left, right map[symbol.ID]ssa.Version) bool {
	if len(left) != len(right) {
		return false
	}
	for sym, leftVersion := range left {
		rightVersion, ok := right[sym]
		if !ok || !versionSemanticallyEqual(leftVersion, rightVersion) {
			return false
		}
	}
	return true
}
