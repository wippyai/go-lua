package cfg

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// Graph holds CFG with pre-extracted node info. Immutable after Build().
type Graph struct {
	cfg    *basecfg.CFG
	info   map[basecfg.Point]NodeInfo
	nested []NestedFunc
	fn     *ast.FunctionExpr

	// Binding table (AST ident -> symbol, populated before CFG build)
	bindings *bind.BindingTable

	// SSA versioning (keyed by basecfg.SymbolID for scope-aware versioning)
	phiNodes       []PhiInfo                              // Phi nodes at join points
	visibleVersion map[Point]map[basecfg.SymbolID]Version // (point, symbol) -> visible version
	nextVersionID  map[basecfg.SymbolID]int               // symbol -> next version ID

	// Scope visibility (structural, computed during build)
	symbolScope   map[Point]map[string]basecfg.SymbolID   // point -> name -> symbol
	globals       map[string]basecfg.SymbolID             // lazy globals overlay
	mergedSymbols map[Point]map[string]basecfg.SymbolID   // cached merged local+globals
	declPoints    map[basecfg.SymbolID]Point              // symbol -> declaration point
	symbolNames   map[basecfg.SymbolID]string             // symbol -> name (reverse lookup for display)
	symbolKinds   map[basecfg.SymbolID]basecfg.SymbolKind // symbol -> kind (Param/Local/Global)

	// Function parameters (precomputed for downstream use)
	paramNames      []string
	paramSymbols    []basecfg.SymbolID
	paramDeclPoints []Point
}

// Compile-time check that Graph implements VersionedGraph interface.
var _ basecfg.VersionedGraph = (*Graph)(nil)

// Build creates an immutable Graph for a function body.
// If globals is provided, those names are seeded into the binder's root scope
// with stable SymbolIDs before traversal, enabling global symbol resolution.
func Build(fn *ast.FunctionExpr, globals ...string) *Graph {
	if fn == nil {
		return nil
	}

	// Run binder pass first to bind all identifiers to symbols
	bindings := bind.Bind(fn, globals)

	return BuildWithBindings(fn, bindings)
}

// BuildWithBindings creates an immutable Graph using a pre-computed binding table.
// This allows sharing SymbolIDs across multiple function graphs within a module.
func BuildWithBindings(fn *ast.FunctionExpr, bindings *bind.BindingTable) *Graph {
	if fn == nil {
		return nil
	}

	if bindings == nil {
		return Build(fn)
	}

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	// Register globals from bindings in scope tracker at entry point
	entry := b.Cfg.Entry()

	for _, sym := range bindings.Globals() {
		name := bindings.Name(sym)
		b.ScopeTracker.RegisterGlobal(sym, name, entry)
	}

	// Register captured variables as globals so they get SSA versions.
	// This enables flow narrowing for captured variables inside nested functions.
	for _, sym := range bindings.CapturedSymbols(fn) {
		name := bindings.Name(sym)
		if name != "" && sym != 0 {
			b.ScopeTracker.RegisterGlobal(sym, name, entry)
		}
	}

	b.ScopeTracker.SnapshotVisibility(b.Current)
	b.ParamDefs(fn)
	b.Stmts(fn.Stmts)
	b.ResolvePendingGotos()

	if b.CurrentLive {
		// Add implicit return node for functions that don't end with explicit return.
		// This allows effect inference to capture constraints at implicit returns.
		implicitReturn := b.Cfg.AddNode(basecfg.NodeReturn, 0, "")
		b.Cfg.AddEdge(b.Current, implicitReturn, false)
		b.ScopeTracker.SnapshotVisibility(implicitReturn)
		b.Info[implicitReturn] = &ReturnInfo{Exprs: nil}
		b.Cfg.AddEdge(implicitReturn, b.Cfg.Exit(), false)
	}

	b.ScopeTracker.SnapshotVisibility(b.Cfg.Exit())

	// Compute SSA versions and insert phi nodes
	b.ComputeSSAVersions()

	return &Graph{
		cfg:             b.Cfg,
		info:            b.Info,
		nested:          b.Nested,
		fn:              fn,
		bindings:        bindings,
		phiNodes:        b.PhiNodes,
		visibleVersion:  b.VisibleVersion,
		nextVersionID:   b.NextVersionID,
		symbolScope:     b.StealScopeVisibility(),
		globals:         b.StealGlobals(),
		declPoints:      b.StealDeclPoints(),
		symbolNames:     b.StealSymbolNames(),
		symbolKinds:     b.StealSymbolKinds(),
		paramNames:      b.ParamNames,
		paramSymbols:    b.ParamSymbols,
		paramDeclPoints: b.ParamDeclPoints,
	}
}

