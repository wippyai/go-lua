// session.go defines the Session type that holds per-run state for type checking.
// Session is the primary interface for accessing analysis results. SessionStore
// lives in compiler/check/store and manages fixpoint iteration state and interproc
// snapshots.
//
// # LIFECYCLE SEPARATION
//
// SessionStore is structured into three lifecycle-scoped components (see store package):
//
//   - ModuleStore: Immutable module-wide state created once at check start.
//     Contains binding tables, CFG graphs, and module aliases. Never modified
//     during fixpoint iteration.
//
//   - IterationStore: Iteration-local state used during fixpoint convergence
//     (revision counter and constructor field collection).
//
//   - IterationScratch: Single-iteration state cleared at each boundary.
//     Tracks which literals have been analyzed, pending parameter hints,
//     and change detection flags.
//
// # SNAPSHOT PROTOCOL
//
// Interproc facts and effects follow a snapshot protocol:
//
//   - During iteration: Functions read from the previous snapshot
//   - At boundary: New facts/effects replace the snapshot
//   - Convergence: Iteration stops when snapshots are unchanged
//
// This protocol ensures all functions within an iteration see consistent
// cross-function information, enabling deterministic analysis regardless
// of function processing order.
//
// # PARALLELIZATION
//
// Functions within a single fixpoint iteration are independent (read-only inputs).
// To parallelize: run with per-worker db.QueryContext, merge results into next
// maps deterministically at iteration boundary. db.QueryContext is NOT safe for
// concurrent access from multiple goroutines.
package check

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"sort"
	"strings"
)

// Session holds all state and results for analyzing a single Lua module.
// One Session is created per Check call and contains the complete analysis output
// including per-function results, diagnostics, and inter-function channel data.
//
// USAGE PATTERN:
//
//	sess := checker.Check(source, "module.lua")
//	for _, diag := range sess.Diagnostics {
//	    fmt.Println(diag)
//	}
//	exportType := sess.ExportType()
//
// CONCURRENCY: db.QueryContext is NOT safe for concurrent access. For parallel
// analysis, create one Session per worker with independent QueryContexts, then
// merge results. Functions within a single fixpoint iteration read from shared
// snapshots and write to independent per-iteration maps, enabling future parallelization.
//
// MEMORY MANAGEMENT: Call Release() after extracting Manifest data to free heavy
// allocations (CFGs, scopes, flow data). The Session remains valid for Diagnostics
// access after Release.
type Session struct {
	// Ctx provides query infrastructure for memoization. NOT concurrency-safe.
	Ctx *db.QueryContext

	// SourceName identifies the module for diagnostic reporting and path resolution.
	SourceName string

	// Imports tracks loaded module manifests keyed by require path.
	Imports map[string]*io.Manifest

	// Store holds all inter-function channel state. Attached to Ctx for query access.
	Store *store.SessionStore

	// Results contains per-function analysis output keyed by FunctionExpr pointer.
	// Includes root function and all nested functions.
	Results map[*ast.FunctionExpr]*api.FuncResult

	// RootFunc is the synthetic FunctionExpr wrapping the module chunk.
	// Its body contains the module's top-level statements.
	RootFunc *ast.FunctionExpr

	// RootResult provides direct access to the root function's analysis output.
	// Equivalent to Results[RootFunc] but avoids map lookup.
	RootResult *api.FuncResult

	// Diagnostics accumulates type errors, warnings, and suggestions from all phases.
	// Sorted by position after analysis completes.
	Diagnostics []diag.Diagnostic

	// Terminators tracks which functions are known to never return (error, assert(false)).
	Terminators map[string]bool

	// ManifestVars captures @manifest annotations for module metadata export.
	ManifestVars map[string]string

	// cfgCache caches built CFG graphs to avoid rebuilding for the same function.
	cfgCache map[*ast.FunctionExpr]*cfg.Graph

	// pluginStore provides extension storage for analysis plugins.
	pluginStore map[any]any

	// scopeDepthDiagEmitted tracks diagnostics for scope depth limit per function.
	scopeDepthDiagEmitted map[*ast.FunctionExpr]bool
}

