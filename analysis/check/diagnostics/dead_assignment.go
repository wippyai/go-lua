package diagnostics

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type deadAssignments producerContext

type deadAssignmentWrite struct {
	point  cfg.Point
	stmt   ast.Stmt
	symbol symbol.ID
	name   string
	write  diagnostic.Span
}

type deadAssignmentView struct {
	graph         cfg.Graph
	writes        []deadAssignmentWrite
	writesByPoint map[cfg.Point][]deadAssignmentWrite
	readsByPoint  map[cfg.Point]map[symbol.ID]struct{}
}

func (p deadAssignments) Produce(result *body.Result) []diagnostic.Diagnostic {
	_ = p
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	view := newDeadAssignmentView(result, graph)
	if len(view.writes) == 0 {
		return nil
	}
	bySymbol := make(map[symbol.ID][]deadAssignmentWrite)
	for _, write := range view.writes {
		bySymbol[write.symbol] = append(bySymbol[write.symbol], write)
	}
	var out []diagnostic.Diagnostic
	for _, writes := range bySymbol {
		out = append(out, view.diagnosticsForSymbol(writes)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Span.StartLine != out[j].Span.StartLine {
			return out[i].Span.StartLine < out[j].Span.StartLine
		}
		if out[i].Span.StartCol != out[j].Span.StartCol {
			return out[i].Span.StartCol < out[j].Span.StartCol
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func newDeadAssignmentView(result *body.Result, graph cfg.Graph) deadAssignmentView {
	captured := collectDeadAssignmentCapturedSymbols(result)
	writes := collectDeadAssignmentWrites(result, graph, captured)
	view := deadAssignmentView{
		graph:         graph,
		writes:        writes,
		writesByPoint: make(map[cfg.Point][]deadAssignmentWrite),
		readsByPoint:  collectDeadAssignmentReads(result, graph),
	}
	for _, write := range writes {
		view.writesByPoint[write.point] = append(view.writesByPoint[write.point], write)
	}
	return view
}

func collectDeadAssignmentCapturedSymbols(result *body.Result) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	var walk func(*body.Result)
	walk = func(parent *body.Result) {
		for _, child := range parent.FunctionResults() {
			for _, capture := range parent.DirectCaptures(child.Function()) {
				if capture.Captured != 0 {
					out[capture.Captured] = struct{}{}
				}
			}
			walk(child)
		}
	}
	if result != nil {
		walk(result)
	}
	return out
}

func collectDeadAssignmentWrites(result *body.Result, graph cfg.Graph, captured map[symbol.ID]struct{}) []deadAssignmentWrite {
	var writes []deadAssignmentWrite
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			write, ok := localDeadAssignmentWrite(result, point, fact, captured)
			if ok {
				writes = append(writes, write)
			}
			continue
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			write, ok := ordinaryDeadAssignmentWrite(result, point, fact, captured)
			if ok {
				writes = append(writes, write)
			}
		}
	}
	return writes
}

func localDeadAssignmentWrite(result *body.Result, point cfg.Point, fact semantics.LocalAssignmentFact, captured map[symbol.ID]struct{}) (deadAssignmentWrite, bool) {
	if !fact.HasSymbol || fact.Expr == nil || ignoredUnusedLocalName(fact.Name) {
		return deadAssignmentWrite{}, false
	}
	if _, ok := captured[fact.Symbol]; ok {
		return deadAssignmentWrite{}, false
	}
	if !deadAssignmentSymbolKind(result, fact.Symbol) {
		return deadAssignmentWrite{}, false
	}
	write := localNameSpan(fact.Stmt, fact.Index, fact.Name)
	if !write.Valid() {
		return deadAssignmentWrite{}, false
	}
	return deadAssignmentWrite{
		point:  point,
		stmt:   fact.Stmt,
		symbol: fact.Symbol,
		name:   fact.Name,
		write:  write,
	}, true
}

func ordinaryDeadAssignmentWrite(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact, captured map[symbol.ID]struct{}) (deadAssignmentWrite, bool) {
	if !fact.HasSymbol || fact.Value == nil {
		return deadAssignmentWrite{}, false
	}
	if _, ok := captured[fact.Symbol]; ok {
		return deadAssignmentWrite{}, false
	}
	ident, ok := fact.Target.(*ast.IdentExpr)
	if !ok || ignoredUnusedLocalName(ident.Value) {
		return deadAssignmentWrite{}, false
	}
	if !deadAssignmentSymbolKind(result, fact.Symbol) {
		return deadAssignmentWrite{}, false
	}
	write := ast.SpanOf(ident)
	if !write.Valid() {
		write = ast.SpanOf(fact.Target)
	}
	if !write.Valid() {
		return deadAssignmentWrite{}, false
	}
	return deadAssignmentWrite{
		point:  point,
		stmt:   fact.Stmt,
		symbol: fact.Symbol,
		name:   ident.Value,
		write:  write,
	}, true
}

func deadAssignmentSymbolKind(result *body.Result, id symbol.ID) bool {
	kind, ok := result.SymbolKind(id)
	return ok && (kind == symbol.Local || kind == symbol.Param)
}

func collectDeadAssignmentReads(result *body.Result, graph cfg.Graph) map[cfg.Point]map[symbol.ID]struct{} {
	reads := make(map[cfg.Point]map[symbol.ID]struct{})
	add := func(point cfg.Point, id symbol.ID) {
		if id == 0 {
			return
		}
		if reads[point] == nil {
			reads[point] = make(map[symbol.ID]struct{})
		}
		reads[point][id] = struct{}{}
	}
	for _, point := range graph.RPO() {
		collector := deadAssignmentReadCollector{result: result, add: func(id symbol.ID) { add(point, id) }}
		if fact, ok := result.LocalAssignment(point); ok {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			collector.exprs(fact.Rhs)
			collector.lvalues(fact.Lhs)
		}
		if fact, ok := result.Call(point); ok {
			collector.expr(fact.Func)
			collector.expr(fact.Receiver)
			collector.exprs(fact.Args)
		}
		if fact, ok := result.ReturnFact(point); ok {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := result.BranchCondition(point); ok {
			collector.expr(fact.Condition)
		}
		if fact, ok := result.NumericFor(point); ok {
			collector.expr(fact.Init)
			collector.expr(fact.Limit)
			collector.expr(fact.Step)
		}
		if fact, ok := result.GenericFor(point); ok && fact.Role == cfgfacts.GenericForRoleCheck {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := result.FunctionDefinition(point); ok {
			collector.functionNameReads(fact.Name)
		}
	}
	return reads
}

type deadAssignmentReadCollector struct {
	result *body.Result
	add    func(symbol.ID)
}

func (c deadAssignmentReadCollector) exprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.expr(expr)
	}
}

func (c deadAssignmentReadCollector) lvalues(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.lvalue(expr)
	}
}

