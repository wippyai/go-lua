package body

import (
	"reflect"
	"strings"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// StatementIdentity is an opaque syntax-owned statement identity. Readmodels
// use it only for equality within one body result.
type StatementIdentity uintptr

// SymbolReadSets maps each CFG point to the symbol ids read at that point.
type SymbolReadSets map[cfg.Point]map[symbol.ID]struct{}

// Has reports whether id is read at any reachable point.
func (s SymbolReadSets) Has(id symbol.ID) bool {
	for _, reads := range s {
		if _, ok := reads[id]; ok {
			return true
		}
	}
	return false
}

// LocalBindingOccurrence is one local binding declaration.
type LocalBindingOccurrence struct {
	Point  cfg.Point
	Symbol symbol.ID
	Name   string
	Span   SourceSpan
}

// DeadAssignmentWriteOccurrence is one local write eligible for dead-assignment
// proof.
type DeadAssignmentWriteOccurrence struct {
	Point     cfg.Point
	Statement StatementIdentity
	Symbol    symbol.ID
	Name      string
	Span      SourceSpan
}

// DeadAssignmentExitOccurrence is one CFG point that exits the function body.
type DeadAssignmentExitOccurrence struct {
	Point cfg.Point
	Span  SourceSpan
}

// LocalBindingOccurrences returns reachable local declarations in RPO order.
func (r *Result) LocalBindingOccurrences() []LocalBindingOccurrence {
	if r == nil || r.Graph() == nil {
		return nil
	}
	var out []LocalBindingOccurrence
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.LocalAssignment(point)
		if !ok || !fact.HasSymbol || ignoredDiagnosticLocalName(fact.Name) {
			continue
		}
		out = append(out, LocalBindingOccurrence{
			Point:  point,
			Symbol: fact.Symbol,
			Name:   fact.Name,
			Span:   sourceSpanFromAST(localNameSourceSpan(fact.Stmt, fact.Index, fact.Name)),
		})
	}
	return out
}

// DeadAssignmentWriteOccurrences returns reachable writes that can participate
// in dead-assignment proof.
func (r *Result) DeadAssignmentWriteOccurrences() []DeadAssignmentWriteOccurrence {
	if r == nil || r.Graph() == nil {
		return nil
	}
	var out []DeadAssignmentWriteOccurrence
	for _, point := range cfg.RPOReadOnly(r.Graph()) {
		if !r.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.LocalAssignment(point); ok {
			write, ok := localDeadAssignmentWriteOccurrence(point, fact)
			if ok {
				out = append(out, write)
			}
			continue
		}
		if fact, ok := r.OrdinaryAssignment(point); ok {
			write, ok := ordinaryDeadAssignmentWriteOccurrence(point, fact)
			if ok {
				out = append(out, write)
			}
		}
	}
	return out
}

func localDeadAssignmentWriteOccurrence(point cfg.Point, fact LocalAssignmentFact) (DeadAssignmentWriteOccurrence, bool) {
	if !fact.HasSymbol || fact.Expr == nil || ignoredDiagnosticLocalName(fact.Name) {
		return DeadAssignmentWriteOccurrence{}, false
	}
	write := sourceSpanFromAST(localNameSourceSpan(fact.Stmt, fact.Index, fact.Name))
	if !sourceSpanValid(write) {
		return DeadAssignmentWriteOccurrence{}, false
	}
	return DeadAssignmentWriteOccurrence{
		Point:     point,
		Statement: statementIdentity(fact.Stmt),
		Symbol:    fact.Symbol,
		Name:      fact.Name,
		Span:      write,
	}, true
}

func ordinaryDeadAssignmentWriteOccurrence(point cfg.Point, fact OrdinaryAssignmentFact) (DeadAssignmentWriteOccurrence, bool) {
	if !fact.HasSymbol || fact.Value == nil {
		return DeadAssignmentWriteOccurrence{}, false
	}
	ident, ok := fact.Target.(*ast.IdentExpr)
	if !ok || ignoredDiagnosticLocalName(ident.Value) {
		return DeadAssignmentWriteOccurrence{}, false
	}
	write := sourceSpanFromAST(ast.SpanOf(ident))
	if !sourceSpanValid(write) {
		write = sourceSpanFromAST(ast.SpanOf(fact.Target))
	}
	if !sourceSpanValid(write) {
		return DeadAssignmentWriteOccurrence{}, false
	}
	return DeadAssignmentWriteOccurrence{
		Point:     point,
		Statement: statementIdentity(fact.Stmt),
		Symbol:    fact.Symbol,
		Name:      ident.Value,
		Span:      write,
	}, true
}

