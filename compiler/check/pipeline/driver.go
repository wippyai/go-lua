// Package driver provides the fixpoint iteration loop for type analysis.
//
// The driver orchestrates the multi-phase type checking process:
//
//  1. Initialize module bindings and CFG hierarchy
//  2. Run pre-flow return type inference for local functions
//  3. Execute the memoized function analysis pipeline
//  4. Propagate effects and interprocedural facts
//  5. Process nested functions with the current parent context
//  6. Repeat until the interproc product reaches fixpoint
//
// The driver coordinates several inference subsystems:
//   - Return inference: Computes return types for local functions
//   - Nested inference: Processes nested function bodies with parent context
//   - Effect propagation: Tracks side effects through call chains
//   - Interproc facts: Shares type information across function boundaries
package pipeline

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/effects"
	interprocinfer "github.com/wippyai/go-lua/compiler/check/infer/interproc"
	nestedinfer "github.com/wippyai/go-lua/compiler/check/infer/nested"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Config supplies dependencies for the fixpoint driver.
type Config struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Stdlib        *scope.State
	Manifests     *db.DB
	MaxScopeDepth int
	EmitScopeDiag bool
	FuncResultQ   *db.Query[api.FuncKey, *api.FuncResult]

	// RecursiveFamilies is the compilation-scoped recursive-family interner.
	// Inferred-return sealing widens family bodies only through it, so one
	// compilation's convergence seed cannot mutate type state shared with another.
	RecursiveFamilies *typ.RecursiveFamilyInterner
}

// Driver executes the fixpoint loop and function analysis.
type Driver struct {
	cfg Config
}

// New creates a driver with the provided configuration.
func New(cfg Config) *Driver {
	return &Driver{cfg: cfg}
}

// Run analyzes a module chunk using the fixpoint loop.
func (d *Driver) Run(sess api.AnalysisSession, chunk []ast.Stmt) {
	if sess == nil {
		return
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}
	sess.SetRootFuncNode(fn)

	store := sess.StoreHandle()
	if store != nil {
		globals := collectGlobalNames(d.cfg.GlobalTypes)
		store.SetModuleBindings(bind.Bind(fn, globals))
	}

	chunkGraph := sess.GetOrBuildCFG(fn)
	if chunkGraph != nil {
		sess.RegisterGraphHierarchy(chunkGraph)
		if store != nil {
			evidence := store.EvidenceForGraph(chunkGraph)
			store.SetModuleAliases(modules.AliasesFromAssignments(evidence.Assignments, chunkGraph))
			if d.cfg.Stdlib != nil {
				store.SetGraphParentHash(chunkGraph.ID(), d.cfg.Stdlib.Hash())
			}
		}
	}

	parent := d.cfg.Stdlib
	if parent == nil {
		parent = scope.New()
	}
	d.runFixpoint(sess, fn, parent, api.AnalysisContext{})
}

func (d *Driver) runFixpoint(sess api.AnalysisSession, fn *ast.FunctionExpr, parent *scope.State, ctx api.AnalysisContext) {
	zzIter := 0
	for {
		d.prepareIterationState(sess)
		d.checkFunctionFixpoint(sess, fn, parent, ctx)
		zzIter++
		if zzIter > 40 {
			if st := sess.StoreHandle(); st != nil {
				println("ZZFIX iter", zzIter, "diffs", fmt.Sprint(st.FixpointDiffs()))
			}
		}
		if zzIter > 60 {
			panic("ZZFIX non-convergence aborted")
		}
		if d.advanceFixpoint(sess.StoreHandle()) {
			return
		}
	}
}

func (d *Driver) prepareIterationState(sess api.AnalysisSession) {
	sess.ResetDiagnostics()

	scopeState := sess.ScopeDepthDiagState()
	for k := range scopeState {
		delete(scopeState, k)
	}
}

func (d *Driver) advanceFixpoint(store api.IterationStore) bool {
	if store == nil {
		return true
	}
	return !store.FixpointSwap()
}

func (d *Driver) checkFunctionFixpoint(sess api.AnalysisSession, fn *ast.FunctionExpr, parent *scope.State, ctx api.AnalysisContext) {
	graph := sess.GetOrBuildCFG(fn)
	if graph == nil {
		return
	}

	store := sess.StoreHandle()
	parentHash := d.registerParentScope(store, graph.ID(), parent, ctx)

	result := d.loadFunctionResult(sess, graph.ID(), parentHash, store)
	if result == nil {
		return
	}

	results := sess.ResultsMap()
	d.recordFunctionResult(sess, fn, result, results)
	d.emitScopeDepthDiagnostic(sess, fn, result)

	funcSym := cfg.SymbolID(0)
	if store != nil {
		if sym, ok := store.SymbolForFunc(fn); ok {
			funcSym = sym
		}
	}
	d.storeFunctionRefinement(store, result, funcSym)
	interprocinfer.StoreFactsFromResult(store, fn, result, parent, d.cfg.RecursiveFamilies)
	d.processNestedFunctions(sess, store, graph, results, result)
}

