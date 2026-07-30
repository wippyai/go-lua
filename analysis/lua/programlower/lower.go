// Package programlower lowers parser AST plus binder identity into Program.
package programlower

import (
	"fmt"

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
	l := lowerer{
		sourceName: sourceName,
		binding:    binding,
		builder:    program.NewBuilder(),
		cells:      make(map[symbol.ID]program.Term),
		aliases:    make(map[symbol.ID]program.Term),
		functions:  make(map[symbol.ID]program.Term),
		captures:   prepareCaptures(binding),
	}
	bodySpan := l.chunkSpan(stmts)
	l.body = l.builder.Body(bodySpan)
	if l.body == 0 {
		return nil, fmt.Errorf("programlower: could not create chunk body")
	}
	if !l.builder.SetEntry(l.body) {
		return nil, fmt.Errorf("programlower: could not set chunk Entry")
	}
	terminated, err := l.lowerStmts(stmts)
	if err != nil {
		return nil, err
	}
	if !terminated {
		values := l.builder.Values(bodySpan, nil, 0)
		l.appendRoot(l.builder.Normal(bodySpan, values))
	}
	if !l.builder.SetBody(l.body, l.roots...) {
		return nil, fmt.Errorf("programlower: could not finalize chunk body")
	}
	sealed, err := l.builder.Seal()
	if err != nil {
		return nil, fmt.Errorf("programlower: seal: %w", err)
	}
	return sealed, nil
}

type lowerer struct {
	sourceName string
	binding    *bind.Result
	builder    *program.Builder
	body       program.Term
	function   *ast.FunctionExpr
	cells      map[symbol.ID]program.Term
	aliases    map[symbol.ID]program.Term
	functions  map[symbol.ID]program.Term
	captures   map[*ast.FunctionExpr][]bind.Capture
	roots      []program.Term
}

// prepareCaptures computes the closure cells that must cross each lexical
// function boundary. FunctionOrigins is lexical preorder, so an already-seen
// edge also proves that the same symbol was propagated through every earlier
// ancestor boundary and propagation can stop.
func prepareCaptures(binding *bind.Result) map[*ast.FunctionExpr][]bind.Capture {
	origins := binding.FunctionOrigins()
	if len(origins) == 0 {
		return nil
	}

	parents := make(map[*ast.FunctionExpr]*ast.FunctionExpr, len(origins))
	direct := make(map[*ast.FunctionExpr][]bind.Capture, len(origins))
	captures := make(map[*ast.FunctionExpr][]bind.Capture, len(origins))
	seen := make(map[*ast.FunctionExpr]map[symbol.ID]struct{}, len(origins))
	add := func(fn *ast.FunctionExpr, capture bind.Capture) bool {
		if fn == nil || capture.Captured == 0 {
			return false
		}
		symbols := seen[fn]
		if symbols == nil {
			symbols = make(map[symbol.ID]struct{})
			seen[fn] = symbols
		}
		if _, exists := symbols[capture.Captured]; exists {
			return false
		}
		symbols[capture.Captured] = struct{}{}
		captures[fn] = append(captures[fn], capture)
		return true
	}

	for _, origin := range origins {
		if origin.Func == nil {
			continue
		}
		parents[origin.Func] = origin.Parent
		direct[origin.Func] = binding.DirectCaptures(origin.Func)
		for _, capture := range direct[origin.Func] {
			add(origin.Func, capture)
		}
	}
	for _, origin := range origins {
		for _, capture := range direct[origin.Func] {
			for parent := origin.Parent; parent != nil && parent != capture.DeclaringFunction; parent = parents[parent] {
				if !add(parent, capture) {
					break
				}
			}
		}
	}
	return captures
}

// appendRoot is the only path into Body ownership. Expressions and the
// relations that encode their evaluation order remain reachable through their
// typed parents instead of becoming a second, flattened execution sequence.
func (l *lowerer) appendRoot(term program.Term) program.Term {
	if term != 0 {
		l.roots = append(l.roots, term)
	}
	return term
}

func (l *lowerer) cell(id symbol.ID) (program.Term, bool) {
	if id == 0 {
		return 0, false
	}
	if alias, ok := l.aliases[id]; ok && alias != 0 {
		return alias, true
	}
	cell, ok := l.cells[id]
	return cell, ok && cell != 0
}

