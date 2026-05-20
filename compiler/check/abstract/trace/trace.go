// Package trace owns CFG event discovery for the checker abstract interpreter.
package trace

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
)

// GraphEvidence records graph-local events that do not require a solved flow
// state. This is the single event discovery entry point used by the interpreter
// and consumers that need aliases, assignments, calls, returns, branches, or
// function definitions before the full interpreter result is available.
func GraphEvidence(graph *cfg.Graph, bindings *bind.BindingTable) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	bindings = graphBindings(graph, bindings)
	expressions := ExpressionEvidence(graph, bindings)
	functions := FunctionDefinitions(graph)
	return api.FlowEvidence{
		Calls:               expressions.Calls,
		Returns:             expressions.Returns,
		Assignments:         expressions.Assignments,
		Branches:            expressions.Branches,
		NormalExit:          NormalExitEvidence(graph),
		IdentifierUses:      expressions.IdentifierUses,
		FieldDefaults:       expressions.FieldDefaults,
		ParameterUses:       ParameterUses(graph, graph.Func()),
		FreshTableLiterals:  FreshTableLiterals(graph, expressions.Assignments, bindings),
		FunctionDefinitions: functions,
		EscapedFunctions:    FunctionEscapes(graph, bindings),
		LocalTypePredicates: LocalTypePredicates(functions),
	}
}

// NormalExitEvidence records the implicit function exit point.
func NormalExitEvidence(graph *cfg.Graph) api.NormalExitEvidence {
	if graph == nil || graph.CFG() == nil {
		return api.NormalExitEvidence{}
	}
	return api.NormalExitEvidence{Point: graph.Exit(), Valid: true}
}

