package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type callbackProtocolAPI struct {
	name string
	fn   *ast.FunctionExpr
}

type callbackProtocolStore struct {
	api        string
	paramIndex int
	slot       string
	kind       callbackProtocolStoreKind
}

type callbackProtocolStoreKind uint8

const (
	callbackProtocolPhaseStore callbackProtocolStoreKind = iota + 1
	callbackProtocolInvocationStore
)

// publishCallbackProtocols infers framework callback ordering from the
// provider implementation itself: exported APIs store callback parameters into
// stable named slots, and replay code later invokes those slots in an observed
// order. The manifest stores only the public protocol facts; consumers do not
// re-analyze provider source.
func publishCallbackProtocols(m *manifest.Manifest, modulePath string, result program.Result) {
	root := result.RootResult()
	if m == nil || modulePath == "" || root == nil || root.Graph() == nil {
		return
	}
	apis := exportedCallbackProtocolAPIs(modulePath, root)
	if len(apis) == 0 {
		return
	}
	results := allFunctionResults(root)
	byFn := callbackProtocolResultsByFunction(results)
	stores := callbackProtocolStores(root, byFn, apis)
	if len(stores) == 0 {
		return
	}
	replays := callbackProtocolReplayOrders(results)
	if len(replays) == 0 {
		return
	}

	phaseStoresBySlot := make(map[string][]callbackProtocolStore)
	invocationStoresBySlot := make(map[string][]callbackProtocolStore)
	for _, store := range stores {
		switch store.kind {
		case callbackProtocolPhaseStore:
			phaseStoresBySlot[store.slot] = append(phaseStoresBySlot[store.slot], store)
		case callbackProtocolInvocationStore:
			invocationStoresBySlot[store.slot] = append(invocationStoresBySlot[store.slot], store)
		}
	}

	for _, replay := range replays {
		for index, slot := range replay {
			invocations := invocationStoresBySlot[slot]
			if len(invocations) == 0 {
				continue
			}
			before, after := callbackProtocolPhasesAround(replay, index, phaseStoresBySlot)
			if len(before) == 0 && len(after) == 0 {
				continue
			}
			for _, invocation := range invocations {
				m.DefineCallbackPhaseInvocation(invocation.api, invocation.paramIndex, before, after)
			}
			for _, phase := range append(append([]string(nil), before...), after...) {
				for _, registration := range phaseStoresBySlot[phase] {
					m.DefineCallbackPhaseRegistration(registration.api, registration.paramIndex, phase)
				}
			}
		}
	}
}

func exportedCallbackProtocolAPIs(modulePath string, root *body.Result) []callbackProtocolAPI {
	if modulePath == "" || root == nil || root.Graph() == nil {
		return nil
	}
	dom := dominance.ComputeImmediateDominatorInfo(root.Graph())
	var out []callbackProtocolAPI
	seen := make(map[*ast.FunctionExpr]struct{})
	for _, exportRoot := range returnedExportSourcePaths(root) {
		for _, point := range root.Graph().RPO() {
			fact, ok := root.FunctionDefinition(point)
			if !ok || !dominatesAllReturnPoints(dom, point, exportRoot.points) || fact.Func == nil || fact.Name == nil {
				continue
			}
			member, ok := functionDefinitionExportMember(root, exportRoot.path, fact.Name)
			if !ok {
				continue
			}
			name, ok := callbackProtocolMemberName(member)
			if !ok {
				continue
			}
			if _, duplicate := seen[fact.Func]; duplicate {
				continue
			}
			seen[fact.Func] = struct{}{}
			out = append(out, callbackProtocolAPI{name: name, fn: fact.Func})
		}
		for _, point := range root.Graph().RPO() {
			fact, ok := root.OrdinaryAssignment(point)
			if !ok || !dominatesAllReturnPoints(dom, point, exportRoot.points) || !fact.HasPath || fact.Path.Symbol != exportRoot.path.Symbol {
				continue
			}
			member, ok := directMemberSegment(exportRoot.path.Segments, fact.Path.Segments)
			if !ok {
				continue
			}
			name, ok := callbackProtocolMemberName(member)
			if !ok {
				continue
			}
			fn, ok := ordinaryAssignmentRHSExpr(fact).(*ast.FunctionExpr)
			if !ok || fn == nil {
				continue
			}
			if _, duplicate := seen[fn]; duplicate {
				continue
			}
			seen[fn] = struct{}{}
			out = append(out, callbackProtocolAPI{name: name, fn: fn})
		}
	}
	return out
}

func callbackProtocolMemberName(member segment.Segment) (string, bool) {
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return member.Name, member.Name != ""
	default:
		return "", false
	}
}

func callbackProtocolResultsByFunction(results []*body.Result) map[*ast.FunctionExpr]*body.Result {
	out := make(map[*ast.FunctionExpr]*body.Result, len(results))
	for _, result := range results {
		if result == nil || result.Function() == nil {
			continue
		}
		out[result.Function()] = result
	}
	return out
}

