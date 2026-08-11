// Package eval owns expression-local lowering and Lua Values adjustment.
//
// It owns only expressions whose Program relations are value occurrences:
// literals, unary/binary/select, and source value claims. Vararg remains
// lexical because its identity is function-boundary evidence. Lookup, storage
// lenses, calls, tables, and function construction stay in their own verticals.
package eval

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	lowercollector "github.com/wippyai/go-lua/program/lower/internal/collector"
	"github.com/wippyai/go-lua/program/lower/internal/inbox"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
	"github.com/wippyai/go-lua/program/source"
)

// Values owns all unfinished scalar and list evaluation for one Program walk.
// A completed scalar records whether it is an unadjusted Lua multi-value
// producer. That property is produced by the expression owner, never inferred
// later by inspecting an AST node.
type Values struct {
	phase       *phase.Stack
	collector   *lowercollector.Collector
	expressions *inbox.Expressions
	statics     *inbox.Statics

	terms []keyspace.Term
	open  []bool
	steps []step
}

type stepKind uint8

const (
	stepExpression stepKind = iota + 1
	stepValues
	stepUnary
	stepBinaryLeft
	stepBinaryRight
	stepClaimOperand
	stepClaimTarget
)

// step is private eval continuation state. It contains no callback and no
// semantic payload belonging to another vertical.
type step struct {
	kind  stepKind
	owner keyspace.Term
	span  source.Span
	expr  ast.Expr

	exprs    []ast.Expr
	index    int
	awaiting bool
	mark     int

	unary   flowkind.UnaryOp
	binary  flowkind.BinaryOp
	select_ flowkind.SelectOp
	left    keyspace.Term

	claim     flowkind.ValueClaimKind
	target    ast.TypeExpr
	claimTerm keyspace.Term
}

// New creates the sole eval authority for one unfinished Program. The two
// injected inboxes are the only irreducible crossings: nested expressions
// return to source grammar dispatch, while cast targets return to Static.
// Vararg identity remains a direct lexical operation; eval does not reproduce
// function-boundary lookup.
func New(
	stack *phase.Stack,
	collector *lowercollector.Collector,
	expressions *inbox.Expressions,
	statics *inbox.Statics,
) Values {
	return Values{
		phase:       stack,
		collector:   collector,
		expressions: expressions,
		statics:     statics,
	}
}

// ScheduleExpression schedules one expression owned by eval. Source owns the
// closed grammar dispatch and calls this only for the exact eval subset; a
// foreign node is an invariant violation, never a fall-through route.
func (v *Values) ScheduleExpression(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if v == nil || v.phase == nil || v.collector == nil || v.expressions == nil || v.statics == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid eval authority")
	}
	if !evalExpression(expr) {
		return fmt.Errorf("programlower: eval received foreign or absent expression %T", expr)
	}
	v.schedule(step{kind: stepExpression, owner: owner, span: span, expr: expr})
	return nil
}

// ScheduleValues schedules exact left-to-right Lua expression-list lowering.
// The final tail decision consumes completed-result metadata, not source AST
// shape: only a producer that publishes open=true may become the Values tail.
func (v *Values) ScheduleValues(exprs []ast.Expr, owner keyspace.Term, span source.Span) error {
	if v == nil || v.phase == nil || v.collector == nil || v.expressions == nil || v.statics == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid eval Values authority")
	}
	v.schedule(step{kind: stepValues, owner: owner, span: span, exprs: exprs, mark: len(v.terms)})
	return nil
}

// Field completes one table field Values pack. The table vertical supplies the
// completed value's openness from phase; eval never recovers it from AST
// spelling. Only a final list field may preserve that open tail.
func (v *Values) Field(
	span source.Span,
	owner keyspace.Term,
	value keyspace.Term,
	open bool,
	allowOpen bool,
) (keyspace.Term, error) {
	if v == nil || v.collector == nil || owner == 0 || value == 0 {
		return 0, fmt.Errorf("programlower: invalid table field Values")
	}
	var fixed []keyspace.Term
	var tail keyspace.Term
	if allowOpen && open {
		tail = value
	} else {
		fixed = []keyspace.Term{value}
	}
	term := v.collector.Flow().Values().Values(span, owner, fixed, tail)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create table field Values")
	}
	return term, nil
}

