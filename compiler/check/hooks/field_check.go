// field_check.go implements field access validation for the type checker.
//
// This pass runs after flow analysis to validate that field accesses are valid
// on their narrowed types. It catches errors like:
//   - Accessing non-existent fields on records/interfaces
//   - Indexing non-indexable types
//   - Invalid operands in arithmetic expressions
//
// The checker walks the CFG and examines AttrGetExpr nodes, checking each
// field access against the narrowed type of the receiver at that program point.
//
// For union types, the error message describes which union members have the
// field and which are missing it, helping the user understand why narrowing
// is needed.
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonicaldiag "github.com/wippyai/go-lua/compiler/check/canonical/diagnostic"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldaccess"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CheckFields validates field accesses on narrowed types.
func CheckFields(graph *cfg.Graph, evidence api.FlowEvidence, observer observation.Projector, flowOps api.FlowOps, sourceName string) []diag.Diagnostic {
	if graph == nil {
		return nil
	}

	bindings := graph.Bindings()
	observer = observer.WithGradualParamReads()
	resolver := fieldResolverImpl{observer: observer, bindings: bindings, graph: graph, flowOps: flowOps}
	preStateResolver := resolver
	if flowOps != nil {
		preStateResolver.observer = observer.WithPreStateReads()
	}

	var diags []diag.Diagnostic
	for _, assign := range evidence.Assignments {
		p := assign.Point
		if fieldPointIsDead(flowOps, p) {
			continue
		}
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachSource(func(_ int, source ast.Expr) {
			diags = append(diags, checkFieldExpr(source, p, preStateResolver, make(map[ast.Expr]bool), sourceName)...)
		})
		if info.NumericFor != nil {
			diags = append(diags, checkNumericFor(info.NumericFor, p, resolver, sourceName)...)
		}
	}

	for _, call := range evidence.Calls {
		if call.Origin != api.CallOriginStatement {
			continue
		}
		p := call.Point
		if fieldPointIsDead(flowOps, p) {
			continue
		}
		info := call.Info
		if info == nil {
			continue
		}
		if info.Callee != nil {
			diags = append(diags, checkFieldExpr(info.Callee, p, resolver, make(map[ast.Expr]bool), sourceName)...)
		}
		if info.Receiver != nil {
			diags = append(diags, checkFieldExpr(info.Receiver, p, resolver, make(map[ast.Expr]bool), sourceName)...)
		}
		for _, arg := range info.Args {
			diags = append(diags, checkFieldExpr(arg, p, resolver, make(map[ast.Expr]bool), sourceName)...)
		}
	}

	for _, ret := range evidence.Returns {
		p := ret.Point
		if fieldPointIsDead(flowOps, p) {
			continue
		}
		info := ret.Info
		if info == nil {
			continue
		}
		for _, expr := range info.Exprs {
			diags = append(diags, checkFieldExpr(expr, p, resolver, make(map[ast.Expr]bool), sourceName)...)
		}
	}

	for _, branch := range evidence.Branches {
		p := branch.Point
		if fieldPointIsDead(flowOps, p) {
			continue
		}
		info := branch.Info
		if info == nil {
			continue
		}
		diags = append(diags, checkFieldProbe(info.Condition, p, resolver, make(map[ast.Expr]bool), sourceName)...)
	}

	return dedupeFieldDiagnostics(diags)
}

func fieldPointIsDead(flowOps api.FlowOps, p cfg.Point) bool {
	return flowOps != nil && flowOps.IsPointDead(p)
}

func dedupeFieldDiagnostics(diags []diag.Diagnostic) []diag.Diagnostic {
	if len(diags) < 2 {
		return diags
	}
	type key struct {
		file     string
		line     int
		column   int
		code     diag.Code
		severity diag.Severity
		message  string
	}
	seen := make(map[key]struct{}, len(diags))
	out := diags[:0]
	for _, d := range diags {
		k := key{
			file:     d.Position.File,
			line:     d.Position.Line,
			column:   d.Position.Column,
			code:     d.Code,
			severity: d.Severity,
			message:  d.Message,
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, d)
	}
	return out
}

type fieldResolverImpl struct {
	observer observation.Projector
	bindings *bind.BindingTable
	graph    *cfg.Graph
	flowOps  api.FlowOps
}

func (r fieldResolverImpl) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	return r.observer.TypeOf(expr, p)
}

func (r fieldResolverImpl) Field(t typ.Type, name string) (typ.Type, bool) {
	return r.observer.Field(t, name)
}