// Source returns the session source name.
func (s *Session) Source() string {
	if s == nil {
		return ""
	}
	return s.SourceName
}

// Context returns the query context for the session.
func (s *Session) Context() *db.QueryContext {
	if s == nil {
		return nil
	}
	return s.Ctx
}

// StoreHandle returns the session store as an interation-capable interface.
func (s *Session) StoreHandle() api.IterationStore {
	if s == nil {
		return nil
	}
	return s.Store
}

// ResultsMap returns the function result map.
func (s *Session) ResultsMap() map[*ast.FunctionExpr]*api.FuncResult {
	if s == nil {
		return nil
	}
	return s.Results
}

// RootFuncNode returns the root function for this session.
func (s *Session) RootFuncNode() *ast.FunctionExpr {
	if s == nil {
		return nil
	}
	return s.RootFunc
}

// SetRootFuncNode sets the root function for this session.
func (s *Session) SetRootFuncNode(fn *ast.FunctionExpr) {
	if s == nil {
		return
	}
	s.RootFunc = fn
}

// RootResultValue returns the root function result.
func (s *Session) RootResultValue() *api.FuncResult {
	if s == nil {
		return nil
	}
	return s.RootResult
}

// SetRootResultValue sets the root function result.
func (s *Session) SetRootResultValue(result *api.FuncResult) {
	if s == nil {
		return
	}
	s.RootResult = result
}

// ResetDiagnostics clears all diagnostics.
func (s *Session) ResetDiagnostics() {
	if s == nil {
		return
	}
	s.Diagnostics = nil
}

// AppendDiagnostics appends diagnostics to the session.
func (s *Session) AppendDiagnostics(diags ...diag.Diagnostic) {
	if s == nil || len(diags) == 0 {
		return
	}
	s.Diagnostics = append(s.Diagnostics, diags...)
}

// DiagnosticsSlice returns the current diagnostics slice.
func (s *Session) DiagnosticsSlice() []diag.Diagnostic {
	if s == nil {
		return nil
	}
	return s.Diagnostics
}

// ScopeDepthDiagState returns the map tracking scope depth diagnostics.
func (s *Session) ScopeDepthDiagState() map[*ast.FunctionExpr]bool {
	if s == nil {
		return nil
	}
	if s.scopeDepthDiagEmitted == nil {
		s.scopeDepthDiagEmitted = make(map[*ast.FunctionExpr]bool)
	}
	return s.scopeDepthDiagEmitted
}

// New creates a session for checking a file.
func New(ctx *db.QueryContext, name string) *Session {
	store := store.NewSessionStore()
	api.AttachStore(ctx, store)
	sess := &Session{
		Ctx:                   ctx,
		SourceName:            name,
		Store:                 store,
		Imports:               make(map[string]*io.Manifest),
		Results:               make(map[*ast.FunctionExpr]*api.FuncResult),
		Terminators:           make(map[string]bool),
		ManifestVars:          make(map[string]string),
		cfgCache:              make(map[*ast.FunctionExpr]*cfg.Graph),
		scopeDepthDiagEmitted: make(map[*ast.FunctionExpr]bool),
	}
	api.AttachGraphs(ctx, sess)
	return sess
}

// GetOrBuildCFG returns a cached CFG for the function or builds and caches a new one.
func (s *Session) GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if g := s.cfgCache[fn]; g != nil {
		return g
	}
	g := cfg.BuildWithBindings(fn, s.Store.ModuleBindings())
	s.cfgCache[fn] = g
	return g
}