func (d *Driver) processNestedFunctions(
	sess api.AnalysisSession,
	store api.IterationStore,
	graph *cfg.Graph,
	results map[*ast.FunctionExpr]*api.FuncResult,
	result *api.FuncResult,
) {
	nestedProc := nestedinfer.New(nestedinfer.Config{
		Stdlib: d.cfg.Stdlib,
		Store:  store,
		Graphs: sess,
		Check: func(fn *ast.FunctionExpr, parent *scope.State, ctx api.AnalysisContext) {
			d.checkFunctionFixpoint(sess, fn, parent, ctx)
		},
		ResultForFunc: func(fn *ast.FunctionExpr) *api.FuncAnalysisView {
			if results == nil {
				return nil
			}
			return api.ViewFromResult(results[fn])
		},
		RootResult: api.ViewFromResult(sess.RootResultValue()),
	})
	nestedProc.ProcessNestedFunctions(graph, api.ViewFromResult(result))
}

func (d *Driver) registerParentScope(store api.IterationStore, graphID uint64, parent *scope.State, ctx api.AnalysisContext) uint64 {
	parentHash := uint64(0)
	if parent != nil {
		parentHash = parent.Hash()
	} else {
		parentHash = api.ParentHashForGraph(store, graphID, parent)
	}
	parentHash = ctx.ParentHash(parentHash)
	if store != nil && parentHash != 0 {
		store.SetParentScope(parentHash, parent)
		store.SetGraphParentHash(graphID, parentHash)
		if !ctx.Empty() {
			if contextual, ok := store.(interface {
				SetGraphAnalysisContext(api.GraphKey, api.AnalysisContext)
			}); ok {
				contextual.SetGraphAnalysisContext(api.GraphKey{GraphID: graphID, ParentHash: parentHash}, ctx)
			}
		}
	}
	return parentHash
}

func (d *Driver) loadFunctionResult(
	sess api.AnalysisSession,
	graphID, parentHash uint64,
	store api.IterationStore,
) *api.FuncResult {
	if d.cfg.FuncResultQ == nil {
		return nil
	}
	return d.cfg.FuncResultQ.Get(sess.Context(), api.FuncKey{
		GraphID:    graphID,
		ParentHash: parentHash,
	})
}

func (d *Driver) recordFunctionResult(
	sess api.AnalysisSession,
	fn *ast.FunctionExpr,
	result *api.FuncResult,
	results map[*ast.FunctionExpr]*api.FuncResult,
) {
	if result == nil {
		return
	}
	if results != nil {
		results[fn] = result
	}
	if fn == sess.RootFuncNode() {
		sess.SetRootResultValue(result)
	}
}

func (d *Driver) emitScopeDepthDiagnostic(sess api.AnalysisSession, fn *ast.FunctionExpr, result *api.FuncResult) {
	if result == nil || !d.cfg.EmitScopeDiag || d.cfg.MaxScopeDepth <= 0 || !result.DepthLimitExceeded {
		return
	}
	scopeState := sess.ScopeDepthDiagState()
	if scopeState[fn] {
		return
	}
	pos := diag.Position{File: sess.Source()}
	span := diag.Span{}
	if fn != nil && fn.Line() > 0 {
		pos.Line = fn.Line()
		pos.Column = fn.Column()
		span.StartLine = fn.Line()
		span.StartCol = fn.Column()
		span.EndLine = fn.LastLine()
		span.EndCol = fn.LastColumn()
	}
	sess.AppendDiagnostics(diag.Diagnostic{
		Position: pos,
		Span:     span,
		Severity: diag.SeverityWarning,
		Message:  fmt.Sprintf("scope depth limit exceeded (max=%d); analysis may be incomplete", d.cfg.MaxScopeDepth),
	})
	scopeState[fn] = true
}

func (d *Driver) storeFunctionRefinement(store api.IterationStore, result *api.FuncResult, funcSym cfg.SymbolID) {
	if result == nil || store == nil || funcSym == 0 {
		return
	}
	refinementFacts := refinementFactsFrom(store)
	lookup := func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		return effects.ResolveRefinementBySym(refinementFacts, store.ModuleBindings(), d.cfg.GlobalTypes, sym)
	}
	fnEffect := effects.Propagate(result, lookup)
	if fnEffect == nil {
		return
	}
	key, ok := store.ParentGraphKeyForSymbol(funcSym)
	if !ok {
		return
	}
	builder := functionfact.NewBuilder()
	builder.AddRefinement(funcSym, fnEffect)
	store.MergeInterprocFactsNext(key, interprocdomain.FunctionFactsDelta(builder.Build()))
}

func collectGlobalNames(globalTypes map[string]typ.Type) []string {
	if globalTypes == nil {
		return nil
	}
	all := cfg.SortedFieldNames(globalTypes)
	names := make([]string, 0, len(all))
	for _, name := range all {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
