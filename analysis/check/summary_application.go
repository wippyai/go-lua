package check

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type summaryApplication struct {
	summaries summary.Reader
	keyFor    callresult.KeyFunc
}

func (a summaryApplication) ok() bool {
	return a.summaries != nil && a.keyFor != nil
}

type summaryFunction struct {
	key summary.SummaryKey
	fn  *ast.FunctionExpr
}

type summaryTargets struct {
	symbols map[symbol.ID]summary.SummaryKey
	paths   map[path.PathKey]summary.SummaryKey
}

func (c *Checker) functionSummaries(stmts []ast.Stmt, bindings *bind.Result) (summaryApplication, error) {
	if c.config.SummaryResults != nil && c.config.SummaryKeyFor != nil {
		return summaryApplication{summaries: c.config.SummaryResults, keyFor: c.config.SummaryKeyFor}, nil
	}
	functions, targets := collectSummaryFunctions(bindings, stmts)
	if len(functions) == 0 {
		return summaryApplication{}, nil
	}
	keyFor := callresult.ByCalleeIdentity(targets.symbols, targets.paths)
	equations := make([]query.Function, 0, len(functions))
	for _, fn := range functions {
		fn := fn
		equations = append(equations, query.Function{
			Key: fn.key,
			Body: func(ctx query.Context) (summary.Summary, error) {
				checker := c.withSummaryApplication(summaryApplication{
					summaries: ctx.Summaries,
					keyFor:    keyFor,
				})
				result, err := checker.checkBoundFunctionBody(fn.fn, bindings)
				if errors.Is(err, ErrUnsupportedCFG) {
					return summary.Summary{}, nil
				}
				if err != nil {
					return summary.Summary{}, err
				}
				return summary.FromResult(result), nil
			},
		})
	}
	snapshot, err := query.Run(query.Config{
		Registry:  c.config.Registry,
		Functions: equations,
	})
	if err != nil {
		return summaryApplication{}, err
	}
	return summaryApplication{summaries: snapshot, keyFor: keyFor}, nil
}

func collectSummaryFunctions(bindings *bind.Result, stmts []ast.Stmt) ([]summaryFunction, summaryTargets) {
	if bindings == nil {
		return nil, summaryTargets{}
	}
	pathTargets := collectFunctionPathTargets(bindings, stmts)
	origins := bindings.FunctionOrigins()
	functions := make([]summaryFunction, 0, len(origins))
	targets := summaryTargets{
		symbols: make(map[symbol.ID]summary.SummaryKey),
		paths:   make(map[path.PathKey]summary.SummaryKey),
	}
	for _, origin := range origins {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		functions = append(functions, summaryFunction{key: key, fn: origin.Func})
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			targets.symbols[origin.TargetSymbol] = key
		}
		if targetPath, ok := pathTargets[origin.Func]; ok {
			targets.paths[targetPath.Key()] = key
		}
	}
	return functions, targets
}

func collectFunctionPathTargets(bindings *bind.Result, stmts []ast.Stmt) map[*ast.FunctionExpr]path.Path {
	if bindings == nil {
		return nil
	}
	out := make(map[*ast.FunctionExpr]path.Path)
	collectFunctionPathTargetsInStmts(out, bindings, stmts)
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func == nil {
			continue
		}
		collectFunctionPathTargetsInStmts(out, bindings, origin.Func.Stmts)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectFunctionPathTargetsInStmts(out map[*ast.FunctionExpr]path.Path, bindings *bind.Result, stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.LocalAssignStmt:
			symbols := bindings.LocalSymbols(stmt)
			for i, expr := range stmt.Exprs {
				if i >= len(symbols) || symbols[i] == 0 {
					continue
				}
				root := path.NewPath(symbols[i], bindings.Name(symbols[i]))
				collectFunctionPathTargetsInExpr(out, root, expr)
			}
		case *ast.AssignStmt:
			for i, expr := range stmt.Rhs {
				if i >= len(stmt.Lhs) {
					continue
				}
				target, ok := pathexpr.Resolve(stmt.Lhs[i], bindings)
				if !ok || target.IsEmpty() {
					continue
				}
				collectFunctionPathTargetsInExpr(out, target, expr)
			}
		case *ast.DoBlockStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.IfStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Then)
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Else)
		case *ast.WhileStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.RepeatStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.NumberForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.GenericForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		}
	}
}

func collectFunctionPathTargetsInExpr(out map[*ast.FunctionExpr]path.Path, root path.Path, expr ast.Expr) {
	if root.IsEmpty() {
		return
	}
	expr = unwrapFunctionValueTarget(expr)
	switch expr := expr.(type) {
	case *ast.FunctionExpr:
		out[expr] = root
	case *ast.TableExpr:
		collectFunctionPathTargetsInTable(out, root, expr)
	}
}

func collectFunctionPathTargetsInTable(out map[*ast.FunctionExpr]path.Path, root path.Path, table *ast.TableExpr) {
	if table == nil {
		return
	}
	arrayIndex := 0
	for _, field := range table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			continue
		}
		if !suffix.CanNameSummaryPath() {
			continue
		}
		target := appendPath(root, suffix.Path)
		collectFunctionPathTargetsInExpr(out, target, field.Value)
	}
}

func unwrapFunctionValueTarget(expr ast.Expr) ast.Expr {
	for {
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			expr = wrapped.Expr
		default:
			return expr
		}
	}
}

func appendPath(root path.Path, suffix path.Path) path.Path {
	out := root
	for _, seg := range suffix.Segments {
		out = out.Append(seg)
	}
	return out
}

func (c *Checker) withSummaryApplication(summaries summaryApplication) *Checker {
	if !summaries.ok() {
		return c
	}
	config := c.config
	config.SummaryResults = summaries.summaries
	config.SummaryKeyFor = summaries.keyFor
	config.CallOutcome = callresult.OutcomeProvider(summaries.summaries, summaries.keyFor)
	return &Checker{config: config}
}