// BuildBlock creates an immutable Graph for a block of statements.
// If globals is provided, those names are seeded into the binder's root scope
// with stable SymbolIDs before traversal, enabling global symbol resolution.
func BuildBlock(stmts []ast.Stmt, globals ...string) *Graph {
	// Run binder pass first to bind all identifiers to symbols
	// Wrap statements in a synthetic function for binding
	syntheticFn := &ast.FunctionExpr{Stmts: stmts}
	bindings := bind.Bind(syntheticFn, globals)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	// Register globals from bindings in scope tracker at entry point
	entry := b.Cfg.Entry()

	for _, sym := range bindings.Globals() {
		name := bindings.Name(sym)
		b.ScopeTracker.RegisterGlobal(sym, name, entry)
	}

	b.ScopeTracker.SnapshotVisibility(b.Current)
	b.Stmts(stmts)
	b.ResolvePendingGotos()

	if b.CurrentLive {
		b.Cfg.AddEdge(b.Current, b.Cfg.Exit(), false)
	}

	b.ScopeTracker.SnapshotVisibility(b.Cfg.Exit())

	// Compute SSA versions and insert phi nodes
	b.ComputeSSAVersions()

	return &Graph{
		cfg:            b.Cfg,
		info:           b.Info,
		nested:         b.Nested,
		fn:             syntheticFn,
		bindings:       bindings,
		phiNodes:       b.PhiNodes,
		visibleVersion: b.VisibleVersion,
		nextVersionID:  b.NextVersionID,
		symbolScope:    b.StealScopeVisibility(),
		globals:        b.StealGlobals(),
		declPoints:     b.StealDeclPoints(),
		symbolNames:    b.StealSymbolNames(),
		symbolKinds:    b.StealSymbolKinds(),
	}
}

// CFG returns the underlying control flow graph.
func (g *Graph) CFG() *basecfg.CFG {
	if g == nil {
		return nil
	}

	return g.cfg
}

// Func returns the root function expression for this graph, if any.
func (g *Graph) Func() *ast.FunctionExpr {
	if g == nil {
		return nil
	}

	return g.fn
}

// Info returns the node info at point p, or nil if none.
func (g *Graph) Info(p Point) NodeInfo {
	if g == nil || g.info == nil {
		return nil
	}

	return g.info[p]
}

// Assign returns AssignInfo at p, or nil if not an assign node.
func (g *Graph) Assign(p Point) *AssignInfo {
	info, _ := g.Info(p).(*AssignInfo)

	return info
}

// Call returns CallInfo at p, or nil if not a call node.
func (g *Graph) Call(p Point) *CallInfo {
	info, _ := g.Info(p).(*CallInfo)

	return info
}

// Return returns ReturnInfo at p, or nil if not a return node.
func (g *Graph) Return(p Point) *ReturnInfo {
	info, _ := g.Info(p).(*ReturnInfo)

	return info
}

// Branch returns BranchInfo at p, or nil if not a branch node.
func (g *Graph) Branch(p Point) *BranchInfo {
	info, _ := g.Info(p).(*BranchInfo)

	return info
}

// FuncDef returns FuncDefInfo at p, or nil if not a funcdef node.
func (g *Graph) FuncDef(p Point) *FuncDefInfo {
	info, _ := g.Info(p).(*FuncDefInfo)

	return info
}

