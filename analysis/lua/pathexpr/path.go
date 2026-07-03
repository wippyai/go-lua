// Package pathexpr resolves Lua AST expressions into analysis access paths.
package pathexpr

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Resolve extracts a static access path from expr using lexical binding data.
//
// Supported forms are identifiers, dot fields, string indexes, integer indexes,
// and nested combinations of those forms. Dynamic indexes are rejected.
func Resolve(expr ast.Expr, bindings *bind.Result) (path.Path, bool) {
	return ViewOf(expr, bindings).SyntaxPath()
}

// View carries the path identities available for a source expression. Different
// consumers need different identities: branch and assignment logic asks what
// syntax was written, while call-boundary evidence asks which runtime location
// the expression aliases after proof-preserving wrappers.
type View struct {
	expr     ast.Expr
	bindings *bind.Result
}

func ViewOf(expr ast.Expr, bindings *bind.Result) View {
	return View{expr: expr, bindings: bindings}
}

// SyntaxPath extracts the path explicitly written in source syntax. It does not
// cross assertion or cast wrappers, so guards like `(x :: T) ~= nil` do not
// silently become proofs about x.
func (v View) SyntaxPath() (path.Path, bool) {
	return resolveSyntaxPath(v.expr, v.bindings)
}

// AliasPath extracts the runtime location aliased by this expression. It
// unwraps non-nil assertions and non-any casts because those wrappers do not
// allocate a new value; a direct any cast remains a proof boundary.
func (v View) AliasPath() (path.Path, bool) {
	inner, ok := sourceprovenance.ProofInner(v.expr)
	if !ok {
		return path.Path{}, false
	}
	return resolveSyntaxPath(inner, v.bindings)
}

func resolveSyntaxPath(expr ast.Expr, bindings *bind.Result) (path.Path, bool) {
	switch expr := expr.(type) {
	case *ast.IdentExpr:
		return resolveIdent(expr, bindings)
	case *ast.AttrGetExpr:
		return resolveAttr(expr, bindings)
	default:
		return path.Path{}, false
	}
}

// ResolveAlias extracts the access path aliased by expr's runtime value. Unlike
// Resolve, it unwraps non-nil assertions and non-any casts, so call-boundary
// evidence can bind `f(x :: Wider)` to x. A direct cast to any is a deliberate
// proof boundary and is rejected by sourceprovenance.ProofInner.
func ResolveAlias(expr ast.Expr, bindings *bind.Result) (path.Path, bool) {
	return ViewOf(expr, bindings).AliasPath()
}

// ResolveContainer extracts the receiver/container path for an attribute/index
// expression. The full expression may still be unresolvable when its key is
// dynamic.
// ResolveMutationContainer extracts the nearest statically known table
// ancestor for an assignment target. Static member writes resolve exactly
// through Resolve; this helper is for unresolved targets such as t[k].x where
// the mutation must still invalidate descendants of t.
func ResolveMutationContainer(expr ast.Expr, bindings *bind.Result) (path.Path, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return path.Path{}, false
	}
	return resolveMutationContainer(attr, bindings)
}

func resolveMutationContainer(expr *ast.AttrGetExpr, bindings *bind.Result) (path.Path, bool) {
	if expr == nil {
		return path.Path{}, false
	}
	if p, ok := Resolve(expr.Object, bindings); ok {
		return p, true
	}
	parent, ok := expr.Object.(*ast.AttrGetExpr)
	if !ok {
		return path.Path{}, false
	}
	return resolveMutationContainer(parent, bindings)
}

// ResolveFuncName extracts the static assignment target for a function
// definition name. Method definitions target receiver.method; self parameter
// semantics are handled by binding, not by path resolution.
func ResolveFuncName(name *ast.FuncName, bindings *bind.Result) (path.Path, bool) {
	if name == nil {
		return path.Path{}, false
	}
	if name.Method != "" {
		receiver, ok := Resolve(name.Receiver, bindings)
		if !ok || receiver.IsEmpty() {
			return path.Path{}, false
		}
		return receiver.Field(name.Method), true
	}
	if name.Receiver != nil {
		return path.Path{}, false
	}
	return Resolve(name.Func, bindings)
}

func resolveIdent(expr *ast.IdentExpr, bindings *bind.Result) (path.Path, bool) {
	if expr == nil || bindings == nil {
		return path.Path{}, false
	}
	id, ok := bindings.SymbolOf(expr)
	if !ok || id == 0 {
		return path.Path{}, false
	}
	name := bindings.Name(id)
	if name == "" {
		name = expr.Value
	}
	return path.NewPath(id, name), true
}

func resolveAttr(expr *ast.AttrGetExpr, bindings *bind.Result) (path.Path, bool) {
	if expr == nil {
		return path.Path{}, false
	}
	base, ok := Resolve(expr.Object, bindings)
	if !ok {
		return path.Path{}, false
	}

	switch key := expr.Key.(type) {
	case *ast.StringExpr:
		switch expr.KeySyntax {
		case ast.AttrKeyDot:
			if key.Value == "" {
				return path.Path{}, false
			}
			return base.Field(key.Value), true
		case ast.AttrKeyIndex:
			return base.IndexStr(key.Value), true
		default:
			if isIdentName(key.Value) {
				return base.Field(key.Value), true
			}
			return base.IndexStr(key.Value), true
		}
	case *ast.NumberExpr:
		index, ok := parseNonNegativeDecimalInt(key.Value)
		if !ok {
			return path.Path{}, false
		}
		return base.IndexInt(index), true
	default:
		return path.Path{}, false
	}
}

func parseNonNegativeDecimalInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	value := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		digit := int(ch - '0')
		if value > (maxInt-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	return value, true
}

func isIdentName(s string) bool {
	if s == "" {
		return false
	}
	if !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentContinue(s[i]) {
			return false
		}
	}
	return true
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentContinue(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}