// RegisterGraphHierarchy registers the root graph and all nested graphs.
// This populates graph/function maps and nested metadata for query lookups.
func (s *Session) RegisterGraphHierarchy(root *cfg.Graph) {
	if s == nil || root == nil || s.Store == nil {
		return
	}
	visited := make(map[uint64]bool)
	var walk func(g *cfg.Graph)
	walk = func(g *cfg.Graph) {
		if g == nil {
			return
		}
		id := g.ID()
		if id == 0 || visited[id] {
			return
		}
		visited[id] = true
		if fn := g.Func(); fn != nil {
			s.Store.RegisterGraph(g, fn)
			if bindings := g.Bindings(); bindings != nil {
				if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
					s.Store.RegisterFunctionRef(sym, fn, g, 0, 0)
				}
			}
		}
		// Register the local functions of this graph.
		g.EachLocalFunction(func(p cfg.Point, sym cfg.SymbolID, fnExpr *ast.FunctionExpr) {
			child := s.GetOrBuildCFG(fnExpr)
			if child == nil {
				return
			}
			s.Store.RegisterGraph(child, fnExpr)
			s.Store.RegisterNestedMeta(child.ID(), g.ID(), p)
			s.Store.RegisterFunctionRef(sym, fnExpr, child, g.ID(), p)
		})
		for _, nf := range g.NestedFunctions() {
			if nf.Func == nil {
				continue
			}
			child := s.GetOrBuildCFG(nf.Func)
			if child == nil {
				continue
			}
			s.Store.RegisterGraph(child, nf.Func)
			s.Store.RegisterNestedMeta(child.ID(), g.ID(), nf.Point)
			nestedSym := nf.Symbol
			if nestedSym == 0 && child.Bindings() != nil {
				if sym, ok := child.Bindings().FuncLitSymbol(nf.Func); ok {
					nestedSym = sym
				}
			}
			if nestedSym != 0 {
				s.Store.RegisterFunctionRef(nestedSym, nf.Func, child, g.ID(), nf.Point)
			}
			walk(child)
		}
	}
	walk(root)
}

// PluginStore stores a value for a plugin extension.
func (s *Session) PluginStore(key, val any) {
	if s == nil {
		return
	}
	if s.pluginStore == nil {
		s.pluginStore = make(map[any]any)
	}
	s.pluginStore[key] = val
}

// PluginLoad retrieves a value stored by a plugin extension.
func (s *Session) PluginLoad(key any) any {
	if s == nil || s.pluginStore == nil {
		return nil
	}
	return s.pluginStore[key]
}

// ExportType computes the module's exported type from return statements.
// This is the primary method for extracting the module's public interface for
// manifest generation and import resolution.
//
// RETURN TYPE COMPUTATION:
// The export type is computed from all return statements in the root function:
//   - Multiple returns are joined into a union type
//   - Unreachable returns (dead code) are excluded via flow analysis
//   - Empty returns contribute nil to the union
//
// EFFECT ENRICHMENT:
// If the returned value is a table/record, its method types are enriched with
// function refinements computed during analysis. This ensures exported functions
// carry their refinement summaries.
//
// USAGE:
//
//	sess := checker.Check(source, "module.lua")
//	exportType := sess.ExportType()
//	manifest := io.NewManifest(modulePath, exportType, sess.ExportTypes())
func (s *Session) ExportType() typ.Type {
	if s == nil {
		return typ.Nil
	}
	var refinements map[cfg.SymbolID]*constraint.FunctionRefinement
	if s.Store != nil {
		if s.Store.InterprocPrev != nil {
			refinements = s.Store.InterprocPrev.Refinements
		}
	}
	return modules.ExportType(s.RootResult, refinements)
}

