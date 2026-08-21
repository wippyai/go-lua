// Package heapindex owns the canonical cold row and family binding for one
// Program heap index access.
package heapindex

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

// Lens ordinals preserve the canonical exact/dynamic key shape.
const (
	LensExact   uint8 = 1
	LensDynamic uint8 = 2
)

// Index is one scalar index-read/index-write candidate. A read carries a
// result span and no Values payload; a write carries Values geometry and its
// exact position, matching the cold Program row shape.
type Index struct {
	id         identity.ContentID
	read       bool
	baseSpan   identity.ContentID
	resultSpan identity.ContentID
	keySpan    identity.ContentID
	lensKind   uint8
	exactKey   uint64
	valuesSpan identity.ContentID
	valuesID   identity.ContentID
	position   int
}

// NewIndex copies one canonical index row without importing Program's
// keyspace.Key type; exact keys retain their historical uint32 values in a
// uint64 scalar carrier for parity with the ingress neutral row.
func NewIndex(id identity.ContentID, read bool, baseSpan, resultSpan, keySpan identity.ContentID, lensKind uint8, exactKey uint64, valuesSpan, valuesID identity.ContentID, position int) (Index, bool) {
	row := Index{id: id, read: read, baseSpan: baseSpan, resultSpan: resultSpan, keySpan: keySpan, lensKind: lensKind, exactKey: exactKey, valuesSpan: valuesSpan, valuesID: valuesID, position: position}
	return row, row.Available()
}

func (row Index) Available() bool {
	if !row.id.Available() || !row.baseSpan.Available() {
		return false
	}
	if row.lensKind == LensExact {
		if row.exactKey == 0 {
			return false
		}
	} else if row.lensKind == LensDynamic {
		if !row.keySpan.Available() {
			return false
		}
	} else {
		return false
	}
	if row.read {
		return row.resultSpan.Available() && !row.valuesSpan.Available() && !row.valuesID.Available() && row.position == -1
	}
	return !row.resultSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.position >= 0
}

func (row Index) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Index) Read() bool { return row.Available() && row.read }

func (row Index) BaseSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.baseSpan
}

func (row Index) ResultSpan() identity.ContentID {
	if !row.Available() || !row.read {
		return identity.ContentID{}
	}
	return row.resultSpan
}

func (row Index) DynamicKeySpan() identity.ContentID {
	if !row.Available() || row.lensKind != LensDynamic {
		return identity.ContentID{}
	}
	return row.keySpan
}

func (row Index) ExactKey() (uint64, bool) {
	if !row.Available() || row.lensKind != LensExact {
		return 0, false
	}
	return row.exactKey, true
}

func (row Index) Values() (identity.ContentID, int, bool) {
	if !row.Available() || row.read {
		return identity.ContentID{}, 0, false
	}
	return row.valuesSpan, row.position, true
}

func (row Index) ValuesID() identity.ContentID {
	if !row.Available() || row.read {
		return identity.ContentID{}
	}
	return row.valuesID
}

// LensKind is the retained exact/dynamic key-shape ordinal. It is part of the
// row's published geometry rather than a derived property.
func (row Index) LensKind() uint8 {
	if !row.Available() {
		return 0
	}
	return row.lensKind
}

// Position is the exact write position, and -1 for a read.
func (row Index) Position() int {
	if !row.Available() {
		return 0
	}
	return row.position
}

// Family is the canonical catalog binding for index rows.
func Family() programfamily.Family[Index] {
	return programfamily.New[Index](programcatalog.HeapIndex())
}