// TypeDef returns TypeDefInfo at p, or nil if not a typedef node.
func (g *Graph) TypeDef(p Point) *TypeDefInfo {
	info, _ := g.Info(p).(*TypeDefInfo)

	return info
}

// Iteration helpers for bulk processing.

// EachAssign calls fn for each assignment node in point order.
func (g *Graph) EachAssign(fn func(Point, *AssignInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0)

	for p, info := range g.info {
		if _, ok := info.(*AssignInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p].(*AssignInfo))
	}
}

// AssignPoints returns all assignment points.
func (g *Graph) AssignPoints() []Point {
	if g == nil || g.info == nil {
		return nil
	}

	var points []Point

	for p, info := range g.info {
		if _, ok := info.(*AssignInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	return points
}

// EachCall calls fn for each call node in point order.
func (g *Graph) EachCall(fn func(Point, *CallInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0)

	for p, info := range g.info {
		if _, ok := info.(*CallInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p].(*CallInfo))
	}
}

// EachCallSite calls fn for each call site in point order.
//
// A call site includes:
//   - Call statement nodes (foo())
//   - Call expressions inside assignment sources (local x = foo())
//   - Call expressions inside return statements (return foo())
//
// Calls embedded in assignment/return sources are yielded in source order.
func (g *Graph) EachCallSite(fn func(Point, *CallInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0, len(g.info))

	for p := range g.info {
		points = append(points, p)
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		switch info := g.info[p].(type) {
		case *CallInfo:
			fn(p, info)
		case *AssignInfo:
			if info == nil {
				continue
			}

			for _, call := range info.SourceCalls {
				if call != nil {
					fn(p, call)
				}
			}
		case *ReturnInfo:
			if info == nil {
				continue
			}

			for _, call := range info.SourceCalls {
				if call != nil {
					fn(p, call)
				}
			}
		}
	}
}

// EachReturn calls fn for each return node in point order.
func (g *Graph) EachReturn(fn func(Point, *ReturnInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0)

	for p, info := range g.info {
		if _, ok := info.(*ReturnInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p].(*ReturnInfo))
	}
}

// EachBranch calls fn for each branch node in point order.
func (g *Graph) EachBranch(fn func(Point, *BranchInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0)

	for p, info := range g.info {
		if _, ok := info.(*BranchInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p].(*BranchInfo))
	}
}

// EachFuncDef calls fn for each function definition node in point order.
func (g *Graph) EachFuncDef(fn func(Point, *FuncDefInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0)

	for p, info := range g.info {
		if _, ok := info.(*FuncDefInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p].(*FuncDefInfo))
	}
}

// EachTypeDef calls fn for each type definition node in point order.
func (g *Graph) EachTypeDef(fn func(Point, *TypeDefInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0)

	for p, info := range g.info {
		if _, ok := info.(*TypeDefInfo); ok {
			points = append(points, p)
		}
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p].(*TypeDefInfo))
	}
}

// EachNode calls fn for each node with info in point order.
func (g *Graph) EachNode(fn func(Point, NodeInfo)) {
	if g == nil || g.info == nil {
		return
	}

	points := make([]Point, 0, len(g.info))

	for p := range g.info {
		points = append(points, p)
	}

	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	for _, p := range points {
		fn(p, g.info[p])
	}
}

// NestedFunctions returns all nested functions found during build.
func (g *Graph) NestedFunctions() []NestedFunc {
	if g == nil {
		return nil
	}

	return g.nested
}

// CFG delegated methods.

// Node returns the base CFG node at point p.
func (g *Graph) Node(p Point) *basecfg.Node {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.Node(p)
}

// ID returns the unique identifier.
func (g *Graph) ID() uint64 {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.ID()
}

// Entry returns the entry point.
func (g *Graph) Entry() Point {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.Entry()
}

// Exit returns the exit point.
func (g *Graph) Exit() Point {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.Exit()
}

// Size returns the number of nodes.
func (g *Graph) Size() int {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.Size()
}

// RPO returns nodes in reverse post-order.
func (g *Graph) RPO() []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.RPO()
}