// Release frees heavy allocations to reduce memory pressure after analysis.
// Call this after extracting all needed data (ExportType, ExportTypes, Diagnostics).
//
// WHAT IS FREED:
//   - CFG graphs and binding tables
//   - Scope states and flow solutions
//   - Inter-function channel data
//   - Synthesis engines
//
// WHAT REMAINS VALID:
//   - Diagnostics slice (still accessible)
//   - Session struct itself
//
// WHEN TO CALL: After manifest generation in batch processing scenarios.
// For single-file analysis, releasing is optional as Session is GC'd normally.
func (s *Session) Release() {
	if s == nil {
		return
	}

	// Clear store maps
	if s.Store != nil {
		// Clear module store
		if s.Store.Module != nil {
			s.Store.Module.ModuleBindings = nil
			clear(s.Store.Module.Graphs)
			clear(s.Store.Module.Funcs)
			clear(s.Store.Module.GraphToFunc)
			clear(s.Store.Module.Parents)
			clear(s.Store.Module.ModuleAliases)
		}

		// Clear interproc snapshots
		if s.Store.InterprocPrev != nil {
			clear(s.Store.InterprocPrev.Facts)
			clear(s.Store.InterprocPrev.Refinements)
			clear(s.Store.InterprocPrev.ConstructorFields)
		}
		if s.Store.InterprocNext != nil {
			clear(s.Store.InterprocNext.Facts)
			clear(s.Store.InterprocNext.Refinements)
			clear(s.Store.InterprocNext.ConstructorFields)
		}

		// Clear iteration scratch (empty placeholder)
		_ = s.Store.Scratch
	}

	// Clear per-function results
	for _, result := range s.Results {
		if result != nil {
			result.Graph = nil
			result.BaseScope = nil
			result.Scopes = nil
			result.Facts = nil
			result.FlowInputs = nil
			result.FlowSolution = nil
			result.NarrowSynth = nil
		}
	}
	clear(s.Results)
	clear(s.cfgCache)

	s.RootFunc = nil
	s.RootResult = nil
	s.pluginStore = nil
	clear(s.scopeDepthDiagEmitted)
}

// ExportTypes extracts module-local type definitions for manifest generation.
// Returns a map of type names to their resolved types.
//
// INCLUDED: Types defined via @type or type alias syntax in this module.
// EXCLUDED: Standard library types, imported types from other modules.
//
// The returned map is used to populate Manifest.Types for import resolution.
// When another module imports this one, these types become available in
// the importer's type namespace.
func (s *Session) ExportTypes() map[string]typ.Type {
	if s == nil {
		return nil
	}
	return modules.ExportTypes(s.RootResult)
}

// ExportManifest builds a module manifest from this session using canonical export policy.
//
// The manifest includes:
//   - Export type from root returns (with converged function refinements applied)
//   - Exported type definitions
//   - Function summaries for exported functions (ensures/effects for cross-module narrowing)
//
// Use this helper instead of manually stitching ExportType/ExportTypes so callers do not
// accidentally omit function summaries.
func (s *Session) ExportManifest(modulePath string) *io.Manifest {
	if s == nil {
		return nil
	}
	if modulePath == "" {
		modulePath = s.SourceName
	}

	manifest := io.NewManifest(modulePath)
	manifest.BodyBacked = true
	exportType := s.ExportType()
	manifest.SetExport(exportType)

	for typeName, t := range s.ExportTypes() {
		manifest.DefineType(typeName, t)
	}

	modules.ExportFunctionSummaries(manifest, exportType, s.RootGraph(), s.RefinementsForExport())
	s.exportModuleCallWrites(manifest)
	s.exportPossibleModuleCallWrites(manifest)
	s.exportTruthyCallbackCalls(manifest)
	manifest.ArgumentContracts = s.exportArgumentContracts()
	return manifest
}

