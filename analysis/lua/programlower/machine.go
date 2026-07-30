// Package programlower lowers parser AST plus binder identity into Program.
package programlower

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
	"github.com/wippyai/go-lua/program"
)

// Lower converts one parsed and bound source chunk into a sealed Program.
// Unsupported syntax fails explicitly; no generic operation is substituted.
func Lower(sourceName string, stmts []ast.Stmt, binding *bind.Result) (*program.Program, error) {
	if binding == nil {
		return nil, fmt.Errorf("programlower: nil binding result")
	}

	captures := make(captureIndex)
	binding.ForEachEntryCapture(captures.add)
	l := lowerer{
		sourceName: sourceName,
		binding:    binding,
		builder:    program.NewBuilder(),
		active:     make(map[symbol.ID]program.Term),
		captures:   captures,
	}

	span := l.chunkSpan(stmts)
	entry := l.builder.Body(span)
	if entry == 0 {
		return nil, fmt.Errorf("programlower: could not create chunk body")
	}
	if !l.builder.SetEntry(entry) {
		return nil, fmt.Errorf("programlower: could not set chunk Entry")
	}
	l.enterBody(entry, nil, span)
	l.push(
		step{kind: stepFinishEntry},
		step{kind: stepStmts, stmts: stmts},
	)
	if err := l.run(); err != nil {
		return nil, err
	}
	if len(l.bodies) != 0 {
		return nil, fmt.Errorf("programlower: unfinished lexical body")
	}
	if len(l.active) != 0 || len(l.activeUndo) != 0 {
		return nil, fmt.Errorf("programlower: unfinished lexical mappings")
	}
	if len(l.roots) != 0 || len(l.values) != 0 || len(l.targets) != 0 ||
		len(l.tableKeys) != 0 || len(l.tableValues) != 0 || len(l.tableKinds) != 0 ||
		len(l.capturePairs) != 0 {
		return nil, fmt.Errorf("programlower: unfinished assembly scratch")
	}
	sealed, err := l.builder.Seal()
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

	active     map[symbol.ID]program.Term
	activeUndo []activeUndo
	bodies     []bodyFrame
	steps      []step

	result program.Term

	roots        []program.Term
	values       []program.Term
	targets      []program.Term
	tableKeys    []program.Term
	tableValues  []program.Term
	tableKinds   []program.FieldKind
	capturePairs []program.Capture
}

type activeUndo struct {
	id      symbol.ID
	prior   program.Term
	existed bool
}

