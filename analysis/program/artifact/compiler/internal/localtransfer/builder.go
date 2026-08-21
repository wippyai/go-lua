// Package localtransfer owns the compiler's private local-transport assembly
// state. It derives transport identities from the canonical write set, closes
// the rows in (From, To, ID) order, and transfers the resulting dense schema
// planes exactly once. The parent compiler owns neither the draft vocabulary
// nor the conversion loop.
package localtransfer

import (
	"github.com/wippyai/go-lua/analysis/identity"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// Fault identifies a transport row that prevented canonical closure. Index is
// the row's position after the canonical sort, matching the compiler's old
// duplicate-row failure coordinate.
type Fault struct {
	index  int
	failed bool
}

func (fault Fault) Failed() bool { return fault.failed }
func (fault Fault) Index() int   { return fault.index }

type draft struct {
	id     identity.ContentID
	from   identity.ContentID
	to     identity.ContentID
	full   bool
	writes []schema.Key
}

func (row draft) available() bool {
	if !row.id.Available() || !row.from.Available() || !row.to.Available() || row.from == row.to || row.full == (len(row.writes) != 0) {
		return false
	}
	for index, write := range row.writes {
		if !write.Available() || index != 0 && row.writes[index-1] >= write {
			return false
		}
	}
	return true
}

// Builder is the one-shot mutable owner of local-transfer drafts.
type Builder struct {
	format      uint64
	rows        []draft
	sealed      bool
	faulted     bool
	transferred bool
}

// New creates an empty local-transfer owner for one artifact format.
func New(format uint64) *Builder { return &Builder{format: format} }

// Append admits one transport, canonicalizing its write set before deriving
// the content identity. A false result preserves the parent compiler's
// occurrence-stage environment-unavailable failure mapping.
func (builder *Builder) Append(domain string, from, to identity.ContentID, full bool, writes ...schema.Key) bool {
	if builder == nil || builder.sealed || builder.transferred {
		return false
	}
	ordered, orderedOK := issuance.OrderedKeys(writes)
	if !orderedOK {
		return false
	}
	fields := []artifactdigest.Field{
		artifactdigest.ContentID(from), artifactdigest.ContentID(to),
		artifactdigest.Bool(full), artifactdigest.Uint(uint64(len(ordered))),
	}
	for _, write := range ordered {
		fields = append(fields, artifactdigest.Key(write))
	}
	row := draft{
		id:     artifactdigest.Digest(domain, builder.format, fields...),
		from:   from,
		to:     to,
		full:   full,
		writes: ordered,
	}
	if !row.available() {
		return false
	}
	builder.rows = append(builder.rows, row)
	return true
}

// Seal canonically orders the drafts and rejects duplicate transport IDs. The
// least-significant key is applied first so the final order is (From, To, ID),
// exactly matching the former compiler finalizer.
func (builder *Builder) Seal() Fault {
	if builder == nil || builder.sealed || builder.transferred {
		return Fault{index: -1, failed: true}
	}
	identity.SortByContentID(builder.rows, func(row draft) identity.ContentID { return row.id })
	identity.SortByContentID(builder.rows, func(row draft) identity.ContentID { return row.to })
	identity.SortByContentID(builder.rows, func(row draft) identity.ContentID { return row.from })
	for index := 1; index < len(builder.rows); index++ {
		if builder.rows[index-1].id == builder.rows[index].id {
			builder.sealed = true
			builder.faulted = true
			return Fault{index: index, failed: true}
		}
	}
	builder.sealed = true
	return Fault{}
}

// TakeCanonicalPlanes validates and transfers the one canonical transport and
// write plane. It succeeds only after Seal and only once.
func (builder *Builder) TakeCanonicalPlanes() ([]programschema.LocalTransfer, []programschema.LocalTransferWrite, bool) {
	if builder == nil || !builder.sealed || builder.faulted || builder.transferred {
		return nil, nil, false
	}
	transfers := make([]programschema.LocalTransfer, 0, len(builder.rows))
	writes := make([]programschema.LocalTransferWrite, 0)
	for _, row := range builder.rows {
		if !row.available() || !fitsUint32(len(writes)) || !fitsUint32(len(row.writes)) {
			return nil, nil, false
		}
		offset := uint32(len(writes))
		for _, key := range row.writes {
			write, ok := programschema.NewLocalTransferWrite(key)
			if !ok {
				return nil, nil, false
			}
			writes = append(writes, write)
		}
		converted, ok := programschema.NewLocalTransfer(row.id, row.from, row.to, row.full, offset, uint32(len(row.writes)))
		if !ok {
			return nil, nil, false
		}
		transfers = append(transfers, converted)
	}
	builder.rows = nil
	builder.transferred = true
	return transfers, writes, true
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }
