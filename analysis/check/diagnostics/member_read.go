package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// memberRead reports static field reads of a field that is provably absent on a
// closed record receiver (or a union all of whose members are closed records).
// It mirrors memberCall's missing-member diagnostic for plain reads.
type memberRead producerContext

func (p memberRead) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := producerContext(p).guardEnvironments(result)
	collector := newMemberReadCollector()
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		typers := memberReadTypers{
			narrowed: newStructuralFlowExpressionTyper(result, p.resolver, point, envs[point]),
			base:     newStructuralFlowExpressionTyper(result, p.resolver, point, guardEnv{}),
			result:   result,
			point:    point,
			flow:     producerContext(p).flow,
		}
		emit := func(expr ast.Expr) {
			p.walk(expr, typers, collector)
		}
		if fact, ok := result.LocalAssignment(point); ok {
			emit(fact.Expr)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			emit(fact.Value)
			emitAssignmentTargetReads(fact.Target, emit)
		}
		if fact, ok := result.Call(point); ok {
			emit(fact.Call)
		}
		if fact, ok := result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				emit(expr)
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			emit(fact.Condition)
		}
		if fact, ok := result.ExpressionEvaluation(point); ok {
			emit(fact.Expr)
		}
	}
	return collector.Diagnostics()
}

type memberReadCollector struct {
	pending    []memberReadPending
	byExpr     map[*ast.AttrGetExpr]int
	suppressed map[*ast.AttrGetExpr]struct{}
}

type memberReadPending struct {
	expr *ast.AttrGetExpr
	diag diagnostic.Diagnostic
}

func newMemberReadCollector() *memberReadCollector {
	return &memberReadCollector{
		byExpr:     make(map[*ast.AttrGetExpr]int),
		suppressed: make(map[*ast.AttrGetExpr]struct{}),
	}
}

func (c *memberReadCollector) Suppress(expr *ast.AttrGetExpr) {
	if c == nil || expr == nil {
		return
	}
	c.suppressed[expr] = struct{}{}
}

func (c *memberReadCollector) Add(expr *ast.AttrGetExpr, diag diagnostic.Diagnostic) {
	if c == nil || expr == nil {
		return
	}
	if _, suppressed := c.suppressed[expr]; suppressed {
		return
	}
	if _, exists := c.byExpr[expr]; exists {
		return
	}
	c.byExpr[expr] = len(c.pending)
	c.pending = append(c.pending, memberReadPending{expr: expr, diag: diag})
}

func (c *memberReadCollector) Diagnostics() []diagnostic.Diagnostic {
	if c == nil || len(c.pending) == 0 {
		return nil
	}
	out := make([]diagnostic.Diagnostic, 0, len(c.pending))
	for _, pending := range c.pending {
		if _, suppressed := c.suppressed[pending.expr]; suppressed {
			continue
		}
		out = append(out, pending.diag)
	}
	return out
}

func (p memberRead) walk(expr ast.Expr, typers memberReadTypers, collector *memberReadCollector) {
	p.walkDepth(expr, typers, collector, 0, false)
}