func (r fieldResolverImpl) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	return r.observer.Index(t, key)
}

func (r fieldResolverImpl) RuntimeIndex(t typ.Type, key typ.Type) (typ.Type, bool) {
	return r.observer.RuntimeIndex(t, key)
}

func (r fieldResolverImpl) IndexedReadProofType(expr *ast.AttrGetExpr, p cfg.Point, keyType typ.Type) (typ.Type, bool) {
	if expr == nil {
		return nil, false
	}
	return r.observer.IndexedReadProofType(expr.Object, expr.Key, keyType, p)
}

func (r fieldResolverImpl) PathOf(expr ast.Expr, p cfg.Point) constraint.Path {
	return path.FromExprWithBindingsAt(expr, nil, r.bindings, r.graph, p)
}

func (r fieldResolverImpl) ExprHasLengthProof(expr ast.Expr, p cfg.Point) bool {
	if r.flowOps == nil {
		return false
	}
	if r.observer.ExprHasLengthProof(expr, p) {
		return true
	}
	path := r.PathOf(expr, p)
	if path.IsEmpty() {
		return false
	}
	if _, _, ok := r.flowOps.LengthBoundsAt(p, path); ok {
		return true
	}
	return ops.MayHaveLength(r.flowOps.NarrowedTypeAt(p, path))
}

func (r fieldResolverImpl) FieldAccessHasPresentValue(expr *ast.AttrGetExpr, p cfg.Point) bool {
	if expr == nil {
		return false
	}
	path := r.PathOf(expr, p)
	if path.IsEmpty() {
		return false
	}
	return r.observer.PathHasPresentProductValue(p, path)
}

func (r fieldResolverImpl) WithExprCondition(expr ast.Expr, p cfg.Point, truthy bool) fieldResolverImpl {
	r.observer = r.observer.WithExprCondition(expr, p, truthy)
	return r
}

type fieldUse uint8

const (
	fieldUseValue fieldUse = iota
	fieldUseProbe
)

func checkFieldExpr(expr ast.Expr, p cfg.Point, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string) []diag.Diagnostic {
	return checkFieldExprUse(expr, p, resolver, seen, sourceName, fieldUseValue)
}

func checkFieldProbe(expr ast.Expr, p cfg.Point, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string) []diag.Diagnostic {
	return checkFieldExprUse(expr, p, resolver, seen, sourceName, fieldUseProbe)
}

func checkFieldExprUse(expr ast.Expr, p cfg.Point, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string, use fieldUse) []diag.Diagnostic {
	if expr == nil || seen[expr] {
		return nil
	}
	seen[expr] = true

	var diags []diag.Diagnostic

	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		diags = append(diags, checkAttrGet(e, p, resolver, seen, sourceName, use)...)
	case *ast.FuncCallExpr:
		diags = append(diags, checkFieldExpr(e.Func, p, resolver, seen, sourceName)...)
		for _, arg := range e.Args {
			if guard.IsTypeCall(e) {
				diags = append(diags, checkTypeProbeArg(arg, p, resolver, seen, sourceName)...)
				continue
			}
			diags = append(diags, checkFieldExpr(arg, p, resolver, seen, sourceName)...)
		}
	case *ast.TableExpr:
		for _, f := range e.Fields {
			diags = append(diags, checkFieldExpr(f.Value, p, resolver, seen, sourceName)...)
		}
	case *ast.LogicalOpExpr:
		lhsUse := use
		if e.Operator == "and" || e.Operator == "or" {
			lhsUse = fieldUseProbe
		}
		diags = append(diags, checkFieldExprUse(e.Lhs, p, resolver, seen, sourceName, lhsUse)...)
		lhsType := resolver.TypeOf(e.Lhs, p)
		if e.Operator == "and" && ops.IsFalsy(lhsType) {
			return diags
		}
		if e.Operator == "or" && ops.IsTruthy(lhsType) {
			return diags
		}
		rhsResolver := applyLogicalOpNarrowing(e, p, resolver)
		diags = append(diags, checkFieldExprUse(e.Rhs, p, rhsResolver, seen, sourceName, use)...)
	case *ast.RelationalOpExpr:
		operandUse := fieldUseValue
		if use == fieldUseProbe && relationalIsEquality(e) {
			operandUse = fieldUseProbe
		}
		diags = append(diags, checkFieldExprUse(e.Lhs, p, resolver, seen, sourceName, operandUse)...)
		diags = append(diags, checkFieldExprUse(e.Rhs, p, resolver, seen, sourceName, operandUse)...)
		diags = append(diags, checkRelational(e, p, resolver, sourceName)...)
	case *ast.ArithmeticOpExpr:
		diags = append(diags, checkFieldExpr(e.Lhs, p, resolver, seen, sourceName)...)
		diags = append(diags, checkFieldExpr(e.Rhs, p, resolver, seen, sourceName)...)
		diags = append(diags, checkArithmetic(e, p, resolver, sourceName)...)
	case *ast.StringConcatOpExpr:
		diags = append(diags, checkFieldExpr(e.Lhs, p, resolver, seen, sourceName)...)
		diags = append(diags, checkFieldExpr(e.Rhs, p, resolver, seen, sourceName)...)
		diags = append(diags, checkStringConcat(e, p, resolver, sourceName)...)
	case *ast.UnaryMinusOpExpr:
		diags = append(diags, checkFieldExpr(e.Expr, p, resolver, seen, sourceName)...)
		diags = append(diags, checkUnaryMinus(e, p, resolver, sourceName)...)
	case *ast.UnaryLenOpExpr:
		diags = append(diags, checkUnaryLength(e, p, resolver, sourceName)...)
	case *ast.UnaryBNotOpExpr:
		diags = append(diags, checkFieldExpr(e.Expr, p, resolver, seen, sourceName)...)
		diags = append(diags, checkUnaryBNot(e, p, resolver, sourceName)...)
	case *ast.UnaryNotOpExpr:
		diags = append(diags, checkFieldExprUse(e.Expr, p, resolver, seen, sourceName, fieldUseProbe)...)
	}

	return diags
}

