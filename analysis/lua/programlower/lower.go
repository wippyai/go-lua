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
	}
	bodySpan := l.chunkSpan(stmts)
	l.body = l.builder.Body(bodySpan)
	if l.body == 0 {
		return nil, fmt.Errorf("programlower: could not create chunk body")
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
	cells      map[symbol.ID]program.Term
	roots      []program.Term
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
	fixed := make([]program.Term, len(exprs))
	for i, expr := range exprs {
		if expr == nil {
			return 0, fmt.Errorf("programlower: nil expression in value list")
		}
		if ast.CanProduceMultipleValues(expr) {
			return 0, fmt.Errorf("programlower: unsupported open value producer %T", expr)
		}
		term, err := l.lowerExpr(expr)
		if err != nil {
			return 0, err
		}
		fixed[i] = term
	}
	values := l.builder.Values(span, fixed, 0)
	if values == 0 {
		return 0, fmt.Errorf("programlower: could not create Values")
	}
	return values, nil
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
		cell, ok := l.cells[id]
		if !ok {
			return 0, fmt.Errorf("programlower: unsupported non-local identifier binding")
		}
		term = l.builder.Read(span, cell)
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

func (l *lowerer) lowerTarget(expr ast.Expr) (program.Term, error) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		id, ok := l.binding.SymbolOf(e)
		if !ok || id == 0 {
			return 0, fmt.Errorf("binder has no symbol for identifier target")
		}
		cell, ok := l.cells[id]
		if !ok {
			return 0, fmt.Errorf("unsupported non-local identifier target")
		}
		return cell, nil
	case *ast.AttrGetExpr:
		base, err := l.lowerExpr(e.Object)
		if err != nil {
			return 0, err
		}
		return l.lowerLens(l.span(e), base, e.Key)
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
		value, err := l.lowerExpr(field.Value)
		if err != nil {
			return 0, fmt.Errorf("programlower: table field %d value: %w", i, err)
		}
		fieldValues := l.builder.Values(l.span(field.Value), []program.Term{value}, 0)
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

func (l *lowerer) lowerLens(span program.Span, base program.Term, keyExpr ast.Expr, keys ...program.Term) (program.Term, error) {
	var key program.Term
	if len(keys) != 0 {
		key = keys[0]
	} else {
		var err error
		key, err = l.lowerExpr(keyExpr)
		if err != nil {
			return 0, err
		}
	}
	var lens program.Term
	switch keyExpr.(type) {
	case nil, *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		lens = l.builder.LensExact(span, base, key)
	default:
		lens = l.builder.LensKey(span, base, key)
	}
	if lens == 0 {
		return 0, fmt.Errorf("programlower: could not create Lens")
	}
	return lens, nil
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
