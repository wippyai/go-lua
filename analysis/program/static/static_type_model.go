package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// ContentID returns the sealed authored Static identity. A zero Component or
// an unsealed component fails closed with an unavailable identity.
func (component *Component) ContentID() identity.ContentID {
	if component == nil {
		return identity.ContentID{}
	}
	return component.contentID
}

// StaticTypeTerm reports whether term is one of the authored static type
// roots. It is a family/ordinal membership check against the sealed census
// column; it does not scan or materialize the enumeration.
func (component *Component) StaticTypeTerm(term keyspace.Term) bool {
	if component == nil || !component.contentID.Available() {
		return false
	}
	family := keyspace.TermFamily(term)
	if !staticTypeFamily(family) {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(component.census[family])
}

// StaticTypeTermCount returns the complete finite authored static-type
// authority. It sums the closed forest window of the census column Build
// sealed once, so it visits a fixed number of families and allocates nothing.
func (component *Component) StaticTypeTermCount() int {
	if component == nil {
		return 0
	}
	total := 0
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if staticTypeFamily(family) {
			total += int(component.census[family])
		}
	}
	return total
}

// StaticTypeTermAt returns the stable derived order of the authored static
// type forest. It returns no bare term outside the sealed enumeration.
func (component *Component) StaticTypeTermAt(index int) (keyspace.Term, bool) {
	if component == nil || index < 0 {
		return 0, false
	}
	offset := uint64(index)
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if !staticTypeFamily(family) {
			continue
		}
		count := uint64(component.census[family])
		if offset < count {
			return keyspace.MakeTerm(family, uint32(offset+1)), true
		}
		offset -= count
	}
	return 0, false
}

// staticTypeFamily is the canonical typed-family vocabulary used by Static's
// forest queries. Declaration roots come from the role owner that admits
// declaration targets; expression families come from the role owner that
// admits static type nodes. The numeric walk preserves the stable keyspace
// order without another inventory or offset table.
func staticTypeFamily(family keyspace.Family) bool {
	return staticrole.TypeReferenceTargetFamily(family) || staticrole.NodeFamily(family)
}