func (c deadAssignmentReadCollector) lvalue(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		return
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	default:
		c.expr(expr)
	}
}

func (c deadAssignmentReadCollector) functionNameReads(name *ast.FuncName) {
	if name == nil {
		return
	}
	c.lvalue(name.Func)
	c.expr(name.Receiver)
}

func (c deadAssignmentReadCollector) expr(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		if id, ok := c.result.SymbolOfIdent(e); ok {
			c.add(id)
		}
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.expr(field.Value)
		}
	case *ast.FuncCallExpr:
		c.expr(e.Func)
		c.expr(e.Receiver)
		c.exprs(e.Args)
	case *ast.LogicalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.expr(e.Expr)
	case *ast.CastExpr:
		c.expr(e.Expr)
	case *ast.NonNilAssertExpr:
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		return
	}
}

func (v deadAssignmentView) diagnosticsForSymbol(writes []deadAssignmentWrite) []diagnostic.Diagnostic {
	var out []diagnostic.Diagnostic
	for i, previous := range writes {
		if ambiguousSameStatementWrite(previous, writes) {
			continue
		}
		for j := i + 1; j < len(writes); j++ {
			next := writes[j]
			if previous.stmt == next.stmt || ambiguousSameStatementWrite(next, writes) {
				continue
			}
			if v.mustReachOverwriteBeforeRead(previous, next) {
				out = append(out, deadAssignmentDiagnostic(previous, next))
				break
			}
		}
	}
	return out
}