type bodyFrame struct {
	body       program.Term
	function   *ast.FunctionExpr
	span       program.Span
	undoMark   int
	rootMark   int
	terminated bool
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
	stepFinishCallStmt
	stepFinishReturn
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
	key   ast.Expr

	local   *ast.LocalAssignStmt
	assign  *ast.AssignStmt
	return_ *ast.ReturnStmt
	call    *ast.FuncCallExpr
	attr    *ast.AttrGetExpr
	fn      *ast.FunctionExpr
	table   *ast.TableExpr

	slots    []bind.ParamSlot
	captures []bind.Capture

	span    program.Span
	unary   program.UnaryOp
	binary  program.BinaryOp
	select_ program.SelectOp

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
			child := l.currentBody().body
			terminated, err := l.finishBody()
			if err != nil {
				return err
			}
			if err := l.appendRoot(child); err != nil {
				return err
			}
			if terminated {
				l.currentBody().terminated = true
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
			l.targets = append(l.targets, l.result)
		case stepFinishAssign:
			term := l.builder.Assign(
				l.span(current.assign),
				l.owner(),
				l.targets[current.mark:],
				l.result,
			)
			l.targets = l.targets[:current.mark]
			if term == 0 {
				return fmt.Errorf("programlower: could not lower assignment")
			}
			if err := l.appendRoot(term); err != nil {
				return err
			}
		case stepFinishCallStmt:
			if err := l.appendRoot(l.result); err != nil {
				return err
			}
		case stepFinishReturn:
			term := l.builder.Return(l.span(current.return_), l.owner(), l.result)
			if term == 0 {
				return fmt.Errorf("programlower: could not lower return")
			}
			if err := l.appendRoot(term); err != nil {
				return err
			}
			l.currentBody().terminated = true
		case stepValues:
			if err := l.runValues(current); err != nil {
				return err
			}
		case stepAppendValue:
			l.values = append(l.values, l.result)
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
			l.values = append(l.values, l.result)
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
			if current.mark < 0 || current.mark >= len(l.values) {
				return fmt.Errorf("programlower: missing left operand")
			}
			left, right := l.values[current.mark], l.result
			if current.select_ != 0 {
				l.result = l.builder.Select(current.span, l.owner(), current.select_, left, right)
			} else {
				l.result = l.builder.Binary(current.span, l.owner(), current.binary, left, right)
			}
			l.values = l.values[:current.mark]
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
			if current.mark < 0 || current.mark >= len(l.values) {
				return fmt.Errorf("programlower: missing Function result")
			}
			function := l.values[current.mark]
			if _, err := l.finishBody(); err != nil {
				return err
			}
			l.values = l.values[:current.mark]
			l.result = function
		case stepFinishCallCallee:
			l.values = append(l.values, l.result)
			l.push(
				step{kind: stepFinishCall, call: current.call, mark: current.mark},
				step{
					kind:  stepValues,
					exprs: current.call.Args,
					span:  l.span(current.call),
					mark:  len(l.values),
				},
			)
		case stepFinishCall:
			if current.mark < 0 || current.mark >= len(l.values) {
				return fmt.Errorf("programlower: missing Call callee")
			}
			callee := l.values[current.mark]
			l.result = l.builder.Call(l.span(current.call), l.owner(), callee, 0, l.result)
			l.values = l.values[:current.mark]
			if l.result == 0 {
				return fmt.Errorf("programlower: could not create Call")
			}
		case stepTableFields:
			if err := l.runTableFields(current); err != nil {
				return err
			}
		case stepFinishTableKey:
			l.tableKeys = append(l.tableKeys, l.result)
			l.tableKinds = append(l.tableKinds, tableFieldKind(current.key))
			l.push(
				step{
					kind:      stepFinishTableValue,
					allowOpen: current.allowOpen,
					expr:      current.expr,
				},
				step{kind: stepExpr, expr: current.expr},
			)
		case stepFinishTableValue:
			var fixed []program.Term
			var tail program.Term
			if current.allowOpen && openProducer(current.expr) {
				tail = l.result
			} else {
				fixed = []program.Term{l.result}
			}
			values := l.builder.Values(l.span(current.expr), l.owner(), fixed, tail)
			if values == 0 {
				return fmt.Errorf("programlower: could not create table field Values")
			}
			l.tableValues = append(l.tableValues, values)
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
	if l.currentBody().terminated {
		return fmt.Errorf("programlower: statement after terminal %T", stmt)
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
			step{kind: stepFinishLocal, local: stmt, mark: len(l.targets)},
			step{kind: stepValues, exprs: stmt.Exprs, span: l.span(stmt), mark: len(l.values)},
		)
	case *ast.AssignStmt:
		if len(stmt.Lhs) == 0 {
			return fmt.Errorf("programlower: assignment has no targets")
		}
		l.push(step{kind: stepTargets, assign: stmt, mark: len(l.targets)})
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
		return fmt.Errorf("programlower: unsupported global or qualified function definition")
	case *ast.ReturnStmt:
		l.push(
			step{kind: stepFinishReturn, return_: stmt},
			step{kind: stepValues, exprs: stmt.Exprs, span: l.span(stmt), mark: len(l.values)},
		)
	case *ast.DoBlockStmt:
		span := l.span(stmt)
		body := l.builder.Body(span)
		if body == 0 {
			return fmt.Errorf("programlower: could not create do-block body")
		}
		l.enterBody(body, l.currentBody().function, span)
		l.push(
			step{kind: stepFinishDo},
			step{kind: stepStmts, stmts: stmt.Stmts},
		)
	default:
		return l.unsupportedStmt(stmt)
	}
	return nil
}

