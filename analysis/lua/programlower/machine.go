// Package programlower lowers parser AST plus binder identity into Program.
package programlower

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/programlower/internal/control"
	"github.com/wippyai/go-lua/analysis/lua/programlower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/programlower/internal/lexical"
	"github.com/wippyai/go-lua/analysis/lua/programlower/internal/store"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program"
)

// Lower converts one parsed and bound source chunk into a sealed Program.
// Unsupported syntax fails explicitly; no generic operation is substituted.
func Lower(sourceName string, stmts []ast.Stmt, binding *bind.Result) (*program.Program, error) {
	if binding == nil {
		return nil, fmt.Errorf("programlower: nil binding result")
	}
	if issues := binding.ControlIssues(); len(issues) != 0 {
		return nil, fmt.Errorf(
			"programlower: binding has %d invalid control statement(s)",
			len(issues),
		)
	}

	captures := make(captureIndex)
	binding.ForEachEntryCapture(captures.add)
	builder := program.NewBuilder()
	l := lowerer{
		sourceName: sourceName,
		binding:    binding,
		builder:    builder,
		captures:   captures,
		scopes:     lexical.New(builder),
		controls:   control.New(builder),
		packs:      eval.New(builder),
		access:     store.New(builder),
	}

	span := l.chunkSpan(stmts)
	_, err := l.scopes.Entry(span)
	if err != nil {
		return nil, err
	}
	if err := l.predeclareLabels(stmts); err != nil {
		return nil, err
	}
	l.push(
		step{kind: stepFinishEntry, span: span},
		step{kind: stepStmts, stmts: stmts},
	)
	if err := l.run(); err != nil {
		return nil, err
	}
	if !l.scopes.Clean() || !l.controls.Clean() ||
		!l.packs.Clean() || !l.access.Clean() {
		return nil, fmt.Errorf("programlower: unfinished assembly scratch")
	}
	sealed, err := builder.Seal()
	if err != nil {
		return nil, fmt.Errorf("programlower: seal: %w", err)
	}
	return sealed, nil
}

type captureIndex map[*ast.FunctionExpr][]bind.Capture

func (c captureIndex) add(fn *ast.FunctionExpr, capture bind.Capture) bool {
	c[fn] = append(c[fn], capture)
	return true
}

type lowerer struct {
	sourceName string
	binding    *bind.Result
	builder    *program.Builder
	captures   captureIndex

	scopes   lexical.Bodies
	controls control.Writer
	packs    eval.Values
	access   store.Access
	steps    []step

	result program.Term
}

type stepKind uint8

const (
	stepStmts stepKind = iota + 1
	stepFinishEntry
	stepFinishDo
	stepFinishLocal
	stepTargets
	stepTarget
	stepAppendTarget
	stepFinishAssign
	stepFuncDefFunction
	stepFinishFuncDef
	stepFinishCallStmt
	stepFinishReturn
	stepFinishIfCondition
	stepFinishIfThen
	stepFinishIfElse
	stepFinishWhileCondition
	stepFinishLoopBody
	stepFinishRepeat
	stepNumberControls
	stepFinishGenericControls
	stepValues
	stepAppendValue
	stepExpr
	stepFinishUnary
	stepFinishBinaryLeft
	stepFinishBinary
	stepFinishLensBase
	stepFinishLens
	stepFunctionParams
	stepFunctionCaptures
	stepFinishFunctionBody
	stepFinishCallCallee
	stepFinishCall
	stepTableFields
	stepFinishTableKey
	stepFinishTableValue
)

// step is a closed, phase-private instruction. Its AST fields keep the
// lowering schema explicit; cursors retain one list element of work at a time.
type step struct {
	kind stepKind

	stmts []ast.Stmt
	exprs []ast.Expr
	expr  ast.Expr
	node  ast.PositionHolder

	local   *ast.LocalAssignStmt
	assign  *ast.AssignStmt
	funcdef *ast.FuncDefStmt
	return_ *ast.ReturnStmt
	if_     *ast.IfStmt
	call    *ast.FuncCallExpr
	attr    *ast.AttrGetExpr
	fn      *ast.FunctionExpr
	table   *ast.TableExpr

	slots    []bind.ParamSlot
	captures []bind.Capture

	span      program.Span
	condition program.Term
	whenTrue  program.Term
	whenFalse program.Term
	unary     program.UnaryOp
	binary    program.BinaryOp
	select_   program.SelectOp

	index     int
	ordinal   int
	mark      int
	valueMark int
	kindMark  int

	readLens  bool
	allowOpen bool
}

