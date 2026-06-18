package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// memberRead reports dot-field reads of a field that is provably absent on a
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
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	seen := make(map[*ast.AttrGetExpr]struct{})
	for _, point := range graph.RPO() {
		typers := memberReadTypers{
			narrowed: newStructuralFlowExpressionTyper(result, p.resolver, point, envs[point]),
			base:     newStructuralFlowExpressionTyper(result, p.resolver, point, guardEnv{}),
			result:   result,
			point:    point,
		}
		emit := func(expr ast.Expr) {
			p.walk(expr, typers, seen, &out)
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
	}
	return out
}

func (p memberRead) walk(expr ast.Expr, typers memberReadTypers, seen map[*ast.AttrGetExpr]struct{}, out *[]diagnostic.Diagnostic) {
	p.walkDepth(expr, typers, seen, out, 0)
}

func (p memberRead) walkDepth(expr ast.Expr, typers memberReadTypers, seen map[*ast.AttrGetExpr]struct{}, out *[]diagnostic.Diagnostic, depth int) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		p.walkDepth(e.Object, typers, seen, out, depth+1)
		if e.KeySyntax == ast.AttrKeyIndex {
			p.walkDepth(e.Key, typers, seen, out, depth+1)
			return
		}
		if _, done := seen[e]; done {
			return
		}
		seen[e] = struct{}{}
		if d, ok := p.read(e, typers); ok {
			*out = append(*out, d)
		}
	case *ast.FuncCallExpr:
		// The callee's own field access is a member call validated by memberCall;
		// descend into its object but do not report the called member as a read.
		if callee, ok := e.Func.(*ast.AttrGetExpr); ok && callee.KeySyntax == ast.AttrKeyDot {
			p.walkDepth(callee.Object, typers, seen, out, depth+1)
		} else {
			p.walkDepth(e.Func, typers, seen, out, depth+1)
		}
		p.walkDepth(e.Receiver, typers, seen, out, depth+1)
		for _, arg := range e.Args {
			p.walkDepth(arg, typers, seen, out, depth+1)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				p.walkDepth(field.Key, typers, seen, out, depth+1)
			}
			p.walkDepth(field.Value, typers, seen, out, depth+1)
		}
	case *ast.LogicalOpExpr:
		p.walkDepth(e.Lhs, typers, seen, out, depth+1)
		p.walkDepth(e.Rhs, typers, seen, out, depth+1)
	case *ast.RelationalOpExpr:
		p.walkDepth(e.Lhs, typers, seen, out, depth+1)
		p.walkDepth(e.Rhs, typers, seen, out, depth+1)
	case *ast.StringConcatOpExpr:
		p.walkDepth(e.Lhs, typers, seen, out, depth+1)
		p.walkDepth(e.Rhs, typers, seen, out, depth+1)
	case *ast.ArithmeticOpExpr:
		p.walkDepth(e.Lhs, typers, seen, out, depth+1)
		p.walkDepth(e.Rhs, typers, seen, out, depth+1)
	case *ast.UnaryMinusOpExpr:
		p.walkDepth(e.Expr, typers, seen, out, depth+1)
	case *ast.UnaryNotOpExpr:
		p.walkDepth(e.Expr, typers, seen, out, depth+1)
	case *ast.UnaryLenOpExpr:
		p.walkDepth(e.Expr, typers, seen, out, depth+1)
	case *ast.UnaryBNotOpExpr:
		p.walkDepth(e.Expr, typers, seen, out, depth+1)
	case *ast.CastExpr:
		p.walkDepth(e.Expr, typers, seen, out, depth+1)
	case *ast.NonNilAssertExpr:
		p.walkDepth(e.Expr, typers, seen, out, depth+1)
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
}