// Singleton packs one already-evaluated scalar for a context that requires a
// fixed Lua Values result. It never schedules or evaluates source syntax.
func (v *Values) Singleton(span source.Span, owner, value keyspace.Term) (keyspace.Term, error) {
	terms := [1]keyspace.Term{value}
	return v.Fixed(span, owner, terms[:])
}

// Fixed packs already-completed scalar terms with no open tail. Control and
// declaration owners use this exact authority when their source grammar
// requires scalar adjustment; eval retains no cross-owner scratch marks.
func (v *Values) Fixed(span source.Span, owner keyspace.Term, terms []keyspace.Term) (keyspace.Term, error) {
	if v == nil || v.collector == nil || owner == 0 {
		return 0, fmt.Errorf("programlower: invalid fixed Values authority")
	}
	term := v.collector.Flow().Values().Values(span, owner, terms, 0)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create fixed Values")
	}
	return term, nil
}

// Run executes exactly one scheduled private eval continuation. Cross-vertical
// child work enters its typed inbox; phase records only which owner runs next.
func (v *Values) Run() error {
	if v == nil || v.phase == nil || v.collector == nil || v.expressions == nil || v.statics == nil || len(v.steps) == 0 {
		return fmt.Errorf("programlower: missing eval continuation")
	}
	last := len(v.steps) - 1
	current := v.steps[last]
	v.steps = v.steps[:last]

	switch current.kind {
	case stepExpression:
		return v.runExpression(current)
	case stepValues:
		return v.runValues(current)
	case stepUnary:
		term, _ := v.phase.Result()
		term = v.collector.Flow().Operators().Unary(
			current.span, current.owner, current.unary, term,
		)
		if term == 0 {
			return fmt.Errorf("programlower: could not create unary operation")
		}
		v.phase.SetResult(term, false)
		return nil
	case stepBinaryLeft:
		left, _ := v.phase.Result()
		if left == 0 {
			return fmt.Errorf("programlower: missing binary left operand")
		}
		current.left = left
		current.kind = stepBinaryRight
		v.schedule(current)
		return v.enqueueExpression(current.expr, current.owner, current.span)
	case stepBinaryRight:
		right, _ := v.phase.Result()
		if right == 0 {
			return fmt.Errorf("programlower: missing binary right operand")
		}
		var term keyspace.Term
		if current.select_ != 0 {
			term = v.collector.Flow().Operators().Select(
				current.span, current.owner, current.select_, current.left, right,
			)
		} else {
			term = v.collector.Flow().Operators().Binary(
				current.span, current.owner, current.binary, current.left, right,
			)
		}
		if term == 0 {
			return fmt.Errorf("programlower: could not create binary operation")
		}
		v.phase.SetResult(term, false)
		return nil
	case stepClaimOperand:
		return v.finishClaimOperand(current)
	case stepClaimTarget:
		target, _ := v.phase.Result()
		if target == 0 || !v.collector.Flow().Operands().FillValueClaimTarget(current.claimTerm, target) {
			return fmt.Errorf("programlower: could not finalize ValueClaim target")
		}
		v.phase.SetResult(current.claimTerm, false)
		return nil
	default:
		return fmt.Errorf("programlower: invalid eval continuation %d", current.kind)
	}
}