func callbackProtocolStores(root *body.Result, byFn map[*ast.FunctionExpr]*body.Result, apis []callbackProtocolAPI) []callbackProtocolStore {
	var out []callbackProtocolStore
	for _, api := range apis {
		result := byFn[api.fn]
		if result == nil {
			continue
		}
		paramSlots := root.FunctionParamSlots(api.fn)
		params := make(map[symbol.ID]int, len(paramSlots))
		for i, slot := range paramSlots {
			if slot.Symbol != 0 && !slot.ImplicitSelf {
				params[slot.Symbol] = i
			}
		}
		if len(params) == 0 {
			continue
		}
		for _, store := range callbackProtocolDirectStores(result, params) {
			store.api = api.name
			out = append(out, store)
		}
		for _, store := range callbackProtocolObjectStores(result, params) {
			store.api = api.name
			out = append(out, store)
		}
	}
	return out
}

func callbackProtocolDirectStores(result *body.Result, params map[symbol.ID]int) []callbackProtocolStore {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var out []callbackProtocolStore
	for _, point := range graph.RPO() {
		fact, ok := result.OrdinaryAssignment(point)
		if !ok || !fact.HasPath || fact.Path.Symbol == 0 {
			continue
		}
		slot, ok := callbackProtocolSlotFromSegments(fact.Path.Segments)
		if !ok {
			continue
		}
		index, ok := callbackProtocolParamIndex(result, ordinaryAssignmentRHSExpr(fact), params)
		if !ok {
			continue
		}
		out = append(out, callbackProtocolStore{paramIndex: index, slot: slot, kind: callbackProtocolPhaseStore})
	}
	return out
}

func callbackProtocolObjectStores(result *body.Result, params map[symbol.ID]int) []callbackProtocolStore {
	fn := result.Function()
	if fn == nil {
		return nil
	}
	var out []callbackProtocolStore
	callbackProtocolWalkStmts(fn.Stmts, func(expr ast.Expr) {
		table, ok := expr.(*ast.TableExpr)
		if !ok {
			return
		}
		for _, entry := range pathexpr.ObjectEntries(table) {
			slot, ok := callbackProtocolSlotFromSegments(entry.Suffix.Segments)
			if !ok {
				continue
			}
			index, ok := callbackProtocolParamIndex(result, entry.Value, params)
			if !ok {
				continue
			}
			out = append(out, callbackProtocolStore{paramIndex: index, slot: slot, kind: callbackProtocolInvocationStore})
		}
	})
	return out
}

func callbackProtocolParamIndex(result *body.Result, expr ast.Expr, params map[symbol.ID]int) (int, bool) {
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return 0, false
	}
	index, ok := params[p.Symbol]
	return index, ok
}

func callbackProtocolReplayOrders(results []*body.Result) [][]string {
	var out [][]string
	for _, result := range results {
		graph := result.Graph()
		if graph == nil {
			continue
		}
		var order []string
		for _, point := range graph.RPO() {
			call, ok := result.Call(point)
			if !ok {
				continue
			}
			if slot, ok := callbackProtocolCallSlot(result, call); ok {
				order = append(order, slot)
			}
		}
		if len(order) >= 2 {
			out = append(out, order)
		}
	}
	return out
}

func callbackProtocolCallSlot(result *body.Result, call body.CallFact) (string, bool) {
	if (isBareGlobalCall(result, call, "pcall") || isBareGlobalCall(result, call, "xpcall")) && len(call.Args) != 0 {
		if p, ok := result.ExpressionPath(call.Args[0]); ok {
			return callbackProtocolSlotFromSegments(p.Segments)
		}
	}
	if call.HasCalleePath {
		if slot, ok := callbackProtocolSlotFromSegments(call.CalleePath.Segments); ok {
			return slot, true
		}
	}
	if p, ok := result.ExpressionPath(call.Func); ok {
		return callbackProtocolSlotFromSegments(p.Segments)
	}
	return "", false
}

func callbackProtocolPhasesAround(replay []string, index int, storesBySlot map[string][]callbackProtocolStore) ([]string, []string) {
	before := callbackProtocolUniqueStoredSlots(replay[:index], storesBySlot)
	after := callbackProtocolUniqueStoredSlots(replay[index+1:], storesBySlot)
	return before, after
}

func callbackProtocolUniqueStoredSlots(slots []string, storesBySlot map[string][]callbackProtocolStore) []string {
	var out []string
	seen := make(map[string]struct{}, len(slots))
	for _, slot := range slots {
		if len(storesBySlot[slot]) == 0 {
			continue
		}
		if _, ok := seen[slot]; ok {
			continue
		}
		seen[slot] = struct{}{}
		out = append(out, slot)
	}
	return out
}

