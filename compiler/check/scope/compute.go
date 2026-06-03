package scope

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// PointScopeGraph defines the CFG interface required for point-scope
// computation.
type PointScopeGraph interface {
	basecfg.VersionedGraph
	Assign(p cfg.Point) *cfg.AssignInfo
	FuncDef(p cfg.Point) *cfg.FuncDefInfo
	TypeDef(p cfg.Point) *cfg.TypeDefInfo
}

// PointScopeOptions controls point-scope computation limits.
type PointScopeOptions struct {
	MaxDepth      int
	DepthExceeded *bool
}

// BuildPointScopes walks the CFG in reverse postorder and computes lexical
// scope at each point. It tracks local names, block entry/exit, and type
// definitions. Value facts are owned by the product-state flow.
func BuildPointScopes(
	graph PointScopeGraph,
	base *State,
	resolver TypeDefResolver,
	opts PointScopeOptions,
) map[cfg.Point]*State {
	if graph == nil {
		return nil
	}
	if base == nil {
		base = New()
	}

	result := make(map[cfg.Point]*State)
	result[graph.Entry()] = base

	for _, p := range graph.RPO() {
		if p == graph.Entry() {
			continue
		}

		current := scopeFromPredecessor(graph, result, p, base)
		node := graph.Node(p)
		if node == nil {
			result[p] = current
			continue
		}

		switch node.Kind {
		case basecfg.NodeScopeEnter:
			current = enterScope(current, node, graph, opts)
		case basecfg.NodeScopeExit:
			if parent := current.Parent(); parent != nil {
				current = MergeScopeExit(parent, current)
			} else {
				current = base
			}
		case basecfg.NodeAssign:
			current = applyAssign(graph, p, current)
		case basecfg.NodeTypeDef:
			current = applyPointTypeDef(graph, p, current, resolver)
		}

		result[p] = current
	}

	return result
}

func scopeFromPredecessor(graph PointScopeGraph, result map[cfg.Point]*State, p cfg.Point, base *State) *State {
	preds := graph.Predecessors(p)
	if len(preds) == 0 {
		return base
	}

	merged := result[preds[0]]
	if merged == nil {
		merged = base
	}

	for i := 1; i < len(preds); i++ {
		if s, ok := result[preds[i]]; ok && s != nil {
			merged = mergeScopes(merged, s)
		}
	}
	return merged
}

func enterScope(current *State, node *basecfg.Node, graph PointScopeGraph, opts PointScopeOptions) *State {
	if current == nil {
		current = New()
	}
	if opts.MaxDepth > 0 && current.Depth()+1 > opts.MaxDepth {
		if opts.DepthExceeded != nil {
			*opts.DepthExceeded = true
		}
		return current
	}
	child := current.Child()
	if len(node.LoopLocals) > 0 && graph != nil {
		var localNames []string
		for _, sym := range node.LoopLocals {
			if name := graph.NameOf(sym); name != "" {
				localNames = append(localNames, name)
			}
		}
		if len(localNames) > 0 {
			child = child.WithLocalNames(localNames)
		}
	}
	return child
}

func mergeScopes(a, b *State) *State {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := a
	var mutated []string
	b.RangeMutations(func(name string) bool {
		if !a.IsMutated(name) {
			mutated = append(mutated, name)
		}
		return true
	})
	if len(mutated) > 0 {
		out = out.WithMutatedNames(mutated)
	}
	return out
}

func applyAssign(graph PointScopeGraph, p cfg.Point, current *State) *State {
	if funcDef := graph.FuncDef(p); funcDef != nil {
		return applyFuncDef(funcDef, current)
	}

	info := graph.Assign(p)
	if info == nil || !info.IsLocal {
		return current
	}

	var localNames []string
	info.EachTarget(func(_ int, target cfg.AssignTarget) {
		if target.Kind == cfg.TargetIdent && target.Name != "" {
			localNames = append(localNames, target.Name)
		}
	})
	if len(localNames) > 0 {
		current = current.WithLocalNames(localNames)
	}
	return current
}

func applyFuncDef(info *cfg.FuncDefInfo, current *State) *State {
	if info.Name == "" || info.FuncExpr == nil {
		return current
	}
	if info.TargetKind == cfg.FuncDefGlobal {
		return current.WithLocalName(info.Name)
	}
	return current
}

func applyPointTypeDef(graph PointScopeGraph, p cfg.Point, current *State, resolver TypeDefResolver) *State {
	info := graph.TypeDef(p)
	if info == nil || info.Name == "" || info.TypeExpr == nil || resolver == nil {
		return current
	}
	resolved := resolver(info.Name, info.TypeExpr, ToTypeParamExprs(info.TypeParams), current)
	if resolved == nil {
		resolved = typ.Unknown
	}
	if _, isGeneric := resolved.(*typ.Generic); isGeneric {
		return current.WithType(info.Name, resolved)
	}
	return current.WithType(info.Name, typ.NewAlias(info.Name, resolved))
}