func (l *lowerer) lowerStmts(stmts []ast.Stmt) (bool, error) {
	terminated := false
	for _, stmt := range stmts {
		if stmt == nil {
			return false, fmt.Errorf("programlower: nil statement")
		}
		if terminated {
			return false, fmt.Errorf("programlower: statement after terminal %T", stmt)
		}
		var err error
		switch s := stmt.(type) {
		case *ast.LocalAssignStmt:
			err = l.lowerLocalAssign(s)
		case *ast.AssignStmt:
			err = l.lowerAssign(s)
		case *ast.FuncCallStmt:
			callExpr, ok := s.Expr.(*ast.FuncCallExpr)
			if !ok {
				err = fmt.Errorf("programlower: call statement does not contain a call")
			} else {
				var call program.Term
				call, err = l.lowerCallExpr(callExpr)
				if err == nil {
					l.appendRoot(call)
				}
			}
		case *ast.FuncDefStmt:
			err = fmt.Errorf("programlower: unsupported global or qualified function definition")
		case *ast.ReturnStmt:
			err = l.lowerReturn(s)
			terminated = err == nil
		case *ast.DoBlockStmt:
			terminated, err = l.lowerDoBlock(s)
		default:
			err = l.unsupportedStmt(stmt)
		}
		if err != nil {
			return false, err
		}
	}
	return terminated, nil
}

func (l *lowerer) lowerDoBlock(stmt *ast.DoBlockStmt) (bool, error) {
	bodySpan := l.span(stmt)
	body := l.builder.Body(bodySpan)
	if body == 0 {
		return false, fmt.Errorf("programlower: could not create do-block body")
	}

	child := *l
	child.body = body
	child.roots = nil
	terminated, err := child.lowerStmts(stmt.Stmts)
	if err != nil {
		return false, err
	}
	if !terminated {
		values := l.builder.Values(bodySpan, nil, 0)
		child.appendRoot(l.builder.Normal(bodySpan, values))
	}
	if !l.builder.SetBody(body, child.roots...) {
		return false, fmt.Errorf("programlower: could not finalize do-block body")
	}
	l.appendRoot(body)
	return terminated, nil
}

func (l *lowerer) lowerLocalAssign(stmt *ast.LocalAssignStmt) error {
	if len(stmt.Names) == 0 {
		return fmt.Errorf("programlower: local assignment has no declarations")
	}
	for i, declared := range stmt.Types {
		if declared != nil {
			return fmt.Errorf("programlower: unsupported declared type for local slot %d", i)
		}
	}
	for i, expr := range stmt.Exprs {
		if _, ambiguous := expr.(*ast.FunctionExpr); ambiguous {
			return fmt.Errorf("programlower: unsupported function initializer for local slot %d: recursive-local syntax was erased", i)
		}
	}
	values, err := l.lowerValues(l.span(stmt), stmt.Exprs)
	if err != nil {
		return err
	}
	targets := make([]program.Term, len(stmt.Names))
	for i := range stmt.Names {
		id, ok := l.binding.LocalSymbolAt(stmt, i)
		if !ok || id == 0 {
			return fmt.Errorf("programlower: binder has no symbol for local slot %d", i)
		}
		if _, exists := l.cells[id]; exists {
			return fmt.Errorf("programlower: duplicate binder symbol for local slot %d", i)
		}
		cell := l.builder.Cell(l.nameSpan(stmt, i), l.body)
		if cell == 0 {
			return fmt.Errorf("programlower: could not create local cell %d", i)
		}
		l.cells[id] = cell
		targets[i] = cell
	}
	if l.appendRoot(l.builder.Bind(l.span(stmt), targets, values)) == 0 {
		return fmt.Errorf("programlower: could not lower local declaration")
	}
	return nil
}

func (l *lowerer) lowerAssign(stmt *ast.AssignStmt) error {
	if len(stmt.Lhs) == 0 {
		return fmt.Errorf("programlower: assignment has no targets")
	}
	targets := make([]program.Term, len(stmt.Lhs))
	for i, expr := range stmt.Lhs {
		target, err := l.lowerTarget(expr)
		if err != nil {
			return fmt.Errorf("programlower: assignment target %d: %w", i, err)
		}
		targets[i] = target
	}
	values, err := l.lowerValues(l.span(stmt), stmt.Rhs)
	if err != nil {
		return err
	}
	if l.appendRoot(l.builder.Assign(l.span(stmt), targets, values)) == 0 {
		return fmt.Errorf("programlower: could not lower assignment")
	}
	return nil
}