func callbackProtocolSlotFromSegments(segments []segment.Segment) (string, bool) {
	if len(segments) == 0 {
		return "", false
	}
	last := segments[len(segments)-1]
	if last.Kind != segment.SegmentField && last.Kind != segment.SegmentIndexString {
		return "", false
	}
	return last.Name, last.Name != ""
}

func callbackProtocolWalkStmts(stmts []ast.Stmt, visit func(ast.Expr)) {
	for _, stmt := range stmts {
		callbackProtocolWalkStmt(stmt, visit)
	}
}

func callbackProtocolWalkStmt(stmt ast.Stmt, visit func(ast.Expr)) {
	switch n := stmt.(type) {
	case *ast.AssignStmt:
		for _, expr := range n.Lhs {
			callbackProtocolWalkExpr(expr, visit)
		}
		for _, expr := range n.Rhs {
			callbackProtocolWalkExpr(expr, visit)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range n.Exprs {
			callbackProtocolWalkExpr(expr, visit)
		}
	case *ast.FuncCallStmt:
		callbackProtocolWalkExpr(n.Expr, visit)
	case *ast.DoBlockStmt:
		callbackProtocolWalkStmts(n.Stmts, visit)
	case *ast.WhileStmt:
		callbackProtocolWalkExpr(n.Condition, visit)
		callbackProtocolWalkStmts(n.Stmts, visit)
	case *ast.RepeatStmt:
		callbackProtocolWalkStmts(n.Stmts, visit)
		callbackProtocolWalkExpr(n.Condition, visit)
	case *ast.IfStmt:
		callbackProtocolWalkExpr(n.Condition, visit)
		callbackProtocolWalkStmts(n.Then, visit)
		callbackProtocolWalkStmts(n.Else, visit)
	case *ast.NumberForStmt:
		callbackProtocolWalkExpr(n.Init, visit)
		callbackProtocolWalkExpr(n.Limit, visit)
		callbackProtocolWalkExpr(n.Step, visit)
		callbackProtocolWalkStmts(n.Stmts, visit)
	case *ast.GenericForStmt:
		for _, expr := range n.Exprs {
			callbackProtocolWalkExpr(expr, visit)
		}
		callbackProtocolWalkStmts(n.Stmts, visit)
	case *ast.FuncDefStmt:
		// Nested function bodies are solved as separate body results; do not
		// attribute their local object literals to the enclosing API.
		if n.Name != nil {
			callbackProtocolWalkExpr(n.Name.Func, visit)
			callbackProtocolWalkExpr(n.Name.Receiver, visit)
		}
	case *ast.ReturnStmt:
		for _, expr := range n.Exprs {
			callbackProtocolWalkExpr(expr, visit)
		}
	}
}

func callbackProtocolWalkExpr(expr ast.Expr, visit func(ast.Expr)) {
	if expr == nil {
		return
	}
	visit(expr)
	switch n := expr.(type) {
	case *ast.AttrGetExpr:
		callbackProtocolWalkExpr(n.Object, visit)
		callbackProtocolWalkExpr(n.Key, visit)
	case *ast.TableExpr:
		for _, field := range n.Fields {
			callbackProtocolWalkExpr(field.Key, visit)
			callbackProtocolWalkExpr(field.Value, visit)
		}
	case *ast.FuncCallExpr:
		callbackProtocolWalkExpr(n.Func, visit)
		callbackProtocolWalkExpr(n.Receiver, visit)
		for _, arg := range n.Args {
			callbackProtocolWalkExpr(arg, visit)
		}
	case *ast.LogicalOpExpr:
		callbackProtocolWalkExpr(n.Lhs, visit)
		callbackProtocolWalkExpr(n.Rhs, visit)
	case *ast.RelationalOpExpr:
		callbackProtocolWalkExpr(n.Lhs, visit)
		callbackProtocolWalkExpr(n.Rhs, visit)
	case *ast.StringConcatOpExpr:
		callbackProtocolWalkExpr(n.Lhs, visit)
		callbackProtocolWalkExpr(n.Rhs, visit)
	case *ast.ArithmeticOpExpr:
		callbackProtocolWalkExpr(n.Lhs, visit)
		callbackProtocolWalkExpr(n.Rhs, visit)
	case *ast.UnaryMinusOpExpr:
		callbackProtocolWalkExpr(n.Expr, visit)
	case *ast.UnaryNotOpExpr:
		callbackProtocolWalkExpr(n.Expr, visit)
	case *ast.UnaryLenOpExpr:
		callbackProtocolWalkExpr(n.Expr, visit)
	case *ast.UnaryBNotOpExpr:
		callbackProtocolWalkExpr(n.Expr, visit)
	case *ast.FunctionExpr:
		// Nested function bodies are solved as separate body results.
	}
}
