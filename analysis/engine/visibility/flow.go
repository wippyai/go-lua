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

	in := make(map[cfg.Point]map[symbol.ID]ssa.Version, len(rpo))
	out := make(map[cfg.Point]map[symbol.ID]ssa.Version, len(rpo))
	initializedOut := make(map[cfg.Point]struct{}, len(rpo))
	phis := make(map[lookup]ssa.Version)
	table := &Table{}

	changed := true
	for changed {
		changed = false
		for _, point := range rpo {
			nextIn := mergePredecessors(graph, point, out, initializedOut, phis, next)
			if !versionMapsEqual(in[point], nextIn) {
				in[point] = nextIn
				changed = true
			}

			nextOut := versionMapWithDefinitions(nextIn, defsAt[point])
			if !versionMapsEqual(out[point], nextOut) {
				out[point] = nextOut
				changed = true
			}
			initializedOut[point] = struct{}{}
		}
	}

	for _, point := range rpo {
		for sym, version := range out[point] {
			table.set(point, sym, version)
		}
	}
	return table
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
	out map[cfg.Point]map[symbol.ID]ssa.Version,
	initializedOut map[cfg.Point]struct{},
	phis map[lookup]ssa.Version,
	next map[symbol.ID]int,
) map[symbol.ID]ssa.Version {
	preds := graph.Predecessors(point)
	if len(preds) == 0 {
		return nil
	}
	knownPreds := make([]cfg.Point, 0, len(preds))
	for _, pred := range preds {
		if _, ok := initializedOut[pred]; ok {
			knownPreds = append(knownPreds, pred)
		}
	}
	if len(knownPreds) == 0 {
		return nil
	}
	if len(knownPreds) == 1 {
		return out[knownPreds[0]]
	}

	symbols := make(map[symbol.ID]struct{})
	for _, pred := range knownPreds {
		for sym := range out[pred] {
			symbols[sym] = struct{}{}
		}
	}
	if len(symbols) == 0 {
		return nil
	}

	merged := make(map[symbol.ID]ssa.Version, len(symbols))
	for sym := range symbols {
		first := ssa.Version{}
		same := true
		root := ""
		for i, pred := range knownPreds {
			version := out[pred][sym]
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
				merged[sym] = first
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
		merged[sym] = phi
	}
	return merged
}

func versionMapWithDefinitions(
	base map[symbol.ID]ssa.Version,
	defs []ssa.Version,
) map[symbol.ID]ssa.Version {
	if len(defs) == 0 {
		return base
	}
	next := cloneVersionMap(base)
	if next == nil {
		next = make(map[symbol.ID]ssa.Version, len(defs))
	}
	for _, version := range defs {
		next[version.Symbol] = version
	}
	return next
}

func versionSemanticallyEqual(left, right ssa.Version) bool {
	return left.Symbol == right.Symbol && left.ID == right.ID
}

func cloneVersionMap(in map[symbol.ID]ssa.Version) map[symbol.ID]ssa.Version {
	if len(in) == 0 {
		return nil
	}
	out := make(map[symbol.ID]ssa.Version, len(in))
	for sym, version := range in {
		out[sym] = version
	}
	return out
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
