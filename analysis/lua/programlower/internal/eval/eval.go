// Package eval owns expression scratch and exact Lua value-list adjustment.
package eval

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
	"github.com/wippyai/go-lua/program"
)

// Values owns every temporary expression term and value-pack range for one walk.
type Values struct {
	builder *program.Builder
	terms   []program.Term
}

// New creates the value authority for one unfinished Program.
func New(builder *program.Builder) Values {
	return Values{builder: builder}
}

// Mark identifies the start of one ordered temporary range.
func (v *Values) Mark() int {
	return len(v.terms)
}

// Append retains one expression-list element in source order.
func (v *Values) Append(term program.Term) {
	v.terms = append(v.terms, term)
}

// Hold retains one scalar across nested evaluation and returns its LIFO mark.
func (v *Values) Hold(term program.Term) int {
	mark := len(v.terms)
	v.terms = append(v.terms, term)
	return mark
}

// Take completes one scalar hold. Nested ranges must already be complete.
func (v *Values) Take(mark int) (program.Term, error) {
	if mark < 0 || mark != len(v.terms)-1 {
		return 0, fmt.Errorf("programlower: incomplete scalar evaluation")
	}
	term := v.terms[mark]
	v.terms = v.terms[:mark]
	return term, nil
}

// Pack completes one source expression list with final-open adjustment.
func (v *Values) Pack(
	span program.Span,
	owner program.Term,
	mark int,
	exprs []ast.Expr,
) (program.Term, error) {
	if mark < 0 || mark > len(v.terms) ||
		len(v.terms)-mark != len(exprs) {
		return 0, fmt.Errorf("programlower: incomplete value list")
	}
	fixed := v.terms[mark:]
	var tail program.Term
	if len(exprs) != 0 && openProducer(exprs[len(exprs)-1]) {
		tail = fixed[len(fixed)-1]
		fixed = fixed[:len(fixed)-1]
	}
	term := v.builder.Values(span, owner, fixed, tail)
	v.terms = v.terms[:mark]
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create Values")
	}
	return term, nil
}

// Field completes one table field pack with its context-specific open law.
func (v *Values) Field(
	span program.Span,
	owner program.Term,
	expr ast.Expr,
	value program.Term,
	allowOpen bool,
) (program.Term, error) {
	var fixed []program.Term
	var tail program.Term
	if allowOpen && openProducer(expr) {
		tail = value
	} else {
		fixed = []program.Term{value}
	}
	term := v.builder.Values(span, owner, fixed, tail)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create table field Values")
	}
	return term, nil
}

// Empty records the canonical empty pack for one normal Body outcome.
func (v *Values) Empty(span program.Span, owner program.Term) (program.Term, error) {
	term := v.builder.Values(span, owner, nil, 0)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create normal Values")
	}
	return term, nil
}

// Number parses and records one authored numeric constant.
func (v *Values) Number(
	span program.Span,
	owner program.Term,
	literal string,
) (program.Term, error) {
	if integer, ok := numparse.ParseIntegerLiteral(literal); ok {
		term := v.builder.Integer(span, owner, integer)
		if term == 0 {
			return 0, fmt.Errorf("programlower: could not create integer literal")
		}
		return term, nil
	}
	value, ok := numparse.ParseFloatLiteral(literal)
	if !ok {
		return 0, fmt.Errorf("programlower: invalid numeric literal %q", literal)
	}
	term := v.builder.Float(span, owner, value)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create float literal")
	}
	return term, nil
}

func openProducer(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.FuncCallExpr:
		return expr != nil && !expr.AdjustRet
	case *ast.Comma3Expr:
		return expr != nil && !expr.AdjustRet
	default:
		return false
	}
}

// Clean reports whether every expression scratch range completed.
func (v *Values) Clean() bool {
	return len(v.terms) == 0
}