func (p memberRead) walkDepth(
	expr ast.Expr,
	typers memberReadTypers,
	collector *memberReadCollector,
	depth int,
	allowExactNilRead bool,
) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		p.walkDepth(e.Object, typers, collector, depth+1, false)
		if e.KeySyntax == ast.AttrKeyIndex {
			p.walkDepth(e.Key, typers, collector, depth+1, false)
		}
		if d, ok, suppressed := p.read(e, typers, allowExactNilRead); suppressed {
			collector.Suppress(e)
		} else if ok {
			collector.Add(e, d)
		}
	case *ast.FuncCallExpr:
		// The callee's own field access is a member call validated by memberCall;
		// descend into its object but do not report the called member as a read.
		if callee, ok := e.Func.(*ast.AttrGetExpr); ok && callee.KeySyntax == ast.AttrKeyDot {
			p.walkDepth(callee.Object, typers, collector, depth+1, false)
		} else {
			p.walkDepth(e.Func, typers, collector, depth+1, false)
		}
		p.walkDepth(e.Receiver, typers, collector, depth+1, false)
		for _, arg := range e.Args {
			p.walkDepth(arg, typers, collector, depth+1, false)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				p.walkDepth(field.Key, typers, collector, depth+1, false)
			}
			p.walkDepth(field.Value, typers, collector, depth+1, false)
		}
	case *ast.LogicalOpExpr:
		p.walkDepth(e.Lhs, typers, collector, depth+1, e.Operator == "or")
		switch e.Operator {
		case "and":
			next, reachable := typers.withExpressionEdgeFacts(e.Lhs, true)
			if reachable {
				p.walkDepth(e.Rhs, next, collector, depth+1, false)
			}
		case "or":
			next, reachable := typers.withExpressionEdgeFacts(e.Lhs, false)
			if reachable {
				p.walkDepth(e.Rhs, next, collector, depth+1, false)
			}
		default:
			p.walkDepth(e.Rhs, typers, collector, depth+1, false)
		}
	case *ast.RelationalOpExpr:
		p.walkDepth(e.Lhs, typers, collector, depth+1, false)
		p.walkDepth(e.Rhs, typers, collector, depth+1, false)
	case *ast.StringConcatOpExpr:
		p.walkDepth(e.Lhs, typers, collector, depth+1, false)
		p.walkDepth(e.Rhs, typers, collector, depth+1, false)
	case *ast.ArithmeticOpExpr:
		p.walkDepth(e.Lhs, typers, collector, depth+1, false)
		p.walkDepth(e.Rhs, typers, collector, depth+1, false)
	case *ast.UnaryMinusOpExpr:
		p.walkDepth(e.Expr, typers, collector, depth+1, false)
	case *ast.UnaryNotOpExpr:
		p.walkDepth(e.Expr, typers, collector, depth+1, false)
	case *ast.UnaryLenOpExpr:
		p.walkDepth(e.Expr, typers, collector, depth+1, false)
	case *ast.UnaryBNotOpExpr:
		p.walkDepth(e.Expr, typers, collector, depth+1, false)
	case *ast.CastExpr:
		p.walkDepth(e.Expr, typers, collector, depth+1, false)
	case *ast.NonNilAssertExpr:
		p.walkDepth(e.Expr, typers, collector, depth+1, false)
	}
}

// memberReadTypers pairs the flow-narrowed receiver typer with the broad typer
// that yields the un-narrowed declared shape. Absence is reported only when the
// broad shape admits the field but discriminant narrowing collapsed the receiver
// to a closed record that lacks it. A field absent on the broad shape too is a
// single-shape observed snapshot the read may legitimately extend, so it is
// never flagged.
type memberReadTypers struct {
	narrowed expressionTyper
	base     expressionTyper
	result   *body.Result
	point    cfg.Point
	flow     *diagnosticFlowCache
}

func (t memberReadTypers) withExpressionEdgeFacts(expr ast.Expr, cond bool) (memberReadTypers, bool) {
	if t.result == nil || expr == nil {
		return t, true
	}
	env, reachable := applyExpressionEdgeGuards(t.result, t.point, expr, cond, t.narrowed.env)
	if !reachable {
		return t, false
	}
	t.narrowed.env = env
	return t, true
}

// receiverType resolves the dot-read receiver type, preferring the flow-narrowed
// typer and falling back to the solved boundary value with presence. A nested
// path built from a local table literal (artifact.meta) has no annotation the
// flow typer can lower, but its converged boundary value carries the declared
// union, so the union-arm field-read check still applies.
func (t memberReadTypers) receiverType(obj ast.Expr) (typ.Type, bool) {
	if rt, ok := t.boundaryType(obj); ok {
		if narrowed, ok := t.stableLiteralNarrowedBroadType(obj, rt); ok {
			return narrowed, true
		}
		return rt, true
	}
	if rt, ok := t.narrowed.typeOf(obj); ok && rt != nil {
		if narrowed, ok := t.stableLiteralNarrowedBroadType(obj, rt); ok {
			return narrowed, true
		}
		return rt, true
	}
	return nil, false
}