// DeadAssignmentExitOccurrences returns reachable points whose only successors
// are the synthetic function exit.
func (r *Result) DeadAssignmentExitOccurrences() map[cfg.Point]DeadAssignmentExitOccurrence {
	out := make(map[cfg.Point]DeadAssignmentExitOccurrence)
	if r == nil || r.Graph() == nil {
		return out
	}
	graph := r.Graph()
	exit := graph.Exit()
	for _, point := range graph.RPO() {
		if !r.PointNormallyReachable(point) || point == exit {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, point)
		if len(successors) == 0 {
			continue
		}
		allExit := true
		for _, succ := range successors {
			if succ != exit {
				allExit = false
				break
			}
		}
		if !allExit {
			continue
		}
		var span SourceSpan
		if fact, ok := r.ReturnFact(point); ok {
			span = sourceSpanFromAST(ast.SpanOf(fact.Stmt))
		}
		out[point] = DeadAssignmentExitOccurrence{Point: point, Span: span}
	}
	return out
}

// ReachableSymbolReads returns every reachable symbol read, including child
// function captures attributed to the function expression read site.
func (r *Result) ReachableSymbolReads() SymbolReadSets {
	reads := make(SymbolReadSets)
	if r == nil || r.Graph() == nil {
		return reads
	}
	functionCaptures := r.functionCaptureReads()
	add := func(point cfg.Point, id symbol.ID) {
		if id == 0 {
			return
		}
		if reads[point] == nil {
			reads[point] = make(map[symbol.ID]struct{})
		}
		reads[point][id] = struct{}{}
	}
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		collector := symbolReadCollector{result: r, functionCaptures: functionCaptures, add: func(id symbol.ID) { add(point, id) }}
		if fact, ok := r.LocalAssignment(point); ok {
			collector.exprs(fact.Exprs)
			collector.typeExprs(fact.Types)
		}
		if fact, ok := r.OrdinaryAssignment(point); ok {
			collector.exprs(fact.Rhs)
			collector.lvalues(fact.Lhs)
		}
		if fact, ok := r.Call(point); ok {
			collector.expr(fact.Func)
			collector.expr(fact.Receiver)
			collector.exprs(fact.Args)
		}
		if fact, ok := r.ReturnFact(point); ok {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := r.BranchCondition(point); ok {
			collector.expr(fact.Condition)
		}
		if fact, ok := r.NumericFor(point); ok {
			collector.expr(fact.Init)
			collector.expr(fact.Limit)
			collector.expr(fact.Step)
		}
		if fact, ok := r.GenericFor(point); ok && fact.Role == cfgfacts.GenericForRoleCheck {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := r.TypeDefinition(point); ok {
			collector.typeDefinition(fact)
		}
		if fact, ok := r.FunctionDefinition(point); ok {
			collector.functionNameReads(fact.Name)
			collector.expr(fact.Func)
		}
	}
	return reads
}

func (r *Result) functionCaptureReads() map[*ast.FunctionExpr][]symbol.ID {
	sets := make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
	var walk func(*Result) map[symbol.ID]struct{}
	walk = func(parent *Result) map[symbol.ID]struct{} {
		out := make(map[symbol.ID]struct{})
		if parent == nil {
			return out
		}
		for _, child := range parent.FunctionResults() {
			childSet := make(map[symbol.ID]struct{})
			for _, capture := range parent.DirectCaptures(child.Function()) {
				if capture.Captured != 0 {
					childSet[capture.Captured] = struct{}{}
				}
			}
			for id := range walk(child) {
				childSet[id] = struct{}{}
			}
			if len(childSet) > 0 && child.Function() != nil {
				sets[child.Function()] = childSet
			}
			for id := range childSet {
				out[id] = struct{}{}
			}
		}
		return out
	}
	walk(r)
	out := make(map[*ast.FunctionExpr][]symbol.ID, len(sets))
	for fn, set := range sets {
		ids := make([]symbol.ID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		out[fn] = ids
	}
	return out
}

type symbolReadCollector struct {
	result           *Result
	functionCaptures map[*ast.FunctionExpr][]symbol.ID
	add              func(symbol.ID)
}

func (c symbolReadCollector) exprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.expr(expr)
	}
}

func (c symbolReadCollector) typeExprs(exprs []ast.TypeExpr) {
	for _, expr := range exprs {
		c.typeExpr(expr)
	}
}