// ExpressionEvidence records expression-level events discovered from the graph.
func ExpressionEvidence(graph *cfg.Graph, bindings *bind.BindingTable) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	if bindings == nil {
		bindings = graph.Bindings()
	}
	var out api.FlowEvidence

	var collectExpr func(cfg.Point, ast.Expr, api.CallOrigin)
	collectCall := func(p cfg.Point, call *ast.FuncCallExpr, origin api.CallOrigin) {
		if call == nil {
			return
		}
		info := graph.CallSiteAt(p, call)
		if info == nil {
			info = callEvidenceInfoFromExpr(call, bindings)
		}
		if info != nil {
			out.Calls = append(out.Calls, api.CallEvidence{Point: p, Info: info, Origin: origin})
		}
		collectExpr(p, call.Func, api.CallOriginExpression)
		collectExpr(p, call.Receiver, api.CallOriginExpression)
		for _, arg := range call.Args {
			collectExpr(p, arg, api.CallOriginExpression)
		}
	}
	collectExpr = func(p cfg.Point, expr ast.Expr, origin api.CallOrigin) {
		if expr == nil {
			return
		}
		switch e := expr.(type) {
		case *ast.IdentExpr:
			out.IdentifierUses = append(out.IdentifierUses, api.IdentifierUseEvidence{Point: p, Expr: e})
		case *ast.FuncCallExpr:
			collectCall(p, e, origin)
		case *ast.AttrGetExpr:
			collectExpr(p, e.Object, api.CallOriginExpression)
			collectExpr(p, e.Key, api.CallOriginExpression)
		case *ast.TableExpr:
			for _, field := range e.Fields {
				if field == nil {
					continue
				}
				collectExpr(p, field.Key, api.CallOriginExpression)
				collectExpr(p, field.Value, api.CallOriginExpression)
			}
		case *ast.LogicalOpExpr:
			if e.Operator == "or" {
				if sym, field, ok := fieldDefaultTarget(e.Lhs, bindings); ok {
					out.FieldDefaults = append(out.FieldDefaults, api.FieldDefaultEvidence{
						Point:  p,
						Target: sym,
						Field:  field,
						Value:  e.Rhs,
					})
				}
			}
			collectExpr(p, e.Lhs, api.CallOriginExpression)
			collectExpr(p, e.Rhs, api.CallOriginExpression)
		case *ast.RelationalOpExpr:
			collectExpr(p, e.Lhs, api.CallOriginExpression)
			collectExpr(p, e.Rhs, api.CallOriginExpression)
		case *ast.StringConcatOpExpr:
			collectExpr(p, e.Lhs, api.CallOriginExpression)
			collectExpr(p, e.Rhs, api.CallOriginExpression)
		case *ast.ArithmeticOpExpr:
			collectExpr(p, e.Lhs, api.CallOriginExpression)
			collectExpr(p, e.Rhs, api.CallOriginExpression)
		case *ast.UnaryMinusOpExpr:
			collectExpr(p, e.Expr, api.CallOriginExpression)
		case *ast.UnaryNotOpExpr:
			collectExpr(p, e.Expr, api.CallOriginExpression)
		case *ast.UnaryLenOpExpr:
			collectExpr(p, e.Expr, api.CallOriginExpression)
		case *ast.UnaryBNotOpExpr:
			collectExpr(p, e.Expr, api.CallOriginExpression)
		case *ast.CastExpr:
			collectExpr(p, e.Expr, api.CallOriginExpression)
		case *ast.NonNilAssertExpr:
			collectExpr(p, e.Expr, api.CallOriginExpression)
		}
	}

	graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			collectCall(p, info.Call, api.CallOriginStatement)
		}
	})
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		out.Assignments = append(out.Assignments, api.AssignmentEvidence{Point: p, Info: info})
		for _, expr := range info.Sources {
			collectExpr(p, expr, api.CallOriginAssignment)
		}
		for _, expr := range info.IterExprs {
			collectExpr(p, expr, api.CallOriginAssignment)
		}
		if info.NumericFor != nil {
			collectExpr(p, info.NumericFor.Init, api.CallOriginAssignment)
			collectExpr(p, info.NumericFor.Limit, api.CallOriginAssignment)
			collectExpr(p, info.NumericFor.Step, api.CallOriginAssignment)
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetField || target.Kind == cfg.TargetIndex {
				collectExpr(p, target.Base, api.CallOriginAssignment)
				collectExpr(p, target.Key, api.CallOriginAssignment)
			}
		}
	})
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info != nil {
			out.Returns = append(out.Returns, api.ReturnEvidence{Point: p, Info: info})
			for _, expr := range info.Exprs {
				collectExpr(p, expr, api.CallOriginReturn)
			}
		}
	})
	graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info != nil {
			out.Branches = append(out.Branches, api.BranchEvidence{Point: p, Info: info})
			collectExpr(p, info.Condition, api.CallOriginBranch)
		}
	})

	return out
}

// FunctionDefinitions records nested function definitions and the identity the
// checker uses for sibling grouping, local return inference, and interproc
// publication.
func FunctionDefinitions(graph *cfg.Graph) []api.FunctionDefinitionEvidence {
	if graph == nil {
		return nil
	}
	nestedFns := graph.NestedFunctions()
	if len(nestedFns) == 0 {
		return nil
	}
	out := make([]api.FunctionDefinitionEvidence, 0, len(nestedFns))
	for _, nf := range nestedFns {
		if nf.Func == nil {
			continue
		}
		funcDef := graph.FuncDef(nf.Point)
		name, sym, isLocal := functionDefinitionIdentity(graph, nf, funcDef)
		out = append(out, api.FunctionDefinitionEvidence{
			Nested:  nf,
			FuncDef: funcDef,
			Name:    name,
			Symbol:  sym,
			IsLocal: isLocal,
		})
	}
	return out
}

