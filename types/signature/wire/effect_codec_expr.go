package wire

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/constraint/expr"
)

// The constraint grammar is the expr package's, and its members are answered by
// that package's dispatch. What this package owns is the boundary spelling of
// those members: the kind string written for each form, the token written for
// each operator, and which exprWire fields a kind carries. That spelling lives
// in the two tables below and nowhere else, so the write side and the read side
// consult one statement instead of two hand-kept switches.

// exprWirePayload names the exprWire fields a kind carries. It is the field
// applicability rule of the wire struct, stated once beside the vocabulary:
// encode writes exactly the named fields and decode reads exactly them, so
// neither side can quietly carry a field the other ignores.
type exprWirePayload uint8

const (
	exprPayloadName exprWirePayload = iota
	exprPayloadValue
	exprPayloadIndex
	exprPayloadOperands
	exprPayloadOperatorOperands
)

// exprWireForm is one grammar form's boundary spelling: the kind written for
// it, the wire fields that kind carries, and the term rebuilt from them.
type exprWireForm struct {
	kind    string
	payload exprWirePayload
	build   func(exprTermFields) expr.Expr
}

// exprTermFields is a grammar term flattened to the fields the wire carries. It
// is the one shape both directions move through: the write side fills it from
// the grammar's dispatch, the read side fills it from the wire.
type exprTermFields struct {
	name     string
	value    int64
	index    int
	operator expr.Op
	left     expr.Expr
	right    expr.Expr
}

// exprWireForms is the boundary vocabulary, one row per grammar form, indexed
// by the form's own ordinal.
var exprWireForms = [expr.FormCount + 1]exprWireForm{
	expr.FormVar: {
		kind:    "var",
		payload: exprPayloadName,
		build:   func(f exprTermFields) expr.Expr { return expr.Var{Name: f.name} },
	},
	expr.FormConst: {
		kind:    "const",
		payload: exprPayloadValue,
		build:   func(f exprTermFields) expr.Expr { return expr.Const{Value: f.value} },
	},
	expr.FormBinOp: {
		kind:    "binop",
		payload: exprPayloadOperatorOperands,
		build: func(f exprTermFields) expr.Expr {
			return expr.BinOp{Op: f.operator, Left: f.left, Right: f.right}
		},
	},
	expr.FormLen: {
		kind:    "len",
		payload: exprPayloadName,
		build:   func(f exprTermFields) expr.Expr { return expr.Len{Of: f.name} },
	},
	expr.FormParam: {
		kind:    "param",
		payload: exprPayloadIndex,
		build:   func(f exprTermFields) expr.Expr { return expr.Param{Index: f.index} },
	},
	expr.FormRet: {
		kind:    "ret",
		payload: exprPayloadIndex,
		build:   func(f exprTermFields) expr.Expr { return expr.Ret{Index: f.index} },
	},
	expr.FormParamLen: {
		kind:    "paramLen",
		payload: exprPayloadIndex,
		build:   func(f exprTermFields) expr.Expr { return expr.ParamLen{Index: f.index} },
	},
	expr.FormRetLen: {
		kind:    "retLen",
		payload: exprPayloadIndex,
		build:   func(f exprTermFields) expr.Expr { return expr.RetLen{Index: f.index} },
	},
	expr.FormMin: {
		kind:    "min",
		payload: exprPayloadOperands,
		build:   func(f exprTermFields) expr.Expr { return expr.Min{Left: f.left, Right: f.right} },
	},
	expr.FormMax: {
		kind:    "max",
		payload: exprPayloadOperands,
		build:   func(f exprTermFields) expr.Expr { return expr.Max{Left: f.left, Right: f.right} },
	},
}

// exprWireFormsByKind is the read side's index into the same rows, so a kind
// the vocabulary does not spell is unknown to the boundary by construction.
var exprWireFormsByKind = func() map[string]expr.Form {
	byKind := make(map[string]expr.Form, expr.FormCount)
	for _, form := range expr.Forms() {
		row := exprWireForms[form]
		if row.kind == "" {
			continue
		}
		byKind[row.kind] = form
	}
	return byKind
}()

// exprWireOperators is the boundary spelling of the grammar's operators. The
// tokens are the codec's serialization commitment, so they are written here and
// not read off the operator's display form.
var exprWireOperators = [...]struct {
	operator expr.Op
	token    string
}{
	{operator: expr.OpAdd, token: "+"},
	{operator: expr.OpSub, token: "-"},
	{operator: expr.OpMul, token: "*"},
	{operator: expr.OpDiv, token: "/"},
	{operator: expr.OpMod, token: "%"},
}