// exportPossibleModuleCallWrites follows local calls and imported calls made by
// each exported function. The summary describes types that may be written; it
// does not claim the write occurs on every path through the function.
func (s *Session) exportPossibleModuleCallWrites(manifest *io.Manifest) {
	if s == nil || s.Store == nil || manifest == nil || s.RootGraph() == nil {
		return
	}
	rec, ok := manifest.Export.(*typ.Record)
	if !ok {
		return
	}
	root := s.RootGraph()
	returnedTables := make(map[cfg.SymbolID]bool)
	root.EachReturn(func(_ cfg.Point, ret *cfg.ReturnInfo) {
		if ret != nil && len(ret.Symbols) > 0 && ret.Symbols[0] != 0 {
			returnedTables[ret.Symbols[0]] = true
		}
	})
	seenExports := make(map[string]bool)
	addExport := func(name string, sym cfg.SymbolID) {
		if name == "" || sym == 0 || seenExports[name] {
			return
		}
		field := rec.GetField(name)
		if field == nil {
			return
		}
		if _, ok := field.Type.(*typ.Function); !ok {
			return
		}
		seenExports[name] = true
		writes := make(map[string]io.ModuleWrite)
		s.collectPossibleModuleCallWrites(sym, make(map[cfg.SymbolID]bool), writes)
		if len(writes) == 0 {
			return
		}
		keys := make([]string, 0, len(writes))
		for key := range writes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if manifest.MayCallWrites == nil {
			manifest.MayCallWrites = make(map[string][]io.ModuleWrite)
		}
		for _, key := range keys {
			manifest.MayCallWrites[name] = append(manifest.MayCallWrites[name], writes[key])
		}
	}
	for _, nested := range root.NestedFunctions() {
		name := root.NameOf(nested.Symbol)
		if _, field, hasRoot := strings.Cut(name, "."); hasRoot {
			if strings.Contains(field, ".") {
				continue
			}
			name = field
		}
		addExport(name, nested.Symbol)
	}
	// A local function may be exported under another field name by assignment.
	// The source symbol identifies the same checked function body.
	root.EachAssign(func(_ cfg.Point, assign *cfg.AssignInfo) {
		if assign == nil {
			return
		}
		for i, target := range assign.Targets {
			if target.Kind != cfg.TargetField || len(target.FieldPath) != 1 ||
				!returnedTables[target.BaseSymbol] || i >= len(assign.SourceSymbols) {
				continue
			}
			sym := assign.SourceSymbols[i]
			if s.Store.FunctionRefBySym(sym) != nil {
				addExport(target.FieldPath[0], sym)
			}
		}
	})
}

func (s *Session) collectPossibleModuleCallWrites(fn cfg.SymbolID, seen map[cfg.SymbolID]bool, writes map[string]io.ModuleWrite) {
	if fn == 0 || seen[fn] {
		return
	}
	seen[fn] = true
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return
	}
	graph := s.Store.Graphs()[ref.GraphID]
	if graph == nil {
		return
	}
	bindings := graph.Bindings()
	aliases := modules.MergeAliases(s.Store.ModuleAliases(), modules.CollectAliases(graph))
	add := func(write io.ModuleWrite) {
		if write.Module == "" || write.Field == "" || write.Type == nil {
			return
		}
		key := write.Module + "\x00" + write.Path + "\x00" + write.Field
		if prior, ok := writes[key]; ok {
			write.Type = api.JoinFieldWrite(api.FieldWriteKey{Path: write.Path, Field: write.Field}, prior.Type, write.Type)
		}
		writes[key] = write
	}
	source := returns.StoreFieldWriteSource{Store: s.Store, Bindings: s.Store.ModuleBindings()}
	direct := returns.DirectFieldWriteKeys(graph)
	for target, set := range source.FieldWritesOf(fn) {
		module := aliases[target]
		if module == "" || (bindings != nil && bindings.IsReassigned(target)) ||
			(s.Store.ModuleBindings() != nil && s.Store.ModuleBindings().IsReassigned(target)) {
			continue
		}
		for key, t := range set {
			if !direct[target][key] {
				continue
			}
			add(io.ModuleWrite{Module: module, Path: key.Path, Field: key.Field, Type: t})
		}
	}
	reachable := graph.CFG().Reachable()
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || !reachable[p] {
			return
		}
		if len(info.CalleePath.Segments) == 1 {
			segment := info.CalleePath.Segments[0]
			if segment.Kind == constraint.SegmentField || segment.Kind == constraint.SegmentIndexString {
				moduleSym := info.CalleePath.Symbol
				module := aliases[moduleSym]
				if module != "" && (bindings == nil || !bindings.IsReassigned(moduleSym)) &&
					(s.Store.ModuleBindings() == nil || !s.Store.ModuleBindings().IsReassigned(moduleSym)) &&
					!modules.AssignedModuleField(graph, moduleSym, segment.Name) {
					if imported := s.Ctx.DB().Manifest(module); imported != nil && imported.BodyBacked {
						for _, write := range imported.CallWrites[segment.Name] {
							add(write)
						}
						for _, write := range imported.MayCallWrites[segment.Name] {
							add(write)
						}
					}
				}
			}
		}
		callee := callsite.SelectPreferredSymbol(
			callsite.CallableCalleeSymbolCandidates(info, graph, bindings, s.Store.ModuleBindings()),
			func(sym cfg.SymbolID) bool { return s.Store.FunctionRefBySym(sym) != nil },
		)
		if callee != 0 {
			s.collectPossibleModuleCallWrites(callee, seen, writes)
		}
	})
}

