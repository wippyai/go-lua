package expr

import "fmt"

// Min represents the minimum of two expressions.
type Min struct {
	Left  Expr
	Right Expr
}

func (Min) exprNode()        {}
func (m Min) String() string { return fmt.Sprintf("min(%s, %s)", m.Left, m.Right) }

func (m Min) Substitute(subst map[string]Expr) Expr {
	return Min{
		Left:  m.Left.Substitute(subst),
		Right: m.Right.Substitute(subst),
	}
}

func (m Min) Variables() []string {
	return collectVars(m.Left, m.Right)
}

func (m Min) Eval(env map[string]int64) (int64, bool) {
	left, ok := m.Left.Eval(env)
	if !ok {
		return 0, false
	}

	right, ok := m.Right.Eval(env)
	if !ok {
		return 0, false
	}

	if left < right {
		return left, true
	}

	return right, true
}

// Max represents the maximum of two expressions.
type Max struct {
	Left  Expr
	Right Expr
}

func (Max) exprNode()        {}
func (m Max) String() string { return fmt.Sprintf("max(%s, %s)", m.Left, m.Right) }

func (m Max) Substitute(subst map[string]Expr) Expr {
	return Max{
		Left:  m.Left.Substitute(subst),
		Right: m.Right.Substitute(subst),
	}
}

func (m Max) Variables() []string {
	return collectVars(m.Left, m.Right)
}

func (m Max) Eval(env map[string]int64) (int64, bool) {
	left, ok := m.Left.Eval(env)
	if !ok {
		return 0, false
	}

	right, ok := m.Right.Eval(env)
	if !ok {
		return 0, false
	}

	if left > right {
		return left, true
	}

	return right, true
}
