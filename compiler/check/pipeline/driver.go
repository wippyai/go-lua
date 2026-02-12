// Package driver provides the fixpoint iteration loop for type analysis.
//
// The driver orchestrates the multi-phase type checking process:
//
//  1. Initialize module bindings and CFG hierarchy
//  2. Run pre-flow return type inference for local functions
//  3. Execute the memoized function analysis pipeline
//  4. Propagate effects and interprocedural facts
//  5. Process nested functions recursively
//  6. Repeat until fixpoint (no channel changes) or max iterations
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
	"github.com/wippyai/go-lua/compiler/check/effects"
	interprocinfer "github.com/wippyai/go-lua/compiler/check/infer/interproc"
	nestedinfer "github.com/wippyai/go-lua/compiler/check/infer/nested"
	returninfer "github.com/wippyai/go-lua/compiler/check/infer/return"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Config supplies dependencies for the fixpoint driver.
type Config struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Stdlib        *scope.State
	Manifests     *db.DB
	MaxIterations int
	MaxScopeDepth int
	EmitScopeDiag bool
	FuncResultQ   *db.Query[api.FuncKey, *api.FuncResult]
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
			store.SetModuleAliases(modules.CollectAliases(chunkGraph))
			if d.cfg.Stdlib != nil {
				store.SetGraphParentHash(chunkGraph.ID(), d.cfg.Stdlib.Hash())
			}
		}
	}

	d.runFixpoint(sess, fn, d.cfg.Stdlib)
}

func (d *Driver) runFixpoint(sess api.AnalysisSession, fn *ast.FunctionExpr, parent *scope.State) {
	store := sess.StoreHandle()
	maxIterations := d.cfg.MaxIterations
	if maxIterations < 1 {
		maxIterations = 1
	}

	converged := false
	for iter := 0; iter < maxIterations; iter++ {
		if d.cfg.FuncResultQ != nil {
			d.cfg.FuncResultQ.Clear()
		}
		sess.ResetDiagnostics()

		scopeState := sess.ScopeDepthDiagState()
		for k := range scopeState {
			delete(scopeState, k)
		}

		d.checkFunctionFixpoint(sess, fn, parent)

		changed := false
		if store != nil {
			changed = store.FixpointSwap()
		}
		if !changed {
			converged = true
			break
		}

		store.BumpRevision()
	}

	if !converged {
		diffs := []string(nil)
		if store != nil {
			diffs = store.FixpointChannelDiffs()
		}
		msg := "inter-function fixpoint did not converge"
		if len(diffs) > 0 {
			msg += "; unstable channels: " + fmt.Sprintf("%v", diffs)
		}
		sess.AppendDiagnostics(diag.Diagnostic{
			Position: diag.Position{File: sess.Source()},
			Severity: diag.SeverityWarning,
			Message:  msg,
		})
	}
}