func (l *lowerer) run() error {
	for len(l.steps) != 0 {
		last := len(l.steps) - 1
		current := l.steps[last]
		l.steps = l.steps[:last]

		switch current.kind {
		case stepStmts:
			if err := l.runStmts(current); err != nil {
				return err
			}
		case stepFinishEntry:
			if _, err := l.finishBody(); err != nil {
				return err
			}
		case stepFinishDo:
			child, err := l.finishBody()
			if err != nil {
				return err
			}
			if err := l.scopes.Append(child); err != nil {
				return err
			}
		case stepFinishLocal:
			if err := l.finishLocal(current); err != nil {
				return err
			}
		case stepTargets:
			if err := l.runTargets(current); err != nil {
				return err
			}
		case stepTarget:
			if err := l.runTarget(current.expr); err != nil {
				return err
			}
		case stepAppendTarget:
			l.access.RememberTarget(l.result)
		case stepFinishAssign:
			term, err := l.access.Assign(
				l.span(current.assign),
				l.owner(),
				current.mark,
				l.result,
			)
			if err != nil {
				return err
			}
			if err := l.scopes.Append(term); err != nil {
				return err
			}
		case stepFuncDefFunction:
			if err := l.startFunctionBody(current.funcdef.Func); err != nil {
				return err
			}
		case stepFinishFuncDef:
			mark := l.packs.Hold(l.result)
			values, err := l.packs.Fixed(
				l.span(current.funcdef),
				l.owner(),
				mark,
				1,
			)
			if err != nil {
				return err
			}
			term, err := l.access.Assign(
				l.span(current.funcdef),
				l.owner(),
				current.mark,
				values,
			)
			if err != nil {
				return err
			}
			if err := l.scopes.Append(term); err != nil {
				return err
			}
		case stepFinishCallStmt:
			if err := l.scopes.Append(l.result); err != nil {
				return err
			}
		case stepFinishReturn:
			term, err := l.controls.Return(
				l.span(current.return_),
				l.owner(),
				l.result,
			)
			if err != nil {
				return err
			}
			if err := l.scopes.Append(term); err != nil {
				return err
			}
		case stepFinishIfCondition:
			if err := l.beginIfThen(current.if_, l.result); err != nil {
				return err
			}
		case stepFinishIfThen:
			if err := l.finishIfThen(current); err != nil {
				return err
			}
		case stepFinishIfElse:
			if err := l.finishIfElse(current); err != nil {
				return err
			}
		case stepFinishWhileCondition:
			stmt, ok := current.node.(*ast.WhileStmt)
			if !ok || stmt == nil {
				return fmt.Errorf("programlower: invalid while continuation")
			}
			if err := l.beginWhileBody(stmt, l.result); err != nil {
				return err
			}
		case stepFinishLoopBody:
			if err := l.finishLoopBody(current); err != nil {
				return err
			}
		case stepFinishRepeat:
			current.condition = l.result
			if err := l.finishLoopBody(current); err != nil {
				return err
			}
		case stepNumberControls:
			if err := l.runNumberControls(current); err != nil {
				return err
			}
		case stepFinishGenericControls:
			stmt, ok := current.node.(*ast.GenericForStmt)
			if !ok || stmt == nil {
				return fmt.Errorf("programlower: invalid generic for continuation")
			}
			if err := l.beginGenericForBody(stmt, l.result); err != nil {
				return err
			}
		case stepValues:
			if err := l.runValues(current); err != nil {
				return err
			}
		case stepAppendValue:
			l.packs.Append(l.result)
		case stepExpr:
			if err := l.runExpr(current.expr); err != nil {
				return err
			}
		case stepFinishUnary:
			l.result = l.builder.Unary(current.span, l.owner(), current.unary, l.result)
			if l.result == 0 {
				return fmt.Errorf("programlower: could not create unary operation")
			}
		case stepFinishBinaryLeft:
			if mark := l.packs.Hold(l.result); mark != current.mark {
				return fmt.Errorf("programlower: invalid left operand mark")
			}
			l.push(
				step{
					kind:    stepFinishBinary,
					span:    current.span,
					binary:  current.binary,
					select_: current.select_,
					mark:    current.mark,
				},
				step{kind: stepExpr, expr: current.expr},
			)
		case stepFinishBinary:
			left, err := l.packs.Take(current.mark)
			if err != nil {
				return fmt.Errorf("programlower: missing left operand")
			}
			right := l.result
			if current.select_ != 0 {
				l.result = l.builder.Select(current.span, l.owner(), current.select_, left, right)
			} else {
				l.result = l.builder.Binary(current.span, l.owner(), current.binary, left, right)
			}
			if l.result == 0 {
				if current.select_ != 0 {
					return fmt.Errorf("programlower: could not create logical selection")
				}
				return fmt.Errorf("programlower: could not create binary operation")
			}
		case stepFinishLensBase:
			if err := l.finishLensBase(current); err != nil {
				return err
			}
		case stepFinishLens:
			if err := l.finishLens(current, l.result); err != nil {
				return err
			}
		case stepFunctionParams:
			if err := l.runFunctionParams(current); err != nil {
				return err
			}
		case stepFunctionCaptures:
			if err := l.runFunctionCaptures(current); err != nil {
				return err
			}
		case stepFinishFunctionBody:
			function, err := l.packs.Take(current.mark)
			if err != nil {
				return fmt.Errorf("programlower: missing Function result")
			}
			if _, err := l.finishBody(); err != nil {
				return err
			}
			l.result = function
		case stepFinishCallCallee:
			if mark := l.packs.Hold(l.result); mark != current.mark {
				return fmt.Errorf("programlower: invalid Call callee mark")
			}
			l.push(
				step{kind: stepFinishCall, call: current.call, mark: current.mark},
				step{
					kind:  stepValues,
					exprs: current.call.Args,
					span:  l.span(current.call),
					mark:  l.packs.Mark(),
				},
			)
		case stepFinishCall:
			callee, err := l.packs.Take(current.mark)
			if err != nil {
				return fmt.Errorf("programlower: missing Call callee")
			}
			l.result = l.builder.Call(l.span(current.call), l.owner(), callee, 0, l.result)
			if l.result == 0 {
				return fmt.Errorf("programlower: could not create Call")
			}
		case stepTableFields:
			if err := l.runTableFields(current); err != nil {
				return err
			}
		case stepFinishTableKey:
			key, ok := current.node.(ast.Expr)
			if !ok || !astNodePresent(key) {
				return fmt.Errorf("programlower: invalid table key continuation")
			}
			l.access.KeyField(l.result, key)
			l.push(
				step{
					kind:      stepFinishTableValue,
					allowOpen: current.allowOpen,
					expr:      current.expr,
				},
				step{kind: stepExpr, expr: current.expr},
			)
		case stepFinishTableValue:
			values, err := l.packs.Field(
				l.span(current.expr),
				l.owner(),
				current.expr,
				l.result,
				current.allowOpen,
			)
			if err != nil {
				return err
			}
			l.access.FieldValues(values)
		default:
			return fmt.Errorf("programlower: invalid lowering step %d", current.kind)
		}
	}
	return nil
}

