package subst

import (
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SelfRef replaces self-referential local type references in a method type with
// the concrete receiver type.
//
// A colon-method declared as m(self: R, ...) -> R names its receiver with an
// explicit local Ref ("R") rather than the Self keyword. When the receiver value
// is materialized structurally (its alias name dropped), that Ref is unresolvable
// at the call site and the result collapses to Top. The method's first parameter
// (its self binding) carries the receiver Ref node. That exact Ref is replaced
// with receiverType; unrelated references with the same name are left untouched.
//
// Substitution is skipped when the self parameter is not a local Ref (the Self
// keyword is handled by Self) or when the receiver type is missing.
func SelfRef(method *typ.Function, receiverType typ.Type) typ.Type {
	if method == nil || receiverType == nil {
		return method
	}
	selfRef, ok := selfParamRef(method)
	if !ok {
		return method
	}
	return transform.Rewrite(method, func(n typ.Type) (typ.Type, bool) {
		if ref, ok := n.(*typ.Ref); ok && ref == selfRef {
			return receiverType, true
		}
		return nil, false
	})
}

// selfParamRef returns the local Ref naming the receiver from the method's first
// (self) parameter, if the self binding is an explicit local Ref.
func selfParamRef(method *typ.Function) (*typ.Ref, bool) {
	if method == nil || len(method.Params) == 0 {
		return nil, false
	}
	ref, ok := method.Params[0].Type.(*typ.Ref)
	if !ok || ref.Module != "" || ref.Name == "" {
		return nil, false
	}
	return ref, true
}