// FunctionEscapes records function values assigned to locations that may be
// invoked outside the local graph.
func FunctionEscapes(graph *cfg.Graph, bindings *bind.BindingTable) []api.FunctionEscapeEvidence {
	if graph == nil {
		return nil
	}
	var out []api.FunctionEscapeEvidence
	seen := make(map[api.FunctionEscapeEvidence]bool)
	appendEscape := func(p cfg.Point, sym cfg.SymbolID) {
		if sym == 0 {
			return
		}
		ev := api.FunctionEscapeEvidence{Point: p, Symbol: sym}
		if seen[ev] {
			return
		}
		seen[ev] = true
		out = append(out, ev)
	}

	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || !funcDefEscapesGraph(info.TargetKind) {
			return
		}
		appendEscape(p, info.Symbol)
	})

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		info.EachTargetSource(func(i int, target cfg.AssignTarget, src ast.Expr) {
			if !assignmentTargetEscapesGraph(target) {
				return
			}
			appendEscape(p, assignmentSourceFunctionSymbol(info, i, src, bindings))
		})
	})

	return out
}

// LocalTypePredicates records local functions whose return value is a builtin
// type(param) predicate. The interpreter consumes this to lower predicate-call
// guards without reopening function bodies.
func LocalTypePredicates(functions []api.FunctionDefinitionEvidence) []api.LocalTypePredicateEvidence {
	if len(functions) == 0 {
		return nil
	}
	var out []api.LocalTypePredicateEvidence
	for _, def := range functions {
		if def.Symbol == 0 || def.Nested.Func == nil || def.Nested.Func.ParList == nil {
			continue
		}
		names := def.Nested.Func.ParList.Names
		for paramIndex, paramName := range names {
			if paramName == "" {
				continue
			}
			if kindName, ok := functionReturnsTypePredicate(def.Nested.Func, paramName); ok {
				out = append(out, api.LocalTypePredicateEvidence{
					Symbol:     def.Symbol,
					ParamName:  paramName,
					ParamIndex: paramIndex,
					Kind:       kindName,
				})
				break
			}
		}
	}
	return out
}

// FreshTableLiterals records table-literal provenance required by structured
// assignment diagnostics. It is intentionally narrow: only identifier sources
// used in non-identifier assignment targets need this proof.
func FreshTableLiterals(
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	bindings *bind.BindingTable,
) []api.FreshTableLiteralEvidence {
	if graph == nil || len(assignments) == 0 {
		return nil
	}
	if bindings == nil {
		bindings = graph.Bindings()
	}
	if bindings == nil {
		return nil
	}
	seen := make(map[freshTableQuery]bool)
	var out []api.FreshTableLiteralEvidence
	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind == cfg.TargetIdent || source == nil {
				return
			}
			ident, ok := source.(*ast.IdentExpr)
			if !ok || ident == nil {
				return
			}
			sym, ok := bindings.SymbolOf(ident)
			if !ok || sym == 0 {
				return
			}
			version := graph.VisibleVersion(p, sym)
			if version.Symbol == 0 || version.ID == 0 {
				return
			}
			query := freshTableQuery{Point: p, Symbol: sym, Version: version}
			if seen[query] {
				return
			}
			seen[query] = true
			if fresh, ok := currentFreshTableLiteral(graph, bindings, p, sym, version); ok {
				out = append(out, fresh)
			}
		})
	}
	return out
}

type freshTableQuery struct {
	Point   cfg.Point
	Symbol  cfg.SymbolID
	Version cfg.Version
}

func currentFreshTableLiteral(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	at cfg.Point,
	sym cfg.SymbolID,
	version cfg.Version,
) (api.FreshTableLiteralEvidence, bool) {
	current := at
	seen := make(map[cfg.Point]struct{}, 4)
	for {
		preds := graph.PredecessorsReadOnly(current)
		if len(preds) != 1 {
			return api.FreshTableLiteralEvidence{}, false
		}
		pred := preds[0]
		if _, ok := seen[pred]; ok {
			return api.FreshTableLiteralEvidence{}, false
		}
		seen[pred] = struct{}{}

		switch info := graph.Info(pred).(type) {
		case *cfg.AssignInfo:
			if fresh, found, ok := freshTableAssignment(info, pred, sym, version, graph); found {
				return api.FreshTableLiteralEvidence{
					Point:           at,
					Symbol:          sym,
					Version:         version,
					Table:           fresh,
					AssignmentPoint: pred,
				}, ok
			}
			if assignmentInvalidatesFreshness(info, sym, bindings) {
				return api.FreshTableLiteralEvidence{}, false
			}
		case *cfg.CallInfo:
			return api.FreshTableLiteralEvidence{}, false
		case *cfg.ReturnInfo:
			if returnInvalidatesFreshness(info, sym, bindings) {
				return api.FreshTableLiteralEvidence{}, false
			}
		case *cfg.FuncDefInfo:
			return api.FreshTableLiteralEvidence{}, false
		}

		current = pred
	}
}

