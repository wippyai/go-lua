package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// LocalTransfer is one immutable acyclic stage transport in a compiled
// Program. Full transports carry the complete environment. Factor transports
// name a contiguous span in LocalTransferWriteFamily whose rows carry the
// ordered schema keys they move.
type LocalTransfer struct {
	id          identity.ContentID
	from        identity.ContentID
	to          identity.ContentID
	full        bool
	writeOffset uint32
	writeCount  uint32
}

// NewLocalTransfer copies one compiler transport into the canonical Program
// vocabulary. Full transports have an empty write span; factor transports
// name a non-empty span.
func NewLocalTransfer(id, from, to identity.ContentID, full bool, writeOffset, writeCount uint32) (LocalTransfer, bool) {
	row := LocalTransfer{id: id, from: from, to: to, full: full, writeOffset: writeOffset, writeCount: writeCount}
	return row, row.Available()
}

func (row LocalTransfer) Available() bool {
	return row.id.Available() && row.from.Available() && row.to.Available() && row.from != row.to &&
		row.full == (row.writeCount == 0) && uint64(row.writeOffset)+uint64(row.writeCount) <= uint64(^uint32(0))
}

func (row LocalTransfer) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row LocalTransfer) From() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.from
}

func (row LocalTransfer) To() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.to
}

func (row LocalTransfer) Full() bool { return row.Available() && row.full }

func (row LocalTransfer) WritesCount() int {
	if !row.Available() || row.full {
		return 0
	}
	return int(row.writeCount)
}

func (row LocalTransfer) WriteSpan() (offset, count uint32, ok bool) {
	return row.writeOffset, row.writeCount, row.Available()
}

// LocalTransferWrite is one ordered factor key moved by a partial local
// transport. Its ordinal is its position in LocalTransferWriteFamily.
type LocalTransferWrite struct{ key schema.Key }

func NewLocalTransferWrite(key schema.Key) (LocalTransferWrite, bool) {
	row := LocalTransferWrite{key: key}
	return row, row.Available()
}

func (row LocalTransferWrite) Available() bool { return row.key.Available() }

func (row LocalTransferWrite) Key() (schema.Key, bool) { return row.key, row.Available() }
