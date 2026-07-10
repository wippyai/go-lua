package expr

// Var represents a symbolic variable (e.g., "i", "len_arr").
type Var struct {
	Name string
}

func (Var) exprNode()        {}
func (v Var) String() string { return varBindingRef(v.Name).String() }

func (v Var) Substitute(subst map[string]Expr) Expr {
	return varBindingRef(v.Name).Substitute(subst, v)
}

func (v Var) Variables() []string {
	return varBindingRef(v.Name).Variables()
}

func (v Var) Eval(env map[string]int64) (int64, bool) {
	return varBindingRef(v.Name).Eval(env)
}

// Len represents the length of a symbolic array/tuple.
type Len struct {
	Of string // variable name of the array
}

func (Len) exprNode()        {}
func (l Len) String() string { return lenBindingRef(l.Of).String() }

func (l Len) Substitute(subst map[string]Expr) Expr {
	return lenBindingRef(l.Of).Substitute(subst, l)
}

func (l Len) Variables() []string {
	return lenBindingRef(l.Of).Variables()
}

func (l Len) Eval(env map[string]int64) (int64, bool) {
	return lenBindingRef(l.Of).Eval(env)
}

// Param represents a reference to a function parameter by index.
// Used in function refinements: len(Param(0)) = 5
type Param struct {
	Index int // 0-based parameter index
}

func (Param) exprNode()        {}
func (p Param) String() string { return paramBindingRef(p.Index).String() }

func (p Param) Substitute(subst map[string]Expr) Expr {
	return paramBindingRef(p.Index).Substitute(subst, p)
}

func (p Param) Variables() []string {
	return paramBindingRef(p.Index).Variables()
}

func (p Param) Eval(env map[string]int64) (int64, bool) {
	return paramBindingRef(p.Index).Eval(env)
}

// Ret represents a reference to a function return value by index.
// Used in function refinements: len(Ret(0)) = len(Param(0))
type Ret struct {
	Index int // 0-based return value index
}

func (Ret) exprNode()        {}
func (r Ret) String() string { return retBindingRef(r.Index).String() }

func (r Ret) Substitute(subst map[string]Expr) Expr {
	return retBindingRef(r.Index).Substitute(subst, r)
}

func (r Ret) Variables() []string {
	return retBindingRef(r.Index).Variables()
}

func (r Ret) Eval(env map[string]int64) (int64, bool) {
	return retBindingRef(r.Index).Eval(env)
}

// ParamLen represents the length of a function parameter.
// Shorthand for Len applied to Param.
type ParamLen struct {
	Index int // 0-based parameter index
}

func (ParamLen) exprNode()        {}
func (p ParamLen) String() string { return paramLenBindingRef(p.Index).String() }

func (p ParamLen) Substitute(subst map[string]Expr) Expr {
	return paramLenBindingRef(p.Index).Substitute(subst, p)
}

func (p ParamLen) Variables() []string {
	return paramLenBindingRef(p.Index).Variables()
}

func (p ParamLen) Eval(env map[string]int64) (int64, bool) {
	return paramLenBindingRef(p.Index).Eval(env)
}

// RetLen represents the length of a function return value.
// Shorthand for Len applied to Ret.
type RetLen struct {
	Index int // 0-based return value index
}

func (RetLen) exprNode()        {}
func (r RetLen) String() string { return retLenBindingRef(r.Index).String() }

func (r RetLen) Substitute(subst map[string]Expr) Expr {
	return retLenBindingRef(r.Index).Substitute(subst, r)
}

func (r RetLen) Variables() []string {
	return retLenBindingRef(r.Index).Variables()
}

func (r RetLen) Eval(env map[string]int64) (int64, bool) {
	return retLenBindingRef(r.Index).Eval(env)
}