func (l *lowerer) lowerReturn(stmt *ast.ReturnStmt) error {
	values, err := l.lowerValues(l.span(stmt), stmt.Exprs)
	if err != nil {
		return err
	}
	if l.appendRoot(l.builder.Return(l.span(stmt), values)) == 0 {
		return fmt.Errorf("programlower: could not lower return")
	}
	return nil
}

func (l *lowerer) lowerValues(span program.Span, exprs []ast.Expr) (program.Term, error) {
	fixed := make([]program.Term, 0, len(exprs))
	var tail program.Term
	for i, expr := range exprs {
		if expr == nil {
			return 0, fmt.Errorf("programlower: nil expression in value list")
		}
		term, err := l.lowerExpr(expr)
		if err != nil {
			return 0, err
		}
		if i == len(exprs)-1 && openProducer(expr) {
			tail = term
		} else {
			fixed = append(fixed, term)
		}
	}
	values := l.builder.Values(span, fixed, tail)
	if values == 0 {
		return 0, fmt.Errorf("programlower: could not create Values")
	}
	return values, nil
}

func openProducer(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		return !e.AdjustRet
	case *ast.Comma3Expr:
		return !e.AdjustRet
	default:
		return false
	}
}

func (l *lowerer) lowerExpr(expr ast.Expr) (program.Term, error) {
	span := l.span(expr)
	var term program.Term
	switch e := expr.(type) {
	case *ast.NilExpr:
		term = l.builder.Nil(span)
	case *ast.TrueExpr:
		term = l.builder.Bool(span, true)
	case *ast.FalseExpr:
		term = l.builder.Bool(span, false)
	case *ast.NumberExpr:
		if integer, ok := numparse.ParseIntegerLiteral(e.Value); ok {
			term = l.builder.Integer(span, integer)
			break
		}
		value, ok := numparse.ParseFloatLiteral(e.Value)
		if !ok {
			return 0, fmt.Errorf("programlower: invalid numeric literal %q", e.Value)
		}
		term = l.builder.Float(span, value)
	case *ast.StringExpr:
		term = l.builder.String(span, e.Value)
	case *ast.IdentExpr:
		id, ok := l.binding.SymbolOf(e)
		if !ok || id == 0 {
			return 0, fmt.Errorf("programlower: binder has no symbol for identifier occurrence")
		}
		cell, ok := l.cell(id)
		if !ok {
			return 0, fmt.Errorf("programlower: unsupported non-local identifier binding")
		}
		term = l.builder.Read(span, cell)
	case *ast.Comma3Expr:
		if l.function == nil {
			return 0, fmt.Errorf("programlower: vararg expression outside function")
		}
		id, ok := l.binding.VarargSymbol(l.function)
		if !ok {
			return 0, fmt.Errorf("programlower: vararg expression in non-vararg function")
		}
		cell, ok := l.cell(id)
		if !ok {
			return 0, fmt.Errorf("programlower: missing vararg Cell")
		}
		term = l.builder.Vararg(span, cell)
	case *ast.AttrGetExpr:
		base, err := l.lowerExpr(e.Object)
		if err != nil {
			return 0, err
		}
		lens, err := l.lowerLens(span, base, e.Key, e.KeySyntax)
		if err != nil {
			return 0, err
		}
		term = l.builder.Read(span, lens)
	case *ast.UnaryMinusOpExpr:
		return l.lowerUnary(span, program.UnaryNeg, e.Expr)
	case *ast.UnaryNotOpExpr:
		return l.lowerUnary(span, program.UnaryNot, e.Expr)
	case *ast.UnaryLenOpExpr:
		return l.lowerUnary(span, program.UnaryLen, e.Expr)
	case *ast.UnaryBNotOpExpr:
		return l.lowerUnary(span, program.UnaryBitNot, e.Expr)
	case *ast.ArithmeticOpExpr:
		op, ok := arithmeticOp(e.Operator)
		if !ok {
			return 0, fmt.Errorf("programlower: unsupported arithmetic operator %q", e.Operator)
		}
		return l.lowerBinary(span, op, e.Lhs, e.Rhs)
	case *ast.StringConcatOpExpr:
		return l.lowerBinary(span, program.BinaryConcat, e.Lhs, e.Rhs)
	case *ast.RelationalOpExpr:
		op, ok := relationalOp(e.Operator)
		if !ok {
			return 0, fmt.Errorf("programlower: unsupported relational operator %q", e.Operator)
		}
		return l.lowerBinary(span, op, e.Lhs, e.Rhs)
	case *ast.LogicalOpExpr:
		op, ok := selectOp(e.Operator)
		if !ok {
			return 0, fmt.Errorf("programlower: unsupported logical operator %q", e.Operator)
		}
		left, err := l.lowerExpr(e.Lhs)
		if err != nil {
			return 0, err
		}
		right, err := l.lowerExpr(e.Rhs)
		if err != nil {
			return 0, err
		}
		term = l.builder.Select(span, op, left, right)
	case *ast.FunctionExpr:
		return l.lowerFunction(e)
	case *ast.FuncCallExpr:
		return l.lowerCallExpr(e)
	case *ast.TableExpr:
		return l.lowerTable(e)
	default:
		return 0, l.unsupportedExpr(expr)
	}
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower expression %T", expr)
	}
	return term, nil
}