func (l *lowerer) finishLocal(current step) error {
	stmt := current.local
	for i := range stmt.Names {
		id, ok := l.binding.LocalSymbolAt(stmt, i)
		if !ok || id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for local slot %d", i)
		}
		if _, exists := l.active[id]; exists {
			return fmt.Errorf("programlower: duplicate binder symbol for local slot %d", i)
		}
		cell := l.builder.Cell(l.nameSpan(stmt, i), l.owner())
		if cell == 0 {
			return fmt.Errorf("programlower: could not create local cell %d", i)
		}
		l.install(id, cell)
		l.targets = append(l.targets, cell)
	}
	term := l.builder.Bind(l.span(stmt), l.owner(), l.targets[current.mark:], l.result)
	l.targets = l.targets[:current.mark]
	if term == 0 {
		return fmt.Errorf("programlower: could not lower local declaration")
	}
	return l.appendRoot(term)
}

func (l *lowerer) runTargets(current step) error {
	if current.index == len(current.assign.Lhs) {
		l.push(
			step{kind: stepFinishAssign, assign: current.assign, mark: current.mark},
			step{
				kind:  stepValues,
				exprs: current.assign.Rhs,
				span:  l.span(current.assign),
				mark:  len(l.values),
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
		cell, ok := l.active[id]
		if !ok || cell == 0 {
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
		fixed := l.values[current.mark:]
		var tail program.Term
		if len(current.exprs) != 0 && openProducer(current.exprs[len(current.exprs)-1]) {
			tail = fixed[len(fixed)-1]
			fixed = fixed[:len(fixed)-1]
		}
		l.result = l.builder.Values(current.span, l.owner(), fixed, tail)
		l.values = l.values[:current.mark]
		if l.result == 0 {
			return fmt.Errorf("programlower: could not create Values")
		}
		return nil
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
	switch expr := expr.(type) {
	case *ast.NilExpr:
		l.result = l.builder.Nil(span, l.owner())
	case *ast.TrueExpr:
		l.result = l.builder.Bool(span, l.owner(), true)
	case *ast.FalseExpr:
		l.result = l.builder.Bool(span, l.owner(), false)
	case *ast.NumberExpr:
		if integer, ok := numparse.ParseIntegerLiteral(expr.Value); ok {
			l.result = l.builder.Integer(span, l.owner(), integer)
			break
		}
		value, ok := numparse.ParseFloatLiteral(expr.Value)
		if !ok {
			return fmt.Errorf("programlower: invalid numeric literal %q", expr.Value)
		}
		l.result = l.builder.Float(span, l.owner(), value)
	case *ast.StringExpr:
		l.result = l.builder.String(span, l.owner(), expr.Value)
	case *ast.IdentExpr:
		id, ok := l.binding.SymbolOf(expr)
		if !ok || id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for identifier occurrence")
		}
		cell, ok := l.active[id]
		if !ok || cell == 0 {
			return fmt.Errorf("programlower: unsupported non-local identifier binding")
		}
		l.result = l.builder.Read(span, l.owner(), cell)
	case *ast.Comma3Expr:
		fn := l.currentBody().function
		if fn == nil {
			return fmt.Errorf("programlower: vararg expression outside function")
		}
		id, ok := l.binding.VarargSymbol(fn)
		if !ok {
			return fmt.Errorf("programlower: vararg expression in non-vararg function")
		}
		cell, ok := l.active[id]
		if !ok || cell == 0 {
			return fmt.Errorf("programlower: missing vararg Cell")
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
		l.push(step{
			kind:      stepTableFields,
			table:     expr,
			mark:      len(l.tableKeys),
			valueMark: len(l.tableValues),
			kindMark:  len(l.tableKinds),
		})
		return nil
	default:
		return l.unsupportedExpr(expr)
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
			mark:    len(l.values),
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
			mark:     len(l.values),
			readLens: read,
		},
		step{kind: stepExpr, expr: attr.Object},
	)
}

func (l *lowerer) finishLensBase(current step) error {
	l.values = append(l.values, l.result)
	if !astNodePresent(current.attr.Key) {
		return fmt.Errorf("programlower: absent attribute key %T", current.attr.Key)
	}
	switch current.attr.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := current.attr.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf("programlower: dot attribute key is not a string literal")
		}
		key := l.builder.Name(l.span(name), l.owner(), name.Value)
		if key == 0 {
			return fmt.Errorf("programlower: could not create attribute Name")
		}
		return l.finishLens(current, key)
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
	if current.mark < 0 || current.mark >= len(l.values) {
		return fmt.Errorf("programlower: missing Lens base")
	}
	base := l.values[current.mark]
	var lens program.Term
	switch current.attr.KeySyntax {
	case ast.AttrKeyDot:
		lens = l.builder.LensExact(current.span, l.owner(), base, key, program.FieldName)
	case ast.AttrKeyIndex:
		kind := tableFieldKind(current.attr.Key)
		if kind == program.FieldExact {
			lens = l.builder.LensExact(current.span, l.owner(), base, key, kind)
		} else {
			lens = l.builder.LensKey(current.span, l.owner(), base, key)
		}
	default:
		return fmt.Errorf("programlower: unsupported attribute key syntax %d", current.attr.KeySyntax)
	}
	l.values = l.values[:current.mark]
	if lens == 0 {
		return fmt.Errorf("programlower: could not create Lens")
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

func (l *lowerer) startFunction(fn *ast.FunctionExpr) error {
	origin, ok := l.binding.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginLiteral {
		return fmt.Errorf("programlower: unsupported ambiguous function origin")
	}
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
	body := l.builder.Body(span)
	if body == 0 {
		return fmt.Errorf("programlower: could not create function Body")
	}
	l.enterBody(body, fn, span)
	l.push(step{
		kind:      stepFunctionParams,
		fn:        fn,
		slots:     slots,
		captures:  l.captures[fn],
		mark:      len(l.targets),
		valueMark: len(l.capturePairs),
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
	if _, exists := l.active[slot.Symbol]; exists {
		return fmt.Errorf("programlower: duplicate binder symbol for function formal %q", slot.Name)
	}
	cell := l.builder.Cell(l.positionSpan(slot.Position), l.owner())
	if cell == 0 {
		return fmt.Errorf("programlower: could not create function formal Cell")
	}
	l.install(slot.Symbol, cell)
	l.targets = append(l.targets, cell)
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
	outer, exists := l.active[capture.Captured]
	if !exists || outer == 0 {
		return fmt.Errorf(
			"programlower: missing outer Cell for capture %q",
			capture.CapturedName,
		)
	}
	inner := l.builder.Cell(l.span(current.fn), l.owner())
	if inner == 0 {
		return fmt.Errorf("programlower: could not create capture Cell")
	}
	l.capturePairs = append(l.capturePairs, program.Capture{Inner: inner, Outer: outer})
	l.install(capture.Captured, inner)
	current.index++
	l.push(current)
	return nil
}

func (l *lowerer) beginFunctionBody(current step) error {
	params := l.targets[current.mark:]
	formals := params
	var vararg program.Term
	for i, slot := range current.slots {
		if !slot.Vararg {
			continue
		}
		if vararg != 0 {
			return fmt.Errorf("programlower: function has multiple vararg Cells")
		}
		if i != len(current.slots)-1 {
			return fmt.Errorf("programlower: function vararg Cell is not final")
		}
		vararg = params[i]
		formals = params[:i]
	}
	if len(l.bodies) < 2 {
		return fmt.Errorf("programlower: Function has no lexical owner")
	}
	owner := l.bodies[len(l.bodies)-2].body
	function := l.builder.Function(
		l.span(current.fn),
		owner,
		l.owner(),
		formals,
		vararg,
		l.capturePairs[current.valueMark:],
	)
	l.targets = l.targets[:current.mark]
	l.capturePairs = l.capturePairs[:current.valueMark]
	if function == 0 {
		return fmt.Errorf("programlower: could not create Function")
	}

	functionMark := len(l.values)
	l.values = append(l.values, function)
	l.push(
		step{kind: stepFinishFunctionBody, mark: functionMark},
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
	mark := len(l.values)
	l.push(
		step{kind: stepFinishCallCallee, call: call, mark: mark},
		step{kind: stepExpr, expr: call.Func},
	)
	return nil
}

func (l *lowerer) runTableFields(current step) error {
	if current.index == len(current.table.Fields) {
		keys := l.tableKeys[current.mark:]
		values := l.tableValues[current.valueMark:]
		kinds := l.tableKinds[current.kindMark:]
		if len(keys) != len(values) || len(keys) != len(kinds) {
			return fmt.Errorf("programlower: incomplete table fields")
		}
		l.result = l.builder.Table(l.span(current.table), l.owner(), keys, values, kinds)
		l.tableKeys = l.tableKeys[:current.mark]
		l.tableValues = l.tableValues[:current.valueMark]
		l.tableKinds = l.tableKinds[:current.kindMark]
		if l.result == 0 {
			return fmt.Errorf("programlower: could not create table allocation")
		}
		return nil
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
		key := l.builder.List(
			program.Span{File: l.sourceName},
			l.owner(),
			int64(current.ordinal),
		)
		if key == 0 {
			return fmt.Errorf("programlower: could not create table list key %d", index)
		}
		l.tableKeys = append(l.tableKeys, key)
		l.tableKinds = append(l.tableKinds, program.FieldList)
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
		key := l.builder.Name(l.span(name), l.owner(), name.Value)
		if key == 0 {
			return fmt.Errorf("programlower: could not create table field Name %d", index)
		}
		l.tableKeys = append(l.tableKeys, key)
		l.tableKinds = append(l.tableKinds, program.FieldName)
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
				key:       field.Key,
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

func tableFieldKind(expr ast.Expr) program.FieldKind {
	switch expr.(type) {
	case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		return program.FieldExact
	default:
		return program.FieldKey
	}
}

func openProducer(expr ast.Expr) bool {
	if !astNodePresent(expr) {
		return false
	}
	switch expr := expr.(type) {
	case *ast.FuncCallExpr:
		return !expr.AdjustRet
	case *ast.Comma3Expr:
		return !expr.AdjustRet
	default:
		return false
	}
}

func (l *lowerer) push(steps ...step) {
	l.steps = append(l.steps, steps...)
}

func (l *lowerer) enterBody(body program.Term, fn *ast.FunctionExpr, span program.Span) {
	l.bodies = append(l.bodies, bodyFrame{
		body:     body,
		function: fn,
		span:     span,
		undoMark: len(l.activeUndo),
		rootMark: len(l.roots),
	})
}

func (l *lowerer) finishBody() (bool, error) {
	if len(l.bodies) == 0 {
		return false, fmt.Errorf("programlower: no lexical body to finalize")
	}
	frame := l.bodies[len(l.bodies)-1]
	if !frame.terminated {
		values := l.builder.Values(frame.span, frame.body, nil, 0)
		if values == 0 {
			return false, fmt.Errorf("programlower: could not create normal Values")
		}
		normal := l.builder.Normal(frame.span, frame.body, values)
		if normal == 0 {
			return false, fmt.Errorf("programlower: could not create Normal outcome")
		}
		l.roots = append(l.roots, normal)
	}
	if !l.builder.SetBody(frame.body, l.roots[frame.rootMark:]...) {
		return false, fmt.Errorf("programlower: could not finalize Body")
	}
	l.roots = l.roots[:frame.rootMark]
	l.restore(frame.undoMark)
	l.bodies = l.bodies[:len(l.bodies)-1]
	return frame.terminated, nil
}

func (l *lowerer) currentBody() *bodyFrame {
	return &l.bodies[len(l.bodies)-1]
}

func (l *lowerer) owner() program.Term {
	return l.currentBody().body
}

func (l *lowerer) appendRoot(term program.Term) error {
	if term == 0 {
		return fmt.Errorf("programlower: could not create Body root")
	}
	l.roots = append(l.roots, term)
	return nil
}

func (l *lowerer) install(id symbol.ID, term program.Term) {
	prior, existed := l.active[id]
	l.activeUndo = append(l.activeUndo, activeUndo{id: id, prior: prior, existed: existed})
	l.active[id] = term
}

func (l *lowerer) restore(mark int) {
	for i := len(l.activeUndo) - 1; i >= mark; i-- {
		undo := l.activeUndo[i]
		if undo.existed {
			l.active[undo.id] = undo.prior
		} else {
			delete(l.active, undo.id)
		}
	}
	l.activeUndo = l.activeUndo[:mark]
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
