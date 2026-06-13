package sourceprovenance

import "github.com/wippyai/go-lua/compiler/ast"

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
		return producer.Call.AdjustRet
	case ProducerVararg:
		return producer.Vararg.AdjustRet
	default:
		return false
	}
}

func AssertionInner(expr ast.Expr) ast.Expr {
	for {
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			expr = wrapped.Expr
		default:
			return expr
		}
	}
}
