package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

func (c *checker) checkFunction(sub, super *typ.Function, depth int) bool {
	subReq := minRequiredArgs(sub)
	superReq := minRequiredArgs(super)
	if subReq > superReq || (super.Variadic == nil && subReq > len(super.Params)) {
		return false
	}
	if sub.Variadic == nil && len(super.Params) > len(sub.Params) {
		return false
	}

	maxParams := len(sub.Params)
	if len(super.Params) > maxParams {
		maxParams = len(super.Params)
	}
	for i := 0; i < maxParams; i++ {
		var subT, superT typ.Type
		if i < len(sub.Params) {
			subT = sub.Params[i].Type
		} else if sub.Variadic != nil {
			subT = sub.Variadic
		}
		if i < len(super.Params) {
			superT = super.Params[i].Type
		} else if super.Variadic != nil {
			superT = super.Variadic
		}
		if subT == nil || superT == nil {
			continue
		}
		if !c.check(superT, subT, depth+1) {
			return false
		}
	}
	if sub.Variadic != nil && super.Variadic != nil {
		if !c.check(super.Variadic, sub.Variadic, depth+1) {
			return false
		}
	}
	for i := 0; i < len(super.Returns); i++ {
		subReturn := typ.Nil
		if i < len(sub.Returns) {
			subReturn = sub.Returns[i]
		}
		if !c.check(subReturn, super.Returns[i], depth+1) {
			return false
		}
	}
	return true
}

func minRequiredArgs(fn *typ.Function) int {
	if fn == nil {
		return 0
	}
	required := 0
	for i, p := range fn.Params {
		if !p.Optional {
			required = i + 1
		}
	}
	return required
}
