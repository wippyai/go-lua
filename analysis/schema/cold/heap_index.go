package cold

import "github.com/wippyai/go-lua/analysis/identity"

// HeapIndexLens ordinals preserve the canonical exact/dynamic key shape.
const (
	HeapIndexLensExact   uint8 = 1
	HeapIndexLensDynamic uint8 = 2
)

// HeapIndex is one scalar index-read/index-write candidate. A read carries a
// result span and no Values payload; a write carries Values geometry and its
// exact position, matching HeapIndexRow's closed shape.
type HeapIndex struct {
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

// NewHeapIndex copies one canonical HeapIndexRow without importing Program's
// keyspace.Key type; exact keys retain their historical uint32 values in a
// uint64 scalar carrier for parity with the ingress neutral row.
func NewHeapIndex(id identity.ContentID, read bool, baseSpan, resultSpan, keySpan identity.ContentID, lensKind uint8, exactKey uint64, valuesSpan, valuesID identity.ContentID, position int) (HeapIndex, bool) {
	row := HeapIndex{
		id: id, read: read, baseSpan: baseSpan, resultSpan: resultSpan,
		keySpan: keySpan, lensKind: lensKind, exactKey: exactKey,
		valuesSpan: valuesSpan, valuesID: valuesID, position: position,
	}
	return row, row.Available()
}

func (row HeapIndex) Available() bool {
	if !row.id.Available() || !row.baseSpan.Available() {
		return false
	}
	if row.lensKind == HeapIndexLensExact {
		if row.exactKey == 0 {
			return false
		}
	} else if row.lensKind == HeapIndexLensDynamic {
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

func (row HeapIndex) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row HeapIndex) Read() bool { return row.Available() && row.read }

func (row HeapIndex) BaseSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.baseSpan
}

func (row HeapIndex) ResultSpan() identity.ContentID {
	if !row.Available() || !row.read {
		return identity.ContentID{}
	}
	return row.resultSpan
}

func (row HeapIndex) DynamicKeySpan() identity.ContentID {
	if !row.Available() || row.lensKind != HeapIndexLensDynamic {
		return identity.ContentID{}
	}
	return row.keySpan
}

func (row HeapIndex) ExactKey() (uint64, bool) {
	if !row.Available() || row.lensKind != HeapIndexLensExact {
		return 0, false
	}
	return row.exactKey, true
}

func (row HeapIndex) Values() (identity.ContentID, int, bool) {
	if !row.Available() || row.read {
		return identity.ContentID{}, 0, false
	}
	return row.valuesSpan, row.position, true
}

func (row HeapIndex) ValuesID() identity.ContentID {
	if !row.Available() || row.read {
		return identity.ContentID{}
	}
	return row.valuesID
}