func (l *lowerer) runStmts(current step) error {
	if current.index == len(current.stmts) {
		return nil
	}
	if current.index < 0 || current.index > len(current.stmts) {
		return fmt.Errorf("programlower: invalid statement cursor")
	}
	stmt := current.stmts[current.index]
	if !astNodePresent(stmt) {
		return fmt.Errorf("programlower: absent statement %T", stmt)
	}
	l.push(step{kind: stepStmts, stmts: current.stmts, index: current.index + 1})

	switch stmt := stmt.(type) {
	case *ast.LocalAssignStmt:
		if len(stmt.Names) == 0 {
			return fmt.Errorf("programlower: local assignment has no declarations")
		}
		for i, declared := range stmt.Types {
			if declared != nil {
				return fmt.Errorf("programlower: unsupported declared type for local slot %d", i)
			}
		}
		for i, expr := range stmt.Exprs {
			if fn, ambiguous := expr.(*ast.FunctionExpr); ambiguous && fn != nil {
				return fmt.Errorf(
					"programlower: unsupported function initializer for local slot %d: recursive-local syntax was erased",
					i,
				)
			}
		}
		l.push(
			step{kind: stepFinishLocal, local: stmt, mark: l.scopes.CellMark()},
			step{kind: stepValues, exprs: stmt.Exprs, span: l.span(stmt), mark: l.packs.Mark()},
		)
	case *ast.AssignStmt:
		if len(stmt.Lhs) == 0 {
			return fmt.Errorf("programlower: assignment has no targets")
		}
		l.push(step{kind: stepTargets, assign: stmt, mark: l.access.TargetMark()})
	case *ast.FuncCallStmt:
		call, ok := stmt.Expr.(*ast.FuncCallExpr)
		if !ok || call == nil {
			return fmt.Errorf("programlower: call statement does not contain a call")
		}
		l.push(
			step{kind: stepFinishCallStmt},
			step{kind: stepExpr, expr: call},
		)
	case *ast.FuncDefStmt:
		return l.startFuncDef(stmt)
	case *ast.ReturnStmt:
		l.push(
			step{kind: stepFinishReturn, return_: stmt},
			step{kind: stepValues, exprs: stmt.Exprs, span: l.span(stmt), mark: l.packs.Mark()},
		)
	case *ast.IfStmt:
		l.push(
			step{kind: stepFinishIfCondition, if_: stmt},
			step{kind: stepExpr, expr: stmt.Condition},
		)
	case *ast.WhileStmt:
		l.push(
			step{kind: stepFinishWhileCondition, node: stmt},
			step{kind: stepExpr, expr: stmt.Condition},
		)
	case *ast.RepeatStmt:
		return l.startRepeat(stmt)
	case *ast.NumberForStmt:
		return l.startNumberFor(stmt)
	case *ast.GenericForStmt:
		return l.startGenericFor(stmt)
	case *ast.BreakStmt:
		term, err := l.controls.Break(l.span(stmt), l.owner())
		if err != nil {
			return err
		}
		return l.scopes.Append(term)
	case *ast.LabelStmt:
		return l.controls.PlaceLabel(stmt, l.scopes.Cursor())
	case *ast.GotoStmt:
		target, ok := l.binding.GotoTarget(stmt)
		if !ok {
			return fmt.Errorf("programlower: binder has no legal target for goto")
		}
		term, err := l.controls.Goto(l.span(stmt), l.owner(), target)
		if err != nil {
			return err
		}
		return l.scopes.Append(term)
	case *ast.DoBlockStmt:
		span := l.span(stmt)
		_, err := l.scopes.EnterBlock(span)
		if err != nil {
			return fmt.Errorf("programlower: could not create do-block body: %w", err)
		}
		if err := l.predeclareLabels(stmt.Stmts); err != nil {
			return err
		}
		l.push(
			step{kind: stepFinishDo, span: span},
			step{kind: stepStmts, stmts: stmt.Stmts},
		)
	default:
		return l.unsupportedStmt(stmt)
	}
	return nil
}

func (l *lowerer) beginIfThen(stmt *ast.IfStmt, condition program.Term) error {
	span := l.chunkSpan(stmt.Then)
	body, err := l.scopes.EnterBlock(span)
	if err != nil {
		return fmt.Errorf("programlower: could not create Then Body: %w", err)
	}
	if err := l.predeclareLabels(stmt.Then); err != nil {
		return err
	}
	l.push(
		step{
			kind:      stepFinishIfThen,
			if_:       stmt,
			condition: condition,
			whenTrue:  body,
			span:      span,
		},
		step{kind: stepStmts, stmts: stmt.Then},
	)
	return nil
}

func (l *lowerer) finishIfThen(current step) error {
	if l.owner() != current.whenTrue {
		return fmt.Errorf("programlower: mismatched Then Body")
	}
	_, err := l.finishBody()
	if err != nil {
		return err
	}

	span := l.chunkSpan(current.if_.Else)
	body, err := l.scopes.EnterBlock(span)
	if err != nil {
		return fmt.Errorf("programlower: could not create Else Body: %w", err)
	}
	if err := l.predeclareLabels(current.if_.Else); err != nil {
		return err
	}
	l.push(
		step{
			kind:      stepFinishIfElse,
			if_:       current.if_,
			condition: current.condition,
			whenTrue:  current.whenTrue,
			whenFalse: body,
			span:      span,
		},
		step{kind: stepStmts, stmts: current.if_.Else},
	)
	return nil
}

func (l *lowerer) finishIfElse(current step) error {
	if l.owner() != current.whenFalse {
		return fmt.Errorf("programlower: mismatched Else Body")
	}
	_, err := l.finishBody()
	if err != nil {
		return err
	}

	term, err := l.controls.Branch(
		l.span(current.if_),
		l.owner(),
		current.condition,
		current.whenTrue,
		current.whenFalse,
	)
	if err != nil {
		return err
	}
	return l.scopes.Append(term)
}

func (l *lowerer) beginWhileBody(stmt *ast.WhileStmt, control program.Term) error {
	bodySpan := l.chunkSpan(stmt.Stmts)
	body, err := l.scopes.EnterBlock(bodySpan)
	if err != nil {
		return fmt.Errorf("programlower: could not create while Body: %w", err)
	}
	if err := l.predeclareLabels(stmt.Stmts); err != nil {
		return err
	}
	l.push(
		step{
			kind:      stepFinishLoopBody,
			node:      stmt,
			span:      bodySpan,
			whenTrue:  body,
			condition: control,
			mark:      l.controls.CellMark(),
		},
		step{kind: stepStmts, stmts: stmt.Stmts},
	)
	return nil
}