// Predecessors returns all predecessors of p.
func (g *Graph) Predecessors(p Point) []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.Predecessors(p)
}

// Successors returns all successors of p.
func (g *Graph) Successors(p Point) []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.Successors(p)
}

// Successor returns single successor (for non-branch nodes).
func (g *Graph) Successor(p Point) Point {
	if g == nil || g.cfg == nil {
		return p
	}

	return g.cfg.Successor(p)
}

// Predecessor returns single predecessor (for non-join nodes).
func (g *Graph) Predecessor(p Point) Point {
	if g == nil || g.cfg == nil {
		return p
	}

	return g.cfg.Predecessor(p)
}

// IsJoin returns true if p has multiple predecessors.
func (g *Graph) IsJoin(p Point) bool {
	if g == nil || g.cfg == nil {
		return false
	}

	return g.cfg.IsJoin(p)
}

// IsBranch returns true if p has multiple successors.
func (g *Graph) IsBranch(p Point) bool {
	if g == nil || g.cfg == nil {
		return false
	}

	return g.cfg.IsBranch(p)
}

// EdgeCond returns the condition value for edge from->to.
func (g *Graph) EdgeCond(from, to Point) (bool, bool) {
	if g == nil || g.cfg == nil {
		return false, false
	}

	return g.cfg.EdgeCond(from, to)
}

// Edges returns all edges.
func (g *Graph) Edges() []basecfg.Edge {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.Edges()
}

// SSA versioning methods (implements cfg.SSAVersioned)

// PhiNodes returns all phi nodes in the graph.
// Implements cfg.SymbolScope.
func (g *Graph) PhiNodes() []basecfg.PhiNode {
	if g == nil {
		return nil
	}

	return g.phiNodes
}

// VisibleVersion returns the SSA version of a symbol visible at a point.
// Returns a zero Version if the symbol is not defined on all paths to this point.
// Implements cfg.SymbolScope.
func (g *Graph) VisibleVersion(p Point, sym basecfg.SymbolID) Version {
	if g == nil || g.visibleVersion == nil {
		return Version{}
	}

	if m := g.visibleVersion[p]; m != nil {
		return m[sym]
	}

	return Version{}
}

// AllVisibleVersions returns all symbol versions visible at a point.
// The returned map should not be modified.
// Implements cfg.SSAVersioned.
func (g *Graph) AllVisibleVersions(p Point) map[basecfg.SymbolID]Version {
	if g == nil || g.visibleVersion == nil {
		return nil
	}

	return g.visibleVersion[p]
}

// HasPhiAt returns true if there's a phi node at point p for the given symbol.
func (g *Graph) HasPhiAt(_ Point, sym basecfg.SymbolID) bool {
	if g == nil {
		return false
	}

	for _, phi := range g.phiNodes {
		if phi.Target.Symbol == sym {
			return true
		}
	}

	return false
}

// SymbolResolver resolves variable names to SymbolIDs at CFG points.
type SymbolResolver func(p Point, name string) basecfg.SymbolID

// PopulateSymbols fills in basecfg.SymbolID fields for all node infos using the resolver.
func (g *Graph) PopulateSymbols(resolve SymbolResolver) {
	if g == nil || g.info == nil || resolve == nil {
		return
	}

	for p, info := range g.info {
		switch v := info.(type) {
		case *AssignInfo:
			for i := range v.Targets {
				target := &v.Targets[i]
				switch target.Kind {
				case TargetIdent:
					if target.Name != "" {
						target.Symbol = resolve(p, target.Name)
					}
				case TargetField:
					if target.BaseName != "" {
						target.BaseSymbol = resolve(p, target.BaseName)
					}
				}
			}
		case *BranchInfo:
			if v.CondVar != "" {
				root := extraction.ExtractRootName(v.CondVar)
				if root != "" {
					v.CondSymbol = resolve(p, root)
				}
			}
		case *CallInfo:
			if v.CalleeName != "" {
				v.CalleeSymbol = resolve(p, v.CalleeName)
			}

			if v.ReceiverName != "" {
				v.ReceiverSymbol = resolve(p, v.ReceiverName)
			}

			if len(v.ArgNames) > 0 {
				v.ArgSymbols = make([]basecfg.SymbolID, len(v.ArgNames))
				for i, name := range v.ArgNames {
					if name != "" {
						v.ArgSymbols[i] = resolve(p, name)
					}
				}
			}
		case *ReturnInfo:
			if len(v.Names) > 0 {
				v.Symbols = make([]basecfg.SymbolID, len(v.Names))
				for i, name := range v.Names {
					if name != "" {
						v.Symbols[i] = resolve(p, name)
					}
				}
			}
		case *FuncDefInfo:
			if v.Name != "" && v.TargetKind == FuncDefGlobal {
				v.Symbol = resolve(p, v.Name)
			}
		}
	}
}