func (v *Values) runExpression(current step) error {
	switch expr := current.expr.(type) {
	case *ast.NilExpr:
		v.phase.SetResult(v.collector.Source().Literals().Nil(current.span, current.owner), false)
	case *ast.TrueExpr:
		v.phase.SetResult(v.collector.Source().Literals().Bool(current.span, current.owner, true), false)
	case *ast.FalseExpr:
		v.phase.SetResult(v.collector.Source().Literals().Bool(current.span, current.owner, false), false)
	case *ast.NumberExpr:
		term, err := v.number(current.span, current.owner, expr.Value)
		if err != nil {
			return err
		}
		v.phase.SetResult(term, false)
	case *ast.StringExpr:
		term := v.collector.Source().Literals().String(current.span, current.owner, expr.Value)
		if term == 0 {
			return fmt.Errorf("programlower: could not create string literal")
		}
		v.phase.SetResult(term, false)
	case *ast.UnaryMinusOpExpr:
		return v.startUnary(current, flowkind.UnaryNeg, expr.Expr)
	case *ast.UnaryNotOpExpr:
		return v.startUnary(current, flowkind.UnaryNot, expr.Expr)
	case *ast.UnaryLenOpExpr:
		return v.startUnary(current, flowkind.UnaryLen, expr.Expr)
	case *ast.UnaryBNotOpExpr:
		return v.startUnary(current, flowkind.UnaryBitNot, expr.Expr)
	case *ast.ArithmeticOpExpr:
		op, ok := arithmetic(expr.Operator)
		if !ok {
			return fmt.Errorf("programlower: unsupported arithmetic operator %q", expr.Operator)
		}
		return v.startBinary(current, op, 0, expr.Lhs, expr.Rhs)
	case *ast.StringConcatOpExpr:
		return v.startBinary(current, flowkind.BinaryConcat, 0, expr.Lhs, expr.Rhs)
	case *ast.RelationalOpExpr:
		op, ok := relational(expr.Operator)
		if !ok {
			return fmt.Errorf("programlower: unsupported relational operator %q", expr.Operator)
		}
		return v.startBinary(current, op, 0, expr.Lhs, expr.Rhs)
	case *ast.LogicalOpExpr:
		op, ok := selection(expr.Operator)
		if !ok {
			return fmt.Errorf("programlower: unsupported logical operator %q", expr.Operator)
		}
		return v.startBinary(current, 0, op, expr.Lhs, expr.Rhs)
	case *ast.NonNilAssertExpr:
		return v.startClaim(current, flowkind.ValueClaimNonNil, expr.Expr, nil)
	case *ast.CastExpr:
		if !typeExpression(expr.Type) || !expression(expr.Expr) {
			return fmt.Errorf("programlower: invalid cast expression")
		}
		var kind flowkind.ValueClaimKind
		switch expr.Syntax {
		case ast.CastSyntaxAs:
			kind = flowkind.ValueClaimTypeAs
		case ast.CastSyntaxColonColon:
			kind = flowkind.ValueClaimTypeColonColon
		default:
			return fmt.Errorf("programlower: unsupported cast syntax %d", expr.Syntax)
		}
		return v.startClaim(current, kind, expr.Expr, expr.Type)
	default:
		return fmt.Errorf("programlower: eval routed foreign expression %T", expr)
	}
	term, _ := v.phase.Result()
	if term == 0 {
		return fmt.Errorf("programlower: could not create expression %T", current.expr)
	}
	return nil
}

func (v *Values) runValues(current step) error {
	if current.awaiting {
		term, open := v.phase.Result()
		if term == 0 {
			return fmt.Errorf("programlower: missing Values element")
		}
		v.terms = append(v.terms, term)
		v.open = append(v.open, open)
		current.awaiting = false
	}
	if current.index == len(current.exprs) {
		if current.mark < 0 || current.mark > len(v.terms) || len(v.open) != len(v.terms) {
			return fmt.Errorf("programlower: invalid Values mark")
		}
		fixed := v.terms[current.mark:]
		open := v.open[current.mark:]
		if len(fixed) != len(current.exprs) {
			return fmt.Errorf("programlower: incomplete Values list")
		}
		fixedCount := len(fixed)
		var tail keyspace.Term
		if fixedCount != 0 && open[fixedCount-1] {
			tail = fixed[fixedCount-1]
			fixedCount--
		}
		term := v.collector.Flow().Values().Values(
			current.span, current.owner, fixed[:fixedCount], tail,
		)
		if term == 0 {
			return fmt.Errorf("programlower: could not create Values")
		}
		v.terms = v.terms[:current.mark]
		v.open = v.open[:current.mark]
		v.phase.SetResult(term, false)
		return nil
	}
	if current.index < 0 || current.index > len(current.exprs) || !expression(current.exprs[current.index]) {
		return fmt.Errorf("programlower: invalid Values expression %d", current.index)
	}
	expr := current.exprs[current.index]
	current.index++
	current.awaiting = true
	v.schedule(current)
	return v.enqueueExpression(expr, current.owner, current.span)
}

func (v *Values) startUnary(current step, op flowkind.UnaryOp, operand ast.Expr) error {
	if !expression(operand) {
		return fmt.Errorf("programlower: absent unary operand")
	}
	v.schedule(step{kind: stepUnary, owner: current.owner, span: current.span, unary: op})
	return v.enqueueExpression(operand, current.owner, current.span)
}