func (l *lowerer) startRepeat(stmt *ast.RepeatStmt) error {
	bodySpan := l.chunkSpan(stmt.Stmts)
	body, err := l.scopes.EnterBlock(bodySpan)
	if err != nil {
		return fmt.Errorf("programlower: could not create repeat Body: %w", err)
	}
	if err := l.predeclareLabels(stmt.Stmts); err != nil {
		return err
	}
	l.push(
		step{
			kind:     stepFinishRepeat,
			node:     stmt,
			span:     bodySpan,
			whenTrue: body,
			mark:     l.controls.CellMark(),
		},
		step{kind: stepExpr, expr: stmt.Condition},
		step{kind: stepStmts, stmts: stmt.Stmts},
	)
	return nil
}

func (l *lowerer) startNumberFor(stmt *ast.NumberForStmt) error {
	if !astNodePresent(stmt.Init) {
		return fmt.Errorf("programlower: absent numeric for initializer %T", stmt.Init)
	}
	if !astNodePresent(stmt.Limit) {
		return fmt.Errorf("programlower: absent numeric for limit %T", stmt.Limit)
	}
	if stmt.Step != nil && !astNodePresent(stmt.Step) {
		return fmt.Errorf("programlower: absent numeric for step %T", stmt.Step)
	}
	l.push(step{
		kind: stepNumberControls,
		node: stmt,
		mark: l.packs.Mark(),
	})
	return nil
}

func (l *lowerer) runNumberControls(current step) error {
	stmt, ok := current.node.(*ast.NumberForStmt)
	if !ok || stmt == nil {
		return fmt.Errorf("programlower: invalid numeric for continuation")
	}
	count := 2
	if stmt.Step != nil {
		count = 3
	}
	if current.index == count {
		control, err := l.packs.Fixed(
			l.span(stmt),
			l.owner(),
			current.mark,
			count,
		)
		if err != nil {
			return err
		}
		return l.beginNumberForBody(stmt, control)
	}
	if current.index < 0 || current.index > count {
		return fmt.Errorf("programlower: invalid numeric for control cursor")
	}
	var expr ast.Expr
	switch current.index {
	case 0:
		expr = stmt.Init
	case 1:
		expr = stmt.Limit
	case 2:
		expr = stmt.Step
	}
	current.index++
	l.push(
		current,
		step{kind: stepAppendValue},
		step{kind: stepExpr, expr: expr},
	)
	return nil
}

func (l *lowerer) beginNumberForBody(
	stmt *ast.NumberForStmt,
	control program.Term,
) error {
	id, ok := l.binding.NumForSymbol(stmt)
	if !ok || id == 0 {
		return fmt.Errorf("programlower: binder has no symbol for numeric for variable")
	}
	bodySpan := l.chunkSpan(stmt.Stmts)
	body, err := l.scopes.EnterBlock(bodySpan)
	if err != nil {
		return fmt.Errorf("programlower: could not create numeric for Body: %w", err)
	}
	if err := l.predeclareLabels(stmt.Stmts); err != nil {
		return err
	}
	cellMark := l.controls.CellMark()
	cell, err := l.scopes.DeclareLoop(id, l.positionSpan(stmt.NamePosition))
	if err != nil {
		return fmt.Errorf("programlower: could not create numeric for Cell: %w", err)
	}
	if err := l.controls.RememberCell(cell); err != nil {
		return err
	}
	l.push(
		step{
			kind:      stepFinishLoopBody,
			node:      stmt,
			span:      bodySpan,
			whenTrue:  body,
			condition: control,
			mark:      cellMark,
		},
		step{kind: stepStmts, stmts: stmt.Stmts},
	)
	return nil
}

func (l *lowerer) startGenericFor(stmt *ast.GenericForStmt) error {
	if len(stmt.Names) == 0 {
		return fmt.Errorf("programlower: generic for has no variables")
	}
	if len(stmt.Exprs) == 0 {
		return fmt.Errorf("programlower: generic for has no iterator expressions")
	}
	l.push(
		step{kind: stepFinishGenericControls, node: stmt},
		step{
			kind:  stepValues,
			exprs: stmt.Exprs,
			span:  l.span(stmt),
			mark:  l.packs.Mark(),
		},
	)
	return nil
}

func (l *lowerer) beginGenericForBody(
	stmt *ast.GenericForStmt,
	control program.Term,
) error {
	ids := l.binding.GenericForSymbols(stmt)
	if len(ids) != len(stmt.Names) {
		return fmt.Errorf("programlower: binder has incomplete generic for symbols")
	}
	bodySpan := l.chunkSpan(stmt.Stmts)
	body, err := l.scopes.EnterBlock(bodySpan)
	if err != nil {
		return fmt.Errorf("programlower: could not create generic for Body: %w", err)
	}
	if err := l.predeclareLabels(stmt.Stmts); err != nil {
		return err
	}
	cellMark := l.controls.CellMark()
	for i, id := range ids {
		if id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for generic for variable %d", i)
		}
		var span program.Span
		if i < len(stmt.NamePositions) {
			span = l.positionSpan(stmt.NamePositions[i])
		} else {
			span = l.span(stmt)
		}
		cell, err := l.scopes.DeclareLoop(id, span)
		if err != nil {
			return fmt.Errorf("programlower: could not create generic for Cell %d: %w", i, err)
		}
		if err := l.controls.RememberCell(cell); err != nil {
			return err
		}
	}
	l.push(
		step{
			kind:      stepFinishLoopBody,
			node:      stmt,
			span:      bodySpan,
			whenTrue:  body,
			condition: control,
			mark:      cellMark,
		},
		step{kind: stepStmts, stmts: stmt.Stmts},
	)
	return nil
}

func (l *lowerer) finishLoopBody(current step) error {
	if l.owner() != current.whenTrue {
		return fmt.Errorf("programlower: mismatched loop Body")
	}
	body, err := l.finishBody()
	if err != nil {
		return err
	}
	var kind program.LoopKind
	switch current.node.(type) {
	case *ast.WhileStmt:
		kind = program.LoopWhile
	case *ast.RepeatStmt:
		kind = program.LoopRepeat
	case *ast.NumberForStmt:
		kind = program.LoopNumericFor
	case *ast.GenericForStmt:
		kind = program.LoopGenericFor
	default:
		return fmt.Errorf("programlower: invalid loop continuation %T", current.node)
	}
	term, err := l.controls.Loop(
		l.span(current.node),
		l.owner(),
		body,
		current.condition,
		current.mark,
		kind,
	)
	if err != nil {
		return err
	}
	return l.scopes.Append(term)
}

