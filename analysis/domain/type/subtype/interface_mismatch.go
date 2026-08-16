package subtype

import (
	"github.com/wippyai/go-lua/analysis/domain/type/subst"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// InterfaceMismatchKind classifies why a record fails to implement an
// interface. It is a proof explanation for callers that already know the
// ordinary subtype check failed; it does not change subtype semantics.
type InterfaceMismatchKind uint8

const (
	InterfaceMismatchMissingMethod InterfaceMismatchKind = iota + 1
	InterfaceMismatchMethodType
)

// InterfaceMismatch records the first required interface method a record fails
// to satisfy.
type InterfaceMismatch struct {
	Kind     InterfaceMismatchKind
	Method   typ.Method
	Actual   typ.Type
	Expected typ.Type
}

// RecordInterfaceMismatch explains the first direct record-to-interface
// structural mismatch. It intentionally stays narrow: callers use it to carry
// evidence for diagnostics, while IsSubtype remains the authority for truth.
func RecordInterfaceMismatch(sub, super typ.Type) (InterfaceMismatch, bool) {
	rec, _ := unwrap.Alias(sub).(*typ.Record)
	if rec == nil {
		return InterfaceMismatch{}, false
	}
	iface, _ := unwrap.Alias(super).(*typ.Interface)
	if iface == nil {
		return InterfaceMismatch{}, false
	}
	c := &checker{}
	for _, method := range iface.Methods {
		field := rec.GetField(method.Name)
		checkExpected := subst.Self(method.Type, rec)
		if field == nil {
			return InterfaceMismatch{
				Kind:     InterfaceMismatchMissingMethod,
				Method:   method,
				Expected: method.Type,
			}, true
		}
		if !c.check(field.Type, checkExpected) {
			return InterfaceMismatch{
				Kind:     InterfaceMismatchMethodType,
				Method:   method,
				Actual:   field.Type,
				Expected: method.Type,
			}, true
		}
	}
	return InterfaceMismatch{}, false
}
