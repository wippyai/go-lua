package expr

import (
	"fmt"
	"strconv"
)

// Var represents a symbolic variable (e.g., "i", "len_arr").
type Var struct {
	Name string
}

func (Var) exprNode()        {}
func (v Var) String() string { return v.Name }

func (v Var) Substitute(subst map[string]Expr) Expr {
	if e, ok := subst[v.Name]; ok {
		return e
	}

	return v
}

func (v Var) Variables() []string {
	return []string{v.Name}
}

func (v Var) Eval(env map[string]int64) (int64, bool) {
	if val, ok := env[v.Name]; ok {
		return val, true
	}

	return 0, false
}

// Len represents the length of a symbolic array/tuple.
type Len struct {
	Of string // variable name of the array
}

func (Len) exprNode()        {}
func (l Len) String() string { return fmt.Sprintf("len(%s)", l.Of) }

func (l Len) Substitute(subst map[string]Expr) Expr {
	// Check if we have a substitution for this length (stored as "name.len")
	if e, ok := subst[l.Of+".len"]; ok {
		return e
	}

	return l
}

func (l Len) Variables() []string {
	return []string{l.Of + ".len"}
}

func (l Len) Eval(env map[string]int64) (int64, bool) {
	if val, ok := env[l.Of+".len"]; ok {
		return val, true
	}

	return 0, false
}

// Param represents a reference to a function parameter by index.
// Used in function refinements: len(Param(0)) = 5
type Param struct {
	Index int // 0-based parameter index
}

func (Param) exprNode()        {}
func (p Param) String() string { return paramExprKey(p.Index) }

func (p Param) Substitute(subst map[string]Expr) Expr {
	key := paramExprKey(p.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return p
}

func (p Param) Variables() []string {
	return []string{paramExprKey(p.Index)}
}

func (p Param) Eval(env map[string]int64) (int64, bool) {
	key := paramExprKey(p.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// Ret represents a reference to a function return value by index.
// Used in function refinements: len(Ret(0)) = len(Param(0))
type Ret struct {
	Index int // 0-based return value index
}

func (Ret) exprNode()        {}
func (r Ret) String() string { return retExprKey(r.Index) }

func (r Ret) Substitute(subst map[string]Expr) Expr {
	key := retExprKey(r.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return r
}

func (r Ret) Variables() []string {
	return []string{retExprKey(r.Index)}
}

func (r Ret) Eval(env map[string]int64) (int64, bool) {
	key := retExprKey(r.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// ParamLen represents the length of a function parameter.
// Shorthand for Len applied to Param.
type ParamLen struct {
	Index int // 0-based parameter index
}

func (ParamLen) exprNode()        {}
func (p ParamLen) String() string { return "len(" + paramExprKey(p.Index) + ")" }

func (p ParamLen) Substitute(subst map[string]Expr) Expr {
	key := paramLenExprKey(p.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return p
}

func (p ParamLen) Variables() []string {
	return []string{paramLenExprKey(p.Index)}
}

func (p ParamLen) Eval(env map[string]int64) (int64, bool) {
	key := paramLenExprKey(p.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// RetLen represents the length of a function return value.
// Shorthand for Len applied to Ret.
type RetLen struct {
	Index int // 0-based return value index
}

func (RetLen) exprNode()        {}
func (r RetLen) String() string { return "len(" + retExprKey(r.Index) + ")" }

func (r RetLen) Substitute(subst map[string]Expr) Expr {
	key := retLenExprKey(r.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return r
}

func (r RetLen) Variables() []string {
	return []string{retLenExprKey(r.Index)}
}

func (r RetLen) Eval(env map[string]int64) (int64, bool) {
	key := retLenExprKey(r.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

func paramExprKey(index int) string {
	return "param[" + strconv.Itoa(index) + "]"
}

func retExprKey(index int) string {
	return "ret[" + strconv.Itoa(index) + "]"
}

func paramLenExprKey(index int) string {
	return paramExprKey(index) + ".len"
}

func retLenExprKey(index int) string {
	return retExprKey(index) + ".len"
}