func (l *lowerer) lowerFunction(fn *ast.FunctionExpr) (program.Term, error) {
	if fn == nil {
		return 0, fmt.Errorf("programlower: nil function expression")
	}
	origin, ok := l.binding.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginLiteral {
		return 0, fmt.Errorf("programlower: unsupported ambiguous function origin")
	}
	if len(fn.TypeParams) != 0 || len(fn.ReturnTypes) != 0 {
		return 0, fmt.Errorf("programlower: unsupported typed function")
	}
	slots := l.binding.ParamSlots(fn)
	for _, slot := range slots {
		if slot.Type != nil {
			return 0, fmt.Errorf("programlower: unsupported typed function parameter %q", slot.Name)
		}
	}
	if fn.ParList != nil && fn.ParList.VarargType != nil {
		return 0, fmt.Errorf("programlower: unsupported typed function vararg")
	}

	span := l.span(fn)
	body := l.builder.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("programlower: could not create function Body")
	}
	formals := make([]program.Term, 0, len(slots))
	var vararg program.Term
	for _, slot := range slots {
		cell := l.builder.Cell(l.positionSpan(slot.Position), body)
		if cell == 0 {
			return 0, fmt.Errorf("programlower: could not create function formal Cell")
		}
		l.cells[slot.Symbol] = cell
		if slot.Vararg {
			if vararg != 0 {
				return 0, fmt.Errorf("programlower: function has multiple vararg Cells")
			}
			vararg = cell
		} else {
			formals = append(formals, cell)
		}
	}
	function := l.builder.Function(span, l.body, body, 0, formals, vararg)
	if function == 0 {
		return 0, fmt.Errorf("programlower: could not create Function")
	}
	functionSymbol, ok := l.binding.FunctionSymbol(fn)
	if !ok {
		return 0, fmt.Errorf("programlower: binder has no function identity")
	}
	if l.functions[functionSymbol] != 0 {
		return 0, fmt.Errorf("programlower: duplicate function identity")
	}
	l.functions[functionSymbol] = function

	captures := l.captures[fn]
	captureTerms := make([]program.Term, 0, len(captures))
	type savedAlias struct {
		id      symbol.ID
		alias   program.Term
		existed bool
	}
	saved := make([]savedAlias, 0, len(captures))
	for _, capture := range captures {
		outer, exists := l.cell(capture.Captured)
		if !exists || outer == 0 {
			return 0, fmt.Errorf("programlower: missing outer Cell for capture %q", capture.CapturedName)
		}
		inner := l.builder.Cell(span, body)
		if inner == 0 {
			return 0, fmt.Errorf("programlower: could not create capture Cell")
		}
		prior, existed := l.aliases[capture.Captured]
		saved = append(saved, savedAlias{id: capture.Captured, alias: prior, existed: existed})
		l.aliases[capture.Captured] = inner
		relation := l.builder.Capture(span, function, inner, outer)
		if relation == 0 {
			return 0, fmt.Errorf("programlower: could not create Capture")
		}
		captureTerms = append(captureTerms, relation)
	}

	child := *l
	child.body = body
	child.function = fn
	child.roots = nil
	terminated, err := child.lowerStmts(fn.Stmts)
	for i := len(saved) - 1; i >= 0; i-- {
		prior := saved[i]
		if prior.existed {
			l.aliases[prior.id] = prior.alias
		} else {
			delete(l.aliases, prior.id)
		}
	}
	if err != nil {
		return 0, err
	}
	if !terminated {
		values := l.builder.Values(span, nil, 0)
		child.appendRoot(l.builder.Normal(span, values))
	}
	if !l.builder.SetBody(body, child.roots...) {
		return 0, fmt.Errorf("programlower: could not finalize function Body")
	}
	if !l.builder.SetFunctionCaptures(function, captureTerms) {
		return 0, fmt.Errorf("programlower: could not finalize Function captures")
	}
	return function, nil
}