func applyLogicalOpNarrowing(
	expr *ast.LogicalOpExpr,
	p cfg.Point,
	resolver fieldResolverImpl,
) fieldResolverImpl {
	if expr == nil {
		return resolver
	}

	if resolver.bindings == nil {
		return resolver
	}

	switch expr.Operator {
	case "and", "or":
	default:
		return resolver
	}

	if expr.Operator == "and" {
		return resolver.WithExprCondition(expr.Lhs, p, true)
	}
	if expr.Operator == "or" {
		return resolver.WithExprCondition(expr.Lhs, p, false)
	}
	return resolver
}

func checkArithmetic(e *ast.ArithmeticOpExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	check := func(expr ast.Expr) *diag.Diagnostic {
		t := resolver.TypeOf(expr, p)
		if t == nil || ops.AllowsNumericOperand(t) || typ.IsNever(t) {
			return nil
		}
		msg := "cannot perform arithmetic on " + typ.FormatShort(t) + ", expected number"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return &diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
			Span:     ast.SpanOf(expr),
			Message:  msg,
			Help:     help,
		}
	}
	if d := check(e.Lhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if d := check(e.Rhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	return nil
}

func checkRelational(e *ast.RelationalOpExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	switch e.Operator {
	case "<", "<=", ">", ">=":
	default:
		return nil
	}

	left := resolver.TypeOf(e.Lhs, p)
	right := resolver.TypeOf(e.Rhs, p)
	check := func(expr ast.Expr) *diag.Diagnostic {
		t := resolver.TypeOf(expr, p)
		if t == nil || ops.MayBeOrderable(t) || typ.IsNever(t) || t.Kind() == kind.Nil {
			return nil
		}
		msg := "cannot compare " + typ.FormatShort(t) + ", expected orderable type"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return &diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
			Span:     ast.SpanOf(expr),
			Message:  msg,
			Help:     help,
		}
	}
	if d := check(e.Lhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if d := check(e.Rhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if left != nil && right != nil && !typ.IsNever(left) && !typ.IsNever(right) &&
		left.Kind() != kind.Nil && right.Kind() != kind.Nil &&
		!ops.MayBeSameOrderedFamily(left, right) {
		msg := "cannot compare " + typ.FormatShort(left) + " with " + typ.FormatShort(right) + ", expected both operands to be numbers or both strings"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()},
			Span:     ast.SpanOf(e),
			Message:  msg,
			Help:     help,
		}}
	}
	return nil
}