func (l *lowerer) finishLocal(current step) error {
	stmt := current.local
	for i := range stmt.Names {
		id, ok := l.binding.LocalSymbolAt(stmt, i)
		if !ok || id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for local slot %d", i)
		}
		if l.scopes.Has(id) {
			return fmt.Errorf("programlower: duplicate binder symbol for local slot %d", i)
		}
		if _, err := l.scopes.Declare(id, l.nameSpan(stmt, i)); err != nil {
			return fmt.Errorf("programlower: could not create local cell %d", i)
		}
	}
	return l.scopes.Bind(current.mark, l.span(stmt), l.result)
}

func (l *lowerer) runTargets(current step) error {
	if current.index == len(current.assign.Lhs) {
		l.push(
			step{kind: stepFinishAssign, assign: current.assign, mark: current.mark},
			step{
				kind:  stepValues,
				exprs: current.assign.Rhs,
				span:  l.span(current.assign),
				mark:  l.packs.Mark(),
			},
		)
		return nil
	}
	if current.index < 0 || current.index > len(current.assign.Lhs) {
		return fmt.Errorf("programlower: invalid assignment-target cursor")
	}
	expr := current.assign.Lhs[current.index]
	if !astNodePresent(expr) {
		return fmt.Errorf("programlower: absent assignment target %T", expr)
	}
	l.push(
		step{
			kind:   stepTargets,
			assign: current.assign,
			index:  current.index + 1,
			mark:   current.mark,
		},
		step{kind: stepAppendTarget},
		step{kind: stepTarget, expr: expr},
	)
	return nil
}

func (l *lowerer) runTarget(expr ast.Expr) error {
	switch expr := expr.(type) {
	case *ast.IdentExpr:
		id, ok := l.binding.SymbolOf(expr)
		if !ok || id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for identifier target")
		}
		cell, ok := l.scopes.Resolve(id)
		if !ok {
			return fmt.Errorf("programlower: unsupported non-local identifier target")
		}
		l.result = cell
	case *ast.AttrGetExpr:
		l.startLens(expr, false)
	default:
		return l.unsupportedExpr(expr)
	}
	return nil
}

func (l *lowerer) runValues(current step) error {
	if current.index == len(current.exprs) {
		var err error
		l.result, err = l.packs.Pack(current.span, l.owner(), current.mark, current.exprs)
		return err
	}
	if current.index < 0 || current.index > len(current.exprs) {
		return fmt.Errorf("programlower: invalid value-list cursor")
	}
	expr := current.exprs[current.index]
	if !astNodePresent(expr) {
		return fmt.Errorf(
			"programlower: absent expression in value list at index %d: %T",
			current.index,
			expr,
		)
	}
	l.push(
		step{
			kind:  stepValues,
			exprs: current.exprs,
			span:  current.span,
			index: current.index + 1,
			mark:  current.mark,
		},
		step{kind: stepAppendValue},
		step{kind: stepExpr, expr: expr},
	)
	return nil
}

func (l *lowerer) runExpr(expr ast.Expr) error {
	if !astNodePresent(expr) {
		return fmt.Errorf("programlower: absent expression %T", expr)
	}
	span := l.span(expr)
	var err error
	switch expr := expr.(type) {
	case *ast.NilExpr:
		l.result = l.builder.Nil(span, l.owner())
	case *ast.TrueExpr:
		l.result = l.builder.Bool(span, l.owner(), true)
	case *ast.FalseExpr:
		l.result = l.builder.Bool(span, l.owner(), false)
	case *ast.NumberExpr:
		l.result, err = l.packs.Number(span, l.owner(), expr.Value)
	case *ast.StringExpr:
		l.result = l.builder.String(span, l.owner(), expr.Value)
	case *ast.IdentExpr:
		id, ok := l.binding.SymbolOf(expr)
		if !ok || id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for identifier occurrence")
		}
		cell, ok := l.scopes.Resolve(id)
		if !ok {
			return fmt.Errorf("programlower: unsupported non-local identifier binding")
		}
		l.result = l.builder.Read(span, l.owner(), cell)
	case *ast.Comma3Expr:
		cell, resolveErr := l.scopes.Vararg(l.binding)
		if resolveErr != nil {
			return resolveErr
		}
		l.result = l.builder.Vararg(span, l.owner(), cell)
	case *ast.AttrGetExpr:
		l.startLens(expr, true)
		return nil
	case *ast.UnaryMinusOpExpr:
		l.startUnary(span, program.UnaryNeg, expr.Expr)
		return nil
	case *ast.UnaryNotOpExpr:
		l.startUnary(span, program.UnaryNot, expr.Expr)
		return nil
	case *ast.UnaryLenOpExpr:
		l.startUnary(span, program.UnaryLen, expr.Expr)
		return nil
	case *ast.UnaryBNotOpExpr:
		l.startUnary(span, program.UnaryBitNot, expr.Expr)
		return nil
	case *ast.ArithmeticOpExpr:
		op, ok := arithmeticOp(expr.Operator)
		if !ok {
			return fmt.Errorf("programlower: unsupported arithmetic operator %q", expr.Operator)
		}
		l.startBinary(span, op, 0, expr.Lhs, expr.Rhs)
		return nil
	case *ast.StringConcatOpExpr:
		l.startBinary(span, program.BinaryConcat, 0, expr.Lhs, expr.Rhs)
		return nil
	case *ast.RelationalOpExpr:
		op, ok := relationalOp(expr.Operator)
		if !ok {
			return fmt.Errorf("programlower: unsupported relational operator %q", expr.Operator)
		}
		l.startBinary(span, op, 0, expr.Lhs, expr.Rhs)
		return nil
	case *ast.LogicalOpExpr:
		op, ok := selectOp(expr.Operator)
		if !ok {
			return fmt.Errorf("programlower: unsupported logical operator %q", expr.Operator)
		}
		l.startBinary(span, 0, op, expr.Lhs, expr.Rhs)
		return nil
	case *ast.FunctionExpr:
		return l.startFunction(expr)
	case *ast.FuncCallExpr:
		return l.startCall(expr)
	case *ast.TableExpr:
		keyMark, valueMark, kindMark := l.access.TableMark()
		l.push(step{
			kind:      stepTableFields,
			table:     expr,
			mark:      keyMark,
			valueMark: valueMark,
			kindMark:  kindMark,
		})
		return nil
	default:
		return l.unsupportedExpr(expr)
	}
	if err != nil {
		return err
	}
	if l.result == 0 {
		return fmt.Errorf("programlower: could not lower expression %T", expr)
	}
	return nil
}

