package coalesce

import (
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func joinNonDiscriminantField(a, b typ.Type) (typ.Type, bool) {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
	al, aOK := literal.ExtractAliasOnly(a)
	bl, bOK := literal.ExtractAliasOnly(b)
	if aOK && bOK && al.Base == bl.Base {
		if typ.LiteralEquals(al, bl) {
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