func checkStringConcat(e *ast.StringConcatOpExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	check := func(expr ast.Expr) *diag.Diagnostic {
		t := resolver.TypeOf(expr, p)
		if t == nil || ops.MayBeStringable(t) || typ.IsNever(t) || t.Kind() == kind.Nil || isOptionalFalseOnly(t) {
			return nil
		}
		msg := "cannot concatenate " + typ.FormatShort(t) + ", expected string or number"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return &diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
			Span:     ast.SpanOf(expr),
			Message:  msg,
			Help:     help,
		}
	}
	if d := check(e.Lhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if d := check(e.Rhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	return nil
}

func isOptionalFalseOnly(t typ.Type) bool {
	t = unwrap.Alias(t)
	opt, ok := t.(*typ.Optional)
	if !ok || opt == nil || opt.Inner == nil {
		return false
	}
	inner := unwrap.Alias(opt.Inner)
	lit, ok := inner.(*typ.Literal)
	if !ok || lit == nil {
		return false
	}
	v, ok := lit.Value.(bool)
	return ok && !v
}

func checkUnaryMinus(e *ast.UnaryMinusOpExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	t := resolver.TypeOf(e.Expr, p)
	if t == nil || ops.AllowsNumericOperand(t) {
		return nil
	}
	msg := "cannot apply unary - to " + typ.FormatShort(t) + ", expected number"
	_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
	return []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Code:     diag.ErrInvalidOperand,
		Position: diag.Position{File: sourceName, Line: e.Expr.Line(), Column: e.Expr.Column()},
		Span:     ast.SpanOf(e.Expr),
		Message:  msg,
		Help:     help,
	}}
}

func checkUnaryLength(e *ast.UnaryLenOpExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	if resolver.ExprHasLengthProof(e.Expr, p) {
		return nil
	}
	t := resolver.TypeOf(e.Expr, p)
	if t == nil || ops.MayHaveLength(t) || typ.IsNever(t) || t.Kind() == kind.Nil {
		return nil
	}
	msg := "cannot apply length operator to " + typ.FormatShort(t) + ", expected string or table"
	_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
	return []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Code:     diag.ErrInvalidOperand,
		Position: diag.Position{File: sourceName, Line: e.Expr.Line(), Column: e.Expr.Column()},
		Span:     ast.SpanOf(e.Expr),
		Message:  msg,
		Help:     help,
	}}
}

func checkUnaryBNot(e *ast.UnaryBNotOpExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	t := resolver.TypeOf(e.Expr, p)
	if t == nil || ops.AllowsBitwiseNumericOperand(t) {
		return nil
	}
	msg := "cannot apply unary ~ to " + typ.FormatShort(t) + ", expected integer"
	_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
	return []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Code:     diag.ErrInvalidOperand,
		Position: diag.Position{File: sourceName, Line: e.Expr.Line(), Column: e.Expr.Column()},
		Span:     ast.SpanOf(e.Expr),
		Message:  msg,
		Help:     help,
	}}
}

func checkNumericFor(info *cfg.NumericForInfo, p cfg.Point, resolver fieldResolverImpl, sourceName string) []diag.Diagnostic {
	var diags []diag.Diagnostic
	check := func(expr ast.Expr, part string) {
		if expr == nil {
			return
		}
		t := resolver.TypeOf(expr, p)
		if t != nil && !ops.AllowsNumericOperand(t) {
			msg := "for loop " + part + " must be numeric, got " + typ.FormatShort(t)
			_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
			diags = append(diags, diag.Diagnostic{
				Severity: diag.SeverityError,
				Code:     diag.ErrTypeMismatch,
				Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
				Span:     ast.SpanOf(expr),
				Message:  msg,
				Help:     help,
			})
		}
	}
	check(info.Init, "init")
	check(info.Limit, "limit")
	check(info.Step, "step")
	return diags
}