func (l *lowerer) startUnary(span program.Span, op program.UnaryOp, operand ast.Expr) {
	l.push(
		step{kind: stepFinishUnary, span: span, unary: op},
		step{kind: stepExpr, expr: operand},
	)
}

func (l *lowerer) startBinary(
	span program.Span,
	binary program.BinaryOp,
	select_ program.SelectOp,
	left ast.Expr,
	right ast.Expr,
) {
	l.push(
		step{
			kind:    stepFinishBinaryLeft,
			span:    span,
			binary:  binary,
			select_: select_,
			expr:    right,
			mark:    l.packs.Mark(),
		},
		step{kind: stepExpr, expr: left},
	)
}

func (l *lowerer) startLens(attr *ast.AttrGetExpr, read bool) {
	l.push(
		step{
			kind:     stepFinishLensBase,
			attr:     attr,
			span:     l.span(attr),
			mark:     l.packs.Mark(),
			readLens: read,
		},
		step{kind: stepExpr, expr: attr.Object},
	)
}

func (l *lowerer) finishLensBase(current step) error {
	if mark := l.packs.Hold(l.result); mark != current.mark {
		return fmt.Errorf("programlower: invalid Lens base mark")
	}
	if !astNodePresent(current.attr.Key) {
		return fmt.Errorf("programlower: absent attribute key %T", current.attr.Key)
	}
	switch current.attr.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := current.attr.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf("programlower: dot attribute key is not a string literal")
		}
		base, err := l.packs.Take(current.mark)
		if err != nil {
			return fmt.Errorf("programlower: missing Lens base")
		}
		lens, err := l.access.DotLens(
			current.span,
			l.owner(),
			base,
			l.span(name),
			name.Value,
		)
		if err != nil {
			return err
		}
		l.result = lens
		if current.readLens {
			l.result = l.builder.Read(current.span, l.owner(), lens)
			if l.result == 0 {
				return fmt.Errorf("programlower: could not read Lens")
			}
		}
		return nil
	case ast.AttrKeyIndex:
		l.push(
			step{
				kind:     stepFinishLens,
				attr:     current.attr,
				span:     current.span,
				mark:     current.mark,
				readLens: current.readLens,
			},
			step{kind: stepExpr, expr: current.attr.Key},
		)
	case ast.AttrKeyUnknown:
		return fmt.Errorf("programlower: unsupported attribute with unknown key syntax")
	default:
		return fmt.Errorf(
			"programlower: unsupported attribute key syntax %d",
			current.attr.KeySyntax,
		)
	}
	return nil
}

func (l *lowerer) finishLens(current step, key program.Term) error {
	base, err := l.packs.Take(current.mark)
	if err != nil {
		return fmt.Errorf("programlower: missing Lens base")
	}
	lens, err := l.access.IndexLens(
		current.span,
		l.owner(),
		base,
		key,
		current.attr.Key,
	)
	if err != nil {
		return err
	}
	l.result = lens
	if current.readLens {
		l.result = l.builder.Read(current.span, l.owner(), lens)
		if l.result == 0 {
			return fmt.Errorf("programlower: could not read Lens")
		}
	}
	return nil
}

func (l *lowerer) startFuncDef(stmt *ast.FuncDefStmt) error {
	if stmt == nil || stmt.Name == nil || !astNodePresent(stmt.Func) {
		return fmt.Errorf("programlower: invalid function definition")
	}
	if stmt.Name.Method != "" || stmt.Name.Receiver != nil {
		if stmt.Name.Method == "" || stmt.Name.Receiver == nil || stmt.Name.Func != nil {
			return fmt.Errorf("programlower: invalid method function definition shape")
		}
		return fmt.Errorf(
			"programlower: unsupported method function definition: AST has no MethodPosition evidence",
		)
	}
	if !astNodePresent(stmt.Name.Func) {
		return fmt.Errorf("programlower: invalid function definition target")
	}
	if !funcDefTargetShape(stmt.Name.Func) {
		return fmt.Errorf("programlower: unsupported function definition target shape")
	}
	origin, ok := l.binding.FunctionOrigin(stmt.Func)
	if !ok || origin.Kind != bind.FunctionOriginDeclaration ||
		origin.Func != stmt.Func || origin.Stmt != stmt {
		return fmt.Errorf("programlower: unsupported ambiguous function declaration origin")
	}

	mark := l.access.TargetMark()
	l.push(
		step{kind: stepFinishFuncDef, funcdef: stmt, mark: mark},
		step{kind: stepFuncDefFunction, funcdef: stmt},
		step{kind: stepAppendTarget},
		step{kind: stepTarget, expr: stmt.Name.Func},
	)
	return nil
}

func funcDefTargetShape(target ast.Expr) bool {
	for astNodePresent(target) {
		switch current := target.(type) {
		case *ast.IdentExpr:
			return current.Value != ""
		case *ast.AttrGetExpr:
			if current.KeySyntax != ast.AttrKeyDot ||
				!astNodePresent(current.Object) ||
				!astNodePresent(current.Key) {
				return false
			}
			name, ok := current.Key.(*ast.StringExpr)
			if !ok || name.Value == "" {
				return false
			}
			target = current.Object
		default:
			return false
		}
	}
	return false
}

func (l *lowerer) startFunction(fn *ast.FunctionExpr) error {
	origin, ok := l.binding.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginLiteral {
		return fmt.Errorf("programlower: unsupported ambiguous function origin")
	}
	return l.startFunctionBody(fn)
}

