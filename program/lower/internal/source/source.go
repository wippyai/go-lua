// Package source owns source-grammar dispatch and body sequencing for canonical
// Program construction. Semantic verticals own every judgment after dispatch.
package source

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	calllower "github.com/wippyai/go-lua/program/lower/internal/call"
	"github.com/wippyai/go-lua/program/lower/internal/control"
	"github.com/wippyai/go-lua/program/lower/internal/eval"
	functionlower "github.com/wippyai/go-lua/program/lower/internal/function"
	"github.com/wippyai/go-lua/program/lower/internal/inbox"
	"github.com/wippyai/go-lua/program/lower/internal/lexical"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
	staticlower "github.com/wippyai/go-lua/program/lower/internal/static"
	"github.com/wippyai/go-lua/program/lower/internal/store"
	tablelower "github.com/wippyai/go-lua/program/lower/internal/table"
	"github.com/wippyai/go-lua/program/source"
)

// Owner is the sole source-grammar authority for one unfinished Program.
// phase.Source denotes only Owner's private append continuation below; every
// public crossing has its own typed inbox and distinct phase token.
type Owner struct {
	name        string
	phases      *phase.Stack
	expressions *inbox.Expressions
	bodies      *inbox.Bodies
	statics     *inbox.Statics
	scopes      *lexical.Bodies
	controls    *control.Writer
	values      *eval.Values
	storage     *store.Writer
	static      *staticlower.Writer
	calls       *calllower.Writer
	functions   *functionlower.Writer
	tables      *tablelower.Writer
	steps       []step
}

type step struct {
	body keyspace.Term
	span source.Span
}

func New(
	name string,
	phases *phase.Stack,
	expressions *inbox.Expressions,
	bodies *inbox.Bodies,
	statics *inbox.Statics,
	scopes *lexical.Bodies,
	controls *control.Writer,
	values *eval.Values,
	storage *store.Writer,
	static *staticlower.Writer,
	calls *calllower.Writer,
	functions *functionlower.Writer,
	tables *tablelower.Writer,
) *Owner {
	return &Owner{
		name: name, phases: phases, expressions: expressions, bodies: bodies,
		statics: statics, scopes: scopes, controls: controls, values: values,
		storage: storage, static: static, calls: calls, functions: functions,
		tables: tables,
	}
}

// Begin creates and schedules the root lexical Body.
func (o *Owner) Begin(statements []ast.Stmt) (keyspace.Term, error) {
	if err := o.ready(); err != nil {
		return 0, err
	}
	span := o.chunkSpan(statements)
	entry, err := o.scopes.Entry(span)
	if err != nil {
		return 0, err
	}
	if err := o.ScheduleBody(statements, entry, span); err != nil {
		return 0, err
	}
	return entry, nil
}

// ScheduleBody schedules one complete lexical body in prepare, source, close
// order. All three requests carry the already-created Body identity.
func (o *Owner) ScheduleBody(statements []ast.Stmt, body keyspace.Term, span source.Span) error {
	if err := o.ready(); err != nil {
		return err
	}
	if err := o.bodies.PushClose(body, span); err != nil {
		return err
	}
	if err := o.bodies.PushStatements(statements, 0, body, span); err != nil {
		return err
	}
	return o.bodies.PushPrepare(statements, body, span)
}

// Drain executes the one owner-token stack to completion.
func (o *Owner) Drain() error {
	if err := o.ready(); err != nil {
		return err
	}
	for {
		owner, ok := o.phases.Pop()
		if !ok {
			return nil
		}
		var err error
		switch owner {
		case phase.Source:
			err = o.runPrivate()
		case phase.Lexical:
			err = o.scopes.Run()
		case phase.Eval:
			err = o.values.Run()
		case phase.Store:
			err = o.storage.Run()
		case phase.Control:
			err = o.controls.Run()
		case phase.Call:
			err = o.calls.Run()
		case phase.Static:
			err = o.static.Run()
		case phase.Function:
			err = o.functions.Run()
		case phase.Table:
			err = o.tables.Run()
		case phase.SyntaxExpression:
			err = o.runExpression()
		case phase.SyntaxStatements:
			err = o.runStatements()
		case phase.BodyPrepare:
			err = o.runPrepare()
		case phase.BodyClose:
			err = o.runClose()
		case phase.StaticType:
			err = o.runStaticType()
		case phase.StaticDeclaredCellType:
			err = o.runDeclaredCellType()
		default:
			err = fmt.Errorf("programlower: unknown phase owner %d", owner)
		}
		if err != nil {
			return err
		}
	}
}

func (o *Owner) runStatements() error {
	request, err := o.bodies.PopStatements()
	if err != nil {
		return err
	}
	if request.Body != o.scopes.Owner() {
		return fmt.Errorf("programlower: statement Body is not active")
	}
	if request.Index == len(request.Statements) {
		return nil
	}
	statement := request.Statements[request.Index]
	if statement == nil {
		return fmt.Errorf("programlower: absent statement")
	}
	if err := o.bodies.PushStatements(
		request.Statements, request.Index+1, request.Body, request.Span,
	); err != nil {
		return err
	}
	return o.statement(statement, request.Body)
}