func checkAttrGet(e *ast.AttrGetExpr, p cfg.Point, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string, use fieldUse) []diag.Diagnostic {
	var diags []diag.Diagnostic

	diags = append(diags, checkFieldExpr(e.Object, p, resolver, seen, sourceName)...)

	objType := resolver.TypeOf(e.Object, p)

	if indexedReadProofType(e, p, resolver) {
		return diags
	}

	if d, ok := optionalIndexError(objType, e, p, resolver, sourceName); ok {
		diags = append(diags, d)
		return diags
	}

	if !isStringKeyExpr(e.Key) {
		diags = append(diags, checkIndexAccess(e, p, resolver, objType, sourceName)...)
		return diags
	}

	fieldName := ast.KeyName(e.Key)
	result := fieldaccess.Resolve(resolver, e, objType, fieldName, p)

	if result.SkipCheck {
		return diags
	}

	if result.NotIndexable {
		pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
		span := ast.SpanOf(e)
		if e.Object != nil && e.Object.Line() > 0 {
			pos.Line = e.Object.Line()
			pos.Column = e.Object.Column()
			span = ast.SpanOf(e.Object)
		}
		msg := "cannot index type " + typ.FormatShort(objType)
		_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
		diags = append(diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrTypeMismatch,
			Position: pos,
			Span:     span,
			Message:  msg,
			Help:     help,
		})
		return diags
	}

	if !result.Found {
		if use == fieldUseProbe && querycore.MissingFieldReadsNil(objType) {
			return diags
		}
		pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
		span := ast.SpanOf(e)
		if e.Key != nil && e.Key.Line() > 0 {
			pos.Line = e.Key.Line()
			pos.Column = e.Key.Column()
			span = ast.SpanOf(e.Key)
		}
		msg := formatMissingField(fieldName, objType, resolver)
		_, help := diag.ContextualHelp(diag.ErrNoField, msg, "")
		diags = append(diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrNoField,
			Position: pos,
			Span:     span,
			Message:  msg,
			Help:     help,
		})
	}

	return diags
}

func indexedReadProofType(e *ast.AttrGetExpr, p cfg.Point, resolver fieldResolverImpl) bool {
	if e == nil {
		return false
	}
	keyType := resolver.TypeOf(e.Key, p)
	if keyType == nil {
		keyType = typ.Unknown
	}
	proven, ok := resolver.IndexedReadProofType(e, p, keyType)
	return ok && !typ.IsAbsentOrUnknown(proven)
}

func relationalIsEquality(e *ast.RelationalOpExpr) bool {
	if e == nil {
		return false
	}
	switch e.Operator {
	case "==", "~=":
		return true
	}
	return false
}

func checkTypeProbeArg(expr ast.Expr, p cfg.Point, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string) []diag.Diagnostic {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return checkFieldExpr(expr, p, resolver, seen, sourceName)
	}
	return checkFieldExpr(attr.Object, p, resolver, seen, sourceName)
}

func isStringKeyExpr(key ast.Expr) bool {
	if key == nil {
		return false
	}
	_, ok := key.(*ast.StringExpr)
	return ok
}

func checkIndexAccess(e *ast.AttrGetExpr, p cfg.Point, resolver fieldResolverImpl, objType typ.Type, sourceName string) []diag.Diagnostic {
	if e == nil {
		return nil
	}
	if objType == nil || objType.Kind().IsPlaceholder() {
		return nil
	}
	keyType := resolver.TypeOf(e.Key, p)
	if keyType == nil {
		// Treat unresolved key type as unknown at indexability boundary.
		// This preserves dynamic Lua semantics for computed keys while still
		// allowing container-specific checks to reject impossible accesses.
		keyType = typ.Unknown
	}

	if proven, ok := resolver.IndexedReadProofType(e, p, keyType); ok && !typ.IsAbsentOrUnknown(proven) {
		return nil
	}

	_, ok := resolver.RuntimeIndex(objType, keyType)
	if ok {
		return nil
	}

	return []diag.Diagnostic{indexError(objType, keyType, e, p, resolver, sourceName)}
}

// optionalIndexError rejects an index read on an optional container. Indexing a
// value whose type includes nil dereferences nil at runtime when the value is
// absent, so the read is unsound until the optional is narrowed non-nil (a nil
// guard such as `if m then m[k] end` or `m = m or {}`). This mirrors the
// optional-call rule (ErrOptionalCall) for the call site. It fires only when the
// non-nil inner is a keyed container (map, array, or tuple); record field access
// on an optional is reported through field resolution.
func optionalIndexError(objType typ.Type, e *ast.AttrGetExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) (diag.Diagnostic, bool) {
	if e == nil || objType == nil {
		return diag.Diagnostic{}, false
	}
	inner, ok := optionalContainerInner(objType)
	if !ok {
		return diag.Diagnostic{}, false
	}
	if !indexesKeyedContainer(inner) {
		return diag.Diagnostic{}, false
	}
	pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
	span := ast.SpanOf(e)
	if e.Object != nil && e.Object.Line() > 0 {
		pos.Line = e.Object.Line()
		pos.Column = e.Object.Column()
		span = ast.SpanOf(e.Object)
	}
	msg := "cannot index optional value " + typ.FormatShort(objType) + " without nil check"
	_, help := diag.ContextualHelp(diag.ErrOptionalIndex, msg, "")
	return diag.Diagnostic{
		Severity:    diag.SeverityError,
		Code:        diag.ErrOptionalIndex,
		Position:    pos,
		Span:        span,
		Message:     msg,
		Explanation: canonicaldiag.NewBuilder(resolver.observer).ExplainOptionalIndex(e, p, objType),
		Help:        help,
	}, true
}