func (l *lowerer) startFunctionBody(fn *ast.FunctionExpr) error {
	if len(fn.TypeParams) != 0 || len(fn.ReturnTypes) != 0 {
		return fmt.Errorf("programlower: unsupported typed function")
	}
	slots := l.binding.ParamSlots(fn)
	for _, slot := range slots {
		if slot.Type != nil {
			return fmt.Errorf("programlower: unsupported typed function parameter %q", slot.Name)
		}
	}
	if fn.ParList != nil && fn.ParList.VarargType != nil {
		return fmt.Errorf("programlower: unsupported typed function vararg")
	}
	span := l.span(fn)
	if _, err := l.scopes.EnterFunction(span, fn); err != nil {
		return fmt.Errorf("programlower: could not create function Body")
	}
	if err := l.predeclareLabels(fn.Stmts); err != nil {
		return err
	}
	l.push(step{
		kind:      stepFunctionParams,
		fn:        fn,
		slots:     slots,
		captures:  l.captures[fn],
		mark:      l.scopes.CellMark(),
		valueMark: l.scopes.CaptureMark(),
	})
	return nil
}

func (l *lowerer) runFunctionParams(current step) error {
	if current.index == len(current.slots) {
		l.push(step{
			kind:      stepFunctionCaptures,
			fn:        current.fn,
			slots:     current.slots,
			captures:  current.captures,
			mark:      current.mark,
			valueMark: current.valueMark,
		})
		return nil
	}
	if current.index < 0 || current.index > len(current.slots) {
		return fmt.Errorf("programlower: invalid function-parameter cursor")
	}
	slot := current.slots[current.index]
	if slot.Symbol == 0 {
		return fmt.Errorf("programlower: binder has no symbol for function formal %q", slot.Name)
	}
	if l.scopes.Has(slot.Symbol) {
		return fmt.Errorf("programlower: duplicate binder symbol for function formal %q", slot.Name)
	}
	if _, err := l.scopes.Declare(slot.Symbol, l.positionSpan(slot.Position)); err != nil {
		return fmt.Errorf("programlower: could not create function formal Cell")
	}
	current.index++
	l.push(current)
	return nil
}

func (l *lowerer) runFunctionCaptures(current step) error {
	if current.index == len(current.captures) {
		return l.beginFunctionBody(current)
	}
	if current.index < 0 || current.index > len(current.captures) {
		return fmt.Errorf("programlower: invalid function-capture cursor")
	}
	capture := current.captures[current.index]
	outer, exists := l.scopes.Resolve(capture.Captured)
	if !exists {
		return fmt.Errorf(
			"programlower: missing outer Cell for capture %q",
			capture.CapturedName,
		)
	}
	if _, err := l.scopes.Capture(capture.Captured, l.span(current.fn), outer); err != nil {
		return fmt.Errorf("programlower: could not create capture Cell")
	}
	current.index++
	l.push(current)
	return nil
}

func (l *lowerer) beginFunctionBody(current step) error {
	varargIndex := -1
	for i, slot := range current.slots {
		if !slot.Vararg {
			continue
		}
		if varargIndex >= 0 {
			return fmt.Errorf("programlower: function has multiple vararg Cells")
		}
		if i != len(current.slots)-1 {
			return fmt.Errorf("programlower: function vararg Cell is not final")
		}
		varargIndex = i
	}
	function, err := l.scopes.Function(
		l.span(current.fn),
		current.mark,
		current.valueMark,
		varargIndex,
	)
	if err != nil {
		return err
	}

	functionMark := l.packs.Hold(function)
	l.push(
		step{kind: stepFinishFunctionBody, mark: functionMark, span: l.span(current.fn)},
		step{kind: stepStmts, stmts: current.fn.Stmts},
	)
	return nil
}

func (l *lowerer) startCall(call *ast.FuncCallExpr) error {
	if len(call.TypeArgs) != 0 {
		return fmt.Errorf("programlower: unsupported typed call")
	}
	if call.Method != "" || call.Receiver != nil {
		if call.Method == "" || call.Receiver == nil || call.Func != nil {
			return fmt.Errorf("programlower: invalid method call shape")
		}
		return fmt.Errorf("programlower: unsupported method call: AST has no MethodPosition evidence")
	}
	if !astNodePresent(call.Func) {
		return fmt.Errorf("programlower: plain call has no callee")
	}
	mark := l.packs.Mark()
	l.push(
		step{kind: stepFinishCallCallee, call: call, mark: mark},
		step{kind: stepExpr, expr: call.Func},
	)
	return nil
}

func (l *lowerer) runTableFields(current step) error {
	if current.index == len(current.table.Fields) {
		var err error
		l.result, err = l.access.Table(
			l.span(current.table),
			l.owner(),
			current.mark,
			current.valueMark,
			current.kindMark,
		)
		return err
	}
	if current.index < 0 || current.index > len(current.table.Fields) {
		return fmt.Errorf("programlower: invalid table-field cursor")
	}
	index := current.index
	field := current.table.Fields[index]
	if field == nil || !astNodePresent(field.Value) {
		return fmt.Errorf("programlower: absent table field value %d", index)
	}
	next := current
	next.index++
	allowOpen := field.Key == nil && index == len(current.table.Fields)-1

	if field.Key == nil {
		current.ordinal++
		if err := l.access.ListField(
			program.Span{File: l.sourceName},
			l.owner(),
			int64(current.ordinal),
		); err != nil {
			return fmt.Errorf("programlower: could not create table list key %d", index)
		}
		next.ordinal = current.ordinal
		l.push(
			next,
			step{kind: stepFinishTableValue, expr: field.Value, allowOpen: allowOpen},
			step{kind: stepExpr, expr: field.Value},
		)
		return nil
	}
	if !astNodePresent(field.Key) {
		return fmt.Errorf("programlower: absent table field key %d", index)
	}
	switch field.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := field.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf(
				"programlower: table field %d dot key is not a string literal",
				index,
			)
		}
		if err := l.access.NameField(l.span(name), l.owner(), name.Value); err != nil {
			return fmt.Errorf("programlower: could not create table field Name %d", index)
		}
		l.push(
			next,
			step{kind: stepFinishTableValue, expr: field.Value},
			step{kind: stepExpr, expr: field.Value},
		)
	case ast.AttrKeyIndex:
		l.push(
			next,
			step{
				kind:      stepFinishTableKey,
				expr:      field.Value,
				node:      field.Key,
				allowOpen: false,
			},
			step{kind: stepExpr, expr: field.Key},
		)
	case ast.AttrKeyUnknown:
		return fmt.Errorf(
			"programlower: unsupported table field %d with unknown key syntax",
			index,
		)
	default:
		return fmt.Errorf(
			"programlower: unsupported table field %d key syntax %d",
			index,
			field.KeySyntax,
		)
	}
	return nil
}