// receiverType resolves the dot-read receiver type, preferring the flow-narrowed
// typer and falling back to the solved boundary value with presence. A nested
// path built from a local table literal (artifact.meta) has no annotation the
// flow typer can lower, but its converged boundary value carries the declared
// union, so the union-arm field-read check still applies.
func (t memberReadTypers) receiverType(obj ast.Expr) (typ.Type, bool) {
	if rt, ok := t.boundaryType(obj); ok {
		return rt, true
	}
	if rt, ok := t.narrowed.typeOf(obj); ok && rt != nil {
		return rt, true
	}
	return nil, false
}

func (t memberReadTypers) boundaryType(obj ast.Expr) (typ.Type, bool) {
	if t.result == nil {
		return nil, false
	}
	value, ok := t.result.ExpressionValueAtBoundary(t.point, obj)
	if !ok {
		return nil, false
	}
	return readmodel.New(t.result).ValueTypeWithPresence(value)
}

// fullyNarrowedType returns the receiver type with every sound flow narrowing
// applied (discriminant, runtime-kind type() guards, presence). The union-arm
// field-read check must run against this fully narrowed shape so a type() guard
// that removes the offending scalar arm suppresses the diagnostic. The
// witness-refined flow typer is authoritative when it resolves; for a nested path
// built from a local table literal it cannot, so the solved boundary value is
// refined against its own structural witness, which already reflects guards.
func (t memberReadTypers) fullyNarrowedType(obj ast.Expr) (typ.Type, bool) {
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
	value, ok := t.result.ExpressionValueAtBoundary(t.point, obj)
	if !ok {
		return nil, false
	}
	declared, ok := readmodel.New(t.result).ValueTypeWithPresence(value)
	if !ok || declared == nil {
		return nil, false
	}
	if refined, ok := readmodel.New(t.result).RefineDeclaredType(declared, value); ok && refined != nil {
		return refined, true
	}
	return declared, true
}

func (p memberRead) read(expr *ast.AttrGetExpr, typers memberReadTypers) (diagnostic.Diagnostic, bool) {
	if expr == nil || expr.KeySyntax != ast.AttrKeyDot {
		return diagnostic.Diagnostic{}, false
	}
	name := ast.KeyName(expr.Key)
	if name == "" {
		return diagnostic.Diagnostic{}, false
	}
	if fully, ok := typers.fullyNarrowedType(expr.Object); ok && unionArmRejectsFieldRead(fully, name) {
		return missingMemberReadDiagnostic(expr, fully, name), true
	}
	receiver, ok := typers.receiverType(expr.Object)
	if !ok || receiver == nil {
		return diagnostic.Diagnostic{}, false
	}
	broad, ok := typers.base.broadType(expr.Object)
	if !ok || broad == nil {
		return diagnostic.Diagnostic{}, false
	}
	if !isMultiArmUnion(broad) {
		return diagnostic.Diagnostic{}, false
	}
	fieldBroad := broad
	if withoutNil := projectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
		fieldBroad = withoutNil
	}
	if _, ok := access.Field(fieldBroad, name); !ok {
		return diagnostic.Diagnostic{}, false
	}
	if !fieldProvablyAbsent(receiver, name) {
		return diagnostic.Diagnostic{}, false
	}
	return missingMemberReadDiagnostic(expr, receiver, name), true
}

// unionArmRejectsFieldRead reports whether a dot-field read of name on t is a
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

// isMultiArmUnion reports whether t resolves to a union of two or more members,
// i.e. a receiver that discriminant narrowing can soundly collapse.
func isMultiArmUnion(t typ.Type) bool {
	return multiArmUnionDepth(t, 0)
}

func multiArmUnionDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return len(v.Members) >= 2
	case *typ.Alias:
		return multiArmUnionDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return multiArmUnionDepth(v.Body, depth+1)
	default:
		return false
	}
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
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeMissingMember,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s has no member %q", formatType(receiver), name),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("read accesses field %q", name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("receiver type at read is %s", formatType(receiver)),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "missing member read"}},
	}
}