func (l *lowerer) lowerCallExpr(call *ast.FuncCallExpr) (program.Term, error) {
	if call == nil {
		return 0, fmt.Errorf("programlower: nil call expression")
	}
	if len(call.TypeArgs) != 0 {
		return 0, fmt.Errorf("programlower: unsupported typed call")
	}
	span := l.span(call)
	var callee, receiver, direct program.Term
	if call.Method != "" || call.Receiver != nil {
		if call.Method == "" || call.Receiver == nil || call.Func != nil {
			return 0, fmt.Errorf("programlower: invalid method call shape")
		}
		return 0, fmt.Errorf("programlower: unsupported method call: AST has no MethodPosition evidence")
	} else {
		if call.Func == nil {
			return 0, fmt.Errorf("programlower: plain call has no callee")
		}
		var err error
		callee, err = l.lowerExpr(call.Func)
		if err != nil {
			return 0, err
		}
		switch target := call.Func.(type) {
		case *ast.FunctionExpr:
			direct = callee
		case *ast.IdentExpr:
			if binding, ok := l.binding.SymbolOf(target); ok {
				if identity, stable := l.binding.StableLocalFunctionIdentity(binding); stable {
					direct = l.functions[identity]
				}
			}
		}
	}
	actuals, err := l.lowerValues(span, call.Args)
	if err != nil {
		return 0, err
	}
	term := l.builder.Call(span, callee, receiver, actuals, direct)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create Call")
	}
	return term, nil
}

func (l *lowerer) lowerTarget(expr ast.Expr) (program.Term, error) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		id, ok := l.binding.SymbolOf(e)
		if !ok || id == 0 {
			return 0, fmt.Errorf("binder has no symbol for identifier target")
		}
		cell, ok := l.cell(id)
		if !ok {
			return 0, fmt.Errorf("unsupported non-local identifier target")
		}
		return cell, nil
	case *ast.AttrGetExpr:
		base, err := l.lowerExpr(e.Object)
		if err != nil {
			return 0, err
		}
		return l.lowerLens(l.span(e), base, e.Key, e.KeySyntax)
	default:
		return 0, l.unsupportedExpr(expr)
	}
}

