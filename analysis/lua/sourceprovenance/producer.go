package sourceprovenance

import (
	"github.com/wippyai/go-lua/analysis/lua/castsem"
	"github.com/wippyai/go-lua/compiler/ast"
)

type ProducerKind uint8

const (
	ProducerNone ProducerKind = iota
	ProducerCall
	ProducerVararg
)

type Producer struct {
	Kind   ProducerKind
	Expr   ast.Expr
	Call   *ast.FuncCallExpr
	Vararg *ast.Comma3Expr
}

func TopLevelProducer(expr ast.Expr) Producer {
	inner := AssertionInner(expr)
	if exprNil(inner) {
		return Producer{}
	}
	switch inner := inner.(type) {
	case *ast.FuncCallExpr:
		return Producer{Kind: ProducerCall, Expr: inner, Call: inner}
	case *ast.Comma3Expr:
		return Producer{Kind: ProducerVararg, Expr: inner, Vararg: inner}
	default:
		return Producer{Kind: ProducerNone, Expr: inner}
	}
}

func Call(expr ast.Expr) (*ast.FuncCallExpr, bool) {
	producer := TopLevelProducer(expr)
	if producer.Kind != ProducerCall || producer.Call == nil {
		return nil, false
	}
	return producer.Call, true
}

func CanProduceMultipleValues(expr ast.Expr) bool {
	switch TopLevelProducer(expr).Kind {
	case ProducerCall, ProducerVararg:
		return true
	default:
		return false
	}
}

func AdjustRet(expr ast.Expr) bool {
	switch producer := TopLevelProducer(expr); producer.Kind {
	case ProducerCall:
		return producer.Call != nil && producer.Call.AdjustRet
	case ProducerVararg:
		return producer.Vararg != nil && producer.Vararg.AdjustRet
	default:
		return false
	}
}

func AssertionInner(expr ast.Expr) ast.Expr {
	for {
		if exprNil(expr) {
			return nil
		}
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			if wrapped == nil {
				return nil
			}
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			if wrapped == nil {
				return nil
			}
			expr = wrapped.Expr
		default:
			return expr
		}
	}
}

func missingAssertionInner(expr ast.Expr) bool {
	if exprNil(expr) {
		return true
	}
	if cast, ok := expr.(*ast.CastExpr); ok && cast != nil && exprNil(cast.Expr) {
		return false
	}
	if !assertionWrapper(expr) {
		return false
	}
	return exprNil(AssertionInner(expr))
}

func ProofInner(expr ast.Expr) (ast.Expr, bool) {
	for {
		if exprNil(expr) {
			return nil, true
		}
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			if wrapped == nil {
				return nil, true
			}
			if castTargetIsProofBoundary(wrapped.Type) {
				return expr, false
			}
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			if wrapped == nil {
				return nil, true
			}
			expr = wrapped.Expr
		default:
			return expr, true
		}
	}
}

func ProofInnerIsFunction(expr ast.Expr) bool {
	inner, ok := ProofInner(expr)
	if !ok || exprNil(inner) {
		return false
	}
	_, ok = inner.(*ast.FunctionExpr)
	return ok
}

// ProofIdent returns the identifier reached through proof-transparent wrappers.
// It stops at proof boundaries such as an any/unknown cast, matching ProofInner.
func ProofIdent(expr ast.Expr) (*ast.IdentExpr, bool) {
	inner, ok := ProofInner(expr)
	if !ok || exprNil(inner) {
		return nil, false
	}
	ident, ok := inner.(*ast.IdentExpr)
	return ident, ok && ident != nil
}

// ConcreteRuntimeCastSource reports whether source is an expression wrapped in
// a concrete runtime validation cast. Top-like casts such as `:: any` are
// precision boundaries, not validation.
func ConcreteRuntimeCastSource(source ASTSource) bool {
	if source.Kind != SourceExpression || source.Expr == nil {
		return false
	}
	expr := source.Expr
	for {
		switch wrapped := expr.(type) {
		case *ast.NonNilAssertExpr:
			if wrapped == nil {
				return false
			}
			expr = wrapped.Expr
		case *ast.CastExpr:
			if wrapped == nil || wrapped.Type == nil {
				return false
			}
			if wrapped.Syntax != ast.CastSyntaxAs && wrapped.Syntax != ast.CastSyntaxColonColon {
				return false
			}
			if castTargetIsProofBoundary(wrapped.Type) {
				return false
			}
			return true
		default:
			return false
		}
	}
}

func castTargetIsProofBoundary(t ast.TypeExpr) bool {
	primitive, ok := t.(*ast.PrimitiveTypeExpr)
	return ok && castsem.IsTopLikeTarget(primitive.Name)
}

func assertionWrapper(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.CastExpr, *ast.NonNilAssertExpr:
		return true
	default:
		return false
	}
}