func (t memberReadTypers) boundaryType(obj ast.Expr) (typ.Type, bool) {
	if t.result == nil {
		return nil, false
	}
	value, ok := newDiagnosticQuery(t.result).ExpressionValueAtBoundary(t.point, obj)
	if !ok {
		return nil, false
	}
	return newDiagnosticQuery(t.result).ValueTypeWithPresence(value)
}

// fullyNarrowedType returns the receiver type with every sound flow narrowing
// applied (discriminant, runtime-kind type() guards, presence). The union-arm
// field-read check must run against this fully narrowed shape so a type() guard
// that removes the offending scalar arm suppresses the diagnostic. The
// witness-refined flow typer is authoritative when it resolves; for a nested path
// built from a local table literal it cannot, so the solved boundary value is
// refined against its own structural witness, which already reflects guards.
func (t memberReadTypers) fullyNarrowedType(obj ast.Expr) (typ.Type, bool) {
	if rt, ok := t.narrowed.typeOf(obj); ok && rt != nil {
		return rt, true
	}
	witnessTyper := t.narrowed
	witnessTyper.witnessRefine = true
	if rt, ok := witnessTyper.typeOf(obj); ok && rt != nil {
		return rt, true
	}
	if rt, ok := t.boundaryType(obj); ok {
		return rt, true
	}
	if t.result == nil {
		return nil, false
	}
	value, ok := newDiagnosticQuery(t.result).ExpressionValueAtBoundary(t.point, obj)
	if !ok {
		return nil, false
	}
	query := newDiagnosticQuery(t.result)
	declared, ok := query.ValueTypeWithPresence(value)
	if !ok || declared == nil {
		return nil, false
	}
	if refined, ok := query.RefineDeclaredType(declared, value); ok && refined != nil {
		return refined, true
	}
	return declared, true
}

func (p memberRead) read(expr *ast.AttrGetExpr, typers memberReadTypers, allowExactNilRead bool) (diagnostic.Diagnostic, bool, bool) {
	name, ok := staticMemberReadName(expr)
	if !ok {
		return diagnostic.Diagnostic{}, false, false
	}
	if allowExactNilRead && typers.exactLocalMissingFieldReadsNil(expr, name) {
		return diagnostic.Diagnostic{}, false, true
	}
	if typers.receiverHasUntrustedTopOrigin(expr.Object) {
		return diagnostic.Diagnostic{}, false, false
	}
	if fully, ok := typers.fullyNarrowedType(expr.Object); ok && unionArmRejectsFieldRead(fully, name) {
		return missingMemberReadDiagnostic(expr, fully, name), true, false
	}
	receiver, ok := typers.receiverType(expr.Object)
	if !ok || receiver == nil {
		return diagnostic.Diagnostic{}, false, false
	}
	broad, ok := typers.base.broadType(expr.Object)
	if !ok || broad == nil {
		return diagnostic.Diagnostic{}, false, false
	}
	if !inspect.IsMultiArmUnion(broad) {
		return diagnostic.Diagnostic{}, false, false
	}
	fieldBroad := broad
	if withoutNil := projectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
		fieldBroad = withoutNil
	}
	if _, ok := access.Field(fieldBroad, name); !ok {
		return diagnostic.Diagnostic{}, false, false
	}
	if !fieldProvablyAbsent(receiver, name) {
		return diagnostic.Diagnostic{}, false, false
	}
	return missingMemberReadDiagnostic(expr, receiver, name), true, false
}

func (t memberReadTypers) receiverHasUntrustedTopOrigin(obj ast.Expr) bool {
	if t.result == nil || obj == nil {
		return false
	}
	value, ok := newDiagnosticQuery(t.result).ExpressionValueAtBoundary(t.point, obj)
	if !ok {
		return false
	}
	return newDiagnosticQuery(t.result).ValueHasUntrustedTopOrigin(value)
}