func (o *Owner) statement(statement ast.Stmt, body keyspace.Term) error {
	switch node := statement.(type) {
	case *ast.LocalAssignStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent local declaration")
		}
		if node.LocalFunction {
			if len(node.Exprs) != 1 {
				return fmt.Errorf("programlower: recursive local has no Function expression")
			}
			fn, ok := node.Exprs[0].(*ast.FunctionExpr)
			if !ok || fn == nil {
				return fmt.Errorf("programlower: recursive local has invalid Function expression")
			}
			return o.functions.ScheduleRecursiveLocal(node, body, o.span(fn), o.span(node))
		}
		return o.scopes.ScheduleLocal(node, body, o.span(node))
	case *ast.AssignStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent assignment")
		}
		return o.storage.ScheduleAssignment(node, body, o.span(node))
	case *ast.FuncDefStmt:
		if node == nil || node.Func == nil {
			return fmt.Errorf("programlower: absent function definition")
		}
		return o.functions.ScheduleDef(node, body, o.span(node.Func), o.span(node))
	case *ast.FuncCallStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent call statement")
		}
		o.pushAppend(body, o.span(node))
		return o.expressions.Push(node.Expr, body, o.expressionSpan(node.Expr, o.span(node)))
	case *ast.DoBlockStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent do block")
		}
		nested, err := o.scopes.EnterBlock(o.span(node))
		if err != nil {
			return err
		}
		o.pushAppend(body, o.span(node))
		return o.ScheduleBody(node.Stmts, nested, o.chunkSpan(node.Stmts))
	case *ast.TypeDefStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent type alias")
		}
		return o.static.ScheduleAlias(node, body, o.span(node))
	case *ast.InterfaceDefStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent interface declaration")
		}
		return o.static.ScheduleInterface(node, body, o.span(node))
	case *ast.ReturnStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent return")
		}
		return o.controls.ScheduleReturn(node, body, o.span(node))
	case *ast.BreakStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent break")
		}
		return o.controls.ScheduleBreak(node, body, o.span(node))
	case *ast.LabelStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent label")
		}
		return o.controls.ScheduleLabel(node, body, o.span(node))
	case *ast.GotoStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent goto")
		}
		return o.controls.ScheduleGoto(node, body, o.span(node))
	case *ast.IfStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent if")
		}
		return o.controls.ScheduleIf(node, body, o.span(node))
	case *ast.WhileStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent while")
		}
		return o.controls.ScheduleWhile(node, body, o.span(node))
	case *ast.RepeatStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent repeat")
		}
		return o.controls.ScheduleRepeat(node, body, o.span(node))
	case *ast.NumberForStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent numeric for")
		}
		return o.controls.ScheduleNumberFor(node, body, o.span(node))
	case *ast.GenericForStmt:
		if node == nil {
			return fmt.Errorf("programlower: absent generic for")
		}
		return o.controls.ScheduleGenericFor(node, body, o.span(node))
	default:
		return fmt.Errorf("programlower: unsupported statement %T", statement)
	}
}

func (o *Owner) runExpression() error {
	request, err := o.expressions.Pop()
	if err != nil {
		return err
	}
	if request.Host != o.scopes.Owner() {
		return fmt.Errorf("programlower: expression request crossed Body boundary")
	}
	return o.expression(request)
}

func (o *Owner) expression(request inbox.Expression) error {
	switch node := request.Expr.(type) {
	case *ast.TrueExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.FalseExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.NilExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.NumberExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.StringExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.Comma3Expr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.scopes.ScheduleVararg(node, request.Host, request.Span)
	case *ast.LogicalOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.RelationalOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.StringConcatOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.ArithmeticOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.UnaryMinusOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.UnaryNotOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.UnaryLenOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.UnaryBNotOpExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.CastExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.NonNilAssertExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.values.ScheduleExpression(node, request.Host, request.Span)
	case *ast.IdentExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.storage.ScheduleExpression(node, request.Host, request.Span)
	case *ast.AttrGetExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.storage.ScheduleExpression(node, request.Host, request.Span)
	case *ast.FuncCallExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.calls.Schedule(node, request.Host, request.Span)
	case *ast.FunctionExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.functions.ScheduleExpr(node, request.Host, request.Span)
	case *ast.TableExpr:
		if node == nil {
			return absentExpression(request.Expr)
		}
		return o.tables.Schedule(node, request.Host, request.Span)
	default:
		return fmt.Errorf("programlower: unsupported expression %T at %v", request.Expr, request.Span)
	}
}

func (o *Owner) runPrepare() error {
	request, err := o.bodies.PopPrepare()
	if err != nil {
		return err
	}
	if request.Body != o.scopes.Owner() {
		return fmt.Errorf("programlower: prepared Body is not active")
	}
	if err := o.controls.Predeclare(request.Statements, request.Body); err != nil {
		return err
	}
	return o.static.Predeclare(request.Body, request.Statements)
}