func (l *lowerer) lowerTable(table *ast.TableExpr) (program.Term, error) {
	tableTerm := l.builder.Table(l.span(table))
	if tableTerm == 0 {
		return 0, fmt.Errorf("programlower: could not create table allocation")
	}
	keys := make([]program.Term, 0, len(table.Fields))
	values := make([]program.Term, 0, len(table.Fields))
	kinds := make([]program.FieldKind, 0, len(table.Fields))
	arrayIndex := int64(1)
	for i, field := range table.Fields {
		if field == nil || field.Value == nil {
			return 0, fmt.Errorf("programlower: invalid table field %d", i)
		}
		var key program.Term
		var kind program.FieldKind
		if field.Key == nil {
			key = l.builder.Integer(program.Span{File: l.sourceName}, arrayIndex)
			arrayIndex++
			kind = program.FieldList
		} else {
			switch field.KeySyntax {
			case ast.AttrKeyDot:
				if _, ok := field.Key.(*ast.StringExpr); !ok {
					return 0, fmt.Errorf("programlower: table field %d dot key is not a string literal", i)
				}
				kind = program.FieldExact
			case ast.AttrKeyIndex:
				switch field.Key.(type) {
				case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
					kind = program.FieldExact
				default:
					kind = program.FieldKey
				}
			case ast.AttrKeyUnknown:
				return 0, fmt.Errorf("programlower: unsupported table field %d with unknown key syntax", i)
			default:
				return 0, fmt.Errorf("programlower: unsupported table field %d key syntax %d", i, field.KeySyntax)
			}
			var err error
			key, err = l.lowerExpr(field.Key)
			if err != nil {
				return 0, fmt.Errorf("programlower: table field %d key: %w", i, err)
			}
		}
		fieldValues, err := l.lowerTableFieldValues(
			field.Value,
			kind == program.FieldList && i == len(table.Fields)-1,
		)
		if err != nil {
			return 0, fmt.Errorf("programlower: table field %d value: %w", i, err)
		}
		if key == 0 || fieldValues == 0 {
			return 0, fmt.Errorf("programlower: could not lower table field %d", i)
		}
		keys = append(keys, key)
		values = append(values, fieldValues)
		kinds = append(kinds, kind)
	}
	if !l.builder.SetTable(tableTerm, keys, values, kinds) {
		return 0, fmt.Errorf("programlower: could not finalize table initialization")
	}
	return tableTerm, nil
}

func (l *lowerer) lowerTableFieldValues(expr ast.Expr, allowOpen bool) (program.Term, error) {
	value, err := l.lowerExpr(expr)
	if err != nil {
		return 0, err
	}
	var fixed []program.Term
	var tail program.Term
	if allowOpen && openProducer(expr) {
		tail = value
	} else {
		fixed = []program.Term{value}
	}
	values := l.builder.Values(l.span(expr), fixed, tail)
	if values == 0 {
		return 0, fmt.Errorf("programlower: could not create table field Values")
	}
	return values, nil
}

func (l *lowerer) lowerLens(span program.Span, base program.Term, keyExpr ast.Expr, syntax ast.AttrKeySyntax) (program.Term, error) {
	if keyExpr == nil {
		return 0, fmt.Errorf("programlower: attribute has no key")
	}
	exact := false
	switch syntax {
	case ast.AttrKeyDot:
		if _, ok := keyExpr.(*ast.StringExpr); !ok {
			return 0, fmt.Errorf("programlower: dot attribute key is not a string literal")
		}
		exact = true
	case ast.AttrKeyIndex:
		switch keyExpr.(type) {
		case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
			exact = true
		}
	case ast.AttrKeyUnknown:
		return 0, fmt.Errorf("programlower: unsupported attribute with unknown key syntax")
	default:
		return 0, fmt.Errorf("programlower: unsupported attribute key syntax %d", syntax)
	}
	key, err := l.lowerExpr(keyExpr)
	if err != nil {
		return 0, err
	}
	var lens program.Term
	if exact {
		lens = l.builder.LensExact(span, base, key)
	} else {
		lens = l.builder.LensKey(span, base, key)
	}
	if lens == 0 {
		return 0, fmt.Errorf("programlower: could not create Lens")
	}
	return lens, nil
}

func (l *lowerer) lowerUnary(span program.Span, op program.UnaryOp, operandExpr ast.Expr) (program.Term, error) {
	operand, err := l.lowerExpr(operandExpr)
	if err != nil {
		return 0, err
	}
	term := l.builder.Unary(span, op, operand)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create unary operation")
	}
	return term, nil
}

func (l *lowerer) lowerBinary(span program.Span, op program.BinaryOp, leftExpr, rightExpr ast.Expr) (program.Term, error) {
	left, err := l.lowerExpr(leftExpr)
	if err != nil {
		return 0, err
	}
	right, err := l.lowerExpr(rightExpr)
	if err != nil {
		return 0, err
	}
	term := l.builder.Binary(span, op, left, right)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create binary operation")
	}
	return term, nil
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

func (l *lowerer) unsupportedStmt(stmt ast.Stmt) error {
	return fmt.Errorf("programlower: unsupported statement %T at %d:%d", stmt, stmt.Line(), stmt.Column())
}

func (l *lowerer) unsupportedExpr(expr ast.Expr) error {
	return fmt.Errorf("programlower: unsupported expression %T at %d:%d", expr, expr.Line(), expr.Column())
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
	if first == nil || last == nil {
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