func (t memberReadTypers) stableLiteralNarrowedBroadType(obj ast.Expr, current typ.Type) (typ.Type, bool) {
	if t.result == nil || obj == nil || current == nil || typ.IsNever(current) {
		return nil, false
	}
	receiverPath, ok := t.result.ExpressionPath(obj)
	if !ok || receiverPath.IsEmpty() {
		return nil, false
	}
	broad, ok := t.base.broadType(obj)
	if !ok || broad == nil || typ.IsNever(broad) {
		return nil, false
	}
	query := newDiagnosticQuery(t.result)
	if query.IsEquivalent(current, broad) {
		return nil, false
	}
	stable := guardEnv{}
	for _, constraint := range t.narrowed.env.constraints {
		if constraint.target.IsEmpty() || constraint.value == nil {
			continue
		}
		if _, ok := suffixFromReceiver(receiverPath, constraint.target); !ok {
			continue
		}
		if t.guardInvalidatedBeforeRead(receiverPath, constraint) {
			continue
		}
		stable.constraints = append(stable.constraints, constraint)
	}
	if len(stable.constraints) == 0 {
		return nil, false
	}
	narrowed, changed := applyLiteralNarrowing(broad, receiverPath, stable)
	if !changed || narrowed == nil || typ.IsNever(narrowed) || query.IsEquivalent(narrowed, broad) {
		return nil, false
	}
	return narrowed, true
}

func (t memberReadTypers) guardInvalidatedBeforeRead(receiver pathdom.Path, constraint literalConstraint) bool {
	if guardPoint, ok := t.guardPointForSpan(constraint.span); ok {
		return pathInvalidatedBetween(t.result, t.flow, guardPoint, t.point, receiver) ||
			pathInvalidatedBetween(t.result, t.flow, guardPoint, t.point, constraint.target)
	}
	return pathInvalidatedBefore(t.result, t.flow, t.point, receiver) ||
		pathInvalidatedBefore(t.result, t.flow, t.point, constraint.target)
}

func (t memberReadTypers) guardPointForSpan(span diagnostic.Span) (cfg.Point, bool) {
	if !span.Valid() || t.result == nil {
		return 0, false
	}
	graph := t.result.Graph()
	if graph == nil {
		return 0, false
	}
	for _, point := range graph.RPO() {
		if !diagnosticCanReach(t.flow, graph, point, t.point) {
			continue
		}
		fact, ok := t.result.BranchCondition(point)
		if !ok {
			continue
		}
		condSpan := ast.SpanOf(fact.Condition)
		if !condSpan.Valid() {
			condSpan = ast.SpanOf(fact.Stmt)
		}
		if spansEqual(condSpan, span) {
			return point, true
		}
	}
	return 0, false
}

func spansEqual(left, right diagnostic.Span) bool {
	return left.Valid() && right.Valid() &&
		left.StartLine == right.StartLine &&
		left.StartCol == right.StartCol &&
		left.EndLine == right.EndLine &&
		left.EndCol == right.EndCol
}

func pathInvalidatedBefore(result *body.Result, flow *diagnosticFlowCache, to cfg.Point, target pathdom.Path) bool {
	if result == nil || target.IsEmpty() {
		return false
	}
	graph := result.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to || !diagnosticCanReach(flow, graph, candidate, to) {
			continue
		}
		if invalidation, ok := result.PathDescendantInvalidation(candidate); ok && target.HasStrictPrefix(invalidation.ContainerPath()) {
			return true
		}
		if fact, ok := result.OrdinaryAssignment(candidate); ok && ordinaryAssignmentInvalidatesMemberPath(fact, target) {
			return true
		}
		if callMayInvalidateTrackedPath(result, candidate, target) {
			return true
		}
	}
	return false
}

