package expr

import "strconv"

// Expr represents an arithmetic expression in the constraint language.
// Expressions are immutable and can be composed freely.
type Expr interface {
	exprNode()
	String() string
	// Substitute replaces variables according to the substitution map.
	Substitute(subst map[string]Expr) Expr
	// Variables returns all variable names in this expression.
	Variables() []string
	// Eval evaluates the expression given concrete variable values.
	// Returns (value, true) if fully evaluated, (0, false) if unknown variables remain.
	Eval(env map[string]int64) (int64, bool)
}

// Const represents a constant integer value.
type Const struct {
	Value int64
}

func (Const) exprNode()        {}
func (c Const) String() string { return strconv.FormatInt(c.Value, 10) }

func (c Const) Substitute(map[string]Expr) Expr {
	return c
}

func (Const) Variables() []string {
	return nil
}

func (c Const) Eval(map[string]int64) (int64, bool) {
	return c.Value, true
}

// Constructors for ergonomic building

// V creates a variable expression.
func V(name string) Var { return Var{Name: name} }

// C creates a constant expression.
func C(val int64) Const { return Const{Value: val} }

// Add creates an addition expression.
func Add(left, right Expr) BinOp { return BinOp{Op: OpAdd, Left: left, Right: right} }

// Sub creates a subtraction expression.
func Sub(left, right Expr) BinOp { return BinOp{Op: OpSub, Left: left, Right: right} }

// Mul creates a multiplication expression.
func Mul(left, right Expr) BinOp { return BinOp{Op: OpMul, Left: left, Right: right} }

// Div creates a division expression.
func Div(left, right Expr) BinOp { return BinOp{Op: OpDiv, Left: left, Right: right} }

// Mod creates a modulo expression.
func Mod(left, right Expr) BinOp { return BinOp{Op: OpMod, Left: left, Right: right} }

// L creates a length expression.
func L(name string) Len { return Len{Of: name} }

// P creates a parameter reference expression.
func P(index int) Param { return Param{Index: index} }

// R creates a return value reference expression.
func R(index int) Ret { return Ret{Index: index} }

// PL creates a parameter length expression.
func PL(index int) ParamLen { return ParamLen{Index: index} }

// RL creates a return length expression.
func RL(index int) RetLen { return RetLen{Index: index} }

// MinExpr creates a min expression.
func MinExpr(left, right Expr) Min { return Min{Left: left, Right: right} }

// MaxExpr creates a max expression.
func MaxExpr(left, right Expr) Max { return Max{Left: left, Right: right} }

// ExprVisitor dispatches on expression variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type ExprVisitor[R any] struct {
	Var      func(Var) R
	Const    func(Const) R
	BinOp    func(BinOp) R
	Len      func(Len) R
	Param    func(Param) R
	Ret      func(Ret) R
	ParamLen func(ParamLen) R
	RetLen   func(RetLen) R
	Min      func(Min) R
	Max      func(Max) R
	Default  func(Expr) R
}

// VisitExpr applies the first matching handler in v to e.
func VisitExpr[R any](e Expr, v ExprVisitor[R]) R {
	switch ee := e.(type) {
	case Var:
		if v.Var != nil {
			return v.Var(ee)
		}
	case *Var:
		if v.Var != nil {
			return v.Var(*ee)
		}
	case Const:
		if v.Const != nil {
			return v.Const(ee)
		}
	case *Const:
		if v.Const != nil {
			return v.Const(*ee)
		}
	case BinOp:
		if v.BinOp != nil {
			return v.BinOp(ee)
		}
	case *BinOp:
		if v.BinOp != nil {
			return v.BinOp(*ee)
		}
	case Len:
		if v.Len != nil {
			return v.Len(ee)
		}
	case *Len:
		if v.Len != nil {
			return v.Len(*ee)
		}
	case Param:
		if v.Param != nil {
			return v.Param(ee)
		}
	case *Param:
		if v.Param != nil {
			return v.Param(*ee)
		}
	case Ret:
		if v.Ret != nil {
			return v.Ret(ee)
		}
	case *Ret:
		if v.Ret != nil {
			return v.Ret(*ee)
		}
	case ParamLen:
		if v.ParamLen != nil {
			return v.ParamLen(ee)
		}
	case *ParamLen:
		if v.ParamLen != nil {
			return v.ParamLen(*ee)
		}
	case RetLen:
		if v.RetLen != nil {
			return v.RetLen(ee)
		}
	case *RetLen:
		if v.RetLen != nil {
			return v.RetLen(*ee)
		}
	case Min:
		if v.Min != nil {
			return v.Min(ee)
		}
	case *Min:
		if v.Min != nil {
			return v.Min(*ee)
		}
	case Max:
		if v.Max != nil {
			return v.Max(ee)
		}
	case *Max:
		if v.Max != nil {
			return v.Max(*ee)
		}
	}
	if v.Default != nil {
		return v.Default(e)
	}
	var zero R
	return zero
}
