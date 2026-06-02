package topology

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
)

// FunctionDiscoveryInput is the canonical module-function topology input.
type FunctionDiscoveryInput struct {
	Root         *cfg.Graph
	GraphForFunc func(*ast.FunctionExpr) *cfg.Graph
}

// FunctionTopology is the immutable topology of module-local functions. It owns
// discovery order, graph/function identity, lexical parent edges, and method
// definition ownership; solver layers consume this carrier instead of rebuilding
// ad hoc graph walks.
type FunctionTopology struct {
	refs             []ref.FuncRef
	graphs           map[ref.FuncRef]*cfg.Graph
	graphRefs        map[uint64]ref.FuncRef
	funcs            map[ref.FuncRef]*ast.FunctionExpr
	funcRefs         map[*ast.FunctionExpr]ref.FuncRef
	funcRefsBySymbol map[cfg.SymbolID]ref.FuncRef
	methodDefs       map[ref.FuncRef]*cfg.FuncDefInfo
	nestedRefs       map[ref.FuncRef][]ref.FuncRef
	parentRefs       map[ref.FuncRef]ref.FuncRef
}

// FunctionEntry is a normalized function-topology row. It is useful for tests
// and future cached topology sources that already have graph/function identities
// instead of discovering them from AST/CFG traversal.
type FunctionEntry struct {
	Ref       ref.FuncRef
	Graph     *cfg.Graph
	Function  *ast.FunctionExpr
	Symbols   []cfg.SymbolID
	MethodDef *cfg.FuncDefInfo
	Parent    ref.FuncRef
}

// NewFunctionTopology builds an immutable topology from normalized rows.
func NewFunctionTopology(entries []FunctionEntry) FunctionTopology {
	out := newFunctionTopology()
	seen := make(map[ref.FuncRef]bool)
	for _, entry := range entries {
		fnRef := entry.Ref
		if fnRef == (ref.FuncRef{}) && entry.Graph != nil {
			fnRef = ref.FuncRef{GraphID: entry.Graph.ID()}
		}
		if fnRef == (ref.FuncRef{}) {
			continue
		}
		if !seen[fnRef] {
			out.refs = append(out.refs, fnRef)
			seen[fnRef] = true
		}
		if entry.Graph != nil {
			out.graphs[fnRef] = entry.Graph
			out.graphRefs[entry.Graph.ID()] = fnRef
		}
		if entry.Function != nil {
			out.funcs[fnRef] = entry.Function
			out.funcRefs[entry.Function] = fnRef
		}
		for _, sym := range entry.Symbols {
			if sym != 0 {
				out.funcRefsBySymbol[sym] = fnRef
			}
		}
		if entry.MethodDef != nil {
			out.methodDefs[fnRef] = entry.MethodDef
			if entry.MethodDef.FuncExpr != nil {
				out.funcs[fnRef] = entry.MethodDef.FuncExpr
				out.funcRefs[entry.MethodDef.FuncExpr] = fnRef
			}
		}
		if entry.Parent != (ref.FuncRef{}) {
			out.parentRefs[fnRef] = entry.Parent
			out.nestedRefs[entry.Parent] = append(out.nestedRefs[entry.Parent], fnRef)
		}
	}
	return out
}

// DiscoverFunctions walks the transitive nested-function hierarchy in
// deterministic BFS order and returns the finite function topology the canonical
// solver ranges over.
func DiscoverFunctions(in FunctionDiscoveryInput) FunctionTopology {
	out := newFunctionTopology()
	if in.Root == nil {
		return out
	}

	methodDefExprs := make(map[*ast.FunctionExpr]*cfg.FuncDefInfo)
	WalkHierarchy(HierarchyInput{Root: in.Root, GraphForFunc: in.GraphForFunc}, func(node HierarchyNode) {
		g := node.Graph
		if g == nil {
			return
		}
		curRef := out.addGraph(g)
		g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
			if methodDefOwnsBody(info) {
				methodDefExprs[info.FuncExpr] = info
			}
		})
		for _, nested := range g.NestedFunctions() {
			if nested.Func == nil || in.GraphForFunc == nil {
				continue
			}
			ng := in.GraphForFunc(nested.Func)
			if ng == nil {
				continue
			}
			childRef := ref.FuncRef{GraphID: ng.ID()}
			if info := methodDefExprs[nested.Func]; info != nil {
				out.methodDefs[childRef] = info
			}
			if _, hasParent := out.parentRefs[childRef]; g.Func() != nil && !hasParent {
				out.nestedRefs[curRef] = append(out.nestedRefs[curRef], childRef)
				out.parentRefs[childRef] = curRef
			}
		}
	})
	for fn, info := range methodDefExprs {
		if fnRef, ok := out.RefForFunction(fn); ok {
			out.methodDefs[fnRef] = info
		}
	}
	return out
}