func (o *Owner) runClose() error {
	request, err := o.bodies.PopClose()
	if err != nil {
		return err
	}
	if request.Body != o.scopes.Owner() {
		return fmt.Errorf("programlower: closed Body is not active")
	}
	if err := o.controls.ResolveFaults(request.Body, o.scopes); err != nil {
		return err
	}
	body, err := o.scopes.Finish()
	if err != nil {
		return err
	}
	if body != request.Body {
		return fmt.Errorf("programlower: lexical Body changed during close")
	}
	o.phases.SetResult(body, false)
	return nil
}

func (o *Owner) runStaticType() error {
	request, err := o.statics.PopType()
	if err != nil {
		return err
	}
	if request.Body != o.scopes.Owner() {
		return fmt.Errorf("programlower: static type request crossed Body boundary")
	}
	return o.static.ScheduleType(request.Type, request.Host, request.Body, request.Span)
}

func (o *Owner) runDeclaredCellType() error {
	request, err := o.statics.PopDeclaredCell()
	if err != nil {
		return err
	}
	if request.Body != o.scopes.Owner() {
		return fmt.Errorf("programlower: declared Cell type request crossed Body boundary")
	}
	return o.static.ScheduleDeclaredCellType(request.Type, request.Cell, request.Body, request.Span)
}

func (o *Owner) pushAppend(body keyspace.Term, span source.Span) {
	o.steps = append(o.steps, step{body: body, span: span})
	o.phases.Push(phase.Source)
}

func (o *Owner) runPrivate() error {
	if len(o.steps) == 0 {
		return fmt.Errorf("programlower: source token has no private continuation")
	}
	last := len(o.steps) - 1
	current := o.steps[last]
	o.steps = o.steps[:last]
	if current.body != o.scopes.Owner() {
		return fmt.Errorf("programlower: completed statement Body is not active")
	}
	term, _ := o.phases.Result()
	if term == 0 {
		return fmt.Errorf("programlower: invalid statement result at %v", current.span)
	}
	return o.scopes.Append(term)
}

func (o *Owner) Clean() bool {
	return o != nil && o.phases.Clean() && o.expressions.Clean() && o.bodies.Clean() &&
		o.statics.Clean() && len(o.steps) == 0 && o.scopes.Clean() &&
		o.controls.Clean() && o.values.Clean() && o.storage.Clean() &&
		o.static.Clean() && o.calls.Clean() && o.functions.Clean() && o.tables.Clean()
}

func (o *Owner) ready() error {
	if o == nil || o.name == "" || o.phases == nil || o.expressions == nil ||
		o.bodies == nil || o.statics == nil || o.scopes == nil || o.controls == nil ||
		o.values == nil || o.storage == nil || o.static == nil || o.calls == nil ||
		o.functions == nil || o.tables == nil {
		return fmt.Errorf("programlower: invalid source authority")
	}
	return nil
}

func (o *Owner) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: o.name}
	}
	span, ok := sourcecoord.Build(o.name, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return sourcecoord.Invalid(o.name)
	}
	return span
}

func (o *Owner) chunkSpan(statements []ast.Stmt) source.Span {
	span := source.Span{File: o.name}
	if len(statements) == 0 {
		return span
	}
	first, firstOK := concreteStatement(statements[0])
	last, lastOK := concreteStatement(statements[len(statements)-1])
	if !firstOK || !lastOK {
		return sourcecoord.Invalid(o.name)
	}
	result, ok := sourcecoord.Build(o.name, first.Line(), first.Column(), last.LastLine(), last.LastColumn())
	if !ok {
		return sourcecoord.Invalid(o.name)
	}
	return result
}

func (o *Owner) expressionSpan(expression ast.Expr, fallback source.Span) source.Span {
	span, ok := inbox.ExpressionSpan(expression, o.name)
	if !ok {
		return fallback
	}
	return span
}

func absentExpression(expr ast.Expr) error {
	return fmt.Errorf("programlower: absent expression %T", expr)
}

func concreteStatement(statement ast.Stmt) (ast.PositionHolder, bool) {
	switch node := statement.(type) {
	case *ast.AssignStmt:
		return node, node != nil
	case *ast.LocalAssignStmt:
		return node, node != nil
	case *ast.FuncCallStmt:
		return node, node != nil
	case *ast.DoBlockStmt:
		return node, node != nil
	case *ast.WhileStmt:
		return node, node != nil
	case *ast.RepeatStmt:
		return node, node != nil
	case *ast.IfStmt:
		return node, node != nil
	case *ast.NumberForStmt:
		return node, node != nil
	case *ast.GenericForStmt:
		return node, node != nil
	case *ast.FuncDefStmt:
		return node, node != nil
	case *ast.ReturnStmt:
		return node, node != nil
	case *ast.BreakStmt:
		return node, node != nil
	case *ast.LabelStmt:
		return node, node != nil
	case *ast.GotoStmt:
		return node, node != nil
	case *ast.TypeDefStmt:
		return node, node != nil
	case *ast.InterfaceDefStmt:
		return node, node != nil
	default:
		return nil, false
	}
}