func (v *Values) startBinary(current step, binary flowkind.BinaryOp, select_ flowkind.SelectOp, left, right ast.Expr) error {
	if !expression(left) || !expression(right) {
		return fmt.Errorf("programlower: absent binary operand")
	}
	v.schedule(step{kind: stepBinaryLeft, owner: current.owner, span: current.span, binary: binary, select_: select_, expr: right})
	return v.enqueueExpression(left, current.owner, current.span)
}

func (v *Values) startClaim(current step, kind flowkind.ValueClaimKind, operand ast.Expr, target ast.TypeExpr) error {
	if !expression(operand) || (kind == flowkind.ValueClaimNonNil) != (target == nil) ||
		(kind != flowkind.ValueClaimNonNil && !typeExpression(target)) {
		return fmt.Errorf("programlower: invalid ValueClaim")
	}
	v.schedule(step{kind: stepClaimOperand, owner: current.owner, span: current.span, claim: kind, target: target})
	return v.enqueueExpression(operand, current.owner, current.span)
}

func (v *Values) finishClaimOperand(current step) error {
	operand, _ := v.phase.Result()
	if operand == 0 {
		return fmt.Errorf("programlower: missing ValueClaim operand")
	}
	claim := v.collector.Flow().Operands().DeclareValueClaim(
		current.span, current.owner, current.claim, operand,
	)
	if claim == 0 {
		return fmt.Errorf("programlower: could not create ValueClaim")
	}
	if current.claim == flowkind.ValueClaimNonNil {
		v.phase.SetResult(claim, false)
		return nil
	}
	if current.target == nil {
		return fmt.Errorf("programlower: missing ValueClaim target")
	}
	v.schedule(step{kind: stepClaimTarget, claimTerm: claim})
	targetSpan, ok := inbox.TypeSpan(current.target, current.span.File)
	if !ok {
		return fmt.Errorf("programlower: missing ValueClaim target span")
	}
	return v.statics.PushType(current.target, claim, current.owner, targetSpan)
}

func (v *Values) enqueueExpression(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if !expression(expr) {
		return fmt.Errorf("programlower: invalid nested expression %T", expr)
	}
	span, ok := sourcecoord.Build(span.File, expr.Line(), expr.Column(), expr.LastLine(), expr.LastColumn())
	if !ok {
		span = sourcecoord.Invalid(span.File)
	}
	return v.expressions.Push(expr, owner, span)
}

func (v *Values) schedule(next step) {
	v.steps = append(v.steps, next)
	v.phase.Push(phase.Eval)
}

func (v *Values) number(span source.Span, owner keyspace.Term, literal string) (keyspace.Term, error) {
	if integer, ok := numparse.ParseIntegerLiteral(literal); ok {
		term := v.collector.Source().Literals().Integer(span, owner, integer)
		if term != 0 {
			return term, nil
		}
		return 0, fmt.Errorf("programlower: could not create integer literal")
	}
	value, ok := numparse.ParseFloatLiteral(literal)
	if !ok {
		return 0, fmt.Errorf("programlower: invalid numeric literal %q", literal)
	}
	term := v.collector.Source().Literals().Float(span, owner, value)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create float literal")
	}
	return term, nil
}

// expression is the closed concrete AST expression vocabulary. It makes
// typed nil invalid at every nested expression boundary.
func expression(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.NilExpr:
		return typed != nil
	case *ast.TrueExpr:
		return typed != nil
	case *ast.FalseExpr:
		return typed != nil
	case *ast.NumberExpr:
		return typed != nil
	case *ast.StringExpr:
		return typed != nil
	case *ast.Comma3Expr:
		return typed != nil
	case *ast.IdentExpr:
		return typed != nil
	case *ast.AttrGetExpr:
		return typed != nil
	case *ast.TableExpr:
		return typed != nil
	case *ast.FuncCallExpr:
		return typed != nil
	case *ast.LogicalOpExpr:
		return typed != nil
	case *ast.RelationalOpExpr:
		return typed != nil
	case *ast.StringConcatOpExpr:
		return typed != nil
	case *ast.ArithmeticOpExpr:
		return typed != nil
	case *ast.UnaryMinusOpExpr:
		return typed != nil
	case *ast.UnaryNotOpExpr:
		return typed != nil
	case *ast.UnaryLenOpExpr:
		return typed != nil
	case *ast.UnaryBNotOpExpr:
		return typed != nil
	case *ast.FunctionExpr:
		return typed != nil
	case *ast.CastExpr:
		return typed != nil
	case *ast.NonNilAssertExpr:
		return typed != nil
	default:
		return false
	}
}