// SymbolAt returns the basecfg.SymbolID for a variable name at a specific CFG point.
// This reflects lexical scoping - the same name may resolve to different
// symbols at different points due to shadowing.
// Returns (0, false) if the name is not visible at that point.
// Implements cfg.SSAVersioned.
func (g *Graph) SymbolAt(p Point, name string) (basecfg.SymbolID, bool) {
	if g == nil {
		return 0, false
	}

	if g.symbolScope != nil {
		if vis := g.symbolScope[p]; vis != nil {
			if sym, ok := vis[name]; ok {
				return sym, true
			}
		}
	}

	if g.globals != nil {
		if sym, ok := g.globals[name]; ok {
			return sym, true
		}
	}

	return 0, false
}

// AllSymbolsAt returns all visible symbols at a CFG point.
func (g *Graph) AllSymbolsAt(p Point) map[string]basecfg.SymbolID {
	if g == nil {
		return nil
	}

	local := g.symbolScope[p]

	if len(g.globals) == 0 {
		return local
	}

	if local == nil {
		return g.globals
	}
	// Check cache
	if g.mergedSymbols != nil {
		if cached, ok := g.mergedSymbols[p]; ok {
			return cached
		}
	}
	// Merge local and globals (local shadows global)
	merged := make(map[string]basecfg.SymbolID, len(local)+len(g.globals))
	for name, sym := range g.globals {
		merged[name] = sym
	}

	for name, sym := range local {
		merged[name] = sym
	}

	// Cache result
	if g.mergedSymbols == nil {
		g.mergedSymbols = make(map[Point]map[string]basecfg.SymbolID)
	}

	g.mergedSymbols[p] = merged

	return merged
}

// DeclarationPoint returns the CFG point where a symbol was declared.
// Returns (0, false) if the symbol is unknown.
// Implements cfg.SSAVersioned.
func (g *Graph) DeclarationPoint(sym basecfg.SymbolID) (Point, bool) {
	if g == nil || g.declPoints == nil {
		return 0, false
	}

	p, ok := g.declPoints[sym]

	return p, ok
}

// NameOf returns the variable name for a symbol (for display purposes).
// Returns empty string if the symbol is unknown.
func (g *Graph) NameOf(sym basecfg.SymbolID) string {
	if g == nil || g.symbolNames == nil {
		if g == nil || g.bindings == nil {
			return ""
		}

		return g.bindings.Name(sym)
	}

	if name := g.symbolNames[sym]; name != "" {
		return name
	}

	if g.bindings != nil {
		return g.bindings.Name(sym)
	}

	return ""
}

// GlobalSymbol returns the symbol ID for a global name.
// Returns (0, false) if the name is not a known global.
func (g *Graph) GlobalSymbol(name string) (basecfg.SymbolID, bool) {
	if g == nil || g.globals == nil {
		return 0, false
	}

	sym, ok := g.globals[name]

	return sym, ok
}

// AllSymbolIDs returns a set of all symbols known to this graph.
// Useful when only symbol identity is needed (avoids AllSymbolsAt merges).
func (g *Graph) AllSymbolIDs() map[basecfg.SymbolID]bool {
	if g == nil {
		return nil
	}

	out := make(map[basecfg.SymbolID]bool, len(g.symbolNames))

	for sym := range g.symbolNames {
		out[sym] = true
	}

	if g.globals != nil {
		for _, sym := range g.globals {
			out[sym] = true
		}
	}

	return out
}

