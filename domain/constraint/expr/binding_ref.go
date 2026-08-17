package expr

import "strconv"

type bindingRefKind uint8

const (
	bindingRefVar bindingRefKind = iota + 1
	bindingRefParam
	bindingRefRet
)

// bindingRef owns the string boundary for symbolic references in constraint
// expressions. The solver API still uses string-keyed environments, but the
// spelling of "x", "x.len", "param[0]", and "len(param[0])" lives here.
type bindingRef struct {
	kind   bindingRefKind
	name   string
	index  int
	length bool
}

func varBindingRef(name string) bindingRef {
	return bindingRef{kind: bindingRefVar, name: name}
}

func lenBindingRef(name string) bindingRef {
	return bindingRef{kind: bindingRefVar, name: name, length: true}
}

func paramBindingRef(index int) bindingRef {
	return bindingRef{kind: bindingRefParam, index: index}
}

func paramLenBindingRef(index int) bindingRef {
	return bindingRef{kind: bindingRefParam, index: index, length: true}
}

func retBindingRef(index int) bindingRef {
	return bindingRef{kind: bindingRefRet, index: index}
}

func retLenBindingRef(index int) bindingRef {
	return bindingRef{kind: bindingRefRet, index: index, length: true}
}

func (r bindingRef) String() string {
	base := r.baseKey()
	if r.length {
		return "len(" + base + ")"
	}
	return base
}

func (r bindingRef) Key() string {
	base := r.baseKey()
	if r.length {
		return base + ".len"
	}
	return base
}

func (r bindingRef) Substitute(subst map[string]Expr, self Expr) Expr {
	if e, ok := subst[r.Key()]; ok {
		return e
	}
	return self
}

func (r bindingRef) Variables() []string {
	return []string{r.Key()}
}

func (r bindingRef) Eval(env map[string]int64) (int64, bool) {
	val, ok := env[r.Key()]
	if !ok {
		return 0, false
	}
	return val, true
}

func (r bindingRef) baseKey() string {
	switch r.kind {
	case bindingRefParam:
		return "param[" + strconv.Itoa(r.index) + "]"
	case bindingRefRet:
		return "ret[" + strconv.Itoa(r.index) + "]"
	default:
		return r.name
	}
}