func encodeExpr(e expr.Expr) (*exprWire, error) {
	if e == nil {
		return nil, nil
	}
	form := expr.FormOf(e)
	if !form.Valid() {
		return nil, fmt.Errorf("signature/wire: unsupported or nil constraint expr %T", e)
	}
	row := exprWireForms[form]
	if row.kind == "" {
		return nil, fmt.Errorf("signature/wire: unsupported constraint expr %T", e)
	}
	fields := exprTermFieldsOf(e)
	wire := &exprWire{Kind: row.kind}
	switch row.payload {
	case exprPayloadName:
		wire.Name = fields.name
	case exprPayloadValue:
		wire.Value = fields.value
	case exprPayloadIndex:
		wire.Index = encodeInt(fields.index)
	case exprPayloadOperatorOperands:
		operator, err := encodeExprOp(fields.operator)
		if err != nil {
			return nil, err
		}
		wire.Op = operator
		left, right, err := encodeRequiredExprPair(row.kind, fields.left, fields.right)
		if err != nil {
			return nil, err
		}
		wire.Left, wire.Right = left, right
	case exprPayloadOperands:
		left, right, err := encodeRequiredExprPair(row.kind, fields.left, fields.right)
		if err != nil {
			return nil, err
		}
		wire.Left, wire.Right = left, right
	default:
		return nil, fmt.Errorf("signature/wire: constraint expr kind %q carries no stated wire payload", row.kind)
	}
	return wire, nil
}

// exprTermFieldsOf reads a term's fields through the grammar's own dispatch, so
// a pointer term and a value term are the same term here and neither the write
// side nor this codec restates the member list to tell them apart.
func exprTermFieldsOf(e expr.Expr) exprTermFields {
	return expr.VisitExpr(e, expr.ExprVisitor[exprTermFields]{
		Var:      func(t expr.Var) exprTermFields { return exprTermFields{name: t.Name} },
		Const:    func(t expr.Const) exprTermFields { return exprTermFields{value: t.Value} },
		BinOp:    func(t expr.BinOp) exprTermFields { return exprTermFields{operator: t.Op, left: t.Left, right: t.Right} },
		Len:      func(t expr.Len) exprTermFields { return exprTermFields{name: t.Of} },
		Param:    func(t expr.Param) exprTermFields { return exprTermFields{index: t.Index} },
		Ret:      func(t expr.Ret) exprTermFields { return exprTermFields{index: t.Index} },
		ParamLen: func(t expr.ParamLen) exprTermFields { return exprTermFields{index: t.Index} },
		RetLen:   func(t expr.RetLen) exprTermFields { return exprTermFields{index: t.Index} },
		Min:      func(t expr.Min) exprTermFields { return exprTermFields{left: t.Left, right: t.Right} },
		Max:      func(t expr.Max) exprTermFields { return exprTermFields{left: t.Left, right: t.Right} },
		Default:  func(expr.Expr) exprTermFields { return exprTermFields{} },
	})
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
		return nil, fmt.Errorf("signature/wire: %s %s constraint expr is required", parent, side)
	}
	return wire, nil
}

func decodeExpr(w *exprWire) (expr.Expr, error) {
	if w == nil {
		return nil, nil
	}
	form, known := exprWireFormsByKind[w.Kind]
	if !known {
		return nil, fmt.Errorf("signature/wire: unknown constraint expr kind %q", w.Kind)
	}
	row := exprWireForms[form]
	var fields exprTermFields
	switch row.payload {
	case exprPayloadName:
		fields.name = w.Name
	case exprPayloadValue:
		fields.value = w.Value
	case exprPayloadIndex:
		index, err := decodeRequiredInt(w.Index, row.kind+" index missing")
		if err != nil {
			return nil, err
		}
		fields.index = index
	case exprPayloadOperatorOperands:
		operator, err := decodeExprOp(w.Op)
		if err != nil {
			return nil, err
		}
		fields.operator = operator
		left, right, err := decodeRequiredExprPair(row.kind, w.Left, w.Right)
		if err != nil {
			return nil, err
		}
		fields.left, fields.right = left, right
	case exprPayloadOperands:
		left, right, err := decodeRequiredExprPair(row.kind, w.Left, w.Right)
		if err != nil {
			return nil, err
		}
		fields.left, fields.right = left, right
	default:
		return nil, fmt.Errorf("signature/wire: constraint expr kind %q carries no stated wire payload", row.kind)
	}
	return row.build(fields), nil
}

func decodeRequiredExprPair(parent string, leftWire, rightWire *exprWire) (expr.Expr, expr.Expr, error) {
	left, err := decodeRequiredExpr(parent, "left", leftWire)
	if err != nil {
		return nil, nil, err
	}
	right, err := decodeRequiredExpr(parent, "right", rightWire)
	if err != nil {
		return nil, nil, err
	}
	return left, right, nil
}

func decodeRequiredExpr(parent, side string, w *exprWire) (expr.Expr, error) {
	if w == nil {
		return nil, fmt.Errorf("signature/wire: %s %s constraint expr is required", parent, side)
	}
	return decodeExpr(w)
}

func encodeExprOp(op expr.Op) (string, error) {
	for _, spelling := range exprWireOperators {
		if spelling.operator == op {
			return spelling.token, nil
		}
	}
	return "", fmt.Errorf("signature/wire: unsupported expr op %d", op)
}

func decodeExprOp(token string) (expr.Op, error) {
	for _, spelling := range exprWireOperators {
		if spelling.token == token {
			return spelling.operator, nil
		}
	}
	return 0, fmt.Errorf("signature/wire: unknown expr op %q", token)
}
