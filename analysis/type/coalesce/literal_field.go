package coalesce

import (
	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func joinNonDiscriminantField(a, b typ.Type) (typ.Type, bool) {
	if identity.SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
	al, aOK := literal.ExtractAliasOnly(a)
	bl, bOK := literal.ExtractAliasOnly(b)
	if aOK && bOK && al.Base == bl.Base {
		if al.Value == bl.Value {
			return a, true
		}
		return literal.PrimitiveBase(al), true
	}
	left, ok := literal.FamilyBase(a)
	if !ok {
		return nil, false
	}
	right, ok := literal.FamilyBase(b)
	if !ok {
		return nil, false
	}
	return literal.MergeFamilyBases(left, right)
}
