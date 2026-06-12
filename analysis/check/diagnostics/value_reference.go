package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/compiler/ast"
)

// UnresolvedValueReferences reports value reads that the binder had to bind as
// implicit globals. Predeclared globals and global assignment targets are left
// to binding policy and are not reported here.
type UnresolvedValueReferences Config

func (p UnresolvedValueReferences) Produce(result *check.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var out []diagnostic.Diagnostic
	seen := make(map[*ast.IdentExpr]struct{})
	resolver := p.Resolver
	emitExpr := func(expr ast.Expr) {
		walkValueExpr(expr, resolver, func(ident *ast.IdentExpr) {
			if ident == nil {
				return
			}
			if _, ok := seen[ident]; ok {
				return
			}
			seen[ident] = struct{}{}
			if !result.IsImplicitGlobalUse(ident) {
				return
			}
			if isAmbientLuaGlobal(ident.Value) {
				return
			}
			if resolvesTypeName(ident.Value, resolver) {
				return
			}
			out = append(out, unresolvedValueDiagnostic(ident))
		})
	}
	emitExprs := func(exprs []ast.Expr) {
		for _, expr := range exprs {
			emitExpr(expr)
		}
	}

	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			emitExpr(fact.Expr)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			emitAssignmentTargetReads(fact.Target, emitExpr)
			emitExpr(fact.Value)
		}
		if fact, ok := result.Call(point); ok {
			emitExpr(fact.Call)
		}
		if fact, ok := result.ReturnFact(point); ok {
			emitExprs(fact.Exprs)
		}
		if fact, ok := result.BranchCondition(point); ok {
			emitExpr(fact.Condition)
		}
	}
	return out
}

func emitAssignmentTargetReads(target ast.Expr, emitExpr func(ast.Expr)) {
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		emitExpr(t.Object)
		if t.KeySyntax == ast.AttrKeyIndex {
			emitExpr(t.Key)
		}
	case *ast.CastExpr:
		emitAssignmentTargetReads(t.Expr, emitExpr)
	case *ast.NonNilAssertExpr:
		emitAssignmentTargetReads(t.Expr, emitExpr)
	}
}

func walkValueExpr(expr ast.Expr, resolver typeannotation.Resolver, visit func(*ast.IdentExpr)) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		visit(e)
	case *ast.AttrGetExpr:
		walkValueExpr(e.Object, resolver, visit)
		if e.KeySyntax == ast.AttrKeyIndex {
			walkValueExpr(e.Key, resolver, visit)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				walkValueExpr(field.Key, resolver, visit)
			}
			walkValueExpr(field.Value, resolver, visit)
		}
	case *ast.FuncCallExpr:
		if !typeSyntaxCallee(e, resolver) {
			walkValueExpr(e.Func, resolver, visit)
		}
		if !typeSyntaxReceiver(e, resolver) {
			walkValueExpr(e.Receiver, resolver, visit)
		}
		for _, arg := range e.Args {
			walkValueExpr(arg, resolver, visit)
		}
	case *ast.LogicalOpExpr:
		walkValueExpr(e.Lhs, resolver, visit)
		walkValueExpr(e.Rhs, resolver, visit)
	case *ast.RelationalOpExpr:
		walkValueExpr(e.Lhs, resolver, visit)
		walkValueExpr(e.Rhs, resolver, visit)
	case *ast.StringConcatOpExpr:
		walkValueExpr(e.Lhs, resolver, visit)
		walkValueExpr(e.Rhs, resolver, visit)
	case *ast.ArithmeticOpExpr:
		walkValueExpr(e.Lhs, resolver, visit)
		walkValueExpr(e.Rhs, resolver, visit)
	case *ast.UnaryMinusOpExpr:
		walkValueExpr(e.Expr, resolver, visit)
	case *ast.UnaryNotOpExpr:
		walkValueExpr(e.Expr, resolver, visit)
	case *ast.UnaryLenOpExpr:
		walkValueExpr(e.Expr, resolver, visit)
	case *ast.UnaryBNotOpExpr:
		walkValueExpr(e.Expr, resolver, visit)
	case *ast.CastExpr:
		walkValueExpr(e.Expr, resolver, visit)
	case *ast.NonNilAssertExpr:
		walkValueExpr(e.Expr, resolver, visit)
	}
}

func typeSyntaxCallee(call *ast.FuncCallExpr, resolver typeannotation.Resolver) bool {
	if call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	return ok && resolvesTypeName(ident.Value, resolver)
}

func typeSyntaxReceiver(call *ast.FuncCallExpr, resolver typeannotation.Resolver) bool {
	if call == nil || call.Method == "" || len(call.TypeArgs) != 0 {
		return false
	}
	ident, ok := call.Receiver.(*ast.IdentExpr)
	return ok && resolvesTypeName(ident.Value, resolver)
}

func resolvesTypeName(name string, resolver typeannotation.Resolver) bool {
	if name == "" {
		return false
	}
	if isBuiltinPrimitiveTypeName(name) {
		return true
	}
	switch name {
	case "int", "bool":
		return true
	}
	if resolver == nil {
		return false
	}
	if resultResolver, ok := resolver.(*resultResolver); ok && resultResolver.hasKnownTypeName(name) {
		return true
	}
	_, ok := resolver.ResolveTypeRef([]string{name})
	return ok
}

func isAmbientLuaGlobal(name string) bool {
	switch name {
	case "_G", "_GOPHER_LUA_VERSION", "_VERSION",
		"assert", "coroutine", "debug", "error", "errors",
		"getmetatable", "ipairs", "math", "next", "package",
		"pairs", "pcall", "print", "rawequal", "rawget", "rawset",
		"select", "setmetatable", "string", "table", "tonumber",
		"tostring", "type", "unpack", "utf8", "xpcall":
		return true
	default:
		return false
	}
}

func unresolvedValueDiagnostic(ident *ast.IdentExpr) diagnostic.Diagnostic {
	span := ast.SpanOf(ident)
	name := "<missing>"
	if ident != nil && ident.Value != "" {
		name = ident.Value
	}
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeUnresolvedValueReference,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("unknown value %s", name),
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: fmt.Sprintf("value reference %s is not declared or predeclared here", name),
		}),
		Labels: []diagnostic.Label{{Span: span, Message: "unresolved value"}},
	}
}