// evalExpression is the exact subset whose semantics belong to eval. Vararg
// is deliberately lexical because its Cell identity is function-boundary
// evidence, not expression-local evaluation.
func evalExpression(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.NilExpr:
		return typed != nil
	case *ast.TrueExpr:
		return typed != nil
	case *ast.FalseExpr:
		return typed != nil
	case *ast.NumberExpr:
		return typed != nil
	case *ast.StringExpr:
		return typed != nil
	case *ast.UnaryMinusOpExpr:
		return typed != nil
	case *ast.UnaryNotOpExpr:
		return typed != nil
	case *ast.UnaryLenOpExpr:
		return typed != nil
	case *ast.UnaryBNotOpExpr:
		return typed != nil
	case *ast.ArithmeticOpExpr:
		return typed != nil
	case *ast.StringConcatOpExpr:
		return typed != nil
	case *ast.RelationalOpExpr:
		return typed != nil
	case *ast.LogicalOpExpr:
		return typed != nil
	case *ast.CastExpr:
		return typed != nil
	case *ast.NonNilAssertExpr:
		return typed != nil
	default:
		return false
	}
}

// typeExpression is the closed concrete static AST vocabulary. It keeps a
// typed-nil cast target from entering Static through the narrow inbox.
func typeExpression(expr ast.TypeExpr) bool {
	switch typed := expr.(type) {
	case *ast.AnnotatedTypeExpr:
		return typed != nil
	case *ast.PrimitiveTypeExpr:
		return typed != nil
	case *ast.OptionalTypeExpr:
		return typed != nil
	case *ast.UnionTypeExpr:
		return typed != nil
	case *ast.IntersectionTypeExpr:
		return typed != nil
	case *ast.ArrayTypeExpr:
		return typed != nil
	case *ast.MapTypeExpr:
		return typed != nil
	case *ast.RecordTypeExpr:
		return typed != nil
	case *ast.FunctionTypeExpr:
		return typed != nil
	case *ast.AssertsTypeExpr:
		return typed != nil
	case *ast.TypeRefExpr:
		return typed != nil
	case *ast.GenericTypeExpr:
		return typed != nil
	case *ast.LiteralTypeExpr:
		return typed != nil
	case *ast.TypeOfExpr:
		return typed != nil
	case *ast.KeyOfExpr:
		return typed != nil
	case *ast.IndexAccessExpr:
		return typed != nil
	case *ast.ConditionalTypeExpr:
		return typed != nil
	default:
		return false
	}
}

func arithmetic(operator string) (flowkind.BinaryOp, bool) {
	switch operator {
	case "+":
		return flowkind.BinaryAdd, true
	case "-":
		return flowkind.BinarySub, true
	case "*":
		return flowkind.BinaryMul, true
	case "/":
		return flowkind.BinaryDiv, true
	case "//":
		return flowkind.BinaryIDiv, true
	case "%":
		return flowkind.BinaryMod, true
	case "^":
		return flowkind.BinaryPow, true
	case "&":
		return flowkind.BinaryBitAnd, true
	case "|":
		return flowkind.BinaryBitOr, true
	case "~":
		return flowkind.BinaryBitXor, true
	case "<<":
		return flowkind.BinaryShiftLeft, true
	case ">>":
		return flowkind.BinaryShiftRight, true
	default:
		return 0, false
	}
}

func relational(operator string) (flowkind.BinaryOp, bool) {
	switch operator {
	case "==":
		return flowkind.BinaryEqual, true
	case "~=":
		return flowkind.BinaryNotEqual, true
	case "<":
		return flowkind.BinaryLess, true
	case "<=":
		return flowkind.BinaryLessEqual, true
	case ">":
		return flowkind.BinaryGreater, true
	case ">=":
		return flowkind.BinaryGreaterEqual, true
	default:
		return 0, false
	}
}

func selection(operator string) (flowkind.SelectOp, bool) {
	switch operator {
	case "and":
		return flowkind.SelectAnd, true
	case "or":
		return flowkind.SelectOr, true
	default:
		return 0, false
	}
}

// Clean reports whether every scalar and Values continuation completed.
func (v *Values) Clean() bool {
	return v != nil && len(v.terms) == 0 && len(v.open) == 0 && len(v.steps) == 0
}