// exportModuleCallWrites preserves effects on another imported module's table
// without applying them to the module's export before the function is called.
func (s *Session) exportModuleCallWrites(manifest *io.Manifest) {
	if s == nil || s.Store == nil || manifest == nil || s.RootGraph() == nil {
		return
	}
	rec, ok := manifest.Export.(*typ.Record)
	if !ok {
		return
	}
	graph := s.RootGraph()
	aliases := s.Store.ModuleAliases()
	source := returns.StoreFieldWriteSource{Store: s.Store, Bindings: s.Store.ModuleBindings()}
	for _, nested := range graph.NestedFunctions() {
		name := graph.NameOf(nested.Symbol)
		if _, field, hasRoot := strings.Cut(name, "."); hasRoot {
			if strings.Contains(field, ".") {
				continue
			}
			name = field
		}
		field := rec.GetField(name)
		if field == nil {
			continue
		}
		if _, ok := field.Type.(*typ.Function); !ok {
			continue
		}
		writes := source.FieldWritesOf(nested.Symbol)
		must := source.MustWritesOf(nested.Symbol)
		for _, target := range cfg.SortedSymbolIDs(writes) {
			module := aliases[target]
			if module == "" {
				continue
			}
			for _, key := range api.SortedFieldWriteKeys(writes[target]) {
				if !must[target][key] {
					continue
				}
				t := writes[target][key]
				if t == nil {
					continue
				}
				if manifest.CallWrites == nil {
					manifest.CallWrites = make(map[string][]io.ModuleWrite)
				}
				manifest.CallWrites[name] = append(manifest.CallWrites[name], io.ModuleWrite{
					Module: module, Path: key.Path, Field: key.Field, Type: t,
				})
			}
		}
	}
}

// RefinementsForExport extracts computed function refinements for manifest generation.
// Returns refinements from the final converged interproc snapshot.
//
// The returned map associates each function's SymbolID with its computed refinement,
// including IO effects (row), termination status, and conditional effects.
// This enables importers to see side effect information for exported functions.
func (s *Session) RefinementsForExport() map[cfg.SymbolID]*constraint.FunctionRefinement {
	if s == nil || s.Store == nil {
		return nil
	}
	if s.Store.InterprocPrev == nil {
		return nil
	}
	return modules.CopyRefinementsForExport(s.Store.InterprocPrev.Refinements)
}

// RootGraph returns the root function's control flow graph.
// The root function wraps the module's top-level statements. Its CFG contains:
//   - Module-level assignments and function definitions
//   - Module-level control flow (if, for, while)
//   - Return statements for module export
//
// Use for advanced introspection or manifest generation that needs CFG access.
func (s *Session) RootGraph() *cfg.Graph {
	if s == nil || s.RootResult == nil {
		return nil
	}
	return s.RootResult.Graph
}