// DirectAliasSymbol returns the source symbol for a direct local alias assignment.
// Handles patterns like `local f = B` in the current graph.
// Returns 0 if the alias is ambiguous or not a direct ident assignment.
func (g *Graph) DirectAliasSymbol(targetSym basecfg.SymbolID) basecfg.SymbolID {
	if g == nil || targetSym == 0 {
		return 0
	}

	bindings := g.Bindings()

	if bindings == nil {
		return 0
	}

	var (
		sourceSym basecfg.SymbolID
		ambiguous bool
	)

	g.EachAssign(func(_ Point, info *AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}

		for i, target := range info.Targets {
			if target.Symbol != targetSym || i >= len(info.Sources) {
				continue
			}

			srcIdent, ok := info.Sources[i].(*ast.IdentExpr)
			if !ok || srcIdent == nil {
				ambiguous = true

				return
			}

			sym, ok := bindings.SymbolOf(srcIdent)
			if !ok || sym == 0 {
				ambiguous = true

				return
			}

			if sourceSym == 0 {
				sourceSym = sym
			} else if sourceSym != sym {
				ambiguous = true

				return
			}
		}
	})

	if ambiguous {
		return 0
	}

	return sourceSym
}

// SymbolKind returns the kind of a symbol (Param, Local, or Global).
// Returns (SymbolUnknown, false) if the symbol is not known.
func (g *Graph) SymbolKind(sym basecfg.SymbolID) (basecfg.SymbolKind, bool) {
	if g == nil || g.symbolKinds == nil {
		return basecfg.SymbolUnknown, false
	}

	kind, ok := g.symbolKinds[sym]

	return kind, ok
}

// HasScopeTracking returns true if scope visibility was computed during build.
func (g *Graph) HasScopeTracking() bool {
	return g != nil && g.symbolScope != nil
}

// Bindings returns the binding table populated during binder pass.
func (g *Graph) Bindings() *bind.BindingTable {
	if g == nil {
		return nil
	}

	return g.bindings
}

// ParamNames returns a copy of the function parameter names.
// Returns nil for block graphs or functions with no parameters.
func (g *Graph) ParamNames() []string {
	if g == nil || len(g.paramNames) == 0 {
		return nil
	}

	result := make([]string, len(g.paramNames))
	copy(result, g.paramNames)

	return result
}

// ParamSymbols returns a copy of the function parameter symbol IDs.
// Returns nil for block graphs or functions with no parameters.
func (g *Graph) ParamSymbols() []basecfg.SymbolID {
	if g == nil || len(g.paramSymbols) == 0 {
		return nil
	}

	result := make([]basecfg.SymbolID, len(g.paramSymbols))
	copy(result, g.paramSymbols)

	return result
}

// ParamDeclPoints returns a copy of the CFG points where parameters are declared.
// Returns nil for block graphs or functions with no parameters.
func (g *Graph) ParamDeclPoints() []Point {
	if g == nil || len(g.paramDeclPoints) == 0 {
		return nil
	}

	result := make([]Point, len(g.paramDeclPoints))
	copy(result, g.paramDeclPoints)

	return result
}

// CalleePathAt returns the callee path for a call at point p.
// Returns empty path if p is not a call node.
func (g *Graph) CalleePathAt(p Point) constraint.Path {
	if g == nil {
		return constraint.Path{}
	}

	if info := g.Call(p); info != nil {
		return info.CalleePath
	}

	return constraint.Path{}
}

// FuncDefPathAt returns the target path for a function definition at point p.
// Returns empty path if p is not a function definition node.
func (g *Graph) FuncDefPathAt(p Point) constraint.Path {
	if g == nil {
		return constraint.Path{}
	}

	if info := g.FuncDef(p); info != nil {
		return info.TargetPath
	}
	return constraint.Path{}
}