func (d *Driver) checkFunctionFixpoint(sess api.AnalysisSession, fn *ast.FunctionExpr, parent *scope.State) {
	graph := sess.GetOrBuildCFG(fn)
	if graph == nil {
		return
	}

	store := sess.StoreHandle()
	parentHash := uint64(0)
	if store != nil {
		if stable := store.GraphParentHashOf(graph.ID()); stable != 0 {
			parentHash = stable
		}
	}
	if parentHash == 0 && parent != nil {
		parentHash = parent.Hash()
	}

	if store != nil {
		store.SetParentScope(parentHash, parent)
	}

	if store != nil {
		inferencer := returninfer.New(returninfer.Config{
			Types:         d.cfg.Types,
			GlobalTypes:   d.cfg.GlobalTypes,
			Manifests:     d.cfg.Manifests,
			Stdlib:        d.cfg.Stdlib,
			Store:         store,
			Graphs:        sess,
			SourceName:    sess.Source(),
			MaxIterations: returns.MaxReturnSummaryIterations,
		})
		var effectLookup constraint.EffectLookupBySym
		if es := store.EffectStore(); es != nil {
			effectLookup = es.LookupEffectBySym
		}
		var parentFacts flow.TypeFacts

		if meta, ok := store.NestedMetaFor(graph.ID()); ok && meta.ParentGraphID != 0 {
			if results := sess.ResultsMap(); results != nil {
				if parentGraph := store.Graphs()[meta.ParentGraphID]; parentGraph != nil {
					if parentFn := store.FuncForGraph(parentGraph); parentFn != nil {
						if parentResult := results[parentFn]; parentResult != nil {
							parentFacts = parentResult.Facts
						}
					}
				}
			}
		}

		summaries, funcTypes, diags := inferencer.ComputeForGraph(returninfer.RunContext{
			Ctx:          sess.Context(),
			ParentFacts:  parentFacts,
			EffectLookup: effectLookup,
		}, graph, parent)
		if len(diags) > 0 {
			sess.AppendDiagnostics(diags...)
		}
		if len(summaries) > 0 {
			if key, ok := store.GraphKeyFor(graph, parent); ok {
				store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
					for sym, rets := range summaries {
						reconciled := returns.ReconcileFunctionFact(returns.ReconcileFunctionFactInput{
							ExistingSummary:  facts.ReturnSummaries[sym],
							ExistingNarrow:   facts.NarrowReturns[sym],
							ExistingFunc:     facts.FuncTypes[sym],
							CandidateSummary: rets,
							CandidateFunc:    funcTypes[sym],
						})
						if len(reconciled.Summary) > 0 {
							if facts.ReturnSummaries == nil {
								facts.ReturnSummaries = make(api.ReturnSummaries, len(summaries))
							}
							facts.ReturnSummaries[sym] = reconciled.Summary
						}
						if len(reconciled.Narrow) > 0 {
							if facts.NarrowReturns == nil {
								facts.NarrowReturns = make(api.NarrowReturnSummaries, len(summaries))
							}
							facts.NarrowReturns[sym] = reconciled.Narrow
						}
						if reconciled.Func != nil {
							if facts.FuncTypes == nil {
								facts.FuncTypes = make(api.FuncTypes, len(summaries))
							}
							facts.FuncTypes[sym] = reconciled.Func
						}
					}
				})
			}
		}
	}

	var revision uint64
	if store != nil {
		revision = store.Revision()
	}

	result := (*api.FuncResult)(nil)
	if d.cfg.FuncResultQ != nil {
		result = d.cfg.FuncResultQ.Get(sess.Context(), api.FuncKey{
			GraphID:       graph.ID(),
			ParentHash:    parentHash,
			StoreRevision: revision,
		})
	}
	if result == nil {
		return
	}

	results := sess.ResultsMap()
	if results != nil {
		results[fn] = result
	}
	if fn == sess.RootFuncNode() {
		sess.SetRootResultValue(result)
	}

	if d.cfg.EmitScopeDiag && d.cfg.MaxScopeDepth > 0 && result.DepthLimitExceeded {
		scopeState := sess.ScopeDepthDiagState()
		if !scopeState[fn] {
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
	}

	funcSym := cfg.SymbolID(0)
	if store != nil {
		if sym, ok := store.SymbolForFunc(fn); ok {
			funcSym = sym
		}
	}
	d.storeFunctionEffect(store, result, funcSym)
	interprocinfer.StoreFactsFromResult(store, fn, result, parent)

	nestedProc := nestedinfer.New(nestedinfer.Config{
		Stdlib: d.cfg.Stdlib,
		Store:  store,
		Graphs: sess,
		Check: func(fn *ast.FunctionExpr, parent *scope.State) {
			d.checkFunctionFixpoint(sess, fn, parent)
		},
		ResultForFunc: func(fn *ast.FunctionExpr) *api.FuncResultView {
			if results == nil {
				return nil
			}
			return api.ViewFromResult(results[fn])
		},
		RootResult: api.ViewFromResult(sess.RootResultValue()),
	})
	nestedProc.ProcessNestedFunctions(graph, api.ViewFromResult(result))
}

func (d *Driver) storeFunctionEffect(store api.IterationStore, result *api.FuncResult, funcSym cfg.SymbolID) {
	if result == nil || store == nil || funcSym == 0 {
		return
	}
	lookup := func(sym cfg.SymbolID) *constraint.FunctionEffect {
		return effects.LookupEffectBySym(store.EffectStore(), store.ModuleBindings(), d.cfg.GlobalTypes, sym)
	}
	fnEffect := effects.Propagate(result, lookup)
	if fnEffect == nil {
		return
	}
	store.StoreFunctionEffect(funcSym, fnEffect)
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
