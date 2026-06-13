package manifest

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
)

func encodeExpr(e expr.Expr) (*exprWire, error) {
	if e == nil {
		return nil, nil
	}
	switch ex := e.(type) {
	case expr.Var:
		return &exprWire{Kind: "var", Name: ex.Name}, nil
	case *expr.Var:
		return encodeExpr(*ex)
	case expr.Const:
		return &exprWire{Kind: "const", Value: ex.Value}, nil
	case *expr.Const:
		return encodeExpr(*ex)
	case expr.BinOp:
		left, err := encodeExpr(ex.Left)
		if err != nil {
			return nil, err
		}
		right, err := encodeExpr(ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "binop", Op: ex.Op.String(), Left: left, Right: right}, nil
	case *expr.BinOp:
		return encodeExpr(*ex)
	case expr.Len:
		return &exprWire{Kind: "len", Name: ex.Of}, nil
	case *expr.Len:
		return encodeExpr(*ex)
	case expr.Param:
		return &exprWire{Kind: "param", Index: ex.Index}, nil
	case *expr.Param:
		return encodeExpr(*ex)
	case expr.Ret:
		return &exprWire{Kind: "ret", Index: ex.Index}, nil
	case *expr.Ret:
		return encodeExpr(*ex)
	case expr.ParamLen:
		return &exprWire{Kind: "paramLen", Index: ex.Index}, nil
	case *expr.ParamLen:
		return encodeExpr(*ex)
	case expr.RetLen:
		return &exprWire{Kind: "retLen", Index: ex.Index}, nil
	case *expr.RetLen:
		return encodeExpr(*ex)
	case expr.Min:
		left, err := encodeExpr(ex.Left)
		if err != nil {
			return nil, err
		}
		right, err := encodeExpr(ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "min", Left: left, Right: right}, nil
	case *expr.Min:
		return encodeExpr(*ex)
	case expr.Max:
		left, err := encodeExpr(ex.Left)
		if err != nil {
			return nil, err
		}
		right, err := encodeExpr(ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "max", Left: left, Right: right}, nil
	case *expr.Max:
		return encodeExpr(*ex)
	default:
		return nil, fmt.Errorf("manifest: unsupported constraint expr %T", e)
	}
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
		left, err := decodeExpr(w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeExpr(w.Right)
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
		left, err := decodeExpr(w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeExpr(w.Right)
		if err != nil {
			return nil, err
		}
		return expr.Min{Left: left, Right: right}, nil
	case "max":
		left, err := decodeExpr(w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeExpr(w.Right)
		if err != nil {
			return nil, err
		}
		return expr.Max{Left: left, Right: right}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown constraint expr kind %q", w.Kind)
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