func (c symbolReadCollector) typeParams(params []ast.TypeParamExpr) {
	for _, param := range params {
		c.typeExpr(param.Constraint)
	}
}

func (c symbolReadCollector) functionParams(params []ast.FunctionParamExpr) {
	for _, param := range params {
		c.typeExpr(param.Type)
	}
}

func (c symbolReadCollector) typeDefinition(fact cfgfacts.TypeDefinitionFact) {
	if fact.Type != nil {
		c.typeParams(fact.Type.TypeParams)
		c.typeExpr(fact.Type.Type)
	}
	if fact.Interface != nil {
		for _, field := range fact.Interface.Fields {
			c.typeExpr(field.Type)
		}
		for _, method := range fact.Interface.Methods {
			if method.Type != nil {
				c.typeExpr(method.Type)
			}
		}
	}
}

func (c symbolReadCollector) functionTypeExprs(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	c.typeParams(fn.TypeParams)
	if fn.ParList != nil {
		c.typeExprs(fn.ParList.Types)
		c.typeExpr(fn.ParList.VarargType)
	}
	c.typeExprs(fn.ReturnTypes)
}

func (c symbolReadCollector) typeExpr(expr ast.TypeExpr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.PrimitiveTypeExpr, *ast.SelfTypeExpr, *ast.LiteralTypeExpr, *ast.TypeRefExpr:
		return
	case *ast.OptionalTypeExpr:
		c.typeExpr(e.Inner)
	case *ast.UnionTypeExpr:
		c.typeExprs(e.Types)
	case *ast.IntersectionTypeExpr:
		c.typeExprs(e.Types)
	case *ast.ArrayTypeExpr:
		c.typeExpr(e.Element)
	case *ast.MapTypeExpr:
		c.typeExpr(e.Key)
		c.typeExpr(e.Value)
	case *ast.RecordTypeExpr:
		for _, field := range e.Fields {
			c.typeExpr(field.Type)
		}
	case *ast.FunctionTypeExpr:
		c.typeParams(e.TypeParams)
		c.functionParams(e.Params)
		c.typeExpr(e.Variadic)
		c.typeExprs(e.Returns)
	case *ast.AssertsTypeExpr:
		c.typeExpr(e.NarrowTo)
	case *ast.GenericTypeExpr:
		c.typeExprs(e.Args)
	case *ast.MetaTypeExpr:
		c.typeExpr(e.Inner)
	case *ast.TupleTypeExpr:
		c.typeExprs(e.Elements)
	case *ast.TypeOfExpr:
		c.expr(e.Expr)
	case *ast.KeyOfExpr:
		c.typeExpr(e.Inner)
	case *ast.IndexAccessExpr:
		c.typeExpr(e.Object)
		c.typeExpr(e.Index)
	case *ast.ConditionalTypeExpr:
		c.typeExpr(e.Check)
		c.typeExpr(e.Extends)
		c.typeExpr(e.Then)
		c.typeExpr(e.Else)
	}
}

func (c symbolReadCollector) lvalues(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.lvalue(expr)
	}
}

func (c symbolReadCollector) lvalue(expr ast.Expr) {
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

func (c symbolReadCollector) functionNameReads(name *ast.FuncName) {
	if name == nil {
		return
	}
	c.lvalue(name.Func)
	c.expr(name.Receiver)
}

func (c symbolReadCollector) expr(expr ast.Expr) {
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
		c.functionTypeExprs(e)
		for _, id := range c.functionCaptures[e] {
			c.add(id)
		}
	}
}

func localNameSourceSpan(stmt *ast.LocalAssignStmt, index int, name string) source.Span {
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
			return source.Span{
				StartLine: pos.Line,
				StartCol:  pos.Column,
				EndLine:   endLine,
				EndCol:    endCol,
			}
		}
	}
	return ast.SpanOf(stmt)
}

func ignoredDiagnosticLocalName(name string) bool {
	return name == "" || strings.HasPrefix(name, "_")
}

func sourceSpanValid(span SourceSpan) bool {
	return span.StartLine != 0 || span.StartCol != 0 || span.EndLine != 0 || span.EndCol != 0
}

func statementIdentity(stmt ast.Stmt) StatementIdentity {
	if stmt == nil {
		return 0
	}
	v := reflect.ValueOf(stmt)
	if v.Kind() != reflect.Pointer && v.Kind() != reflect.UnsafePointer {
		return 0
	}
	return StatementIdentity(v.Pointer())
}