// optionalContainerInner returns the non-nil inner of an optional object type.
// A plain Optional yields its Inner; a union carrying nil alongside a single
// other member yields that member.
func optionalContainerInner(t typ.Type) (typ.Type, bool) {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		if v == nil || v.Inner == nil {
			return nil, false
		}
		return v.Inner, true
	case *typ.Union:
		if v == nil {
			return nil, false
		}
		var inner typ.Type
		hasNil := false
		for _, m := range v.Members {
			if m == nil {
				continue
			}
			if m.Kind() == kind.Nil {
				hasNil = true
				continue
			}
			if inner != nil {
				return nil, false
			}
			inner = m
		}
		if !hasNil || inner == nil {
			return nil, false
		}
		return inner, true
	}
	return nil, false
}

// indexesKeyedContainer reports whether t is a keyed container whose index read
// dereferences the container at runtime, so reaching it through an optional is
// unsound (map, array, or tuple). Record field access on an optional is reported
// through field resolution / result optionality, not here.
func indexesKeyedContainer(t typ.Type) bool {
	switch unwrap.Alias(t).(type) {
	case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	}
	return false
}

func indexError(objType typ.Type, keyType typ.Type, e *ast.AttrGetExpr, p cfg.Point, resolver fieldResolverImpl, sourceName string) diag.Diagnostic {
	pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
	span := ast.SpanOf(e)
	if e.Object != nil && e.Object.Line() > 0 {
		pos.Line = e.Object.Line()
		pos.Column = e.Object.Column()
		span = ast.SpanOf(e.Object)
	}
	msg := "cannot index type " + typ.FormatShort(objType)
	_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
	return diag.Diagnostic{
		Severity:    diag.SeverityError,
		Code:        diag.ErrTypeMismatch,
		Position:    pos,
		Span:        span,
		Message:     msg,
		Explanation: canonicaldiag.NewBuilder(resolver.observer).ExplainIndexFailure(e, p, objType, keyType),
		Help:        help,
	}
}

func formatMissingField(field string, objType typ.Type, resolver fieldResolverImpl) string {
	if union, ok := objType.(*typ.Union); ok && len(union.Members) > 1 {
		var withField, withoutField []string
		for _, m := range union.Members {
			name := memberTypeName(m)
			if memberHasField(m, field, resolver) {
				withField = append(withField, name)
			} else {
				withoutField = append(withoutField, name)
			}
		}
		if len(withField) > 0 && len(withoutField) > 0 {
			return "field '" + field + "' missing on " + joinNames(withoutField) +
				" (present on " + joinNames(withField) + ")"
		}
	}
	return "field '" + field + "' does not exist on type " + typ.FormatShort(objType)
}

func memberHasField(t typ.Type, field string, resolver fieldResolverImpl) bool {
	_, ok := resolver.Field(t, field)
	return ok
}

func memberTypeName(t typ.Type) string {
	switch v := t.(type) {
	case *typ.Interface:
		if v.Name != "" {
			return v.Name
		}
	case *typ.Alias:
		return v.Name
	case *typ.Record:
		if len(v.Fields) > 0 {
			for _, f := range v.Fields {
				if f.Name == "id" || f.Name == "type" || f.Name == "name" {
					if lit, ok := f.Type.(*typ.Literal); ok {
						return "{" + f.Name + ": " + lit.String() + ", ...}"
					}
					return "{" + f.Name + ": " + typ.FormatShort(f.Type) + ", ...}"
				}
			}
			f := v.Fields[0]
			return "{" + f.Name + ": ..., ...}"
		}
		return "{}"
	case *typ.Recursive:
		if v.Name != "" {
			return v.Name
		}
	case *typ.Instantiated:
		if v.Generic != nil && v.Generic.Name != "" {
			return v.Generic.Name + "<...>"
		}
	}
	s := typ.FormatShort(t)
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	if len(names) == 2 {
		return names[0] + " and " + names[1]
	}
	result := names[0]
	for i := 1; i < len(names)-1; i++ {
		result += ", " + names[i]
	}
	return result + ", and " + names[len(names)-1]
}
