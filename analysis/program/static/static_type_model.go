package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
	for _, candidate := range staticTypeFamilies {
		if family == candidate {
			ordinal := keyspace.TermOrdinal(term)
			return ordinal != 0 && uint64(ordinal) <= uint64(component.census[family])
		}
	}
	return false
}

// StaticTypeTermCount returns the complete finite authored static-type
// authority. It sums the closed forest window of the census column Build
// sealed once, so it visits a fixed number of families and allocates nothing.
func (component *Component) StaticTypeTermCount() int {
	if component == nil {
		return 0
	}
	total := 0
	for _, family := range staticTypeFamilies {
		total += int(component.census[family])
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
	for _, family := range staticTypeFamilies {
		count := uint64(component.census[family])
		if offset < count {
			return keyspace.MakeTerm(family, uint32(offset+1)), true
		}
		offset -= count
	}
	return 0, false
}