func freshTableAssignment(
	info *cfg.AssignInfo,
	p cfg.Point,
	sym cfg.SymbolID,
	version cfg.Version,
	graph *cfg.Graph,
) (*ast.TableExpr, bool, bool) {
	if info == nil {
		return nil, false, false
	}
	var table *ast.TableExpr
	found := false
	ok := false
	info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
		if found || target.Kind != cfg.TargetIdent || target.Symbol != sym {
			return
		}
		found = true
		assignedVersion := graph.VisibleVersion(p, sym)
		if assignedVersion.Symbol != version.Symbol || assignedVersion.ID != version.ID {
			return
		}
		if t, isTable := src.(*ast.TableExpr); isTable && t != nil {
			table = t
			ok = true
		}
	})
	return table, found, ok
}

func assignmentInvalidatesFreshness(info *cfg.AssignInfo, sym cfg.SymbolID, bindings *bind.BindingTable) bool {
	if info == nil {
		return false
	}
	for _, call := range info.SourceCalls {
		if call != nil {
			return true
		}
	}
	for i, target := range info.Targets {
		if target.Kind != cfg.TargetIdent {
			if target.BaseSymbol == sym || provenance.ExprReferencesSymbol(target.Expr, sym, bindings) {
				return true
			}
		}
		if provenance.ExprMayExposeSymbolValue(info.SourceAt(i), sym, bindings) {
			return true
		}
	}
	return false
}

func returnInvalidatesFreshness(info *cfg.ReturnInfo, sym cfg.SymbolID, bindings *bind.BindingTable) bool {
	if info == nil {
		return false
	}
	for _, call := range info.SourceCalls {
		if call != nil {
			return true
		}
	}
	for _, expr := range info.Exprs {
		if provenance.ExprMayExposeSymbolValue(expr, sym, bindings) {
			return true
		}
	}
	return false
}

func functionReturnsTypePredicate(fn *ast.FunctionExpr, paramName string) (string, bool) {
	if fn == nil || paramName == "" {
		return "", false
	}
	for _, stmt := range fn.Stmts {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || ret == nil || len(ret.Exprs) == 0 {
			continue
		}
		if kindName, ok := exprTypePredicateKind(ret.Exprs[0], paramName); ok {
			return kindName, true
		}
	}
	return "", false
}

func exprTypePredicateKind(expr ast.Expr, paramName string) (string, bool) {
	switch e := expr.(type) {
	case *ast.LogicalOpExpr:
		if kindName, ok := exprTypePredicateKind(e.Lhs, paramName); ok {
			return kindName, true
		}
		return exprTypePredicateKind(e.Rhs, paramName)
	case *ast.RelationalOpExpr:
		if e.Operator != "==" {
			return "", false
		}
		if callIsTypeOfParam(e.Lhs, paramName) {
			if s, ok := e.Rhs.(*ast.StringExpr); ok && s.Value != "" {
				return s.Value, true
			}
		}
		if callIsTypeOfParam(e.Rhs, paramName) {
			if s, ok := e.Lhs.(*ast.StringExpr); ok && s.Value != "" {
				return s.Value, true
			}
		}
	}
	return "", false
}

