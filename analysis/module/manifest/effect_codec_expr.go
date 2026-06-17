package manifest

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
)

func encodeExpr(e expr.Expr) (*exprWire, error) {
	var err error
	e, err = exprValueForManifest(e)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, nil
	}
	switch ex := e.(type) {
	case expr.Var:
		return &exprWire{Kind: "var", Name: ex.Name}, nil
	case expr.Const:
		return &exprWire{Kind: "const", Value: ex.Value}, nil
	case expr.BinOp:
		op, err := encodeExprOp(ex.Op)
		if err != nil {
			return nil, err
		}
		left, right, err := encodeRequiredExprPair("binop", ex.Left, ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "binop", Op: op, Left: left, Right: right}, nil
	case expr.Len:
		return &exprWire{Kind: "len", Name: ex.Of}, nil
	case expr.Param:
		return &exprWire{Kind: "param", Index: ex.Index}, nil
	case expr.Ret:
		return &exprWire{Kind: "ret", Index: ex.Index}, nil
	case expr.ParamLen:
		return &exprWire{Kind: "paramLen", Index: ex.Index}, nil
	case expr.RetLen:
		return &exprWire{Kind: "retLen", Index: ex.Index}, nil
	case expr.Min:
		left, right, err := encodeRequiredExprPair("min", ex.Left, ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "min", Left: left, Right: right}, nil
	case expr.Max:
		left, right, err := encodeRequiredExprPair("max", ex.Left, ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "max", Left: left, Right: right}, nil
	default:
		return nil, fmt.Errorf("manifest: unsupported constraint expr %T", e)
	}
}

func exprValueForManifest(e expr.Expr) (expr.Expr, error) {
	if e == nil {
		return nil, nil
	}
	value := reflect.ValueOf(e)
	if value.Kind() != reflect.Pointer {
		return e, nil
	}
	if value.IsNil() {
		return nil, fmt.Errorf("manifest: nil constraint expr %T", e)
	}
	if normalized, ok := value.Elem().Interface().(expr.Expr); ok {
		return normalized, nil
	}
	return e, nil
}

func encodeRequiredExprPair(parent string, leftExpr, rightExpr expr.Expr) (*exprWire, *exprWire, error) {
	left, err := encodeRequiredExpr(parent, "left", leftExpr)
	if err != nil {
		return nil, nil, err
	}
	right, err := encodeRequiredExpr(parent, "right", rightExpr)
	if err != nil {
		return nil, nil, err
	}
	return left, right, nil
}

func encodeRequiredExpr(parent, side string, e expr.Expr) (*exprWire, error) {
	wire, err := encodeExpr(e)
	if err != nil {
		return nil, err
	}
	if wire == nil {
		return nil, fmt.Errorf("manifest: %s %s constraint expr is required", parent, side)
	}
	return wire, nil
}

func decodeExpr(w *exprWire) (expr.Expr, error) {
	if w == nil {
		return nil, nil
	}
	switch w.Kind {
	case "var":
		return expr.Var{Name: w.Name}, nil
	case "const":
		return expr.Const{Value: w.Value}, nil
	case "binop":
		op, err := decodeExprOp(w.Op)
		if err != nil {
			return nil, err
		}
		left, err := decodeRequiredExpr("binop", "left", w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeRequiredExpr("binop", "right", w.Right)
		if err != nil {
			return nil, err
		}
		return expr.BinOp{Op: op, Left: left, Right: right}, nil
	case "len":
		return expr.Len{Of: w.Name}, nil
	case "param":
		return expr.Param{Index: w.Index}, nil
	case "ret":
		return expr.Ret{Index: w.Index}, nil
	case "paramLen":
		return expr.ParamLen{Index: w.Index}, nil
	case "retLen":
		return expr.RetLen{Index: w.Index}, nil
	case "min":
		left, err := decodeRequiredExpr("min", "left", w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeRequiredExpr("min", "right", w.Right)
		if err != nil {
			return nil, err
		}
		return expr.Min{Left: left, Right: right}, nil
	case "max":
		left, err := decodeRequiredExpr("max", "left", w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeRequiredExpr("max", "right", w.Right)
		if err != nil {
			return nil, err
		}
		return expr.Max{Left: left, Right: right}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown constraint expr kind %q", w.Kind)
	}
}

func decodeRequiredExpr(parent, side string, w *exprWire) (expr.Expr, error) {
	if w == nil {
		return nil, fmt.Errorf("manifest: %s %s constraint expr is required", parent, side)
	}
	return decodeExpr(w)
}

func encodeExprOp(op expr.Op) (string, error) {
	switch op {
	case expr.OpAdd, expr.OpSub, expr.OpMul, expr.OpDiv, expr.OpMod:
		return op.String(), nil
	default:
		return "", fmt.Errorf("manifest: unsupported expr op %d", op)
	}
}

func decodeExprOp(op string) (expr.Op, error) {
	switch op {
	case "+":
		return expr.OpAdd, nil
	case "-":
		return expr.OpSub, nil
	case "*":
		return expr.OpMul, nil
	case "/":
		return expr.OpDiv, nil
	case "%":
		return expr.OpMod, nil
	default:
		return 0, fmt.Errorf("manifest: unknown expr op %q", op)
	}
}