func ambiguousSameStatementWrite(write deadAssignmentWrite, writes []deadAssignmentWrite) bool {
	if write.stmt == nil {
		return false
	}
	for _, other := range writes {
		if other.point != write.point && other.stmt == write.stmt {
			return true
		}
	}
	return false
}

func (v deadAssignmentView) mustReachOverwriteBeforeRead(previous, next deadAssignmentWrite) bool {
	if v.graph == nil || previous.point == next.point {
		return false
	}
	memo := make(map[cfg.Point]bool)
	visiting := make(map[cfg.Point]bool)
	var walk func(cfg.Point) bool
	walk = func(point cfg.Point) bool {
		if v.pointReadsSymbol(point, previous.symbol) {
			return false
		}
		if point == next.point {
			return true
		}
		if v.pointWritesSymbol(point, previous.symbol) {
			return false
		}
		if point == v.graph.Exit() {
			return false
		}
		if cached, ok := memo[point]; ok {
			return cached
		}
		if visiting[point] {
			memo[point] = false
			return false
		}
		visiting[point] = true
		successors := v.graph.Successors(point)
		ok := len(successors) > 0
		for _, succ := range successors {
			if !walk(succ) {
				ok = false
				break
			}
		}
		delete(visiting, point)
		memo[point] = ok
		return ok
	}
	successors := v.graph.Successors(previous.point)
	if len(successors) == 0 {
		return false
	}
	for _, succ := range successors {
		if !walk(succ) {
			return false
		}
	}
	return true
}

func (v deadAssignmentView) pointReadsSymbol(point cfg.Point, id symbol.ID) bool {
	reads := v.readsByPoint[point]
	if len(reads) == 0 {
		return false
	}
	_, ok := reads[id]
	return ok
}

func (v deadAssignmentView) pointWritesSymbol(point cfg.Point, id symbol.ID) bool {
	for _, write := range v.writesByPoint[point] {
		if write.symbol == id {
			return true
		}
	}
	return false
}

func localNameSpan(stmt *ast.LocalAssignStmt, index int, name string) diagnostic.Span {
	if stmt != nil && index >= 0 && index < len(stmt.NamePositions) {
		pos := stmt.NamePositions[index]
		if pos.Valid() {
			endLine, endCol := pos.EndLine, pos.EndColumn
			if endLine == 0 {
				endLine = pos.Line
			}
			if endCol == 0 {
				endCol = pos.Column + len(name)
			}
			return diagnostic.Span{
				StartLine: pos.Line,
				StartCol:  pos.Column,
				EndLine:   endLine,
				EndCol:    endCol,
			}
		}
	}
	return ast.SpanOf(stmt)
}

func deadAssignmentDiagnostic(previous, next deadAssignmentWrite) diagnostic.Diagnostic {
	message := fmt.Sprintf("assigned value for %q is overwritten before it is read", previous.name)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      previous.write.StartLine,
			Column:    previous.write.StartCol,
			EndLine:   previous.write.EndLine,
			EndColumn: previous.write.EndCol,
		},
		Span:     previous.write,
		Code:     CodeDeadAssignment,
		Severity: diagnostic.SeverityWarning,
		Message:  message,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    previous.write,
				Message: fmt.Sprintf("%q receives a value here", previous.name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    next.write,
				Message: fmt.Sprintf("%q is assigned again on every path before a read", previous.name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    next.write,
				Message: "CFG proof requires this overwriting assignment before any bound read",
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnostic.TrustProven,
				Span:    previous.write,
				Message: fmt.Sprintf("no bound read of %q exists between these writes", previous.name),
			},
		),
		Labels: []diagnostic.Label{
			{Span: previous.write, Message: "overwritten value"},
			{Span: next.write, Message: "overwriting assignment"},
		},
		Help: "Remove the first value assignment, or keep only its side effects before the overwriting write.",
	}
}