func newFunctionTopology() FunctionTopology {
	return FunctionTopology{
		graphs:           make(map[ref.FuncRef]*cfg.Graph),
		graphRefs:        make(map[uint64]ref.FuncRef),
		funcs:            make(map[ref.FuncRef]*ast.FunctionExpr),
		funcRefs:         make(map[*ast.FunctionExpr]ref.FuncRef),
		funcRefsBySymbol: make(map[cfg.SymbolID]ref.FuncRef),
		methodDefs:       make(map[ref.FuncRef]*cfg.FuncDefInfo),
		nestedRefs:       make(map[ref.FuncRef][]ref.FuncRef),
		parentRefs:       make(map[ref.FuncRef]ref.FuncRef),
	}
}

func methodDefOwnsBody(info *cfg.FuncDefInfo) bool {
	return info != nil &&
		info.Name != "" &&
		info.FuncExpr != nil &&
		(info.TargetKind == cfg.FuncDefMethod || info.TargetKind == cfg.FuncDefField) &&
		info.Receiver != nil
}

func (t *FunctionTopology) addGraph(g *cfg.Graph) ref.FuncRef {
	fnRef := ref.FuncRef{GraphID: g.ID()}
	if _, exists := t.graphs[fnRef]; exists {
		return fnRef
	}
	t.refs = append(t.refs, fnRef)
	t.graphs[fnRef] = g
	t.graphRefs[g.ID()] = fnRef
	if fn := g.Func(); fn != nil {
		t.funcs[fnRef] = fn
		t.funcRefs[fn] = fnRef
		if bindings := g.Bindings(); bindings != nil {
			if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
				t.funcRefsBySymbol[sym] = fnRef
			}
		}
	}
	return fnRef
}

// Refs returns module functions in deterministic discovery order.
func (t FunctionTopology) Refs() []ref.FuncRef {
	return append([]ref.FuncRef(nil), t.refs...)
}

// Graph returns the CFG for ref.
func (t FunctionTopology) Graph(fnRef ref.FuncRef) *cfg.Graph {
	return t.graphs[fnRef]
}

// RefForGraph resolves a graph to its canonical function identity by graph ID.
func (t FunctionTopology) RefForGraph(g *cfg.Graph) (ref.FuncRef, bool) {
	if g == nil || t.graphRefs == nil {
		return ref.FuncRef{}, false
	}
	fnRef, ok := t.graphRefs[g.ID()]
	return fnRef, ok
}

// Function returns the source function literal for ref.
func (t FunctionTopology) Function(fnRef ref.FuncRef) *ast.FunctionExpr {
	return t.funcs[fnRef]
}

// RefForFunction resolves a source function literal to its function identity.
func (t FunctionTopology) RefForFunction(fn *ast.FunctionExpr) (ref.FuncRef, bool) {
	if fn == nil || t.funcRefs == nil {
		return ref.FuncRef{}, false
	}
	fnRef, ok := t.funcRefs[fn]
	return fnRef, ok
}

// RefForSymbol resolves a CFG symbol that denotes a module-local function.
func (t FunctionTopology) RefForSymbol(sym cfg.SymbolID) (ref.FuncRef, bool) {
	if sym == 0 || t.funcRefsBySymbol == nil {
		return ref.FuncRef{}, false
	}
	fnRef, ok := t.funcRefsBySymbol[sym]
	return fnRef, ok
}

// MethodDef returns the method/field definition that owns ref's body, if any.
func (t FunctionTopology) MethodDef(fnRef ref.FuncRef) *cfg.FuncDefInfo {
	return t.methodDefs[fnRef]
}

// NestedRefs returns direct lexical children of ref in deterministic order.
func (t FunctionTopology) NestedRefs(fnRef ref.FuncRef) []ref.FuncRef {
	return append([]ref.FuncRef(nil), t.nestedRefs[fnRef]...)
}

// ParentRef returns ref's direct lexical parent.
func (t FunctionTopology) ParentRef(fnRef ref.FuncRef) (ref.FuncRef, bool) {
	parent, ok := t.parentRefs[fnRef]
	return parent, ok
}

// ParentChain returns ref's lexical ancestors from nearest to outermost.
func (t FunctionTopology) ParentChain(fnRef ref.FuncRef) []ref.FuncRef {
	var out []ref.FuncRef
	seen := make(map[ref.FuncRef]bool)
	for cur, ok := t.ParentRef(fnRef); ok; cur, ok = t.ParentRef(cur) {
		if seen[cur] {
			break
		}
		seen[cur] = true
		out = append(out, cur)
	}
	return out
}