func (t memberReadTypers) exactLocalMissingFieldReadsNil(expr *ast.AttrGetExpr, name string) bool {
	if t.result == nil || expr == nil || name == "" {
		return false
	}
	value, ok := newDiagnosticQuery(t.result).ExpressionValueBeforeBoundary(t.point, expr.Object)
	if !ok {
		return false
	}
	query := newDiagnosticQuery(t.result)
	if !query.ValueHasLocalExclusiveExactIdentity(t.point, value) {
		return false
	}
	receiver, ok := query.ValueType(value)
	if !ok || receiver == nil {
		return false
	}
	return recordProvablyMissesField(receiver, name)
}

func staticMemberReadName(expr *ast.AttrGetExpr) (string, bool) {
	if expr == nil {
		return "", false
	}
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		name := ast.KeyName(expr.Key)
		return name, name != ""
	case ast.AttrKeyIndex:
		key, ok := expr.Key.(*ast.StringExpr)
		if !ok || key.Value == "" {
			return "", false
		}
		return key.Value, true
	default:
		return "", false
	}
}

// unionArmRejectsFieldRead reports whether a static field read of name on t is a
// sound type error because t is a multi-arm union where at least one arm carries
// the field while another arm is a non-table value (string, number, boolean) that
// neither carries it nor yields nil on a missing read. Indexing that scalar arm is
// a runtime error, so the read is unsound until the union is narrowed to its table
// arms. A union all of whose arms are tables (missing reads yield nil) is allowed.
func unionArmRejectsFieldRead(t typ.Type, name string) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if projectionHasNil(t) {
		return false
	}
	union, ok := unwrap.Annotated(t).(*typ.Union)
	if !ok || len(union.Members) < 2 {
		return false
	}
	carriesField := false
	rejectingArm := false
	for _, member := range union.Members {
		if _, ok := access.Field(member, name); ok {
			carriesField = true
			continue
		}
		if access.MissingFieldReadsNil(member) {
			continue
		}
		rejectingArm = true
	}
	return carriesField && rejectingArm
}

// fieldProvablyAbsent reports whether a dot-field read of name on t is a sound
// type error: t resolves to a closed record (or a union all of whose members
// are closed records) that statically lacks the field. Any receiver shape that
// admits the read (any/unknown/never, open record, map component, metatable,
// optional/nil-bearing, interface, map) yields false so the read is allowed.
func fieldProvablyAbsent(t typ.Type, name string) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if projectionHasNil(t) {
		return false
	}
	if _, ok := access.Field(t, name); ok {
		return false
	}
	return closedRecordLacksField(t, name, 0)
}

// closedRecordLacksField reports whether every reachable member of t is a closed
// record without a map component or metatable that lacks name. A union qualifies
// only when all members qualify; any other shape disqualifies the whole receiver.
func closedRecordLacksField(t typ.Type, name string, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return closedRecordWithoutField(v, name)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !closedRecordLacksField(member, name, depth+1) {
				return false
			}
		}
		return true
	case *typ.Alias:
		return closedRecordLacksField(v.UnaliasedTarget(), name, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return closedRecordLacksField(v.Body, name, depth+1)
	default:
		return false
	}
}

func closedRecordWithoutField(r *typ.Record, name string) bool {
	if r == nil || r.Open || r.HasMapComponent() || r.Metatable != nil {
		return false
	}
	if r.GetField(name) != nil {
		return false
	}
	if r.GetStaticStringIndex(name) != nil {
		return false
	}
	return true
}

func missingMemberReadDiagnostic(expr *ast.AttrGetExpr, receiver typ.Type, name string) diagnostic.Diagnostic {
	span := ast.SpanOf(expr)
	readPath := exprEvidenceNameOK(expr)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeMissingMember,
		Severity: diagnostic.SeverityError,
		Message:  missingMemberMessage(receiver, name),
		Labels:   []diagnostic.Label{sourceLabel(span, labelMemberRead)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: memberReadReceiverEvidence(readPath, name, receiver),
			},
		),
		Help: missingMemberHelp(name),
	})
}