func (l *lowerer) push(steps ...step) {
	l.steps = append(l.steps, steps...)
}

func (l *lowerer) finishBody() (program.Term, error) {
	return l.scopes.Finish()
}

func (l *lowerer) predeclareLabels(stmts []ast.Stmt) error {
	owner := l.owner()
	for _, stmt := range stmts {
		label, ok := stmt.(*ast.LabelStmt)
		if !ok {
			continue
		}
		if !astNodePresent(label) {
			return fmt.Errorf("programlower: absent statement %T", stmt)
		}
		if err := l.controls.PredeclareLabel(label, l.span(label), owner); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) owner() program.Term {
	return l.scopes.Owner()
}

func arithmeticOp(operator string) (program.BinaryOp, bool) {
	switch operator {
	case "+":
		return program.BinaryAdd, true
	case "-":
		return program.BinarySub, true
	case "*":
		return program.BinaryMul, true
	case "/":
		return program.BinaryDiv, true
	case "//":
		return program.BinaryIDiv, true
	case "%":
		return program.BinaryMod, true
	case "^":
		return program.BinaryPow, true
	case "&":
		return program.BinaryBitAnd, true
	case "|":
		return program.BinaryBitOr, true
	case "~":
		return program.BinaryBitXor, true
	case "<<":
		return program.BinaryShiftLeft, true
	case ">>":
		return program.BinaryShiftRight, true
	default:
		return 0, false
	}
}

func relationalOp(operator string) (program.BinaryOp, bool) {
	switch operator {
	case "==":
		return program.BinaryEqual, true
	case "~=":
		return program.BinaryNotEqual, true
	case "<":
		return program.BinaryLess, true
	case "<=":
		return program.BinaryLessEqual, true
	case ">":
		return program.BinaryGreater, true
	case ">=":
		return program.BinaryGreaterEqual, true
	default:
		return 0, false
	}
}

func selectOp(operator string) (program.SelectOp, bool) {
	switch operator {
	case "and":
		return program.SelectAnd, true
	case "or":
		return program.SelectOr, true
	default:
		return 0, false
	}
}

// astNodePresent is the sole AST pointer-presence boundary for lowering.
// AST interfaces can contain typed nil pointers. PositionHolder is the
// canonical shared AST boundary, so newly added pointer node families inherit
// this check without duplicating an AST schema in the lowerer.
func astNodePresent(node ast.PositionHolder) bool {
	if node == nil {
		return false
	}
	value := reflect.ValueOf(node)
	return value.Kind() != reflect.Ptr || !value.IsNil()
}

func (l *lowerer) unsupportedStmt(stmt ast.Stmt) error {
	if !astNodePresent(stmt) {
		return fmt.Errorf("programlower: absent statement %T", stmt)
	}
	return fmt.Errorf(
		"programlower: unsupported statement %T at %d:%d",
		stmt,
		stmt.Line(),
		stmt.Column(),
	)
}

func (l *lowerer) unsupportedExpr(expr ast.Expr) error {
	if !astNodePresent(expr) {
		return fmt.Errorf("programlower: absent expression %T", expr)
	}
	return fmt.Errorf(
		"programlower: unsupported expression %T at %d:%d",
		expr,
		expr.Line(),
		expr.Column(),
	)
}

func (l *lowerer) span(holder ast.PositionHolder) program.Span {
	if holder == nil {
		return program.Span{File: l.sourceName}
	}
	endLine, endCol := holder.LastLine(), holder.LastColumn()
	if endLine <= 0 || endCol <= 0 {
		endLine, endCol = 0, 0
	}
	return program.Span{
		File:      l.sourceName,
		StartLine: holder.Line(),
		StartCol:  holder.Column(),
		EndLine:   endLine,
		EndCol:    endCol,
	}
}

func (l *lowerer) nameSpan(stmt *ast.LocalAssignStmt, index int) program.Span {
	if index >= 0 && index < len(stmt.NamePositions) {
		pos := stmt.NamePositions[index]
		endLine, endCol := pos.EndLine, pos.EndColumn
		if endLine <= 0 || endCol <= 0 {
			endLine, endCol = 0, 0
		}
		return program.Span{
			File:      l.sourceName,
			StartLine: pos.Line,
			StartCol:  pos.Column,
			EndLine:   endLine,
			EndCol:    endCol,
		}
	}
	return l.span(stmt)
}

func (l *lowerer) positionSpan(pos ast.Position) program.Span {
	if !pos.Valid() {
		return program.Span{File: l.sourceName}
	}
	endLine, endCol := pos.EndLine, pos.EndColumn
	if endLine <= 0 || endCol <= 0 {
		endLine, endCol = 0, 0
	}
	return program.Span{
		File:      l.sourceName,
		StartLine: pos.Line,
		StartCol:  pos.Column,
		EndLine:   endLine,
		EndCol:    endCol,
	}
}

func (l *lowerer) chunkSpan(stmts []ast.Stmt) program.Span {
	if len(stmts) == 0 {
		return program.Span{File: l.sourceName}
	}
	first, last := stmts[0], stmts[len(stmts)-1]
	if !astNodePresent(first) || !astNodePresent(last) {
		return program.Span{File: l.sourceName}
	}
	endLine, endCol := last.LastLine(), last.LastColumn()
	if endLine <= 0 || endCol <= 0 {
		endLine, endCol = 0, 0
	}
	return program.Span{
		File:      l.sourceName,
		StartLine: first.Line(),
		StartCol:  first.Column(),
		EndLine:   endLine,
		EndCol:    endCol,
	}
}