func callIsTypeOfParam(expr ast.Expr, paramName string) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil || callsite.IsMethodLikeExpr(call) || len(call.Args) != 1 {
		return false
	}
	fnIdent, ok := call.Func.(*ast.IdentExpr)
	if !ok || fnIdent == nil || fnIdent.Value != "type" {
		return false
	}
	argIdent, ok := call.Args[0].(*ast.IdentExpr)
	return ok && argIdent != nil && argIdent.Value == paramName
}

func functionDefinitionIdentity(
	graph *cfg.Graph,
	nf cfg.NestedFunc,
	funcDef *cfg.FuncDefInfo,
) (string, cfg.SymbolID, bool) {
	if funcDef != nil {
		return funcDef.Name, funcDef.Symbol, funcDef.TargetKind == cfg.FuncDefGlobal
	}
	if graph != nil {
		if assignInfo := graph.Assign(nf.Point); assignInfo != nil && assignInfo.IsLocal {
			if len(assignInfo.Targets) == 1 && assignInfo.Targets[0].Kind == cfg.TargetIdent {
				if len(assignInfo.Sources) == 1 && assignInfo.Sources[0] == nf.Func {
					return assignInfo.Targets[0].Name, assignInfo.Targets[0].Symbol, true
				}
			}
		}
	}
	if nf.Symbol != 0 {
		return "", nf.Symbol, true
	}
	return "", 0, false
}

func fieldDefaultTarget(expr ast.Expr, bindings *bind.BindingTable) (cfg.SymbolID, string, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil || bindings == nil {
		return 0, "", false
	}
	obj, ok := attr.Object.(*ast.IdentExpr)
	if !ok || obj == nil {
		return 0, "", false
	}
	key, ok := attr.Key.(*ast.StringExpr)
	if !ok || key == nil || key.Value == "" {
		return 0, "", false
	}
	sym, ok := bindings.SymbolOf(obj)
	if !ok || sym == 0 {
		return 0, "", false
	}
	return sym, key.Value, true
}

func callEvidenceInfoFromExpr(ex *ast.FuncCallExpr, bindings *bind.BindingTable) *cfg.CallInfo {
	if ex == nil {
		return nil
	}
	info := cfg.BuildCallInfo(ex, false)
	if bindings == nil {
		return info
	}
	info.CalleeSymbol = callsite.SymbolFromExpr(ex.Func, bindings)
	if ex.Receiver != nil {
		info.ReceiverSymbol = callsite.SymbolFromExpr(ex.Receiver, bindings)
		if id, ok := ex.Receiver.(*ast.IdentExpr); ok {
			info.ReceiverName = id.Value
		}
	}
	info.ArgSymbols = make([]cfg.SymbolID, len(ex.Args))
	for i, arg := range ex.Args {
		info.ArgSymbols[i] = callsite.SymbolFromExpr(arg, bindings)
	}
	return info
}

func funcDefEscapesGraph(kind cfg.FuncDefTargetKind) bool {
	switch kind {
	case cfg.FuncDefGlobal, cfg.FuncDefField, cfg.FuncDefMethod:
		return true
	default:
		return false
	}
}

func assignmentTargetEscapesGraph(target cfg.AssignTarget) bool {
	switch target.Kind {
	case cfg.TargetField, cfg.TargetIndex:
		return true
	default:
		return false
	}
}

func assignmentSourceFunctionSymbol(
	info *cfg.AssignInfo,
	i int,
	src ast.Expr,
	bindings *bind.BindingTable,
) cfg.SymbolID {
	if info != nil && i >= 0 && i < len(info.SourceSymbols) {
		if sym := info.SourceSymbols[i]; sym != 0 {
			return sym
		}
	}
	if fn, ok := src.(*ast.FunctionExpr); ok && bindings != nil {
		if sym, found := bindings.FuncLitSymbol(fn); found {
			return sym
		}
	}
	return 0
}

func graphBindings(graph *cfg.Graph, module *bind.BindingTable) *bind.BindingTable {
	if graph != nil {
		if bindings := graph.Bindings(); bindings != nil {
			return bindings
		}
	}
	return module
}
